// Copyright © 2015, The T Authors.

// Package edit provides sam-style editing of rune buffers.
// See sam(1) for an overview: http://swtch.com/plan9port/man/man1/sam.html.
package edit

import (
	"errors"
	"io"
	"unicode/utf8"

	"github.com/eaburns/T/edit/runes"
)

// An Edit is an operation that can be made on a Buffer by an Editor.
type Edit interface {
	// String returns the string representation of the edit.
	// The returned string will result in an equivalent edit
	// when parsed with Ed().
	String() string

	do(*Editor, io.Writer) (addr, error)
}

type change struct {
	a   Address
	op  rune
	str string
}

// Change returns an Edit
// that changes the string at a to str,
// and sets dot to the changed runes.
func Change(a Address, str string) Edit { return change{a: a, op: 'c', str: str} }

// Append returns an Edit
// that appends str after the string at a,
// and sets dot to the appended runes.
func Append(a Address, str string) Edit { return change{a: a, op: 'a', str: str} }

// Insert returns an Edit
// that inserts str before the string at a,
// and sets dot to the inserted runes.
func Insert(a Address, str string) Edit { return change{a: a, op: 'i', str: str} }

// Delete returns an Edit
// that deletes the string at a,
// and sets dot to the empty string
// that was deleted.
func Delete(a Address) Edit { return change{a: a, op: 'd'} }

func (e change) String() string { return e.a.String() + string(e.op) + escape(e.str) }

func (e change) do(ed *Editor, _ io.Writer) (addr, error) {
	switch e.op {
	case 'a':
		e.a = e.a.Plus(Rune(0))
	case 'i':
		e.a = e.a.Minus(Rune(0))
	}
	at, err := e.a.where(ed)
	if err != nil {
		return addr{}, err
	}
	return at, pend(ed, at, runes.StringReader(e.str))
}

func escape(str string) string {
	if r, _ := utf8.DecodeLastRuneInString(str); r == '\n' {
		// Use multi-line format.
		return "\n" + str + ".\n"
	}

	const (
		delim = '/'
		esc   = '\\'
	)
	es := []rune{delim}
	for _, r := range str {
		switch r {
		case '\n':
			es = append(es, esc, 'n')
		case delim:
			es = append(es, esc, r)
		default:
			es = append(es, r)
		}
	}
	return string(append(es, delim))
}

type move struct {
	src, dst Address
}

// Move returns an Edit
// that moves runes from src to after dst
// and sets dot to the moved runes.
// It is an error if the end of dst is within src.
func Move(src, dst Address) Edit { return move{src: src, dst: dst} }

func (e move) String() string { return e.src.String() + "m" + e.dst.String() }

func (e move) do(ed *Editor, _ io.Writer) (addr, error) {
	s, err := e.src.where(ed)
	if err != nil {
		return addr{}, err
	}
	d, err := e.dst.where(ed)
	if err != nil {
		return addr{}, err
	}
	d.from = d.to

	if d.from > s.from && d.from < s.to {
		return addr{}, errors.New("addresses overlap")
	}

	if d.from >= s.to {
		// Moving to after the source. Delete the source first.
		if err := pend(ed, s, runes.EmptyReader()); err != nil {
			return addr{}, err
		}
	}
	r := runes.LimitReader(ed.buf.runes.Reader(s.from), s.size())
	if err := pend(ed, d, r); err != nil {
		return addr{}, err
	}
	if d.from <= s.from {
		// Moving to before the source. Delete the source second.
		if err := pend(ed, s, runes.EmptyReader()); err != nil {
			return addr{}, err
		}
	}
	return d, nil
}

type cpy struct {
	src, dst Address
}

// Copy returns an Edit
// that copies runes from src to after dst
// and sets dot to the copied runes.
func Copy(src, dst Address) Edit { return cpy{src: src, dst: dst} }

func (e cpy) String() string { return e.src.String() + "t" + e.dst.String() }

func (e cpy) do(ed *Editor, _ io.Writer) (addr, error) {
	s, err := e.src.where(ed)
	if err != nil {
		return addr{}, err
	}
	d, err := e.dst.where(ed)
	if err != nil {
		return addr{}, err
	}
	d.from = d.to
	r := runes.LimitReader(ed.buf.runes.Reader(s.from), s.size())
	return d, pend(ed, d, r)
}

type set struct {
	a Address
	m rune
}

// Set returns an Edit
// that sets the dot or mark m to a.
// The mark m must be either
// a lower-case or upper-case letter or dot: [a-zA-Z.].
// Any other rune is an error.
// If the mark is . then dot is set to a,
// otherwise the named mark is set to a.
func Set(a Address, m rune) Edit { return set{a: a, m: m} }

func (e set) String() string {
	if e.m == '.' {
		return e.a.String() + "k"
	}
	return e.a.String() + "k" + string(e.m)
}

func (e set) do(ed *Editor, _ io.Writer) (addr, error) {
	if !isMarkRune(e.m) && e.m != '.' {
		return addr{}, errors.New("bad mark: " + string(e.m))
	}
	at, err := e.a.where(ed)
	if err != nil {
		return addr{}, err
	}
	ed.marks[e.m] = at
	return ed.marks['.'], nil
}
