// Copyright © 2015, The T Authors.

package edit

import (
	"bytes"
	"errors"
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

func TestRetry(t *testing.T) {
	ed := NewEditor(NewBuffer())
	defer ed.Close()

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
		{init: "Hello\n 世界!", a: Regexp("/Hello"), at: addr{0, 5}},
		{init: "Hello\n 世界!", a: Regexp("/世界"), at: addr{7, 9}},
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
