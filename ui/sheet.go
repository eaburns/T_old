// Copyright © 2016, The T Authors.

package ui

import (
	"image"
	"image/color"
	"image/draw"
	"strconv"
	"sync"

	"github.com/eaburns/T/ui/text"
	"golang.org/x/exp/shiny/screen"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/mouse"
)

var (
	colors = []color.Color{
		color.NRGBA{R: 0xE6, G: 0xF0, B: 0xFA, A: 0xFF},
		color.NRGBA{R: 0xE6, G: 0xFA, B: 0xF0, A: 0xFF},
		color.NRGBA{R: 0xF0, G: 0xE6, B: 0xFA, A: 0xFF},
		color.NRGBA{R: 0xF0, G: 0xFA, B: 0xE6, A: 0xFF},
		color.NRGBA{R: 0xFA, G: 0xE6, B: 0xF0, A: 0xFF},
	}
	nextColor      = 0
	lock           sync.Mutex
	separatorColor = color.Gray16{0xAAAA}
)

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

	// TODO(eaburns): this just mimics a tag and body, implement the real things.
	tagOpts, bodyOpts     text.Options
	tagSetter, bodySetter *text.Setter
	tagText, bodyText     []byte
	tag, body             *text.Text
	sep                   image.Rectangle

	p      image.Point
	button mouse.Button

	origX int
	origY float64
}

func newSheet(id string, w *window) *sheet {
	lock.Lock()
	defer lock.Unlock()
	s := &sheet{
		id:  id,
		win: w,
		tagOpts: text.Options{
			DefaultStyle: text.Style{
				Face: basicfont.Face7x13,
				FG:   color.Black,
				BG:   colors[nextColor%len(colors)],
			},
			TabWidth: 4,
			Padding:  2,
		},
		tagText: []byte("/sheet/" + strconv.Itoa(nextColor)),
		bodyOpts: text.Options{
			DefaultStyle: text.Style{
				Face: basicfont.Face7x13,
				FG:   color.Black,
				BG:   color.NRGBA{R: 0xFA, G: 0xF0, B: 0xE6, A: 0xFF},
			},
			TabWidth: 4,
			Padding:  2,
		},
		bodyText: elPaso,
	}
	nextColor++
	s.tagSetter = text.NewSetter(s.tagOpts)
	s.tagSetter.Add(s.tagText)
	s.tag = s.tagSetter.Set()
	s.bodySetter = text.NewSetter(s.bodyOpts)
	s.bodySetter.Add(s.bodyText)
	s.body = s.bodySetter.Set()
	return s
}

func (s *sheet) close() { s.win = nil }

func (s *sheet) bounds() image.Rectangle { return s.Rectangle }

func (s *sheet) setBounds(b image.Rectangle) {
	if s.Size() != b.Size() {
		s.setText(b)
	}
	s.Rectangle = b
	s.sep = image.Rectangle{
		Min: image.Pt(b.Min.X, b.Min.Y+minFrameSize),
		Max: image.Pt(b.Max.X, b.Min.Y+minFrameSize+borderWidth),
	}
}

func (s *sheet) setText(b image.Rectangle) {
	s.tagOpts.Size = image.Pt(b.Dx(), minFrameSize)
	s.tag.Release()
	s.tagSetter.Reset(s.tagOpts)
	s.tagSetter.Add(s.tagText)
	s.tag = s.tagSetter.Set()

	s.bodyOpts.Size = image.Pt(b.Dx(), b.Dy()-minFrameSize-borderWidth)
	s.body.Release()
	s.bodySetter.Reset(s.bodyOpts)
	s.bodySetter.Add(s.bodyText)
	s.body = s.bodySetter.Set()
}

func (s *sheet) setColumn(c *column) { s.col = c }

func (s *sheet) focus(p image.Point) handler { return s }

func (s *sheet) draw(scr screen.Screen, win screen.Window) {
	p := s.Min
	s.tag.Draw(p, scr, win)
	win.Fill(s.sep, separatorColor, draw.Over)
	p.Y += s.tag.Size().Y + s.sep.Dy()
	s.body.Draw(p, scr, win)
}

// DrawLast is called if the sheet is in focus, after the entire window has been drawn.
// It draws the sheet if being dragged.
func (s *sheet) drawLast(scr screen.Screen, win screen.Window) {
	if s.col == nil {
		s.draw(scr, win)
		drawBorder(s.bounds(), win)
	}
}

func (s *sheet) key(w *window, event key.Event) bool {
	switch event.Code {
	case key.CodeLeftShift, key.CodeRightShift:
		if event.Direction == key.DirRelease && s.col == nil {
			// We were dragging, and shift was released. Put it back.
			if _, c := columnAt(w, s.origX); !c.addFrame(s.origY, s) {
				panic("can't put it back")
			}
			return true
		}
	}
	return false
}

func (s *sheet) mouse(w *window, event mouse.Event) bool {
	p := image.Pt(int(event.X), int(event.Y))

	switch event.Direction {
	case mouse.DirPress:
		if s.button == mouse.ButtonNone {
			s.p = p
			s.button = event.Button
			return false
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
				if slideUp(s.col, i, minFrameSize) {
					return true
				}
				return slideDown(s.col, i, minFrameSize)
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
			s.col.win.server.delSheet(s.id)
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
	return false
}

var elPaso = []byte(`Out in the West Texas town of El Paso
I fell in love with a Mexican girl
Nighttime would find me in Rosa's cantina
Music would play and Felina would whirl
Blacker than night were the eyes of Felina
Wicked and evil while casting a spell
My love was deep for this Mexican maiden
I was in love but in vain, I could tell
One night a wild young cowboy came in
Wild as the West Texas wind
Dashing and daring, a drink he was sharing
With wicked Felina, the girl that I loved
So in anger I
Challenged his right for the love of this maiden
Down went his hand for the gun that he wore
My challenge was answered in less than a heartbeat
The handsome young stranger lay dead on the floor
Just for a moment I stood there in silence
Shocked by the foul evil deed I had done
Many thoughts raced through my mind as I stood there
I had but one chance and that was to run
Out through the back door of Rosa's I ran
Out where the horses were tied
I caught a good one, it looked like it could run
Up on its back and away I did ride
Just as fast as I
Could from the West Texas town of El Paso
Out to the badlands of New Mexico
Back in El Paso my life would be worthless
Everything's gone in life; nothing is left
It's been so long since I've seen the young maiden
My love is stronger than my fear of death
I saddled up and away I did go
Riding alone in the dark
Maybe tomorrow, a bullet may find me
Tonight nothing's worse than this pain in my heart
And at last here I
Am on the hill overlooking El Paso
I can see Rosa's cantina below
My love is strong and it pushes me onward
Down off the hill to Felina I go
Off to my right I see five mounted cowboys
Off to my left ride a dozen or more
Shouting and shooting, I can't let them catch me
I have to make it to Rosa's back door
Something is dreadfully wrong for I feel
A deep burning pain in my side
Though I am trying to stay in the saddle
I'm getting weary, unable to ride
But my love for
Felina is strong and I rise where I've fallen
Though I am weary I can't stop to rest
I see the white puff of smoke from the rifle
I feel the bullet go deep in my chest
From out of nowhere Felina has found me
Kissing my cheek as she kneels by my side
Cradled by two loving arms that I'll die for
One little kiss and Felina, goodbye`)
