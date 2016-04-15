// Copyright © 2016, The T Authors.

// Package text handles rich-formatted text layout and drawing.
//
// A Setter lays out styalized text into a bounding box
// by repeatedly calling Add or AddStyle, and then calling Set.
// Set returns a Text which contains the text
// from all previous calls to Add or AddStyle.
//
// A Text can be queried for the byte-index of points
// and it can be drawn to a window.
// Rasterization of the lines of text is done lazily.
// Once finished, Text.Release releases the rasterized lines
// back to its setter to be reused by the next call to Set.
//
// A typical use
//
// First create a setter, add bytes to the setter,
// set it into a Text, and draw the Text.
//
// When the text changes,
// release the old Text, re-add the bytes to the setter,
// re-set it into a new Text, and draw the new Text.
//
// The new Text re-uses pre-rendered lines from the old text.
// In the common case, where little changed,
// drawing the new Text is very efficient.
package text

import (
	"image"
	"image/color"
	"image/draw"
	"unicode"
	"unicode/utf8"

	"golang.org/x/exp/shiny/screen"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// A Style describes a font face and colors.
type Style struct {
	Face   font.Face
	FG, BG color.Color
}

// Options control text layout by a setter.
type Options struct {
	// Size is the size of the Text returned by Set.
	Size image.Point

	// DefaultStyle dictates
	// the default background color of text,
	// the minimum line height of lines of text,
	// and the units of tab width.
	DefaultStyle Style

	// TabWidth is the number of DefaultStyle space-widths
	// between tab stops.
	TabWidth int

	// Padding is the number of pixels
	// between the borders of Bounds
	// and the Text.
	Padding int
}

// A Setter lays out text to fit in a rectangle.
type Setter struct {
	opts              Options
	lines, reuseLines []*line
}

type line struct {
	spans   []*span
	w, h, a fixed.Int26_6
	buf     screen.Buffer
}

type span struct {
	Style
	text   string
	x0, x1 fixed.Int26_6
}

// NewSetter returns a new Setter.
func NewSetter(opts Options) *Setter { return &Setter{opts: opts} }

// Release releases the resources of the Setter.
//
// The Setter may continue to be used after calling Release.
func (s *Setter) Release() {
	for _, l := range append(s.lines, s.reuseLines...) {
		if l.buf != nil {
			l.buf.Release()
		}
	}
}

// Reset clears any added lines, and resets the setter with new Options.
func (s *Setter) Reset(opts Options) {
	s.lines = s.lines[:0]
	s.opts = opts
}

// Tab returns the next tab stop.
func (s *Setter) tab(x fixed.Int26_6) fixed.Int26_6 {
	sp := advance(&s.opts.DefaultStyle, ' ')
	w := sp * fixed.Int26_6(s.opts.TabWidth)
	t := w - (x % w) + x
	if t-x < sp {
		return t + w
	}
	return t
}

// Add adds text to the Setter using the default style.
func (s *Setter) Add(text []byte) { s.AddStyle(&s.opts.DefaultStyle, text) }

// AddStyle adds text to the Setter using the given style.
func (s *Setter) AddStyle(sty *Style, text []byte) {
	if len(text) == 0 {
		return
	}

	ymax := fixed.I(s.opts.Size.Y - 2*s.opts.Padding)
	var h fixed.Int26_6
	for _, l := range s.lines {
		h += l.h
	}
	if h > ymax {
		return
	}

	m := s.opts.DefaultStyle.Face.Metrics()
	if len(s.lines) == 0 {
		s.lines = append(s.lines, &line{h: m.Height, a: m.Ascent})
	}
	for len(text) > 0 {
		text = add1(s, sty, text)
		if len(text) > 0 {
			h += s.lines[len(s.lines)-1].h
			if h > ymax {
				break
			}
			s.lines = append(s.lines, &line{h: m.Height, a: m.Ascent})
		}
	}
}

func add1(s *Setter, sty *Style, text []byte) []byte {
	l := s.lines[len(s.lines)-1]
	var x0 fixed.Int26_6
	width := fixed.I(s.opts.Size.X - 2*s.opts.Padding)
	if len(l.spans) > 0 && len(l.spans[len(l.spans)-1].text) > 0 {
		lastSpan := l.spans[len(l.spans)-1]
		lastText := lastSpan.text
		if r, _ := utf8.DecodeLastRuneInString(lastText); r == '\n' {
			return text
		}
		x0 = lastSpan.x1
		if len(text) > 0 && lastSpan.Face == sty.Face {
			r, _ := utf8.DecodeRune(text)
			if len(lastText) > 0 {
				p, _ := utf8.DecodeLastRuneInString(lastText)
				x0 += sty.Face.Kern(p, r)
			}
		}
	}
	sp := &span{Style: *sty, x0: x0, x1: x0}
	var start, i int
	for i < len(text) {
		r, w := utf8.DecodeRune(text[i:])
		adv := advance(sty, r)
		if i > 0 {
			p, _ := utf8.DecodeLastRune(text[:i])
			adv += sty.Face.Kern(p, r)
		}
		if r == '\t' {
			adv = s.tab(sp.x1) - sp.x1
		}
		if r == '\n' || sp.x1+adv > width {
			// Always add newline or non-fitting tabs to the end of the line,
			// but ignore their width.
			if r == '\n' || r == '\t' {
				i += w
			}
			// If the line is empty and the first rune doesn't fit, add it anyway,
			// and bump the line width up so it's too wide to draw.
			if len(l.spans) == 0 && i == 0 {
				i += w
				sp.x1 += adv
			}
			break
		}
		i += w
		sp.x1 += adv
	}

	m := sp.Face.Metrics()
	if m.Height > l.h {
		l.h = m.Height
	}
	if m.Ascent > l.a {
		l.a = m.Ascent
	}
	l.w = sp.x1
	sp.text = string(text[start:i])
	l.spans = append(l.spans, sp)
	return text[i:]
}

func advance(sty *Style, r rune) fixed.Int26_6 {
	adv, ok := sty.Face.GlyphAdvance(r)
	if !ok {
		adv, _ = sty.Face.GlyphAdvance(unicode.ReplacementChar)
	}
	return adv
}

// Set returns the Text containing the text from all calls to Add or AddStyle
// since the previous call to Set.
//
// Where possible, the returned Text uses pre-rasterized lines
// that were released to the Setter
// by the previous call to Text.Release.
func (s *Setter) Set() *Text {
	var h1 int
	for _, line := range s.lines {
		// Find resue line with the exact same spans and reuse its buffer.
		for _, reuseLine := range s.reuseLines {
			if reuseLine.buf == nil || len(reuseLine.spans) != len(line.spans) {
				continue
			}
			match := true
			for i, reuseSpan := range reuseLine.spans {
				span := line.spans[i]
				if reuseSpan.Style != span.Style || reuseSpan.text != span.text {
					match = false
					break
				}
			}
			if match {
				line.buf = reuseLine.buf
				reuseLine.buf = nil
				break
			}
		}
		h1 += int(line.h >> 6)
	}
	t := &Text{setter: s, lines: s.lines, size: s.opts.Size}
	for _, l := range s.reuseLines {
		if l.buf != nil {
			l.buf.Release()
			l.buf = nil
		}
	}
	s.lines = s.reuseLines[:0]
	s.reuseLines = nil
	return t
}

// A Text is a type-set text.
// BUG(eaburns): The text often reaches into the setter's opts.
// If the opts change, the Text will be broken.
type Text struct {
	setter *Setter
	lines  []*line
	size   image.Point
}

// Size returns the size of the Text.
func (t *Text) Size() image.Point { return t.size }

// Release releases the rasterized lines of the Text
// back to the Setter that created it
// for reuse by the next call to Set.
//
// The Text should no longer be used after it is released.
//
// To release the resources back to the operating system,
// first release them to the Setter using this method,
// then call Setter.Release.
func (t *Text) Release() {
	t.setter.reuseLines = append(t.setter.reuseLines, t.lines...)
	t.lines = nil
}

// Index returns the byte index into the text
// corresponding to the glyph at the given point.
// 0,0 is the top left of the text.
func (t *Text) Index(p image.Point) int {
	px, py := fixed.I(p.X), fixed.I(p.Y)
	pad := t.setter.opts.Padding
	y := fixed.I(pad)
	if len(t.lines) == 0 || py < y {
		return 0
	}

	var i, l int
	for l = 0; l < len(t.lines); l++ {
		line := t.lines[l]
		y += line.h
		if y > py {
			break
		}
		i += line.len()
	}
	if l >= len(t.lines) {
		return i
	}

	var w int
	line := t.lines[l]
	for _, sp := range line.spans {
		x := sp.x0 + fixed.I(pad)
		var j int
		for j < len(sp.text) {
			var r rune
			r, w = utf8.DecodeRuneInString(sp.text[j:])
			if r == '\t' {
				x = t.setter.tab(x)
			} else {
				x += advance(&sp.Style, r)
				if j > 0 {
					p, _ := utf8.DecodeLastRuneInString(sp.text[:j])
					x += sp.Face.Kern(p, r)
				}
			}
			if x > px {
				return i
			}
			j += w
			i += w
		}
	}
	return i - w
}

// Len returns the length of the line in bytes.
func (l *line) len() int {
	var n int
	for i := range l.spans {
		n += len(l.spans[i].text)
	}
	return n
}

// Draw draws the Text to the Window.
// The entire size is filled even if the lines of text do not occupy the entire space.
func (t *Text) Draw(at image.Point, scr screen.Screen, win screen.Window) {
	y0 := t.DrawLines(at, scr, win)
	bg := t.setter.opts.DefaultStyle.BG
	x0, x1, y1 := at.X, at.X+t.size.X, at.Y+t.size.Y
	win.Fill(image.Rect(x0, y0, x1, y1), bg, draw.Src)
}

// LinesHeight returns the height of the lines of text.
// This may differ from Size().Y,
// because the lines may not occupy the entire vertical space
// allowed by the text.
func (t *Text) LinesHeight() int {
	pad := t.setter.opts.Padding
	if t.size.Y < 0 {
		return 0
	}
	if t.size.Y-2*pad < 0 {
		return t.size.Y
	}
	y := pad
	for _, l := range t.lines {
		h := int(l.h >> 6)
		if y+h > t.size.Y-pad {
			break
		}
		y += h
	}
	if h := trailingNewlineHeight(t); h > 0 && y+h <= t.size.Y-pad {
		y += h
	}
	return y + pad
}

// DrawLines draws the lines of text and padding.
// If the lines stop short of the maximum height,
// bottom padding is drawn, but the full height is not filled.
// However, the entire width of each line is filled
// even if the line does not use the full width.
//
// The return value is the first y pixel after the bottom padding.
func (t *Text) DrawLines(at image.Point, scr screen.Screen, win screen.Window) int {
	pad := t.setter.opts.Padding
	bg := t.setter.opts.DefaultStyle.BG
	x0, y0, x1, y1 := at.X, at.Y, at.X+t.size.X, at.Y+t.size.Y
	if y1 < y0 {
		y1 = y0
	}
	if x1 < x0 {
		x1 = x0
	}
	if t.size.X < pad*2 || t.size.Y < pad*2 {
		// Too small, just fill what's there with background.
		win.Fill(image.Rect(x0, y0, x1, y1), bg, draw.Src)
		return y1
	}

	var y int
	x, ynext := at.X+pad, at.Y+pad
	textWidth := (x1 - x0) - 2*pad
	for _, l := range t.lines {
		y = ynext
		ynext = y + int(l.h>>6)
		if ynext > y1-pad {
			ynext = y
			break
		}
		if l.buf == nil && int(l.w>>6) > 0 {
			var err error
			size := image.Pt(int(l.w>>6), int(l.h>>6))
			l.buf, err = scr.NewBuffer(size)
			if err != nil {
				panic(err)
			}
			drawLine(t, l, l.buf.RGBA())
		}
		var dx int
		if l.buf != nil && l.w <= fixed.I(textWidth) {
			b := l.buf.Bounds()
			if b.Dx() > textWidth {
				b.Max.X = b.Min.X + textWidth
			}
			dx = b.Dx()
			win.Upload(image.Pt(x, y), l.buf, b)
		}
		if dx < textWidth {
			lineBG := bg
			if len(l.spans) > 0 {
				lineBG = l.spans[len(l.spans)-1].BG
			}
			win.Fill(image.Rect(x+dx, y, x1-pad, ynext), lineBG, draw.Src)
		}
	}
	if h := trailingNewlineHeight(t); h > 0 && ynext+h <= y1-pad {
		win.Fill(image.Rect(x, ynext, x1-pad, ynext+h), bg, draw.Src)
		ynext += h
	}
	y1 = ynext
	win.Fill(image.Rect(x0, y0, x1, y0+pad), bg, draw.Src)     // top
	win.Fill(image.Rect(x0, y0+pad, x0+pad, y1), bg, draw.Src) // left
	win.Fill(image.Rect(x1-pad, y0+pad, x1, y1), bg, draw.Src) // right
	win.Fill(image.Rect(x0, y1, x1, y1+pad), bg, draw.Src)     // bottom
	return y1 + pad
}

func trailingNewlineHeight(t *Text) int {
	// If the last line ends with a newline,
	// add the height of one more empty line if it fits.
	if len(t.lines) > 0 {
		l := t.lines[len(t.lines)-1]
		if len(l.spans) > 0 {
			s := l.spans[len(l.spans)-1]
			r, _ := utf8.DecodeLastRuneInString(s.text)
			if r == '\n' {
				return int(t.setter.opts.DefaultStyle.Face.Metrics().Height >> 6)
			}
		}
	}
	return 0
}

func drawLine(t *Text, l *line, img draw.Image) {
	for _, sp := range l.spans {
		fg := image.NewUniform(sp.FG)
		bg := image.NewUniform(sp.BG)
		box := image.Rect(int(sp.x0>>6), 0, int(sp.x1>>6), int(l.h>>6))
		draw.Draw(img, box, bg, image.ZP, draw.Src)
		x := sp.x0
		for i, r := range sp.text {
			if r == '\t' {
				x = t.setter.tab(x)
				continue
			}
			if r == '\n' {
				continue
			}
			if i > 0 {
				p, _ := utf8.DecodeLastRuneInString(sp.text[:i])
				x += sp.Face.Kern(p, r)
			}
			pt := fixed.Point26_6{X: x, Y: l.a}
			dr, mask, maskp, _, ok := sp.Face.Glyph(pt, r)
			if !ok {
				dr, mask, maskp, _, _ = sp.Face.Glyph(pt, unicode.ReplacementChar)
			}
			draw.DrawMask(img, dr, fg, image.ZP, mask, maskp, draw.Over)
			x += advance(&sp.Style, r)
		}
	}
}
