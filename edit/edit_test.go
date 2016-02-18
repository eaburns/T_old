// Copyright © 2015, The T Authors.

package edit

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var escTests = []struct {
	unescaped, escaped string
	esc                string
}{
	{unescaped: `\`, escaped: `\\`},
	{unescaped: "\n", escaped: `\n`},
	{unescaped: "a\nb\nc", escaped: `a\nb\nc`},
	{unescaped: `\n`, escaped: `\\n`},
	{unescaped: `\\`, escaped: `\\\\`},
	{unescaped: "abcxyz", escaped: "abcxyz"},
	{unescaped: "aaaa", escaped: `\a\a\a\a`, esc: "a"},
	{unescaped: "abc", escaped: `\a\b\c`, esc: "abc"},
}

func TestEscape(t *testing.T) {
	for _, test := range escTests {
		esc := []rune(test.esc)
		got := Escape(test.unescaped, esc...)
		if got != test.escaped {
			t.Errorf("Escape(%q, %v)=%q, want %q", test.unescaped, esc, got, test.escaped)
		}
	}
}

func TestUnescape(t *testing.T) {
	for _, test := range escTests {
		got := Unescape(test.escaped)
		if got != test.unescaped {
			t.Errorf("Unescape(%q)=%q, want %q", test.escaped, got, test.unescaped)
		}
	}

	// Test trailing \.
	given := `abcxyz\`
	want := given
	if got := Unescape(given); got != want {
		t.Errorf("Unescape(%q)=%q, want %q", given, got, want)
	}
}

func TestEd(t *testing.T) {
	tests := []struct {
		str, left string
		edit      Edit
		error     string
	}{
		{str: "#0 UNKNOWN", error: "unknown command"},
		{str: strconv.FormatInt(math.MaxInt64, 10) + "0", error: "value out of range"},

		{str: "", edit: Set(Dot, '.')},
		{str: ".", edit: Set(Dot, '.')},
		{str: "  .", edit: Set(Dot, '.')},
		{str: "#0", edit: Set(Rune(0), '.')},
		{str: "#0+1", edit: Set(Rune(0).Plus(Line(1)), '.')},
		{str: " #0 + 1 ", edit: Set(Rune(0).Plus(Line(1)), '.')},
		{str: "#0+1\nc/abc", left: "\nc/abc", edit: Set(Rune(0).Plus(Line(1)), '.')},
		{str: "/abc\n1c/xyz", left: "\n1c/xyz", edit: Set(Regexp("abc"), '.')},

		{str: "k", edit: Set(Dot, '.')},
		{str: " k ", edit: Set(Dot, '.')},
		{str: "#0k ", edit: Set(Rune(0), '.')},
		{str: "#0ka", edit: Set(Rune(0), 'a')},
		{str: "#0k a", edit: Set(Rune(0), 'a')},
		{str: "#0k	 a", edit: Set(Rune(0), 'a')},
		{str: "#0k	 α", edit: Set(Rune(0), 'α')},

		{str: "c/αβξ", edit: Change(Dot, "αβξ")},
		{str: "c   /αβξ", edit: Change(Dot, "αβξ")},
		{str: "c", edit: Change(Dot, "")},
		{str: "c  /", edit: Change(Dot, "")},
		{str: "c/αβξ/", edit: Change(Dot, "αβξ")},
		{str: "c/αβξ\n", left: "\n", edit: Change(Dot, "αβξ")},
		{str: "c/αβξ/xyz", left: "xyz", edit: Change(Dot, "αβξ")},
		{str: "c/αβξ\nxyz", left: "\nxyz", edit: Change(Dot, "αβξ")},
		{str: "#1,#2c/αβξ/", edit: Change(Rune(1).To(Rune(2)), "αβξ")},
		{str: " #1 , #2 c/αβξ/", edit: Change(Rune(1).To(Rune(2)), "αβξ")},
		{str: "c/αβξ\\/", edit: Change(Dot, "αβξ/")},
		{str: `c/αβξ\n`, edit: Change(Dot, "αβξ\n")},
		{str: "c\nαβξ\n.\n", left: "\n", edit: Change(Dot, "αβξ\n")},
		{str: "c\nαβξ\n.", edit: Change(Dot, "αβξ\n")},
		{str: "c\nαβξ\n\n.", edit: Change(Dot, "αβξ\n\n")},
		{str: "c\nαβξ\nabc\n.", edit: Change(Dot, "αβξ\nabc\n")},
		{str: "c \n", edit: Change(Dot, "")},
		{str: `c/\n`, edit: Change(Dot, "\n")},
		{str: `c/\\n`, edit: Change(Dot, `\n`)},
		{str: `c/\/`, edit: Change(Dot, `/`)},
		{str: `c/\//`, edit: Change(Dot, `/`)},
		{str: `c/\`, edit: Change(Dot, `\`)},
		{str: `c/\\`, edit: Change(Dot, `\`)},
		{str: `c/\\/`, edit: Change(Dot, `\`)},

		{str: "a/αβξ", edit: Append(Dot, "αβξ")},
		{str: "a   /αβξ", edit: Append(Dot, "αβξ")},
		{str: "a", edit: Append(Dot, "")},
		{str: "a  /", edit: Append(Dot, "")},
		{str: "a/αβξ/", edit: Append(Dot, "αβξ")},
		{str: "a/αβξ\n", left: "\n", edit: Append(Dot, "αβξ")},
		{str: "a/αβξ/xyz", left: "xyz", edit: Append(Dot, "αβξ")},
		{str: "a/αβξ\nxyz", left: "\nxyz", edit: Append(Dot, "αβξ")},
		{str: "#1,#2a/αβξ/", edit: Append(Rune(1).To(Rune(2)), "αβξ")},
		{str: " #1 , #2 a/αβξ/", edit: Append(Rune(1).To(Rune(2)), "αβξ")},
		{str: "a/αβξ\\/", edit: Append(Dot, "αβξ/")},
		{str: `a/αβξ\n`, edit: Append(Dot, "αβξ\n")},
		{str: "a\nαβξ\n.\n", left: "\n", edit: Append(Dot, "αβξ\n")},
		{str: "a\nαβξ\n.", edit: Append(Dot, "αβξ\n")},
		{str: "a\nαβξ\n\n.", edit: Append(Dot, "αβξ\n\n")},
		{str: "a\nαβξ\nabc\n.", edit: Append(Dot, "αβξ\nabc\n")},
		{str: "a \n", edit: Append(Dot, "")},

		{str: "i/αβξ", edit: Insert(Dot, "αβξ")},
		{str: "i   /αβξ", edit: Insert(Dot, "αβξ")},
		{str: "i", edit: Insert(Dot, "")},
		{str: "i  /", edit: Insert(Dot, "")},
		{str: "i/αβξ/", edit: Insert(Dot, "αβξ")},
		{str: "i/αβξ\n", left: "\n", edit: Insert(Dot, "αβξ")},
		{str: "i/αβξ/xyz", left: "xyz", edit: Insert(Dot, "αβξ")},
		{str: "i/αβξ\nxyz", left: "\nxyz", edit: Insert(Dot, "αβξ")},
		{str: "#1,#2i/αβξ/", edit: Insert(Rune(1).To(Rune(2)), "αβξ")},
		{str: " #1 , #2 i/αβξ/", edit: Insert(Rune(1).To(Rune(2)), "αβξ")},
		{str: "i/αβξ\\/", edit: Insert(Dot, "αβξ/")},
		{str: `i/αβξ\n`, edit: Insert(Dot, "αβξ\n")},
		{str: "i\nαβξ\n.\n", left: "\n", edit: Insert(Dot, "αβξ\n")},
		{str: "i\nαβξ\n.", edit: Insert(Dot, "αβξ\n")},
		{str: "i\nαβξ\n\n.", edit: Insert(Dot, "αβξ\n\n")},
		{str: "i\nαβξ\nabc\n.", edit: Insert(Dot, "αβξ\nabc\n")},
		{str: "i \n", edit: Insert(Dot, "")},

		{str: "d", edit: Delete(Dot)},
		{str: "#1,#2d", edit: Delete(Rune(1).To(Rune(2)))},
		{str: "dxyz", left: "xyz", edit: Delete(Dot)},
		{str: "d\nxyz", left: "\nxyz", edit: Delete(Dot)},
		{str: "d  \nxyz", left: "  \nxyz", edit: Delete(Dot)},

		{str: "m", edit: Move(Dot, Dot)},
		{str: "m/abc/", edit: Move(Dot, Regexp("abc"))},
		{str: "/abc/m/def/", edit: Move(Regexp("abc"), Regexp("def"))},
		{str: "#1+1m$", edit: Move(Rune(1).Plus(Line(1)), End)},
		{str: " #1 + 1 m $", edit: Move(Rune(1).Plus(Line(1)), End)},
		{str: "1m$xyz", left: "xyz", edit: Move(Line(1), End)},
		{str: "1m\n$xyz", left: "\n$xyz", edit: Move(Line(1), Dot)},
		{str: "m" + strconv.FormatInt(math.MaxInt64, 10) + "0", error: "value out of range"},

		{str: "t", edit: Copy(Dot, Dot)},
		{str: "t/abc/", edit: Copy(Dot, Regexp("abc"))},
		{str: "/abc/t/def/", edit: Copy(Regexp("abc"), Regexp("def"))},
		{str: "#1+1t$", edit: Copy(Rune(1).Plus(Line(1)), End)},
		{str: " #1 + 1 t $", edit: Copy(Rune(1).Plus(Line(1)), End)},
		{str: "1t$xyz", left: "xyz", edit: Copy(Line(1), End)},
		{str: "1t\n$xyz", left: "\n$xyz", edit: Copy(Line(1), Dot)},
		{str: "t" + strconv.FormatInt(math.MaxInt64, 10) + "0", error: "value out of range"},

		{str: "p", edit: Print(Dot)},
		{str: "pxyz", left: "xyz", edit: Print(Dot)},
		{str: "#1+1p", edit: Print(Rune(1).Plus(Line(1)))},
		{str: " #1 + 1 p", edit: Print(Rune(1).Plus(Line(1)))},

		{str: "=", edit: WhereLine(Dot)},
		{str: "=xyz", left: "xyz", edit: WhereLine(Dot)},
		{str: "#1+1=", edit: WhereLine(Rune(1).Plus(Line(1)))},
		{str: " #1 + 1 =", edit: WhereLine(Rune(1).Plus(Line(1)))},

		{str: "=#", edit: Where(Dot)},
		{str: "=#xyz", left: "xyz", edit: Where(Dot)},
		{str: "#1+1=#", edit: Where(Rune(1).Plus(Line(1)))},
		{str: " #1 + 1 =#", edit: Where(Rune(1).Plus(Line(1)))},

		{str: "s/a/b", edit: Sub(Dot, "a", "b")},
		{str: "s;a;b", edit: Sub(Dot, "a", "b")},
		{str: "s/a*|b*//", edit: Sub(Dot, "a*|b*", "")},
		{str: "s/a//", edit: Sub(Dot, "a", "")},
		{str: "s/a/\n/g", left: "\n/g", edit: Sub(Dot, "a", "")},
		{str: `s/(.*)/a\1`, edit: Sub(Dot, "(.*)", "a1")},
		{str: ".s/a/b", edit: Sub(Dot, "a", "b")},
		{str: "#1+1s/a/b", edit: Sub(Rune(1).Plus(Line(1)), "a", "b")},
		{str: " #1 + 1 s/a/b", edit: Sub(Rune(1).Plus(Line(1)), "a", "b")},
		{str: " #1 + 1 s/a/b", edit: Sub(Rune(1).Plus(Line(1)), "a", "b")},
		{str: "s/a/b/xyz", left: "xyz", edit: Sub(Dot, "a", "b")},
		{str: "s/a/b\nxyz", left: "\nxyz", edit: Sub(Dot, "a", "b")},
		{str: "s1/a/b", edit: Sub(Dot, "a", "b")},
		{str: "s/a/b/g", edit: SubGlobal(Dot, "a", "b")},
		{str: " #1 + 1 s/a/b/g", edit: SubGlobal(Rune(1).Plus(Line(1)), "a", "b")},
		{str: "s2/a/b", edit: Substitute{A: Dot, RE: "a", With: "b", From: 2}},
		{str: "s2;a;b", edit: Substitute{A: Dot, RE: "a", With: "b", From: 2}},
		{str: "s1000/a/b", edit: Substitute{A: Dot, RE: "a", With: "b", From: 1000}},
		{str: "s 2 /a/b", edit: Substitute{A: Dot, RE: "a", With: "b", From: 2}},
		{str: "s 1000 /a/b/g", edit: Substitute{A: Dot, RE: "a", With: "b", Global: true, From: 1000}},
		{str: "s", edit: Sub(Dot, "", "")},
		{str: "s\nabc", left: "\nabc", edit: Sub(Dot, "", "")},
		{str: "s/", edit: Sub(Dot, "", "")},
		{str: "s//b", edit: Sub(Dot, "", "b")},
		{str: "s/\n/b", left: "\n/b", edit: Sub(Dot, "", "")},
		{str: "s" + strconv.FormatInt(math.MaxInt64, 10) + "0" + "/a/b/g", error: "value out of range"},
		{str: "s/*", error: "missing"},

		{str: "|cmd", edit: Pipe(Dot, "cmd")},
		{str: "|	   cmd", edit: Pipe(Dot, "cmd")},
		{str: "|cmd\nleft", left: "\nleft", edit: Pipe(Dot, "cmd")},
		{str: "|	   cmd\nleft", left: "\nleft", edit: Pipe(Dot, "cmd")},
		{str: "|cmd\\nleft", edit: Pipe(Dot, "cmd\nleft")},
		{str: "|	   cmd\\nleft", edit: Pipe(Dot, "cmd\nleft")},
		{str: "  	|cmd", edit: Pipe(Dot, "cmd")},
		{str: ",|cmd", edit: Pipe(All, "cmd")},
		{str: "$|cmd", edit: Pipe(End, "cmd")},
		{str: ",	  |cmd", edit: Pipe(All, "cmd")},
		{str: "  	,	  |cmd", edit: Pipe(All, "cmd")},
		{str: ",+3|cmd", edit: Pipe(Line(0).To(Dot.Plus(Line(3))), "cmd")},
		{str: `|\x\y\z`, edit: Pipe(Dot, `\x\y\z`)},

		{str: ">cmd", edit: PipeTo(Dot, "cmd")},
		{str: ">	   cmd", edit: PipeTo(Dot, "cmd")},
		{str: ">cmd\nleft", left: "\nleft", edit: PipeTo(Dot, "cmd")},
		{str: ">	   cmd\nleft", left: "\nleft", edit: PipeTo(Dot, "cmd")},
		{str: ">cmd\\nleft", edit: PipeTo(Dot, "cmd\nleft")},
		{str: ">	   cmd\\nleft", edit: PipeTo(Dot, "cmd\nleft")},
		{str: "  	>cmd", edit: PipeTo(Dot, "cmd")},
		{str: ",>cmd", edit: PipeTo(All, "cmd")},
		{str: "$>cmd", edit: PipeTo(End, "cmd")},
		{str: ",	  >cmd", edit: PipeTo(All, "cmd")},
		{str: "  	,	  >cmd", edit: PipeTo(All, "cmd")},
		{str: ",+3>cmd", edit: PipeTo(Line(0).To(Dot.Plus(Line(3))), "cmd")},
		{str: `>\x\y\z`, edit: PipeTo(Dot, `\x\y\z`)},

		{str: "<cmd", edit: PipeFrom(Dot, "cmd")},
		{str: "<	   cmd", edit: PipeFrom(Dot, "cmd")},
		{str: "<cmd\nleft", left: "\nleft", edit: PipeFrom(Dot, "cmd")},
		{str: "<	   cmd\nleft", left: "\nleft", edit: PipeFrom(Dot, "cmd")},
		{str: "<cmd\\nleft", edit: PipeFrom(Dot, "cmd\nleft")},
		{str: "<	   cmd\\nleft", edit: PipeFrom(Dot, "cmd\nleft")},
		{str: "  	<cmd", edit: PipeFrom(Dot, "cmd")},
		{str: ",<cmd", edit: PipeFrom(All, "cmd")},
		{str: "$<cmd", edit: PipeFrom(End, "cmd")},
		{str: ",	  <cmd", edit: PipeFrom(All, "cmd")},
		{str: "  	,	  <cmd", edit: PipeFrom(All, "cmd")},
		{str: ",+3<cmd", edit: PipeFrom(Line(0).To(Dot.Plus(Line(3))), "cmd")},
		{str: `<\x\y\z`, edit: PipeFrom(Dot, `\x\y\z`)},

		{str: "u", edit: Undo(1)},
		{str: " u", edit: Undo(1)},
		{str: "u1", edit: Undo(1)},
		{str: " u1", edit: Undo(1)},
		{str: "u100", edit: Undo(100)},
		{str: " u100", edit: Undo(100)},
		{str: "u" + strconv.FormatInt(math.MaxInt64, 10) + "0", error: "value out of range"},
		{str: "r", edit: Redo(1)},
		{str: " r", edit: Redo(1)},
		{str: "r1", edit: Redo(1)},
		{str: " r1", edit: Redo(1)},
		{str: "r100", edit: Redo(100)},
		{str: " r100", edit: Redo(100)},
		{str: "r" + strconv.FormatInt(math.MaxInt64, 10) + "0", error: "value out of range"},
	}
	for _, test := range tests {
		rs := strings.NewReader(test.str)
		e, err := Ed(rs)
		if !matchesError(test.error, err) || !reflect.DeepEqual(e, test.edit) {
			t.Errorf(`Ed(%q)=%q,%v, want %q,%q`, test.str, e, err, test.edit, test.error)
		}
		if test.error != "" {
			continue
		}
		left, err := ioutil.ReadAll(rs)
		if err != nil {
			t.Fatal(err)
		}
		if string(left) != test.left {
			t.Errorf(`Ed(%q) leftover %q want %q`, test.str, string(left), test.left)
		}
	}
}

func TestEditString(t *testing.T) {
	tests := []struct {
		edit Edit
		str  string
	}{
		{Change(All, `xyz`), `0,$c/xyz/`},
		{Change(Dot, "a\nb\nc"), `.c/a\nb\nc/`},
		{Change(Dot, `a\nb\nc`), `.c/a\\nb\\nc/`},
		{Change(Regexp("a*"), `b`), `/a*/c/b/`},
		{Change(Regexp("/*"), `b`), `/\/*/c/b/`},
		{Change(Dot, `//`), `.c/\/\//`},
		{Change(Dot, "\n"), `.c/\n/`},

		{Append(All, "xyz"), "0,$a/xyz/"},
		{Append(Dot, "a\nb\nc"), `.a/a\nb\nc/`},
		{Append(Dot, `a\nb\nc`), `.a/a\\nb\\nc/`},
		{Append(Regexp("a*"), `b`), `/a*/a/b/`},
		{Append(Regexp("/*"), `b`), `/\/*/a/b/`},
		{Append(Dot, `//`), `.a/\/\//`},
		{Append(Dot, "\n"), `.a/\n/`},

		{Insert(All, "xyz"), "0,$i/xyz/"},
		{Insert(Dot, "a\nb\nc"), `.i/a\nb\nc/`},
		{Insert(Dot, `a\nb\nc`), `.i/a\\nb\\nc/`},
		{Insert(Regexp("a*"), `b`), `/a*/i/b/`},
		{Insert(Regexp("/*"), `b`), `/\/*/i/b/`},
		{Insert(Dot, `//`), `.i/\/\//`},
		{Insert(Dot, "\n"), `.i/\n/`},

		{Delete(All), `0,$d`},
		{Delete(Dot), `.d`},
		{Delete(Regexp("a*")), `/a*/d`},
		{Delete(Regexp("/*")), `/\/*/d`},

		{Copy(Dot, Line(2)), `.t2`},
		{Copy(Line(1), Dot), `1t.`},
		{Copy(Line(1), Line(2)), `1t2`},
		{Copy(Regexp("a*"), Dot), `/a*/t.`},
		{Copy(Regexp("/*"), Dot), `/\/*/t.`},
		{Copy(Dot, Regexp("b*")), `.t/b*/`},
		{Copy(Dot, Regexp("/*")), `.t/\/*/`},
		{Copy(Regexp("a*"), Regexp("b*")), `/a*/t/b*/`},

		{Move(Dot, Line(2)), `.m2`},
		{Move(Line(1), Dot), `1m.`},
		{Move(Line(1), Line(2)), `1m2`},
		{Move(Regexp("a*"), Dot), `/a*/m.`},
		{Move(Regexp("/*"), Dot), `/\/*/m.`},
		{Move(Dot, Regexp("b*")), `.m/b*/`},
		{Move(Dot, Regexp("/*")), `.m/\/*/`},
		{Move(Regexp("a*"), Regexp("b*")), `/a*/m/b*/`},

		{Pipe(All, "cat"), "0,$|cat\n"},
		{Pipe(Regexp("a*"), "cat"), "/a*/|cat\n"},
		{Pipe(Regexp("/*"), "cat"), "/\\/*/|cat\n"},

		{PipeTo(All, "cat"), "0,$>cat\n"},
		{PipeTo(Regexp("a*"), "cat"), "/a*/>cat\n"},
		{PipeTo(Regexp("/*"), "cat"), "/\\/*/>cat\n"},

		{PipeFrom(All, "cat"), "0,$<cat\n"},
		{PipeFrom(Regexp("a*"), "cat"), "/a*/<cat\n"},
		{PipeFrom(Regexp("/*"), "cat"), "/\\/*/<cat\n"},

		{Print(All), `0,$p`},
		{Print(Dot), `.p`},
		{Print(Regexp("a*")), `/a*/p`},
		{Print(Regexp("/*")), `/\/*/p`},

		{Where(All), `0,$=#`},
		{Where(Dot), `.=#`},
		{Where(Regexp("a*")), `/a*/=#`},
		{Where(Regexp("/*")), `/\/*/=#`},

		{WhereLine(All), `0,$=`},
		{WhereLine(Dot), `.=`},
		{WhereLine(Regexp("a*")), `/a*/=`},
		{WhereLine(Regexp("/*")), `/\/*/=`},

		{Undo(1), "u1"},
		{Undo(2), "u2"},
		{Undo(0), "u1"},
		{Undo(-4), "u1"},

		{Redo(1), "r1"},
		{Redo(2), "r2"},
		{Redo(0), "r1"},
		{Redo(-4), "r1"},

		{Sub(All, "a*", "b"), `0,$s/a*/b/`},
		{Sub(All, "/*", "b"), `0,$s/\/*/b/`},
		{Sub(All, "a*", "/"), `0,$s/a*/\//`},
		{Sub(All, "\n*", "b"), `0,$s/\n*/b/`},
		{Sub(All, "a*", "\n"), `0,$s/a*/\n/`},
		{Sub(All, `(a*)bc`, `\1`), `0,$s/(a*)bc/\\1/`},

		{SubGlobal(All, "a*", "b"), `0,$s/a*/b/g`},
		{SubGlobal(All, "/*", "b"), `0,$s/\/*/b/g`},
		{SubGlobal(All, "a*", "/"), `0,$s/a*/\//g`},
		{SubGlobal(All, "\n*", "b"), `0,$s/\n*/b/g`},
		{SubGlobal(All, "a*", "\n"), `0,$s/a*/\n/g`},
		{SubGlobal(All, `(a*)bc`, `\1`), `0,$s/(a*)bc/\\1/g`},

		{Substitute{A: All, RE: "a*", With: "b", From: 2}, `0,$s2/a*/b/`},
		{Substitute{A: All, RE: "a*", With: "b", From: 0}, `0,$s/a*/b/`},
		{Substitute{A: All, RE: "a*", With: "b", From: -1}, `0,$s/a*/b/`},
	}
	for _, test := range tests {
		if s := test.edit.String(); s != test.str {
			t.Errorf("String()=%q, want %q\n", s, test.str)
		}
	}
}

var changeTests = []editTest{
	{
		name:  "out of range",
		do:    []Edit{Change(Rune(1), "")},
		error: "out of range",
	},
	{
		name:  "delete empty",
		given: "{..}Hello 世界",
		do:    []Edit{Change(Dot, "")},
		want:  "{..}Hello 世界",
	},
	{
		name:  "delete everything",
		given: "{..}Hello 世界",
		do:    []Edit{Change(All, "")},
		want:  "{..}",
	},
	{
		name:  "insert at beginning",
		given: "{..}Hello 世界",
		do:    []Edit{Change(Dot, "XYZ")},
		want:  "{.}XYZ{.}Hello 世界",
	},
	{
		name:  "insert in middle",
		given: "H{..}ello 世界",
		do:    []Edit{Change(Dot, "XYZ")},
		want:  "H{.}XYZ{.}ello 世界",
	},
	{
		name:  "insert at end",
		given: "Hello 世界{..}",
		do:    []Edit{Change(Dot, "XYZ")},
		want:  "Hello 世界{.}XYZ{.}",
	},
	{
		name:  "replace in middle",
		given: "H{.}ello 世{.}界",
		do:    []Edit{Change(Dot, "XYZ")},
		want:  "H{.}XYZ{.}界",
	},
	{
		name:  "delete updates marks",
		given: "{..m}abc{mn}123{no}xyz{o}",
		do:    []Edit{Change(Regexp("2"), "")},
		want:  "{m}abc{mn}1{..}3{no}xyz{o}",
	},
	{
		name:  "insert updates marks",
		given: "{..m}abc{mn}123{no}xyz{o}",
		do:    []Edit{Change(Regexp("2"), "xxx")},
		want:  "{m}abc{mn}1{.}xxx{.}3{no}xyz{o}",
	},

	// Test escaping.
	{
		name:  "only delimiter",
		given: "{..}",
		do:    []Edit{Change(All, "/")},
		want:  "{.}/{.}",
	},
	{
		name:  "delimiter",
		given: "{..}",
		do:    []Edit{Change(All, "/abc/")},
		want:  "{.}/abc/{.}",
	},
	{
		name:  "escape rune",
		given: "{..}",
		do:    []Edit{Change(All, `\\\`)},
		want:  `{.}\\\{.}`,
	},
	{
		name:  "only escape",
		given: "{..}",
		do:    []Edit{Change(All, `\`)},
		want:  `{.}\{.}`,
	},
	{
		name:  "escape at end",
		given: "{..}",
		do:    []Edit{Change(All, `text\`)},
		want:  `{.}text\{.}`,
	},
	{
		name:  "raw newline",
		given: "{..}",
		do:    []Edit{Change(All, "\n")},
		want:  "{.}\n{.}",
	},
	{
		name:  `escaped \ then raw newline`,
		given: "{..}",
		do:    []Edit{Change(All, "\\\n")},
		want:  "{.}\\\n{.}",
	},
	{
		name:  `escaped \ then n`,
		given: "{..}",
		do:    []Edit{Change(All, `\n`)},
		want:  `{.}\n{.}`,
	},
}

func TestEditChange(t *testing.T) {
	for _, test := range changeTests {
		test.run(t)
	}
}

func TestEditChangeFromString(t *testing.T) {
	for _, test := range changeTests {
		test.runFromString(t)
	}
}

var appendTests = []editTest{
	{
		name:  "out of range",
		do:    []Edit{Append(Rune(1), "")},
		error: "out of range",
	},
	{
		name:  "append empty to beginning",
		given: "{..}Hello 世界",
		do:    []Edit{Append(Dot, "")},
		want:  "{..}Hello 世界",
	},
	{
		name:  "append empty to end",
		given: "{..}Hello 世界",
		do:    []Edit{Append(All, "")},
		want:  "Hello 世界{..}",
	},
	{
		name:  "append at beginning",
		given: "{..}Hello 世界",
		do:    []Edit{Append(Dot, "XYZ")},
		want:  "{.}XYZ{.}Hello 世界",
	},
	{
		name:  "append in middle",
		given: "{.}H{.}ello 世界",
		do:    []Edit{Append(Dot, "XYZ")},
		want:  "H{.}XYZ{.}ello 世界",
	},
	{
		name:  "append at end",
		given: "{..}Hello 世界",
		do:    []Edit{Append(All, "XYZ")},
		want:  "Hello 世界{.}XYZ{.}",
	},
	{
		name:  "updates marks",
		given: "{..m}abc{mn}123{no}xyz{o}",
		do:    []Edit{Append(Regexp("2"), "xxx")},
		want:  "{m}abc{mn}12{.}xxx{.}3{no}xyz{o}",
	},
}

func TestEditAppend(t *testing.T) {
	for _, test := range appendTests {
		test.run(t)
	}
}

func TestEditAppendFromString(t *testing.T) {
	for _, test := range appendTests {
		test.runFromString(t)
	}
}

var insertTests = []editTest{
	{
		name:  "out of range",
		do:    []Edit{Insert(Rune(1), "")},
		error: "out of range",
	},
	{
		name:  "insert empty at beginning",
		given: "{..}Hello 世界",
		do:    []Edit{Insert(Dot, "")},
		want:  "{..}Hello 世界",
	},
	{
		name:  "insert empty at end",
		given: "Hello 世界{..}",
		do:    []Edit{Insert(Dot, "")},
		want:  "Hello 世界{..}",
	},
	{
		name:  "insert at beginning",
		given: "{..}Hello 世界",
		do:    []Edit{Insert(All, "XYZ")},
		want:  "{.}XYZ{.}Hello 世界",
	},
	{
		name:  "insert in middle",
		given: "H{.}e{.}llo 世界",
		do:    []Edit{Insert(Dot, "XYZ")},
		want:  "H{.}XYZ{.}ello 世界",
	},
	{
		name:  "insert at end",
		given: "Hello 世界{..}",
		do:    []Edit{Insert(Dot, "XYZ")},
		want:  "Hello 世界{.}XYZ{.}",
	},
	{
		name:  "updates marks",
		given: "{..m}abc{mn}123{no}xyz{o}",
		do:    []Edit{Insert(Regexp("2"), "xxx")},
		want:  "{m}abc{mn}1{.}xxx{.}23{no}xyz{o}",
	},
}

func TestEditInsert(t *testing.T) {
	for _, test := range insertTests {
		test.run(t)
	}
}

func TestEditInsertFromString(t *testing.T) {
	for _, test := range insertTests {
		test.runFromString(t)
	}
}

var deleteTests = []editTest{
	{
		name:  "out of range",
		do:    []Edit{Delete(Rune(1))},
		error: "out of range",
	},
	{
		name:  "delete empty buffer",
		given: "{..}",
		do:    []Edit{Delete(All)},
		want:  "{..}",
	},
	{
		name:  "delete all",
		given: "{..}Hello 世界",
		do:    []Edit{Delete(All)},
		want:  "{..}",
	},
	{
		name:  "delete nothing",
		given: "{..}Hello 世界",
		do:    []Edit{Delete(Dot)},
		want:  "{..}Hello 世界",
	},
	{
		name:  "delete from beginning",
		given: "{.}XYZ{.}Hello 世界",
		do:    []Edit{Delete(Dot)},
		want:  "{..}Hello 世界",
	},
	{
		name:  "delete from middle",
		given: "Hell{.}XYZ{.}o 世界",
		do:    []Edit{Delete(Dot)},
		want:  "Hell{..}o 世界",
	},
	{
		name:  "delete from end",
		given: "Hello 世界{.}XYZ{.}",
		do:    []Edit{Delete(Dot)},
		want:  "Hello 世界{..}",
	},
	{
		name:  "updates marks",
		given: "{..m}abc{mn}123{no}xyz{o}",
		do:    []Edit{Delete(Regexp("2"))},
		want:  "{m}abc{mn}1{..}3{no}xyz{o}",
	},
}

func TestEditDelete(t *testing.T) {
	for _, test := range deleteTests {
		test.run(t)
	}
}

func TestEditDeleteFromString(t *testing.T) {
	for _, test := range deleteTests {
		test.runFromString(t)
	}
}

var moveTests = []editTest{
	{
		name:  "first address out of range",
		do:    []Edit{Move(Rune(1), Rune(2))},
		error: "out of range",
	},
	{
		name:  "second address out of range",
		given: "{..}a",
		do:    []Edit{Move(Rune(1), Rune(2))},
		error: "out of range",
		want:  "{..}a",
	},
	{
		name:  "overlap beginning",
		given: "{.}abcd{.}",
		do:    []Edit{Move(Dot, Rune(1))},
		error: "overlap",
		want:  "{.}abcd{.}",
	},
	{
		name:  "overlap middle",
		given: "{.}abcd{.}",
		do:    []Edit{Move(Dot, Rune(2))},
		error: "overlap",
		want:  "{.}abcd{.}",
	},
	{
		name:  "overlap end",
		given: "{.}abcd{.}",
		do:    []Edit{Move(Dot, Rune(3))},
		error: "overlap",
		want:  "{.}abcd{.}",
	},
	{
		name:  "move to same place",
		given: "{.}abc{.}",
		do:    []Edit{Move(Regexp("abc"), Dot)},
		want:  "{.}abc{.}",
	},
	{
		name:  "move to beginning",
		given: "xyz{.}abc{.}",
		do:    []Edit{Move(Dot, Rune(0))},
		want:  "{.}abc{.}xyz",
	},
	{
		name:  "move to middle",
		given: "111{.}abc{.}yz222",
		do:    []Edit{Move(Dot, Regexp("y"))},
		want:  "111y{.}abc{.}z222",
	},
	{
		name:  "move to end",
		given: "{.}xyz{.}abc",
		do:    []Edit{Move(Dot, End)},
		want:  "abc{.}xyz{.}",
	},
	{
		name:  "update marks",
		given: "{..m}abc{mn}123{no}xyz{opp}",
		do:    []Edit{Move(Regexp("123"), Regexp("x"))},
		want:  "{m}abc{mnno}x{.}123{.}yz{opp}",
	},
}

func TestEditMove(t *testing.T) {
	for _, test := range moveTests {
		test.run(t)
	}
}

func TestEditMoveFromString(t *testing.T) {
	for _, test := range moveTests {
		test.runFromString(t)
	}
}

var copyTests = []editTest{
	{
		name:  "first address out of range",
		do:    []Edit{Copy(Rune(1), Rune(2))},
		error: "out of range",
	},
	{
		name:  "second address out of range",
		given: "{..}a",
		do:    []Edit{Copy(Rune(1), Rune(2))},
		error: "out of range",
		want:  "{..}a",
	},
	{
		name:  "copy to beginning",
		given: "xyz{.}abc{.}",
		do:    []Edit{Copy(Dot, Rune(0))},
		want:  "{.}abc{.}xyzabc",
	},
	{
		name:  "copy to middle",
		given: "111{.}abc{.}yz222",
		do:    []Edit{Copy(Dot, Regexp("y"))},
		want:  "111abcy{.}abc{.}z222",
	},
	{
		name:  "copy to end",
		given: "{.}xyz{.}abc",
		do:    []Edit{Copy(Dot, End)},
		want:  "xyzabc{.}xyz{.}",
	},
	{
		name:  "overlap beginning",
		given: "{.}abcd{.}",
		do:    []Edit{Copy(Dot, Rune(1))},
		want:  "a{.}abcd{.}bcd",
	},
	{
		name:  "overlap middle",
		given: "{.}abcd{.}",
		do:    []Edit{Copy(Dot, Rune(2))},
		want:  "ab{.}abcd{.}cd",
	},
	{
		name:  "overlap end",
		given: "{.}abcd{.}",
		do:    []Edit{Copy(Dot, Rune(3))},
		want:  "abc{.}abcd{.}d",
	},
	{
		name:  "update marks",
		given: "{..m}abc{mn}123{no}xyz{opp}",
		do:    []Edit{Copy(Regexp("123"), Regexp("x"))},
		want:  "{m}abc{mn}123{no}x{.}123{.}yz{opp}",
	},
}

func TestEditCopy(t *testing.T) {
	for _, test := range copyTests {
		test.run(t)
	}
}

func TestEditCopyFromString(t *testing.T) {
	for _, test := range copyTests {
		test.runFromString(t)
	}
}

var setTests = []editTest{
	{
		name:  "out of range",
		do:    []Edit{Set(Rune(1), '.')},
		error: "out of range",
	},
	{
		name:  "set dot to beginning",
		given: "abc{.}123{.}xyz",
		do:    []Edit{Set(Rune(0), '.')},
		want:  "{..}abc123xyz",
	},
	{
		name:  "set non-dot to beginning",
		given: "abc{.}123{.}xyz",
		do:    []Edit{Set(Rune(0), 'm')},
		want:  "{mm}abc{.}123{.}xyz",
	},
	{
		name:  "set dot to middle",
		given: "{..}abc123xyz",
		do:    []Edit{Set(Regexp("123"), '.')},
		want:  "abc{.}123{.}xyz",
	},
	{
		name:  "set non-dot to middle",
		given: "abc{.}123{.}xyz",
		do:    []Edit{Set(Regexp("123"), 'm')},
		want:  "abc{.m}123{.m}xyz",
	},
	{
		name:  "set dot to end",
		given: "abc{.}123{.}xyz",
		do:    []Edit{Set(End, '.')},
		want:  "abc123xyz{..}",
	},
	{
		name:  "set non-dot to end",
		given: "abc{.}123{.}xyz",
		do:    []Edit{Set(End, 'm')},
		want:  "abc{.}123{.}xyz{mm}",
	},
	{
		name:  "set space sets dot",
		given: "{..}abc",
		do:    []Edit{Set(Regexp("b"), ' ')},
		want:  "a{.}b{.}c",
	},
}

func TestEditSet(t *testing.T) {
	for _, test := range setTests {
		test.run(t)
	}
}

func TestEditSetFromString(t *testing.T) {
	for _, test := range setTests {
		test.runFromString(t)
	}
}

var printTests = []editTest{
	{
		name:  "out of range",
		do:    []Edit{Print(Rune(1))},
		error: "out of range",
	},
	{
		name:  "print empty range",
		given: "abc{..}xyz",
		do:    []Edit{Print(Dot)},
		want:  "abc{..}xyz",
		print: "",
	},
	{
		name:  "print empty buffer",
		given: "{..}",
		do:    []Edit{Print(All)},
		want:  "{..}",
		print: "",
	},
	{
		name:  "print nonempty buffer",
		given: "{..}abcxyz",
		do:    []Edit{Print(All)},
		want:  "{.}abcxyz{.}",
		print: "abcxyz",
	},
	{
		name:  "print non-empty",
		given: "{..}abcαβξxyz",
		do:    []Edit{Print(Regexp("αβξ"))},
		want:  "abc{.}αβξ{.}xyz",
		print: "αβξ",
	},
}

func TestEditPrint(t *testing.T) {
	for _, test := range printTests {
		test.run(t)
	}
}

func TestEditPrintFromString(t *testing.T) {
	for _, test := range printTests {
		test.runFromString(t)
	}
}

var whereTests = []editTest{
	{
		name:  "out of range",
		do:    []Edit{Where(Rune(1))},
		error: "out of range",
	},
	{
		name:  "where empty buffer",
		given: "{..}",
		do:    []Edit{Where(All)},
		want:  "{..}",
		print: "#0",
	},
	{
		name:  "where beginning point",
		given: "abcxyz{..}",
		do:    []Edit{Where(Rune(0))},
		want:  "{..}abcxyz",
		print: "#0",
	},
	{
		name:  "where middle point",
		given: "{..}abcxyz",
		do:    []Edit{Where(Rune(3))},
		want:  "abc{..}xyz",
		print: "#3",
	},
	{
		name:  "where end point",
		given: "{..}abcxyz",
		do:    []Edit{Where(End)},
		want:  "abcxyz{..}",
		print: "#6",
	},
	{
		name:  "where beginning range",
		given: "abcxyz{..}",
		do:    []Edit{Where(Regexp("abc"))},
		want:  "{.}abc{.}xyz",
		print: "#0,#3",
	},
	{
		name:  "where middle range",
		given: "{..}abcxyz",
		do:    []Edit{Where(Regexp("cx"))},
		want:  "ab{.}cx{.}yz",
		print: "#2,#4",
	},
	{
		name:  "where end range",
		given: "{..}abcxyz",
		do:    []Edit{Where(Regexp("xyz"))},
		want:  "abc{.}xyz{.}",
		print: "#3,#6",
	},
}

func TestEditWhere(t *testing.T) {
	for _, test := range whereTests {
		test.run(t)
	}
}

func TestEditWhereFromString(t *testing.T) {
	for _, test := range whereTests {
		test.runFromString(t)
	}
}

var whereLineTests = []editTest{
	{
		name:  "out of range",
		do:    []Edit{WhereLine(Rune(1))},
		error: "out of range",
	},
	{
		name:  "where line empty buffer",
		given: "{..}",
		do:    []Edit{WhereLine(All)},
		want:  "{..}",
		print: "1",
	},
	{
		name:  "where line 0",
		given: "{..}abc\nxyz\n123\n",
		do:    []Edit{WhereLine(Line(0))},
		want:  "{..}abc\nxyz\n123\n",
		print: "1",
	},
	{
		name:  "where line 1",
		given: "{..}abc\nxyz\n123\n",
		do:    []Edit{WhereLine(Line(1))},
		want:  "{.}abc\n{.}xyz\n123\n",
		print: "1",
	},
	{
		name:  "where line 2",
		given: "{..}abc\nxyz\n123\n",
		do:    []Edit{WhereLine(Line(2))},
		want:  "abc\n{.}xyz\n{.}123\n",
		print: "2",
	},
	{
		name:  "where line 3",
		given: "{..}abc\nxyz\n123\n",
		do:    []Edit{WhereLine(Line(3))},
		want:  "abc\nxyz\n{.}123\n{.}",
		print: "3",
	},
	{
		name:  "where line empty at end",
		given: "{..}abc\nxyz\n123\n",
		do:    []Edit{WhereLine(Line(4))},
		want:  "abc\nxyz\n123\n{..}",
		print: "4",
	},
	{
		name:  "where line multi-line",
		given: "{..}abc\nxyz\n123\n",
		do:    []Edit{WhereLine(Line(2).To(Line(3)))},
		want:  "abc\n{.}xyz\n123\n{.}",
		print: "2,3",
	},
}

func TestEditWhereLine(t *testing.T) {
	for _, test := range whereLineTests {
		test.run(t)
	}
}

func TestEditWhereLineFromString(t *testing.T) {
	for _, test := range whereLineTests {
		test.runFromString(t)
	}
}

var substituteTests = []editTest{
	{
		name:  "out of range",
		do:    []Edit{Sub(Rune(1), "a", "b")},
		error: "out of range",
	},
	{
		name:  "bad regexp",
		do:    []Edit{Substitute{A: Rune(0), RE: "*"}},
		error: "missing",
	},
	{
		name:  "empty buffer",
		given: "{..}",
		do:    []Edit{Sub(All, ".*", "abc")},
		want:  "{.}abc{.}",
	},
	{
		name:  "empty regexp",
		given: "{.}xyz{.}",
		do:    []Edit{Sub(All, "", "abc")},
		want:  "{.}abcxyz{.}",
	},
	{
		name:  "delete everything",
		given: "{.}xyz{.}",
		do:    []Edit{Sub(All, ".*", "")},
		want:  "{..}",
	},
	{
		name:  "replace beginning",
		given: "{.}Hi 世界{.}",
		do:    []Edit{Sub(All, "Hi", "Hello")},
		want:  "{.}Hello 世界{.}",
	},
	{
		name:  "replace middle",
		given: "{.}Hello 世界{.}",
		do:    []Edit{Sub(All, " ", " there ")},
		want:  "{.}Hello there 世界{.}",
	},
	{
		name:  "replace end",
		given: "{.}Hello 世界{.}",
		do:    []Edit{Sub(All, "世界", "World")},
		want:  "{.}Hello World{.}",
	},
	{
		name:  "not global",
		given: "{.}aaa{.}",
		do:    []Edit{Sub(All, "a", "b")},
		want:  "{.}baa{.}",
	},
	{
		name:  "global all in a row",
		given: "{.}aaa{.}",
		do:    []Edit{SubGlobal(All, "a", "b")},
		want:  "{.}bbb{.}",
	},
	{
		name:  "global not in a row",
		given: "{.}abaca{.}",
		do:    []Edit{SubGlobal(All, "a", "x")},
		want:  "{.}xbxcx{.}",
	},
	{
		name:  "restricted to address",
		given: "a{.}a{.}a",
		do:    []Edit{SubGlobal(Dot, "a", "b")},
		want:  "a{.}b{.}a",
	},
	{
		name:  "update marks",
		given: "{..}:abc:;123;,xyz,//",
		do:    []Edit{Sub(All, "2", "xxx")},
		want:  "{.}:abc:;1xxx3;,xyz,//{.}",
	},
	{
		name:  "subexprs",
		given: "{..}abcdefghi",
		do:    []Edit{Sub(All, "(abc)(def)(ghi)", `$0 $1 $2 $3`)},
		want:  "{.}abcdefghi abc def ghi{.}",
	},
	{
		name:  "subexprs and literals",
		given: "{..}abcxyz",
		do:    []Edit{Sub(All, "(abc)(xyz)", `${2}123$1`)},
		want:  "{.}xyz123abc{.}",
	},
	{
		name:  "missing subexpr",
		given: "{..}abc",
		do:    []Edit{Sub(All, "abc(xyz)?", `123${1}456`)},
		want:  "{.}123456{.}",
	},
	{
		name:  "missing subexpr",
		given: "{..}abc",
		do:    []Edit{Sub(All, "abc(xyz)?", `123${2}456`)},
		want:  "{.}123456{.}",
	},
	{
		name:  "subexpr is entire match",
		given: "{..}...===...\n",
		do:    []Edit{Sub(All, "(=+)", `---${1}---`)},
		want:  "{.}...---===---...\n{.}",
	},
	{
		name:  "non-ASCII submatch replace",
		given: "{..}α",
		do:    []Edit{Sub(All, "(.)", `${1}β`)},
		want:  "{.}αβ{.}",
	},
	{
		name:  "replace every rune",
		given: "{..}Hello 世界",
		do:    []Edit{SubGlobal(All, "(.)", `${1}x`)},
		want:  "{.}Hxexlxlxox x世x界x{.}",
	},
	{
		name: "replace whitespace",
		given: "{..}a b		c",
		do:   []Edit{SubGlobal(All, `\s+`, `\`)},
		want: `{.}a\b\c{.}`,
	},
	{
		name:  "global empty first match",
		given: "{..}b",
		do:    []Edit{SubGlobal(All, "a*", "x")},
		want:  "{.}xbx{.}",
	},
	{
		name:  "global empty last match",
		given: "{..}abab",
		do:    []Edit{SubGlobal(All, "a*", "x")},
		want:  "{.}xbxbx{.}",
	},
	{
		name:  "from negative",
		given: "{..}aaaa",
		do:    []Edit{Substitute{A: All, RE: "a", With: "b", From: -1}},
		want:  "{.}baaa{.}",
	},
	{
		name:  "from 0",
		given: "{..}aaaa",
		do:    []Edit{Substitute{A: All, RE: "a", With: "b", From: 0}},
		want:  "{.}baaa{.}",
	},
	{
		name:  "from 1",
		given: "{..}aaaa",
		do:    []Edit{Substitute{A: All, RE: "a", With: "b", From: 1}},
		want:  "{.}baaa{.}",
	},
	{
		name:  "from 2",
		given: "{..}aaaa",
		do:    []Edit{Substitute{A: All, RE: "a", With: "b", From: 2}},
		want:  "{.}abaa{.}",
	},
	{
		name:  "from 3",
		given: "{..}aaaa",
		do:    []Edit{Substitute{A: All, RE: "a", With: "b", From: 3}},
		want:  "{.}aaba{.}",
	},
	{
		name:  "from 3 global",
		given: "{..}aaaa",
		do:    []Edit{Substitute{A: All, RE: "a", With: "b", From: 3, Global: true}},
		want:  "{.}aabb{.}",
	},

	// Test regexp escaping.
	{
		name:  "only delimiter regexp",
		given: "{..}/",
		do:    []Edit{Sub(All, "/", "xyz")},
		want:  "{.}xyz{.}",
	},
	{
		name:  "delimiter regexp",
		given: "{..}/abc/",
		do:    []Edit{Sub(All, "/abc/", "xyz")},
		want:  "{.}xyz{.}",
	},
	{
		name:  "escaped delimiter regexp",
		given: "{..}/abc/",
		do:    []Edit{Sub(All, `\/abc\/`, "xyz")},
		want:  "{.}xyz{.}",
	},
	{
		name:  `only \`,
		given: `{..}\`,
		do:    []Edit{Sub(All, `\`, "xyz")},
		want:  "{.}xyz{.}",
	},
	{
		name:  `trailing \`,
		given: `{..}abc\`,
		do:    []Edit{Sub(All, `abc\`, "xyz")},
		want:  "{.}xyz{.}",
	},
	{
		name:  "raw newline regexp",
		given: "{..}abc\n",
		do:    []Edit{Sub(All, "abc\n", "xyz")},
		want:  "{.}xyz{.}",
	},
	{
		name:  "escaped raw newline regexp",
		given: "{..}abc\n",
		do:    []Edit{Sub(All, "abc\\\n", "xyz")},
		want:  "{.}xyz{.}",
	},
	{
		name:  "literal newline regexp",
		given: "{..}abc\n",
		do:    []Edit{Sub(All, `abc\n`, "xyz")},
		want:  "{.}xyz{.}",
	},
	{
		name:  `escape \ then n regexp`,
		given: `{..}abc\n`,
		do:    []Edit{Sub(All, `abc\\n`, "xyz")},
		want:  `{.}xyz{.}`,
	},

	// Test with escaping.
	{
		name:  "only delimiter with",
		given: "{..}abc",
		do:    []Edit{Sub(All, "abc", "/")},
		want:  "{.}/{.}",
	},
	{
		name:  "delimiter with",
		given: "{..}abc",
		do:    []Edit{Sub(All, "abc", "/xyz/")},
		want:  "{.}/xyz/{.}",
	},
	{
		name:  "escaped delimiter with",
		given: "{..}abc",
		do:    []Edit{Sub(All, "abc", `\/xyz\/`)},
		want:  `{.}\/xyz\/{.}`,
	},
	{
		name:  "various escaped runes with",
		given: "{..}abc",
		do:    []Edit{Sub(All, "abc", `\x\y\z`)},
		want:  `{.}\x\y\z{.}`,
	},
	{
		name:  "only escape with",
		given: "{..}abc",
		do:    []Edit{Sub(All, "abc", `\`)},
		want:  `{.}\{.}`,
	},
	{
		name:  "escape at end with",
		given: "{..}abc",
		do:    []Edit{Sub(All, "abc", `xyz\`)},
		want:  `{.}xyz\{.}`,
	},
	{
		name:  "raw newline with",
		given: "{..}abc",
		do:    []Edit{Sub(All, "abc", "xyz\n")},
		want:  "{.}xyz\n{.}",
	},
	{
		name:  "escaped raw newline with",
		given: "{..}abc",
		do:    []Edit{Sub(All, "abc", "xyz\\\n")},
		want:  "{.}xyz\\\n{.}",
	},
	{
		name:  "literal newline with",
		given: "{..}abc",
		do:    []Edit{Sub(All, "abc", "xyz\n")},
		want:  "{.}xyz\n{.}",
	},
	{
		name:  `escape \ then n with`,
		given: "{..}abc",
		do:    []Edit{Sub(All, `abc`, `xyz\\n`)},
		want:  `{.}xyz\\n{.}`,
	},
}

func TestEditSubstitute(t *testing.T) {
	for _, test := range substituteTests {
		test.run(t)
	}
}

func TestEditSubstituteFromString(t *testing.T) {
	for _, test := range substituteTests {
		test.runFromString(t)
	}
}

var pipeFromTests = []editTest{
	{
		name:  "out of range",
		do:    []Edit{PipeFrom(Rune(1), "echo hi")},
		error: "out of range",
	},
	{
		name:  "command fails",
		do:    []Edit{PipeFrom(End, "false")},
		error: "exit status 1",
	},
	{
		name:  "print stderr",
		do:    []Edit{PipeFrom(End, "echo -n stderr 1>&2")},
		print: "stderr",
	},
	{
		name:  "fill empty buffer",
		given: "{..}",
		do:    []Edit{PipeFrom(All, "echo -n Hello 世界")},
		want:  "{.}Hello 世界{.}",
	},
	{
		name:  "insert before",
		given: "{..} 世界",
		do:    []Edit{PipeFrom(Dot, "echo -n Hello")},
		want:  "{.}Hello{.} 世界",
	},
	{
		name:  "insert middle",
		given: "{..}He 世界",
		do:    []Edit{PipeFrom(Regexp("He").Plus(Rune(0)), "echo -n llo")},
		want:  "He{.}llo{.} 世界",
	},
	{
		name:  "insert after",
		given: "{..}Hello ",
		do:    []Edit{PipeFrom(End, "echo -n 世界")},
		want:  "Hello {.}世界{.}",
	},
}

func TestEditPipeFrom(t *testing.T) {
	for _, test := range pipeFromTests {
		test.run(t)
	}
}

func TestEditPipeFromFromString(t *testing.T) {
	for _, test := range pipeFromTests {
		test.runFromString(t)
	}
}

var pipeToTests = []editTest{
	{
		name:  "out of range",
		do:    []Edit{PipeTo(Rune(1), "echo hi")},
		error: "out of range",
	},
	{
		name:  "command fails",
		do:    []Edit{PipeTo(End, "false")},
		error: "exit status 1",
	},
	{
		name:  "print stdout",
		do:    []Edit{PipeTo(End, "echo -n stdout")},
		print: "stdout",
	},
	{
		name:  "print stderr",
		do:    []Edit{PipeTo(End, "echo -n stderr 1>&2")},
		print: "stderr",
	},
	{
		name:  "empty",
		given: "{..}",
		do:    []Edit{PipeTo(All, "cat")},
		print: "",
		want:  "{..}",
	},
	{
		name:  "non-empty",
		given: "{.}Hello 世界{.}",
		do:    []Edit{PipeTo(Dot, "cat")},
		print: "Hello 世界",
		want:  "{.}Hello 世界{.}",
	},
}

func TestEditPipeTo(t *testing.T) {
	for _, test := range pipeToTests {
		test.run(t)
	}
}

func TestEditPipeToFromString(t *testing.T) {
	for _, test := range pipeToTests {
		test.runFromString(t)
	}
}

var pipeTests = []editTest{
	{
		name:  "out of range",
		do:    []Edit{Pipe(Rune(1), "echo hi")},
		error: "out of range",
	},
	{
		name:  "command fails",
		do:    []Edit{Pipe(End, "false")},
		error: "exit status 1",
	},
	{
		name:  "print stderr",
		do:    []Edit{Pipe(End, "echo -n stderr 1>&2")},
		print: "stderr",
	},
	{
		name:  "empty",
		given: "{..}",
		do:    []Edit{Pipe(All, "cat")},
		want:  "{..}",
	},
	{
		name:  "empty inserts",
		given: "{..}",
		do:    []Edit{Pipe(All, "echo -n Hello 世界")},
		want:  "{.}Hello 世界{.}",
	},
	{
		name:  "empty replaces",
		given: "{.}Hello 世界{.}",
		do:    []Edit{Pipe(Dot, "sed 's/世界/World/'")},
		want:  "{.}Hello World{.}",
	},
}

func TestEditPipe(t *testing.T) {
	for _, test := range pipeTests {
		test.run(t)
	}
}

func TestEditPipeFromString(t *testing.T) {
	for _, test := range pipeTests {
		test.runFromString(t)
	}
}

func TestPipeDefaultShell(t *testing.T) {
	// Unset the shell and make sure that everything still works.
	if err := os.Unsetenv("SHELL"); err != nil {
		t.Fatal(err)
	}
	for _, test := range pipeTests {
		test.run(t)
	}
}

var undoTests = []editTest{
	{
		name:  "empty undo 1",
		given: "{..}",
		do:    []Edit{Undo(1)},
		want:  "{..}",
	},
	{
		name:  "empty undo 100",
		given: "{..}",
		do:    []Edit{Undo(100)},
		want:  "{..}",
	},
	{
		name:  "1 append, undo 1",
		given: "{..}",
		do:    []Edit{Append(End, "abc"), Undo(1)},
		want:  "{..}",
	},
	{
		name:  "1 append, undo 0",
		given: "{..}",
		do:    []Edit{Append(End, "abc"), Undo(0)},
		want:  "{..}",
	},
	{
		name:  "1 append, undo -1",
		given: "{..}",
		do:    []Edit{Append(End, "abc"), Undo(-1)},
		want:  "{..}",
	},
	{
		name:  "2 appends, undo 1",
		given: "{..}",
		do:    []Edit{Append(End, "abc"), Append(End, "xyz"), Undo(1)},
		want:  "abc{..}",
	},
	{
		name:  "2 appends, undo 2",
		given: "{..}",
		do:    []Edit{Append(End, "abc"), Append(End, "xyz"), Undo(2)},
		want:  "{..}",
	},
	{
		name:  "2 appends, undo 3",
		given: "{..}",
		do:    []Edit{Append(End, "abc"), Append(End, "xyz"), Undo(3)},
		want:  "{..}",
	},
	{
		name:  "1 delete, undo 1",
		given: "{.}abc{.}",
		do:    []Edit{Delete(All), Undo(1)},
		want:  "{.}abc{.}",
	},
	{
		name:  "2 deletes, undo 1",
		given: "{.}abc{.}",
		do: []Edit{
			Delete(Rune(2).To(End)),
			Delete(Rune(1).To(End)),
			Undo(1),
		},
		want: "a{.}b{.}",
	},
	{
		name:  "2 deletes, undo 2",
		given: "{.}abc{.}",
		do: []Edit{
			Delete(Rune(2).To(End)),
			Delete(Rune(1).To(End)),
			Undo(2),
		},
		want: "ab{.}c{.}",
	},
	{
		name:  "delete middle, undo",
		given: "{.}abc{.}",
		do: []Edit{
			Delete(Rune(1).To(Rune(2))),
			Undo(1),
		},
		want: "a{.}b{.}c",
	},
	{
		name:  "multi-change undo",
		given: "{.}a.a.a{.}",
		do: []Edit{
			SubGlobal(All, "[.]", "z"),
			Undo(1),
		},
		want: "a{.}.a.{.}a",
	},
	{
		name:  "update marks",
		given: "{.}{z}ZZZ{z}{a}AbA{a}{x}XXX{x}{.}",
		do: []Edit{
			Delete(Regexp("b")),
			Undo(1),
		},
		want: "{z}ZZZ{z}{a}A{.}b{.}A{a}{x}XXX{x}",
	},
}

func TestEditUndo(t *testing.T) {
	for _, test := range undoTests {
		test.run(t)
	}
}

func TestEditUndoFromString(t *testing.T) {
	for _, test := range undoTests {
		test.runFromString(t)
	}
}

var redoTests = []editTest{
	{
		name:  "empty redo 1",
		given: "{.}abc{.}",
		do:    []Edit{Redo(1)},
		want:  "{.}abc{.}",
	},
	{
		name:  "empty redo 100",
		given: "{.}abc{.}",
		do:    []Edit{Redo(100)},
		want:  "{.}abc{.}",
	},
	{
		name:  "undo 1 append redo 1",
		given: "{..}",
		do:    []Edit{Append(End, "abc"), Undo(1), Redo(1)},
		want:  "{.}abc{.}",
	},
	{
		name:  "undo 1 append redo 0",
		given: "{..}",
		do:    []Edit{Append(End, "abc"), Undo(1), Redo(0)},
		want:  "{.}abc{.}",
	},
	{
		name:  "undo 1 append redo -1",
		given: "{..}",
		do:    []Edit{Append(End, "abc"), Undo(1), Redo(-1)},
		want:  "{.}abc{.}",
	},
	{
		name:  "undo 1 append redo 2",
		given: "{..}",
		do:    []Edit{Append(End, "abc"), Undo(1), Redo(2)},
		want:  "{.}abc{.}",
	},
	{
		name:  "undo 2 appends redo 1",
		given: "{..}",
		do:    []Edit{Append(End, "abc"), Append(End, "xyz"), Undo(2), Redo(1)},
		want:  "{.}abcxyz{.}",
	},
	{
		name:  "undo undo appends redo 1",
		given: "{..}",
		do:    []Edit{Append(End, "abc"), Append(End, "xyz"), Undo(1), Undo(1), Redo(1)},
		want:  "{.}abc{.}",
	},
	{
		name:  "multi-change redo",
		given: "{.}a.a.a{.}",
		do: []Edit{
			SubGlobal(All, "[.]", "z"),
			Undo(1),
			Redo(1),
		},
		want: "a{.}zaz{.}a",
	},
	{
		name:  "update marks",
		given: "{.}{z}ZZZ{z}{a}AbA{a}{x}XXX{x}{.}",
		do: []Edit{
			Delete(Regexp("b")),
			Undo(1),
			Redo(1),
		},
		want: "{z}ZZZ{z}{a}A{..}A{a}{x}XXX{x}",
	},
	{
		name:  "append append undo undo redo undo redo",
		given: "{..}",
		do: []Edit{
			Append(End, "abc"),
			Append(End, "xyz"),
			Undo(1),
			Redo(1),
			Undo(1),
			Redo(1),
		},
		want: "abc{.}xyz{.}",
	},
}

func TestEditRedo(t *testing.T) {
	for _, test := range redoTests {
		test.run(t)
	}
}

func TestEditRedoFromString(t *testing.T) {
	for _, test := range redoTests {
		test.runFromString(t)
	}
}

var updateMarkTests = []editTest{
	{
		name:  "delete after mark",
		given: "{m}abc{m}___{.}xyz{.}",
		do:    []Edit{Delete(Dot)},
		want:  "{m}abc{m}___{..}",
	},
	{
		name:  "append after mark",
		given: "{m}abc{m}___{..}",
		do:    []Edit{Append(Dot, "xyz")},
		want:  "{m}abc{m}___{.}xyz{.}",
	},
	{
		name:  "change after mark",
		given: "{m}abc{m}___{.}xyz{.}",
		do:    []Edit{Change(Dot, "123")},
		want:  "{m}abc{m}___{.}123{.}",
	},
	{
		name:  "delete inside mark",
		given: "{m}a{.}b{.}c{m}",
		do:    []Edit{Delete(Dot)},
		want:  "{m}a{..}c{m}",
	},
	{
		name:  "append inside mark",
		given: "{m}a{.}b{.}c{m}",
		do:    []Edit{Append(Dot, "αβξ")},
		want:  "{m}ab{.}αβξ{.}c{m}",
	},
	{
		name:  "change inside mark",
		given: "{m}a{.}b{.}c{m}",
		do:    []Edit{Change(Dot, "αβξ")},
		want:  "{m}a{.}αβξ{.}c{m}",
	},
	{
		name:  "delete before mark",
		given: "{.}xyz{.}___{m}abc{m}",
		do:    []Edit{Delete(Dot)},
		want:  "{..}___{m}abc{m}",
	},
	{
		name:  "append before mark",
		given: "{..}___{m}abc{m}",
		do:    []Edit{Append(Dot, "xyz")},
		want:  "{.}xyz{.}___{m}abc{m}",
	},
	{
		name:  "change before mark",
		given: "{.}xyz{.}___{m}abc{m}",
		do:    []Edit{Change(Dot, "123")},
		want:  "{.}123{.}___{m}abc{m}",
	},
	{
		name:  "delete prefix of mark",
		given: "{.}xyz{m}a{.}bc{m}",
		do:    []Edit{Delete(Dot)},
		want:  "{..m}bc{m}",
	},
	{
		name:  "append prefix of mark",
		given: "{.}xyz{m}a{.}bc{m}",
		do:    []Edit{Append(Dot, "123")},
		want:  "xyz{m}a{.}123{.}bc{m}",
	},
	{
		name:  "change prefix of mark",
		given: "{.}xyz{m}a{.}bc{m}",
		do:    []Edit{Change(Dot, "123")},
		want:  "{.}123{.}{m}bc{m}",
	},
	{
		name:  "delete suffix of mark",
		given: "{m}ab{.}c{m}xyz{.}",
		do:    []Edit{Delete(Dot)},
		want:  "{m}ab{..m}",
	},
	{
		name:  "append suffix of mark",
		given: "{m}ab{.}c{m}xyz{.}",
		do:    []Edit{Append(Dot, "123")},
		want:  "{m}abc{m}xyz{.}123{.}",
	},
	{
		name:  "change suffix of mark",
		given: "{m}ab{.}c{m}xyz{.}",
		do:    []Edit{Change(Dot, "123")},
		want:  "{m}ab{.m}123{.}",
	},
	{
		name:  "delete beginning of mark",
		given: "{.}xyz{m.}abc{m}",
		do:    []Edit{Delete(Dot)},
		want:  "{..m}abc{m}",
	},
	{
		name:  "append beginning of mark",
		given: "{.}xyz{m.}abc{m}",
		do:    []Edit{Append(Dot, "123")},
		want:  "xyz{.}123{.}{m}abc{m}",
	},
	{
		name:  "change beginning of mark",
		given: "{.}xyz{m.}abc{m}",
		do:    []Edit{Change(Dot, "123")},
		want:  "{.}123{.m}abc{m}",
	},
	{
		name:  "delete end of mark",
		given: "{m}abc{.m}xyz{.}",
		do:    []Edit{Delete(Dot)},
		want:  "{m}abc{..m}",
	},
	{
		name:  "append end of mark",
		given: "{m}abc{.m}xyz{.}",
		do:    []Edit{Append(Dot, "123")},
		want:  "{m}abc{m}xyz{.}123{.}",
	},
	{
		name:  "change end of mark",
		given: "{m}abc{.m}xyz{.}",
		do:    []Edit{Change(Dot, "123")},
		want:  "{m}abc{.m}123{.}",
	},
}

func TestUpdateMarks(t *testing.T) {
	for _, test := range updateMarkTests {
		test.run(t)
	}
}

type editTest struct {
	name string
	// Given describes the editor state before the edits,
	// and want describes the desired editor state after the edits.
	//
	// The editor state descriptions describe
	// the contents of the buffer and the editor's marks.
	// Runes that are not between { and } represent the buffer contents.
	// Each rune between { and } represent
	// the beginning (first occurrence)
	// or end (second occurrence)
	// of a mark region with the name of the rune.
	//
	// For example:
	// 	"{mm}abc{.}xyz{.n}123{n}αβξ"
	// Is a buffer with the contents:
	// 	"abcxyz123αβξ"
	// The mark m is the empty string at the beginning of the buffer.
	// The mark . contains the text "xyz".
	// The mark n contains the text "123".
	given, want string
	// Print is the desired text printed to the io.Writer passed to Editor.Do
	// after all edits are performed.
	print string
	// Error is the first error expected when performing the Edits.
	// If error is the empty string, no error is expected,
	// otherwise error is a regular expression of the first error expected
	// when performing the do Edits in sequence.
	// After an error is encountered, no further Edits in the sequence are performed.
	error string
	// Do is the sequence of edits to perform for the test.
	do []Edit
}

func (test editTest) run(t *testing.T) {
	ed := newTestEditor(test.given)
	defer ed.buf.Close()

	print := bytes.NewBuffer(nil)
	for i, e := range test.do {
		err := ed.Do(e, print)
		if !matchesError(test.error, err) {
			t.Errorf("%s: Do(do[%d]=%q)=%v, want %q", test.name, i, e, err, test.error)
		}
		if err != nil {
			break
		}
	}
	if !ed.hasState(test.want) {
		t.Errorf("%s: got %q, want %q", test.name, ed.stateString(), test.want)
	}
	if got := print.String(); got != test.print {
		t.Errorf("%s: printed %q, want %q", test.name, got, test.print)
	}
}

// RunFromString replaces each edit with the parsed of its string and calls run.
func (test editTest) runFromString(t *testing.T) {
	for i, e := range test.do {
		rs := strings.NewReader(e.String())
		E, err := Ed(rs)
		if err != nil {
			// If the test expects an error, it is typically expected from Do.
			// Here, we allow it from Ed.
			// For example, a bad regexp will error will show when Ed
			// parses the bad regexp string, before we even get to Do.
			if !matchesError(test.error, err) {
				t.Errorf("%s: Ed(do[%d]=%q)=%v, want nil", test.name, i, e, err)
			}
			return
		}
		if rs.Len() > 0 {
			// Nothing should be left over after the parse…
			left, err := ioutil.ReadAll(rs)
			if err != nil {
				panic(err)
			}
			// …  except perhaps \n for pipe edits.
			if len(left) != 1 || left[0] != '\n' {
				t.Errorf("%s: Ed(do[%d]=%q) left over %q, want \"\"", test.name, i, e, string(left))
			}
		}
		test.do[i] = E
	}
	test.run(t)
}

func newTestEditor(str string) *Editor {
	contents, marks := parseState(str)
	ed := NewEditor(NewBuffer())
	r := strings.NewReader(contents)
	if _, err := ed.ReaderFrom(All).ReadFrom(r); err != nil {
		ed.buf.Close()
		panic(err)
	}
	ed.marks = marks
	return ed
}

func (ed *Editor) hasState(want string) bool {
	want, marks := parseState(want)
	if got := ed.String(); got != want {
		return false
	}
	for m, a := range ed.marks {
		if marks[m] != a {
			return false
		}
		delete(marks, m)
	}
	return len(marks) == 0
}

func (ed *Editor) stateString() string {
	marks := make(map[int64]RuneSlice)
	for m, a := range ed.marks {
		marks[a.from] = append(marks[a.from], m)
		marks[a.to] = append(marks[a.to], m)
	}
	var c []rune
	str := []rune(ed.String())
	for i := 0; i < len(str)+1; i++ {
		if ms := marks[int64(i)]; len(ms) > 0 {
			sort.Sort(ms)
			c = append(c, '{')
			c = append(c, ms...)
			c = append(c, '}')
		}
		if i < len(str) {
			c = append(c, str[i])
		}
	}
	return string(c)
}

type RuneSlice []rune

func (rs RuneSlice) Len() int           { return len(rs) }
func (rs RuneSlice) Less(i, j int) bool { return rs[i] < rs[j] }
func (rs RuneSlice) Swap(i, j int)      { rs[i], rs[j] = rs[j], rs[i] }

func parseState(str string) (string, map[rune]addr) {
	var mark bool
	var contents []rune
	marks := make(map[rune]addr)
	count := make(map[rune]int)
	for _, r := range str {
		switch {
		case !mark && r == '{':
			mark = true
		case mark && r == '}':
			mark = false
		case mark:
			count[r]++
			at := int64(len(contents))
			if a, ok := marks[r]; !ok {
				marks[r] = addr{from: at}
			} else {
				marks[r] = addr{from: a.from, to: at}
			}
		default:
			contents = append(contents, r)
		}
	}
	for m, c := range count {
		if c != 2 {
			panic(fmt.Sprintf("%q, mark %c appears %d times", str, m, c))
		}
	}
	return string(contents), marks
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
