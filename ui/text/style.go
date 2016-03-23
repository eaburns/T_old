// Copyright © 2016, The T Authors.

package text

import (
	"image/color"
	"unicode"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// A Style describes a font face and colors.
type Style struct {
	Face   font.Face
	FG, BG color.Color

	height, ascent fixed.Int26_6
}

// Advance returns the advance width of the rune.
func (sty *Style) Advance(r rune) fixed.Int26_6 {
	adv, ok := sty.Face.GlyphAdvance(r)
	if !ok {
		adv, _ = sty.Face.GlyphAdvance(unicode.ReplacementChar)
	}
	return adv
}

// Height returns the line height of the Style's font.
func (sty *Style) Height() fixed.Int26_6 {
	sty.ensureVerticalMetrics()
	return sty.height
}

// Ascent returns the ascent of the Style's font.
func (sty *Style) Ascent() fixed.Int26_6 {
	sty.ensureVerticalMetrics()
	return sty.ascent
}

// BUG(eaburns): This is not the correct way to compute the font face height.
// However, the line height and ascent are not available from x/image/font,
// so we estimate it by computing the maximum height and ascent for ASCII.
func (sty *Style) ensureVerticalMetrics() {
	if sty.height > 0 {
		return
	}
	var descent fixed.Int26_6
	for r := rune(0); r < 128; r++ {
		b, _, ok := sty.Face.GlyphBounds(r)
		if !ok {
			continue
		}
		if a := -b.Min.Y; a > sty.ascent {
			sty.ascent = a
		}
		if d := b.Max.Y; d > descent {
			descent = d
		}
	}
	sty.height = sty.ascent + descent
}
