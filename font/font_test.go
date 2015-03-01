package font_test

import (
	"testing"

	"github.com/eaburns/T/font"
	"github.com/eaburns/T/font/testfont"
)

var tf = &testfont.Font{
	Adv: map[font.Glyph]font.Fix32{
		glyph('a'): 1 << 8,
		glyph('b'): 1 << 8,
		glyph('c'): 1 << 8,
		glyph('X'): 2 << 8,
		glyph('Y'): 2 << 8,
		glyph('Z'): 2 << 8,
	},
	Kern: map[[2]font.Glyph]font.Fix32{
		pair('Y', 'a'): -0x80, // -0.5
		pair('a', 'Y'): -0x80,
	},
}

func pair(p, c rune) [2]font.Glyph {
	return [2]font.Glyph{testfont.Glyph(p), testfont.Glyph(c)}
}

func glyph(r rune) font.Glyph { return testfont.Glyph(r) }

func TestWidth(t *testing.T) {
	tests := []struct {
		str string
		w   font.Fix32
	}{
		{"abc", 3 << 8},
		{"XYZ", 6 << 8},
		{"Ya", 2<<8 | 0x80},
		{"Yab", 3<<8 | 0x80},
		{"XYab", 5<<8 | 0x80},
		{"YaYa", 4<<8 | 0x80},
	}
	for _, test := range tests {
		var gs font.Glyphs
		for _, r := range test.str {
			gs.Append(tf.Glyph(r))
		}
		if w := font.Width(tf, gs); w != test.w {
			t.Errorf("Width(_, %q)=%v, want %v", test.str, w, test.w)
		}
	}
}

func TestKerning(t *testing.T) {
	tests := []struct {
		str string
		g   rune
		k   font.Fix32
	}{
		{"", 'a', 0},
		{"", 'Y', 0},
		{"abc", 'a', 0},
		{"XYZ", 'a', 0},
		{"abcY", 'Z', 0},
		{"Y", 'a', -0x80},
		{"abcY", 'a', -0x80},
		{"bca", 'Y', -0x80},
	}
	for _, test := range tests {
		var gs font.Glyphs
		for _, r := range test.str {
			gs.Append(tf.Glyph(r))
		}
		if k := font.Kerning(tf, gs, glyph(test.g)); k != test.k {
			t.Errorf("Kerning(_, %q, %c)=%v, want %v", test.str, test.g, k, test.k)
		}
	}
}
