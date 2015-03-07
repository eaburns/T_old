package gui

import (
	"image"
	"reflect"
	"testing"

	"github.com/eaburns/T/font"
	"github.com/eaburns/T/font/testfont"
)

var testFont = &testfont.Font{
	A: 4 << 8,
	H: 5 << 8,
	Adv: map[font.Glyph]font.Fix32{
		testfont.Glyph(' '): 0x80,
		testfont.Glyph('a'): 2 << 8,
		testfont.Glyph('b'): 2 << 8,
		testfont.Glyph('c'): 2 << 8,
		testfont.Glyph('d'): 2 << 8,
		testfont.Glyph('e'): 2 << 8,
		testfont.Glyph('f'): 2 << 8,
		testfont.Glyph('X'): 4 << 8,
		testfont.Glyph('Y'): 4 << 8,
		testfont.Glyph('Z'): 4 << 8,
	},
	Kern: map[[2]font.Glyph]font.Fix32{
		[2]font.Glyph{testfont.Glyph('Y'), testfont.Glyph('a')}: -(1 << 8),
	},
}

func TestTab(t *testing.T) {
	tests := []struct {
		width  int
		x, tab font.Fix32
	}{
		{width: 4, x: 0, tab: 2 << 8},
		{width: 4, x: 1 << 8, tab: 2 << 8},
		{width: 4, x: 1<<8 | 0x80, tab: 2 << 8},
		{width: 4, x: 2 << 8, tab: 4 << 8},
		{width: 4, x: 3 << 8, tab: 4 << 8},
		{width: 4, x: 3<<8 | 0x80, tab: 4 << 8},
		{width: 3, x: 0, tab: 1<<8 | 0x80},
		{width: 3, x: 1 << 8, tab: 1<<8 | 0x80},
		{width: 3, x: 2<<8 | 0x80, tab: 3 << 8},
		{width: 3, x: 1<<8 | 0x10, tab: 3 << 8},
	}
	for _, test := range tests {
		sty := &TextStyle{Font: testFont, TabWidth: test.width}
		if got := sty.tab(test.x); got != test.tab {
			t.Errorf("TextStyle{TabWidth: %d}.tab(%v)=%v, want %v",
				test.width, test.x, got, test.tab)
		}
	}
}

// Tests that add correctly breaks lines.
func TestAddLines(t *testing.T) {
	const tabWidth = 4

	tests := []struct {
		width int
		add   []string
		lines []string
	}{
		{6, []string{""}, []string{""}},
		{6, []string{"\n"}, []string{"\n"}},
		{6, []string{"abc"}, []string{"abc"}},
		{6, []string{"a", "b", "c"}, []string{"abc"}},
		{6, []string{"a\nb\nc\n"}, []string{"a\n", "b\n", "c\n"}},
		{6, []string{"abcd"}, []string{"abc", "d"}},
		{6, []string{"abcde"}, []string{"abc", "de"}},
		{6, []string{"abcdef"}, []string{"abc", "def"}},
		{6, []string{"a", "b", "c", "d", "e", "f"}, []string{"abc", "def"}},
		{6, []string{"YZ"}, []string{"Y", "Z"}},
		{6, []string{"Y", "Z"}, []string{"Y", "Z"}},

		// kerning.
		{7, []string{"Y", "a"}, []string{"Ya"}},
		{7, []string{"Y", "aa"}, []string{"Yaa"}}, // 4 + 2 - 1 + 2 = 7
		{6, []string{"Y", "aa"}, []string{"Ya", "a"}},
		{6, []string{"Y", ""}, []string{"Y"}},

		// tabs
		{4, []string{"\ta"}, []string{"\ta"}},
		{4, []string{"\t", "a"}, []string{"\ta"}},
		{4, []string{"a\t"}, []string{"a\t"}},
		{4, []string{"a", "\t"}, []string{"a\t"}},
		{4, []string{"\t\t"}, []string{"\t\t"}},
		{4, []string{"\t\ta"}, []string{"\t\t", "a"}},
		{4, []string{" \t"}, []string{" \t"}},         // 0.5→2
		{4, []string{"  \t"}, []string{"  \t"}},       // 0.5+0.5→2
		{4, []string{"   \t"}, []string{"   \t"}},     // 0.5+0.5+0.5→2
		{4, []string{"    \t"}, []string{"    \t"}},   // 0.5+0.5+0.5+0.5→4
		{4, []string{"     \t"}, []string{"     \t"}}, // 0.5+0.5+0.5+0.5+0.5→4
		{4, []string{"\t\t\t"}, []string{"\t\t", "\t"}},
		{4, []string{"\t\t\t\t\t\t\t"}, []string{"\t\t", "\t\t", "\t\t", "\t"}},
		{5, []string{"\t\t\t\t\t\t\t"}, []string{"\t\t", "\t\t", "\t\t", "\t"}},
		{5, []string{"aa \t"}, []string{"aa ", "\t"}},

		{1, []string{"Y"}, nil},               // nothing fits.
		{3, []string{"aYzzz"}, []string{"a"}}, // a fits, but Y doesn't.
	}
	for _, test := range tests {
		ta := TextArea{Bounds: image.Rect(0, 0, test.width+pad*2, 0)}
		sty := &TextStyle{Font: testFont, TabWidth: tabWidth}
		for _, s := range test.add {
			ta.Add(sty, []rune(s))
		}
		var lines []string
		for _, l := range ta.nextLines {
			lines = append(lines, l.String())
		}
		if !reflect.DeepEqual(lines, test.lines) {
			t.Errorf("Add(%#v)=%#v, want %#v", test.add, lines, test.lines)
		}
	}
}
