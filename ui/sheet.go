// Copyright © 2016, The T Authors.

package ui

import (
	"image"
	"image/color"
	"image/draw"
	"net/url"
	"sync"

	"github.com/eaburns/T/edit"
	"github.com/eaburns/T/ui/text"
	"golang.org/x/exp/shiny/screen"
	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/mouse"
	"golang.org/x/mobile/event/paint"
)

var (
	separatorColor = color.Gray16{0xAAAA}
	tagColors      = []color.Color{
		color.NRGBA{R: 0xE6, G: 0xF0, B: 0xFA, A: 0xFF},
		color.NRGBA{R: 0xE6, G: 0xFA, B: 0xF0, A: 0xFF},
		color.NRGBA{R: 0xF0, G: 0xE6, B: 0xFA, A: 0xFF},
		color.NRGBA{R: 0xF0, G: 0xFA, B: 0xE6, A: 0xFF},
		color.NRGBA{R: 0xFA, G: 0xE6, B: 0xF0, A: 0xFF},
	}
	mu           sync.Mutex
	nextTagColor = 0
)

const sheetTagText = "Get Undo Look"

// A sheet is an editable view of a buffer of text.
// Each sheet contains an editable tag and body.
// The tag is a, typically short, header,
// beginning with the name of the sheet's file (if any)
// followed by various commands to operate on the sheet.
// The body contains the body text of the sheet.
type sheet struct {
	id  string
	col *column
	win *window
	image.Rectangle

	tag  *textBox
	body *textBox
	sep  image.Rectangle

	// SubFocus is either the tag, the body, or nil.
	subFocus handler

	p      image.Point
	button mouse.Button

	origX int
	origY float64
}

// NewSheet creates a new sheet.
// URL is either the root path to an editor server,
// or the path to an open buffer of an editor server.
// The body uses the given URL for its buffer (either a new one or existing).
// The tag uses a new buffer created on the window server's editor.
func newSheet(id string, URL *url.URL, w *window) (*sheet, error) {
	s := &sheet{id: id, win: w}

	mu.Lock()
	tagBG := tagColors[nextTagColor%len(tagColors)]
	nextTagColor++
	mu.Unlock()

	tag, err := newTextBox(w, *w.server.editorURL, text.Style{
		Face: w.face,
		FG:   color.Black,
		BG:   tagBG,
	})
	if err != nil {
		return nil, err
	}
	tag.view.Do(nil,
		edit.Change(edit.All, "/sheet/"+id+" "+sheetTagText+" "),
		edit.Set(edit.End, '.'))
	s.tag = tag

	body, err := newTextBox(w, *URL, text.Style{
		Face: w.face,
		FG:   color.Black,
		BG:   color.NRGBA{R: 0xFA, G: 0xF0, B: 0xE6, A: 0xFF},
	})
	if err != nil {
		tag.close()
		return nil, err
	}
	s.body = body

	return s, nil
}

func (s *sheet) close() {
	if s.win == nil {
		// Already closed.
		// This can happen if the sheet is in focus when the window is closed.
		// The in-focus handler is closed, and so are all columns.
		return
	}
	s.tag.close()
	s.body.close()
	s.win = nil
}

func (s *sheet) updateText() {
	b := &s.Rectangle

	tagMax := b.Dy()
	if min := s.minHeight(); tagMax < min {
		tagMax = min
	}
	// TODO(eaburns): This is awful; it's too easy to forget to set topLeft,
	// it's too unintuitive that setSize needs to be called to update the text.
	s.tag.topLeft = b.Min
	s.tag.setSize(image.Pt(b.Dx(), tagMax))
	tagHeight := s.tag.text.LinesHeight()

	s.body.topLeft = image.Pt(b.Min.X, b.Min.Y+tagHeight+borderWidth)
	s.body.setSize(image.Pt(b.Dx(), b.Dy()-tagHeight-borderWidth))

	s.sep = image.Rectangle{
		Min: image.Pt(b.Min.X, b.Min.Y+tagHeight),
		Max: image.Pt(b.Max.X, b.Min.Y+tagHeight+borderWidth),
	}
}

func (s *sheet) minHeight() int { return minHeight(s.tag.opts) }

func (s *sheet) bounds() image.Rectangle { return s.Rectangle }

func (s *sheet) setBounds(b image.Rectangle) {
	s.Rectangle = b
	s.updateText()
}

func (s *sheet) setColumn(c *column) { s.col = c }

func (s *sheet) focus(p image.Point) handler {
	prev := s.subFocus
	if p.Y < s.sep.Min.Y {
		s.subFocus = s.tag
	} else if p.Y >= s.sep.Max.Y {
		s.subFocus = s.body
	}
	if s.subFocus != prev {
		if prev != nil {
			prev.changeFocus(s.win, false)
		}
		if s.subFocus != nil {
			s.subFocus.changeFocus(s.win, true)
		}
		// Always redraw on focus change.
		s.win.Send(paint.Event{})
	}
	return s
}

func (s *sheet) draw(scr screen.Screen, win screen.Window) {
	s.updateText()

	s.tag.drawLines(scr, win)
	win.Fill(s.sep, separatorColor, draw.Over)
	s.body.draw(scr, win)
}

// DrawLast is called if the sheet is in focus, after the entire window has been drawn.
// It draws the sheet if being dragged.
func (s *sheet) drawLast(scr screen.Screen, win screen.Window) {
	if s.col == nil {
		s.draw(scr, win)
		drawBorder(s.bounds(), win)
	}
}

func (s *sheet) changeFocus(win *window, inFocus bool) {
	if s.subFocus != nil {
		s.subFocus.changeFocus(win, inFocus)
	}
}

func (s *sheet) tick(win *window) bool {
	if s.subFocus != nil {
		return s.subFocus.tick(win)
	}
	return false
}

func (s *sheet) key(w *window, event key.Event) bool {
	var redraw bool
	switch event.Code {
	case key.CodeLeftShift, key.CodeRightShift:
		if event.Direction == key.DirRelease && s.col == nil {
			// We were dragging, and shift was released. Put it back.
			if _, c := columnAt(w, s.origX); !c.addFrame(s.origY, s) {
				panic("can't put it back")
			}
			redraw = true
		}
	}
	if s.subFocus != nil && s.subFocus.key(w, event) {
		redraw = true
	}
	return redraw
}

func (s *sheet) mouse(w *window, event mouse.Event) bool {
	p := image.Pt(int(event.X), int(event.Y))

	switch event.Direction {
	case mouse.DirPress:
		if s.button == mouse.ButtonNone {
			s.p = p
			s.button = event.Button
			break
		}
		// A second button was pressed while the first was held.
		// Sheets don't use chords; treat this as a release of the first.
		event.Button = s.button
		fallthrough

	case mouse.DirRelease:
		if event.Button != s.button {
			// It's not the pressed button. Ignore it.
			break
		}
		defer func() { s.button = mouse.ButtonNone }()

		if event.Modifiers != key.ModShift {
			break
		}
		switch s.button {
		case mouse.ButtonLeft:
			if s.col != nil {
				defer func() { s.col.setBounds(s.col.bounds()) }()
				i := frameIndex(s.col, s)
				if slideUp(s.col, i, s.minHeight()) {
					return true
				}
				return slideDown(s.col, i, s.minHeight())
			}
			_, c := columnAt(w, p.X)
			yfrac := float64(s.Min.Y) / float64(c.Dy())
			if c.addFrame(yfrac, s) {
				return true
			}
			if _, c = columnAt(w, s.origX); !c.addFrame(s.origY, s) {
				panic("can't put it back")
			}
			return true
		case mouse.ButtonMiddle:
			s.win.server.delSheet(s.id)
			return false
		}

	case mouse.DirNone:
		if s.button == mouse.ButtonNone || event.Modifiers != key.ModShift {
			break
		}
		switch s.button {
		case mouse.ButtonLeft:
			if s.col == nil {
				s.setBounds(s.Add(p.Sub(s.Min)))
				return true
			}
			dx := s.p.X - p.X
			dy := s.p.Y - p.Y
			if dx*dx+dy*dy > 100 {
				s.p = p
				i := frameIndex(s.col, s)
				if i < 0 {
					return false
				}
				s.origX = s.Min.X + s.Dx()/2
				s.origY = s.col.ys[i]
				s.col.removeFrame(s)
				return true
			}
		}
	}

	if s.subFocus != nil {
		return s.subFocus.mouse(w, event)
	}
	return false
}
