package re1

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
)

// TestJustParse just tests parse errors (or lack thereof).
func TestJustParse(t *testing.T) {
	tests := []struct {
		re string
		// What the compiler actually used.
		// If delim==false, initialized to re.
		expr           string
		delim, literal bool
		err            error
	}{
		{re: ""},
		{re: "abc"},
		{re: "(abc)"},
		{re: "a**"},
		{re: "a*?+bc"},
		{re: "a(bcd)*e"},
		{re: `a\|`},
		{re: `a\(`},
		{re: `a\)`},
		{re: `a\[`},
		{re: `a\]`},
		{re: `a|`, literal: true},
		{re: `a(`, literal: true},
		{re: `a)`, literal: true},
		{re: `a[`, literal: true},
		{re: `a]`, literal: true},
		{re: "a(bcd", err: ParseError{Position: 1}},
		{re: "a(b(c)d", err: ParseError{Position: 1}},
		{re: "a(b(cd", err: ParseError{Position: 3}},
		{re: "a|", err: ParseError{Position: 1}},
		{re: "a)", err: ParseError{Position: 1}},
		{re: "a)xyz", err: ParseError{Position: 1}},
		{re: "()xyz", err: ParseError{Position: 0}},
		// Acme allows this, treating ] as a literal ']'.
		// We don't allow it. The man page is a bit unclear,
		// but I think it intends to say that metacharacters
		// must be escaped to be literals. Otherwise, how
		// does one distinguish?
		{re: "a]", err: ParseError{Position: 1}},
		{re: "a]xyz", err: ParseError{Position: 1}},

		// Character classes.
		{re: `[]`, err: ParseError{Position: 0}},
		{re: `[`, err: ParseError{Position: 0}},
		{re: `[a-`, err: ParseError{Position: 2}},
		{re: `[b-a`, err: ParseError{Position: 3}},
		{re: `[^`, err: ParseError{Position: 0}},
		{re: `[^a-`, err: ParseError{Position: 3}},
		{re: `[^b-a`, err: ParseError{Position: 4}},
		{re: `[xyz`, err: ParseError{Position: 0}},
		{re: `[xyza-`, err: ParseError{Position: 5}},
		{re: `[xyzb-a`, err: ParseError{Position: 6}},
		{re: `[^xyz`, err: ParseError{Position: 0}},
		{re: `[^xyza-`, err: ParseError{Position: 6}},
		{re: `[^xyzb-a`, err: ParseError{Position: 7}},
		{re: `[a]`},
		{re: `[^a]`},
		{re: `[abc]`},
		{re: `[^abc]`},
		{re: `[a-zA-Z0-9_]`},
		{re: `[^a-zA-Z0-9_]`},
		{re: `[\^\-\]]`},

		// Delimiters.
		{re: "/abc", delim: true, expr: "/abc"},
		{re: "/abc/", delim: true, expr: "/abc/"},
		{re: "/abc/xyz", delim: true, expr: "/abc/"},
		{re: `/abc\/xyz`, delim: true, expr: `/abc\/xyz`},
		{re: `/abc\/xyz/`, delim: true, expr: `/abc\/xyz/`},
		// No error, since we hit the delimiter before the would-be error.
		{re: `/abc/(`, delim: true, expr: `/abc/`},
		{re: `/abc[/]xyz`, delim: true, err: ParseError{Position: 4}},
		{re: `/abc[\/]xyz`, delim: true, expr: `/abc[\/]xyz`},
	}
	for _, test := range tests {
		if !test.delim {
			test.expr = test.re
		}
		re, err := Compile([]rune(test.re), Options{Delimited: test.delim, Literal: test.literal})
		switch {
		case test.err == nil && err != nil:
			t.Errorf(`Compile("%s")="%v", want nil`, test.re, err)
		case test.err != nil && err == nil:
			t.Errorf(`Compile("%s")=nil, want %v`, test.re, test.err)
		case test.err != nil && err != nil && test.err.(ParseError).Position != err.(ParseError).Position:
			t.Errorf(`Compile("%s")="%v", want "%v"`, test.re, err, test.err)
		}
		if re == nil {
			continue
		}
		if s := string(re.Expression()); s != test.expr {
			t.Errorf(`Compile("%s").Expression()="%s", want "%s"`, test.re, s, test.expr)
		}
	}
}

type regexpTest struct {
	re, str string
	want    []string
	opts    Options
	bol     bool
}

var (
	rev         = Options{Reverse: true}
	lit         = Options{Literal: true}
	del         = Options{Delimited: true}
	regexpTests = []regexpTest{
		// No match.
		{re: "a", str: "", want: nil},
		{re: "a", str: "x", want: nil},
		{re: "a", str: "xyz", want: nil},
		{re: "ba+", str: "b", want: nil},
		{re: "[a]", str: "xyz", want: nil},
		{re: "[^a]", str: "a", want: nil},
		{re: ".", str: "\n", want: nil},

		// Empty match.
		{re: "", str: "", want: []string{""}},
		{re: "a*", str: "x", want: []string{""}},
		{re: "a?", str: "x", want: []string{""}},
		{re: "[a]*", str: "xyz", want: []string{""}},
		{re: "[^a]*", str: "aaa", want: []string{""}},
		{re: "[^a]*", str: "", want: []string{""}},
		{re: ".*", str: "", want: []string{""}},

		{re: "a", str: "a", want: []string{"a"}},
		{re: "☺", str: "☺", want: []string{"☺"}},
		{re: "ab", str: "ab", want: []string{"ab"}},
		{re: "ab", str: "abcdefg", want: []string{"ab"}},
		{re: "a|b", str: "a", want: []string{"a"}},
		{re: "a|b", str: "b", want: []string{"b"}},
		{re: "a*", str: "", want: []string{""}},
		{re: "a*", str: "a", want: []string{"a"}},
		{re: "a*", str: "aaa", want: []string{"aaa"}},
		{re: "a*", str: "aaabcd", want: []string{"aaa"}},
		{re: "☺*", str: "☺☺☹", want: []string{"☺☺"}},
		{re: "ba+", str: "ba", want: []string{"ba"}},
		{re: "ba+", str: "baaaaad", want: []string{"baaaaa"}},
		{re: "ba?d", str: "bd", want: []string{"bd"}},
		{re: "ba?d", str: "bad", want: []string{"bad"}},

		{re: ".*", str: "abcdefg\n", want: []string{"abcdefg"}},
		{re: "a.*", str: "abcdefg\n", want: []string{"abcdefg"}},
		{re: ".*g", str: "abcdefg\n", want: []string{"abcdefg"}},
		{re: "a.*g", str: "abcdefg\n", want: []string{"abcdefg"}},

		{re: "(abc)|(def)", str: "abc", want: []string{"abc", "abc", ""}},
		{re: "(abc)|(def)", str: "abcdef", want: []string{"abc", "abc", ""}},
		{re: "(abc)|(def)", str: "def", want: []string{"def", "", "def"}},
		{re: "(abc)|(def)", str: "defabc", want: []string{"def", "", "def"}},
		{re: "(abc)*", str: "abcabcdef", want: []string{"abcabc", "abc"}},
		{re: "(abc)*|(def)*", str: "abcabcdef", want: []string{"abcabc", "abc", ""}},
		{re: "(abc)*|(def)*", str: "defdefabc", want: []string{"defdef", "", "def"}},
		{re: "(abc|def)*", str: "defdefabc", want: []string{"defdefabc", "abc"}},
		{re: "(abc)d|abce", str: "abce", want: []string{"abce", ""}},
		{re: "abcd|(abc)e", str: "abcd", want: []string{"abcd", ""}},
		{re: "(☺|☹)*", str: "☺☹☺☹☺☹☺", want: []string{"☺☹☺☹☺☹☺", "☺"}},

		{re: `[a]*`, str: `aab`, want: []string{`aa`}},
		{re: `[abc]*`, str: `abcdefg`, want: []string{`abc`}},
		{re: `[a-c]*`, str: `abcdefg`, want: []string{`abc`}},
		{re: `[a-cdef]*`, str: `abcdefg`, want: []string{`abcdef`}},
		{re: `[abcd-f]*`, str: `abcdefg`, want: []string{`abcdef`}},
		{re: `[a-cd-f]*`, str: `abcdefg`, want: []string{`abcdef`}},
		{re: `[a-f]*`, str: `abcdefg`, want: []string{`abcdef`}},
		{re: `[_a-zA-Z0-9]*`, str: "_ident", want: []string{`_ident`}},
		{re: `[*|+?()]*`, str: `*|+?()☺`, want: []string{`*|+?()`}},
		{re: `[\^\-\]]*`, str: `^-]]]^^-☺`, want: []string{`^-]]]^^-`}},
		{re: `[[]*`, str: `[[[☺`, want: []string{`[[[`}},
		{re: `[^d]*`, str: `abcdef`, want: []string{`abc`}},
		{re: `[^d-f]*`, str: `abcef`, want: []string{`abc`}},
		{re: `[^^]*`, str: `a^`, want: []string{`a`}},
		{re: `[^abc]*`, str: "xyz\n", want: []string{`xyz`}},

		{re: "^abc", str: "abc", want: nil},
		{re: "^abc", bol: true, str: "abc", want: []string{"abc"}},
		{re: "abc$", str: "abcxyz", want: nil},
		{re: "abc$", str: "abc", want: []string{"abc"}},
		{re: "abc$", str: "abc\nxyz", want: []string{"abc"}},
		{re: "^abc$", str: "abcxyz", want: nil},
		{re: "^abc$", str: "abc\nxyz", want: nil},
		{re: "^abc$", bol: true, str: "abcxyz", want: nil},
		{re: "^abc$", bol: true, str: "abc\nxyz", want: []string{"abc"}},

		{opts: rev, re: "a", str: "", want: nil},
		{opts: rev, re: "a", str: "x", want: nil},
		{opts: rev, re: "", str: "", want: []string{""}},
		{opts: rev, re: "a", str: "ba", want: []string{"a"}},
		{opts: rev, re: "a*", str: "baaaa", want: []string{"aaaa"}},
		{opts: rev, re: "ba*", str: "baaaa", want: []string{"baaaa"}},
		{opts: rev, re: "(abc|def)*g", str: "defabcg", want: []string{"defabcg", "def"}},

		{opts: rev, re: "abc$", str: "abc", want: nil},
		{opts: rev, re: "abc$", bol: true, str: "abc", want: []string{"abc"}},
		{opts: rev, re: "^abc", str: "xyzabc", want: nil},
		{opts: rev, re: "^abc", str: "abc", want: []string{"abc"}},
		{opts: rev, re: "^abc", str: "xyz\nabc", want: []string{"abc"}},
		{opts: rev, re: "^abc$", str: "xyzabc", want: nil},
		{opts: rev, re: "^abc$", str: "xyz\nabc", want: nil},
		{opts: rev, re: "^abc$", bol: true, str: "xyzabc", want: nil},
		{opts: rev, re: "^abc$", bol: true, str: "xyz\nabc", want: []string{"abc"}},

		{opts: lit, re: "a", str: "", want: nil},
		{opts: lit, re: "a", str: "x", want: nil},
		{opts: lit, re: "", str: "", want: []string{""}},
		{opts: lit, re: "[abc]()*?+.", str: "[abc]()*?+.", want: []string{"[abc]()*?+."}},
		{opts: lit, re: "[abc]()*?+.", str: "[abc]()*?+.☺", want: []string{"[abc]()*?+."}},
		{opts: lit, re: "Hello, 世界", str: "Hello, 世界!!!!", want: []string{"Hello, 世界"}},

		{opts: del, re: "/abc", str: "abc", want: []string{"abc"}},
		{opts: del, re: "/abc/", str: "abc", want: []string{"abc"}},
		{opts: del, re: "/abc/def", str: "abc", want: []string{"abc"}},
		{opts: del, re: `/abc\//def`, str: "abc/", want: []string{"abc/"}},
		{opts: del, re: `/[abc\/]*/def`, str: "abc/", want: []string{"abc/"}},
		{opts: del, re: `/(.*),\n/def`, str: "hi,\n", want: []string{"hi,\n", "hi"}},

		{
			opts: Options{Literal: true, Reverse: true},
			re:   "[abc]()*?+.",
			str:  "☺[abc]()*?+.",
			want: []string{"[abc]()*?+."},
		},

		{
			// Not sure why, but might as well make sure it works.
			opts: Options{Literal: true, Delimited: true},
			re:   "/[abc]()*?+./",
			str:  "[abc]()*?+.",
			want: []string{"[abc]()*?+."},
		},
	}
)

func TestMatch(t *testing.T) {
	for _, test := range regexpTests {
		re, err := Compile([]rune(test.re), test.opts)
		if err != nil {
			t.Fatalf(`Compile("%s", %+v)=%v, want nil`, test.re, test.opts, err)
		}
		str := test.str
		if test.opts.Reverse {
			str = reverse(test.str)
		}
		b := bytes.NewBufferString(str)
		es, err := re.Match(b, test.bol)
		if err != nil {
			t.Fatalf(`Compile("%s", %+v).Match("%s", %t)=%v want nil`,
				test.re, test.opts, test.str, test.bol, err)
		}
		ms := matches(test.str, es, test.opts.Reverse)
		if (es == nil && test.want == nil) ||
			(len(es) == len(test.want) && reflect.DeepEqual(ms, test.want)) {
			continue
		}
		got := "<nil>"
		if es != nil {
			got = fmt.Sprintf("%v (%v)", es, ms)
		}
		want := "<nil>"
		if test.want != nil {
			want = fmt.Sprintf("%v", test.want)
		}
		t.Errorf(`Compile("%s", %+v).Match("%s", %t)=%s, want %s`,
			test.re, test.opts, test.str, test.bol, got, want)
	}
}

func matches(str string, es [][2]int, rev bool) []string {
	rs := []rune(str)
	ss := make([]string, len(es))
	for i, e := range es {
		if e[0] < e[1] && e[0] >= 0 && e[1] <= len(rs) {
			l, u := e[0], e[1]
			if rev {
				l, u = len(rs)-u, len(rs)-l
			}
			ss[i] = string(rs[l:u])
		}
	}
	return ss
}

func reverse(s string) string {
	r := bytes.NewBuffer(nil)
	for i := len(s) - 1; i >= 0; i-- {
		r.WriteByte(s[i])
	}
	return r.String()
}
