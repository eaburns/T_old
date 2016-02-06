// Copyright © 2015, The T Authors.

package edit

import (
	"errors"
	"io"
	"io/ioutil"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/eaburns/T/edit/runes"
	"github.com/eaburns/T/re1"
)

func TestDotAddress(t *testing.T) {
	str := "Hello, 世界!"
	sz := int64(utf8.RuneCountInString(str))
	tests := []addressTest{
		{text: str, dot: pt(0), addr: Dot, want: pt(0)},
		{text: str, dot: pt(5), addr: Dot, want: pt(5)},
		{text: str, dot: rng(5, 6), addr: Dot, want: rng(5, 6)},
		{text: str, dot: pt(sz), addr: Dot, want: pt(sz)},
		{text: str, dot: rng(0, sz), addr: Dot, want: rng(0, sz)},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestMarkAddress(t *testing.T) {
	str := "Hello, 世界!"
	tests := []addressTest{
		{text: str, marks: map[rune]addr{}, addr: Mark('☺'), err: "bad mark"},

		{text: str, marks: map[rune]addr{'a': {0, 0}}, addr: Mark('a'), want: pt(0)},
		{text: str, marks: map[rune]addr{}, addr: Mark('a'), want: pt(0)},
		{text: str, marks: map[rune]addr{'z': {0, 0}}, addr: Mark('z'), want: pt(0)},
		{text: str, marks: map[rune]addr{'z': {1, 9}}, addr: Mark('z'), want: rng(1, 9)},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestEndAddress(t *testing.T) {
	tests := []addressTest{
		{text: "", addr: End, want: pt(0)},
		{text: "Hello, World!", addr: End, want: pt(13)},
		{text: "Hello, 世界!", addr: End, want: pt(10)},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestRuneAddress(t *testing.T) {
	str := "Hello, 世界!"
	sz := int64(utf8.RuneCountInString(str))
	tests := []addressTest{
		{text: str, addr: Rune(0), want: pt(0)},
		{text: str, addr: Rune(3), want: pt(3)},
		{text: str, addr: Rune(sz), want: pt(sz)},

		{text: str, dot: pt(sz), addr: Rune(0), want: pt(sz)},
		{text: str, dot: pt(sz), addr: Rune(-3), want: pt(sz - 3)},
		{text: str, dot: pt(sz), addr: Rune(-sz), want: pt(0)},

		{text: str, addr: Rune(10000), err: "out of range"},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestLineAddress(t *testing.T) {
	tests := []addressTest{
		{text: "", addr: Line(0), want: pt(0)},
		{text: "aa", addr: Line(0), want: pt(0)},
		{text: "aa\n", addr: Line(0), want: pt(0)},
		{text: "aa", addr: Line(1), want: rng(0, 2)},
		{text: "aa\n", addr: Line(1), want: rng(0, 3)},
		{text: "\n", addr: Line(1), want: rng(0, 1)},
		{text: "", addr: Line(1), want: pt(0)},
		{text: "aa\nbb", addr: Line(2), want: rng(3, 5)},
		{text: "aa\nbb\n", addr: Line(2), want: rng(3, 6)},
		{text: "aa\n", addr: Line(2), want: pt(3)},
		{text: "aa\nbb\ncc", addr: Line(3), want: rng(6, 8)},
		{text: "aa\nbb\ncc\n", addr: Line(3), want: rng(6, 9)},
		{text: "aa\nbb\n", addr: Line(3), want: pt(6)},

		{dot: pt(2), text: "aa", addr: Line(0), want: pt(2)},
		{dot: pt(3), text: "aa\n", addr: Line(0), want: pt(3)},

		{text: "", addr: Line(2), err: "out of range"},
		{text: "aa", addr: Line(2), err: "out of range"},
		{text: "aa\n", addr: Line(3), err: "out of range"},
		{text: "aa\nbb", addr: Line(3), err: "out of range"},
		{text: "aa\nbb", addr: Line(10), err: "out of range"},

		{text: "", addr: Line(-1), want: pt(0)},
		{dot: pt(2), text: "aa", addr: Line(0).reverse(), want: rng(0, 2)},
		{dot: pt(3), text: "aa\n", addr: Line(0).reverse(), want: pt(3)},
		{dot: pt(2), text: "aa", addr: Line(-1), want: pt(0)},
		{dot: pt(1), text: "aa", addr: Line(-1), want: pt(0)},
		{dot: pt(1), text: "abc\ndef", addr: Line(-1), want: pt(0)},
		{dot: pt(3), text: "aa\n", addr: Line(-1), want: rng(0, 3)},
		{dot: pt(1), text: "\n", addr: Line(-1), want: rng(0, 1)},
		{dot: pt(5), text: "aa\nbb", addr: Line(-2), want: pt(0)},
		{dot: pt(6), text: "aa\nbb\n", addr: Line(-2), want: rng(0, 3)},
		{dot: pt(3), text: "aa\n", addr: Line(-2), want: pt(0)},
		{dot: pt(8), text: "aa\nbb\ncc", addr: Line(-3), want: pt(0)},
		{dot: pt(9), text: "aa\nbb\ncc\n", addr: Line(-3), want: rng(0, 3)},
		{dot: pt(6), text: "aa\nbb\n", addr: Line(-3), want: pt(0)},

		{text: "", addr: Line(-2), err: "out of range"},
		{dot: pt(2), text: "aa", addr: Line(-2), err: "out of range"},
		{dot: pt(3), text: "aa\n", addr: Line(-3), err: "out of range"},
		{dot: pt(5), text: "aa\nbb", addr: Line(-3), err: "out of range"},
		{dot: pt(5), text: "aa\nbb", addr: Line(-10), err: "out of range"},

		{text: "abc\ndef", dot: pt(1), addr: Line(0), want: rng(1, 4)},
		{text: "abc\ndef", dot: pt(4), addr: Line(1), want: rng(4, 7)},
		{text: "abc\ndef", dot: pt(3), addr: Line(-1), want: pt(0)},
		{text: "abc\ndef", dot: pt(4), addr: Line(-1), want: rng(0, 4)},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestRegexpAddress(t *testing.T) {
	tests := []addressTest{
		{text: "Hello, 世界!", addr: Regexp(re1.Must("")), want: pt(0)},
		{text: "Hello, 世界!", addr: Regexp(re1.Must("H")), want: rng(0, 1)},
		{text: "Hello, 世界!", addr: Regexp(re1.Must(".")), want: rng(0, 1)},
		{text: "Hello, 世界!", addr: Regexp(re1.Must("世界")), want: rng(7, 9)},
		{text: "Hello, 世界!", addr: Regexp(re1.Must("[^!]+")), want: rng(0, 9)},

		{text: "Hello, 世界!", dot: pt(10), addr: Regexp(re1.Must("?")), want: pt(10)},
		{text: "Hello, 世界!", dot: pt(10), addr: Regexp(re1.Must("?!")), want: rng(9, 10)},
		{text: "Hello, 世界!", dot: pt(10), addr: Regexp(re1.Must("?.")), want: rng(9, 10)},
		{text: "Hello, 世界!", dot: pt(10), addr: Regexp(re1.Must("?H")), want: rng(0, 1)},
		{text: "Hello, 世界!", dot: pt(10), addr: Regexp(re1.Must("?[^!]+")), want: rng(0, 9)},

		{text: "Hello, 世界!", dot: pt(10), addr: Regexp(re1.Must("H")).reverse(), want: rng(0, 1)},
		{text: "Hello, 世界!", addr: Regexp(re1.Must("?H")).reverse(), want: rng(0, 1)},

		// Wrap.
		{text: "Hello, 世界!", addr: Regexp(re1.Must("?世界")), want: rng(7, 9)},
		{text: "Hello, 世界!", dot: pt(8), addr: Regexp(re1.Must("世界")), want: rng(7, 9)},

		{text: "Hello, 世界!", addr: Regexp(re1.Must("☺")), err: "no match"},
		{text: "Hello, 世界!", addr: Regexp(re1.Must("?☺")), err: "no match"},
	}
	for _, test := range tests {
		test.run(t)
	}
}

// Tests regexp String().
func TestRegexpString(t *testing.T) {
	tests := []struct {
		re, want string
	}{
		{``, `//`},
		{`/`, `/\//`},
		{`☺`, `/☺/`},
		{`//`, `/\/\//`},
		{`\/`, `/\//`},
		{`abc\/`, `/abc\//`},
		{`?`, `??`},
		{`?abc`, `?abc?`},
	}
	for _, test := range tests {
		re := Regexp(re1.Must(test.re))
		if s := re.String(); s != test.want {
			t.Errorf("Regexp(re1.Must(%q)).String()=%q, want %q", test.re, s, test.want)
		}
	}
}

func TestPlusAddress(t *testing.T) {
	tests := []addressTest{
		{text: "abc", addr: Line(0).Plus(Rune(3)), want: pt(3)},
		{text: "abc", addr: Rune(2).Plus(Rune(1)), want: pt(3)},
		{text: "abc", addr: Rune(2).Plus(Rune(-1)), want: pt(1)},
		{text: "abc\ndef", addr: Line(0).Plus(Line(1)), want: rng(0, 4)},
		{text: "abc\ndef", addr: Line(1).Plus(Line(1)), want: rng(4, 7)},
		{text: "abc\ndef", addr: Line(0).Plus(Line(-1)), want: pt(0)},
		{text: "abc\ndef", addr: Line(1).Plus(Line(-1)), want: rng(0, 4)},
		{text: "abc\ndef", addr: Rune(1).Plus(Line(0)), want: rng(1, 4)},

		{text: "abc\ndef", dot: pt(1), addr: Dot.Plus(Line(-1)), want: pt(0)},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestMinusAddress(t *testing.T) {
	tests := []addressTest{
		{text: "abc", addr: Line(0).Minus(Rune(0)), want: pt(0)},
		{text: "abc", addr: Rune(2).Minus(Rune(1)), want: pt(1)},
		{text: "abc", addr: Rune(2).Minus(Rune(-1)), want: pt(3)},
		{text: "abc\ndef", addr: Line(1).Minus(Line(1)), want: pt(0)},
		{text: "abc\ndef", dot: rng(1, 6), addr: Dot.Minus(Line(1)).Plus(Line(1)), want: rng(0, 4)},
		{text: "abc", dot: pt(3), addr: Dot.Minus(Regexp(re1.Must("aa?"))), want: rng(0, 1)},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestToAddress(t *testing.T) {
	tests := []addressTest{
		{text: "abc", addr: Line(0).To(End), want: rng(0, 3)},
		{text: "abc", dot: pt(1), addr: Dot.To(End), want: rng(1, 3)},
		{text: "abc\ndef", addr: Line(0).To(Line(1)), want: rng(0, 4)},
		{text: "abc\ndef", addr: Line(1).To(Line(2)), want: rng(0, 7)},
		{
			text: "abcabc",
			addr: Regexp(re1.Must("abc")).To(Regexp(re1.Must("b"))),
			want: rng(0, 2),
		},
		{
			text: "abc\ndef\nghi\njkl",
			dot:  pt(11),
			addr: Regexp(re1.Must("?abc")).Plus(Line(1)).To(Dot),
			want: rng(4, 11),
		},
		{text: "abc\ndef", addr: Line(0).To(Line(1)).To(Line(2)), want: rng(0, 7)},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestThenAddress(t *testing.T) {
	tests := []addressTest{
		{text: "abcabc", addr: Regexp(re1.Must("abc")).Then(Regexp(re1.Must("b"))), want: rng(0, 5)},
		{text: "abcabc", addr: Regexp(re1.Must("abc")).Then(Dot.Plus(Rune(1))), want: rng(0, 4)},
		{text: "abcabc", addr: Line(0).Plus(Rune(1)).Then(Dot.Plus(Rune(1))), want: rng(1, 2)},
		{text: "abcabc", addr: Line(0).To(Rune(1)).Then(Dot.Plus(Rune(1))), want: rng(0, 2)},
	}
	for _, test := range tests {
		test.run(t)
	}
}

type addressTest struct {
	text string
	// If rev==false, the match starts from 0.
	// If rev==true, the match starts from len(text).
	dot   addr
	marks map[rune]addr
	addr  Address
	want  addr
	err   string // regexp matching the error string
}

func (test addressTest) run(t *testing.T) {
	ed := NewEditor(NewBuffer())
	defer ed.buf.Close()
	if err := ed.buf.runes.Insert([]rune(test.text), 0); err != nil {
		t.Fatalf(`Put("%s")=%v, want nil`, test.text, err)
	}
	if test.marks != nil {
		ed.marks = test.marks
	}
	ed.marks['.'] = test.dot // Reset dot to the test dot.
	a, err := test.addr.whereFrom(test.dot.to, ed)
	var errStr string
	if err != nil {
		errStr = err.Error()
	}
	if a != test.want ||
		(test.err == "" && errStr != "") ||
		(test.err != "" && !regexp.MustCompile(test.err).MatchString(errStr)) {
		t.Errorf(`Address(%q).range(%d, %q)=%v, %v, want %v, %v`,
			test.addr.String(), test.dot, test.text, a, err,
			test.want, test.err)
	}
}

func pt(p int64) addr         { return addr{from: p, to: p} }
func rng(from, to int64) addr { return addr{from: from, to: to} }

func TestAddr(t *testing.T) {
	tests := []struct {
		a, left string
		want    Address
		err     string
	}{
		{a: "", want: nil},
		{a: " ", want: nil},
		{a: "u", want: nil, left: "u"},
		{a: " u", want: nil, left: "u"},
		{a: "r", want: nil, left: "r"},
		{a: " r", want: nil, left: "r"},
		{a: "\t\t", want: nil},
		{a: "\t\n\txyz", left: "\txyz", want: nil},
		{a: "\n#1", left: "#1", want: nil},

		{a: "#0", want: Rune(0)},
		{a: "#1", want: Rune(1)},
		{a: "#", want: Rune(1)},
		{a: "#12345", want: Rune(12345)},
		{a: "#12345xyz", left: "xyz", want: Rune(12345)},
		{a: " #12345xyz", left: "xyz", want: Rune(12345)},
		{a: " #1\t\n\txyz", left: "\txyz", want: Rune(1)},
		{a: "#" + strconv.FormatInt(math.MaxInt64, 10) + "0", err: "out of range"},

		{a: "0", want: Line(0)},
		{a: "1", want: Line(1)},
		{a: "12345", want: Line(12345)},
		{a: "12345xyz", left: "xyz", want: Line(12345)},
		{a: " 12345xyz", left: "xyz", want: Line(12345)},
		{a: " 1\t\n\txyz", left: "\txyz", want: Line(1)},
		{a: strconv.FormatInt(math.MaxInt64, 10) + "0", err: "out of range"},

		{a: "/", want: Regexp(re1.Must(""))},
		{a: "//", want: Regexp(re1.Must(""))},
		{a: "?", want: Regexp(re1.Must("?"))},
		{a: "/abcdef", want: Regexp(re1.Must("abcdef"))},
		{a: "/abc/def", left: "def", want: Regexp(re1.Must("abc"))},
		{a: "/abc def", want: Regexp(re1.Must("abc def"))},
		{a: "/abc def\nxyz", left: "xyz", want: Regexp(re1.Must("abc def"))},
		{a: "?abcdef", want: Regexp(re1.Must("?abcdef"))},
		{a: "?abc?def", left: "def", want: Regexp(re1.Must("?abc"))},
		{a: "?abc def", want: Regexp(re1.Must("?abc def"))},
		{a: " ?abc def", want: Regexp(re1.Must("?abc def"))},
		{a: "?abc def\nxyz", left: "xyz", want: Regexp(re1.Must("?abc def"))},
		{a: "/()", err: "operand"},

		{a: "$", want: End},
		{a: " $", want: End},
		{a: " $\t", want: End},

		{a: ".", want: Dot},
		{a: " .", want: Dot},
		{a: " .\t", want: Dot},

		{a: "'m", want: Mark('m')},
		{a: " 'z", want: Mark('z')},
		{a: " ' a", want: Mark('a')},
		{a: " ' a\t", want: Mark('a')},
		{a: "'\na", err: "bad mark"},
		{a: "'☺", err: "bad mark"},
		{a: "' ☺", err: "bad mark"},
		{a: "'", err: "bad mark"},

		{a: "+", want: Dot.Plus(Line(1))},
		{a: "+\n2", left: "2", want: Dot.Plus(Line(1))},
		{a: "+xyz", left: "xyz", want: Dot.Plus(Line(1))},
		{a: "+5", want: Dot.Plus(Line(5))},
		{a: "5+", want: Line(5).Plus(Line(1))},
		{a: "5+6", want: Line(5).Plus(Line(6))},
		{a: " 5 + 6", want: Line(5).Plus(Line(6))},
		{a: "-", want: Dot.Minus(Line(1))},
		{a: "-xyz", left: "xyz", want: Dot.Minus(Line(1))},
		{a: "-5", want: Dot.Minus(Line(5))},
		{a: "5-", want: Line(5).Minus(Line(1))},
		{a: "5-6", want: Line(5).Minus(Line(6))},
		{a: " 5 - 6", want: Line(5).Minus(Line(6))},
		{a: ".+#5", want: Dot.Plus(Rune(5))},
		{a: "$-#5", want: End.Minus(Rune(5))},
		{a: "$ - #5 + #3", want: End.Minus(Rune(5)).Plus(Rune(3))},
		{a: "+-", want: Dot.Plus(Line(1)).Minus(Line(1))},
		{a: " + - ", want: Dot.Plus(Line(1)).Minus(Line(1))},
		{a: " - + ", want: Dot.Minus(Line(1)).Plus(Line(1))},
		{a: "/abc/+++---", want: Regexp(re1.Must("abc")).Plus(Line(1)).Plus(Line(1)).Plus(Line(1)).Minus(Line(1)).Minus(Line(1)).Minus(Line(1))},

		{a: ".+/aa?/", want: Dot.Plus(Regexp(re1.Must("aa?")))},
		{a: ".-/aa?/", want: Dot.Minus(Regexp(re1.Must("aa?")))},

		{a: ",", want: Line(0).To(End)},
		{a: ",xyz", left: "xyz", want: Line(0).To(End)},
		{a: " , ", want: Line(0).To(End)},
		{a: ",\n1", left: "1", want: Line(0).To(End)},
		{a: ",1", want: Line(0).To(Line(1))},
		{a: "1,", want: Line(1).To(End)},
		{a: "0,$", want: Line(0).To(End)},
		{a: ".,$", want: Dot.To(End)},
		{a: "1,2", want: Line(1).To(Line(2))},
		{a: " 1 , 2 ", want: Line(1).To(Line(2))},
		{a: ",-#5", want: Line(0).To(Dot.Minus(Rune(5)))},
		{a: " , - #5", want: Line(0).To(Dot.Minus(Rune(5)))},
		{a: ";", want: Line(0).Then(End)},
		{a: ";xyz", left: "xyz", want: Line(0).Then(End)},
		{a: " ; ", want: Line(0).Then(End)},
		{a: " ;\n1", left: "1", want: Line(0).Then(End)},
		{a: ";1", want: Line(0).Then(Line(1))},
		{a: "1;", want: Line(1).Then(End)},
		{a: "0;$", want: Line(0).Then(End)},
		{a: ".;$", want: Dot.Then(End)},
		{a: "1;2", want: Line(1).Then(Line(2))},
		{a: " 1 ; 2 ", want: Line(1).Then(Line(2))},
		{a: ";-#5", want: Line(0).Then(Dot.Minus(Rune(5)))},
		{a: " ; - #5 ", want: Line(0).Then(Dot.Minus(Rune(5)))},
		{a: ";,", want: Line(0).Then(Line(0).To(End))},
		{a: " ; , ", want: Line(0).Then(Line(0).To(End))},

		// Implicit +.
		{a: "1#2", want: Line(1).Plus(Rune(2))},
		{a: "#2 1", want: Rune(2).Plus(Line(1))},
		{a: "1/abc", want: Line(1).Plus(Regexp(re1.Must("abc")))},
		{a: "/abc/1", want: Regexp(re1.Must("abc")).Plus(Line(1))},
		{a: "?abc?1", want: Regexp(re1.Must("?abc?")).Plus(Line(1))},
		{a: "$?abc", want: End.Plus(Regexp(re1.Must("?abc")))},
	}
	for _, test := range tests {
		rs := strings.NewReader(test.a)
		a, err := Addr(rs)
		if test.err != "" {
			if !regexp.MustCompile(test.err).MatchString(err.Error()) {
				t.Errorf(`Addr(%q)=%q,%v, want %q,%q`,
					test.a, a, err, test.want, test.err)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(a, test.want) {
			t.Errorf(`Addr(%q)=%q,%v, want %q,%q`, test.a, a, err, test.want, test.err)
			continue
		}
		left, err := ioutil.ReadAll(rs)
		if err != nil {
			t.Fatal(err)
		}
		if string(left) != test.left {
			t.Errorf(`Addr(%q) leftover %q want %q`, test.a, string(left), test.left)
			continue
		}
	}
}

func TestUpdate(t *testing.T) {
	tests := []struct {
		a, b, want addr
		n          int64
	}{
		// b after a
		{a: addr{10, 20}, b: addr{25, 30}, n: 0, want: addr{10, 20}},
		{a: addr{10, 20}, b: addr{25, 30}, n: 100, want: addr{10, 20}},
		{a: addr{10, 20}, b: addr{20, 30}, n: 0, want: addr{10, 20}},
		{a: addr{10, 20}, b: addr{20, 30}, n: 100, want: addr{10, 20}},

		// b before a
		{a: addr{10, 20}, b: addr{0, 0}, n: 0, want: addr{10, 20}},
		{a: addr{10, 20}, b: addr{0, 0}, n: 100, want: addr{110, 120}},
		{a: addr{10, 20}, b: addr{0, 10}, n: 0, want: addr{0, 10}},
		{a: addr{10, 20}, b: addr{0, 10}, n: 100, want: addr{100, 110}},

		// b over the beginning of a
		{a: addr{10, 20}, b: addr{0, 15}, n: 0, want: addr{0, 5}},
		{a: addr{10, 20}, b: addr{0, 15}, n: 10, want: addr{10, 15}},
		{a: addr{0, 3}, b: addr{0, 1}, n: 7, want: addr{7, 9}},

		// b over the end of a
		{a: addr{10, 20}, b: addr{15, 21}, n: 0, want: addr{10, 15}},
		{a: addr{10, 20}, b: addr{15, 21}, n: 10, want: addr{10, 15}},

		// b within a
		{a: addr{10, 20}, b: addr{12, 18}, n: 0, want: addr{10, 14}},
		{a: addr{10, 20}, b: addr{12, 18}, n: 1, want: addr{10, 15}},
		{a: addr{10, 20}, b: addr{12, 18}, n: 100, want: addr{10, 114}},
		{a: addr{10, 20}, b: addr{15, 20}, n: 0, want: addr{10, 15}},
		{a: addr{10, 20}, b: addr{15, 20}, n: 10, want: addr{10, 25}},
		{a: addr{0, 19}, b: addr{18, 19}, n: 2, want: addr{0, 20}},

		// b over all of a
		{a: addr{10, 20}, b: addr{10, 20}, n: 0, want: addr{10, 10}},
		{a: addr{10, 20}, b: addr{0, 40}, n: 0, want: addr{0, 0}},
		{a: addr{10, 20}, b: addr{0, 40}, n: 100, want: addr{100, 100}},
	}
	for _, test := range tests {
		a := test.a.update(test.b, test.n)
		if a != test.want {
			t.Errorf("%v.update(%v, %d)=%v, want %v",
				test.a, test.b, test.n, a, test.want)
			continue
		}
	}
}

// TestAddressString tests that well-formed addresses
// have valid and parsable address Strings()s.
func TestAddressString(t *testing.T) {
	tests := []struct {
		addr, want Address // If want==nil, want is set to addr.
	}{
		{addr: Dot},
		{addr: End},
		{addr: All},
		{addr: Rune(0)},
		{addr: Rune(100)},
		// Rune(-100) is the string -#100, when parsed, the implicit . is inserted: .-#100.
		{addr: Rune(-100), want: Dot.Minus(Rune(100))},
		{addr: Line(0)},
		{addr: Line(100)},
		// Line(-100) is the string -100, when parsed, the implicit . is inserted: .-100.
		{addr: Line(-100), want: Dot.Minus(Line(100))},
		{addr: Mark('a')},
		{addr: Mark('z')},
		{addr: Regexp(re1.Must("☺☹"))},
		{addr: Regexp(re1.Must("?☺☹"))},
		{addr: Dot.Plus(Line(1))},
		{addr: Dot.Minus(Line(1))},
		{addr: Dot.Minus(Line(1)).Plus(Line(1))},
		{addr: Rune(1).To(Rune(2))},
		{addr: Rune(1).Then(Rune(2))},
		{addr: Regexp(re1.Must("func")).Plus(Regexp(re1.Must(`\(`)))},
	}
	for _, test := range tests {
		if test.want == nil {
			test.want = test.addr
		}
		str := test.addr.String()
		got, err := Addr(strings.NewReader(str))
		if err != nil || !reflect.DeepEqual(got, test.want) {
			t.Errorf("Addr(%q)=%v,%v want %q,nil", str, got, err, test.want.String())
			continue
		}
	}
}

type errReaderAt struct{ error }

func (e *errReaderAt) ReadAt([]byte, int64) (int, error)      { return 0, e.error }
func (e *errReaderAt) WriteAt(b []byte, _ int64) (int, error) { return len(b), nil }

// TestIOErrors tests IO errors when computing addresses.
func TestIOErrors(t *testing.T) {
	helloWorld := []rune("Hello,\nWorld!")
	tests := []string{
		"1",
		"#1+1",
		"$-1",
		"#3-1",
		"/World",
		"?World",
		".+/World",
		".-/World",
		"0,/World",
		"0;/World",
		"/Hello/+",
		"/Hello/-",
		"/Hello/,",
		"/Hello/;",
		"/Hello/,/World",
		"/Hello/;/World",
		"/Hello/+/World",
		"/Hello/-/World",
		"/Hello/,/World",
		"/Hello/;/World",
	}
	for _, test := range tests {
		rs := strings.NewReader(test)
		addr, err := Addr(rs)
		if err != nil {
			t.Errorf("Addr(%q)=%q,%v want _,nil", test, addr, err)
			continue
		}
		switch l, err := ioutil.ReadAll(rs); {
		case err != nil && err != io.EOF:
			t.Fatal(err)
		case len(l) > 0:
			t.Errorf("Addr(%q) leftover %q want []", test, string(l))
			continue
		}
		f := &errReaderAt{nil}
		r := runes.NewBufferReaderWriterAt(1, f)
		ed := NewEditor(newBuffer(r))
		defer ed.buf.Close()

		if err := ed.buf.runes.Insert(helloWorld, 0); err != nil {
			t.Fatalf("ed.buf.runes.Insert(%v, 0)=%v, want nil", strconv.Quote(string(helloWorld)), err)
		}

		// All subsequent reads will be errors.
		f.error = errors.New("read error")
		if a, err := addr.where(ed); err != f.error {
			t.Errorf("Addr(%q).addr()=%v,%v, want addr{},%q", test, a, err, f.error)
			continue
		}
	}
}
