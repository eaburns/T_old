// Copyright © 2016, The T Authors.

package text

import (
	"bytes"
	"image"
	"testing"
	"unicode"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

func TestAdd(t *testing.T) {
	opts := Options{
		DefaultStyle: Style{Face: &unitFace{}},
		Size:         image.Pt(5, 5),
		TabWidth:     2,
	}

	tests := []struct {
		name string
		opts Options
		adds []string
		want string
	}{
		{
			name: "nothing added",
			opts: opts,
			adds: []string{},
			want: "",
		},
		{
			name: "add empty",
			opts: opts,
			adds: []string{"", "", ""},
			want: "",
		},
		{
			name: "single add fits line",
			opts: opts,
			adds: []string{"12345"},
			want: "[12345]",
		},
		{
			name: "multi-add fits line",
			opts: opts,
			adds: []string{"1", "2", "3", "4", "5"},
			want: "[12345]",
		},
		{
			name: "single add width breaks line",
			opts: opts,
			adds: []string{"1234567890abcde"},
			want: "[12345][67890][abcde]",
		},
		{
			name: "multi-add width breaks line",
			opts: opts,
			adds: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "0", "a", "b", "c", "d", "e"},
			want: "[12345][67890][abcde]",
		},
		{
			name: "newline breaks line",
			opts: opts,
			adds: []string{"1\n2\n", "3\n4\n5\n"},
			want: "[1\n][2\n][3\n][4\n][5\n]",
		},
		{
			name: "non-ASCII",
			opts: opts,
			adds: []string{"αβξδεφγθικ"},
			want: "[αβξδε][φγθικ]",
		},
		{
			name: "tab",
			opts: opts,
			adds: []string{"\t\t5", "6\t\t0", "a\t\t\tbreak"},
			want: "[\t\t5][6\t\t0][a\t\t\t][break]",
		},
		{
			name: "tab no less than space",
			opts: Options{
				DefaultStyle: advStyle(map[rune]fixed.Int26_6{
					' ': fixed.I(2),
					'a': fixed.I(1),
				}),
				Size:     image.Pt(5, 5),
				TabWidth: 1,
			},
			// Would tab to 1→2, but that is less than 2, so go ahead to 3.
			adds: []string{"a\taaaaaa"},
			want: "[a\ta][aaaaa]",
		},
		{
			name: "replacement rune",
			opts: Options{
				DefaultStyle: advStyle(map[rune]fixed.Int26_6{
					unicode.ReplacementChar: fixed.I(1),
				}),
				Size: image.Pt(5, 5),
			},
			// Would tab to 1→2, but that is less than 2, so go ahead to 3.
			adds: []string{"1234567890"},
			want: "[12345][67890]",
		},
		{
			name: "stop adding when max height is exceeded",
			opts: Options{
				DefaultStyle: Style{Face: &unitFace{}},
				Size:         image.Pt(1, 2),
				TabWidth:     2,
			},
			adds: []string{"12345", "67890"},
			want: "[1][2][3]",
		},
		{
			name: "add to empty line doesn't fit",
			opts: Options{
				DefaultStyle: Style{Face: &unitFace{}},
				Size:         image.Pt(0, 10),
				TabWidth:     2,
			},
			adds: []string{"12345"},
			want: "[1][2][3][4][5]",
		},
	}

	for _, test := range tests {
		s := NewSetter(test.opts)
		for _, str := range test.adds {
			s.Add([]byte(str))
		}
		if got := lineString(s.Set()); got != test.want {
			t.Errorf("%s s.Set()=%q, want, %q", test.name, got, test.want)
		}
	}
}

func TestAddVerticalMetrics(t *testing.T) {
	tallHeight, tallAscent := fixed.I(1000), fixed.I(800)
	tall := Style{
		Face: &testFace{
			adv:    map[rune]fixed.Int26_6{'a': fixed.I(1)},
			height: tallHeight,
			ascent: tallAscent,
		},
	}
	mediumHeight, mediumAscent := fixed.I(100), fixed.I(80)
	medium := Style{
		Face: &testFace{
			adv:    map[rune]fixed.Int26_6{'a': fixed.I(1)},
			height: mediumHeight,
			ascent: mediumAscent,
		},
	}
	short := Style{
		Face: &testFace{
			adv:    map[rune]fixed.Int26_6{'a': fixed.I(1)},
			height: fixed.I(10),
			ascent: fixed.I(8),
		},
	}
	opts := Options{
		DefaultStyle: medium,
		Size:         image.Pt(5, 100000),
	}
	s := NewSetter(opts)

	// First line has the tall height, which is taller than the default.
	s.Add([]byte{'a'})
	s.AddStyle(&tall, []byte{'a', '\n'})

	// Second line has the medium height, since short is shorter than default.
	s.Add([]byte{'a'})
	s.AddStyle(&short, []byte{'a', '\n'})

	txt := s.Set()

	if len(txt.lines) != 2 {
		t.Fatalf("txt.len(%v)=%d, want 2", txt.lines, len(txt.lines))
	}
	if x := txt.lines[0].h; x != tallHeight {
		t.Errorf("txt.lines[0].h=%v, want %v", x, tallHeight)
	}
	if x := txt.lines[0].a; x != tallAscent {
		t.Errorf("txt.lines[0].a=%v, want %v", x, tallAscent)
	}
	if x := txt.lines[1].h; x != mediumHeight {
		t.Errorf("txt.lines[1].h=%v, want %v", x, mediumHeight)
	}
	if x := txt.lines[1].a; x != mediumAscent {
		t.Errorf("txt.lines[1].a=%v, want %v", x, mediumAscent)
	}
}

func TestLinesHeight(t *testing.T) {
	const (
		pad = 3
		// Height fits 10 unit-height lines plus 2*pad.
		height = 16
	)
	s := NewSetter(Options{
		DefaultStyle: Style{Face: &unitFace{}},
		Size:         image.Pt(1000, height),
		Padding:      pad,
	})

	// Lines less than size.
	s.Add([]byte("1\n2\n3"))
	txt := s.Set()
	if txt.Size().Y != height {
		t.Errorf("txt.Size().Y=%d, want %d", txt.Size().Y, height)
	}
	if h := txt.LinesHeight(); h != len(txt.lines)+2*pad {
		t.Errorf("without trailing newline txt.LinesHeight()=%d, want %d",
			h, len(txt.lines)+2*pad)
	}

	// Lines less than size, trailing newline
	s.Add([]byte("1\n2\n3\n"))
	txt = s.Set()
	if txt.Size().Y != height {
		t.Errorf("txt.Size().Y=%d, want %d", txt.Size().Y, height)
	}
	if h := txt.LinesHeight(); h != len(txt.lines)+1+2*pad {
		t.Errorf("with trailing newline txt.LinesHeight()=%d, want %d",
			h, len(txt.lines)+1+2*pad)
	}

	// Size less than line height.
	s.Add([]byte("1\n2\n3\n4\n5\n6\n7\n8\n9\n0\nX\nY\nZ"))
	txt = s.Set()
	if txt.Size().Y != height {
		t.Errorf("txt.Size().Y=%d, want %d", txt.Size().Y, height)
	}
	const maxLines = 10 // Only 10 lines fit the height.
	if h := txt.LinesHeight(); h != maxLines+2*pad {
		t.Errorf("size less than height txt.LinesHeight()=%d, want %d",
			h, maxLines+2*pad)
	}

	// Height less than padding
	s0 := NewSetter(Options{
		DefaultStyle: Style{Face: &unitFace{}},
		Size:         image.Pt(5, pad/2),
		Padding:      pad,
	})
	s0.Add([]byte("1\n2\n3\n"))
	txt = s0.Set()
	if h := txt.LinesHeight(); h != s0.opts.Size.Y {
		t.Errorf("height less than padding txt.LinesHeight()=%d, want %d",
			h, s0.opts.Size.Y)
	}

	// Negative height.
	s1 := NewSetter(Options{
		DefaultStyle: Style{Face: &unitFace{}},
		Size:         image.Pt(5, -1),
		Padding:      pad,
	})
	s1.Add([]byte("1\n2\n3\n"))
	txt = s1.Set()
	if h := txt.LinesHeight(); h != 0 {
		t.Errorf("negative height txt.LinesHeight()=%d, want %d", h, 0)
	}
}

func TestReset(t *testing.T) {
	s := NewSetter(Options{
		DefaultStyle: Style{Face: &unitFace{}},
		Size:         image.Pt(5, 5),
	})

	s.Add([]byte("1234567890abcde"))

	s.Reset(Options{
		DefaultStyle: Style{Face: &unitFace{}},
		Size:         image.Pt(10, 5),
	})

	s.Add([]byte("1234567890abcde"))

	// Previously added text is removed.
	// Lines break at 10, not 5.
	want := "[1234567890][abcde]"
	got := lineString(s.Set())
	if want != got {
		t.Errorf("got=%q, want=%q", got, want)
	}
}

func TestTextIndex(t *testing.T) {
	s := NewSetter(Options{
		DefaultStyle: Style{
			Face: &testFace{
				adv: map[rune]fixed.Int26_6{
					'α': fixed.I(10),
					'β': fixed.I(10),
					'ξ': fixed.I(10),
					'd': fixed.I(10),
					' ': fixed.I(10),
					'f': fixed.I(10),
					'←': fixed.I(10),
					'→': fixed.I(10),
				},
				height: fixed.I(10),
			}},
		Size:     image.Pt(50, 50),
		Padding:  10,
		TabWidth: 1,
	})
	s.Add([]byte("αβξ"))
	s.Add([]byte("d\tf"))
	s.Add([]byte("←→"))
	txt := s.Set()

	// 10x10 px squares.
	// We check the index at the middle point of each.
	//   01234
	// 0 _____
	// 1 _αβξ_
	// 2 _d\tf_
	// 3 _←→__
	// 4 _____
	wants := [25]rune{
		'α', 'α', 'α', 'α', 'α',
		'α', 'α', 'β', 'ξ', 'd',
		'd', 'd', '\t', 'f', '←',
		'←', '←', '→', '·', '·',
		'·', '·', '·', '·', '·',
	}
	text := []byte("αβξd\tf←→·")
	for y := 0; y < 5; y++ {
		for x := 0; x < 5; x++ {
			pt := image.Pt(10*x+5, 10*y+5)
			want := wants[y*5+x]
			wanti := bytes.IndexRune([]byte(text), want)
			goti := txt.Index(pt)
			got, _ := utf8.DecodeRune(text[goti:])
			if got != want {
				t.Errorf("txt.Index(%v)=%d (%q), want %d (%q)",
					pt, goti, got, wanti, want)
			}
		}
	}
}

func TestTextGlyphBox(t *testing.T) {
	const (
		pad        = 3
		lineHeight = 1
	)

	opts := Options{
		DefaultStyle: Style{Face: &unitFace{}},
		Size:         image.Pt(100, 100),
		Padding:      pad,
		TabWidth:     2,
	}

	tests := []struct {
		name  string
		opts  Options
		text  string
		index int
		want  image.Rectangle
	}{
		{
			name:  "empty text",
			opts:  opts,
			text:  "",
			index: 0,
			want:  image.Rect(pad, pad, pad, lineHeight+pad),
		},
		{
			name: "text too small for padding",
			opts: Options{
				DefaultStyle: Style{Face: &unitFace{}},
				Size:         image.Pt(1, 1),
				Padding:      pad,
			},
			text:  "abc\ndef",
			index: 1,
			want:  image.ZR,
		},
		{
			name:  "index beyond end",
			opts:  opts,
			text:  "abc\ndef",
			index: 8,
			// We want the empty rectangle after 'f', the 3rd glyph of line 2.
			want: image.Rect(pad+3, pad+1, pad+3, pad+2),
		},
		{
			name:  "index way beyond end",
			opts:  opts,
			text:  "abc\ndef",
			index: 8000,
			// We want the empty rectangle after 'f', the 3rd glyph of line 2.
			want: image.Rect(pad+3, pad+1, pad+3, pad+2),
		},
		{
			name:  "negative index",
			opts:  opts,
			text:  "abc\ndef",
			index: -1,
			want:  image.Rect(pad, pad, pad+1, pad+1),
		},
		{
			name:  "first line first rune",
			opts:  opts,
			text:  "abc\ndef",
			index: 0,
			want:  image.Rect(pad, pad, pad+1, pad+1),
		},
		{
			name:  "first line second rune",
			opts:  opts,
			text:  "abc\ndef",
			index: 1,
			want:  image.Rect(pad+1, pad, pad+2, pad+1),
		},
		{
			name:  "first line last rune",
			opts:  opts,
			text:  "abc\ndef",
			index: 3,
			want:  image.Rect(pad+3, pad, pad+4, pad+1),
		},
		{
			name:  "second line first rune",
			opts:  opts,
			text:  "abc\ndef",
			index: 4,
			want:  image.Rect(pad, pad+1, pad+1, pad+2),
		},
		{
			name:  "tab",
			opts:  opts,
			text:  "a\tb\tc",
			index: 1,
			want:  image.Rect(pad+1, pad, pad+3, pad+1),
		},
		{
			name:  "trailing newline",
			opts:  opts,
			text:  "a\n",
			index: 3,
			// There is a newline at the end,
			// so the box beyond the text
			// is the start of the next line.
			want: image.Rect(pad, pad+1, pad, pad+2),
		},
		{
			name: "last line extends beyond ymax",
			opts: Options{
				DefaultStyle: Style{Face: &unitFace{}},
				Size:         image.Pt(100, 2*pad+1),
				Padding:      pad,
			},
			text:  "a\nb",
			index: 3,
			// Only 1 line fits with padding.
			// B will be added, but it will extend just beyond ymax,
			// so its box is reported as the zero Rectangle.
			want: image.ZR,
		},
	}

	for _, test := range tests {
		s := NewSetter(test.opts)
		if test.text != "" {
			s.Add([]byte(test.text))
		}
		txt := s.Set()
		if got := txt.GlyphBox(test.index); got != test.want {
			t.Errorf("%s txt.GlyphBox(%d)=%v, want %v",
				test.name, test.index, got, test.want)
		}
	}
}

func lineString(t *Text) string {
	buf := bytes.NewBuffer(nil)
	for _, l := range t.lines {
		buf.WriteRune('[')
		for _, s := range l.spans {
			buf.WriteString(s.text)
		}
		buf.WriteRune(']')
	}
	return buf.String()
}

type unitFace struct{}

func (unitFace) Close() error { return nil }

func (unitFace) Glyph(fixed.Point26_6, rune) (image.Rectangle, image.Image, image.Point, fixed.Int26_6, bool) {
	panic("unimplemented")
}

func (unitFace) GlyphAdvance(rune) (fixed.Int26_6, bool) { return fixed.I(1), true }

func (unitFace) Kern(rune, rune) fixed.Int26_6 { return 0 }

func (unitFace) GlyphBounds(rune) (fixed.Rectangle26_6, fixed.Int26_6, bool) {
	return fixed.R(0, 0, 1, 1), fixed.I(1), true
}

func (unitFace) Metrics() font.Metrics {
	return font.Metrics{Height: fixed.I(1), Ascent: fixed.I(1)}
}

func advStyle(adv map[rune]fixed.Int26_6) Style {
	return Style{Face: &testFace{adv: adv, height: fixed.I(1)}}
}

type testFace struct {
	adv            map[rune]fixed.Int26_6
	kern           map[[2]rune]fixed.Int26_6
	height, ascent fixed.Int26_6
}

func (testFace) Close() error { return nil }

func (testFace) Glyph(fixed.Point26_6, rune) (image.Rectangle, image.Image, image.Point, fixed.Int26_6, bool) {
	panic("unimplemented")
}

func (f testFace) GlyphAdvance(r rune) (fixed.Int26_6, bool) {
	a, ok := f.adv[r]
	return a, ok
}

func (f testFace) Kern(r0, r1 rune) fixed.Int26_6 { return f.kern[[2]rune{r0, r1}] }

func (f testFace) GlyphBounds(r rune) (fixed.Rectangle26_6, fixed.Int26_6, bool) {
	a, ok := f.adv[r]
	if !ok {
		return fixed.Rectangle26_6{}, 0, false
	}
	b := fixed.Rectangle26_6{
		Min: fixed.Point26_6{Y: -f.ascent},
		Max: fixed.Point26_6{X: a, Y: f.height - f.ascent},
	}
	return b, a, true
}

func (f testFace) Metrics() font.Metrics {
	return font.Metrics{Height: f.height, Ascent: f.ascent}
}
