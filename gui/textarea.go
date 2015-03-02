package gui

import (
	"image"
	"image/color"

	"github.com/eaburns/T/font"
	"github.com/eaburns/T/ui"
)

// Pad is the number of pixels of padding between text
// and the side of the TextArea.
const pad = 2

// A TextStyle describes the font and colors of text.
type TextStyle struct {
	font.Font
	FG, BG   color.Color
	TabWidth int
}

// Tab returns the next tab stop.
func (sty *TextStyle) tab(x font.Fix32) font.Fix32 {
	w := sty.Advance(sty.Glyph(' ')) * font.Fix32(sty.TabWidth)
	return w - (x % w) + x
}

// A TextArea is an area of a window containing text.
type TextArea struct {
	win              ui.Window
	bounds           image.Rectangle
	lines, prevLines []*line
}

type line struct {
	spans      []*span
	w, h, base font.Fix32
	tex        ui.Texture
}

type span struct {
	TextStyle
	runes  []rune
	glyphs font.Glyphs
	x0, x1 font.Fix32
}

// Clear clears the TextArea.
func (ta *TextArea) Clear() {
	ta.lines, ta.prevLines = ta.prevLines[:0], ta.lines
}

// Add adds text with a given style to the TextArea.
func (ta *TextArea) Add(sty *TextStyle, rs []rune) {
	width := font.Fix32((ta.bounds.Dy() - 2*pad) << 8)
	if len(ta.lines) == 0 {
		ta.lines = append(ta.lines, new(line))
	}
	for len(rs) > 0 {
		l := ta.lines[len(ta.lines)-1]
		rs = l.addText(width, sty, rs)
		if len(l.spans) == 0 {
			// The area is too narrow for the text.
			ta.lines = ta.lines[:len(ta.lines)-1]
			break
		}
		if len(rs) > 0 {
			ta.lines = append(ta.lines, new(line))
		}
	}
}

func (l *line) addText(width font.Fix32, sty *TextStyle, rs []rune) []rune {
	x0 := l.nextX(sty, rs)
	sp := &span{TextStyle: *sty, x0: x0, x1: x0}

	for len(rs) > 0 {
		r := rs[0]
		if r == '\t' {
			x0 := sp.x1
			x1 := sty.tab(x0)
			if x1 > width {
				sp.x1 = width
				break
			}
			l.addSpan(sp)
			l.addSpan(&span{
				TextStyle: *sty,
				runes:     []rune{'\t'},
				x0:        x0,
				x1:        x1,
			})
			sp = &span{TextStyle: *sty, x0: x1, x1: x1}
			rs = rs[1:]
			continue
		}

		g := sty.Glyph(r)
		w := sty.Advance(g) + font.Kerning(sty.Font, sp.glyphs, g)
		if sp.x1+w > width {
			sp.x1 = width
			break
		}
		rs = rs[1:]
		sp.glyphs.Append(g)
		sp.runes = append(sp.runes, r)
		sp.x1 += w
		if r == '\n' {
			sp.x1 = width
			break
		}
	}
	l.addSpan(sp)
	return rs
}

func (l *line) addSpan(sp *span) {
	if len(sp.runes) == 0 {
		return
	}
	l.w = sp.x1
	if h := sp.Height(); h > l.h {
		l.h = h
	}
	if a := sp.Ascent(); a > l.base {
		l.base = a
	}
	l.spans = append(l.spans, sp)
}

func (l *line) nextX(sty *TextStyle, rs []rune) font.Fix32 {
	if len(l.spans) == 0 {
		return 0
	}
	sp := l.spans[len(l.spans)-1]
	if len(rs) == 0 {
		return sp.x1
	}
	var g0 font.Glyph
	if r := rs[0]; r == '\t' {
		g0 = sty.Glyph(' ')
	} else {
		g0 = sty.Glyph(r)
	}
	if sp.runes[len(sp.runes)-1] == '\t' {
		return sp.x1 + sty.Kerning(sty.Glyph(' '), g0)
	}
	return sp.x1 + font.Kerning(sp.Font, sp.glyphs, g0)
}

func (l *line) String() string {
	var s string
	for _, sp := range l.spans {
		s += string(sp.runes)
	}
	return s
}
