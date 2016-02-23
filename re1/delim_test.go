package re1

import "testing"

func TestRemoveDelimiter(t *testing.T) {
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
		{
			name: "A delimiter",
			in:   `A\A123`,
			out:  `\A123`,
		},
		{
			name: "z delimiter",
			in:   `z123\z`,
			out:  `123\z`,
		},
	}
	for _, test := range tests {
		if _, str := RemoveDelimiter(test.in); str != test.out {
			t.Errorf("%s (%q).String()=%q, want %q",
				test.name, test.in, str, test.out)
		}
	}
}

func TestAddDelimiter(t *testing.T) {
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
		{
			name:  "A delimiter",
			in:    `\A123`,
			out:   `A\A123A`,
			delim: 'A',
		},
		{
			name:  "z delimiter",
			in:    `123\z`,
			out:   `z123\zz`,
			delim: 'z',
		},
	}
	for _, test := range tests {
		if str := AddDelimiter(test.delim, test.in); str != test.out {
			t.Errorf("%s (%q).DelimitedString(%q)=%q, want %q",
				test.name, test.in, test.delim, str, test.out)
		}
	}
}
