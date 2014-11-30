package re1

import "testing"

// TestJustParse just tests parse errors (or lack thereof).
func TestJustParse(t *testing.T) {
	tests := []struct {
		re    string
		delim bool
		n     int
		err   error
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

		{re: "/abc", delim: true, n: 4},
		{re: "/abc/", delim: true, n: 5},
		{re: "/abc/xyz", delim: true, n: 5},
		{re: `/abc\/xyz`, delim: true, n: 9},
		{re: `/abc\/xyz/`, delim: true, n: 10},
		// No error, since we don't parse that far.
		{re: `/abc/(`, delim: true, n: 5},
		// Delimiter must still be escaped in character classes.
		// TODO(eaburns): implement character classes.
		// {re: `/abc[/]xyz`, delim: true, n: 5},
		// {re: `/abc[\/]xyz`, delim: true, n: 11},
	}
	for _, test := range tests {
		re := []rune(test.re)
		var n int
		var err error
		if test.delim {
			_, n, err = CompileDelim(re)
		} else {
			_, err = Compile(re)
		}
		switch {
		case test.err == nil && err != nil || n != test.n:
			t.Errorf(`Compile("%s")=%d,"%v", want %d,nil`, test.re, n, err, test.n)
		case test.err != nil && err == nil:
			t.Errorf(`Compile("%s")=%d,nil, want %v`, test.re, n, test.err)
		case test.err != nil && err != nil && test.err.(ParseError).Position != err.(ParseError).Position:
			t.Errorf(`Compile("%s")=%d,"%v", want %d,"%v"`, test.re, n, err, test.n, test.err)
		}
	}
}
