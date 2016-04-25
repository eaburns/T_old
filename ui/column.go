// Copyright © 2016, The T Authors.

package ui

import (
	"image"
	"image/color"
	"image/draw"

	"github.com/eaburns/T/edit"
	"github.com/eaburns/T/ui/text"
	"golang.org/x/exp/shiny/screen"
	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/mouse"
)

// A frame is a rectangular section of a win that can be attached to a column.
type frame interface {
	// Bounds returns the current bounds of the frame.
	bounds() image.Rectangle

	// SetBounds sets the bounds of the frame to the given rectangle
	setBounds(image.Rectangle)

	// SetColumn sets the frame's column.
	setColumn(*column)

	// Focus returns the handler that is in focus at the given coordinate.
	// The upper-left of the frame is the Min point of its bounds.
	focus(image.Point) handler

	// Draw draws the frame to the window.
	draw(scr screen.Screen, win screen.Window)

	// Close closes the frame.
	// It is called by the containing object when that object has been removed.
	// Close should release the resources of the frame.
	// It should not remove the frame from its containing object,
	// because close is only intended to be called
	// after the frame has been removed from its container.
	close()

	// MinHeight returns the frame's minimum recommended height in pixels.
	// If the window is smaller than minHeight, the frame may also be smaller.
	minHeight() int
}

type column struct {
	win *window
	image.Rectangle
	frames []frame
	ys     []float64
}

// NewColumn returns a new column, with a body, but no window or bounds.
func newColumn(w *window) (*column, error) {
	c := new(column)
	tag, err := newColumnTag(w)
	if err != nil {
		return nil, err
	}
	c.addFrame(0, tag)
	return c, nil
}

func (c *column) close() {
	// Closing the column is handled by closing the columnTag.
	//
	// TODO(eaburns): this is ugly, merge the columnTag into the column
	// instead of making it a frame.
	c.frames[0].close()
}

func (c *column) bounds() image.Rectangle { return c.Rectangle }

func (c *column) setBounds(bounds image.Rectangle) {
	c.Rectangle = bounds
	height := float64(bounds.Dy())
	for i := len(c.frames) - 1; i >= 0; i-- {
		f := c.frames[i]
		b := bounds
		if i > 0 {
			b.Min.Y = bounds.Min.Y + int(height*c.ys[i])
		}
		if i < len(c.frames)-1 {
			b.Max.Y = c.frames[i+1].bounds().Min.Y - borderWidth
		}
		f.setBounds(b)
	}
}

func (c *column) setAfterResizeBounds(bounds image.Rectangle) {
	c.Rectangle = bounds
	height := float64(bounds.Dy())
	for i := len(c.frames) - 1; i >= 0; i-- {
		f := c.frames[i]
		b := bounds
		if i > 0 {
			if i == 1 {
				b.Min.Y = c.frames[0].bounds().Max.Y + borderWidth
			} else {
				b.Min.Y = bounds.Min.Y + int(height*c.ys[i])
			}
		}
		if i < len(c.frames)-1 {
			b.Max.Y = c.frames[i+1].bounds().Min.Y - borderWidth
		}
		f.setBounds(b)
	}
}

func (c *column) focus(p image.Point) handler {
	for _, f := range c.frames {
		if p.In(f.bounds()) {
			return f.focus(p)
		}
	}
	return nil
}

func (c *column) draw(scr screen.Screen, win screen.Window) {
	for i, f := range c.frames {
		f.draw(scr, win)
		if i == len(c.frames)-1 {
			continue
		}
		g := c.frames[i+1]
		b := c.bounds()
		b.Min.Y = f.bounds().Max.Y
		b.Max.Y = g.bounds().Min.Y
		win.Fill(b, borderColor, draw.Over)
	}
}

func (c *column) removeFrame(f frame) bool {
	i := frameIndex(c, f)
	if i <= 0 {
		return false
	}
	c.frames = append(c.frames[:i], c.frames[i+1:]...)
	c.ys = append(c.ys[:i], c.ys[i+1:]...)
	c.setBounds(c.bounds())
	f.setColumn(nil)
	return true
}

func (c *column) addFrame(yfrac float64, f frame) bool {
	if len(c.frames) == 0 {
		c.frames = []frame{f}
		c.ys = []float64{0.0}
		f.setColumn(c)
		f.setBounds(c.bounds())
		return true
	}
	y := int(yfrac * float64(c.Dy()))
	i, splitFrame := frameAt(c, y)

	// Push away from the column edges.
	if y < 0 {
		y = 0
		yfrac = 0.0
	}
	if max := c.Dy() - f.minHeight() - borderWidth; y > max {
		y = max
		yfrac = float64(y) / float64(c.Dy())
	}

	// The frame we are splitting goes on top.
	// The added frame goes on the bottom.
	splitBounds := splitFrame.bounds()
	if topSize := y - splitBounds.Min.Y; i > 0 && topSize < splitFrame.minHeight() {
		if !slideUp(c, i, splitFrame.minHeight()-topSize) {
			y += splitFrame.minHeight() - topSize
			yfrac = float64(y) / float64(c.Dy())
		}
	}
	if bottomSize := splitBounds.Max.Y - y - borderWidth; bottomSize < f.minHeight() {
		if !slideDown(c, i, f.minHeight()-bottomSize) {
			return false
		}
	}

	c.frames = append(c.frames, nil)
	if i+2 < len(c.frames) {
		copy(c.frames[i+2:], c.frames[i+1:])
	}
	c.frames[i+1] = f

	c.ys = append(c.ys, 0)
	if i+2 < len(c.ys) {
		copy(c.ys[i+2:], c.ys[i+1:])
	}
	c.ys[i+1] = yfrac

	f.setColumn(c)
	c.setBounds(c.bounds())

	return true
}

// FrameIndex returns the index of the frame within the column,
// or -1 if the frame is not in the column.
func frameIndex(c *column, f frame) int {
	for i := range c.frames {
		if c.frames[i] == f {
			return i
		}
	}
	return -1
}

// FrameAt returns the frame containing pixel row y.
// If y < 0, the top-most frame is returned.
// If y > width, the the bottom-most frame is returned.
func frameAt(c *column, y int) (i int, f frame) {
	if y < 0 {
		return 0, c.frames[0]
	}
	for i, f = range c.frames {
		if f.bounds().Max.Y > y {
			return i, f
		}
	}
	return len(c.frames) - 1, c.frames[len(c.frames)-1]
}

func slideUp(c *column, i int, delta int) bool {
	if i <= 0 {
		return false
	}
	min := c.frames[i-1].minHeight()
	y := c.frames[i].bounds().Min.Y - delta
	if sz := y - c.frames[i-1].bounds().Min.Y - borderWidth; sz < min {
		if !slideUp(c, i-1, min-sz) {
			sum := 0
			for j := 0; j < i; j++ {
				sum += c.frames[j].minHeight()
			}
			c.ys[i] = float64(sum) / float64(c.Dy())
			return false
		}
	}
	c.ys[i] = float64(y) / float64(c.Dy())
	return true
}

func slideDown(c *column, i int, delta int) bool {
	if i > len(c.frames)-2 {
		return false
	}
	min := c.frames[i+1].minHeight()
	y := c.frames[i].bounds().Max.Y + delta
	if sz := c.frames[i+1].bounds().Max.Y - borderWidth - y; sz < min {
		if !slideDown(c, i+1, min-sz) {
			sum := 0
			for j := len(c.frames) - 1; j > i; j-- {
				sum += c.frames[j].minHeight()
			}
			c.ys[i+1] = 1. - float64(sum)/float64(c.Dy())
			return false
		}
	}
	c.ys[i+1] = float64(y) / float64(c.Dy())
	return true
}

const columnTagText = "Newcol New Cut Paste Snarf Putall"

type columnTag struct {
	col  *column
	text *textBox
	image.Rectangle

	p      image.Point
	button mouse.Button
	origX  float64
}

func newColumnTag(w *window) (*columnTag, error) {
	text, err := newTextBox(w, *w.server.editorURL, text.Style{
		Face: w.face,
		FG:   color.Black,
		BG:   color.Gray16{0xF5F5},
	})
	if err != nil {
		return nil, err
	}
	text.view.Do(nil, edit.Change(edit.All, columnTagText+" "), edit.Set(edit.End, '.'))
	return &columnTag{text: text}, nil
}

func (t *columnTag) close() {
	if t.col == nil {
		// Already closed.
		// This can happen if the columnTag is in focus when the window is closed.
		// The in-focus handler is closed, and so are all columns.
		return
	}
	// The columnTag is t.col.frames[0]; it's already closing, close the rest.
	for _, f := range t.col.frames[1:] {
		f.close()
	}
	t.col = nil
}

func (t *columnTag) bounds() image.Rectangle { return t.Rectangle }

func (t *columnTag) setBounds(b image.Rectangle) {
	t.text.topLeft = b.Min
	t.text.setSize(b.Size())
	t.Rectangle = b
}

func (*columnTag) minHeight() int { return 0 }

func (t *columnTag) setColumn(c *column) { t.col = c }

func (t *columnTag) focus(image.Point) handler { return t }

func (t *columnTag) draw(scr screen.Screen, win screen.Window) {
	t.text.setSize(t.Size()) // Reset the text in case it changed.
	t.text.draw(scr, win)
}

func (t *columnTag) drawLast(scr screen.Screen, win screen.Window) {
	if t.col.win != nil {
		return
	}
	t.col.draw(scr, win)
	drawBorder(t.col.bounds(), win)
}

func drawBorder(b image.Rectangle, win screen.Window) {
	x0, x1 := b.Min.X, b.Max.X
	y0, y1 := b.Min.Y, b.Max.Y
	win.Fill(image.Rect(x0, y0-borderWidth, x1, y0), borderColor, draw.Over)
	win.Fill(image.Rect(x0-borderWidth, y0, x0, y1), borderColor, draw.Over)
	win.Fill(image.Rect(x0, y1, x1, y1+borderWidth), borderColor, draw.Over)
	win.Fill(image.Rect(x1, y0, x1+borderWidth, y1), borderColor, draw.Over)
}

func (t *columnTag) changeFocus(win *window, inFocus bool) {
	t.text.changeFocus(win, inFocus)
}

func (t *columnTag) tick(win *window) bool {
	return t.text.tick(win)
}

func (t *columnTag) key(w *window, event key.Event) bool {
	var redraw bool
	switch event.Code {
	case key.CodeLeftShift, key.CodeRightShift:
		if event.Direction == key.DirRelease && t.col.win == nil {
			// We were dragging, and shift was released. Put it back.
			// BUG(eaburns): column 0 still ends up as column 1.
			if !w.addColumn(t.origX, t.col) {
				panic("can't put it back")
			}
			redraw = true
		}
	}
	if t.text.key(w, event) {
		redraw = true
	}
	return redraw
}

func (t *columnTag) mouse(w *window, event mouse.Event) bool {
	p := image.Pt(int(event.X), int(event.Y))

	switch event.Direction {
	case mouse.DirPress:
		if t.button == mouse.ButtonNone {
			t.p = p
			t.button = event.Button
			break
		}
		// A second button was pressed while the first was held.
		// ColumnTag doesn't use chords; treat this as a release of the first.
		event.Button = t.button
		fallthrough

	case mouse.DirRelease:
		if event.Button != t.button {
			// It's not the pressed button. Ignore it.
			break
		}
		defer func() { t.button = mouse.ButtonNone }()

		if event.Modifiers != key.ModShift {
			break
		}
		switch t.button {
		case mouse.ButtonLeft:
			if t.col.win != nil {
				defer func() { t.col.setBounds(t.col.bounds()) }()
				return slideDown(t.col, 0, minHeight(t.text.opts))
			}
			if w.addColumn(float64(t.Min.X)/float64(w.Dx()), t.col) {
				return true
			}
			// It didn't fit; just put it back where it came from.
			if !w.addColumn(t.origX, t.col) {
				panic("can't put it back")
			}
			return true
		case mouse.ButtonMiddle:
			w.deleteColumn(t.col)
			return true
		}

	case mouse.DirNone:
		if t.button == mouse.ButtonNone || event.Modifiers != key.ModShift {
			break
		}
		switch t.button {
		case mouse.ButtonLeft:
			if t.col.win == nil {
				t.col.setBounds(t.col.Add(p.Sub(t.col.Min)))
				return true
			}
			dx := t.p.X - p.X
			dy := t.p.Y - p.Y
			if dx*dx+dy*dy > 100 {
				t.p = p
				i := columnIndex(w, t.col)
				if i < 0 {
					return false
				}
				t.origX = w.xs[i]
				return w.removeColumn(t.col)
			}
		}
	}
	return t.text.mouse(w, event)
}

// MinHeight returns the minimum height
// for a single line of text in the default style
// plus padding and a border.
func minHeight(opts text.Options) int {
	return int(opts.DefaultStyle.Face.Metrics().Height>>6) + opts.Padding*2 + borderWidth
}
