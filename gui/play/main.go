package main

import (
	"image"
	"image/color"
	"time"

	"github.com/davecheney/profile"
	"github.com/eaburns/T/font"
	"github.com/eaburns/T/gui"
	"github.com/eaburns/T/ui"
	"github.com/eaburns/T/ui/sdl2"
)

const (
	fontPath = "/home/eaburns/.fonts/LucidaSansRegular.ttf"
)

var (
	bg  = color.NRGBA{R: 0xF8, G: 0xF8, B: 0xF8, A: 0xFF}
	sty = &gui.TextStyle{
		BG: color.NRGBA{R: 0xE0, G: 0xE0, B: 0xEE, A: 0xFF},
		// BG:       color.Transparent,
		FG:       color.Black,
		TabWidth: 8,
	}
)

func main() {
	defer profile.Start(profile.CPUProfile).Stop()
	ttf, err := font.LoadTTF(fontPath)
	if err != nil {
		panic(err)
	}
	sty.Font = font.New(ttf, 11)

	u, err := sdl2.New(5 * time.Millisecond)
	if err != nil {
		panic(err)
	}
	win := u.NewWindow("☺", 640, 480)
	d := &drawer{
		TextArea: gui.NewTextArea(win, bounds(win), bg),
	}

	for e := range win.Events() {
		switch e := e.(type) {
		case ui.CloseEvent:
			return
		case ui.MotionEvent:
			// Continue, don't redraw on mouse motion.
			continue
		case ui.TextEvent:
			d.typeRune(e.Rune)
		case ui.KeyEvent:
			if !e.Down {
				continue
			}
			switch e.Key {
			case ui.KeyReturn:
				d.typeRune('\n')
			case ui.KeyBackspace:
				d.typeRune('\b')
			case ui.KeyTab:
				d.typeRune('\t')
			}
		case ui.ResizeEvent:
			d.Bounds = bounds(win)
			d.Add(sty, d.txt)
			d.Present()
		case ui.RedrawEvent:
		}
		win.Draw(d)
	}
}

func bounds(w ui.Window) image.Rectangle {
	b := w.Bounds()
	return image.Rect(b.Dx()/4, b.Dy()/4, 3*b.Dx()/4, 3*b.Dy()/4)
}

type drawer struct {
	*gui.TextArea
	txt []rune
}

func (d *drawer) Draw(c ui.Canvas) {
	c.Fill(color.Black, c.Bounds())
	d.TextArea.Draw(c)
}

func (d *drawer) typeRune(r rune) {
	if r == '\b' {
		if len(d.txt) > 0 {
			d.txt = d.txt[:len(d.txt)-1]
		}
	} else {
		d.txt = append(d.txt, r)
	}
	d.Add(sty, d.txt)
	d.Present()
}
