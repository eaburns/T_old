// Copyright © 2016, The T Authors.

// +build ignore

// Main is demo program to try out the text package.
package main

import (
	"image"
	"image/color"
	"image/draw"
	"io/ioutil"
	"log"
	"os"
	"path"
	"runtime"
	"unicode/utf8"

	"github.com/eaburns/T/ui/text"
	"github.com/pkg/profile"
	"golang.org/x/exp/shiny/driver"
	"golang.org/x/exp/shiny/screen"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/plan9font"
	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/mouse"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/event/size"
)

const (
	fontPath = "/home/eaburns/.fonts/LucidaSansRegular.ttf"
)

var (
	hi = &text.Style{
		Face: loadFace(),
		BG:   color.NRGBA{R: 0xB6, G: 0xDA, B: 0xFD, A: 0xFF},
		FG:   color.Black,
	}
	opts = text.Options{
		DefaultStyle: text.Style{
			Face: loadFace(),
			BG:   color.NRGBA{R: 0xFA, G: 0xF0, B: 0xE6, A: 0xFF},
			FG:   color.Black,
		},
		TabWidth: 4,
		Padding:  10,
	}
)

func loadFace() font.Face {
	dir := path.Join(os.Getenv("PLAN9"), "font/lucsans")
	file := path.Join(dir, "unicode.8.font")
	f, err := os.Open(file)
	if err != nil {
		log.Println(file, "not found")
		return basicfont.Face7x13
	}
	data, err := ioutil.ReadAll(f)
	if err != nil {
		log.Println("failed to read", file)
		return basicfont.Face7x13
	}
	face, err := plan9font.ParseFont(data, func(f string) ([]byte, error) {
		return ioutil.ReadFile(path.Join(dir, f))
	})
	if err != nil {
		log.Println("failed load", file)
		return basicfont.Face7x13
	}
	return face
}

func init() { runtime.LockOSThread() }

func main() { driver.Main(Main) }

// Main is the logical main function, the real main function is hijacked by shiny.
func Main(scr screen.Screen) {
	defer profile.Start(profile.MemProfile).Stop()

	width, height := 300, 300
	win, err := scr.NewWindow(&screen.NewWindowOptions{
		Width:  width,
		Height: height,
	})
	if err != nil {
		panic(err)
	}
	defer win.Release()

	sz := image.Pt(width, height)
	at := image.Pt(sz.X/20, sz.Y/20)
	opts.Size = image.Pt(sz.X-sz.X/10, sz.Y-sz.Y/10)
	setter := text.NewSetter(opts)
	defer setter.Release()

	var a0, a1 int
	txt := resetText(setter, nil, a0, a1)
	defer txt.Release()

	var drag bool
	for {
		switch e := win.NextEvent().(type) {
		case lifecycle.Event:
			if e.To == lifecycle.StageDead {
				return
			}

		case key.Event:
			if e.Direction == key.DirRelease {
				continue
			}
			switch e.Code {
			case key.CodeDeleteForward:
				elPaso = elPaso[:0]
				a0, a1 = 0, 0
			case key.CodeDeleteBackspace:
				typeRune(&elPaso, '\b')
				if a0 > len(elPaso) {
					a0 = len(elPaso)
				}
				if a1 > len(elPaso) {
					a1 = len(elPaso)
				}
			case key.CodeReturnEnter:
				typeRune(&elPaso, '\n')
			case key.CodeTab:
				typeRune(&elPaso, '\t')
			default:
				if e.Rune < 0 {
					continue
				}
				typeRune(&elPaso, e.Rune)
			}
			txt = resetText(setter, txt, a0, a1)
			win.Send(paint.Event{})

		case mouse.Event:
			switch e.Direction {
			case mouse.DirPress:
				a0 = txt.Index(image.Pt(int(e.X), int(e.Y)))
				a1 = a0
				drag = true
				txt = resetText(setter, txt, a0, a1)
				win.Send(paint.Event{})

			case mouse.DirRelease:
				drag = false

			case mouse.DirNone:
				if !drag {
					continue
				}
				a1Prev := a1
				a1 = txt.Index(image.Pt(int(e.X), int(e.Y)))
				if a1 != a1Prev {
					txt = resetText(setter, txt, a0, a1)
					win.Send(paint.Event{})
				}
			}

		case size.Event:
			sz = e.Size()
			at = image.Pt(sz.X/20, sz.Y/20)
			opts.Size = image.Pt(sz.X-sz.X/10, sz.Y-sz.Y/10)
			setter.Reset(opts)
			txt = resetText(setter, txt, a0, a1)

		case paint.Event:
			win.Fill(image.Rect(0, 0, sz.X, sz.Y), image.White, draw.Over)
			txt.Draw(at, scr, win)
			win.Publish()
		}
	}
}

func resetText(setter *text.Setter, prev *text.Text, a0, a1 int) *text.Text {
	if prev != nil {
		prev.Release()
	}
	if a1 < a0 {
		a0, a1 = a1, a0
	}
	setter.Add(elPaso[:a0])
	setter.AddStyle(hi, elPaso[a0:a1])
	setter.Add(elPaso[a1:len(elPaso)])
	return setter.Set()
}

func typeRune(txt *[]byte, r rune) {
	if r == '\b' {
		if l := len(*txt); l > 0 {
			*txt = (*txt)[:l-1]
		}
	} else {
		var bs [utf8.UTFMax]byte
		*txt = append(*txt, bs[:utf8.EncodeRune(bs[:], r)]...)
	}
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
