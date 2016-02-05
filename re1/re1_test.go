// Copyright © 2015, The T Authors.

package re1

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestNoMatch(t *testing.T) {
	tests := []regexpTest{
		{re: "a", str: "", want: nil},
		{re: "a", str: "x", want: nil},
		{re: "a", str: "xyz", want: nil},
		{re: "ba+", str: "b", want: nil},
		{re: "[a]", str: "xyz", want: nil},
		{re: "[^a]", str: "a", want: nil},
		{re: ".", str: "\n", want: nil},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestEmptyMatch(t *testing.T) {
	tests := []regexpTest{
		{opts: del, re: "", str: "", want: []string{""}},
		{re: "", str: "", want: []string{""}},
		{opts: del, re: "/", str: "", want: []string{""}},
		{re: "a*", str: "x", want: []string{""}},
		{re: "a?", str: "x", want: []string{""}},
		{re: "[a]*", str: "xyz", want: []string{""}},
		{re: "[^a]*", str: "aaa", want: []string{""}},
		{re: "[^a]*", str: "", want: []string{""}},
		{re: ".*", str: "", want: []string{""}},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestSimpleMatch(t *testing.T) {
	tests := []regexpTest{
		{re: "a", str: "a", want: []string{"a"}},
		{re: "☺", str: "☺", want: []string{"☺"}},
		{re: "ab", str: "ab", want: []string{"ab"}},
		{re: "ab", str: "abcdefg", want: []string{"ab"}},
		{re: ".", str: "☺", want: []string{"☺"}},
		{re: `a\?`, str: "", want: nil},
		{re: `a\?`, str: "a?", want: []string{"a?"}},
		{re: `a\*`, str: "aa", want: nil},
		{re: `a\*`, str: "a*", want: []string{"a*"}},
		// This isn't in the spec, but the plan9 code seems to
		// treat a \ at the end of input as a literal.
		{re: `\`, str: `\`, want: []string{`\`}},
		{re: `abc\`, str: `abc\`, want: []string{`abc\`}},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestOrMatch(t *testing.T) {
	tests := []regexpTest{
		{re: "a|b", str: "a", want: []string{"a"}},
		{re: "a|b", str: "b", want: []string{"b"}},
		{re: "a*|a", str: "aaa", want: []string{"aaa"}},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestStarMatch(t *testing.T) {
	tests := []regexpTest{
		{re: "a*", str: "", want: []string{""}},
		{re: "a*", str: "a", want: []string{"a"}},
		{re: "a*", str: "aaa", want: []string{"aaa"}},
		{re: "a*", str: "aaabcd", want: []string{"aaa"}},
		{re: "☺*", str: "☺☺☹", want: []string{"☺☺"}},
		{re: ".*", str: "abcdefg\n", want: []string{"abcdefg"}},
		{re: "a.*", str: "abcdefg\n", want: []string{"abcdefg"}},
		{re: ".*g", str: "abcdefg\n", want: []string{"abcdefg"}},
		{re: "a.*g", str: "abcdefg\n", want: []string{"abcdefg"}},
		{re: "a**", str: "aaabcd", want: []string{"aaa"}},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestPlusMatch(t *testing.T) {
	tests := []regexpTest{
		{re: "ba+", str: "ba", want: []string{"ba"}},
		{re: "ba+", str: "baaaaad", want: []string{"baaaaa"}},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestQuestionMatch(t *testing.T) {
	tests := []regexpTest{
		{re: "ba?d", str: "bd", want: []string{"bd"}},
		{re: "ba?d", str: "bad", want: []string{"bad"}},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestSubexprMatch(t *testing.T) {
	tests := []regexpTest{
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
		{re: "(a(b(c)))", str: "abc", want: []string{"abc", "abc", "bc", "c"}},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestCharClassMatch(t *testing.T) {
	tests := []regexpTest{
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
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestAnchoredMatch(t *testing.T) {
	tests := []regexpTest{
		{re: "^abc", str: "☺abc", from: 1, want: nil},
		{re: "^abc", str: "\nabc", from: 1, want: []string{"abc"}},
		{re: "abc$", str: "abcxyz", want: nil},
		{re: "abc$", str: "abc", want: []string{"abc"}},
		{re: "abc$", str: "abc\nxyz", want: []string{"abc"}},
		{re: "^abc$", str: "☺abcxyz", from: 1, want: nil},
		{re: "^abc$", str: "☺abc\nxyz", from: 1, want: nil},
		{re: "^abc$", str: "\nabcxyz", from: 1, want: nil},
		{re: "^abc$", str: "\nabc\nxyz", from: 1, want: []string{"abc"}},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestReverseMatch(t *testing.T) {
	tests := []regexpTest{
		{opts: rev, re: "a", str: "", want: nil},
		{opts: rev, re: "a", str: "x", want: nil},
		{opts: rev, re: "", str: "", want: []string{""}},
		{opts: rev, re: "a", str: "ba", want: []string{"a"}},
		{opts: rev, re: "a*", str: "baaaa", want: []string{"aaaa"}},
		{opts: rev, re: "ba*", str: "baaaa", want: []string{"baaaa"}},
		{opts: rev, re: "(abc|def)*g", str: "defabcg", want: []string{"defabcg", "def"}},
		{opts: rev, re: "abc$", str: "abc☺", from: 1, want: nil},
		{opts: rev, re: "abc$", str: "abc\n", from: 1, want: []string{"abc"}},
		{opts: rev, re: "^abc", str: "xyzabc", want: nil},
		{opts: rev, re: "^abc", str: "abc", want: []string{"abc"}},
		{opts: rev, re: "^abc", str: "xyz\nabc", want: []string{"abc"}},
		{opts: rev, re: "^abc$", str: "xyzabc", want: nil},
		{opts: rev, re: "^abc$", str: "xyz\nabc☺", from: 1, want: nil},
		{opts: rev, re: "^abc$", str: "xyzabc\n", from: 1, want: nil},
		{opts: rev, re: "^abc$", str: "xyz\nabc\n", from: 1, want: []string{"abc"}},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestLiteralMatch(t *testing.T) {
	tests := []regexpTest{
		{opts: lit, re: "a", str: "", want: nil},
		{opts: lit, re: "a", str: "x", want: nil},
		{opts: lit, re: "", str: "", want: []string{""}},
		{opts: lit, re: "[abc]()*?+.", str: "[abc]()*?+.", want: []string{"[abc]()*?+."}},
		{opts: lit, re: "[abc]()*?+.", str: "[abc]()*?+.☺", want: []string{"[abc]()*?+."}},
		{opts: lit, re: "Hello, 世界", str: "Hello, 世界!!!!", want: []string{"Hello, 世界"}},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestDelimitedMatch(t *testing.T) {
	tests := []regexpTest{
		{opts: del, re: "/abc", str: "abc", want: []string{"abc"}},
		{opts: del, re: "/abc/", str: "abc", want: []string{"abc"}},
		{opts: del, re: "/abc/def", str: "abc", want: []string{"abc"}},
		{opts: del, re: `/abc\//def`, str: "abc/", want: []string{"abc/"}},
		{opts: del, re: `/[abc\/]*/def`, str: "abc/", want: []string{"abc/"}},
		{opts: del, re: `/(.*),\n/def`, str: "hi,\n", want: []string{"hi,\n", "hi"}},
		{opts: del, re: `?abc?`, str: "ab", want: nil},
		{opts: del, re: `?abc?`, str: "abc", want: []string{"abc"}},

		// Test escaped meta delimiters:

		{opts: del, re: `.\.+`, str: "abc", want: []string{"abc"}},
		{opts: del, re: `.\.+.(would be error`, str: "abc", want: []string{"abc"}},
		{opts: del, re: `.xyz\.+`, str: "xyzabc", want: []string{"xyzabc"}},

		{opts: del, re: `*a\*`, str: "", want: []string{""}},
		{opts: del, re: `*a\*`, str: "aaabc", want: []string{"aaa"}},
		{opts: del, re: `*a\**(would be error`, str: "aa", want: []string{"aa"}},
		{opts: del, re: `*a\**`, str: "*", want: []string{""}},
		{opts: del, re: `*a\**`, str: "a*", want: []string{"a"}},

		{opts: del, re: `+a\+`, str: "aa", want: []string{"aa"}},
		{opts: del, re: `+a\+`, str: "aaabc", want: []string{"aaa"}},
		{opts: del, re: `+a\++(would be error`, str: "aa", want: []string{"aa"}},
		{opts: del, re: `+a\++`, str: "a+", want: []string{"a"}},

		{opts: del, re: `?a\?`, str: "", want: []string{""}},
		{opts: del, re: `?a\?`, str: "a?", want: []string{"a"}},
		{opts: del, re: `?a\??(would be error`, str: "a", want: []string{"a"}},
		{opts: del, re: `?a\??`, str: "a?", want: []string{"a"}},

		{opts: del, re: `[\[1-5]*`, str: "[1-9", want: []string{""}},
		{opts: del, re: `[\[1-5]*`, str: "12345", want: []string{"12345"}},
		{opts: del, re: `[\[1-5]*[(would be error`, str: "1", want: []string{"1"}},
		{opts: del, re: `[abc\[[]`, str: "abc[", want: []string{"abc["}},

		{opts: del, re: `][1-5\]*`, str: "[1-5", want: []string{""}},
		{opts: del, re: `][1-5\]*`, str: "12345", want: []string{"12345"}},
		{opts: del, re: `][1-5\]*](would be error`, str: "1", want: []string{"1"}},
		{opts: del, re: `]abc[[\]`, str: "abc[", want: []string{"abc["}},

		{opts: del, re: `(\(abc)`, str: "abc", want: []string{"abc", "abc"}},
		{opts: del, re: `(\(abc)([would be error`, str: "abc", want: []string{"abc", "abc"}},
		{opts: del, re: `(\(a\(b))`, str: "ab", want: []string{"ab", "ab", "b"}},

		{opts: del, re: `)(abc\)`, str: "abc", want: []string{"abc", "abc"}},
		{opts: del, re: `)(abc\))(would be error`, str: "abc", want: []string{"abc", "abc"}},
		{opts: del, re: `)(a(b\)\)`, str: "ab", want: []string{"ab", "ab", "b"}},

		{opts: del, re: `|a\|b`, str: "a", want: []string{"a"}},
		{opts: del, re: `|a\|b`, str: "b", want: []string{"b"}},
		{opts: del, re: `|(a)\|(b)`, str: "a", want: []string{"a", "a", ""}},
		{opts: del, re: `|(a)\|(b)`, str: "b", want: []string{"b", "", "b"}},
		{opts: del, re: `|a\|b|(would be error`, str: "b", want: []string{"b"}},

		{opts: del, re: `^\^abc`, str: "abc", want: []string{"abc"}},
		{opts: del, re: `^\^abc`, str: "xyz\nabc", want: []string{"abc"}},
		{opts: del, re: `^\^abc`, str: "^abc", want: nil},
		{opts: del, re: `^\^a^(would be error`, str: "b\na", want: []string{"a"}},

		{opts: del, re: `$abc\$`, str: "abc", want: []string{"abc"}},
		{opts: del, re: `$abc\$`, str: "abc\nxyz", want: []string{"abc"}},
		{opts: del, re: `$abc\$`, str: "abc$", want: nil},
		{opts: del, re: `$a\$$(would be error`, str: "a\nb", want: []string{"a"}},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestMultiOptionMatch(t *testing.T) {
	tests := []regexpTest{
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
	for _, test := range tests {
		test.run(t)
	}
}

func TestNextMatch(t *testing.T) {
	tests := []regexpTest{
		{re: "abc", str: "xyzabc", want: []string{"abc"}},
		{re: "abc", str: "xyzabc", from: 1, want: []string{"abc"}},
		{re: "abc", str: "xyzabc", from: 2, want: []string{"abc"}},
		{re: "abc(d*)", str: "xyzabcdd", from: 2, want: []string{"abcdd", "dd"}},
		{re: "^abc|def$", str: "☺abc\ndef", from: 1, want: []string{"def"}},
		{opts: rev, re: "abc", str: "abcdef", from: 1, want: []string{"abc"}},
		{re: "a*", str: "xyzbc", want: []string{""}},
		// Match the empty string at the beginning, not the later matches.
		{re: "a*", str: "xyzabc", want: []string{""}},
		{re: "a*", str: "xyzaaabc", want: []string{""}},
		{re: "a**", str: "xyzaaabcd", want: []string{""}},
		{re: ".*", str: "\n\naa", want: []string{""}},
		{re: ".*", str: "\n\naa\n", want: []string{""}},
		{re: "a+", str: "xyzbc", want: nil},
		{re: "a+", str: "xyzabc", want: []string{"a"}},
		{re: "a+", str: "xyzaaabc", want: []string{"aaa"}},
		{re: ".+", str: "\n\n\n", want: nil},
		{re: ".+", str: "\n\naa", want: []string{"aa"}},
		{re: ".+", str: "\n\naa\n", want: []string{"aa"}},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestWrapMatch(t *testing.T) {
	tests := []regexpTest{
		{re: "abc", str: "abcxyz", from: 3, want: []string{"abc"}},
		{re: "abc", str: "abcxyz", from: 6, want: []string{"abc"}},
		{re: "abc", str: "xyzabc", from: 4, want: []string{"abc"}},
		{re: "abc(d*)", str: "xyzabcdd", from: 4, want: []string{"abcdd", "dd"}},
		{re: "^abc|def$", str: "☺abc\ndef", from: 7, want: []string{"def"}},
		{opts: rev, re: "abc", str: "abcdef", from: 5, want: []string{"abc"}},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestReuse(t *testing.T) {
	re, err := Compile(strings.NewReader("(a)(b)(c)|(x)(y)(z)"), Options{})
	if err != nil {
		t.Fatalf(`Compile("(a)(b)(c)|(x)(y)(z)")=%v, want nil`, err)
	}
	str := "abc"
	want := []string{"abc", "a", "b", "c", "", "", ""}
	got := matches(str, re.Match(sliceRunes([]rune(str)), 0), false)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf(`Compile("(a)(b)(c)|(x)(y)(z)").Match(%s)=%v want %v, nil`, str, got, want)
	}
	// This will get different subexpression matches.
	// Make sure that there isn't old data from the previous match.
	str = "xyz"
	want = []string{"xyz", "", "", "", "x", "y", "z"}
	got = matches(str, re.Match(sliceRunes([]rune(str)), 0), false)
	if !reflect.DeepEqual(got, want) {
		t.Errorf(`Compile("(a)(b)(c)|(x)(y)(z)").Match(%s)=%v, want %v,vil`, str, got, want)
	}
}

type sliceRunes []rune

func (s sliceRunes) Rune(i int64) rune { return s[i] }
func (s sliceRunes) Size() int64       { return int64(len(s)) }

var (
	rev = Options{Reverse: true}
	lit = Options{Literal: true}
	del = Options{Delimited: true}
)

type regexpTest struct {
	re, str string
	want    []string
	opts    Options
	from    int64
}

func (test *regexpTest) run(t *testing.T) {
	name := test.re
	re := test.run1(name, t)
	if re == nil {
		return
	}

	test.re = re.String()
	test.opts.Delimited = false
	test.run1(name+" from String()", t)

	for _, d := range "/" + Meta {
		if d == '\\' || d == ']' {
			continue
		}
		test.re = re.DelimitedString(d)
		test.opts.Delimited = true
		test.run1(name+" from DelimitedString("+string(d)+")", t)
	}
}

func (test *regexpTest) run1(name string, t *testing.T) *Regexp {
	re, err := Compile(strings.NewReader(test.re), test.opts)
	if err != nil {
		t.Fatalf(`%s Compile("%s", %+v)=%v, want nil`, name, test.re, test.opts, err)
		return nil
	}
	str := test.str
	if test.opts.Reverse {
		str = reverse(test.str)
	}
	es := re.Match(sliceRunes([]rune(str)), test.from)
	ms := matches(test.str, es, test.opts.Reverse)
	if es == nil && test.want == nil ||
		len(es) == len(test.want) && reflect.DeepEqual(ms, test.want) {
		return re
	}
	got := "<nil>"
	if es != nil {
		got = fmt.Sprintf("%v (%v)", es, ms)
	}
	want := "<nil>"
	if test.want != nil {
		want = fmt.Sprintf("%v", test.want)
	}
	t.Errorf(`%s Compile("%s", %+v).Match("%s", %d)=%v,%v, want %s`,
		name, test.re, test.opts, test.str, test.from, got, err, want)
	return re
}

func matches(str string, es [][2]int64, rev bool) []string {
	if es == nil {
		return nil
	}
	rs := []rune(str)
	ss := make([]string, len(es))
	for i, e := range es {
		if n := int64(len(rs)); e[0] < e[1] && e[0] >= 0 && e[1] <= n {
			l, u := e[0], e[1]
			if rev {
				l, u = n-u, n-l
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

func TestParseErrors(t *testing.T) {
	tests := []struct {
		re string
		// What the compiler actually used.
		// If delim==false, initialized to re.
		delim, literal bool
		err            string
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
		{re: "*", err: "missing operand"},
		{delim: true, re: "/*", err: "missing operand"},
		{re: "+", err: "missing operand"},
		{delim: true, re: "/+", err: "missing operand"},
		{re: "?", err: "missing operand"},
		{delim: true, re: "/?", err: "missing operand"},
		{re: "a(bcd", err: "unclosed"},
		{re: "a(b(c)d", err: "unclosed"},
		{re: "a(b(cd", err: "unclosed"},
		{delim: true, re: "/(abc/", err: "unclosed"},
		{delim: true, re: "/(abc/)", err: "unclosed"},
		{delim: true, re: "/(/", err: "unclosed"},
		{re: "a|", err: "missing operand"},
		//{re: "a)", err: ParseError{Position: 1}},
		//{re: "a)xyz", err: ParseError{Position: 1}},
		{re: "()xyz", err: "missing operand"},
		// Acme allows this, treating ] as a literal ']'.
		// We don't allow it. The man page is a bit unclear,
		// but I think it intends to say that metacharacters
		// must be escaped to be literals. Otherwise, how
		// does one distinguish?
		//{re: "a]", err: ParseError{Position: 1}},
		//{re: "a]xyz", err: ParseError{Position: 1}},

		// Character classes.
		{re: `[]`, err: "missing operand"},
		{re: `[`, err: "unclosed"},
		{re: `[a-`, err: "incomplete range"},
		{re: `[\`, err: "unclosed"},
		{re: `[-]`, err: "range incomplete"},
		{re: `[b-a`, err: "not ascending"},
		{re: `[^`, err: "unclosed"},
		{re: `[^a-`, err: "incomplete range"},
		{re: `[^b-a`, err: "not ascending"},
		{re: `[xyz`, err: "unclosed"},
		{re: `[xyza-`, err: "incomplete range"},
		{re: `[xyzb-a`, err: "not ascending"},
		{re: `[^xyz`, err: "unclosed"},
		{re: `[^xyza-`, err: "incomplete range"},
		{re: `[^xyzb-a`, err: "not ascending"},
		{re: `[a]`},
		{re: `[^a]`},
		{re: `[abc]`},
		{re: `[^abc]`},
		{re: `[a-zA-Z0-9_]`},
		{re: `[^a-zA-Z0-9_]`},
		{re: `[\^\-\]]`},

		// Delimiters.
		{delim: true, re: `\abc`, err: "bad delimiter"},
		{delim: true, re: "/abc"},
		{delim: true, re: "/abc/"},
		{delim: true, re: "/abc/xyz"},
		{delim: true, re: `/abc\/xyz`},
		{delim: true, re: `/abc\/xyz/`},
		{delim: true, re: `/abc/(`}, // No error, we hit the delimiter first.
		{delim: true, re: `/abc[/]xyz`},
		{delim: true, re: `/abc[\/]xyz`},

		{delim: true, re: `][\]]`, err: "missing operand"},
		{delim: true, re: `][]\]]`, err: "missing operand"},
	}
	for _, test := range tests {
		re, err := Compile(strings.NewReader(test.re), Options{Delimited: test.delim, Literal: test.literal})
		switch {
		case test.err == "" && err != nil:
			t.Errorf(`Compile("%s")="%v", want nil`, test.re, err)
		case test.err != "" && err == nil:
			t.Errorf(`Compile("%s")=nil, want %v`, test.re, test.err)
		case test.err != "" && err != nil && !regexp.MustCompile(test.err).MatchString(err.Error()):
			t.Errorf(`Compile("%s")=%q, want matching %q`, test.re, err, test.err)
		}
		if re == nil {
			continue
		}
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		name    string
		in, out string
	}{
		{
			name: "empty",
			in:   "",
			out:  "",
		},
		{
			name: "no ending delimiter",
			in:   "/abc",
			out:  "abc",
		},
		{
			name: "ending delimiter",
			in:   "/abc/",
			out:  "abc",
		},
		{
			name: "escaped delimiter",
			in:   `/ab\/c`,
			out:  `ab/c`,
		},
		{
			name: "escaped delimiter at start",
			in:   `/\/abc`,
			out:  `/abc`,
		},
		{
			name: "escaped delimiter at end",
			in:   `/abc\/`,
			out:  `abc/`,
		},
		{
			name: "charclass delimiter",
			in:   `/ab[/]c`,
			out:  `ab[/]c`,
		},
		{
			name: "escaped and charclass delimiter",
			in:   `/a\/b[/]c/`,
			out:  `a/b[/]c`,
		},
		{
			name: "non-ASCII delimiter",
			in:   `☺abc☺`,
			out:  `abc`,
		},
		{
			name: "escaped and charclass non-ASCII delimiter",
			in:   `☺a\☺b[☺]c☺`,
			out:  `a☺b[☺]c`,
		},
		{
			name: "[ in charclass",
			in:   `/abc[[/]`,
			out:  `abc[[/]`,
		},
		{
			name: "escaped ] in charclass",
			in:   `/abc[/\]]`,
			out:  `abc[/\]]`,
		},
		{
			name: "trailing escape",
			in:   `/abc\`,
			out:  `abc\`,
		},
	}
	for _, test := range tests {
		rs := strings.NewReader(test.in)
		re, err := Compile(rs, Options{Delimited: true})
		if err != nil {
			t.Errorf("%s re1.Compile(%q, Options{Delimited: true}) %v",
				test.name, test.in, err)
		} else if str := re.String(); str != test.out {
			t.Errorf("%s (%q).String()=%q, want %q",
				test.name, test.in, str, test.out)
		}
	}
}

func TestDelimitedString(t *testing.T) {
	tests := []struct {
		name    string
		in, out string
		delim   rune
	}{
		{
			name:  "empty",
			in:    "",
			delim: '/',
			out:   "//",
		},
		{
			name:  "simple",
			in:    `abc`,
			delim: '/',
			out:   `/abc/`,
		},
		{
			name:  "escape delimiter",
			in:    `ab/c`,
			delim: '/',
			out:   `/ab\/c/`,
		},
		{
			name:  "already escaped delimiter",
			in:    `ab\/c`,
			delim: '/',
			out:   `/ab\/c/`,
		},
		{
			name:  "charclass delimiter",
			in:    `ab[/]c`,
			delim: '/',
			out:   `/ab[/]c/`,
		},
		{
			name:  "escape meta delimiter",
			in:    `abc*`,
			delim: '*',
			out:   `*abc\**`,
		},
		{
			name:  "already escaped meta delimiter",
			in:    `a\*`,
			delim: '*',
			out:   `*a[*]*`,
		},
		{
			name:  "charclass meta delimiter",
			in:    `abc\*`,
			delim: '*',
			out:   `*abc[*]*`,
		},
		{
			name:  "obrace delimiter",
			in:    `a[xyz]b`,
			delim: '[',
			out:   `[a\[xyz]b[`,
		},
		{
			name:  "obrace delimiter add charclass",
			in:    `a\[b`,
			delim: '[',
			out:   `[a[[]b[`,
		},
		{
			name:  "trailing escape",
			in:    `abc\`,
			delim: '/',
			out:   `/abc\\/`,
		},
		{
			name:  "only escape",
			in:    `\`,
			delim: '/',
			out:   `/\\/`,
		},
		{
			name:  "escape in charclass",
			in:    `[\]]`,
			delim: '/',
			out:   `/[\]]/`,
		},
		{
			name:  "escaped delim in charclass",
			in:    `[\/]`,
			delim: '/',
			out:   `/[\/]/`,
		},
		{
			name:  "double escape",
			in:    `\\[\\]`,
			delim: '/',
			out:   `/\\[\\]/`,
		},
		{
			name:  "double escape before delimiter",
			in:    `\\/`,
			delim: '/',
			out:   `/\\\//`,
		},
	}
	for _, test := range tests {
		rs := strings.NewReader(test.in)
		re, err := Compile(rs, Options{})
		if err != nil {
			t.Errorf("%s re1.Compile(%q, Options{}) %v", test.name, test.in, err)
		} else if str := re.DelimitedString(test.delim); str != test.out {
			t.Errorf("%s (%q).DelimitedString(%q)=%q, want %q",
				test.name, test.in, test.delim, str, test.out)
		}
	}
}
