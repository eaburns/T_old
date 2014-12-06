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

		{re: "/abc", delim: true, expr: "/abc"},
		{re: "/abc/", delim: true, expr: "/abc/"},
		{re: "/abc/xyz", delim: true, expr: "/abc/"},
		{re: `/abc\/xyz`, delim: true, expr: `/abc\/xyz`},
		{re: `/abc\/xyz/`, delim: true, expr: `/abc\/xyz/`},
		// No error, since we don't parse that far.
		{re: `/abc/(`, delim: true, expr: `/abc/`},
		// Delimiter must still be escaped in character classes.
		// TODO(eaburns): implement character classes.
		// {re: `/abc[/]xyz`, delim: true, expr: `/abc[/`},
		// {re: `/abc[\/]xyz`, delim: true, expr: `/abc[\/]xyz`},
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
		{"", "", []string{""}},
		{"a", "a", []string{"a"}},
		{"ab", "ab", []string{"ab"}},
		{"ab", "abcdefg", []string{"ab"}},
		{"a|b", "a", []string{"a"}},
		{"a|b", "b", []string{"b"}},
		{"a*", "", []string{""}},
		{"a*", "a", []string{"a"}},
		{"a*", "aaa", []string{"aaa"}},
		{"a*", "aaabcd", []string{"aaa"}},
		{"ba+", "b", []string{""}},
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
		for i, s := range ss {
			if s != test.match[i] {
				t.Errorf(`Compile("%s").Match("%s")=%v (%v), want "%v"`,
					test.re, test.str, es, ss, test.match)
				break
			}
		}
	}
}
