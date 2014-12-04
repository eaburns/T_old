package re1

import "testing"

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
