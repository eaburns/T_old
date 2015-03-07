package gui

import (
	"image"
	"image/color"
	"image/draw"

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
	sp := sty.Advance(sty.Glyph(' '))
	w := sp * font.Fix32(sty.TabWidth)
	t := w - (x % w) + x
	if t-x < sp {
		return t + w
	}
	return t
}

// A TextArea is an area of a window containing text.
type TextArea struct {
	BG               color.Color
	Bounds           image.Rectangle
	win              ui.Window
	lines, nextLines []*line
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

func (l *line) String() string {
	var s string
	for _, sp := range l.spans {
		s += string(sp.runes)
	}
	return s
}

// NewTextArea returns a new TextArea for a ui.Window.
func NewTextArea(win ui.Window, b image.Rectangle, bg color.Color) *TextArea {
	return &TextArea{BG: bg, Bounds: b, win: win}
}

// Add adds text with a given style to the TextArea.
func (ta *TextArea) Add(sty *TextStyle, rs []rune) {
	width := font.Fix32((ta.Bounds.Dx() - 2*pad) << 8)
	if len(ta.nextLines) == 0 {
		ta.nextLines = append(ta.nextLines, new(line))
	}
	for len(rs) > 0 {
		l := ta.nextLines[len(ta.nextLines)-1]
		rs = l.addText(width, sty, rs)
		if len(l.spans) == 0 {
			// The area is too narrow for the text.
			ta.nextLines = ta.nextLines[:len(ta.nextLines)-1]
			break
		}
		if len(rs) > 0 {
			ta.nextLines = append(ta.nextLines, new(line))
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
		sp.runes = append(sp.runes, r)
		sp.x1 += w
		if r == '\n' {
			sp.x1 = width
			break
		}
		sp.glyphs.Append(g)
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

// Present stops displaying the current lines
// and displays the lines added since
// the last call to Present.
func (ta *TextArea) Present() {
	bg := image.NewUniform(ta.BG)
	used := make([]bool, len(ta.lines))
	var h1 int
	for _, l := range ta.nextLines {
		for i := range ta.lines {
			if !used[i] && l.eq(ta.lines[i]) {
				l.tex = ta.lines[i].tex
				used[i] = true
				break
			}
		}
		if l.tex == nil && l.w.Int() > 0 {
			l.tex = ta.win.Texture(l.w.Int(), l.h.Int())
			draw.Draw(l.tex, l.tex.Bounds(), bg, image.ZP, draw.Over)
			l.draw(l.tex)
		}
		h1 += l.h.Int()
	}

	var h0 int
	for _, l := range ta.lines {
		h0 += l.h.Int()
	}
	if h1 < h0 {
		// There use to be more lines.
		// Add an empty line to erase the old text.
		h := h0 - h1
		w := ta.Bounds.Dx() - pad
		e := &line{
			h:   font.Fix32(h << 8),
			w:   font.Fix32(w << 8),
			tex: ta.win.Texture(w, h),
		}
		draw.Draw(e.tex, e.tex.Bounds(), bg, image.ZP, draw.Over)
		ta.nextLines = append(ta.nextLines, e)
	}

	ta.lines, ta.nextLines = ta.nextLines, ta.lines
	for i, u := range used {
		l := ta.nextLines[i]
		if !u && l.tex != nil {
			ta.nextLines[i].tex.Close()
		}
	}
	ta.nextLines = ta.nextLines[:0]
}

func (l *line) draw(img draw.Image) {
	for _, sp := range l.spans {
		if l.tex == nil {
			continue
		}
		box := image.Rect(int(sp.x0>>8), 0, int(sp.x1>>8), int(l.h>>8))
		_, _, _, a := sp.BG.RGBA()
		if a > 0 {
			bg := image.NewUniform(sp.BG)
			draw.Draw(img, box, bg, image.ZP, draw.Over)
		}
		pt := font.Point{X: sp.x0, Y: l.base}
		sp.DrawGlyphs(img, sp.glyphs, sp.FG, pt)
	}
}

func (l *line) eq(l1 *line) bool {
	if l == nil || l1 == nil {
		return l == nil && l1 == nil
	}
	if l.w != l1.w || l.h != l1.h || len(l.spans) != len(l1.spans) {
		return false
	}
	for i, sp := range l.spans {
		if !sp.eq(l1.spans[i]) {
			return false
		}
	}
	return true
}

func (sp *span) eq(sp1 *span) bool {
	if sp.TextStyle != sp1.TextStyle || sp.x0 != sp1.x0 || sp.x1 != sp1.x1 || len(sp.runes) != len(sp1.runes) {
		return false
	}
	for j, r := range sp.runes {
		if sp1.runes[j] != r {
			return false
		}
	}
	return true
}

// Draw draws the TextArea to a ui.Canvas.
// The location is determined by the TextArea bounds.
func (ta *TextArea) Draw(c ui.Canvas) {
	c.Fill(ta.BG, ta.Bounds)
	x0 := ta.Bounds.Min.X + pad
	y := ta.Bounds.Min.Y + pad
	for _, l := range ta.lines {
		y1 := y + l.h.Int()
		if y1 > ta.Bounds.Max.Y-pad {
			break
		}
		if l.tex != nil {
			c.Draw(l.tex, image.Pt(x0, y))
		}
		y = y1
	}
}
