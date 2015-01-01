package edit

import (
	"regexp"
	"strconv"
	"testing"
	"unicode/utf8"

	"github.com/eaburns/T/runes"
)

const testBlockSize = 12

func TestDotAddress(t *testing.T) {
	str := "Hello, 世界!"
	sz := int64(utf8.RuneCountInString(str))
	tests := []addressTest{
		{text: str, dot: pt(0), addr: Dot(), want: pt(0)},
		{text: str, dot: pt(5), addr: Dot(), want: pt(5)},
		{text: str, dot: rng(5, 6), addr: Dot(), want: rng(5, 6)},
		{text: str, dot: pt(sz), addr: Dot(), want: pt(sz)},
		{text: str, dot: rng(0, sz), addr: Dot(), want: rng(0, sz)},

		{text: str, dot: pt(-1), addr: Dot(), err: "out of range"},
		{text: str, dot: rng(-1, 0), addr: Dot(), err: "out of range"},
		{text: str, dot: pt(sz + 1), addr: Dot(), err: "out of range"},
		{text: str, dot: rng(0, sz+1), addr: Dot(), err: "out of range"},
		{text: str, dot: rng(-1, sz+1), addr: Dot(), err: "out of range"},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestEndAddress(t *testing.T) {
	tests := []addressTest{
		{text: "", addr: End(), want: pt(0)},
		{text: "Hello, World!", addr: End(), want: pt(13)},
		{text: "Hello, 世界!", addr: End(), want: pt(10)},
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
		{text: "Hello, 世界!", addr: Regexp("/"), want: pt(0)},
		{text: "Hello, 世界!", addr: Regexp("/H"), want: rng(0, 1)},
		{text: "Hello, 世界!", addr: Regexp("/."), want: rng(0, 1)},
		{text: "Hello, 世界!", addr: Regexp("/世界"), want: rng(7, 9)},
		{text: "Hello, 世界!", addr: Regexp("/[^!]+"), want: rng(0, 9)},

		{text: "Hello, 世界!", dot: pt(10), addr: Regexp("?"), want: pt(10)},
		{text: "Hello, 世界!", dot: pt(10), addr: Regexp("?!"), want: rng(9, 10)},
		{text: "Hello, 世界!", dot: pt(10), addr: Regexp("?."), want: rng(9, 10)},
		{text: "Hello, 世界!", dot: pt(10), addr: Regexp("?H"), want: rng(0, 1)},
		{text: "Hello, 世界!", dot: pt(10), addr: Regexp("?[^!]+"), want: rng(0, 9)},

		{text: "Hello, 世界!", dot: pt(10), addr: Regexp("/H").reverse(), want: rng(0, 1)},
		{text: "Hello, 世界!", addr: Regexp("?H").reverse(), want: rng(0, 1)},

		// Wrap.
		{text: "Hello, 世界!", addr: Regexp("?世界"), want: rng(7, 9)},
		{text: "Hello, 世界!", dot: pt(8), addr: Regexp("/世界"), want: rng(7, 9)},

		{text: "Hello, 世界!", addr: Regexp("/☺"), err: "no match"},
		{text: "Hello, 世界!", addr: Regexp("?☺"), err: "no match"},
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
		{"", "//"},
		{"/", "//"},
		{"☺", "☺☺"},
		{"//", "//"},
		{"☺☺", "☺☺"},
		{`/\/`, `/\//`},
		{`☺\☺`, `☺\☺☺`},
		{"/abc", "/abc/"},
		{"/abc/", "/abc/"},
		{"☺abc", "☺abc☺"},
		{"☺abc☺", "☺abc☺"},
		{"/abc", "/abc/"},
		{`/abc\/`, `/abc\//`},
		{`☺abc\☺`, `☺abc\☺☺`},
	}
	for _, test := range tests {
		re := Regexp(test.re)
		if s := re.String(); s != test.want {
			t.Errorf("Regexp(%s).String()=%s, want %s", strconv.Quote(test.re),
				strconv.Quote(s), strconv.Quote(test.want))
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

		{text: "abc\ndef", dot: pt(1), addr: Dot().Plus(Line(-1)), want: pt(0)},
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
		{text: "abc\ndef", dot: rng(1, 6), addr: Dot().Minus(Line(1)).Plus(Line(1)), want: rng(0, 4)},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestToAddress(t *testing.T) {
	tests := []addressTest{
		{text: "abc", addr: Line(0).To(End()), want: rng(0, 3)},
		{text: "abc", dot: pt(1), addr: Dot().To(End()), want: rng(1, 3)},
		{text: "abc\ndef", addr: Line(0).To(Line(1)), want: rng(0, 4)},
		{text: "abc\ndef", addr: Line(1).To(Line(2)), want: rng(0, 7)},
		{
			text: "abcabc",
			addr: Regexp("/abc").To(Regexp("/b")),
			want: rng(0, 2),
		},
		{
			text: "abc\ndef\nghi\njkl",
			dot:  pt(11),
			addr: Regexp("?abc?").Plus(Line(1)).To(Dot()),
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
		{text: "abcabc", addr: Regexp("/abc/").Then(Regexp("/b/")), want: rng(0, 5)},
		{text: "abcabc", addr: Regexp("/abc/").Then(Dot().Plus(Rune(1))), want: rng(0, 4)},
		{text: "abcabc", addr: Line(0).Plus(Rune(1)).Then(Dot().Plus(Rune(1))), want: rng(1, 2)},
		{text: "abcabc", addr: Line(0).To(Rune(1)).Then(Dot().Plus(Rune(1))), want: rng(0, 2)},
	}
	for _, test := range tests {
		test.run(t)
	}
}

type addressTest struct {
	text string
	// If rev==false, the match starts from 0.
	// If rev==true, the match starts from len(text).
	dot  addr
	addr Address
	want addr
	err  string // regexp matching the error string
}

func (test addressTest) run(t *testing.T) {
	e := Editor{dot: test.dot, runes: runes.NewBuffer(testBlockSize)}
	defer e.runes.Close()
	if _, err := e.runes.Insert([]rune(test.text), 0); err != nil {
		t.Fatalf(`Put("%s")=%v, want nil`, test.text, err)
	}
	a, err := test.addr.addrFrom(test.dot.to, &e)
	var errStr string
	if err != nil {
		errStr = err.Error()
	}
	if a != test.want ||
		(test.err == "" && errStr != "") ||
		(test.err != "" && !regexp.MustCompile(test.err).MatchString(errStr)) {
		t.Errorf(`Address("%s").range(%d, %v)=%v, %v, want %v, %v`,
			test.addr.String(), test.dot, strconv.Quote(test.text), a, err,
			test.want, test.err)
	}
}

func pt(p int64) addr         { return addr{from: p, to: p} }
func rng(from, to int64) addr { return addr{from: from, to: to} }

func TestAddr(t *testing.T) {
	tests := []struct {
		addr string
		n    int
		want Address
	}{
		{addr: "", n: 0, want: nil},
		{addr: " ", n: 1, want: nil},
		{addr: "\t\t", n: 2, want: nil},

		{addr: "#0", n: 2, want: Rune(0)},
		{addr: "#1", n: 2, want: Rune(1)},
		{addr: "#", n: 1, want: Rune(1)},
		{addr: "#12345", n: 6, want: Rune(12345)},
		{addr: "#12345xyz", n: 6, want: Rune(12345)},
		{addr: " #12345xyz", n: 7, want: Rune(12345)},

		{addr: "0", n: 1, want: Line(0)},
		{addr: "1", n: 1, want: Line(1)},
		{addr: "12345", n: 5, want: Line(12345)},
		{addr: "12345xyz", n: 5, want: Line(12345)},
		{addr: " 12345xyz", n: 6, want: Line(12345)},

		{addr: "/", n: 1, want: Regexp("/")},
		{addr: "//", n: 2, want: Regexp("//")},
		{addr: "?", n: 1, want: Regexp("?")},
		{addr: "??", n: 2, want: Regexp("??")},
		{addr: "/abcdef", n: 7, want: Regexp("/abcdef")},
		{addr: "/abc/def", n: 5, want: Regexp("/abc/")},
		{addr: "/abc def", n: 8, want: Regexp("/abc def")},
		{addr: "?abcdef", n: 7, want: Regexp("?abcdef")},
		{addr: "?abc?def", n: 5, want: Regexp("?abc?")},
		{addr: "?abc def", n: 8, want: Regexp("?abc def")},
		{addr: " ?abc def", n: 9, want: Regexp("?abc def")},

		{addr: "$", n: 1, want: End()},
		{addr: " $", n: 2, want: End()},

		{addr: ".", n: 1, want: Dot()},
		{addr: " .", n: 2, want: Dot()},

		{addr: "+", n: 1, want: Dot().Plus(Line(1))},
		{addr: "+xyz", n: 1, want: Dot().Plus(Line(1))},
		{addr: "+5", n: 2, want: Dot().Plus(Line(5))},
		{addr: "5+", n: 2, want: Line(5).Plus(Line(1))},
		{addr: "5+6", n: 3, want: Line(5).Plus(Line(6))},
		{addr: " 5 + 6", n: 6, want: Line(5).Plus(Line(6))},
		{addr: "-", n: 1, want: Dot().Minus(Line(1))},
		{addr: "-xyz", n: 1, want: Dot().Minus(Line(1))},
		{addr: "-5", n: 2, want: Dot().Minus(Line(5))},
		{addr: "5-", n: 2, want: Line(5).Minus(Line(1))},
		{addr: "5-6", n: 3, want: Line(5).Minus(Line(6))},
		{addr: " 5 - 6", n: 6, want: Line(5).Minus(Line(6))},
		{addr: ".+#5", n: 4, want: Dot().Plus(Rune(5))},
		{addr: "$-#5", n: 4, want: End().Minus(Rune(5))},
		{addr: "$ - #5 + #3", n: 11, want: End().Minus(Rune(5)).Plus(Rune(3))},
		{addr: "+-", n: 2, want: Dot().Plus(Line(1)).Minus(Line(1))},
		{addr: " + - ", n: 5, want: Dot().Plus(Line(1)).Minus(Line(1))},
		{addr: " - + ", n: 5, want: Dot().Minus(Line(1)).Plus(Line(1))},
		{addr: "/abc/+++---", n: 11, want: Regexp("/abc/").Plus(Line(1)).Plus(Line(1)).Plus(Line(1)).Minus(Line(1)).Minus(Line(1)).Minus(Line(1))},

		{addr: ",", n: 1, want: Line(0).To(End())},
		{addr: ",xyz", n: 1, want: Line(0).To(End())},
		{addr: " , ", n: 3, want: Line(0).To(End())},
		{addr: ",1", n: 2, want: Line(0).To(Line(1))},
		{addr: "1,", n: 2, want: Line(1).To(End())},
		{addr: "0,$", n: 3, want: Line(0).To(End())},
		{addr: ".,$", n: 3, want: Dot().To(End())},
		{addr: "1,2", n: 3, want: Line(1).To(Line(2))},
		{addr: " 1 , 2 ", n: 7, want: Line(1).To(Line(2))},
		{addr: ",-#5", n: 4, want: Line(0).To(Dot().Minus(Rune(5)))},
		{addr: " , - #5", n: 7, want: Line(0).To(Dot().Minus(Rune(5)))},
		{addr: ";", n: 1, want: Line(0).Then(End())},
		{addr: ";xyz", n: 1, want: Line(0).Then(End())},
		{addr: " ; ", n: 3, want: Line(0).Then(End())},
		{addr: ";1", n: 2, want: Line(0).Then(Line(1))},
		{addr: "1;", n: 2, want: Line(1).Then(End())},
		{addr: "0;$", n: 3, want: Line(0).Then(End())},
		{addr: ".;$", n: 3, want: Dot().Then(End())},
		{addr: "1;2", n: 3, want: Line(1).Then(Line(2))},
		{addr: " 1 ; 2 ", n: 7, want: Line(1).Then(Line(2))},
		{addr: ";-#5", n: 4, want: Line(0).Then(Dot().Minus(Rune(5)))},
		{addr: " ; - #5 ", n: 8, want: Line(0).Then(Dot().Minus(Rune(5)))},
		{addr: ";,", n: 2, want: Line(0).Then(Line(0).To(End()))},
		{addr: " ; , ", n: 5, want: Line(0).Then(Line(0).To(End()))},

		// Implicit +.
		{addr: "1#2", n: 3, want: Line(1).Plus(Rune(2))},
		{addr: "#2 1", n: 4, want: Rune(2).Plus(Line(1))},
		{addr: "1/abc", n: 5, want: Line(1).Plus(Regexp("/abc"))},
		{addr: "/abc/1", n: 6, want: Regexp("/abc/").Plus(Line(1))},
		{addr: "?abc?1", n: 6, want: Regexp("?abc?").Plus(Line(1))},
		{addr: "$?abc", n: 5, want: End().Plus(Regexp("?abc"))},
	}
	for _, test := range tests {
		got, n, err := Addr([]rune(test.addr))
		if got != test.want || n != test.n || err != nil {
			t.Errorf(`Addr("%s")=%v,%d,%v, want %v,%d,nil`,
				test.addr, got, n, err, test.want, test.n)
		}
	}
}
