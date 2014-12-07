package re1

import (
	"bytes"
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

func TestMatch(t *testing.T) {
	tests := []struct {
		re, str string
		match   []string
	}{
		// No match.
		{"a", "", nil},
		{"a", "x", nil},
		{"a", "xyz", nil},
		{"ba+", "b", nil},
		{"[a]", "xyz", nil},
		{"[^a]", "a", nil},

		// Empty match.
		{"", "", []string{""}},
		{"a*", "x", []string{""}},
		{"a?", "x", []string{""}},
		{"[a]*", "xyz", []string{""}},
		{"[^a]*", "aaa", []string{""}},
		{"[^a]*", "", []string{""}},

		{"a", "a", []string{"a"}},
		{"ab", "ab", []string{"ab"}},
		{"ab", "abcdefg", []string{"ab"}},
		{"a|b", "a", []string{"a"}},
		{"a|b", "b", []string{"b"}},
		{"a*", "", []string{""}},
		{"a*", "a", []string{"a"}},
		{"a*", "aaa", []string{"aaa"}},
		{"a*", "aaabcd", []string{"aaa"}},
		{"ba+", "ba", []string{"ba"}},
		{"ba+", "baaaaad", []string{"baaaaa"}},
		{"ba?d", "bd", []string{"bd"}},
		{"ba?d", "bad", []string{"bad"}},
		{"(abc)|(def)", "abc", []string{"abc", "abc", ""}},
		{"(abc)|(def)", "abcdef", []string{"abc", "abc", ""}},
		{"(abc)|(def)", "def", []string{"def", "", "def"}},
		{"(abc)|(def)", "defabc", []string{"def", "", "def"}},
		{"(abc)*", "abcabcdef", []string{"abcabc", "abc"}},
		{"(abc)*|(def)*", "abcabcdef", []string{"abcabc", "abc", ""}},
		{"(abc)*|(def)*", "defdefabc", []string{"defdef", "", "def"}},
		{"(abc|def)*", "defdefabc", []string{"defdefabc", "abc"}},

		{`[a]*`, `aab`, []string{`aa`}},
		{`[abc]*`, `abcdefg`, []string{`abc`}},
		{`[a-c]*`, `abcdefg`, []string{`abc`}},
		{`[a-cdef]*`, `abcdefg`, []string{`abcdef`}},
		{`[abcd-f]*`, `abcdefg`, []string{`abcdef`}},
		{`[a-cd-f]*`, `abcdefg`, []string{`abcdef`}},
		{`[a-f]*`, `abcdefg`, []string{`abcdef`}},
		{`[*|+?()]*`, `*|+?()END`, []string{`*|+?()`}},
		{`[\^\-\]]*`, `^-]]]^^-END`, []string{`^-]]]^^-`}},
		{`[[]*`, `[[[END`, []string{`[[[`}},
		{`[^d]*`, `abcdef`, []string{`abc`}},
		{`[^d-f]*`, `abcef`, []string{`abc`}},
		{`[^^]*`, `a^`, []string{`a`}},
		{`[^abc]*`, "xyz\n", []string{`xyz`}},
	}
	for _, test := range tests {
		re, err := Compile([]rune(test.re), Options{})
		if err != nil {
			t.Fatalf(`Compile("%s")=%v, want nil`, test.re, err)
		}
		b := bytes.NewBufferString(test.str)
		es, err := re.Match(b, false)
		if err != nil {
			t.Fatalf(`Compile("%s").Match("%s")=%v want nil`, test.re, test.str, err)
		}

		rs := []rune(test.str)
		ss := make([]string, len(es))
		for i, e := range es {
			if e[0] < e[1] && e[0] >= 0 && e[1] <= len(rs) {
				ss[i] = string(rs[e[0]:e[1]])
			}
		}
		if len(test.match) != len(es) {
			t.Errorf(`Compile("%s").Match("%s")=%v (len=%d), want %v (len=%d)`,
				test.re, test.str, es, len(es), test.match, len(test.match))
			continue
		}
		for i, s := range ss {
			if s != test.match[i] {
				t.Errorf(`Compile("%s").Match("%s")=%v (%v), want "%v"`,
					test.re, test.str, es, ss, test.match)
				break
			}
		}
	}
}
