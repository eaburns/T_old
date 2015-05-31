// Copyright © 2015, The T Authors.

package edit

import (
	"bytes"
	"regexp"
	"strconv"
	"testing"

	"github.com/eaburns/T/edit/runes"
	"github.com/eaburns/T/re1"
)

// String returns a string containing the entire editor contents.
func (ed *Editor) String() string {
	rs, err := runes.ReadAll(ed.buf.runes.Reader(0))
	if err != nil {
		panic(err)
	}
	return string(rs)
}

func (ed *Editor) change(a Address, s string) error {
	return ed.Do(Change(All, s), bytes.NewBuffer(nil))
}

func TestRetry(t *testing.T) {
	ed := NewEditor(NewBuffer())
	defer ed.Close()

	const str = "Hello, 世界!"
	ch := func() (addr, error) {
		if ed.buf.seq < 10 {
			// Simulate concurrent changes, necessitating retries.
			ed.buf.seq++
		}
		return change{a: All, op: 'c', str: str}.do(ed, nil)
	}
	if err := ed.do(ch); err != nil {
		t.Fatalf("ed.do(ch)=%v, want nil", err)
	}
	if s := ed.String(); s != str {
		t.Errorf("ed.String()=%q, want %q,nil\n", s, str)
	}
}

func TestWhere(t *testing.T) {
	tests := []whereTest{
		{init: "", addr: All, rfrom: 0, rto: 0, lfrom: 1, lto: 1},
		{init: "H\ne\nl\nl\no\n 世\n界\n!", addr: All, rfrom: 0, rto: 16, lfrom: 1, lto: 8},
		{init: "Hello\n 世界!", addr: All, rfrom: 0, rto: 10, lfrom: 1, lto: 2},
		{init: "Hello\n 世界!", addr: End, rfrom: 10, rto: 10, lfrom: 2, lto: 2},
		{init: "Hello\n 世界!", addr: Line(1), rfrom: 0, rto: 6, lfrom: 1, lto: 1},
		{init: "Hello\n 世界!", addr: Line(2), rfrom: 6, rto: 10, lfrom: 2, lto: 2},
		{init: "Hello\n 世界!", addr: Regexp("/Hello"), rfrom: 0, rto: 5, lfrom: 1, lto: 1},
		{init: "Hello\n 世界!", addr: Regexp("/世界"), rfrom: 7, rto: 9, lfrom: 2, lto: 2},
	}
	for _, test := range tests {
		test.run(t)
	}
}

type whereTest struct {
	init       string
	addr       Address
	rfrom, rto int64
	lfrom, lto int64
}

func (test *whereTest) run(t *testing.T) {
	ed := NewEditor(NewBuffer())
	defer ed.Close()

	if err := ed.change(All, test.init); err != nil {
		t.Fatalf("ed.Append(All, %q)=%v, want nil", test.init, err)
	}
	at, lfrom, lto, err := ed.Where(test.addr)
	if at.from != test.rfrom || at.to != test.rto || lfrom != test.lfrom || lto != test.lto || err != nil {
		t.Errorf("ed.Where(%q)=%v,%d,%d,%v, want %d,%d,%d,%d,nil",
			test.addr, at, lfrom, lto, err, test.rfrom, test.rto, test.lfrom, test.lto)
	}
}

func TestSubstituteFrom(t *testing.T) {
	tests := []subTest{
		{
			init: "Hello, 世界!",
			addr: ",", re: ".*", sub: ``, g: true, n: 1,
			want: "", dot: addr{0, 0},
		},
		{
			init: "Hello, 世界!",
			addr: ",", re: "世界", sub: `World`, n: 1,
			want: "Hello, World!", dot: addr{0, 13},
		},
		{
			init: "Hello, 世界!",
			addr: ",", re: "(.)", sub: `\1-`, g: true, n: 1,
			want: "H-e-l-l-o-,- -世-界-!-", dot: addr{0, 20},
		},
		{
			init: "abcabc",
			addr: ",", re: "abc", sub: "defg", n: 1,
			want: "defgabc", dot: addr{0, 7},
		},
		{
			init: "abcabcabc",
			addr: ",", re: "abc", sub: "defg", g: true, n: 1,
			want: "defgdefgdefg", dot: addr{0, 12},
		},
		{
			init: "abcabcabc",
			addr: "/abcabc/", re: "abc", sub: "defg", g: true, n: 1,
			want: "defgdefgabc", dot: addr{0, 8},
		},
		{
			init: "abc abc",
			addr: ",", re: "abc", sub: "defg", n: 1,
			want: "defg abc", dot: addr{0, 8},
		},
		{
			init: "abc abc",
			addr: ",", re: "abc", sub: "defg", g: true, n: 1,
			want: "defg defg", dot: addr{0, 9},
		},
		{
			init: "abc abc abc",
			addr: "/abc abc/", re: "abc", sub: "defg", g: true, n: 1,
			want: "defg defg abc", dot: addr{0, 9},
		},
		{
			init: "abcabc",
			addr: ",", re: "abc", sub: "de", n: 1,
			want: "deabc", dot: addr{0, 5},
		},
		{
			init: "abcabcabc",
			addr: ",", re: "abc", sub: "de", g: true, n: 1,
			want: "dedede", dot: addr{0, 6},
		},
		{
			init: "abcabcabc",
			addr: "/abcabc/", re: "abc", sub: "de", g: true, n: 1,
			want: "dedeabc", dot: addr{0, 4},
		},
		{
			init: "func f()",
			addr: ",", re: `func (.*)\(\)`, sub: `func (T) \1()`, g: true, n: 1,
			want: "func (T) f()", dot: addr{0, 12},
		},
		{
			init: "abcdefghi",
			addr: ",", re: "(abc)(def)(ghi)", sub: `\0 \3 \2 \1`, n: 1,
			want: "abcdefghi ghi def abc", dot: addr{0, 21},
		},
		{
			init: "abc",
			addr: ",", re: "abc", sub: `\1`, n: 1,
			want: "", dot: addr{0, 0},
		},
		{
			init: "abcabcabc",
			addr: ",", re: "abc", sub: "def", n: 2,
			want: "abcdefabc", dot: addr{0, 9},
		},
		{
			init: "abcabcabc",
			addr: ",", re: "abc", sub: "def", g: true, n: 2,
			want: "abcdefdef", dot: addr{0, 9},
		},
		{
			init: "abcabcabc",
			addr: ",", re: "abc", sub: "def", g: false, n: 0,
			want: "defabcabc", dot: addr{0, 9},
		},
		{
			init: "abcabcabc",
			addr: ",", re: "notpresent", sub: "def", g: false, n: 4,
			want: "abcabcabc", dot: addr{0, 9},
		},
		{
			init: "abcabcabc",
			addr: ",", re: "abc", sub: "def", g: false, n: 4,
			want: "abcabcabc", dot: addr{0, 9},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

type subTest struct {
	init, addr, re, sub, want string
	g                         bool
	n                         int
	dot                       addr
}

func (test subTest) run(t *testing.T) {
	ed := NewEditor(NewBuffer())
	defer ed.Close()
	defer ed.buf.Close()
	if err := ed.change(All, test.init); err != nil {
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
	if err := ed.SubstituteFrom(addr, re, []rune(test.sub), test.g, test.n); err != nil {
		t.Fatalf("%#v: ed.SubstituteFrom(%q, %q, %q, %v, %v)=%v, want nil",
			test, test.addr, test.re, test.sub, test.g, test.n, err)
	}
	if ed.marks['.'] != test.dot {
		t.Errorf("%#v: dot=%v, want %v\n", test, ed.marks['.'], test.dot)
	}
	if s := ed.String(); s != test.want {
		t.Errorf("ed.String()=%q, want %q,nil\n", s, test.want)
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
			{edit: "0=#", print: "#0", want: "Hello, World!"},
			{edit: "$=#", print: "#13", want: "Hello, World!"},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestEditorEditWhereLine(t *testing.T) {
	tests := []editTest{
		{
			{edit: "=", print: "1", want: ""},
			{edit: "a/Hello\n World!", want: "Hello\n World!"},
			{edit: "=", print: "1,2", want: "Hello\n World!"},
			{edit: "/e.*\n.*/=", print: "1,2", want: "Hello\n World!"},
			{edit: "0=", print: "1", want: "Hello\n World!"},
			{edit: "$=", print: "2", want: "Hello\n World!"},
			{edit: "/Hello/s/Hello/H\ne\nl\nl\no", want: "H\ne\nl\nl\no\n World!"},
			{edit: "=", print: "1,5", want: "H\ne\nl\nl\no\n World!"},
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
		{
			{edit: "a/abc abc abc", want: "abc abc abc"},
			{edit: "s0/abc/hello/", want: "hello abc abc"},
		},
		{
			{edit: "a/abc abc abc", want: "abc abc abc"},
			{edit: "s2/abc/hello/", want: "abc hello abc"},
		},
		{
			{edit: "a/abc abc abc", want: "abc abc abc"},
			{edit: "s2/abc/hello/g", want: "abc hello hello"},
		},
		{
			{edit: "a/abc abc abc", want: "abc abc abc"},
			{edit: "s0/abc/hello/g", want: "hello hello hello"},
		},
		{
			{edit: "a/abc abc abc", want: "abc abc abc"},
			{edit: "s2/notpresent/def/", want: "abc abc abc"},
		},
		{
			{edit: "a/abc abc abc", want: "abc abc abc"},
			{edit: "s4/abc/def/g", want: "abc abc abc"},
		},
		{
			{edit: "a/aaa aaa aaa aaa", want: "aaa aaa aaa aaa"},
			{edit: "s11/a/b/", want: "aaa aaa aaa aba"},
		},
		{
			{edit: "a/aaa aaa aaa aaa", want: "aaa aaa aaa aaa"},
			{edit: "s11/a/b/g", want: "aaa aaa aaa abb"},
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
		w := bytes.NewBuffer(nil)
		err := eds[c.who].Edit([]rune(c.edit), w)
		if pr := w.String(); pr != c.print || err != nil {
			t.Errorf("%v, %d, Edit(%v)=%q,%v, want %q,nil", test, i,
				strconv.Quote(c.edit), pr, err, c.print)
			continue
		}
		if s := eds[c.who].String(); s != c.want {
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
