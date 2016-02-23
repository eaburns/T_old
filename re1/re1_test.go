// Copyright © 2016, The T Authors.

package re1

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func TestFlagsString(t *testing.T) {
	tests := []struct {
		flags Flags
		str   string
	}{
		{0, "0"},
		{Delimited, "Delimited"},
		{Literal, "Literal"},
		{Reverse, "Reverse"},
		{Delimited | Literal, "Delimited|Literal"},
		{Delimited | Reverse, "Delimited|Reverse"},
		{Literal | Reverse, "Literal|Reverse"},
		{Delimited | Literal | Reverse, "Delimited|Literal|Reverse"},
		{0x80, "0x80"},
		{Delimited | 0x80, "Delimited|0x80"},
	}
	for _, test := range tests {
		if test.flags.String() != test.str {
			t.Errorf("(%d).String()=%s, want %s", test.flags, test.flags, test.str)
		}
	}
}

func TestReuse(t *testing.T) {
	re, err := Compile(strings.NewReader("(a)(bc)"))
	if err != nil {
		t.Fatalf(err.Error())
	}
	m0 := re.Match(None, strings.NewReader("abc"), None)
	m1 := re.Match(None, strings.NewReader("abc"), None)
	if !reflect.DeepEqual(m0, m1) {
		t.Fatalf("m0=%v, m1=%v, wanted equal", m0, m1)
	}

	prefix := "αβξ"
	m2 := re.Match(None, strings.NewReader(prefix+"abc"), None)
	for i := range m2 {
		m2[i][0] -= int64(len(prefix))
		m2[i][1] -= int64(len(prefix))
	}
	if !reflect.DeepEqual(m0, m2) {
		t.Fatalf("m0=%v, m2=%v, wanted equal", m0, m2)
	}
}

func TestEmpty(t *testing.T) {
	tests := []regexpTest{
		{
			name:   "empty regexp",
			regexp: "",
			text: []string{
				"{00}",
				"{00}a",
				"{00}abcdef",
				"{00}αβξ",
			},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestLiteral(t *testing.T) {
	tests := []regexpTest{
		{
			name:   "single rune",
			regexp: "a",
			text: []string{
				"",
				"x",
				"{0}a{0}",
				"{0}a{0}def",
				"{0}a{0}aaa",
				"xyz{0}a{0}bc",
				"zzz{0}a{0}aa",
				"αβξ{0}a{0}αβξ",
			},
		},
		{
			name:   "non-ASCII",
			regexp: "☺",
			text: []string{
				"",
				"x",
				"{0}☺{0}",
				"{0}☺{0}def",
				"{0}☺{0}aaa",
				"xyz{0}☺{0}bc",
				"zzz{0}☺{0}aa",
				"αβξ{0}☺{0}αβξ",
			},
		},
		{
			name:   "newline",
			regexp: "\n",
			text: []string{
				"",
				"x",
				`\n`,
				"{0}\n{0}",
			},
		},
		{
			name:   "literal newline",
			regexp: `\n`,
			text: []string{
				"",
				"x",
				`\n`,
				"{0}\n{0}",
			},
		},
		{name: `escaped meta .`, regexp: `\.`, text: []string{`{0}.{0}`}},
		{name: `escaped meta *`, regexp: `\*`, text: []string{`{0}*{0}`}},
		{name: `escaped meta +`, regexp: `\+`, text: []string{`{0}+{0}`}},
		{name: `escaped meta ?`, regexp: `\?`, text: []string{`{0}?{0}`}},
		{name: `escaped meta [`, regexp: `\[`, text: []string{`{0}[{0}`}},
		{name: `escaped meta ]`, regexp: `\]`, text: []string{`{0}]{0}`}},
		{name: `escaped meta (`, regexp: `\(`, text: []string{`{0}({0}`}},
		{name: `escaped meta )`, regexp: `\)`, text: []string{`{0}){0}`}},
		{name: `escaped meta |`, regexp: `\|`, text: []string{`{0}|{0}`}},
		{name: `escaped meta \`, regexp: `\\`, text: []string{`{0}\{0}`}},
		{name: `escaped meta ^`, regexp: `\^`, text: []string{`{0}^{0}`}},
		{name: `escaped meta $`, regexp: `\$`, text: []string{`{0}${0}`}},
		{name: `literal .`, regexp: `.`, flags: Literal, text: []string{`{0}.{0}`}},
		{name: `literal *`, regexp: `*`, flags: Literal, text: []string{`{0}*{0}`}},
		{name: `literal +`, regexp: `+`, flags: Literal, text: []string{`{0}+{0}`}},
		{name: `literal ?`, regexp: `?`, flags: Literal, text: []string{`{0}?{0}`}},
		{name: `literal [`, regexp: `[`, flags: Literal, text: []string{`{0}[{0}`}},
		{name: `literal ]`, regexp: `]`, flags: Literal, text: []string{`{0}]{0}`}},
		{name: `literal (`, regexp: `(`, flags: Literal, text: []string{`{0}({0}`}},
		{name: `literal )`, regexp: `)`, flags: Literal, text: []string{`{0}){0}`}},
		{name: `literal |`, regexp: `|`, flags: Literal, text: []string{`{0}|{0}`}},
		{name: `literal \`, regexp: `\`, flags: Literal, text: []string{`{0}\{0}`}},
		{name: `literal ^`, regexp: `^`, flags: Literal, text: []string{`{0}^{0}`}},
		{name: `literal $`, regexp: `$`, flags: Literal, text: []string{`{0}${0}`}},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestCharClass(t *testing.T) {
	tests := []regexpTest{
		{name: "unclosed", regexp: "[", error: "unclosed"},
		{name: "unclosed with runes", regexp: "[abc", error: "unclosed"},
		{name: "unclosed with escape", regexp: `[\`, error: "unclosed"},
		{name: "unopened", regexp: "abc]", error: "unopened"},
		{name: "incomplete range", regexp: "[a-]", error: "incomplete"},
		{name: "incomplete range EOF", regexp: "[a-", error: "incomplete"},
		{name: "incomplete range no start", regexp: "[-", error: "incomplete"},
		{name: "non-ascending", regexp: "[z-a]", error: "not ascending"},
		{name: "missing operand", regexp: "[]", error: "missing operand"},
		{
			name:   "missing operand meta ] delim",
			regexp: `][\]`,
			flags:  Delimited,
			error:  "missing operand",
		},
		{
			name:   "list",
			regexp: "[abc]",
			text: []string{
				"",
				"d",
				"A",
				"α",
				"{0}a{0}",
				"{0}b{0}",
				"{0}c{0}",
				"{0}a{0}a",
				"αβξ{0}a{0}a",
			},
		},
		{
			name:   "range",
			regexp: "[b-c]",
			text: []string{
				"",
				"a",
				"d",
				"α",
				"{0}b{0}",
				"{0}c{0}",
				"{0}b{0}bbb",
				"αβξ{0}b{0}x",
			},
		},
		{
			name:   "multiple ranges",
			regexp: "[b-cB-C2-3]",
			text: []string{
				"",
				"a",
				"d",
				"A",
				"D",
				"1",
				"4",
				"α",
				"{0}b{0}",
				"{0}c{0}",
				"{0}B{0}",
				"{0}C{0}",
				"{0}2{0}",
				"{0}3{0}",
				"{0}b{0}bbb",
				"αβξ{0}b{0}x",
			},
		},
		{
			name:   "lists and ranges",
			regexp: "[bcB-C23X-Y]",
			text: []string{
				"",
				"a",
				"d",
				"A",
				"D",
				"1",
				"4",
				"W",
				"Z",
				"{0}b{0}",
				"{0}c{0}",
				"{0}B{0}",
				"{0}C{0}",
				"{0}2{0}",
				"{0}3{0}",
				"{0}X{0}",
				"{0}Y{0}",
				"{0}b{0}bbb",
				"αβξ{0}b{0}x",
			},
		},
		{
			name:   "negated",
			regexp: "[^bcB-C]",
			text: []string{
				"",
				"\n",
				"b",
				"c",
				"B",
				"C",
				"{0}a{0}",
				"{0}d{0}",
				"{0}A{0}",
				"{0}D{0}",
				"{0}^{0}",
			},
		},
		{
			name:   "negated ^",
			regexp: "[^^]",
			text: []string{
				"",
				"\n",
				"^",
				"{0}a{0}",
				"{0}d{0}",
				"{0}A{0}",
				"{0}D{0}",
			},
		},
		{
			name:   "meta",
			regexp: `[.*+?[\]()|\\^$]`,
			text: []string{
				"{0}.{0}",
				"{0}*{0}",
				"{0}+{0}",
				"{0}?{0}",
				"{0}[{0}",
				"{0}]{0}",
				"{0}({0}",
				"{0}){0}",
				"{0}|{0}",
				`{0}\{0}`,
				"{0}^{0}",
				"{0}${0}",
			},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestDot(t *testing.T) {
	tests := []regexpTest{
		{
			name:   "list",
			regexp: ".",
			text: []string{
				"",
				"\n",
				"{0}A{0}",
				"{0}\x00{0}",
				"{0}☺{0}",
			},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestBeginLineAnchor(t *testing.T) {
	tests := []regexpTest{
		{
			name:   "no prev",
			regexp: "^abc",
			text: []string{
				"",
				"ab",
				"xyz",
				"{0}abc{0}",
				"☺abc",
				"{0}abc{0}☺",
				"☺abc☺",
				"☺\n{0}abc{0}",
				"{0}abc{0}\n☺",
				"☺\n{0}abc{0}\n☺",
			},
		},
		{
			name:   "newline prev",
			regexp: "^abc",
			prev:   "\n",
			text: []string{
				"",
				"ab",
				"xyz",
				"{0}abc{0}",
				"☺abc",
				"{0}abc{0}☺",
				"☺abc☺",
				"☺\n{0}abc{0}",
				"{0}abc{0}\n☺",
				"☺\n{0}abc{0}\n☺",
			},
		},
		{
			name:   "non-newline prev",
			regexp: "^abc",
			prev:   "x",
			text: []string{
				"",
				"ab",
				"xyz",
				"abc",
				"☺abc",
				"abc☺",
				"☺abc☺",
				"☺\n{0}abc{0}",
				"abc\n☺",
				"☺\n{0}abc{0}\n☺",
			},
		},
		{
			name:   "reverse no next",
			regexp: "^cba",
			flags:  Reverse,
			text: []string{
				"",
				"ab",
				"xyz",
				"{0}abc{0}",
				"☺{0}abc{0}",
				"abc☺",
				"☺abc☺",
				"☺\n{0}abc{0}",
				"{0}abc{0}\n☺",
				"☺\n{0}abc{0}\n☺",
			},
		},
		{
			name:   "reverse newline next",
			regexp: "^cba",
			flags:  Reverse,
			next:   "\n",
			text: []string{
				"",
				"ab",
				"xyz",
				"{0}abc{0}",
				"☺{0}abc{0}",
				"abc☺",
				"☺abc☺",
				"☺\n{0}abc{0}",
				"{0}abc{0}\n☺",
				"☺\n{0}abc{0}\n☺",
			},
		},
		{
			name:   "reverse non-newline next",
			regexp: "^cba",
			flags:  Reverse,
			next:   "x",
			text: []string{
				"",
				"ab",
				"xyz",
				"abc",
				"☺abc",
				"abc☺",
				"☺abc☺",
				"☺\nabc",
				"{0}abc{0}\n☺",
				"☺\n{0}abc{0}\n☺",
			},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestEndLineAnchor(t *testing.T) {
	tests := []regexpTest{
		{
			name:   "no next",
			regexp: "abc$",
			text: []string{
				"",
				"ab",
				"xyz",
				"{0}abc{0}",
				"☺{0}abc{0}",
				"abc☺",
				"☺abc☺",
				"☺\n{0}abc{0}",
				"{0}abc{0}\n☺",
				"☺\n{0}abc{0}\n☺",
			},
		},
		{
			name:   "newline next",
			regexp: "abc$",
			next:   "\n",
			text: []string{
				"",
				"ab",
				"xyz",
				"{0}abc{0}",
				"☺{0}abc{0}",
				"abc☺",
				"☺abc☺",
				"☺\n{0}abc{0}",
				"{0}abc{0}\n☺",
				"☺\n{0}abc{0}\n☺",
			},
		},
		{
			name:   "non-newline next",
			regexp: "abc$",
			next:   "x",
			text: []string{
				"",
				"ab",
				"xyz",
				"abc",
				"☺abc",
				"abc☺",
				"☺abc☺",
				"☺\nabc",
				"{0}abc{0}\n☺",
				"☺\n{0}abc{0}\n☺",
			},
		},
		{
			name:   "reverse no prev",
			regexp: "cba$",
			flags:  Reverse,
			text: []string{
				"",
				"ab",
				"xyz",
				"{0}abc{0}",
				"☺abc",
				"{0}abc{0}☺",
				"☺abc☺",
				"☺\n{0}abc{0}",
				"{0}abc{0}\n☺",
				"☺\n{0}abc{0}\n☺",
			},
		},
		{
			name:   "reverse newline prev",
			regexp: "cba$",
			flags:  Reverse,
			prev:   "\n",
			text: []string{
				"",
				"ab",
				"xyz",
				"{0}abc{0}",
				"☺abc",
				"{0}abc{0}☺",
				"☺abc☺",
				"☺\n{0}abc{0}",
				"{0}abc{0}\n☺",
				"☺\n{0}abc{0}\n☺",
			},
		},
		{
			name:   "reverse non-newline prev",
			regexp: "cba$",
			flags:  Reverse,
			prev:   "x",
			text: []string{
				"",
				"ab",
				"xyz",
				"abc",
				"☺abc",
				"abc☺",
				"☺abc☺",
				"☺\n{0}abc{0}",
				"abc\n☺",
				"☺\n{0}abc{0}\n���",
			},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestBeginTextAnchor(t *testing.T) {
	tests := []regexpTest{
		{
			name:   "no prev",
			regexp: `\Aabc`,
			text: []string{
				"",
				"ab",
				"xyz",
				"{0}abc{0}",
				"☺abc",
				"{0}abc{0}☺",
				"☺abc☺",
				"☺\nabc",
				"{0}abc{0}\n☺",
				"☺\nabc\n☺",
			},
		},
		{
			name:   "newline prev",
			regexp: `\Aabc`,
			prev:   "\n",
			text: []string{
				"",
				"ab",
				"xyz",
				"abc",
				"☺abc",
				"abc☺",
				"☺abc☺",
				"☺\nabc",
				"abc\n☺",
				"☺\nabc\n☺",
			},
		},
		{
			name:   "non-newline prev",
			regexp: `\Aabc`,
			prev:   "x",
			text: []string{
				"",
				"ab",
				"xyz",
				"abc",
				"☺abc",
				"abc☺",
				"☺abc☺",
				"☺\nabc",
				"abc\n☺",
				"☺\nabc\n☺",
			},
		},
		{
			name:   "reverse no next",
			regexp: `\Acba`,
			flags:  Reverse,
			text: []string{
				"",
				"ab",
				"xyz",
				"{0}abc{0}",
				"☺{0}abc{0}",
				"abc☺",
				"☺abc☺",
				"☺\n{0}abc{0}",
				"abc\n☺",
				"☺\nabc\n☺",
			},
		},
		{
			name:   "reverse newline next",
			regexp: `\Acba`,
			flags:  Reverse,
			next:   "\n",
			text: []string{
				"",
				"ab",
				"xyz",
				"abc",
				"☺abc",
				"abc☺",
				"☺abc☺",
				"☺\nabc",
				"abc\n☺",
				"☺\nabc\n☺",
			},
		},
		{
			name:   "reverse non-newline next",
			regexp: `\Acba`,
			flags:  Reverse,
			next:   "x",
			text: []string{
				"",
				"ab",
				"xyz",
				"abc",
				"☺abc",
				"abc☺",
				"☺abc☺",
				"☺\nabc",
				"abc\n☺",
				"☺\nabc\n☺",
			},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestEndTextAnchor(t *testing.T) {
	tests := []regexpTest{
		{
			name:   "no next",
			regexp: `abc\z`,
			text: []string{
				"",
				"ab",
				"xyz",
				"{0}abc{0}",
				"☺{0}abc{0}",
				"abc☺",
				"☺abc☺",
				"☺\n{0}abc{0}",
				"abc\n☺",
				"☺\nabc\n☺",
			},
		},
		{
			name:   "newline next",
			regexp: `abc\z`,
			next:   "\n",
			text: []string{
				"",
				"ab",
				"xyz",
				"abc",
				"☺abc",
				"abc☺",
				"☺abc☺",
				"☺\nabc",
				"abc\n☺",
				"☺\nabc\n☺",
			},
		},
		{
			name:   "non-newline next",
			regexp: `abc\z`,
			next:   "x",
			text: []string{
				"",
				"ab",
				"xyz",
				"abc",
				"☺abc",
				"abc☺",
				"☺abc☺",
				"☺\nabc",
				"abc\n☺",
				"☺\nabc\n☺",
			},
		},
		{
			name:   "reverse no prev",
			regexp: `cba\z`,
			flags:  Reverse,
			text: []string{
				"",
				"ab",
				"xyz",
				"{0}abc{0}",
				"☺abc",
				"{0}abc{0}☺",
				"☺abc☺",
				"☺\nabc",
				"{0}abc{0}\n☺",
				"☺\nabc\n☺",
			},
		},
		{
			name:   "reverse newline prev",
			regexp: `cba\z`,
			flags:  Reverse,
			prev:   "\n",
			text: []string{
				"",
				"ab",
				"xyz",
				"abc",
				"☺abc",
				"abc☺",
				"☺abc☺",
				"☺\nabc",
				"abc\n☺",
				"☺\nabc\n☺",
			},
		},
		{
			name:   "reverse non-newline prev",
			regexp: `cba\z`,
			flags:  Reverse,
			prev:   "x",
			text: []string{
				"",
				"ab",
				"xyz",
				"abc",
				"☺abc",
				"abc☺",
				"☺abc☺",
				"☺\nabc",
				"abc\n☺",
				"☺\nabc\n☺",
			},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestWordBoundary(t *testing.T) {
	tests := []regexpTest{
		{
			name:   "next=None, prev=None",
			regexp: `\babc\b`,
			text: []string{
				"",
				"{0}abc{0}",
				"\t{0}abc{0}",
				"{0}abc{0}\t",
				"\t{0}abc{0}\t",
				"xabc",
				"abcx",
				"xabcx",
				"xabc\t",
				"\tabcx",
			},
		},
		{
			name:   "next=None, prev=word",
			regexp: `\babc\b`,
			prev:   "x",
			text: []string{
				"",
				"abc",
				"\t{0}abc{0}",
				"abc\t",
				"\t{0}abc{0}\t",
				"xabc",
				"abcx",
				"xabcx",
				"xabc\t",
				"\tabcx",
			},
		},
		{
			name:   "next=None, prev=non-word",
			regexp: `\babc\b`,
			prev:   "\t",
			text: []string{
				"",
				"{0}abc{0}",
				"\t{0}abc{0}",
				"{0}abc{0}\t",
				"\t{0}abc{0}\t",
				"xabc",
				"abcx",
				"xabcx",
				"xabc\t",
				"\tabcx",
			},
		},
		{
			name:   "next=word, prev=None",
			regexp: `\babc\b`,
			next:   "x",
			text: []string{
				"",
				"abc",
				"\tabc",
				"{0}abc{0}\t",
				"\t{0}abc{0}\t",
				"xabc",
				"abcx",
				"xabcx",
				"xabc\t",
				"\tabcx",
			},
		},
		{
			name:   "next=word, prev=word",
			regexp: `\babc\b`,
			next:   "x",
			prev:   "x",
			text: []string{
				"",
				"abc",
				"\tabc",
				"abc\t",
				"\t{0}abc{0}\t",
				"xabc",
				"abcx",
				"xabcx",
				"xabc\t",
				"\tabcx",
			},
		},
		{
			name:   "next=word, prev=non-word",
			regexp: `\babc\b`,
			next:   "x",
			prev:   "\t",
			text: []string{
				"",
				"abc",
				"\tabc",
				"{0}abc{0}\t",
				"\t{0}abc{0}\t",
				"xabc",
				"abcx",
				"xabcx",
				"xabc\t",
				"\tabcx",
			},
		},
		{
			name:   "next=non-word, prev=None",
			regexp: `\babc\b`,
			next:   "\t",
			text: []string{
				"",
				"{0}abc{0}",
				"\t{0}abc{0}",
				"{0}abc{0}\t",
				"\t{0}abc{0}\t",
				"xabc",
				"abcx",
				"xabcx",
				"xabc\t",
				"\tabcx",
			},
		},
		{
			name:   "next=non-word, prev=word",
			regexp: `\babc\b`,
			next:   "\t",
			prev:   "x",
			text: []string{
				"",
				"abc",
				"\t{0}abc{0}",
				"abc\t",
				"\t{0}abc{0}\t",
				"xabc",
				"abcx",
				"xabcx",
				"xabc\t",
				"\tabcx",
			},
		},
		{
			name:   "next=non-word, prev=non-word",
			regexp: `\babc\b`,
			next:   "\t",
			prev:   "\t",
			text: []string{
				"",
				"{0}abc{0}",
				"\t{0}abc{0}",
				"{0}abc{0}\t",
				"\t{0}abc{0}\t",
				"xabc",
				"abcx",
				"xabcx",
				"xabc\t",
				"\tabcx",
			},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestNotWordBoundary(t *testing.T) {
	tests := []regexpTest{
		{
			name:   "next=None, prev=None",
			regexp: `\Babc\B`,
			text: []string{
				"",
				"abc",
				"_abc",
				"abc_",
				"_{0}abc{0}_",
				"_abc\t",
				"\tabc_",
				"\tabc",
				"abc\t",
				"\tabc\t",
			},
		},
		{
			name:   "next=None, prev=word",
			regexp: `\Babc\B`,
			prev:   "x",
			text: []string{
				"",
				"abc",
				"_abc",
				"{0}abc{0}_",
				"_{0}abc{0}_",
				"_abc\t",
				"\tabc_",
				"\tabc",
				"abc\t",
				"\tabc\t",
			},
		},
		{
			name:   "next=None, prev=non-word",
			regexp: `\Babc\B`,
			prev:   "\t",
			text: []string{
				"",
				"abc",
				"_abc",
				"abc_",
				"_{0}abc{0}_",
				"_abc\t",
				"\tabc_",
				"\tabc",
				"abc\t",
				"\tabc\t",
			},
		},
		{
			name:   "next=word, prev=None",
			regexp: `\Babc\B`,
			next:   "x",
			text: []string{
				"",
				"abc",
				"_{0}abc{0}",
				"abc_",
				"_{0}abc{0}_",
				"_abc\t",
				"\tabc_",
				"\tabc",
				"abc\t",
				"\tabc\t",
			},
		},
		{
			name:   "next=word, prev=word",
			regexp: `\Babc\B`,
			next:   "x",
			prev:   "x",
			text: []string{
				"",
				"{0}abc{0}",
				"_{0}abc{0}",
				"{0}abc{0}_",
				"_{0}abc{0}_",
				"_abc\t",
				"\tabc_",
				"\tabc",
				"abc\t",
				"\tabc\t",
			},
		},
		{
			name:   "next=word, prev=non-word",
			regexp: `\Babc\B`,
			next:   "x",
			prev:   "\t",
			text: []string{
				"",
				"abc",
				"_{0}abc{0}",
				"abc_",
				"_{0}abc{0}_",
				"_abc\t",
				"\tabc_",
				"\tabc",
				"abc\t",
				"\tabc\t",
			},
		},
		{
			name:   "next=non-word, prev=None",
			regexp: `\Babc\B`,
			next:   "\t",
			text: []string{
				"",
				"abc",
				"_abc",
				"abc_",
				"_{0}abc{0}_",
				"_abc\t",
				"\tabc_",
				"\tabc",
				"abc\t",
				"\tabc\t",
			},
		},
		{
			name:   "next=non-word, prev=word",
			regexp: `\Babc\B`,
			next:   "\t",
			prev:   "x",
			text: []string{
				"",
				"abc",
				"_abc",
				"{0}abc{0}_",
				"_{0}abc{0}_",
				"_abc\t",
				"\tabc_",
				"\tabc",
				"abc\t",
				"\tabc\t",
			},
		},
		{
			name:   "next=non-word, prev=non-word",
			regexp: `\Babc\B`,
			next:   "\t",
			prev:   "\t",
			text: []string{
				"",
				"abc",
				"_abc",
				"abc_",
				"_{0}abc{0}_",
				"_abc\t",
				"\tabc_",
				"\tabc",
				"abc\t",
				"\tabc\t",
			},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestPerlClasses(t *testing.T) {
	tests := []regexpTest{
		{
			name:   `\d`,
			regexp: `\d`,
			text: []string{
				"",
				"a",
				"{0}0{0}",
				"{0}1{0}",
				"{0}2{0}",
				"{0}3{0}",
				"{0}4{0}",
				"{0}5{0}",
				"{0}6{0}",
				"{0}7{0}",
				"{0}8{0}",
				"{0}9{0}",
				"{0}٠{0}", // ARABIC-INDIC DIGIT ZERO
				"abc{0}1{0}",
			},
		},
		{
			name:   `\D`,
			regexp: `\D`,
			text: []string{
				"",
				"0",
				"1",
				"2",
				"3",
				"4",
				"5",
				"6",
				"7",
				"8",
				"9",
				"٠", // ARABIC-INDIC DIGIT ZERO
				"{0}a{0}",
				"{0}α{0}",
				"{0}☺{0}",
				"{0} {0}", // space
				"{0}	{0}", // tab
				"{0}\n{0}",
				"123{0}a{0}",
			},
		},
		{
			name:   `\s`,
			regexp: `\s`,
			text: []string{
				"",
				"a",
				"☺",
				"{0}\t{0}",
				"{0}\n{0}",
				"{0}\v{0}",
				"{0}\f{0}",
				"{0}\r{0}",
				"{0} {0}",      // space
				"{0}\u0085{0}", // NEL
				"{0}\u00A0{0}", // NBSP
				"xyz{0}\t{0}",
			},
		},
		{
			name:   `\S`,
			regexp: `\S`,
			text: []string{
				"",
				"\t",
				"\n",
				"\v",
				"\f",
				"\r",
				" ",      // space
				"\u0085", // NEL
				"\u00A0", // NBSP
				"{0}a{0}",
				"{0}☺{0}",
				"\t\t\n {0}α{0}",
			},
		},
		{
			name:   `\w`,
			regexp: `\w`,
			text: []string{
				"",
				"\t",
				"☺",
				"-",
				",",
				"{0}a{0}",
				"{0}α{0}",
				"{0}1{0}",
				"{0}٠{0}", // ARABIC-INDIC DIGIT ZERO
				"{0}_{0}",
			},
		},
		{
			name:   `\W`,
			regexp: `\W`,
			text: []string{
				"",
				"a",
				"α",
				"1",
				"٠", // ARABIC-INDIC DIGIT ZERO
				"_",
				"{0}\t{0}",
				"{0}☺{0}",
				"{0}-{0}",
				"{0},{0}",
			},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestGroup(t *testing.T) {
	tests := []regexpTest{
		{name: "missing operand", regexp: "()", error: "missing operand"},
		{name: "unclosed", regexp: "(abc", error: "unclosed"},
		{name: "unopened", regexp: "abc)", error: "unopened"},
		{name: "nested error", regexp: "(((*)))", error: "missing operand"},
		{
			name:   "group",
			regexp: "(abc)",
			text: []string{
				"",
				"ab",
				"αβξ{01}abc{01}",
				"ab{01}abc{01}",
				"{01}abc{01}abc",
				"αβξ{01}abc{01}abc",
			},
		},
		{
			name:   "same nested groups",
			regexp: "(((abc)))",
			text: []string{
				"",
				"ab",
				"αβξ{0123}abc{0123}",
				"ab{0123}abc{0123}",
				"{0123}abc{0123}abc",
				"αβξ{0123}abc{0123}abc",
			},
		},
		{
			name:   "nested groups",
			regexp: "((a)((b)(c)))",
			text: []string{
				"",
				"ab",
				"αβξ{012}a{234}b{45}c{0135}",
				"ab{012}a{234}b{45}c{0135}",
				"{012}a{234}b{45}c{0135}abc",
				"αβξ{012}a{234}b{45}c{0135}abc",
			},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestStar(t *testing.T) {
	aStarText := []string{
		"{00}",
		"{00}b",
		"{0}a{0}",
		"{0}aaaaaa{0}",
		"{0}aaaaaa{0}bcd",
		"{0}aaaaaa{0}baaaa",
		"{00}xyzaaaa",
	}
	tests := []regexpTest{
		{
			name:   "missing operand",
			regexp: "*",
			error:  "missing operand",
		},
		{
			name:   "precedence",
			regexp: "abc*",
			text: []string{
				// It applies just to c, not abc.
				"{0}ab{0}",
				"{0}abc{0}abc",
				"{0}abcccc{0}abc",
			},
		},
		{
			name:   "literal rune",
			regexp: "a*",
			text:   aStarText,
		},
		{
			name:   "charclass",
			regexp: "[a-z]*",
			text: []string{
				"{00}",
				"{00}123",
				"{0}a{0}",
				"{0}abcdefg{0}",
				"{0}abcdefg{0}123",
				"{0}abcdefg{0}123abc",
				"{00}123abc",
			},
		},
		{
			name:   "dot",
			regexp: ".*",
			text: []string{
				"{00}",
				"{00}\n",
				"{0}α{0}",
				"{0}\x00\x00{0}",
				"{0}abcdefg{0}",
				"{0}abcdefg{0}\n",
				"{0}abcdefg{0}\nabc",
				"{00}\nabc",
			},
		},
		{
			name:   "group",
			regexp: "(abc)*",
			text: []string{
				"{00}",
				"{00}α",
				"{00}ab",
				"{0}abcabcabcabc{1}abc{01}",
				"{01}abc{01}ab",
				"{01}abc{01}xyzabcabc",
				"{00}xyzabcabc",
			},
		},
		{
			name:   "star star",
			regexp: "a****",
			text:   aStarText,
		},
		{
			name:   "nested beginning star",
			regexp: "(a*bc)*",
			text: []string{
				"{00}",
				"{00}xyz",
				"{00}aaaaa",
				"{00}ab",
				"{00}ac",
				"{01}bc{01}",
				"{0}bcbc{1}bc{01}",
				"{01}aaabc{01}",
				"{0}abcbcbc{1}aaabc{01}",
				"{0}abcbc{1}bc{01}xyz",
				"{0}abcbc{1}bc{01}ab",
			},
		},
		{
			name:   "nested middle star",
			regexp: "(ab*c)*",
			text: []string{
				"{00}",
				"{00}xyz",
				"{00}ab",
				"{00}bc",
				"{01}ac{01}",
				"{0}acac{1}ac{01}",
				"{01}abbbc{01}",
				"{0}abcacac{1}abbbc{01}",
				"{0}abcac{1}ac{01}xyz",
				"{0}abcac{1}ac{01}ab",
			},
		},
		{
			name:   "nested end star",
			regexp: "(abc*)*",
			text: []string{
				"{00}",
				"{00}xyz",
				"{00}ac",
				"{00}bc",
				"{01}ab{01}",
				"{0}abab{1}ab{01}",
				"{01}abccc{01}",
				"{0}abcabab{1}abccc{01}",
				"{0}abcab{1}ab{01}xyz",
				"{0}abcab{1}ab{01}ac",
			},
		},
		{
			name:   "inside capturing group",
			regexp: "(a*)",
			text: []string{
				"{01}aaaaa{01}",
			},
		},
		{
			name:   "outside capturing group",
			regexp: "(a)*",
			text: []string{
				"{0}aaaa{1}a{01}"},
		},
		{
			name:   "capturing groups",
			regexp: "(ab)*(c)*",
			text: []string{
				"{00}",
				"{01}ab{12}c{02}",
				"{01}ab{01}",
				"{0}abab{1}ab{01}",
				"{02}c{02}",
				"{0}cccc{2}c{02}",
			},
		},
		{
			name:   "nested capturing groups",
			regexp: "((a)*(b)(c)*)*",
			text: []string{
				"{00}",
				"{012}a{23}b{34}c{014}",
				"{0}aaabccccabcabc{1}aa{2}a{23}b{3}cc{4}c{014}xyz",
				"{0}abcabcbc{13}b{34}c{014}",
			},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestPlus(t *testing.T) {
	aPlusText := []string{
		"",
		"b",
		"{0}a{0}",
		"{0}aaaaaa{0}",
		"{0}aaaaaa{0}bcd",
		"{0}aaaaaa{0}baaaa",
		"xyz{0}aaaa{0}",
	}
	tests := []regexpTest{
		{
			name:   "missing operand",
			regexp: "+",
			error:  "missing operand",
		},
		{
			name:   "precedence",
			regexp: "abc+",
			text: []string{
				// It applies just to c, not abc.
				"ab",
				"{0}abc{0}abc",
				"{0}abcccc{0}abc",
			},
		},
		{
			name:   "literal rune",
			regexp: "a+",
			text:   aPlusText,
		},
		{
			name:   "charclass",
			regexp: "[a-z]+",
			text: []string{
				"",
				"123",
				"{0}a{0}",
				"{0}abcdefg{0}",
				"{0}abcdefg{0}123",
				"{0}abcdefg{0}123abc",
				"123{0}abc{0}",
			},
		},
		{
			name:   "dot",
			regexp: ".+",
			text: []string{
				"",
				"\n",
				"{0}α{0}",
				"{0}\x00\x00{0}",
				"{0}abcdefg{0}",
				"{0}abcdefg{0}\n",
				"{0}abcdefg{0}\nabc",
				"\n{0}abc{0}",
			},
		},
		{
			name:   "group",
			regexp: "(abc)+",
			text: []string{
				"",
				"α",
				"ab",
				"{0}abcabcabcabc{1}abc{01}",
				"{01}abc{01}ab",
				"{01}abc{01}xyzabcabc",
				"xyz{0}abc{1}abc{01}",
			},
		},
		{
			name:   "plus plus",
			regexp: "a++++",
			text:   aPlusText,
		},
		{
			name:   "nested beginning plus",
			regexp: "(a+bc)+",
			text: []string{
				"",
				"xyz",
				"aaaaa",
				"ab",
				"ac",
				"bc",
				"{01}abc{01}",
				"{01}aaabc{01}",
				"abbbc",
				"{01}abc{01}cc",
				"{01}abc{01}bcbc",
				"{0}abcabc{1}aaabc{01}xyz",
				"{0}abcabc{1}aaabc{01}bc",
				"bc{0}abcabc{1}aaabc{01}bc",
			},
		},
		{
			name:   "nested middle plus",
			regexp: "(ab+c)+",
			text: []string{
				"",
				"xyz",
				"abbbb",
				"ab",
				"ac",
				"bc",
				"{01}abc{01}",
				"aa{01}abc{01}",
				"{01}abbbc{01}",
				"{01}abc{01}cc",
				"{01}abc{01}acac",
				"{0}abcabc{1}abbbc{01}xyz",
				"{0}abcabc{1}abbbc{01}ac",
				"ac{0}abcabc{1}abbbc{01}ac",
			},
		},
		{
			name:   "nested end plus",
			regexp: "(abc+)+",
			text: []string{
				"",
				"xyz",
				"aaaaa",
				"ab",
				"ac",
				"bc",
				"{01}abc{01}",
				"aa{01}abc{01}",
				"abbbc",
				"{01}abccc{01}",
				"{01}abc{01}bcbc",
				"{0}abcabc{1}abccc{01}xyz",
				"{0}abcabc{1}abccc{01}ab",
				"am{0}abcabc{1}abccc{01}ab",
			},
		},
		{
			name:   "inside capturing group",
			regexp: "(a+)",
			text: []string{
				"{01}aaaaa{01}",
			},
		},
		{
			name:   "outside capturing group",
			regexp: "(a)+",
			text: []string{
				"{0}aaaa{1}a{01}"},
		},
		{
			name:   "capturing groups",
			regexp: "(ab)+(c)+",
			text: []string{
				"",
				"{01}ab{12}c{02}",
				"ab",
				"ababab",
				"c",
				"ccccc",
			},
		},
		{
			name:   "nested capturing groups",
			regexp: "((a)+(b)(c)+)+",
			text: []string{
				"",
				"{012}a{23}b{34}c{014}",
				"{0}aaabccccabcabc{1}aa{2}a{23}b{3}cc{4}c{014}xyz",
			},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestQuestion(t *testing.T) {
	aQuestionText := []string{
		"{00}",
		"{00}b",
		"{0}a{0}",
		"{0}a{0}aaaaa",
		"{00}ba",
	}
	tests := []regexpTest{
		{
			name:   "missing operand",
			regexp: "?",
			error:  "missing operand",
		},
		{
			name:   "precedence",
			regexp: "abc?",
			text: []string{
				// It applies just to c, not abc.
				"{0}ab{0}",
				"{0}abc{0}abc",
				"{0}abc{0}cccabc",
			},
		},
		{
			name:   "literal rune",
			regexp: "a?",
			text:   aQuestionText,
		},
		{
			name:   "charclass",
			regexp: "[a-z]?",
			text: []string{
				"{00}",
				"{00}123",
				"{0}a{0}",
				"{0}a{0}bcdefg",
				"{00}123abc",
			},
		},
		{
			name:   "dot",
			regexp: ".?",
			text: []string{
				"{00}",
				"{00}\n",
				"{0}α{0}",
				"{0}\x00{0}\x00",
				"{0}a{0}bcdefg",
				"{00}\nabc",
			},
		},
		{
			name:   "group",
			regexp: "(abc)?",
			text: []string{
				"{00}",
				"{00}α",
				"{00}ab",
				"{01}abc{01}abcabcabc",
				"{00}xyzabc",
			},
		},
		{
			name:   "question question",
			regexp: "a????",
			text:   aQuestionText,
		},
		{
			name:   "nested beginning question",
			regexp: "(a?bc)?",
			text: []string{
				"{00}",
				"{00}xyz",
				"{00}aaaaa",
				"{00}ab",
				"{00}ac",
				"{01}bc{01}",
				"{01}abc{01}",
				"{00}xabc",
			},
		},
		{
			name:   "nested middle question",
			regexp: "(ab?c)?",
			text: []string{
				"{00}",
				"{00}xyz",
				"{00}aaaaa",
				"{00}ab",
				"{01}ac{01}",
				"{00}bc",
				"{01}abc{01}",
				"{00}xabc",
			},
		},
		{
			name:   "nested end question",
			regexp: "(abc?)?",
			text: []string{
				"{00}",
				"{00}xyz",
				"{00}aaaaa",
				"{01}ab{01}",
				"{00}ac",
				"{00}bc",
				"{01}abc{01}",
				"{00}xabc",
			},
		},
		{
			name:   "capturing groups",
			regexp: "(ab)?(c)?",
			text: []string{
				"{00}",
				"{01}ab{12}c{02}",
				"{01}ab{01}",
				"{01}ab{01}abab",
				"{02}c{02}",
				"{02}c{02}cccc",
			},
		},
		{
			name:   "nested capturing groups",
			regexp: "((a)?(b)(c)?)?",
			text: []string{
				"{00}",
				"{012}a{23}b{34}c{014}",
				"{013}b{34}c{014}",
				"{012}a{23}b{013}",
			},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestAThenB(t *testing.T) {
	tests := []regexpTest{
		{name: "left operand error", regexp: "()a", error: "missing operand"},
		{name: "right operand error", regexp: "a()", error: "missing operand"},
		{
			name:   "literal literal",
			regexp: "ab",
			text: []string{
				"",
				"a",
				"ac",
				"cb",
				"{0}ab{0}",
				"12{0}ab{0}yz",
			},
		},
		{
			name:   "literal charclass",
			regexp: "a[123]",
			text: []string{
				"",
				"a",
				"ac",
				"c1",
				"{0}a1{0}",
				"12{0}a1{0}yz",
			},
		},
		{
			name:   "literal dot",
			regexp: "a.",
			text: []string{
				"",
				"a",
				"a\n",
				"z1",
				"{0}a1{0}",
				"12{0}a1{0}yz",
			},
		},
		{
			name:   "literal group",
			regexp: "a(b)",
			text: []string{
				"",
				"a",
				"ac",
				"cb",
				"{0}a{1}b{01}",
				"12{0}a{1}b{01}yz",
			},
		},
		{
			name:   "charclass literal",
			regexp: "[abc]1",
			text: []string{
				"",
				"a",
				"a2",
				"d1",
				"{0}a1{0}",
				"12{0}a1{0}yz",
			},
		},
		{
			name:   "charclass charclass",
			regexp: "[abc][123]",
			text: []string{
				"",
				"a",
				"a4",
				"d1",
				"{0}a1{0}",
				"12{0}a1{0}yz",
			},
		},
		{
			name:   "charclass dot",
			regexp: "[abc].",
			text: []string{
				"",
				"a",
				"a\n",
				"z1",
				"{0}a1{0}",
				"12{0}a1{0}yz",
			},
		},
		{
			name:   "charclass group",
			regexp: "[abc](1)",
			text: []string{
				"",
				"a",
				"a2",
				"d1",
				"{0}a{1}1{01}",
				"12{0}a{1}1{01}yz",
			},
		},
		{
			name:   "dot literal",
			regexp: ".a",
			text: []string{
				"",
				"a",
				"\na",
				"1b",
				"{0}1a{0}",
				"12{0}1a{0}yz",
			},
		},
		{
			name:   "dot charclass",
			regexp: ".[abc]",
			text: []string{
				"",
				"a",
				"\na",
				"1d",
				"{0}1a{0}",
				"12{0}1a{0}yz",
			},
		},
		{
			name:   "dot dot",
			regexp: "..",
			text: []string{
				"",
				"a",
				"\na",
				"a\n",
				"{0}αβ{0}",
				"\n\n{0}αβ{0}\n\n",
			},
		},
		{
			name:   "dot group",
			regexp: ".(a)",
			text: []string{
				"",
				"a",
				"\na",
				"1b",
				"{0}1{1}a{01}",
				"12{0}1{1}a{01}yz",
			},
		},
		{
			name:   "group literal",
			regexp: "(a)b",
			text: []string{
				"",
				"a",
				"ac",
				"cb",
				"{01}a{1}b{0}",
				"12{01}a{1}b{0}yz",
			},
		},
		{
			name:   "group charclass",
			regexp: "(a)[123]",
			text: []string{
				"",
				"a",
				"ac",
				"c1",
				"{01}a{1}1{0}",
				"12{01}a{1}1{0}yz",
			},
		},
		{
			name:   "group dot",
			regexp: "(a).",
			text: []string{
				"",
				"a",
				"a\n",
				"z1",
				"{01}a{1}1{0}",
				"12{01}a{1}1{0}yz",
			},
		},
		{
			name:   "group group",
			regexp: "(a)(b)",
			text: []string{
				"",
				"a",
				"ac",
				"cb",
				"{01}a{12}b{02}",
				"12{01}a{12}b{02}yz",
			},
		},
		{
			name:   "non-star star",
			regexp: "ab*",
			text: []string{
				"",
				"{0}a{0}",
				"b",
				"{0}ab{0}",
				"{0}a{0}ab",
				"{0}abb{0}",
				"{0}a{0}abb",
				"xyz{0}ab{0}123",
			},
		},
		{
			name:   "star non-star",
			regexp: "a*b",
			text: []string{
				"",
				"a",
				"{0}b{0}",
				"{0}ab{0}",
				"{0}aab{0}",
				"{0}ab{0}b",
				"{0}aab{0}b",
				"xyz{0}ab{0}123",
			},
		},
		{
			name:   "non-plus plus",
			regexp: "ab+",
			text: []string{
				"",
				"a",
				"b",
				"{0}ab{0}",
				"a{0}ab{0}",
				"{0}abb{0}",
				"a{0}abb{0}",
				"xyz{0}ab{0}123",
			},
		},
		{
			name:   "plus non-plus",
			regexp: "a+b",
			text: []string{
				"",
				"a",
				"b",
				"{0}ab{0}",
				"{0}aab{0}",
				"{0}ab{0}b",
				"{0}aab{0}b",
				"xyz{0}ab{0}123",
			},
		},
		{
			name:   "non-question question",
			regexp: "ab?",
			text: []string{
				"",
				"{0}a{0}",
				"b",
				"{0}ab{0}",
				"{0}a{0}ab",
				"{0}ab{0}b",
				"{0}a{0}abb",
				"xyz{0}ab{0}123",
			},
		},
		{
			name:   "question non-question",
			regexp: "a?b",
			text: []string{
				"",
				"a",
				"{0}b{0}",
				"{0}ab{0}",
				"a{0}ab{0}",
				"{0}ab{0}b",
				"a{0}ab{0}b",
				"xyz{0}ab{0}123",
			},
		},
		{
			name:   "non-ASCII",
			regexp: "αβξ",
			text: []string{
				"",
				"α",
				"αβ",
				"αβγ",
				"{0}αβξ{0}",
				"α{0}αβξ{0}",
				"αβ{0}αβξ{0}",
				"αβγ{0}αβξ{0}",
				"{0}αβξ{0}αβξ",
				"☺{0}αβξ{0}☹",
			},
		},
		{
			name:   "reversed",
			regexp: "abc",
			flags:  Reverse,
			text: []string{
				"",
				"a",
				"ab",
				"abc",
				"{0}cba{0}",
				"αβξ{0}cba{0}",
				"{0}cba{0}xyz",
				"{0}cba{0}cba",
			},
		},
		{
			name:   "capturing groups",
			regexp: "(a)(bc)(def)",
			text: []string{
				"{01}a{1}{2}bc{23}def{03}",
			},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestAOrB(t *testing.T) {
	tests := []regexpTest{
		{name: "missing operand", regexp: "a|", error: "missing operand"},
		{name: "left operand error", regexp: "()|a", error: "missing operand"},
		{name: "right operand error", regexp: "a|()", error: "missing operand"},
		{
			name:   "precedence",
			regexp: "abc|xyz",
			text: []string{
				// It applies to all of abc and xyx, not just c and x.
				"{0}abc{0}",
				"{0}xyz{0}",
				"{0}abc{0}",
				"ab{0}xyz{0}",
				"{0}abc{0}yz",
			},
		},
		{
			name:   "literal literal",
			regexp: "a|b",
			text: []string{
				"",
				"{0}a{0}",
				"{0}b{0}",
				"xyz{0}a{0}ABC",
				"xyz{0}b{0}ABC",
			},
		},
		{
			name:   "literal charclass",
			regexp: "a|[123]",
			text: []string{
				"",
				"{0}a{0}",
				"{0}1{0}",
				"xyz{0}a{0}ABC",
				"xyz{0}1{0}ABC",
			},
		},
		{
			name:   "literal dot",
			regexp: "a|.",
			text: []string{
				"",
				"{0}a{0}",
				"{0}1{0}",
				"\n\n\n{0}a{0}\n\n\n",
				"\n\n\n{0}1{0}\n\n\n",
			},
		},
		{
			name:   "literal group",
			regexp: "a|(b)",
			text: []string{
				"",
				"{0}a{0}",
				"{01}b{01}",
				"xyz{0}a{0}ABC",
				"xyz{01}b{01}ABC",
			},
		},
		{
			name:   "charclass literal",
			regexp: "[abc]|1",
			text: []string{
				"",
				"{0}a{0}",
				"{0}1{0}",
				"xyz{0}a{0}ABC",
				"xyz{0}1{0}ABC",
			},
		},
		{
			name:   "charclass charclass",
			regexp: "[abc]|[123]",
			text: []string{
				"",
				"{0}a{0}",
				"{0}1{0}",
				"xyz{0}a{0}ABC",
				"xyz{0}1{0}ABC",
			},
		},
		{
			name:   "charclass dot",
			regexp: "[abc]|.",
			text: []string{
				"",
				"{0}a{0}",
				"{0}1{0}",
				"\n\n\n{0}a{0}\n\n\n",
				"\n\n\n{0}1{0}\n\n\n",
			},
		},
		{
			name:   "charclass group",
			regexp: "[abc]|(1)",
			text: []string{
				"",
				"{0}a{0}",
				"{01}1{01}",
				"xyz{0}a{0}ABC",
				"xyz{01}1{01}ABC",
			},
		},
		{
			name:   "dot literal",
			regexp: ".|\n",
			text: []string{
				"",
				"{0}1{0}",
				"{0}\n{0}",
			},
		},
		{
			name:   "dot charclass",
			regexp: ".|[\n]",
			text: []string{
				"",
				"{0}1{0}",
				"{0}\n{0}",
			},
		},
		{
			name:   "dot dot",
			regexp: ".|.",
			text: []string{
				"",
				"{0}1{0}",
				"{0}a{0}",
				"\n\n\n{0}1{0}\n\n\n",
				"\n\n\n{0}a{0}\n\n\n",
			},
		},
		{
			name:   "dot group",
			regexp: ".|(\n)",
			text: []string{
				"",
				"{0}1{0}",
				"{01}\n{01}",
			},
		},
		{
			name:   "group literal",
			regexp: "(a)|b",
			text: []string{
				"",
				"{01}a{01}",
				"{0}b{0}",
				"xyz{01}a{01}ABC",
				"xyz{0}b{0}ABC",
			},
		},
		{
			name:   "group charclass",
			regexp: "(a)|[123]",
			text: []string{
				"",
				"{01}a{01}",
				"{0}1{0}",
				"xyz{01}a{01}ABC",
				"xyz{0}1{0}ABC",
			},
		},
		{
			name:   "group dot",
			regexp: "(a)|.",
			text: []string{
				"",
				"{01}a{01}",
				"{0}b{0}",
				"\n\n\n{01}a{01}\n\n\n",
				"\n\n\n{0}b{0}\n\n\n",
			},
		},
		{
			name:   "group group",
			regexp: "(a)|(b)",
			text: []string{
				"",
				"{01}a{01}",
				"{02}b{02}",
				"xyz{01}a{01}ABC",
				"xyz{02}b{02}ABC",
			},
		},
		{
			name:   "non-star star",
			regexp: "a|b*",
			text: []string{
				"{00}",
				"{0}a{0}",
				"{0}b{0}",
				"{00}c",
				"{0}a{0}a",
				"{0}bb{0}",
				"{0}a{0}ab",
				"{0}a{0}bb",
			},
		},
		{
			name:   "star non-star",
			regexp: "a*|b",
			text: []string{
				"{00}",
				"{0}a{0}",
				"{0}b{0}",
				"{00}c",
				"{0}aa{0}",
				"{0}b{0}b",
				"{0}aa{0}b",
				"{0}a{0}bb",
			},
		},
		{
			name:   "non-plus plus",
			regexp: "a|b+",
			text: []string{
				"",
				"{0}a{0}",
				"{0}b{0}",
				"c",
				"{0}a{0}a",
				"{0}bb{0}",
				"{0}a{0}ab",
				"{0}a{0}bb",
			},
		},
		{
			name:   "plus non-plus",
			regexp: "a+|b",
			text: []string{
				"",
				"{0}a{0}",
				"{0}b{0}",
				"c",
				"{0}aa{0}",
				"{0}b{0}b",
				"{0}aa{0}b",
				"{0}a{0}bb",
			},
		},
		{
			name:   "non-question question",
			regexp: "a|b?",
			text: []string{
				"{00}",
				"{0}a{0}",
				"{0}b{0}",
				"{00}c",
			},
		},
		{
			name:   "question non-question",
			regexp: "a?|b",
			text: []string{
				"{00}",
				"{0}a{0}",
				"{0}b{0}",
				"{00}c",
			},
		},
		{
			name:   "line anchors",
			regexp: "^a|^b|c$|d$|^e$",
			text: []string{
				"",
				"{0}a{0}",
				"{0}b{0}",
				"{0}c{0}",
				"{0}d{0}",
				"{0}e{0}",
				"{0}a{0}x",
				"{0}b{0}x",
				"cx",
				"dx",
				"ex",
				"xa",
				"xb",
				"x{0}c{0}",
				"x{0}d{0}",
				"xe",
				"xax",
				"xbx",
				"xcx",
				"xdx",
				"xex",
				"x\n{0}a{0}x",
				"x\n{0}b{0}x",
				"x\ncx",
				"x\ndx",
				"x\nex",
				"xa\nx",
				"xb\nx",
				"x{0}c{0}\nx",
				"x{0}d{0}\nx",
				"xe\nx",
				"x\n{0}a{0}\nx",
				"x\n{0}b{0}\nx",
				"x\n{0}c{0}\nx",
				"x\n{0}d{0}\nx",
				"x\n{0}e{0}\nx",
			},
		},
		{
			name:   "capturing groups",
			regexp: "(a)|(bc)|(def)",
			text: []string{
				"{01}a{01}",
				"{02}bc{02}",
				"{03}def{03}",
			},
		},
		{
			name:   "overlap",
			regexp: "(a)|([ab])|([abc])",
			text: []string{
				// As far as I am aware,
				// there are no guarantees which branch will be choosen.
				// Let's just make sure that something is choosen.
				"{01}a{01}",
				"{02}b{02}",
				"{03}c{03}",
			},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestDelimter(t *testing.T) {
	tests := []regexpTest{
		{
			name:   "empty regexp", // Gives empty regexp.
			regexp: "",
			flags:  Delimited,
			text: []string{
				"{00}",
				"{00}xyz",
			},
		},
		{
			name:   `escaped . delimiter`,
			regexp: `.\.`,
			flags:  Delimited,
			text: []string{
				"",
				"\n",
				"{0}a{0}",
				"{0}α{0}",
				"{0}☺{0}",
			},
		},
		{
			name:   `escaped * delimiter`,
			regexp: `*a\*`,
			flags:  Delimited,
			text: []string{
				"{00}",
				"{0}aaa{0}",
			},
		},
		{
			name:   `escaped + delimiter`,
			regexp: `+a\+`,
			flags:  Delimited,
			text: []string{
				"",
				"{0}aaa{0}",
			},
		},
		{
			name:   `escaped ? delimiter`,
			regexp: `?a\?`,
			flags:  Delimited,
			text: []string{
				"{00}",
				"{0}a{0}",
			},
		},
		{
			name:   `escaped [ delimiter`,
			regexp: `[\[a-c]`,
			flags:  Delimited,
			text: []string{
				"",
				"{0}a{0}",
				"{0}b{0}",
				"{0}c{0}",
				"d",
			},
		},
		{
			name:   `escaped ] delimiter`,
			regexp: `][a-c\]`,
			flags:  Delimited,
			text: []string{
				"",
				"{0}a{0}",
				"{0}b{0}",
				"{0}c{0}",
				"d",
			},
		},
		{
			name:   `escaped ( delimiter`,
			regexp: `(\(abc)*`,
			flags:  Delimited,
			text: []string{
				"{00}",
				"{01}abc{01}",
				"{0}abcabc{1}abc{01}",
			},
		},
		{
			name:   `escaped ) delimiter`,
			regexp: `)(abc\)*`,
			flags:  Delimited,
			text: []string{
				"{00}",
				"{01}abc{01}",
				"{0}abcabc{1}abc{01}",
			},
		},
		{
			name:   `escaped | delimiter`,
			regexp: `|a\|b`,
			flags:  Delimited,
			text: []string{
				"",
				"{0}a{0}",
				"{0}b{0}",
				"c",
			},
		},
		{
			name:   `escaped \ delimiter`,
			regexp: `\\\`,
			flags:  Delimited,
			error:  "bad delimiter",
		},
		{
			name:   `escaped ^ delimiter`,
			regexp: `^\^abc`,
			flags:  Delimited,
			text: []string{
				"",
				"{0}abc{0}",
				"xyzabc",
				"xyzabc\n{0}abc{0}",
			},
		},
		{
			name:   `escaped $ delimiter`,
			regexp: `$abc\$`,
			flags:  Delimited,
			text: []string{
				"",
				"{0}abc{0}",
				"abcxyz",
				"{0}abc{0}\nxyz",
			},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestLiteralFlagDelimter(t *testing.T) {
	tests := []regexpTest{
		{
			name:   "terminates on delimiter",
			regexp: "/abc/def",
			flags:  Literal | Delimited,
			text: []string{
				"{0}abc{0}",
				"{0}abc{0}/",
				"{0}abc{0}/def",
			},
		},
		{
			name:   "escaped delimiter",
			regexp: `/abc\/def`,
			flags:  Literal | Delimited,
			text: []string{
				"abc",
				"{0}abc/def{0}",
			},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

type regexpTest struct {
	name string
	// Regexp is the regular expression under test.
	regexp string
	flags  Flags
	// Text is a set of texts to match.
	// It is marked up with the location of expected sub-matches.
	// Runes not between { and } represent the text itself.
	// Each digit between { and } represents
	// the beginning (first occurrence)
	// or end (second occurrence)
	// of the submatch corresponding to the digit.
	//
	// For example, "aaa{0}bbb{1}ccc{12}ddd{02}"
	// matches the regular expression against the text "aaabbbcccddd"
	// and expects the 0th subexpression to match "bbbcccddd",
	// the 1st subexpression to match "ccc",
	// and the 2nd subexpression to match "ddd".
	//
	// If 0 does not appear in text, then Regexp.Match is expected to return nil.
	// Other subexpressions not appearing in text are expected to be empty.
	// So, if text is "abcdef", nil is expected.
	// If text is "{00}abcdef", an empty match is expected.
	// If text is "{0}abc{2}def{02}"
	// the regexp is expected to match the entire string,
	// the 1st subexpression is expecteded to be empty,
	// and the 2nd subexpression is expected to match "def".
	text []string
	// Prev and next are the runes just before and just after text.
	// If they are non-empty, they indicate that text is a substring
	// of the middle of a larger text.
	// If prev is the empty string, text starts at the beginning of text.
	// Otherwise prev must be a single rune, preceding the text.
	// If next is the empty string, text ends at the end of text.
	// Otherwise next must be a single rune, following the text.
	prev, next string
	// Error is a regexp describing an expected compilation error.
	// If error is the empty string, no error is expected.
	// Otherwise, a compilation error is expected to match the regexp.
	error string
}

func (test regexpTest) run(t *testing.T) {
	regexp, err := Compile(strings.NewReader(test.regexp), test.flags)
	if !matchesError(test.error, err) {
		t.Errorf("%s Compile(%q, %s)=_,%v, want error matching %q",
			test.name, test.regexp, test.flags, err, test.error)
		return
	}
	prev, next := test.prevNext()
	for _, text := range test.text {
		txt, want := parseMatch(text)
		match := regexp.Match(prev, strings.NewReader(txt), next)
		pass := true
		if len(want) == 0 {
			pass = match == nil
		}
		if match == nil {
			pass = len(want) == 0
		}
		for i, m := range match {
			switch w, ok := want[i]; {
			// BUG(eaburns): Submatches should never have negative size.
			case !ok && m[0] < m[1]:
				pass = false
			case ok && w != m:
				pass = false
			}
		}
		if !pass {
			got := matchString(txt, match)
			t.Errorf("%s Match(%c, %q, %c)=%v (%q) want %q",
				test.name, prev, txt, next, match, got, text)
		}
	}
}

func (test regexpTest) prevNext() (prev, next rune) {
	switch len(test.prev) {
	case 0:
		prev = None
	case 1:
		prev, _ = utf8.DecodeRuneInString(test.prev)
	default:
		panic("len(prev) must be ≤1")
	}
	switch len(test.next) {
	case 0:
		next = None
	case 1:
		next, _ = utf8.DecodeRuneInString(test.next)
	default:
		panic("len(next) must be ≤1")
	}
	return
}

func parseMatch(str string) (string, map[int][2]int64) {
	var i int64
	var text []rune
	var mark bool
	count := make(map[int]int)
	match := make(map[int][2]int64)
	for len(str) > 0 {
		r, w := utf8.DecodeRuneInString(str)
		str = str[w:]
		switch {
		case !mark && r == '{':
			mark = true
		case mark && r == '}':
			mark = false
		case mark:
			if !unicode.IsDigit(r) {
				panic("expected digit, got " + string(r))
			}
			d := int(r - '0')
			count[d]++
			if m, ok := match[d]; !ok {
				match[d] = [2]int64{i}
			} else {
				match[d] = [2]int64{m[0], i}
			}
		default:
			text = append(text, r)
			i += int64(w)
		}
	}
	for m, c := range count {
		if c != 2 {
			panic(fmt.Sprintf("subexpr match %d occurs %d times", m, c))
		}
	}
	return string(text), match
}

func matchString(text string, match [][2]int64) string {
	switch {
	case len(match) < 1:
		return text
	case len(match) > 10:
		panic("too many subexprs")
	}
	var s []rune
	var i int
	for {
		var ms []rune
		for j, m := range match {
			if i := int64(i); m[0] != i && m[1] != i || m[0] > m[1] {
				continue
			}
			if j == 0 || m[0] != m[1] {
				d := strconv.Itoa(j)
				ms = append(ms, rune(d[0]))
				if m[0] == m[1] {
					ms = append(ms, rune(d[0]))
				}
			}
		}
		if len(ms) > 0 {
			s = append(s, '{')
			s = append(s, ms...)
			s = append(s, '}')
		}
		if len(text) == 0 {
			break
		}
		r, w := utf8.DecodeRuneInString(text)
		s = append(s, r)
		text = text[w:]
		i += w
	}
	return string(s)
}

// MatchesError returns whether the regexp matches the error string.
// If re is the empty string, matchesError returns whether err is nil.
// Otherwise, it returns whether the err is non-nil and matched by the regexp.
func matchesError(re string, err error) bool {
	if re == "" {
		return err == nil
	}
	return err != nil && regexp.MustCompile(re).MatchString(err.Error())
}
