// Copyright © 2015, The T Authors.

package edit

import (
	"bytes"
	"regexp"
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
		{init: s, want: s, e: Set(All, 'a'), marks: map[rune]addr{'a': addr{0, 10}}},
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

type eTest struct {
	init, want, print, err string
	e                      Edit
	dot                    addr
	marks                  map[rune]addr
}

func (test eTest) run(t *testing.T) {
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
		if ok, err := regexp.MatchString(test.err, err.Error()); err != nil {
			panic(err)
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
