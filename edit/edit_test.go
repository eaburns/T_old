// Copyright © 2015, The T Authors.

package edit

import (
	"regexp"
	"strconv"
	"testing"

	"github.com/eaburns/T/re1"
)

func TestRetry(t *testing.T) {
	ed := NewEditor(NewBuffer())
	defer ed.Close()

	const str = "Hello, 世界!"
	ch := func() (addr, error) {
		if ed.buf.seq < 10 {
			// Simulate concurrent changes, necessitating retries.
			ed.buf.seq++
		}
		return change(ed, All, []rune(str))
	}
	if err := ed.do(ch); err != nil {
		t.Fatalf("ed.do(ch)=%v, want nil", err)
	}
	rs, err := ed.Print(All)
	if s := string(rs); s != str || err != nil {
		t.Errorf("ed.Print(All)=%q,%v, want %q,nil\n", s, err, str)
	}
}

func TestMark(t *testing.T) {
	tests := []struct {
		init string
		addr Address
		mark rune
		want addr
	}{
		{init: "Hello, 世界!", addr: All, mark: 'a', want: addr{0, 10}},
		{init: "Hello, 世界!", addr: Regexp("/Hello"), mark: 'a', want: addr{0, 5}},
		{init: "Hello, 世界!", addr: Line(0), mark: 'z', want: addr{0, 0}},
		{init: "Hello, 世界!", addr: End, mark: 'm', want: addr{10, 10}},
	}

	for _, test := range tests {
		ed := NewEditor(NewBuffer())
		defer ed.Close()
		if err := ed.Append(Line(0), []rune(test.init)); err != nil {
			t.Fatalf("ed.Append(0, %q)=%v, want nil", test.init, err)
		}
		if err := ed.Mark(test.addr, test.mark); err != nil {
			t.Errorf("ed.Mark(%q, %q)=%v, want nil", test.addr, test.mark, err)
		}
		if r := ed.marks[test.mark]; r != test.want {
			t.Errorf("ed.marks[%q]=%v, want %v", test.mark, r, test.want)
		}
	}
}

func TestWhere(t *testing.T) {
	tests := []whereTest{
		{init: "Hello, 世界!", addr: All, from: 0, to: 10},
		{init: "Hello, 世界!", addr: End, from: 10, to: 10},
		{init: "Hello, 世界!", addr: Line(0), from: 0, to: 0},
		{init: "Hello, 世界!", addr: Line(1), from: 0, to: 10},
		{init: "Hello, 世界!", addr: Regexp("/Hello"), from: 0, to: 5},
		{init: "Hello, 世界!", addr: Regexp("/世界"), from: 7, to: 9},
	}
	for _, test := range tests {
		test.run(t)
	}
}

type whereTest struct {
	init     string
	addr     Address
	from, to int64
}

func (test *whereTest) run(t *testing.T) {
	ed := NewEditor(NewBuffer())
	defer ed.Close()

	if err := ed.Append(All, []rune(test.init)); err != nil {
		t.Fatalf("ed.Append(All, %q)=%v, want nil", test.init, err)
	}
	from, to, err := ed.Where(test.addr)
	if from != test.from || to != test.to || err != nil {
		t.Errorf("ed.Where(%q)=%d,%d,%v, want %d,%d,nil",
			test.addr, from, to, err, test.from, test.to)
	}
}

func TestAppend(t *testing.T) {
	const str = "Hello, 世界!"
	tests := []changeTest{
		{init: str, addr: "#0", add: "XYZ", want: "XYZHello, 世界!", dot: addr{0, 3}},
		{init: str, addr: "$", add: "XYZ", want: "Hello, 世界!XYZ", dot: addr{10, 13}},
		{init: str, addr: "#1", add: "XYZ", want: "HXYZello, 世界!", dot: addr{1, 4}},
		{init: str, addr: "#0,#1", add: "XYZ", want: "HXYZello, 世界!", dot: addr{1, 4}},
	}
	for _, test := range tests {
		test.run((*Editor).Append, "Append", t)
	}
}

func TestInsert(t *testing.T) {
	const str = "Hello, 世界!"
	tests := []changeTest{
		{init: str, addr: "#0", add: "XYZ", want: "XYZHello, 世界!", dot: addr{0, 3}},
		{init: str, addr: "$", add: "XYZ", want: "Hello, 世界!XYZ", dot: addr{10, 13}},
		{init: str, addr: "#1", add: "XYZ", want: "HXYZello, 世界!", dot: addr{1, 4}},
		{init: str, addr: "#0,#1", add: "XYZ", want: "XYZHello, 世界!", dot: addr{0, 3}},
	}
	for _, test := range tests {
		test.run((*Editor).Insert, "Insert", t)
	}
}

func TestChange(t *testing.T) {
	const str = "Hello, 世界!"
	tests := []changeTest{
		{init: str, addr: "#0", add: "XYZ", want: "XYZHello, 世界!", dot: addr{0, 3}},
		{init: str, addr: "$", add: "XYZ", want: "Hello, 世界!XYZ", dot: addr{10, 13}},
		{init: str, addr: "#1", add: "XYZ", want: "HXYZello, 世界!", dot: addr{1, 4}},
		{init: str, addr: "#0,#1", add: "XYZ", want: "XYZello, 世界!", dot: addr{0, 3}},
		{init: str, addr: "#1,$-#1", add: "XYZ", want: "HXYZ!", dot: addr{1, 4}},
	}
	for _, test := range tests {
		test.run((*Editor).Change, "Change", t)
	}
}

type changeTest struct {
	init, want, addr, add string
	dot                   addr
}

func (test changeTest) run(f func(*Editor, Address, []rune) error, name string, t *testing.T) {
	ed := NewEditor(NewBuffer())
	defer ed.Close()
	defer ed.buf.Close()
	if err := ed.Append(Rune(0), []rune(test.init)); err != nil {
		t.Fatalf("%v, init failed", test)
	}
	addr, _, err := Addr([]rune(test.addr))
	if err != nil {
		t.Fatalf("%#v: bad address: %s", test, test.addr)
		return
	}
	if err := f(ed, addr, []rune(test.add)); err != nil {
		t.Fatalf("%#v: %s(%v, %v)=%v, want nil", test, name, test.addr, test.add, err)
	}
	if ed.marks['.'] != test.dot {
		t.Errorf("%#v: dot=%v, want %v\n", test, ed.marks['.'], test.dot)
	}
	rs, err := ed.Print(All)
	if s := string(rs); s != test.want || err != nil {
		t.Errorf("%#v: string=%q, want %q\n", test, s, test.want)
	}
}

func TestCopy(t *testing.T) {
	tests := []copyMoveTest{
		{init: "abc", src: "/abc/", dst: "$", want: "abcabc", dot: addr{3, 6}},
		{init: "abc", src: "/abc/", dst: "0", want: "abcabc", dot: addr{0, 3}},
		{init: "abc", src: "/abc/", dst: "#1", want: "aabcbc", dot: addr{1, 4}},
		{init: "abcdef", src: "/abc/", dst: "#4", want: "abcdabcef", dot: addr{4, 7}},
		{
			init: "abc\ndef\nghi",
			src:  "/def/", dst: "1",
			want: "abc\ndefdef\nghi",
			dot:  addr{4, 7},
		},
	}
	for _, test := range tests {
		test.run((*Editor).Copy, "Copy", t)
	}
}

func TestMove(t *testing.T) {
	tests := []copyMoveTest{
		{init: "abc", src: "/abc/", dst: "#0", want: "abc", dot: addr{0, 3}},
		{init: "abc", src: "/abc/", dst: "#1", err: "overlap"},
		{init: "abc", src: "/abc/", dst: "#2", err: "overlap"},
		{init: "abc", src: "/abc/", dst: "#3", want: "abc", dot: addr{0, 3}},
		{
			init: "abcdef",
			src:  "/abc/", dst: "$",
			want: "defabc",
			dot:  addr{3, 6},
		},
		{
			init: "abcdef",
			src:  "/def/", dst: "0",
			want: "defabc",
			dot:  addr{0, 3},
		},
		{
			init: "abc\ndef\nghi",
			src:  "/def/", dst: "3",
			want: "abc\n\nghidef",
			dot:  addr{8, 11},
		},
	}
	for _, test := range tests {
		test.run((*Editor).Move, "Move", t)
	}
}

type copyMoveTest struct {
	init, want, src, dst, err string
	dot                       addr
}

func (test copyMoveTest) run(f func(ed *Editor, src, dst Address) error, name string, t *testing.T) {
	src, _, err := Addr([]rune(test.src))
	if err != nil {
		t.Fatalf("%#v: bad source address: %s", test, test.src)
		return
	}
	dst, _, err := Addr([]rune(test.dst))
	if err != nil {
		t.Fatalf("%#v: bad destination address: %s", test, test.dst)
		return
	}
	ed := NewEditor(NewBuffer())
	defer ed.Close()
	defer ed.buf.Close()
	if err := ed.Append(Rune(0), []rune(test.init)); err != nil {
		t.Fatalf("%v, init failed", test)
	}
	if err = f(ed, src, dst); !errMatch(test.err, err) {
		t.Errorf("ed.%s(%q, %q)=%v, want %q", name, test.src, test.dst, err, test.err)
	}
	if err != nil {
		return
	}
	if ed.marks['.'] != test.dot {
		t.Errorf("%#v: dot=%v, want %v\n", test, ed.marks['.'], test.dot)
	}
	rs, err := ed.Print(All)
	if s := string(rs); s != test.want || err != nil {
		t.Errorf("%#v: string=%q, want %q\n", test, s, test.want)
	}
}

func TestSubstitute(t *testing.T) {
	tests := []subTest{
		{
			init: "Hello, 世界!",
			addr: ",", re: ".*", sub: ``, g: true,
			want: "", dot: addr{0, 0},
		},
		{
			init: "Hello, 世界!",
			addr: ",", re: "世界", sub: `World`,
			want: "Hello, World!", dot: addr{0, 13},
		},
		{
			init: "Hello, 世界!",
			addr: ",", re: "(.)", sub: `\1-`, g: true,
			want: "H-e-l-l-o-,- -世-界-!-", dot: addr{0, 20},
		},
		{
			init: "abcabc",
			addr: ",", re: "abc", sub: "defg",
			want: "defgabc", dot: addr{0, 7},
		},
		{
			init: "abcabcabc",
			addr: ",", re: "abc", sub: "defg", g: true,
			want: "defgdefgdefg", dot: addr{0, 12},
		},
		{
			init: "abcabcabc",
			addr: "/abcabc/", re: "abc", sub: "defg", g: true,
			want: "defgdefgabc", dot: addr{0, 8},
		},
		{
			init: "abc abc",
			addr: ",", re: "abc", sub: "defg",
			want: "defg abc", dot: addr{0, 8},
		},
		{
			init: "abc abc",
			addr: ",", re: "abc", sub: "defg", g: true,
			want: "defg defg", dot: addr{0, 9},
		},
		{
			init: "abc abc abc",
			addr: "/abc abc/", re: "abc", sub: "defg", g: true,
			want: "defg defg abc", dot: addr{0, 9},
		},
		{
			init: "abcabc",
			addr: ",", re: "abc", sub: "de",
			want: "deabc", dot: addr{0, 5},
		},
		{
			init: "abcabcabc",
			addr: ",", re: "abc", sub: "de", g: true,
			want: "dedede", dot: addr{0, 6},
		},
		{
			init: "abcabcabc",
			addr: "/abcabc/", re: "abc", sub: "de", g: true,
			want: "dedeabc", dot: addr{0, 4},
		},
		{
			init: "func f()",
			addr: ",", re: `func (.*)\(\)`, sub: `func (T) \1()`, g: true,
			want: "func (T) f()", dot: addr{0, 12},
		},
		{
			init: "abcdefghi",
			addr: ",", re: "(abc)(def)(ghi)", sub: `\0 \3 \2 \1`,
			want: "abcdefghi ghi def abc", dot: addr{0, 21},
		},
		{
			init: "abc",
			addr: ",", re: "abc", sub: `\1`,
			want: "", dot: addr{0, 0},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

type subTest struct {
	init, addr, re, sub, want string
	g                         bool
	dot                       addr
}

func (test subTest) run(t *testing.T) {
	ed := NewEditor(NewBuffer())
	defer ed.Close()
	defer ed.buf.Close()
	if err := ed.Append(Rune(0), []rune(test.init)); err != nil {
		t.Fatalf("%#v, init failed", test)
	}
	addr, _, err := Addr([]rune(test.addr))
	if err != nil {
		t.Fatalf("%#v: bad address: %q", test, test.addr)
	}
	re, err := re1.Compile([]rune(test.re), re1.Options{})
	if err != nil {
		t.Fatalf("%#v: bad re: %q %v", test, test.re, err)
	}
	if err := ed.Substitute(addr, re, []rune(test.sub), test.g); err != nil {
		t.Fatalf("%#v: ed.Substitute(%q, %q, %q, %v)=%v, want nil",
			test, test.addr, test.re, test.sub, test.g, err)
	}
	if ed.marks['.'] != test.dot {
		t.Errorf("%#v: dot=%v, want %v\n", test, ed.marks['.'], test.dot)
	}
	rs, err := ed.Print(All)
	if s := string(rs); s != test.want || err != nil {
		t.Errorf("%#v: string=%q, want %q\n", test, s, test.want)
	}
}

func TestEditorEdit(t *testing.T) {
	tests := []editTest{
		{{edit: "$a/Hello, World!/", want: "Hello, World!"}},
		{{edit: "$a\nHello, World!\n.", want: "Hello, World!\n"}},
		{{edit: `$a/Hello, World!\/`, want: "Hello, World!/"}},
		{{edit: `$a/Hello, World!\n`, want: "Hello, World!\n"}},
		{{edit: "$i/Hello, World!/", want: "Hello, World!"}},
		{{edit: "$i/Hello, World!", want: "Hello, World!"}},
		{{edit: "$i\nHello, World!\n.", want: "Hello, World!\n"}},
		{{edit: `$i/Hello, World!\/`, want: "Hello, World!/"}},
		{{edit: `$i/Hello, World!\n`, want: "Hello, World!\n"}},
		{
			{edit: "0a/Hello, World!", want: "Hello, World!"},
			{edit: ",c/Bye", want: "Bye"},
		},
		{
			{edit: "0a/Hello", want: "Hello"},
			{edit: "/ello/c/i", want: "Hi"},
		},
		{
			{edit: "0a/Hello, World!", want: "Hello, World!"},
			{edit: "?World?c/世界", want: "Hello, 世界!"},
		},
		{
			{edit: "0a/Hello, World!", want: "Hello, World!"},
			{edit: "/, World/d", want: "Hello!"},
		},
		{
			{edit: "0a/Hello, World!", want: "Hello, World!"},
			{edit: ",d", want: ""},
		},
		{
			{edit: "0a/Hello, World!", want: "Hello, World!"},
			{edit: "d", want: ""}, // Address defaults to dot.
		},
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: "/World/c/Test", want: "Hello, Test!"},
			{edit: "c/World", want: "Hello, World!"},
		},
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: "/World/", want: "Hello, World!"},
			{edit: "c/Test", want: "Hello, Test!"},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestConcurrentSimpleChanges(t *testing.T) {
	tests := []editTest{
		{
			{who: 0, edit: "0c/世界!", want: "世界!"},
			{who: 1, edit: "0c/Hello, ", want: "Hello, 世界!"},
			{who: 0, edit: ".d", want: "Hello, "},
			{who: 1, edit: ".d", want: ""},
		},
		{
			{who: 0, edit: "0c/世界!", want: "世界!"},
			{who: 1, edit: "0,#1c/Hello, ", want: "Hello, 界!"},
			{who: 0, edit: ".d", want: "Hello, "},
			{who: 1, edit: ".d", want: ""},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestEditorEditMark(t *testing.T) {
	tests := []editTest{
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: "/, World/k a", want: "Hello, World!"},
			{edit: "'a d", want: "Hello!"},
		},
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: "/, World/k", want: "Hello, World!"},
			{edit: "d", want: "Hello!"},
		},
		// Edit after the mark.
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: "/Hello/k a", want: "Hello, World!"},
			{edit: "/, World/d", want: "Hello!"},
			{edit: "'a d", want: "!"},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/k a", want: "abc123xyz"},
			{edit: "/xyz/d", want: "abc123"},
			{edit: "'a d", want: "abc"},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/k a", want: "abc123xyz"},
			{edit: "/z/d", want: "abc123xy"},
			{edit: "'a d", want: "abcxy"},
		},
		{
			{edit: "a/abc123", want: "abc123"},
			{edit: "#3k a", want: "abc123"},
			{edit: "$a/xyz", want: "abc123xyz"},
			{edit: "'a a/...", want: "abc...123xyz"},
		},

		// Edit before the mark.
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: "/World/k a", want: "Hello, World!"},
			{edit: "/Hello, /d", want: "World!"},
			{edit: "'a d", want: "!"},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/k a", want: "abc123xyz"},
			{edit: "/abc/d", want: "123xyz"},
			{edit: "'a d", want: "xyz"},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/k a", want: "abc123xyz"},
			{edit: "/a/d", want: "bc123xyz"},
			{edit: "'a d", want: "bcxyz"},
		},
		{
			{edit: "a/abc123", want: "abc123"},
			{edit: "#3k a", want: "abc123"},
			{edit: "#0a/xyz", want: "xyzabc123"},
			{edit: "'a a/...", want: "xyzabc...123"},
		},
		{
			{edit: "a/abc123", want: "abc123"},
			{edit: "#3k a", want: "abc123"},
			{edit: "#3a/xyz", want: "abcxyz123"},
			{edit: "'a a/...", want: "abcxyz...123"},
		},

		// Edit within the mark.
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: ",k a", want: "Hello, World!"},
			{edit: "/ /c/ Cruel /", want: "Hello, Cruel World!"},
			{edit: "'a d", want: ""},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/k a", want: "abc123xyz"},
			{edit: "/2/d", want: "abc13xyz"},
			{edit: "'a c/123", want: "abc123xyz"},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/k a", want: "abc123xyz"},
			{edit: "/2/a/2.5", want: "abc122.53xyz"},
			{edit: "'a c/123", want: "abc123xyz"},
		},

		// Edit over the mark.
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: "/World/k a", want: "Hello, World!"},
			{edit: ",c/abc", want: "abc"},
			{edit: "'a a/123", want: "abc123"},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/k a", want: "abc123xyz"},
			{edit: "/c123x/d", want: "abyz"},
			{edit: "'a c/123", want: "ab123yz"},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/k a", want: "abc123xyz"},
			{edit: "/c123x/c/C123X", want: "abC123Xyz"},
			{edit: "'a c/...", want: "abC123X...yz"},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/k a", want: "abc123xyz"},
			{edit: "/123xyz/d", want: "abc"},
			{edit: "'a c/...", want: "abc..."},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/k a", want: "abc123xyz"},
			{edit: "/abc123/d", want: "xyz"},
			{edit: "'a c/...", want: "...xyz"},
		},
		{
			{edit: "a/abc123", want: "abc123"},
			{edit: "#3k a", want: "abc123"},
			{edit: "/bc12/d", want: "a3"},
			{edit: "'a c/...", want: "a...3"},
		},

		// Edit over the beginning of the mark.
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: "/World/k a", want: "Hello, World!"},
			{edit: "/W/c/w", want: "Hello, world!"},
			{edit: "'a d", want: "Hello, w!"},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/k a", want: "abc123xyz"},
			{edit: "/bc1/d", want: "a23xyz"},
			{edit: "'a c/bc", want: "abcxyz"},
		},

		// Edit over the end of the mark.
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: "/World/k a", want: "Hello, World!"},
			{edit: "/d/c/D", want: "Hello, WorlD!"},
			{edit: "'a d", want: "Hello, !"},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/k a", want: "abc123xyz"},
			{edit: "/3xy/d", want: "abc12z"},
			{edit: "'a c/xy", want: "abcxyz"},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestEditorEditPrint(t *testing.T) {
	tests := []editTest{
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: "p", print: "Hello, World!", want: "Hello, World!"},
			{edit: "#1p", print: "", want: "Hello, World!"},
			{edit: "#0,#1p", print: "H", want: "Hello, World!"},
			{edit: "1p", print: "Hello, World!", want: "Hello, World!"},
			{edit: "/e.*/p", print: "ello, World!", want: "Hello, World!"},
			{edit: "0p", print: "", want: "Hello, World!"},
			{edit: "$p", print: "", want: "Hello, World!"},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestEditorEditWhere(t *testing.T) {
	tests := []editTest{
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: "=#", print: "#0,#13", want: "Hello, World!"},
			{edit: "/e.*/=#", print: "#1,#13", want: "Hello, World!"},
			{edit: "0=#", print: "#0,#0", want: "Hello, World!"},
			{edit: "$=#", print: "#13,#13", want: "Hello, World!"},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestEditorEditSubstitute(t *testing.T) {
	tests := []editTest{
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: ",s/World/世界", want: "Hello, 世界!"},
			{edit: "d", want: ""},
		},
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: "/World/s/World/世界", want: "Hello, 世界!"},
			{edit: "d", want: "Hello, !"},
		},
		{
			{edit: "a/abcabc", want: "abcabc"},
			{edit: ",s/abc/defg/", want: "defgabc"},
		},
		{
			{edit: "a/abcabcabc", want: "abcabcabc"},
			{edit: ",s/abc/defg/g", want: "defgdefgdefg"},
		},
		{
			{edit: "a/abcabcabc", want: "abcabcabc"},
			{edit: "/abcabc/s/abc/defg/g", want: "defgdefgabc"},
			{edit: "d", want: "abc"},
		},
		{
			{edit: "a/abc abc", want: "abc abc"},
			{edit: ",s/abc/defg/", want: "defg abc"},
		},
		{
			{edit: "a/abc abc abc", want: "abc abc abc"},
			{edit: ",s/abc/defg/g", want: "defg defg defg"},
		},
		{
			{edit: "a/abc abc abc", want: "abc abc abc"},
			{edit: "/abc abc/s/abc/defg/g", want: "defg defg abc"},
			{edit: "d", want: " abc"},
		},
		{
			{edit: "a/abcabc", want: "abcabc"},
			{edit: ",s/abc/de/", want: "deabc"},
		},
		{
			{edit: "a/abcabcabc", want: "abcabcabc"},
			{edit: ",s/abc/de/g", want: "dedede"},
		},
		{
			{edit: "a/abcabcabc", want: "abcabcabc"},
			{edit: "/abcabc/s/abc/de/g", want: "dedeabc"},
			{edit: "d", want: "abc"},
		},
		{
			{edit: "a/abcabcabc", want: "abcabcabc"},
			{edit: "s/abc//g", want: ""},
			{edit: "a/xyz", want: "xyz"},
		},
		{
			{edit: "a/func f()", want: "func f()"},
			{edit: `s/func (.*)\(\)/func (T)\1()`, want: "func (T)f()"},
			{edit: "d", want: ""},
		},
		{
			{edit: "a/abcdefghi", want: "abcdefghi"},
			{edit: `s/(abc)(def)(ghi)/\0 \3 \2 \1`, want: "abcdefghi ghi def abc"},
		},
		{
			{edit: "a/abc", want: "abc"},
			{edit: `s/abc/\1`, want: ""},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestEditorEditCopy(t *testing.T) {
	tests := []editTest{
		{
			{edit: "a/abcdef", want: "abcdef"},
			{edit: "/abc/t$", want: "abcdefabc"},
			{edit: "d", want: "abcdef"},
		},
		{
			{edit: "a/abcdef", want: "abcdef"},
			{edit: "/def/t0", want: "defabcdef"},
			{edit: "d", want: "abcdef"},
		},
		{
			{edit: "a/abcdef", want: "abcdef"},
			{edit: "/abc/t#4", want: "abcdabcef"},
			{edit: "d", want: "abcdef"},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestEditorEditMove(t *testing.T) {
	tests := []editTest{
		{
			{edit: "a/abcdef", want: "abcdef"},
			{edit: "/abc/m$", want: "defabc"},
			{edit: "d", want: "def"},
		},
		{
			{edit: "a/abcdef", want: "abcdef"},
			{edit: "/def/m0", want: "defabc"},
			{edit: "d", want: "abc"},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

// An editTest tests edits performed on a buffer,
// possibly by multiple editors.
// It checks that the buffer has the desired text after each edit.
type editTest []struct {
	who               int
	edit, print, want string
}

func (test editTest) nEditors() int {
	var n int
	for _, c := range test {
		if c.who > n {
			n = c.who
		}
	}
	return n + 1
}

func (test editTest) run(t *testing.T) {
	b := NewBuffer()
	defer b.Close()
	eds := make([]*Editor, test.nEditors())
	for i := range eds {
		eds[i] = NewEditor(b)
		defer eds[i].Close()
	}
	for i, c := range test {
		t.Logf("%q\n", c.edit)
		pr, err := eds[c.who].Edit([]rune(c.edit))
		if pr := string(pr); pr != c.print || err != nil {
			t.Errorf("%v, %d, Edit(%v)=%q,%v, want %q,nil", test, i,
				strconv.Quote(c.edit), pr, err, c.print)
			continue
		}
		rs, err := b.runes.Read(int(b.runes.Size()), 0)
		if err != nil {
			t.Errorf("%v, %d, read failed=%v\n", test, i, err)
			continue
		}
		if s := string(rs); s != c.want {
			t.Errorf("%v, %d: string=%v, want %v\n",
				test, i, strconv.Quote(s), strconv.Quote(c.want))
			continue
		}
	}
}

func errMatch(re string, err error) bool {
	if err == nil {
		return re == ""
	}
	if re == "" {
		return err == nil
	}
	return regexp.MustCompile(re).Match([]byte(err.Error()))
}
