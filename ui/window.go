// Copyright © 2016, The T Authors.

package ui

import (
	"image"
	"image/color"
	"image/draw"
	"time"

	"github.com/eaburns/T/edit"
	"github.com/eaburns/T/ui/text"
	"golang.org/x/exp/shiny/screen"
	"golang.org/x/image/font"
	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/mouse"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/event/size"
)

// A handler is an interactive portion of a window
// that can receive keyboard and mouse events.
//
// Handlers gain focus when the mouse hovers over them
// and they maintain focus until the mouse moves off of them.
// However, during a mouse drag event,
// when the pointer moves while a button is held,
// the handler maintains focus
// even if the pointer moves off of the handler.
type handler interface {
	// ChangeFocus is called when the focus of the handler changes.
	// If the handler is coming into focus,
	// then the bool argument is true,
	// otherwise it is false.
	// The window always redraws on a focus change event.
	changeFocus(*window, bool)

	// Tick is called whenever the window considers redrawing.
	// This occurs at almost-regular intervals/
	// deponding on how long the window required
	// to draw its previous frame.
	// The return value is whether to redraw the window.
	tick(*window) bool

	// Key is called if the handler is in forcus
	// and the window receives a keyboard event.
	// The return value is whether to redraw the window.
	key(*window, key.Event) bool

	// Mouse is called if the handler is in focus
	// and the window receives a mouse event.
	// The return value is whether to redraw the window.
	mouse(*window, mouse.Event) bool

	// DrawLast is called if the handler is in focus
	// while the window is redrawn.
	// It is always called after everything else on the window has been drawn.
	//
	// TODO(eaburns): textbox is a handler, but doesn't implement this.
	// Instead, drawLast should be a separate interface,
	// and only used for handlers that implement it.
	drawLast(scr screen.Screen, win screen.Window)
}

const (
	minFrameWidth = 20 // px
	borderWidth   = 1  // px
)

var borderColor = color.Black

const (
	ptPerInch  = 72
	defaultDPI = 96
)

type window struct {
	id     string
	server *Server
	screen.Window
	face font.Face
	image.Rectangle
	columns []*column
	xs      []float64
	inFocus handler
	p       image.Point

	dpi float64
}

func newWindow(id string, s *Server, size image.Point) (*window, error) {
	win, err := s.screen.NewWindow(&screen.NewWindowOptions{
		Width:  size.X,
		Height: size.Y,
	})
	if err != nil {
		return nil, err
	}
	w := &window{
		id:        id,
		server:    s,
		Window:    win,
		Rectangle: image.Rect(0, 0, size.X, size.Y),

		// dpi is set to the true value by a size.Event.
		dpi: defaultDPI,
	}
	w.getDPI()
	c, err := newColumn(w)
	if err != nil {
		win.Release()
		w.face.Close()
		return nil, err
	}
	w.addColumn(0.0, c)
	go w.events()
	return w, nil
}

// GetDPI reads and discards events until a size.Event, from which the DPI is read.
func (w *window) getDPI() {
	for {
		switch e := w.NextEvent().(type) {
		case size.Event:
			w.dpi = float64(e.PixelsPerPt * ptPerInch)
			w.face = newFace(w.dpi)
			return
		}
	}
}

type closeEvent struct{}

func (w *window) events() {
	events := make(chan interface{})
	go func() {
		for {
			e := w.NextEvent()
			if _, ok := e.(closeEvent); ok {
				close(events)
				return
			}
			events <- e
		}
	}()

	const drawTime = 33 * time.Millisecond
	timer := time.NewTimer(drawTime)
	defer timer.Stop()

	var click int
	var redraw bool
	for {
		select {
		case <-timer.C:
			if w.inFocus != nil && w.inFocus.tick(w) {
				redraw = true
			}
			if !redraw {
				timer.Reset(drawTime)
				break
			}
			w.draw(w.server.screen, w.Window)
			if w.inFocus != nil {
				w.inFocus.drawLast(w.server.screen, w.Window)
			}
			w.Publish()
			timer.Reset(drawTime)
			redraw = false

		case e, ok := <-events:
			if !ok {
				for _, c := range w.columns {
					c.close()
				}
				// TODO(eaburns): Don't call this if the frame is not detached.
				if f, ok := w.inFocus.(frame); ok {
					f.close()
				}
				w.face.Close()
				w.Release()
				return
			}
			switch e := e.(type) {
			case func():
				e()
				redraw = true

			case lifecycle.Event:
				if e.To == lifecycle.StageDead {
					w.server.delWin(w.id)
				}

			case paint.Event:
				redraw = true

			case size.Event:
				w.setBounds(image.Rectangle{Max: e.Size()})

			case key.Event:
				if w.inFocus != nil && w.inFocus.key(w, e) {
					redraw = true
				}

			case mouse.Event:
				var dir mouse.Direction
				w.p, dir = image.Pt(int(e.X), int(e.Y)), e.Direction
				switch dir {
				case mouse.DirPress:
					click++
				case mouse.DirRelease:
					click--
				}
				if dir == mouse.DirNone && click == 0 {
					prev := w.inFocus
					w.inFocus = w.focus(w.p)
					if prev != w.inFocus {
						if prev != nil {
							prev.changeFocus(w, false)
						}
						if w.inFocus != nil {
							w.inFocus.changeFocus(w, true)
						}
						redraw = true
					}
				}
				if w.inFocus != nil {
					if w.inFocus.mouse(w, e) {
						redraw = true
					}
				}
				// After sending a press or release to the focus,
				// check whether it's still in focus.
				if dir != mouse.DirNone {
					prev := w.inFocus
					w.inFocus = w.focus(w.p)
					if prev != w.inFocus {
						if prev != nil {
							prev.changeFocus(w, false)
						}
						if w.inFocus != nil {
							w.inFocus.changeFocus(w, true)
						}
						redraw = true
					}
				}
			}
		}
	}
}

func (w *window) close() {
	w.Send(closeEvent{})
}

func (w *window) bounds() image.Rectangle { return w.Rectangle }

func (w *window) setBounds(bounds image.Rectangle) {
	w.Rectangle = bounds
	width := float64(bounds.Dx())
	for i := len(w.columns) - 1; i >= 0; i-- {
		c := w.columns[i]
		b := bounds
		if i > 0 {
			b.Min.X = bounds.Min.X + int(width*w.xs[i])
		}
		if i < len(w.columns)-1 {
			b.Max.X = w.columns[i+1].bounds().Min.X - borderWidth
		}
		c.setBounds(b)
	}
}

func (w *window) focus(p image.Point) handler {
	for _, c := range w.columns {
		if p.In(c.bounds()) {
			return c.focus(p)
		}
	}
	return nil
}

func (w *window) draw(scr screen.Screen, win screen.Window) {
	for i, c := range w.columns {
		c.draw(scr, win)
		if i == len(w.columns)-1 {
			continue
		}
		d := w.columns[i+1]
		b := w.bounds()
		b.Min.X = c.bounds().Max.X
		b.Max.X = d.bounds().Min.X
		win.Fill(b, borderColor, draw.Over)
	}
}

// AddFrame adds the frame to the last column of the window.
func (w *window) addFrame(f frame) {
	c := w.columns[len(w.columns)-1]
	var y int
	if len(w.columns) == 1 && len(c.frames) == 1 {
		y = minHeight(w.columns[0].frames[0].(*columnTag).text.opts)
	}
	if len(c.frames) > 1 {
		f := c.frames[len(c.frames)-1]
		b := f.bounds()
		y = b.Min.Y + b.Dy()/2
	}
	c.addFrame(float64(y)/float64(c.Dy()), f)
}

func (w *window) deleteFrame(f frame) {
	for _, c := range w.columns {
		for _, g := range c.frames {
			if g == f {
				c.removeFrame(f)
			}
		}
	}
	if h := f.(handler); h == w.inFocus {
		w.inFocus = w.focus(w.p)
	}
	f.close()
}

func (w *window) deleteColumn(c *column) {
	if w.removeColumn(c) {
		c.close()
	}
}

func (w *window) removeColumn(c *column) bool {
	if len(w.columns) < 2 {
		return false
	}
	i := columnIndex(w, c)
	if i < 0 {
		return false
	}
	w.columns = append(w.columns[:i], w.columns[i+1:]...)
	w.xs = append(w.xs[:i], w.xs[i+1:]...)
	w.setBounds(w.bounds())
	c.win = nil
	return true
}

func columnIndex(w *window, c *column) int {
	for i := range w.columns {
		if w.columns[i] == c {
			return i
		}
	}
	return -1
}

// AddCol adds a column to the window such that its left side at pixel xfrac*w.Dx().
// However, if the window has no columns, its left side is always at 0.0.
func (w *window) addColumn(xfrac float64, c *column) bool {
	if len(w.columns) == 0 {
		w.columns = []*column{c}
		w.xs = []float64{0.0}
		c.win = w
		c.setBounds(w.bounds())
		return true
	}
	x := int(float64(w.Dx()) * xfrac)
	i, splitCol := columnAt(w, x)

	// Push away from the window edges.
	if x < minFrameWidth {
		x = minFrameWidth
		xfrac = float64(x) / float64(w.Dx())
	}
	if max := w.Dx() - minFrameWidth - borderWidth; x > max {
		x = max
		xfrac = float64(x) / float64(w.Dx())
	}

	if leftSize := x - splitCol.Min.X; leftSize < minFrameWidth {
		if !slideLeft(w, i, minFrameWidth-leftSize) {
			x += minFrameWidth - leftSize
			xfrac = float64(x) / float64(w.Dx())
		}
	}
	if rightSize := splitCol.Max.X - x - borderWidth; rightSize < minFrameWidth {
		if !slideRight(w, i, minFrameWidth-rightSize) {
			return false
		}
	}

	w.columns = append(w.columns, nil)
	if i+2 < len(w.columns) {
		copy(w.columns[i+2:], w.columns[i+1:])
	}
	w.columns[i+1] = c

	w.xs = append(w.xs, 0)
	if i+2 < len(w.xs) {
		copy(w.xs[i+2:], w.xs[i+1:])
	}
	w.xs[i+1] = xfrac

	c.win = w
	w.setBounds(w.bounds())
	return true
}

// ColumnAt returns the column containing pixel column x.
// If x < 0, the left-most column is returned.
// If x > width, the the right-most column is returned.
func columnAt(w *window, x int) (i int, c *column) {
	if x < 0 {
		return 0, w.columns[0]
	}
	for i, c = range w.columns {
		if c.Max.X > x {
			return i, c
		}
	}
	return len(w.columns) - 1, w.columns[len(w.columns)-1]
}

// TODO(eaburns): slideRight and slideLeft should slide as far as possible.
// Currently, if they can't slide the entire delta, they slide nothing at all.

func slideLeft(w *window, i int, delta int) bool {
	if i <= 0 {
		return false
	}
	x := w.columns[i].Min.X - delta
	if sz := x - w.columns[i-1].Min.X; sz < minFrameWidth {
		if !slideLeft(w, i-1, minFrameWidth-sz) {
			return false
		}
	}
	w.xs[i] = float64(x) / float64(w.Dx())
	return true
}

func slideRight(w *window, i int, delta int) bool {
	if i > len(w.columns)-2 {
		return false
	}
	x := w.columns[i].Max.X + delta
	if sz := w.columns[i+1].Max.X - borderWidth - x; sz < minFrameWidth {
		if !slideRight(w, i+1, minFrameWidth-sz) {
			return false
		}
	}
	w.xs[i+1] = float64(x) / float64(w.Dx())
	return true
}

// A frame is a rectangular section of a win that can be attached to a column.
type frame interface {
	// Bounds returns the current bounds of the frame.
	bounds() image.Rectangle

	// SetBounds sets the bounds of the frame to the given rectangle.
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
	min := c.frames[i].minHeight()
	y := c.frames[i].bounds().Max.Y + delta
	if sz := c.frames[i+1].bounds().Max.Y - borderWidth - y; sz < min {
		if !slideDown(c, i+1, min-sz) {
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
	t.text.setSize(b.Size())
	t.Rectangle = b
}

func (*columnTag) minHeight() int { return 0 }

func (t *columnTag) setColumn(c *column) { t.col = c }

func (t *columnTag) focus(image.Point) handler { return t }

func (t *columnTag) draw(scr screen.Screen, win screen.Window) {
	t.text.setSize(t.Size()) // Reset the text in case it changed.
	t.text.draw(t.bounds().Min, scr, win)
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
