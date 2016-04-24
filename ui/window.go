// Copyright © 2016, The T Authors.

package ui

import (
	"bufio"
	"image"
	"image/color"
	"image/draw"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/eaburns/T/edit"
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
	dpi  float64
	image.Rectangle

	columns []*column
	xs      []float64

	inFocus handler
	p       image.Point
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
				w.setBoundsAfterResize(image.Rectangle{Max: e.Size()})

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
				if dir == mouse.DirNone && click == 0 && w.refocus() {
					redraw = true
				}
				if w.inFocus != nil {
					if w.inFocus.mouse(w, e) {
						redraw = true
					}
				}
				// After sending a press or release to the focus,
				// check whether it's still in focus.
				if dir != mouse.DirNone && w.refocus() {
					redraw = true
				}
			}
		}
	}
}

func (w *window) close() {
	w.Send(closeEvent{})
}

func (w *window) refocus() bool {
	prev := w.inFocus
	for _, c := range w.columns {
		if w.p.In(c.bounds()) {
			w.inFocus = c.focus(w.p)
			break
		}
	}
	if prev == w.inFocus {
		return false
	}
	if prev != nil {
		prev.changeFocus(w, false)
	}
	if w.inFocus != nil {
		w.inFocus.changeFocus(w, true)
	}
	return true
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

func (w *window) setBoundsAfterResize(bounds image.Rectangle) {
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
		c.setAfterResizeBounds(b)
	}
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
		w.refocus()
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

// TODO(eaburns): take a *sheet as an optional argument for setting T_SHEET.
func (w *window) exec(commandLine string) {
	scanner := bufio.NewScanner(strings.NewReader(commandLine))
	scanner.Split(bufio.ScanWords)
	var words []string
	for scanner.Scan() {
		words = append(words, scanner.Text())
	}
	if len(words) == 0 {
		return
	}

	out, in, err := os.Pipe()
	if err != nil {
		log.Println("failed to open pipe:", err)
		return
	}
	go pipeOutput(w, out)

	cmd := exec.Command(words[0], words[1:]...)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "T_WINDOW_PATH="+windowPath(w))
	cmd.Stdout = in
	cmd.Stderr = in
	cmd.Run()
	in.Close()
}

func pipeOutput(w *window, out io.ReadCloser) {
	defer out.Close()
	var buf [4096]byte
	for {
		switch n, err := out.Read(buf[:]); {
		case err == io.EOF:
			return
		case err != nil:
			log.Println("read error:", err)
			return
		default:
			str := string(buf[:n])
			w.Send(func() { w.output(str) })
		}
	}
}

// Output writes the string to the window's output sheet.
//
// Output must be called in the window's UI goroutine.
func (w *window) output(str string) *sheet {
	const outSheetName = "+output"
	var out *sheet
	w.server.Lock()
	for _, s := range w.server.sheets {
		if s.win == w && s.tagFileName() == outSheetName {
			out = s
			break
		}
	}
	if out != nil {
		w.server.Unlock()
	} else {
		var err error
		out, err = w.server.newSheet(w, w.server.editorURL)
		w.server.Unlock()
		if err != nil {
			log.Printf("failed to create %s sheet: %v", outSheetName, err)
			return nil
		}
		out.setTagFileName(outSheetName)
		w.refocus()
	}
	out.body.do(nil, edit.Append(edit.End, str))
	return out
}
