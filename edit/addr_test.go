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

	"github.com/eaburns/T/edit/runes"
)

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
		{a: "\t\n\txyz", left: "\n\txyz", want: nil},
		{a: "\n#1", left: "\n#1", want: nil},

		{a: "#0", want: Rune(0)},
		{a: "#1", want: Rune(1)},
		{a: "#", want: Rune(1)},
		{a: "#12345", want: Rune(12345)},
		{a: "#12345xyz", left: "xyz", want: Rune(12345)},
		{a: " #12345xyz", left: "xyz", want: Rune(12345)},
		{a: " #1\t\n\txyz", left: "\n\txyz", want: Rune(1)},
		{a: "#" + strconv.FormatInt(math.MaxInt64, 10) + "0", err: "out of range"},

		{a: "0", want: Line(0)},
		{a: "1", want: Line(1)},
		{a: "12345", want: Line(12345)},
		{a: "12345xyz", left: "xyz", want: Line(12345)},
		{a: " 12345xyz", left: "xyz", want: Line(12345)},
		{a: " 1\t\n\txyz", left: "\n\txyz", want: Line(1)},
		{a: strconv.FormatInt(math.MaxInt64, 10) + "0", err: "out of range"},

		{a: "/", want: Regexp("")},
		{a: "//", want: Regexp("")},
		{a: "/abcdef", want: Regexp("abcdef")},
		{a: "/abc/def", left: "def", want: Regexp("abc")},
		{a: "/abc def", want: Regexp("abc def")},
		{a: "/abc def\nxyz", left: "\nxyz", want: Regexp("abc def")},

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
		{a: "'\na", want: Mark('.'), left: "a"},
		{a: "'☺", want: Mark('☺')},
		{a: "' ☺", want: Mark('☺')},
		{a: "'", want: Mark('.')},

		{a: "+", want: Dot.Plus(Line(1))},
		{a: "+\n2", left: "\n2", want: Dot.Plus(Line(1))},
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
		{a: "/abc/+++---", want: Regexp("abc").Plus(Line(1)).Plus(Line(1)).Plus(Line(1)).Minus(Line(1)).Minus(Line(1)).Minus(Line(1))},

		{a: ".+/aa?/", want: Dot.Plus(Regexp("aa?"))},
		{a: ".-/aa?/", want: Dot.Minus(Regexp("aa?"))},

		{a: ",", want: Line(0).To(End)},
		{a: ",xyz", left: "xyz", want: Line(0).To(End)},
		{a: " , ", want: Line(0).To(End)},
		{a: ",\n1", left: "\n1", want: Line(0).To(End)},
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
		{a: " ;\n1", left: "\n1", want: Line(0).Then(End)},
		{a: ";1", want: Line(0).Then(Line(1))},
		{a: "1;", want: Line(1).Then(End)},
		{a: "0;$", want: Line(0).Then(End)},
		{a: ".;$", want: Dot.Then(End)},
		{a: "1;2", want: Line(1).Then(Line(2))},
		{a: " 1 ; 2 ", want: Line(1).Then(Line(2))},
		{a: ";-#5", want: Line(0).Then(Dot.Minus(Rune(5)))},
		{a: " ; - #5 ", want: Line(0).Then(Dot.Minus(Rune(5)))},

		// Right associative.
		{a: "#0+#1+#2", want: Rune(0).Plus(Rune(1)).Plus(Rune(2))},
		{a: "#0+#1-#2", want: Rune(0).Plus(Rune(1)).Minus(Rune(2))},
		{a: "#0-#1-#2", want: Rune(0).Minus(Rune(1)).Minus(Rune(2))},
		{a: "#0-#1+#2", want: Rune(0).Minus(Rune(1)).Plus(Rune(2))},
		{a: "#0,#1,#2", want: Rune(0).To(Rune(1)).To(Rune(2))},
		{a: "#0,#1;#2", want: Rune(0).To(Rune(1)).Then(Rune(2))},
		{a: "#0;#1;#2", want: Rune(0).Then(Rune(1)).Then(Rune(2))},
		{a: "#0;#1,#2", want: Rune(0).Then(Rune(1)).To(Rune(2))},

		// + and - have higher precidence than , and ;
		{a: "0+1,2+3", want: Line(0).Plus(Line(1)).To(Line(2).Plus(Line(3)))},
		{a: "0-1,2-3", want: Line(0).Minus(Line(1)).To(Line(2).Minus(Line(3)))},
		{a: "0+1,2-3", want: Line(0).Plus(Line(1)).To(Line(2).Minus(Line(3)))},
		{a: "0-1,2+3", want: Line(0).Minus(Line(1)).To(Line(2).Plus(Line(3)))},
		{a: "0+1;2+3", want: Line(0).Plus(Line(1)).Then(Line(2).Plus(Line(3)))},
		{a: "0-1;2-3", want: Line(0).Minus(Line(1)).Then(Line(2).Minus(Line(3)))},
		{a: "0+1;2-3", want: Line(0).Plus(Line(1)).Then(Line(2).Minus(Line(3)))},
		{a: "0-1;2+3", want: Line(0).Minus(Line(1)).Then(Line(2).Plus(Line(3)))},

		// Implicit +.
		{a: "1#2", want: Line(1).Plus(Rune(2))},
		{a: "#2 1", want: Rune(2).Plus(Line(1))},
		{a: "1/abc", want: Line(1).Plus(Regexp("abc"))},
		{a: "/abc/1", want: Regexp("abc").Plus(Line(1))},
		{a: "1+2 3 - 4", want: Line(1).Plus(Line(2)).Plus(Line(3)).Minus(Line(4))},

		// Clamp
		{a: "!", want: Clamp(Dot)},
		{a: "!25", want: Clamp(Line(25))},
		{a: " !   25", want: Clamp(Line(25))},
		{a: "!#25", want: Clamp(Rune(25))},
		{a: "!$", want: Clamp(End)},
		{a: "!.", want: Clamp(Dot)},
		{a: "!'m", want: Clamp(Mark('m'))},
		{a: "!/hello/", want: Clamp(Regexp("hello"))},
		{a: ".+!25", want: Dot.Plus(Clamp(Line(25)))},
		{a: ".-!25", want: Dot.Minus(Clamp(Line(25)))},
		{a: ".-5,.+!5", want: Dot.Minus(Line(5)).To(Dot.Plus(Clamp(Line(5))))},
		{a: ".-!5,.+5", want: Dot.Minus(Clamp(Line(5))).To(Dot.Plus(Line(5)))},
		{a: ".-!5,.+!5", want: Dot.Minus(Clamp(Line(5))).To(Dot.Plus(Clamp(Line(5))))},
		{a: ".-#5,.+!#5", want: Dot.Minus(Rune(5)).To(Dot.Plus(Clamp(Rune(5))))},
		{a: ".-!#5,.+#5", want: Dot.Minus(Clamp(Rune(5))).To(Dot.Plus(Rune(5)))},
		{a: ".-!#5,.+!#5", want: Dot.Minus(Clamp(Rune(5))).To(Dot.Plus(Clamp(Rune(5))))},
	}
	for _, test := range tests {
		rs := strings.NewReader(test.a)
		a, err := Addr(rs)
		if test.err != "" {
			if err == nil || !regexp.MustCompile(test.err).MatchString(err.Error()) {
				t.Errorf(`Addr(%q)=%q,%v, want %q,%q`, test.a, a, err, test.want, test.err)
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
		{addr: Rune(-100), want: Rune(0)},
		{addr: Line(0)},
		{addr: Line(100)},
		{addr: Line(-100), want: Line(0)},
		{addr: Mark('a')},
		{addr: Mark('z')},
		{addr: Mark(' ')},
		{addr: Regexp("☺☹")},
		{addr: Dot.Plus(Line(1))},
		{addr: Dot.Minus(Line(1))},
		{addr: Dot.Minus(Line(1)).Plus(Line(1))},
		{addr: Rune(1).To(Rune(2))},
		{addr: Rune(1).Then(Rune(2))},
		{addr: Regexp("func").Plus(Regexp("[(]"))},
	}
	for _, test := range tests {
		if test.want == nil {
			test.want = test.addr
		}
		str := test.addr.String()
		got, err := Addr(strings.NewReader(str))
		if err != nil || got != test.want {
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
	const helloWorld = "Hello,\nWorld!"
	tests := []struct {
		addr  string
		error string
	}{
		{addr: "1", error: "read error"},
		{addr: "#1+1", error: "read error"},
		{addr: "$-1", error: "read error"},
		{addr: "#3-1", error: "read error"},
		{addr: "/World", error: "no match"},
		{addr: ".+/World", error: "no match"},
		{addr: ".-/World", error: "no match"},
		{addr: "0,/World", error: "no match"},
		{addr: "0;/World", error: "no match"},
		{addr: "/Hello/+", error: "no match"},
		{addr: "/Hello/-", error: "no match"},
		{addr: "/Hello/,", error: "no match"},
		{addr: "/Hello/;", error: "no match"},
		{addr: "/Hello/,/World", error: "no match"},
		{addr: "/Hello/;/World", error: "no match"},
		{addr: "/Hello/+/World", error: "no match"},
		{addr: "/Hello/-/World", error: "no match"},
		{addr: "/Hello/,/World", error: "no match"},
		{addr: "/Hello/;/World", error: "no match"},
	}
	for _, test := range tests {
		rs := strings.NewReader(test.addr)
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
		buf := newBuffer(r)
		defer buf.Close()

		if _, err := buf.Change(Span{}, strings.NewReader(helloWorld)); err != nil {
			t.Fatalf("buf.Change(Span{}, %q)=%q, want nil", helloWorld, err)
		}
		if err := buf.Apply(); err != nil {
			t.Fatalf("buf.Apply()=%q, want nil", err)
		}

		// All subsequent reads will be errors.
		f.error = errors.New("read error")
		if a, err := addr.Where(buf); !matchesError(test.error, err) {
			t.Errorf("Addr(%q).addr()=%v,%v, want addr{},%q", test, a, err, test.error)
			continue
		}
	}
}

// Address returns an edit that sets the mark a to the given address.
func address(a Address) []Edit { return []Edit{Set(a, 'a')} }

var dotTests = []editTest{
	{
		name:  "empty dot at beginning",
		given: "{..}",
		do:    address(Dot),
		want:  "{..aa}",
	},
	{
		name:  "empty dot in middle",
		given: "abc{..}xyz",
		do:    address(Dot),
		want:  "abc{..aa}xyz",
	},
	{
		name:  "empty dot at end",
		given: "abc{..}",
		do:    address(Dot),
		want:  "abc{..aa}",
	},
	{
		name:  "range dot",
		given: "abc{.}123{.}xyz",
		do:    address(Dot),
		want:  "abc{.a}123{.a}xyz",
	},
	{
		name:  "range dot multi-byte runes",
		given: "abc{.}αβξ{.}xyz",
		do:    address(Dot),
		want:  "abc{.a}αβξ{.a}xyz",
	},
	{
		name:  "dot over all",
		given: "{.}abc{.}",
		do:    address(Dot),
		want:  "{.a}abc{.a}",
	},
}

func TestAddressDot(t *testing.T) {
	for _, test := range dotTests {
		test.run(t)
	}
}

func TestAddressDotFromString(t *testing.T) {
	for _, test := range dotTests {
		test.runFromString(t)
	}
}

var markTests = []editTest{
	{
		name:  "empty mark at beginning",
		given: "{..mm}",
		do:    address(Mark('m')),
		want:  "{..aamm}",
	},
	{
		name:  "empty mark in middle",
		given: "{..}abc{mm}xyz",
		do:    address(Mark('m')),
		want:  "{..}abc{aamm}xyz",
	},
	{
		name:  "empty mark at end",
		given: "abc{..mm}",
		do:    address(Mark('m')),
		want:  "abc{..aamm}",
	},
	{
		name:  "all mark",
		given: "{..m}abc{m}",
		do:    address(Mark('m')),
		want:  "{..am}abc{am}",
	},
	{
		name:  "not-previously-set mark",
		given: "{..}abc",
		do:    address(Mark('m')),
		want:  "{..aa}abc",
	},
	{
		name:  "dot mark",
		given: "a{.}b{.}c",
		do:    address(Mark('.')),
		want:  "a{.a}b{.a}c",
	},
	{
		name:  "whitespace mark is dot",
		given: "a{.}b{.}c",
		do:    address(Mark(' ')),
		want:  "a{.a}b{.a}c",
	},
	{
		name:  "non-ASCII mark",
		given: "{..}a{☺}b{☺}c",
		do:    address(Mark('☺')),
		want:  "{..}a{a☺}b{a☺}c",
	},
}

func TestAddressMark(t *testing.T) {
	for _, test := range markTests {
		test.run(t)
	}
}

func TestAddressMarkFromString(t *testing.T) {
	for _, test := range markTests {
		test.runFromString(t)
	}
}

var endTests = []editTest{
	{
		name:  "empty buffer",
		given: "{..}",
		do:    address(End),
		want:  "{..aa}",
	},
	{
		name:  "non-empty buffer",
		given: "{..}abcxzy",
		do:    address(End),
		want:  "{..}abcxzy{aa}",
	},
	{
		name:  "plus",
		given: "{..}abc",
		do:    address(End.Plus(Rune(0))),
		want:  "{..}abc{aa}",
	},
	{
		name:  "minus",
		given: "{..}abc",
		do:    address(End.Minus(Rune(1))),
		want:  "{..}ab{aa}c",
	},
	{
		name:  "to",
		given: "{..}abc",
		do:    address(End.To(End)),
		want:  "{..}abc{aa}",
	},
	{
		name:  "then",
		given: "{..}abc",
		do:    address(End.Then(End)),
		want:  "{..}abc{aa}",
	},
}

func TestAddressEnd(t *testing.T) {
	for _, test := range endTests {
		test.run(t)
	}
}

func TestAddressEndFromString(t *testing.T) {
	for _, test := range endTests {
		test.runFromString(t)
	}
}

var runeTests = []editTest{
	{
		name:  "out of range",
		given: "{..}",
		do:    address(Rune(1)),
		error: "out of range",
	},
	{
		name:  "out of range negative",
		given: "{..}",
		do:    address(Dot.Minus(Rune(1))),
		error: "out of range",
	},
	{
		name:  "empty buffer",
		given: "{..}",
		do:    address(Rune(0)),
		want:  "{..aa}",
	},
	{
		name:  "beginning",
		given: "abc{..}",
		do:    address(Rune(0)),
		want:  "{aa}abc{..}",
	},
	{
		name:  "reverse to beginning",
		given: "abc{..}",
		do:    address(Rune(1).Minus(Rune(1))),
		want:  "{aa}abc{..}",
	},
	{
		name:  "middle",
		given: "{..}abc",
		do:    address(Rune(1)),
		want:  "{..}a{aa}bc",
	},
	{
		name:  "end",
		given: "{..}abc",
		do:    address(Rune(3)),
		want:  "{..}abc{aa}",
	},
	{
		name:  "negative is zero",
		given: "abc{..}",
		do:    address(Rune(-1)),
		want:  "{aa}abc{..}",
	},
	{
		name:  "plus negative rune",
		given: "{..}abcdefg",
		do:    address(Rune(3).Plus(Rune(-2))), // #3+#0
		want:  "{..}abc{aa}defg",
	},
}

func TestAddressRune(t *testing.T) {
	for _, test := range runeTests {
		test.run(t)
	}
}

func TestAddressRuneFromString(t *testing.T) {
	for _, test := range runeTests {
		test.runFromString(t)
	}
}

var lineTests = []editTest{
	{
		name:  "out of range",
		given: "{..}",
		do:    address(Line(2)),
		error: "out of range",
	},
	{
		name:  "negative out of range",
		given: "{..}",
		do:    address(Dot.Minus(Line(2))),
		error: "out of range",
	},
	{
		name:  "empy buffer line 0",
		given: "{..}",
		do:    address(Line(0)),
		want:  "{..aa}",
	},
	{
		name:  "empy buffer line 1",
		given: "{..}",
		do:    address(Line(1)),
		want:  "{..aa}",
	},
	{
		name:  "line 0",
		given: "{..}abc\n",
		do:    address(Line(0)),
		want:  "{..aa}abc\n",
	},
	{
		name:  "line 1 no newline",
		given: "{..}abc",
		do:    address(Line(1)),
		want:  "{..a}abc{a}",
	},
	{
		name:  "line 1",
		given: "{..}abc\n",
		do:    address(Line(1)),
		want:  "{..a}abc\n{a}",
	},
	{
		name:  "line 2 empty",
		given: "{..}abc\n",
		do:    address(Line(2)),
		want:  "{..}abc\n{aa}",
	},
	{
		name:  "line 2 newline only",
		given: "{..}abc\n\n",
		do:    address(Line(2)),
		want:  "{..}abc\n{a}\n{a}",
	},
	{
		name:  "line 2 no newline",
		given: "{..}abc\nxyz",
		do:    address(Line(2)),
		want:  "{..}abc\n{a}xyz{a}",
	},
	{
		name:  "line 2",
		given: "{..}abc\nxyz\n",
		do:    address(Line(2)),
		want:  "{..}abc\n{a}xyz\n{a}",
	},
	{
		name:  "negative is zero",
		given: "{..}abc",
		do:    address(Line(-1)),
		want:  "{..aa}abc",
	},
	{
		name:  "plus line to EOF",
		given: "abc\n{..}abc",
		do:    address(Dot.Plus(Line(1))),
		want:  "abc\n{..a}abc{a}",
	},
	{
		name:  "plus line minus rune",
		given: "abc{..}",
		do:    address(Rune(3).Plus(Line(1)).Minus(Rune(2))),
		want:  "a{aa}bc{..}",
	},
	{
		name:  "plus negative line",
		given: "{..}abc\ndef\nghi",
		do:    address(Line(2).Plus(Line(-2))), // 2+0
		want:  "{..}abc\ndef\n{aa}ghi",
	},
	// BUG(eaburns): This should be an out of range error.
	{
		name:  "plus to out of range",
		given: "abc{..}",
		do:    address(Rune(3).Plus(Line(1))),
		//error: "out of range",
		want: "abc{..aa}",
	},
}

func TestAddressLine(t *testing.T) {
	for _, test := range lineTests {
		test.run(t)
	}
}

func TestAddressLineFromString(t *testing.T) {
	for _, test := range lineTests {
		test.runFromString(t)
	}
}

var regexpTests = []editTest{
	{
		name:  "bad regexp",
		given: "{..}",
		do:    address(Regexp("*")),
		error: "missing",
	},
	{
		name:  "no match",
		given: "{..}",
		do:    address(Regexp("xyz")),
		error: "no match",
	},
	{
		name:  "empty regexp",
		given: "{..}Hello 世界",
		do:    address(Regexp("")),
		want:  "{..aa}Hello 世界",
	},
	{
		name:  "empty match from beginning",
		given: "{..}aaaaa",
		do:    address(Regexp("b*")),
		want:  "{..aa}aaaaa",
	},
	{
		name:  "empty match",
		given: "aaaa{..}a",
		do:    address(Regexp("b*")),
		want:  "aaaa{..aa}a",
	},
	{
		name:  "empty match from end",
		given: "aaaaa{..}",
		do:    address(Regexp("b*")),
		want:  "aaaaa{..aa}",
	},
	{
		name:  "reverse empty match from beginning",
		given: "{..}aaaaa",
		do:    address(Dot.Minus(Regexp("b*"))),
		want:  "{..aa}aaaaa",
	},
	{
		name:  "reverse empty match",
		given: "aaa{..}aa",
		do:    address(Dot.Minus(Regexp("b*"))),
		want:  "aaa{..aa}aa",
	},
	{
		name:  "reverse empty match from end",
		given: "aaaaa{..}",
		do:    address(Dot.Minus(Regexp("b*"))),
		want:  "aaaaa{..aa}",
	},
	{
		name:  "simple",
		given: "{..}Hello 世界",
		do:    address(Regexp("Hello")),
		want:  "{..a}Hello{a} 世界",
	},
	{
		name:  "meta",
		given: "{..}Hello 世界",
		do:    address(Regexp("[^ ]+")),
		want:  "{..a}Hello{a} 世界",
	},
	{
		name:  "a star empty match #0",
		given: "{..}",
		do:    address(Regexp("a*")),
		want:  "{..aa}",
	},
	{
		name:  "a star match 1",
		given: "{..}a",
		do:    address(Regexp("a*")),
		want:  "{..a}a{a}",
	},
	{
		name:  "a star match 1 mid-line",
		given: "xyz{..}a",
		do:    address(Regexp("a*")),
		want:  "xyz{..a}a{a}",
	},
	{
		name:  "a star match many",
		given: "{..}aaaa",
		do:    address(Regexp("a*")),
		want:  "{..a}aaaa{a}",
	},
	{
		name:  "non-ASCII",
		given: "{..}Hello 世界",
		do:    address(Regexp("世界")),
		want:  "{..}Hello {a}世界{a}",
	},
	{
		name:  "reverse match",
		given: "abc abc abc {..}abc",
		do:    address(Dot.Minus(Regexp("abc"))),
		want:  "abc abc {a}abc{a} {..}abc",
	},
	{
		name:  "reverse match straddle starting point",
		given: "abc{..}def",
		do:    address(Dot.Minus(Regexp("abcdef"))),
		want:  "{a}abc{..}def{a}",
	},
	{
		name:  "reverse match from within would-be match",
		given: "abcdef abc{..}def",
		do:    address(Dot.Minus(Regexp("abcdef"))),
		want:  "{a}abcdef{a} abc{..}def",
	},
	{
		name:  "reverse only match prefix of would-be match",
		given: "aaaa{..}aaaaa",
		do:    address(Dot.Minus(Regexp("a*"))),
		want:  "{a}aaaa{..a}aaaaa",
	},
	{
		name:  "^ starting from beginning of line",
		given: "abc\n{..}def",
		do:    address(Regexp("^def")),
		want:  "abc\n{..a}def{a}",
	},
	{
		name:  "^ starting from beginning of text",
		given: "{..}def",
		do:    address(Regexp("^def")),
		want:  "{..a}def{a}",
	},
	{
		name:  "^ starting from mid line",
		given: "abc{..}def",
		do:    address(Regexp("^def")),
		want:  "abc{..a}def{a}",
	},
	{
		name:  "reverse ^ starting from beginning of line",
		given: "abc\ndef{..}",
		do:    address(Dot.Minus(Regexp("^def"))),
		want:  "abc\n{a}def{..a}",
	},
	{
		name:  "reverse ^ starting from beginning of text",
		given: "def{..}",
		do:    address(Dot.Minus(Regexp("^def"))),
		want:  "{a}def{..a}",
	},
	{
		name:  "reverse ^ starting from mid line",
		given: "abcdef{..}",
		do:    address(Dot.Minus(Regexp("^def"))),
		error: "no match",
		want:  "abcdef{..}",
	},
	{

		name:  `\A matches relative beginning`,
		given: "abc\n{..}def",
		do:    address(Regexp(`\Adef`)),
		want:  "abc\n{..a}def{a}",
	},
	{
		name:  "$ matches end of line",
		given: "{..}abc\ndef",
		do:    address(Regexp("abc$")),
		want:  "{..a}abc{a}\ndef",
	},
	{
		name:  "$ matches end of text",
		given: "{..}abc",
		do:    address(Regexp("abc$")),
		want:  "{..a}abc{a}",
	},
	{
		name:  "$ does not match mid line",
		given: "{..}abcdef",
		do:    address(Regexp("abc$")),
		error: "no match",
		want:  "{..}abcdef",
	},
	{
		name:  "reverse $ starting from end of line",
		given: "abc{..}\ndef",
		do:    address(Dot.Minus(Regexp("abc$"))),
		want:  "{a}abc{..a}\ndef",
	},
	{
		name:  "reverse $ starting from end of text",
		given: "abc{..}",
		do:    address(Dot.Minus(Regexp("abc$"))),
		want:  "{a}abc{..a}",
	},
	{
		name:  "reverse $ starting from mid line",
		given: "abc{..}def",
		do:    address(Dot.Minus(Regexp("abc$"))),
		want:  "{a}abc{..a}def",
	},
	{
		name:  "forward relative to dot.to",
		given: "abcx{.}xxabcxx{.}xabc",
		do:    address(Regexp("abc")),
		want:  "abcx{.}xxabcxx{.}x{a}abc{a}",
	},
	{
		name:  "relative to dot in a range",
		given: "abcabcxyz{..}abc",
		do:    address(Rune(2).To(Regexp("abc"))),
		want:  "ab{a}cabcxyz{..}abc{a}",
	},
	{
		name:  "relative to a1 in a plus",
		given: "12abc{..}xyzabc",
		do:    address(Rune(2).Plus(Regexp("abc"))),
		want:  "12{a}abc{a}{..}xyzabc",
	},
	{
		name:  "relative to a1 in a minus",
		given: "abc{..}xyzabc12",
		do:    address(End.Minus(Regexp("abc"))),
		want:  "abc{..}xyz{a}abc{a}12",
	},
	{
		name:  "wrap",
		given: "xxx abc xxx a{..}bc xxx",
		do:    address(Regexp("abc")),
		want:  "xxx {a}abc{a} xxx a{..}bc xxx",
	},
	{
		name:  "reverse wrap",
		given: "xxx ab{..}c xxx abc xxx",
		do:    address(Dot.Minus(Regexp("abc"))),
		want:  "xxx ab{..}c xxx {a}abc{a} xxx",
	},
	{

		name:  "replacement character no match",
		given: "abc{..}xyz",
		do:    address(Regexp(`\x{FFFD}xyz`)),
		error: "no match",
		want:  "abc{..}xyz{}",
	},
	{

		name:  "replacement character match",
		given: "abc{..}\uFFFD",
		do:    address(Regexp(`\x{FFFD}`)),
		want:  "abc{..a}\uFFFD{a}",
	},
	{

		name:  `only \`,
		given: `{..}abc\`,
		do:    address(Regexp(`\`)),
		want:  `{..}abc{a}\{a}`,
	},
	{

		name:  `trailing \`,
		given: `{..}abc\`,
		do:    address(Regexp(`abc\`)),
		want:  `{..a}abc\{a}`,
	},
	{
		name:  "non-capturing group",
		given: "{..}abc",
		do:    address(Regexp("(?:abc)")),
		want:  "{..a}abc{a}",
	},
}

func TestAddressRegexp(t *testing.T) {
	for _, test := range regexpTests {
		test.run(t)
	}
}

func TestAddressRegexpFromString(t *testing.T) {
	for _, test := range regexpTests {
		test.runFromString(t)
	}
}

// Tests regexp String().
func TestRegexpString(t *testing.T) {
	tests := []struct {
		re, want string
	}{
		{``, `//`},
		{`abc`, `/abc/`},
		{`ab/c`, `/ab\/c/`},
		{`ab[/]c`, `/ab[\/]c/`},
		{"\n", `/\n/`}, // Raw newlines are escaped.
	}
	for _, test := range tests {
		re := Regexp(test.re)
		if s := re.String(); s != test.want {
			t.Errorf("Regexp(%q).String()=%q, want %q", test.re, s, test.want)
		}
	}
}

var plusTests = []editTest{
	{
		name:  "out of range",
		given: "{..}",
		do:    address(Dot.Plus(Rune(1))),
		error: "out of range",
	},
	{
		name:  "plus dot address",
		given: "a{..}bc",
		do:    address(Rune(0).Plus(Dot)),
		want:  "a{..aa}bc",
	},
	{
		name:  "plus end address",
		given: "{..}abc",
		do:    address(Rune(0).Plus(End)),
		want:  "{..}abc{aa}",
	},
	{
		name:  "plus mark address",
		given: "{..}ab{mm}c",
		do:    address(Rune(0).Plus(Mark('m'))),
		want:  "{..}ab{aamm}c",
	},
	{
		name:  "plus rune address",
		given: "{..}abc",
		do:    address(Dot.Plus(Rune(1))),
		want:  "{..}a{aa}bc",
	},
	{
		name:  "full line plus line address",
		given: "{.}abc\n{.}abc",
		do:    address(Dot.Plus(Line(1))),
		want:  "{.}abc\n{.a}abc{a}",
	},
	{
		name:  "partial line plus line address",
		given: "{.}ab{.}c\nabc",
		do:    address(Dot.Plus(Line(1))),
		want:  "{.}ab{.}c\n{a}abc{a}",
	},
	{
		name:  "plus compound address",
		given: "{..}abc",
		do:    address(Rune(1).Plus(Rune(1)).Plus(Rune(1))),
		want:  "{..}abc{aa}",
	},
	{
		name:  "plus range address",
		given: "{..}abc",
		do:    address(Regexp("ab").Plus(Rune(1))),
		want:  "{..}abc{aa}",
	},
}

func TestAddressPlus(t *testing.T) {
	for _, test := range plusTests {
		test.run(t)
	}
}

func TestAddressPlusFromString(t *testing.T) {
	for _, test := range plusTests {
		test.runFromString(t)
	}
}

var minusTests = []editTest{
	{
		name:  "rune out of range",
		given: "{..}",
		do:    address(Dot.Minus(Rune(1))),
		error: "out of range",
	},
	{
		name:  "line out of range",
		given: "{..}",
		do:    address(Dot.Minus(Line(2))),
		error: "out of range",
	},
	{
		name:  "minus dot address",
		given: "a{..}bc",
		do:    address(End.Minus(Dot)),
		want:  "a{..aa}bc",
	},
	{
		name:  "minus end address",
		given: "{..}abc",
		do:    address(End.Minus(End)),
		want:  "{..}abc{aa}",
	},
	{
		name:  "minus mark address",
		given: "{..}ab{mm}c",
		do:    address(End.Minus(Mark('m'))),
		want:  "{..}ab{aamm}c",
	},
	{
		name:  "minus rune",
		given: "abc{..}",
		do:    address(Dot.Minus(Rune(1))),
		want:  "ab{aa}c{..}",
	},
	{
		name:  "end minus line",
		given: "abc\nabc{..}",
		do:    address(Dot.Minus(Line(1))),
		want:  "{a}abc\n{a}abc{..}",
	},
	{
		name:  "full line minus line",
		given: "abc\n{.}abc\n{.}",
		do:    address(Dot.Minus(Line(1))),
		want:  "{a}abc\n{.a}abc\n{.}",
	},
	{
		name:  "partial line minus line",
		given: "abc\na{.}bc\n{.}",
		do:    address(Dot.Minus(Line(1))),
		want:  "{a}abc\n{a}a{.}bc\n{.}",
	},
	{
		name:  "minus line to #0",
		given: "ab{..}c",
		do:    address(Dot.Minus(Line(1))),
		want:  "{aa}ab{..}c",
	},
	{
		name:  "minus line to 1",
		given: "abc\n{.}xyz{.}",
		do:    address(Dot.Minus(Line(1))),
		want:  "{a}abc\n{.a}xyz{.}",
	},
	{
		name:  "minus to non-first line",
		given: "abc\nabc\nabc{..}",
		do:    address(Dot.Minus(Line(1))),
		want:  "abc\n{a}abc\n{a}abc{..}",
	},
	{
		name:  "minus to first line",
		given: "abc\n{.}abc\n{.}abc{",
		do:    address(Dot.Minus(Line(1))),
		want:  "{a}abc\n{a}{.}abc\n{.}abc{",
	},
	{
		name:  "minus compound address",
		given: "abc{..}",
		do:    address(Rune(2).Minus(Rune(1)).Minus(Rune(1))),
		want:  "{aa}abc{..}",
	},
	{
		name:  "minus range address",
		given: "abc{..}",
		do:    address(Regexp("bc").Minus(Rune(1))),
		want:  "{aa}abc{..}",
	},
}

func TestAddressMinus(t *testing.T) {
	for _, test := range minusTests {
		test.run(t)
	}
}

func TestAddressMinusFromString(t *testing.T) {
	for _, test := range minusTests {
		test.runFromString(t)
	}
}

var toTests = []editTest{
	{
		name:  "out of range",
		given: "{..}",
		do:    address(Dot.To(Rune(1))),
		error: "out of range",
	},
	{
		name:  "empty buffer",
		given: "{..}",
		do:    address(Rune(0).To(End)),
		want:  "{..aa}",
	},
	{
		name:  "simple address to simple address",
		given: "{..}abc",
		do:    address(Rune(0).To(Rune(3))),
		want:  "{..a}abc{a}",
	},
	{
		name:  "simple address to compound address",
		given: "{..}abc",
		do:    address(Rune(0).To(Rune(2).Plus(Rune(1)))),
		want:  "{..a}abc{a}",
	},
	{
		name:  "compound address to simple address",
		given: "{..}abc",
		do:    address(Rune(0).Plus(Rune(1)).To(Rune(3))),
		want:  "{..}a{a}bc{a}",
	},
	{
		name:  "compound address to compound address",
		given: "{..}abc",
		do:    address(Rune(0).Plus(Rune(1)).To(Rune(2).Plus(Rune(1)))),
		want:  "{..}a{a}bc{a}",
	},
	{
		name:  "range address to simple address",
		given: "{..}abc",
		do:    address(Rune(0).To(Rune(1)).To(Rune(2))),
		want:  "{..a}ab{a}c",
	},
	{
		name:  "range address to compound address",
		given: "{..}abc",
		do:    address(Rune(0).To(Rune(1)).To(Rune(2).Plus(Rune(1)))),
		want:  "{..a}abc{a}",
	},
}

func TestAddressTo(t *testing.T) {
	for _, test := range toTests {
		test.run(t)
	}
}

func TestAddressToFromString(t *testing.T) {
	for _, test := range toTests {
		test.runFromString(t)
	}
}

var thenTests = []editTest{
	{
		name:  "out of range",
		given: "{..}",
		do:    address(Dot.Then(Rune(1))),
		error: "out of range",
	},
	{
		name:  "empty buffer",
		given: "{..}",
		do:    address(Rune(0).Then(End)),
		want:  "{..aa}",
	},
	{
		name:  "simple address then simple address",
		given: "{..}abc",
		do:    address(Rune(1).Then(Rune(2))),
		want:  "{..}a{a}b{a}c",
	},
	{
		name:  "simple address then compound address",
		given: "{..}abc",
		do:    address(Rune(1).Then(Rune(1).Plus(Rune(1)))),
		want:  "{..}a{a}b{a}c",
	},
	{
		name:  "compound address then simple address",
		given: "{..}abcde",
		do:    address(Rune(0).Plus(Rune(1)).Then(Rune(3))),
		want:  "{..}a{a}bc{a}de",
	},
	{
		name:  "compound address then compound address",
		given: "{..}abcde",
		do:    address(Rune(0).Plus(Rune(1)).Then(Rune(2).Plus(Rune(1)))),
		want:  "{..}a{a}bc{a}de",
	},
	{
		name:  "range address then simple address",
		given: "{..}abcdef",
		do:    address(Rune(0).To(Rune(1)).Then(Rune(2))),
		want:  "{..a}ab{a}cdef",
	},
	{
		name:  "range address then compound address",
		given: "{..}abcde",
		do:    address(Rune(0).To(Rune(1)).Then(Rune(2).Plus(Rune(1)))),
		want:  "{..a}abc{a}de",
	},
	{
		name:  "a2 evaluated from end of a1",
		given: "{..}abcxyzabc",
		do:    address(Regexp("xyz").Then(Regexp("abc"))),
		want:  "{..}abc{a}xyzabc{a}",
	},
	{
		name:  "a2 is a mark",
		given: "{..}1234567{mm}89",
		do:    address(Line(0).Then(Mark('m'))),
		want:  "{..a}1234567{amm}89",
	},
}

func TestAddressThen(t *testing.T) {
	for _, test := range thenTests {
		test.run(t)
	}
}

func TestAddressThenFromString(t *testing.T) {
	for _, test := range thenTests {
		test.runFromString(t)
	}
}

var clampTests = []editTest{
	{
		name:  "clamp line",
		given: "{..}abc\nxyz",
		do:    address(Clamp(Line(25))),
		want:  "{..}abc\nxyz{aa}",
	},
	{
		name:  "clamp rune",
		given: "{..}abc\nxyz",
		do:    address(Clamp(Rune(1000))),
		want:  "{..}abc\nxyz{aa}",
	},
	{
		name:  "no need to clamp",
		given: "12{..}34",
		do:    address(Clamp(Rune(1))),
		want:  "1{aa}2{..}34",
	},
	{
		name:  "clamp dot",
		given: "12{..}34",
		do:    address(Clamp(Dot)),
		want:  "12{..aa}34",
	},
	{
		name:  "clamp end",
		given: "12{..}34",
		do:    address(Clamp(End)),
		want:  "12{..}34{aa}",
	},
	{
		name:  "clamp mark",
		given: "12{..}3{mm}4",
		do:    address(Clamp(Mark('m'))),
		want:  "12{..}3{aamm}4",
	},
	{
		name:  "clamp regexp",
		given: "12{..}34",
		do:    address(Clamp(Regexp("4"))),
		want:  "12{..}3{a}4{a}",
	},
	{
		name:  "clamp regexp not found",
		given: "12{..}34",
		do:    address(Clamp(Regexp("5"))),
		error: "no match",
		want:  "12{..}34",
	},
	{
		name:  "plus clamp",
		given: "{..}abc\nxyz",
		do:    address(Dot.Plus(Clamp(Line(25)))),
		want:  "{..}abc\nxyz{aa}",
	},
	{
		name:  "minus clamp",
		given: "abc\nxyz{..}",
		do:    address(Dot.Minus(Clamp(Rune(1000)))),
		want:  "{aa}abc\nxyz{..}",
	},
	{
		name:  "clamp end but not beginning",
		given: "12{..}34",
		do:    address(Dot.Minus(Rune(5)).To(Dot.Plus(Clamp(Rune(5))))),
		error: "out of range",
		want:  "12{..}34",
	},
	{
		name:  "clamp beginning but not end",
		given: "12{..}34",
		do:    address(Dot.Minus(Clamp(Rune(5))).To(Dot.Plus(Rune(5)))),
		error: "out of range",
		want:  "12{..}34",
	},
	{
		name:  "clamp end and beginning",
		given: "12{..}34",
		do:    address(Dot.Minus(Clamp(Rune(5))).To(Dot.Plus(Clamp(Rune(5))))),
		want:  "{a}12{..}34{a}",
	},
	{
		name:  "clamp plus",
		given: "12{..}34",
		do:    address(Clamp(Rune(5)).Plus(Rune(0))),
		want:  "12{..}34{aa}",
	},
	{
		name:  "clamp minus",
		given: "12{..}34",
		do:    address(Clamp(Rune(5)).Minus(Rune(1))),
		want:  "12{..}3{aa}4",
	},
	{
		name:  "clamp to",
		given: "12{..}34",
		do:    address(Clamp(Rune(5)).To(Rune(4))),
		want:  "12{..}34{aa}",
	},
	{
		name:  "clamp then",
		given: "12{..}34",
		do:    address(Clamp(Rune(5)).Then(Rune(4))),
		want:  "12{..}34{aa}",
	},
}

func TestAddressClamp(t *testing.T) {
	for _, test := range clampTests {
		test.run(t)
	}
}

func TestAddressClampFromString(t *testing.T) {
	for _, test := range clampTests {
		test.runFromString(t)
	}
}
