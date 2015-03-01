// Package testfont implements a fake font for use in tests.
package testfont

import (
	"image/color"
	"image/draw"

	"github.com/eaburns/T/font"
)

// A Font is a fake font for use in testing.
type Font struct {
	// A and H are the font's ascent and height.
	A, H font.Fix32
	// Adv maps each Glyph to it's advance width.
	Adv map[font.Glyph]font.Fix32
	// Kern maps each Glyph pair to it's kerning.
	Kern map[[2]font.Glyph]font.Fix32
}

// Glyph returns a testfont.Font Glyph for the rune.
func Glyph(r rune) font.Glyph { return font.Glyph(r) }

// Glyph returns the rune, converted into a Glyph.
// The conversion simply truncates the rune to uint16.
func (f *Font) Glyph(r rune) font.Glyph { return Glyph(r) }

// Ascent returns the test font's ascent, A.
func (f *Font) Ascent() font.Fix32 { return f.A }

// Height returns the test font's height, H.
func (f *Font) Height() font.Fix32 { return f.H }

// Advance returns the test font's advance for a glyph.
func (f *Font) Advance(g font.Glyph) font.Fix32 { return f.Adv[g] }

// Kerning returns the test font's kerning for a glyph pair
func (f *Font) Kerning(p, c font.Glyph) font.Fix32 {
	return f.Kern[[2]font.Glyph{p, c}]
}

// DrawGlyphs is a no-op.
func (f *Font) DrawGlyphs(draw.Image, font.Glyphs, color.Color, font.Point) {}
