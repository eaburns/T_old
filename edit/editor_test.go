// Copyright © 2015, The T Authors.

package edit

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/eaburns/T/edit/runes"
)

// String returns a string containing the entire editor contents.
func (ed *Editor) String() string {
	rs, err := runes.ReadAll(ed.buf.runes.Reader(0))
	if err != nil {
		panic(err)
	}
	return string(rs)
}

func (ed *Editor) change(a Address, s string) error {
	return ed.Do(Change(All, s), bytes.NewBuffer(nil))
}

func TestEditorClose(t *testing.T) {
	buf := NewBuffer()
	ed := NewEditor(buf)
	if err := ed.Close(); err != nil {
		t.Fatalf("failed to close the editor: %v", err)
	}
	if err := ed.Close(); err == nil {
		t.Fatal("ed.Close()=nil, want error")
	}
	if err := buf.Close(); err != nil {
		t.Fatalf("failed to close the buffer: %v", err)
	}
}

func TestRetry(t *testing.T) {
	ed := NewEditor(NewBuffer())
	defer ed.buf.Close()

	const str = "Hello, 世界!"
	ch := func() (addr, error) {
		if ed.buf.seq < 10 {
			// Simulate concurrent changes, necessitating retries.
			ed.buf.seq++
		}
		return change{a: All, op: 'c', str: str}.do(ed, nil)
	}
	if err := ed.do(ch); err != nil {
		t.Fatalf("ed.do(ch)=%v, want nil", err)
	}
	if s := ed.String(); s != str {
		t.Errorf("ed.String()=%q, want %q,nil\n", s, str)
	}
}

func TestWhere(t *testing.T) {
	tests := []struct {
		init string
		a    Address
		at   addr
	}{
		{init: "", a: All, at: addr{0, 0}},
		{init: "H\ne\nl\nl\no\n 世\n界\n!", a: All, at: addr{0, 16}},
		{init: "Hello\n 世界!", a: All, at: addr{0, 10}},
		{init: "Hello\n 世界!", a: End, at: addr{10, 10}},
		{init: "Hello\n 世界!", a: Line(1), at: addr{0, 6}},
		{init: "Hello\n 世界!", a: Line(2), at: addr{6, 10}},
		{init: "Hello\n 世界!", a: Regexp("Hello"), at: addr{0, 5}},
		{init: "Hello\n 世界!", a: Regexp("世界"), at: addr{7, 9}},
	}
	for _, test := range tests {
		ed := NewEditor(NewBuffer())
		defer ed.buf.Close()
		if err := ed.change(All, test.init); err != nil {
			t.Errorf("failed to init %#v: %v", test, err)
			continue
		}
		at, err := ed.Where(test.a)
		if at != test.at || err != nil {
			t.Errorf("ed.Where(%q)=%v,%v, want %v,<nil>", test.a, at, err, test.at)
		}
	}
}

func TestWriterTo(t *testing.T) {
	tests := []struct {
		init, want string
		a          Address
		dot        addr
	}{
		{init: "", a: All, want: "", dot: addr{}},
		{init: "", a: End, want: "", dot: addr{}},
		{init: "", a: Rune(0), want: "", dot: addr{}},
		{init: "Hello, 世界", a: All, want: "Hello, 世界", dot: addr{0, 9}},
		{init: "Hello, 世界", a: Regexp("Hello"), want: "Hello", dot: addr{0, 5}},
		{init: "Hello, 世界", a: End, want: "", dot: addr{9, 9}},
		{init: "Hello, 世界", a: Rune(0), want: "", dot: addr{}},
		{init: "a\nb\nc\n", a: Line(0), want: "", dot: addr{}},
		{init: "a\nb\nc\n", a: Line(1), want: "a\n", dot: addr{0, 2}},
		{init: "a\nb\nc\n", a: Line(2), want: "b\n", dot: addr{2, 4}},
		{init: "a\nb\nc\n", a: Line(3), want: "c\n", dot: addr{4, 6}},
	}
	for _, test := range tests {
		ed := NewEditor(NewBuffer())
		defer ed.buf.Close()
		if err := ed.change(All, test.init); err != nil {
			t.Errorf("failed to init %#v: %v", test, err)
			continue
		}

		b := bytes.NewBuffer(nil)
		n, err := ed.WriterTo(test.a).WriteTo(b)
		str := b.String()
		if n != int64(len(str)) || err != nil {
			t.Errorf("ed.WriterTo(%q).WriteTo(b)=%d,%v, want %d,<nil>", test.a, n, err, len(str))
			continue
		}
		if str != test.want {
			t.Errorf("ed.WriterTo(%q).WriteTo(b); b.String()=%q, want %q", test.a, str, test.want)
		}
		if dot := ed.marks['.']; dot != test.dot {
			t.Errorf("ed.WriterTo(%q).WriteTo(b); ed.marks['.']=%v, want %v", test.a, dot, test.dot)
		}
	}
}

func TestReaderFrom(t *testing.T) {
	tests := []struct {
		init, read, want string
		a                Address
		dot              addr
	}{
		{init: "", a: All, read: "", want: "", dot: addr{}},
		{init: "", a: All, read: "αβξ", want: "αβξ", dot: addr{0, 3}},
		{init: "Hello, 世界", a: Line(0), read: "", want: "Hello, 世界", dot: addr{}},
		{init: "Hello, 世界", a: End, read: "", want: "Hello, 世界", dot: addr{9, 9}},
		{init: "Hello, 世界", a: All, read: "", want: "", dot: addr{0, 0}},
		{init: "Hello, 世界", a: All, read: "αβξ", want: "αβξ", dot: addr{0, 3}},
		{init: "Hello, 世界", a: Regexp("世界"), read: "World", want: "Hello, World", dot: addr{7, 12}},
		{init: "a\nb\nc\n", a: Line(0), read: "z\n", want: "z\na\nb\nc\n", dot: addr{0, 2}},
		{init: "a\nb\nc\n", a: Line(1), read: "z\n", want: "z\nb\nc\n", dot: addr{0, 2}},
		{init: "a\nb\nc\n", a: Line(2), read: "z\n", want: "a\nz\nc\n", dot: addr{2, 4}},
		{init: "a\nb\nc\n", a: Line(3), read: "z\n", want: "a\nb\nz\n", dot: addr{4, 6}},
	}
	for _, test := range tests {
		ed := NewEditor(NewBuffer())
		defer ed.buf.Close()
		if err := ed.change(All, test.init); err != nil {
			t.Errorf("failed to init %#v: %v", test, err)
			continue
		}

		n, err := ed.ReaderFrom(test.a).ReadFrom(strings.NewReader(test.read))
		if n != int64(len(test.read)) || err != nil {
			t.Errorf("ed.ReaderFrom(%q).ReadFrom(%q)=%d,%v, want %d,<nil>", test.a, test.read, n, err, len(test.read))
			continue
		}
		if str := ed.String(); str != test.want {
			t.Errorf("ed.ReaderFrom(%q).ReadFrom(%q); b.String()=%q, want %q", test.a, test.read, str, test.want)
		}
		if dot := ed.marks['.']; dot != test.dot {
			t.Errorf("ed.ReaderFrom(%q).ReadFrom(%q); ed.marks['.']=%v, want %v", test.a, test.read, dot, test.dot)
		}
	}
}

func TestEditorDoPendingError(t *testing.T) {
	ed := NewEditor(NewBuffer())
	defer ed.buf.Close()
	maddr := addr{5, 10}
	ed.marks['m'] = maddr

	testErr := errors.New("test error")
	err := ed.do(func() (addr, error) {
		// Change a mark, it should be restored to its origin.
		m := ed.marks['m']
		m.to += 10
		m.from += 20
		ed.marks['m'] = m
		return addr{}, testErr
	})
	if err != testErr {
		t.Errorf("ed.do(…)=%v, want %v", err, testErr)
	}
	if ed.marks['m'] != maddr {
		t.Errorf("ed.marks['m']=%v, want %v", ed.marks['m'], maddr)
	}
}

func TestEditorDoOutOfSequence(t *testing.T) {
	ed := NewEditor(NewBuffer())
	defer ed.buf.Close()
	const init = "Hello, 世界"
	if err := ed.change(All, init); err != nil {
		t.Fatalf("failed to init: %v", err)
	}
	maddr := addr{5, 10}
	ed.marks['m'] = maddr

	err := ed.do(func() (addr, error) {
		if err := pend(ed, addr{10, 20}, runes.EmptyReader()); err != nil {
			panic(err)
		}
		if err := pend(ed, addr{0, 10}, runes.EmptyReader()); err != nil {
			panic(err)
		}
		return addr{0, 20}, nil
	})
	if err == nil {
		t.Error("ed.do({out-of-sequence})=<nil>, want error")
	}
	if ed.marks['m'] != maddr {
		t.Errorf("ed.marks['m']=%v, want %v", ed.marks['m'], maddr)
	}
}
