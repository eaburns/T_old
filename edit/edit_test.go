// Copyright © 2015, The T Authors.

package edit

import (
	"bytes"
	"io/ioutil"
	"math"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"testing"
)

func TestEscape(t *testing.T) {
	tests := []struct {
		str, want string
	}{
		{str: "", want: "//"},
		{str: "Hello, World!", want: "/Hello, World!/"},
		{str: "Hello, 世界!", want: "/Hello, 世界!/"},
		{str: "/Hello, World!/", want: `/\/Hello, World!\//`},
		{str: "Hello,\nWorld!", want: `/Hello,\nWorld!/`},
		{str: "/Hello,\nWorld!/", want: `/\/Hello,\nWorld!\//`},
		{str: "Hello,\nWorld!\n", want: "\nHello,\nWorld!\n.\n"},
	}
	for _, test := range tests {
		if got := escape(test.str); got != test.want {
			t.Errorf("escape(%q)=%q, want %q", test.str, got, test.want)
		}
	}
}

func TestChangeEdit(t *testing.T) {
	tests := []eTest{
		{
			init: "Hello, 世界!",
			e:    Change(Rune(0), ""),
			want: "Hello, 世界!",
			dot:  addr{0, 0},
		},
		{
			init: "Hello, 世界!",
			e:    Change(All, ""),
			want: "",
			dot:  addr{0, 0},
		},
		{
			init: "Hello, 世界!",
			e:    Change(Rune(0), "XYZ"),
			want: "XYZHello, 世界!",
			dot:  addr{0, 3},
		},
		{
			init: "Hello, 世界!",
			e:    Change(Rune(1), "XYZ"),
			want: "HXYZello, 世界!",
			dot:  addr{1, 4},
		},
		{
			init: "Hello, 世界!",
			e:    Change(End, "XYZ"),
			want: "Hello, 世界!XYZ",
			dot:  addr{10, 13},
		},
		{
			init: "Hello, 世界!",
			e:    Change(Rune(0).To(Rune(1)), "XYZ"),
			want: "XYZello, 世界!",
			dot:  addr{0, 3},
		},
		{
			init: "Hello, 世界!",
			e:    Change(Rune(1).To(End.Minus(Rune(1))), "XYZ"),
			want: "HXYZ!",
			dot:  addr{1, 4},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestAppendEdit(t *testing.T) {
	tests := []eTest{
		{
			init: "Hello, 世界!",
			e:    Append(Rune(0), ""),
			want: "Hello, 世界!",
			dot:  addr{0, 0},
		},
		{
			init: "Hello,",
			e:    Append(All, " 世界!"),
			want: "Hello, 世界!",
			dot:  addr{6, 10},
		},
		{
			init: " 世界!",
			e:    Append(Rune(0), "Hello,"),
			want: "Hello, 世界!",
			dot:  addr{0, 6},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestInsertEdit(t *testing.T) {
	tests := []eTest{
		{
			init: "Hello, 世界!",
			e:    Insert(Rune(0), ""),
			want: "Hello, 世界!",
			dot:  addr{0, 0},
		},
		{
			init: " 世界!",
			e:    Insert(All, "Hello,"),
			want: "Hello, 世界!",
			dot:  addr{0, 6},
		},
		{
			init: "Hello,",
			e:    Insert(End, " 世界!"),
			want: "Hello, 世界!",
			dot:  addr{6, 10},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestDeleteEdit(t *testing.T) {
	tests := []eTest{
		{
			init: "",
			e:    Delete(All),
			want: "",
			dot:  addr{0, 0},
		},
		{
			init: "Hello, 世界!",
			e:    Delete(All),
			want: "",
			dot:  addr{0, 0},
		},
		{
			init: "Hello, 世界!",
			e:    Delete(Rune(0)),
			want: "Hello, 世界!",
			dot:  addr{0, 0},
		},
		{
			init: "XYZHello, 世界!",
			e:    Delete(Rune(0).To(Rune(3))),
			want: "Hello, 世界!",
			dot:  addr{0, 0},
		},
		{
			init: "Hello,XYZ 世界!",
			e:    Delete(Rune(6).To(Rune(9))),
			want: "Hello, 世界!",
			dot:  addr{6, 6},
		},
		{
			init: "Hello, 世界!XYZ",
			e:    Delete(Rune(10).To(Rune(13))),
			want: "Hello, 世界!",
			dot:  addr{10, 10},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestMoveEdit(t *testing.T) {
	tests := []eTest{
		{init: "abc", e: Move(Regexp("/abc/"), Rune(0)), want: "abc", dot: addr{0, 3}},
		{init: "abc", e: Move(Regexp("/abc/"), Rune(1)), err: "overlap"},
		{init: "abc", e: Move(Regexp("/abc/"), Rune(2)), err: "overlap"},
		{init: "abc", e: Move(Regexp("/abc/"), Rune(3)), want: "abc", dot: addr{0, 3}},
		{init: "abcdef", e: Move(Regexp("/abc/"), End), want: "defabc", dot: addr{3, 6}},
		{init: "abcdef", e: Move(Regexp("/def/"), Line(0)), want: "defabc", dot: addr{0, 3}},
		{init: "abc\ndef\nghi", e: Move(Regexp("/def/"), Line(3)), want: "abc\n\nghidef", dot: addr{8, 11}},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestCopyEdit(t *testing.T) {
	tests := []eTest{
		{init: "abc", e: Copy(Regexp("/abc/"), End), want: "abcabc", dot: addr{3, 6}},
		{init: "abc", e: Copy(Regexp("/abc/"), Line(0)), want: "abcabc", dot: addr{0, 3}},
		{init: "abc", e: Copy(Regexp("/abc/"), Rune(1)), want: "aabcbc", dot: addr{1, 4}},
		{init: "abcdef", e: Copy(Regexp("/abc/"), Rune(4)), want: "abcdabcef", dot: addr{4, 7}},
		{init: "abc\ndef\nghi", e: Copy(Regexp("/def/"), Line(1)), want: "abc\ndefdef\nghi", dot: addr{4, 7}},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestSetEdit(t *testing.T) {
	const s = "Hello, 世界!"
	tests := []eTest{
		{e: Set(All, '.'), dot: addr{0, 0}},
		{e: Set(All, 'm'), marks: map[rune]addr{'m': addr{0, 0}}},
		{init: s, want: s, e: Set(All, '.'), dot: addr{0, 10}},
		{init: s, want: s, e: Set(All, ' '), dot: addr{0, 10}},
		{init: s, want: s, e: Set(All, '\t'), dot: addr{0, 10}},
		{init: s, want: s, e: Set(All, 'a'), marks: map[rune]addr{'a': addr{0, 10}}},
		{init: s, want: s, e: Set(All, 'α'), marks: map[rune]addr{'α': addr{0, 10}}},
		{init: s, want: s, e: Set(Regexp("/Hello"), 'a'), marks: map[rune]addr{'a': addr{0, 5}}},
		{init: s, want: s, e: Set(Line(0), 'z'), marks: map[rune]addr{'z': addr{0, 0}}},
		{init: s, want: s, e: Set(End, 'm'), marks: map[rune]addr{'m': addr{10, 10}}},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestPrintEdit(t *testing.T) {
	const s = "Hello, 世界!"
	tests := []eTest{
		{e: Print(All), print: "", dot: addr{0, 0}},
		{init: s, want: s, e: Print(All), print: s, dot: addr{0, 10}},
		{init: s, want: s, e: Print(End), print: "", dot: addr{10, 10}},
		{init: s, want: s, e: Print(Regexp("/H/")), print: "H", dot: addr{0, 1}},
		{init: s, want: s, e: Print(Regexp("/Hello/")), print: "Hello", dot: addr{0, 5}},
		{init: s, want: s, e: Print(Regexp("/世界/")), print: "世界", dot: addr{7, 9}},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestWhereEdit(t *testing.T) {
	const s = "Hello\n 世界!"
	tests := []eTest{
		{e: Where(All), print: "#0", dot: addr{0, 0}},
		{init: "H\ne\nl\nl\no\n 世\n界\n!", want: "H\ne\nl\nl\no\n 世\n界\n!",
			e: Where(All), print: "#0,#16", dot: addr{0, 16}},
		{init: s, want: s, e: Where(All), print: "#0,#10", dot: addr{0, 10}},
		{init: s, want: s, e: Where(End), print: "#10", dot: addr{10, 10}},
		{init: s, want: s, e: Where(Line(1)), print: "#0,#6", dot: addr{0, 6}},
		{init: s, want: s, e: Where(Line(2)), print: "#6,#10", dot: addr{6, 10}},
		{init: s, want: s, e: Where(Regexp("/Hello")), print: "#0,#5", dot: addr{0, 5}},
		{init: s, want: s, e: Where(Regexp("/世界")), print: "#7,#9", dot: addr{7, 9}},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestWhereLinesEdit(t *testing.T) {
	const s = "Hello\n 世界!"
	tests := []eTest{
		{e: WhereLine(All), print: "1", dot: addr{0, 0}},
		{init: "H\ne\nl\nl\no\n 世\n界\n!", want: "H\ne\nl\nl\no\n 世\n界\n!",
			e: WhereLine(All), print: "1,8", dot: addr{0, 16}},
		{init: s, want: s, e: WhereLine(All), print: "1,2", dot: addr{0, 10}},
		{init: s, want: s, e: WhereLine(End), print: "2", dot: addr{10, 10}},
		{init: s, want: s, e: WhereLine(Line(1)), print: "1", dot: addr{0, 6}},
		{init: s, want: s, e: WhereLine(Line(2)), print: "2", dot: addr{6, 10}},
		{init: s, want: s, e: WhereLine(Regexp("/Hello")), print: "1", dot: addr{0, 5}},
		{init: s, want: s, e: WhereLine(Regexp("/世界")), print: "2", dot: addr{7, 9}},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestSubstituteEdit(t *testing.T) {
	tests := []eTest{
		{
			init: "Hello, 世界!",
			e:    Substitute{A: All, RE: "/.*/", With: "", Global: true},
			want: "", dot: addr{0, 0},
		},
		{
			init: "Hello, 世界!",
			e:    Substitute{A: All, RE: "/世界/", With: "World"},
			want: "Hello, World!", dot: addr{0, 13},
		},
		{
			init: "Hello, 世界!",
			e:    Substitute{A: All, RE: "/(.)/", With: `\1-`, Global: true},
			want: "H-e-l-l-o-,- -世-界-!-", dot: addr{0, 20},
		},
		{
			init: "abcabc",
			e:    Substitute{A: All, RE: "/abc/", With: "defg"},
			want: "defgabc", dot: addr{0, 7},
		},
		{
			init: "abcabcabc",
			e:    Substitute{A: All, RE: "/abc/", With: "defg", Global: true},
			want: "defgdefgdefg", dot: addr{0, 12},
		},
		{
			init: "abcabcabc",
			e:    Substitute{A: Regexp("/abcabc/"), RE: "/abc/", With: "defg", Global: true},
			want: "defgdefgabc", dot: addr{0, 8},
		},
		{
			init: "abc abc",
			e:    Substitute{A: All, RE: "/abc/", With: "defg"},
			want: "defg abc", dot: addr{0, 8},
		},
		{
			init: "abc abc",
			e:    Substitute{A: All, RE: "/abc/", With: "defg", Global: true},
			want: "defg defg", dot: addr{0, 9},
		},
		{
			init: "abc abc abc",
			e:    Substitute{A: Regexp("/abc abc/"), RE: "/abc/", With: "defg", Global: true},
			want: "defg defg abc", dot: addr{0, 9},
		},
		{
			init: "abcabc",
			e:    Substitute{A: All, RE: "/abc/", With: "de"},
			want: "deabc", dot: addr{0, 5},
		},
		{
			init: "abcabcabc",
			e:    Substitute{A: All, RE: "/abc/", With: "de", Global: true},
			want: "dedede", dot: addr{0, 6},
		},
		{
			init: "abcabcabc",
			e:    Substitute{A: Regexp("/abcabc/"), RE: "/abc/", With: "de", Global: true},
			want: "dedeabc", dot: addr{0, 4},
		},
		{
			init: "func f()",
			e:    Substitute{A: All, RE: `/func (.*)\(\)/`, With: `func (T) \1()`, Global: true},
			want: "func (T) f()", dot: addr{0, 12},
		},
		{
			init: "abcdefghi",
			e:    Substitute{A: All, RE: "/(abc)(def)(ghi)/", With: `\0 \3 \2 \1`},
			want: "abcdefghi ghi def abc", dot: addr{0, 21},
		},
		{
			init: "...===...",
			e:    Substitute{A: All, RE: "/(=*)/", With: `---\1---`},
			want: "------...===...", dot: addr{0, 15},
		},
		{
			init: "...===...",
			e:    Substitute{A: All, RE: "/(=+)/", With: `---\1---`},
			want: "...---===---...", dot: addr{0, 15},
		},
		{
			init: "abc",
			e:    Substitute{A: All, RE: "/abc/", With: `\1`},
			want: "", dot: addr{0, 0},
		},
		{
			init: "abcabcabc",
			e:    Substitute{A: All, RE: "/abc/", With: "def", From: 0},
			want: "defabcabc", dot: addr{0, 9},
		},
		{
			init: "abcabcabc",
			e:    Substitute{A: All, RE: "/abc/", With: "def", From: 1},
			want: "defabcabc", dot: addr{0, 9},
		},
		{
			init: "abcabcabc",
			e:    Substitute{A: All, RE: "/abc/", With: "def", From: 2},
			want: "abcdefabc", dot: addr{0, 9},
		},
		{
			init: "abcabcabc",
			e:    Substitute{A: All, RE: "/abc/", With: "def", Global: true, From: 2},
			want: "abcdefdef", dot: addr{0, 9},
		},
		{
			init: "abcabcabc",
			e:    Substitute{A: All, RE: "/notpresent/", With: "def", From: 4},
			want: "abcabcabc", dot: addr{0, 9},
		},
		{
			init: "abcabcabc",
			e:    Substitute{A: All, RE: "/abc/", With: "def", From: 4},
			want: "abcabcabc", dot: addr{0, 9},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestPipeFromEdit(t *testing.T) {
	if err := os.Setenv("SHELL", DefaultShell); err != nil {
		t.Fatal(err)
	}
	const (
		s    = "Hello\n世界!"
		echo = "echo -n '" + s + "'"
	)
	tests := []eTest{
		{e: PipeFrom(End, "false"), err: "exit status 1"},
		{e: PipeFrom(End, "echo -n stderr 1>&2"), print: "stderr"},
		{init: "", e: PipeFrom(All, echo), want: s, dot: addr{0, 9}},
		{init: "ab", e: PipeFrom(Line(0), echo), want: s + "ab", dot: addr{0, 9}},
		{init: "ab", e: PipeFrom(End, echo), want: "ab" + s, dot: addr{2, 11}},
		{init: "ab", e: PipeFrom(Rune(1), echo), want: "a" + s + "b", dot: addr{1, 10}},
		{e: PipeFrom(All, "   echo -n hi"), want: "hi", dot: addr{0, 2}},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestPipeToEdit(t *testing.T) {
	if err := os.Setenv("SHELL", DefaultShell); err != nil {
		t.Fatal(err)
	}
	const s = "Hello\n世界!"
	tests := []eTest{
		{e: PipeTo(End, "false"), err: "exit status 1"},
		{e: PipeTo(End, "echo -n stdout"), print: "stdout"},
		{e: PipeTo(End, "echo -n stderr 1>&2"), print: "stderr"},
		{init: s, e: PipeTo(End, "cat"), want: s, dot: addr{9, 9}},
		{init: s, e: PipeTo(Line(0), "cat"), want: s, dot: addr{0, 0}},
		{init: s, e: PipeTo(All, "cat"), want: s, print: s, dot: addr{0, 9}},
		{init: s, e: PipeTo(Line(1), "cat"), want: s, print: "Hello\n", dot: addr{0, 6}},
		{init: s, e: PipeTo(Line(2), "cat"), want: s, print: "世界!", dot: addr{6, 9}},
		{init: "hi", e: PipeTo(All, "   cat"), want: "hi", print: "hi", dot: addr{0, 2}},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestPipeEdit(t *testing.T) {
	if err := os.Setenv("SHELL", DefaultShell); err != nil {
		t.Fatal(err)
	}
	const s = "Hello\n世界!"
	tests := []eTest{
		{e: Pipe(End, "false"), err: "exit status 1"},
		{e: Pipe(End, "echo -n stderr 1>&2"), print: "stderr"},

		{
			init: s,
			e:    Pipe(All, "sed s/世界/World/"),
			want: "Hello\nWorld!",
			dot:  addr{0, 12},
		},
		{
			init: s,
			e:    Pipe(Line(1), "sed s/世界/World/"),
			want: s,
			dot:  addr{0, 6},
		},
		{
			init: s,
			e:    Pipe(Line(2), "sed s/世界/World/"),
			want: "Hello\nWorld!",
			dot:  addr{6, 12},
		},
		{
			init: s,
			e: Pipe(All, "  	sed s/世界/World/"),
			want: "Hello\nWorld!",
			dot:  addr{0, 12},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

type eTest struct {
	init, want, print, err string
	e                      Edit
	dot                    addr
	marks                  map[rune]addr
}

func (test eTest) run(t *testing.T) {
	test.run1(t)

	// Stringify, parse, and re-test the parsed Edit.
	var err error
	str := test.e.String()
	test.e, _, err = Ed([]rune(str))
	if err != nil {
		t.Fatalf("Failed to parse %q: %v", str, err)
	}
	test.run1(t)
}

func (test eTest) run1(t *testing.T) {
	ed := NewEditor(NewBuffer())
	defer ed.buf.Close()
	if err := ed.change(All, test.init); err != nil {
		t.Errorf("failed to init %#v: %v", test, err)
		return
	}
	ed.marks['.'] = addr{} // Start with dot=#0

	pr := bytes.NewBuffer(nil)
	err := ed.Do(test.e, pr)
	if test.err != "" {
		if err == nil {
			t.Errorf("ed.Do(%q, b)=nil, want %v", test.e, test.err)
			return
		}
		if ok, reErr := regexp.MatchString(test.err, err.Error()); reErr != nil {
			panic(reErr)
		} else if !ok {
			t.Errorf("ed.Do(%q, b)=%v, want matching %q", test.e, err, test.err)
		}
		return
	}
	if err != nil {
		t.Errorf("ed.Do(%q, pr)=%v, want <nil>", test.e, err)
		return
	}
	if s := ed.String(); s != test.want {
		t.Errorf("ed.Do(%q, pr); ed.String()=%q, want %q", test.e, s, test.want)
	}
	if s := pr.String(); s != test.print {
		t.Errorf("ed.Do(%q, pr); pr.String()=%q, want %q", test.e, s, test.print)
	}
	if dot := ed.marks['.']; dot != test.dot {
		t.Errorf("ed.Do(%q, pr); ed.dot=%v, want %v", test.e, dot, test.dot)
	}
	if test.marks != nil {
		for m, at := range test.marks {
			if ed.marks[m] != at {
				t.Errorf("ed.Do(%q, pr); ed.marks[%c]=%v, want %v",
					test.e, m, ed.marks[m], at)
			}
		}
	}
}

func dotAt(from, to int64) map[rune]addr {
	return map[rune]addr{'.': addr{from, to}}
}

func TestUndoEdit(t *testing.T) {
	tests := []undoTest{
		{
			name: "empty undo 1",
			e:    Undo(1),
			want: "",
		},
		{
			name: "empty undo 100",
			e:    Undo(100),
			want: "",
		},
		{
			name:  "1 append, undo 1",
			init:  []Edit{Append(End, "abc")},
			e:     Undo(1),
			want:  "",
			marks: dotAt(0, 0),
		},
		{
			name:  "1 append, undo 0",
			init:  []Edit{Append(End, "abc")},
			e:     Undo(0),
			want:  "",
			marks: dotAt(0, 0),
		},
		{
			name:  "1 append, undo -1",
			init:  []Edit{Append(End, "abc")},
			e:     Undo(-1),
			want:  "",
			marks: dotAt(0, 0),
		},
		{
			name:  "2 appends, undo 1",
			init:  []Edit{Append(End, "abc"), Append(End, "xyz")},
			e:     Undo(1),
			want:  "abc",
			marks: dotAt(3, 3),
		},
		{
			name:  "2 appends, undo 2",
			init:  []Edit{Append(End, "abc"), Append(End, "xyz")},
			e:     Undo(2),
			want:  "",
			marks: dotAt(0, 0),
		},
		{
			name:  "2 appends, undo 3",
			init:  []Edit{Append(End, "abc"), Append(End, "xyz")},
			e:     Undo(3),
			want:  "",
			marks: dotAt(0, 0),
		},
		{
			name:  "1 delete, undo 1",
			init:  []Edit{Append(End, "abc"), Delete(All)},
			e:     Undo(1),
			want:  "abc",
			marks: dotAt(0, 3),
		},
		{
			name: "2 deletes, undo 1",
			init: []Edit{
				Append(End, "abc"),
				Delete(Rune(2).To(End)),
				Delete(Rune(1).To(End)),
			},
			e:     Undo(1),
			want:  "ab",
			marks: dotAt(1, 2),
		},
		{
			name: "2 deletes, undo 2",
			init: []Edit{
				Append(End, "abc"),
				Delete(Rune(2).To(End)),
				Delete(Rune(1).To(End)),
			},
			e:     Undo(2),
			want:  "abc",
			marks: dotAt(2, 3),
		},
		{
			name: "delete middle, undo",
			init: []Edit{
				Append(End, "abc"),
				Delete(Rune(1).To(Rune(2))),
			},
			e:     Undo(1),
			want:  "abc",
			marks: dotAt(1, 2),
		},
		{
			name: "multi-change undo",
			init: []Edit{
				Append(End, "a.a.a"),
				SubGlobal(All, "/[.]/", "z"),
			},
			e:     Undo(1),
			want:  "a.a.a",
			marks: dotAt(1, 4),
		},
		{
			name: "undo maintains marks",
			init: []Edit{
				Append(End, "abcXXX"),
				Set(Regexp("/XXX"), 'm'),
				Delete(Regexp("/b")),
			},
			e:    Undo(1),
			want: "abcXXX",
			marks: map[rune]addr{
				'.': addr{1, 2},
				'm': addr{3, 6},
			},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestRedoEdit(t *testing.T) {
	tests := []undoTest{
		{
			name: "empty redo 1",
			init: []Edit{Append(End, "abc")},
			e:    Redo(1),
			want: "abc",
		},
		{
			name: "empty redo 100",
			init: []Edit{Append(End, "abc")},
			e:    Redo(100),
			want: "abc",
		},
		{
			name:  "undo 1 append redo 1",
			init:  []Edit{Append(End, "abc"), Undo(1)},
			e:     Redo(1),
			want:  "abc",
			marks: dotAt(0, 3),
		},
		{
			name:  "undo 1 append redo 0",
			init:  []Edit{Append(End, "abc"), Undo(1)},
			e:     Redo(0),
			want:  "abc",
			marks: dotAt(0, 3),
		},
		{
			name:  "undo 1 append redo -1",
			init:  []Edit{Append(End, "abc"), Undo(1)},
			e:     Redo(-1),
			want:  "abc",
			marks: dotAt(0, 3),
		},
		{
			name:  "undo 1 append redo 2",
			init:  []Edit{Append(End, "abc"), Undo(1)},
			e:     Redo(2),
			want:  "abc",
			marks: dotAt(0, 3),
		},
		{
			name:  "undo 2 appends redo 1",
			init:  []Edit{Append(End, "abc"), Append(End, "xyz"), Undo(2)},
			e:     Redo(1),
			want:  "abcxyz",
			marks: dotAt(0, 6),
		},
		{
			name:  "undo undo appends redo 1",
			init:  []Edit{Append(End, "abc"), Append(End, "xyz"), Undo(1), Undo(1)},
			e:     Redo(1),
			want:  "abc",
			marks: dotAt(0, 3),
		},
		{
			name: "multi-change redo",
			init: []Edit{
				Append(End, "a.a.a"),
				SubGlobal(All, "/[.]/", "z"),
				Undo(1),
			},
			e:     Redo(1),
			want:  "azaza",
			marks: dotAt(1, 4),
		},
		{
			name: "redo maintains marks",
			init: []Edit{
				Append(End, "abcXXX"),
				Set(Regexp("/XXX"), 'm'),
				Delete(Regexp("/b")),
				Undo(1),
			},
			e:    Redo(1),
			want: "acXXX",
			marks: map[rune]addr{
				'.': addr{1, 1},
				'm': addr{2, 5},
			},
		},
		{
			name: "append append undo undo redo undo redo",
			init: []Edit{
				Append(End, "abc"),
				Append(End, "xyz"),
				Undo(1),
				Redo(1),
				Undo(1),
			},
			e:     Redo(1),
			want:  "abcxyz",
			marks: dotAt(3, 6),
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

type undoTest struct {
	name  string
	init  []Edit
	e     Edit
	want  string
	marks map[rune]addr
}

func (test *undoTest) run(t *testing.T) {
	test.run1(t)

	// Stringify, parse, and re-test the parsed Edit.
	var err error
	str := test.e.String()
	test.e, _, err = Ed([]rune(str))
	if err != nil {
		t.Fatalf("Failed to parse %q: %v", str, err)
	}
	test.name = test.name + " parsed from [" + str + "]"
	test.run1(t)
}

func (test *undoTest) run1(t *testing.T) {
	ed := NewEditor(NewBuffer())
	defer ed.buf.Close()

	for _, e := range test.init {
		if err := ed.Do(e, ioutil.Discard); err != nil {
			t.Fatalf("%s init ed.Do(%q, _)=%v, want nil", test.name, e, err)
		}
	}

	if err := ed.Do(test.e, ioutil.Discard); err != nil {
		t.Fatalf("%s ed.Do(%q, _)=%v, want nil", test.name, test.e, err)
	}

	if s := ed.String(); s != test.want {
		t.Errorf("%s ed.Do(%q, _) leaves %q, want %q", test.name, test.e, s, test.want)
	}
	if test.marks != nil {
		for m, want := range test.marks {
			if got := ed.marks[m]; got != want {
				t.Errorf("%s mark %c=%v, want %v", test.name, m, got, want)
			}
		}
	}
}

func TestEd(t *testing.T) {
	tests := []struct {
		e, left string
		want    Edit
		err     string
	}{
		{e: strconv.FormatInt(math.MaxInt64, 10) + "0", err: "value out of range"},

		{e: "", want: Set(Dot, '.')},
		{e: ".", want: Set(Dot, '.')},
		{e: "  .", want: Set(Dot, '.')},
		{e: "#0", want: Set(Rune(0), '.')},
		{e: "#0+1", want: Set(Rune(0).Plus(Line(1)), '.')},
		{e: " #0 + 1 ", want: Set(Rune(0).Plus(Line(1)), '.')},
		{e: "#0+1\nc/abc", left: "c/abc", want: Set(Rune(0).Plus(Line(1)), '.')},
		{e: "/abc\n1c/xyz", left: "1c/xyz", want: Set(Regexp("/abc/"), '.')},

		{e: "k", want: Set(Dot, '.')},
		{e: " k ", want: Set(Dot, '.')},
		{e: "#0k ", want: Set(Rune(0), '.')},
		{e: "#0ka", want: Set(Rune(0), 'a')},
		{e: "#0k a", want: Set(Rune(0), 'a')},
		{e: "#0k	 a", want: Set(Rune(0), 'a')},
		{e: "#0k	 α", want: Set(Rune(0), 'α')},

		{e: "c/αβξ", want: Change(Dot, "αβξ")},
		{e: "c/αβξ/", want: Change(Dot, "αβξ")},
		{e: "c/αβξ\n", want: Change(Dot, "αβξ")},
		{e: "c/αβξ/xyz", left: "xyz", want: Change(Dot, "αβξ")},
		{e: "c/αβξ\nxyz", left: "xyz", want: Change(Dot, "αβξ")},
		{e: "#1,#2c/αβξ/", want: Change(Rune(1).To(Rune(2)), "αβξ")},
		{e: " #1 , #2 c/αβξ/", want: Change(Rune(1).To(Rune(2)), "αβξ")},
		{e: "c/αβξ\\/", want: Change(Dot, "αβξ/")},
		{e: "c/αβξ\\n", want: Change(Dot, "αβξ\n")},
		{e: "c\nαβξ\n.\n", want: Change(Dot, "αβξ\n")},
		{e: "c\nαβξ\n.", want: Change(Dot, "αβξ\n")},
		{e: "c\nαβξ\n\n.", want: Change(Dot, "αβξ\n\n")},

		{e: "a/αβξ", want: Append(Dot, "αβξ")},
		{e: "a/αβξ/", want: Append(Dot, "αβξ")},
		{e: "a/αβξ\n", want: Append(Dot, "αβξ")},
		{e: "a/αβξ/xyz", left: "xyz", want: Append(Dot, "αβξ")},
		{e: "a/αβξ\nxyz", left: "xyz", want: Append(Dot, "αβξ")},
		{e: "#1,#2a/αβξ/", want: Append(Rune(1).To(Rune(2)), "αβξ")},
		{e: " #1 , #2 a/αβξ/", want: Append(Rune(1).To(Rune(2)), "αβξ")},
		{e: "a/αβξ\\/", want: Append(Dot, "αβξ/")},
		{e: "a/αβξ\\n", want: Append(Dot, "αβξ\n")},
		{e: "a\nαβξ\n.\n", want: Append(Dot, "αβξ\n")},
		{e: "a\nαβξ\n.", want: Append(Dot, "αβξ\n")},
		{e: "a\nαβξ\n\n.", want: Append(Dot, "αβξ\n\n")},

		{e: "i/αβξ", want: Insert(Dot, "αβξ")},
		{e: "i/αβξ/", want: Insert(Dot, "αβξ")},
		{e: "i/αβξ\n", want: Insert(Dot, "αβξ")},
		{e: "i/αβξ/xyz", left: "xyz", want: Insert(Dot, "αβξ")},
		{e: "i/αβξ\nxyz", left: "xyz", want: Insert(Dot, "αβξ")},
		{e: "#1,#2i/αβξ/", want: Insert(Rune(1).To(Rune(2)), "αβξ")},
		{e: " #1 , #2 i/αβξ/", want: Insert(Rune(1).To(Rune(2)), "αβξ")},
		{e: "i/αβξ\\/", want: Insert(Dot, "αβξ/")},
		{e: "i/αβξ\\n", want: Insert(Dot, "αβξ\n")},
		{e: "i\nαβξ\n.\n", want: Insert(Dot, "αβξ\n")},
		{e: "i\nαβξ\n.", want: Insert(Dot, "αβξ\n")},
		{e: "i\nαβξ\n\n.", want: Insert(Dot, "αβξ\n\n")},

		{e: "d", want: Delete(Dot)},
		{e: "#1,#2d", want: Delete(Rune(1).To(Rune(2)))},
		{e: "dxyz", left: "xyz", want: Delete(Dot)},
		{e: "d\nxyz", left: "xyz", want: Delete(Dot)},
		{e: "d  \nxyz", left: "xyz", want: Delete(Dot)},

		{e: "m", want: Move(Dot, Dot)},
		{e: "m/abc/", want: Move(Dot, Regexp("/abc/"))},
		{e: "/abc/m/def/", want: Move(Regexp("/abc/"), Regexp("/def/"))},
		{e: "#1+1m$", want: Move(Rune(1).Plus(Line(1)), End)},
		{e: " #1 + 1 m $", want: Move(Rune(1).Plus(Line(1)), End)},
		{e: "1m$xyz", left: "xyz", want: Move(Line(1), End)},
		{e: "1m\n$xyz", left: "$xyz", want: Move(Line(1), Dot)},
		{e: "m" + strconv.FormatInt(math.MaxInt64, 10) + "0", err: "value out of range"},

		{e: "t", want: Copy(Dot, Dot)},
		{e: "t/abc/", want: Copy(Dot, Regexp("/abc/"))},
		{e: "/abc/t/def/", want: Copy(Regexp("/abc/"), Regexp("/def/"))},
		{e: "#1+1t$", want: Copy(Rune(1).Plus(Line(1)), End)},
		{e: " #1 + 1 t $", want: Copy(Rune(1).Plus(Line(1)), End)},
		{e: "1t$xyz", left: "xyz", want: Copy(Line(1), End)},
		{e: "1t\n$xyz", left: "$xyz", want: Copy(Line(1), Dot)},
		{e: "t" + strconv.FormatInt(math.MaxInt64, 10) + "0", err: "value out of range"},

		{e: "p", want: Print(Dot)},
		{e: "pxyz", left: "xyz", want: Print(Dot)},
		{e: "#1+1p", want: Print(Rune(1).Plus(Line(1)))},
		{e: " #1 + 1 p", want: Print(Rune(1).Plus(Line(1)))},

		{e: "=", want: WhereLine(Dot)},
		{e: "=xyz", left: "xyz", want: WhereLine(Dot)},
		{e: "#1+1=", want: WhereLine(Rune(1).Plus(Line(1)))},
		{e: " #1 + 1 =", want: WhereLine(Rune(1).Plus(Line(1)))},

		{e: "=#", want: Where(Dot)},
		{e: "=#xyz", left: "xyz", want: Where(Dot)},
		{e: "#1+1=#", want: Where(Rune(1).Plus(Line(1)))},
		{e: " #1 + 1 =#", want: Where(Rune(1).Plus(Line(1)))},

		{e: "s/a/b", want: Sub(Dot, "/a/", "b")},
		{e: "s;a;b", want: Sub(Dot, ";a;", "b")},
		{e: "s/a//", want: Sub(Dot, "/a/", "")},
		{e: "s/a/\n/g", left: "/g", want: Sub(Dot, "/a/", "")},
		{e: "s/(.*)/a\\1", want: Sub(Dot, "/(.*)/", "a\\1")},
		{e: ".s/a/b", want: Sub(Dot, "/a/", "b")},
		{e: "#1+1s/a/b", want: Sub(Rune(1).Plus(Line(1)), "/a/", "b")},
		{e: " #1 + 1 s/a/b", want: Sub(Rune(1).Plus(Line(1)), "/a/", "b")},
		{e: " #1 + 1 s/a/b", want: Sub(Rune(1).Plus(Line(1)), "/a/", "b")},
		{e: "s/a/b/xyz", left: "xyz", want: Sub(Dot, "/a/", "b")},
		{e: "s/a/b\nxyz", left: "xyz", want: Sub(Dot, "/a/", "b")},
		{e: "s1/a/b", want: Sub(Dot, "/a/", "b")},
		{e: "s/a/b/g", want: SubGlobal(Dot, "/a/", "b")},
		{e: " #1 + 1 s/a/b/g", want: SubGlobal(Rune(1).Plus(Line(1)), "/a/", "b")},
		{e: "s2/a/b", want: Substitute{A: Dot, RE: "/a/", With: "b", From: 2}},
		{e: "s2;a;b", want: Substitute{A: Dot, RE: ";a;", With: "b", From: 2}},
		{e: "s1000/a/b", want: Substitute{A: Dot, RE: "/a/", With: "b", From: 1000}},
		{e: "s 2 /a/b", want: Substitute{A: Dot, RE: "/a/", With: "b", From: 2}},
		{e: "s 1000 /a/b/g", want: Substitute{A: Dot, RE: "/a/", With: "b", Global: true, From: 1000}},
		{e: "s/", err: "missing pattern"},
		{e: "s//b", err: "missing pattern"},
		{e: "s/\n/b", err: "missing pattern"},
		{e: "s" + strconv.FormatInt(math.MaxInt64, 10) + "0" + "/a/b/g", err: "value out of range"},

		{e: "|cmd", want: Pipe(Dot, "cmd")},
		{e: "|	   cmd", want: Pipe(Dot, "cmd")},
		{e: "|cmd\nleft", left: "left", want: Pipe(Dot, "cmd")},
		{e: "|	   cmd\nleft", left: "left", want: Pipe(Dot, "cmd")},
		{e: "|cmd\\nleft", want: Pipe(Dot, "cmd\nleft")},
		{e: "|	   cmd\\nleft", want: Pipe(Dot, "cmd\nleft")},
		{e: "  	|cmd", want: Pipe(Dot, "cmd")},
		{e: ",|cmd", want: Pipe(All, "cmd")},
		{e: "$|cmd", want: Pipe(End, "cmd")},
		{e: ",	  |cmd", want: Pipe(All, "cmd")},
		{e: "  	,	  |cmd", want: Pipe(All, "cmd")},
		{e: ",+3|cmd", want: Pipe(Line(0).To(Dot.Plus(Line(3))), "cmd")},

		{e: ">cmd", want: PipeTo(Dot, "cmd")},
		{e: ">	   cmd", want: PipeTo(Dot, "cmd")},
		{e: ">cmd\nleft", left: "left", want: PipeTo(Dot, "cmd")},
		{e: ">	   cmd\nleft", left: "left", want: PipeTo(Dot, "cmd")},
		{e: ">cmd\\nleft", want: PipeTo(Dot, "cmd\nleft")},
		{e: ">	   cmd\\nleft", want: PipeTo(Dot, "cmd\nleft")},
		{e: "  	>cmd", want: PipeTo(Dot, "cmd")},
		{e: ",>cmd", want: PipeTo(All, "cmd")},
		{e: "$>cmd", want: PipeTo(End, "cmd")},
		{e: ",	  >cmd", want: PipeTo(All, "cmd")},
		{e: "  	,	  >cmd", want: PipeTo(All, "cmd")},
		{e: ",+3>cmd", want: PipeTo(Line(0).To(Dot.Plus(Line(3))), "cmd")},

		{e: "<cmd", want: PipeFrom(Dot, "cmd")},
		{e: "<	   cmd", want: PipeFrom(Dot, "cmd")},
		{e: "<cmd\nleft", left: "left", want: PipeFrom(Dot, "cmd")},
		{e: "<	   cmd\nleft", left: "left", want: PipeFrom(Dot, "cmd")},
		{e: "<cmd\\nleft", want: PipeFrom(Dot, "cmd\nleft")},
		{e: "<	   cmd\\nleft", want: PipeFrom(Dot, "cmd\nleft")},
		{e: "  	<cmd", want: PipeFrom(Dot, "cmd")},
		{e: ",<cmd", want: PipeFrom(All, "cmd")},
		{e: "$<cmd", want: PipeFrom(End, "cmd")},
		{e: ",	  <cmd", want: PipeFrom(All, "cmd")},
		{e: "  	,	  <cmd", want: PipeFrom(All, "cmd")},
		{e: ",+3<cmd", want: PipeFrom(Line(0).To(Dot.Plus(Line(3))), "cmd")},

		{e: "u", want: Undo(1)},
		{e: " u", want: Undo(1)},
		{e: "u1", want: Undo(1)},
		{e: " u1", want: Undo(1)},
		{e: "u100", want: Undo(100)},
		{e: " u100", want: Undo(100)},
		{e: "u" + strconv.FormatInt(math.MaxInt64, 10) + "0", err: "value out of range"},
		{e: "r", want: Redo(1)},
		{e: " r", want: Redo(1)},
		{e: "r1", want: Redo(1)},
		{e: " r1", want: Redo(1)},
		{e: "r100", want: Redo(100)},
		{e: " r100", want: Redo(100)},
		{e: "r" + strconv.FormatInt(math.MaxInt64, 10) + "0", err: "value out of range"},
	}
	for _, test := range tests {
		e, left, err := Ed([]rune(test.e))
		ok := true
		if test.err != "" {
			ok = err != nil && regexp.MustCompile(test.err).MatchString(err.Error())
		} else {
			ok = err == nil && reflect.DeepEqual(e, test.want) && string(left) == string(test.left)
		}
		if !ok {
			t.Errorf(`Ed(%q)=%q,%q,%q, want %q,%q,%q`, test.e, e, left, err, test.want, test.left, test.err)

		}
	}
}
