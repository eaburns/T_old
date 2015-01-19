package re1

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
)

func TestExpression(t *testing.T) {
	tests := []struct {
		re, want string
		opts     Options
	}{
		{"abc", "abc", Options{}},
		{"/abc", "/abc", Options{Delimited: true}},
		{"/abc/", "/abc/", Options{Delimited: true}},
		{"/", "/", Options{Delimited: true}},
		{"//", "//", Options{Delimited: true}},
		{"", "", Options{Delimited: true}},
	}
	for _, test := range tests {
		re, err := Compile([]rune(test.re), test.opts)
		if err != nil {
			t.Errorf("Compile(%s, %v) had error %v", test.re, test.opts, err)
			continue
		}
		if s := string(re.Expression()); s != test.want {
			t.Errorf("Compile(%s, %v).Expression()=%s, want %s",
				test.re, test.opts, s, test.want)
		}
	}
}

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
		{opts: del, re: `*a\*`, str: "", want: []string{""}},
		{opts: del, re: `*a\*`, str: "aaa", want: []string{"aaa"}},
		{opts: del, re: `*a\**`, str: "aaa", want: []string{"aaa"}},
		{opts: del, re: `?a\?`, str: "", want: []string{""}},
		{opts: del, re: `?a\?`, str: "a", want: []string{"a"}},
		{opts: del, re: `?a\??`, str: "a", want: []string{"a"}},
		{opts: del, re: `|a\|b`, str: "a", want: []string{"a"}},
		{opts: del, re: `|a\|b`, str: "b", want: []string{"b"}},
		{opts: del, re: `|a\|b|`, str: "b", want: []string{"b"}},
		{opts: del, re: `[\[a-z]+`, str: "abc", want: []string{"abc"}},
		{opts: del, re: `[\[a-z]+[`, str: "abc", want: []string{"abc"}},
		{opts: del, re: `$abc\$`, str: "abcdef", want: nil},
		{opts: del, re: `$abc\$`, str: "abc\ndef", want: []string{"abc"}},
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
	re, err := Compile([]rune("(a)(b)(c)|(x)(y)(z)"), Options{})
	if err != nil {
		t.Fatalf(`Compile("(a)(b)(c)|(x)(y)(z)")=%v, want nil`, err)
	}
	str := "abc"
	want := []string{"abc", "a", "b", "c", "", "", ""}
	ms, err := re.Match(sliceRunes([]rune(str)), 0)
	got := matches(str, ms, false)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf(`Compile("(a)(b)(c)|(x)(y)(z)").Match(%s)=%v,%v want %v, nil`, str, got, err, want)
	}
	// This will get different subexpression matches.
	// Make sure that there isn't old data from the previous match.
	str = "xyz"
	want = []string{"xyz", "", "", "", "x", "y", "z"}
	ms, err = re.Match(sliceRunes([]rune(str)), 0)
	got = matches(str, ms, false)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf(`Compile("(a)(b)(c)|(x)(y)(z)").Match(%s)=%v,%v, want %v,vil`, str, got, err, want)
	}
}

type sliceRunes []rune

func (s sliceRunes) Rune(i int64) (rune, error) { return s[i], nil }
func (s sliceRunes) Size() int64                { return int64(len(s)) }

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
	re, err := Compile([]rune(test.re), test.opts)
	if err != nil {
		t.Fatalf(`Compile("%s", %+v)=%v, want nil`, test.re, test.opts, err)
	}
	str := test.str
	if test.opts.Reverse {
		str = reverse(test.str)
	}
	es, err := re.Match(sliceRunes([]rune(str)), test.from)
	if err != nil {
	}
	ms := matches(test.str, es, test.opts.Reverse)
	if err == nil && (es == nil && test.want == nil ||
		len(es) == len(test.want) && reflect.DeepEqual(ms, test.want)) {
		return
	}
	got := "<nil>"
	if es != nil {
		got = fmt.Sprintf("%v (%v)", es, ms)
	}
	want := "<nil>"
	if test.want != nil {
		want = fmt.Sprintf("%v", test.want)
	}
	t.Errorf(`Compile("%s", %+v).Match("%s", %d)=%v,%v, want %s`,
		test.re, test.opts, test.str, test.from, got, err, want)
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
		str            string
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
		{re: `[-]`, err: ParseError{Position: 1}},
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
		{delim: true, re: "/abc", str: "/abc"},
		{delim: true, re: "/abc/", str: "/abc/"},
		{delim: true, re: "/abc/xyz", str: "/abc/"},
		{delim: true, re: `/abc\/xyz`, str: `/abc\/xyz`},
		{delim: true, re: `/abc\/xyz/`, str: `/abc\/xyz/`},
		{delim: true, re: `/abc/(`, str: `/abc/`}, // No error, we hit the delimiter first.
		{delim: true, re: `/abc[/]xyz`, str: `/abc[/]xyz`},
		{delim: true, re: `/abc[\/]xyz`, str: `/abc[\/]xyz`},

		// It's impossible to close a charclass if the delimiter is ].
		{delim: true, re: `][\]\]`, str: "]", err: ParseError{Position: 1}},
		{delim: true, re: `][]\]]`, str: "]", err: ParseError{Position: 1}},
	}
	for _, test := range tests {
		if !test.delim {
			test.str = test.re
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
		if s := string(re.Expression()); s != test.str {
			t.Errorf(`Compile("%s").Expression()="%s", want "%s"`, test.re, s, test.str)
		}
	}
}
