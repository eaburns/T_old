// Copyright © 2016, The T Authors.

package ui

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"net/url"
	"path"
	"sync"
	"time"

	"github.com/eaburns/T/edit"
	"github.com/eaburns/T/editor"
	"github.com/eaburns/T/editor/view"
	"github.com/eaburns/T/ui/text"
	"golang.org/x/exp/shiny/screen"
	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/mouse"
	"golang.org/x/mobile/event/paint"
)

const (
	cursorWidth   = 1 // px
	blinkDuration = 500 * time.Millisecond
)

// A textBox is an editable text box.
type textBox struct {
	bufferURL *url.URL
	view      *view.View
	opts      text.Options
	setter    *text.Setter
	text      *text.Text
	topLeft   image.Point

	textLen  int
	l0, dot0 int64

	// Col is the column number of the cursor, or -1 if unknown.
	col int

	lastBlink        time.Time
	inFocus, blinkOn bool

	mu    sync.RWMutex
	reset bool
	win   *window
}

// NewTextBod creates a new text box.
// URL is either the root path to an editor server,
// or the path to an open buffer of an editor server.
func newTextBox(w *window, URL url.URL, style text.Style) (t *textBox, err error) {
	if URL.Path == "/" {
		URL.Path = path.Join("/", "buffers")
		buf, err := editor.NewBuffer(&URL)
		if err != nil {
			return nil, err
		}
		URL.Path = buf.Path
		defer func(newBufferURL url.URL) {
			if err != nil {
				editor.Close(&newBufferURL)
			}
		}(URL)
	}
	if ok, err := path.Match("/buffer/*", URL.Path); err != nil {
		// The only error is path.ErrBadPattern. This pattern is not bad.
		panic(err)
	} else if !ok {
		return nil, errors.New("bad buffer path: " + URL.Path)
	}

	v, err := view.New(&URL, '.')
	if err != nil {
		return nil, err
	}
	opts := text.Options{
		DefaultStyle: style,
		TabWidth:     4,
		Padding:      2,
	}
	setter := text.NewSetter(opts)
	t = &textBox{
		bufferURL: &URL,
		view:      v,
		opts:      opts,
		setter:    setter,
		text:      setter.Set(),
		col:       -1,
		win:       w,
	}
	go func() {
		for range v.Notify {
			t.mu.Lock()
			t.reset = true
			if t.win != nil {
				t.win.Send(paint.Event{})
			}
			t.mu.Unlock()
		}
	}()
	return t, nil
}

func (t *textBox) close() {
	t.mu.Lock()
	t.win = nil
	t.mu.Unlock()

	t.text.Release()
	t.setter.Release()
	t.view.Close()
	editor.Close(t.bufferURL)
}

// SetSize resets the text if either the size changed or the text changed.
func (t *textBox) setSize(size image.Point) {
	t.mu.Lock()
	if !t.reset && t.opts.Size == size {
		t.mu.Unlock()
		return
	}
	t.reset = false
	t.mu.Unlock()

	h := t.opts.DefaultStyle.Face.Metrics().Height
	t.view.Resize(size.Y / int(h>>6))
	t.text.Release()
	t.opts.Size = size
	t.setter.Reset(t.opts)

	t.view.View(func(text []byte, marks []view.Mark) {
		t.textLen = len(text)
		t.setter.Add(text)
		for _, m := range marks {
			switch m.Name {
			case view.ViewMark:
				t.l0 = m.Where[0]
			case '.':
				t.dot0 = m.Where[0]
			}
		}
	})

	t.text = t.setter.Set()

	if t.inFocus {
		t.blinkOn = true
		t.lastBlink = time.Now()
	}
}

func (t *textBox) draw(scr screen.Screen, win screen.Window) {
	t.text.Draw(t.topLeft, scr, win)
	t.drawDot(t.topLeft, win)
}

func (t *textBox) drawLines(scr screen.Screen, win screen.Window) {
	t.text.DrawLines(t.topLeft, scr, win)
	t.drawDot(t.topLeft, win)
}

func (t *textBox) drawDot(pt image.Point, win screen.Window) {
	l, d := t.l0, t.dot0
	if !t.blinkOn || d < t.l0 || d > l+int64(t.textLen) || t.opts.Size.X < cursorWidth {
		return
	}
	i := int(d - l)
	r := t.text.GlyphBox(i).Add(pt)
	r.Max.X = r.Min.X + cursorWidth
	win.Fill(r, color.Black, draw.Src)
}

func (t *textBox) changeFocus(_ *window, inFocus bool) {
	t.inFocus = inFocus
	t.blinkOn = inFocus
	t.lastBlink = time.Now()
}

func (t *textBox) tick(win *window) bool {
	if s := time.Since(t.lastBlink); s < blinkDuration {
		return false
	}
	t.lastBlink = time.Now()
	t.blinkOn = !t.blinkOn
	return true
}

func (t *textBox) key(_ *window, event key.Event) bool {
	handleKey(t, event)
	return false
}

func (t *textBox) mouse(_ *window, event mouse.Event) bool {
	handleMouse(t, event)
	return false
}

func (t *textBox) drawLast(scr screen.Screen, win screen.Window) {}

func (t *textBox) do(res chan<- []editor.EditResult, eds ...edit.Edit) {
	t.col = -1
	t.view.Do(res, eds...)
}

func (t *textBox) where(p image.Point) int64 {
	return int64(t.text.Index(p.Sub(t.topLeft))) + t.l0
}

func (t *textBox) setColumn(c int) { t.col = c }
func (t *textBox) column() int     { return t.col }

var (
	dot          = edit.Dot
	zero         = edit.Clamp(edit.Rune(0))
	one          = edit.Clamp(edit.Rune(1))
	oneLine      = edit.Clamp(edit.Line(1))
	twoLines     = edit.Clamp(edit.Line(2))
	moveDotRight = edit.Set(dot.Plus(one), '.')
	moveDotLeft  = edit.Set(dot.Minus(one), '.')
	backspace    = edit.Delete(dot.Minus(one).To(dot))
	backline     = edit.Delete(dot.Minus(edit.Line(0)).To(dot.Plus(zero)))
	backword     = edit.Delete(dot.Plus(zero).Minus(edit.Regexp(`\w*\W*`)))
	newline      = []edit.Edit{edit.Change(dot, "\n"), edit.Set(dot.Plus(zero), '.')}
	tab          = []edit.Edit{edit.Change(dot, "\t"), edit.Set(dot.Plus(zero), '.')}
)

type doer interface {
	// Do clears the column marker and performs the edit,
	// sending the result on the channel.
	// If the channel is nil, the edit is performed asynchronously.
	do(chan<- []editor.EditResult, ...edit.Edit)
}

type mouseHandler interface {
	doer
	// Where returns the rune address
	// corresponding to the glyph at the given point.
	where(image.Point) int64
}

func handleMouse(h mouseHandler, event mouse.Event) {
	if event.Modifiers != 0 {
		return
	}

	p := image.Pt(int(event.X), int(event.Y))

	switch event.Direction {
	case mouse.DirPress:
		if event.Button == mouse.ButtonLeft {
			h.do(nil, edit.Set(edit.Rune(h.where(p)), '.'))
		}
	}
}

type keyHandler interface {
	doer
	column() int
	setColumn(int)
}

// HandleKey encapsulates the keyboard editing logic for a textBox.
func handleKey(h keyHandler, event key.Event) {
	if event.Direction == key.DirRelease {
		return
	}
	switch event.Code {
	case key.CodeUpArrow:
		col := getColumn(h)
		re := fmt.Sprintf("(?:.?){%d}", col)
		up := dot.Minus(oneLine).Minus(zero).Plus(edit.Regexp(re)).Plus(zero)
		h.do(nil, edit.Set(up, '.'))
		h.setColumn(col)
	case key.CodeDownArrow:
		col := getColumn(h)
		re := fmt.Sprintf("(?:.?){%d}", col)
		// We use .-1+2, because .+1 does not move dot
		// if it is at the beginning of an empty line.
		up := dot.Minus(oneLine).Plus(twoLines).Minus(zero).Plus(edit.Regexp(re)).Plus(zero)
		h.do(nil, edit.Set(up, '.'))
		h.setColumn(col)
	case key.CodeRightArrow:
		h.do(nil, moveDotRight)
	case key.CodeLeftArrow:
		h.do(nil, moveDotLeft)
	case key.CodeDeleteBackspace:
		h.do(nil, backspace)
	case key.CodeReturnEnter:
		h.do(nil, newline...)
	case key.CodeTab:
		h.do(nil, tab...)
	default:
		switch event.Modifiers {
		case 0, key.ModShift:
			if event.Rune >= 0 {
				r := string(event.Rune)
				h.do(nil, edit.Change(dot, r), edit.Set(dot.Plus(zero), '.'))
			}
		case key.ModControl:
			switch event.Rune {
			case 'a':
				h.do(nil, edit.Set(dot.Minus(edit.Line(0)).Minus(zero), '.'))
			case 'e':
				h.do(nil, edit.Set(dot.Minus(zero).Plus(edit.Regexp("$")), '.'))
			case 'h':
				h.do(nil, backspace)
			case 'u':
				h.do(nil, backline)
			case 'w':
				h.do(nil, backword)
			}
		}
	}
}

// Column returns the desired column number of the keyHandler.
func getColumn(h keyHandler) int {
	if c := h.column(); c >= 0 {
		return c
	}

	// TODO(eaburns): This makes a blocking RPC, but it's called from the key handler.
	// We should find a way to avoid blocking in the key handler.
	resultCh := make(chan []editor.EditResult)
	h.do(resultCh, edit.Where(dot.Minus(edit.Line(0))))
	res := <-resultCh
	if res[0].Error != "" {
		panic("bad result: " + res[0].Error)
	}
	var w [2]int64
	if n, err := fmt.Sscanf(res[0].Print, "#%d,#%d", &w[0], &w[1]); n == 1 {
		w[1] = w[0]
	} else if n != 2 || err != nil {
		panic("failed to scan address: " + res[0].Print)
	}

	var c int
	if d := w[1] - w[0]; d > math.MaxInt32 {
		c = 0
	} else {
		c = int(d)
	}
	h.setColumn(c)
	return c
}
