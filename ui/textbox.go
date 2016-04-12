// Copyright © 2016, The T Authors.

package ui

import (
	"image"
	"net/url"
	"sync"

	"github.com/eaburns/T/edit"
	"github.com/eaburns/T/editor"
	"github.com/eaburns/T/editor/view"
	"github.com/eaburns/T/ui/text"
	"golang.org/x/exp/shiny/screen"
	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/mouse"
	"golang.org/x/mobile/event/paint"
)

// A textBox is an editable text box.
type textBox struct {
	bufferURL *url.URL
	view      *view.View
	opts      text.Options
	setter    *text.Setter
	text      *text.Text
	image.Rectangle

	mu    sync.RWMutex
	reset bool
	sheet *sheet
}

func newTextBox(sheet *sheet, bufferURL *url.URL, style text.Style) (*textBox, error) {
	v, err := view.New(bufferURL, '.')
	if err != nil {
		return nil, err
	}
	opts := text.Options{
		DefaultStyle: style,
		TabWidth:     4,
		Padding:      2,
	}
	setter := text.NewSetter(opts)
	t := &textBox{
		sheet:     sheet,
		bufferURL: bufferURL,
		view:      v,
		opts:      opts,
		setter:    setter,
		text:      setter.Set(),
	}
	go func() {
		for range v.Notify {
			t.mu.Lock()
			t.reset = true
			if t.sheet != nil {
				t.sheet.win.Send(paint.Event{})
			}
			t.mu.Unlock()
		}
		t.mu.Lock()
		if t.sheet != nil {
			t.sheet.win.server.delSheet(t.sheet.id)
		}
		t.mu.Unlock()
	}()
	return t, nil
}

func (t *textBox) close() {
	t.mu.Lock()
	t.sheet = nil
	t.mu.Unlock()

	t.text.Release()
	t.setter.Release()
	t.view.Close()
	editor.Close(t.bufferURL)
}

func (t *textBox) bounds() image.Rectangle { return t.Rectangle }

func (t *textBox) setBounds(b image.Rectangle) {
	if t.Size() != b.Size() {
		h := t.opts.DefaultStyle.Face.Metrics().Height
		t.view.Resize(b.Dy() / int(h>>6))
		t.setText(b.Size())
	}
	t.Rectangle = b
}

func (t *textBox) draw(scr screen.Screen, win screen.Window) {
	t.mu.RLock()
	if t.reset {
		t.reset = false
		t.setText(t.Size())
	}
	t.mu.RUnlock()
	t.text.Draw(t.Min, scr, win)
}

func (t *textBox) setText(size image.Point) {
	t.text.Release()
	t.opts.Size = size
	t.setter.Reset(t.opts)
	t.view.View(func(text []byte, _ []view.Mark) { t.setter.Add(text) })
	t.text = t.setter.Set()
}

var (
	advanceDot = edit.Set(edit.Dot.Plus(edit.Clamp(edit.Rune(1))), '.')
	backspace  = edit.Delete(edit.Dot.Minus(edit.Clamp(edit.Rune(1))).To(edit.Dot))
	newline    = []edit.Edit{edit.Change(edit.Dot, "\n"), advanceDot}
	tab        = []edit.Edit{edit.Change(edit.Dot, "\t"), advanceDot}
)

func (t *textBox) key(w *window, event key.Event) bool {
	if event.Direction == key.DirRelease {
		return false
	}
	switch event.Code {
	case key.CodeDeleteBackspace:
		t.view.Do(nil, backspace)
	case key.CodeReturnEnter:
		t.view.Do(nil, newline...)
	case key.CodeTab:
		t.view.Do(nil, tab...)
	default:
		if event.Rune >= 0 {
			t.view.Do(nil, edit.Change(edit.Dot, string(event.Rune)), advanceDot)
		}
	}
	return false
}

func (t *textBox) mouse(*window, mouse.Event) bool               { return false }
func (t *textBox) drawLast(scr screen.Screen, win screen.Window) {}
