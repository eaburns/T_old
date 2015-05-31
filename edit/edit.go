// Copyright © 2015, The T Authors.

// Package edit provides sam-style editing of rune buffers.
// See sam(1) for an overview: http://swtch.com/plan9port/man/man1/sam.html.
package edit

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"unicode"
	"unicode/utf8"

	"github.com/eaburns/T/edit/runes"
	"github.com/eaburns/T/re1"
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

type print struct{ a Address }

// Print returns an Edit
// that prints the string at a to an io.Writer
// and sets dot to the printed string.
func Print(a Address) Edit { return print{a: a} }

func (e print) String() string { return e.a.String() + "p" }

func (e print) do(ed *Editor, w io.Writer) (addr, error) {
	at, err := e.a.where(ed)
	if err != nil {
		return addr{}, err
	}
	r := runes.LimitReader(ed.buf.runes.Reader(at.from), at.size())
	if _, err := runes.Copy(runes.UTF8Writer(w), r); err != nil {
		return addr{}, err
	}
	return at, nil
}

type where struct {
	a    Address
	line bool
}

// Where returns an Edit
// that prints the rune location of a
// to an io.Writer
// and sets dot to the a.
func Where(a Address) Edit { return where{a: a} }

// WhereLine returns an Edit that prints both
// the rune address and the lines containing a
// to an io.Writer
// and sets dot to the a.
func WhereLine(a Address) Edit { return where{a: a, line: true} }

func (e where) String() string {
	if e.line {
		return e.a.String() + "="
	}
	return e.a.String() + "=#"
}

func (e where) do(ed *Editor, w io.Writer) (addr, error) {
	at, err := e.a.where(ed)
	if err != nil {
		return addr{}, err
	}
	if e.line {
		l0, l1, err := ed.lines(at)
		if err != nil {
			return addr{}, err
		}
		if l0 == l1 {
			_, err = fmt.Fprintf(w, "%d", l0)
		} else {
			_, err = fmt.Fprintf(w, "%d,%d", l0, l1)
		}
	} else {
		if at.size() == 0 {
			_, err = fmt.Fprintf(w, "#%d", at.from)
		} else {
			_, err = fmt.Fprintf(w, "#%d,#%d", at.from, at.to)
		}
	}
	if err != nil {
		return addr{}, err
	}
	return at, err
}

// Substitute is an Edit that substitutes regular expression matches.
type Substitute struct {
	// A is the address in which to search for matches.
	// After performing the edit, Dot is set the modified address A.
	A Address
	// RE is the regular expression to match.
	// It is compiled with re1.Options{Delimited: true}.
	RE string
	// With is the runes with which to replace each match.
	// Within With, a backslash followed by a digit d
	// stands for the string that matched the d-th subexpression.
	// Subexpression 0 is the entire match.
	// It is an error if such a subexpression contains
	// more than MaxRunes runes.
	// \n is a literal newline.
	With string
	// Global is whether to replace all matches, or just one.
	// If Global is false, only one match is replaced.
	// If Global is true, all matches are replaced.
	//
	// When Global is true, matches skipped via From (see below)
	// are not replaced.
	Global bool
	// From is the number of the first match to begin substituting.
	// For example:
	// If From is 1, substitution begins with the first match.
	// If From is 2, substitution begins with the second match,
	// and the first match is left unchanged.
	//
	// If From is less than 1, substitution begins with the first match.
	From int
}

// Sub returns a Substitute Edit
// that substitutes the first occurrence
// of the regular expression within a
// and sets dot to the modified address a.
func Sub(a Address, re, with string) Edit { return Substitute{A: a, RE: re, With: with, From: 1} }

// SubGlobal returns a Substitute Edit
// that substitutes the all occurrences
// of the regular expression within a
// and sets dot to the modified address a.
func SubGlobal(a Address, re, with string) Edit {
	return Substitute{A: a, RE: re, With: with, Global: true, From: 1}
}

func (e Substitute) String() string {
	s := e.A.String() + "s"
	if e.From > 1 {
		s += strconv.Itoa(e.From)
	}
	if e.RE == "" {
		e.RE = "/"
	}
	s += withTrailingDelim(e.RE) + e.With
	if e.Global {
		delim, _ := utf8.DecodeRuneInString(e.RE)
		s += string(delim) + "g"
	}
	return s
}

func (e Substitute) do(ed *Editor, _ io.Writer) (addr, error) {
	if e.From < 1 {
		e.From = 1
	}
	at, err := e.A.where(ed)
	if err != nil {
		return addr{}, err
	}
	re, err := re1.Compile([]rune(e.RE), re1.Options{Delimited: true})
	if err != nil {
		return addr{}, err
	}
	from := at.from
	for {
		m, err := subSingle(ed, addr{from, at.to}, re, e.With, e.From)
		if err != nil {
			return addr{}, err
		}
		if !e.Global || m == nil || m[0][1] == at.to {
			break
		}
		from = m[0][1]
		e.From = 1 // reset n to 1, so that on future iterations of this loop we get the next instance.
	}
	return at, nil

}

// SubSingle substitutes the Nth match of the regular expression
// with the replacement specifier.
func subSingle(ed *Editor, at addr, re *re1.Regexp, with string, n int) ([][2]int64, error) {
	m, err := nthMatch(ed, at, re, n)
	if err != nil || m == nil {
		return m, err
	}
	rs, err := replRunes(ed, m, with)
	if err != nil {
		return nil, err
	}
	at = addr{m[0][0], m[0][1]}
	return m, pend(ed, at, runes.SliceReader(rs))
}

// nthMatch skips past the first n-1 matches of the regular expression
func nthMatch(ed *Editor, at addr, re *re1.Regexp, n int) ([][2]int64, error) {
	var err error
	var m [][2]int64
	if n == 0 {
		n = 1
	}
	for i := 0; i < n; i++ {
		m, err = match(ed, at, re)
		if err != nil || m == nil {
			return nil, err
		}
		at.from = m[0][1]
	}
	return m, err
}

// ReplRunes returns the runes that replace a matched regexp.
func replRunes(ed *Editor, m [][2]int64, with string) ([]rune, error) {
	var rs []rune
	repl := []rune(with)
	for i := 0; i < len(repl); i++ {
		d := escDigit(repl[i:])
		if d < 0 {
			rs = append(rs, repl[i])
			continue
		}
		sub, err := subExprMatch(ed, m, d)
		if err != nil {
			return nil, err
		}
		rs = append(rs, sub...)
		i++
	}
	return rs, nil
}

// EscDigit returns the digit from \[0-9]
// or -1 if the text does not represent an escaped digit.
func escDigit(sub []rune) int {
	if len(sub) >= 2 && sub[0] == '\\' && unicode.IsDigit(sub[1]) {
		return int(sub[1] - '0')
	}
	return -1
}

// SubExprMatch returns the runes of a matched subexpression.
func subExprMatch(ed *Editor, m [][2]int64, i int) ([]rune, error) {
	if i < 0 || i >= len(m) {
		return []rune{}, nil
	}
	n := m[i][1] - m[i][0]
	if n > MaxRunes {
		return nil, errors.New("subexpression too big")
	}
	rs, err := ed.buf.runes.Read(int(n), m[i][0])
	if err != nil {
		return nil, err
	}
	return rs, nil
}

type runeSlice struct {
	buf *runes.Buffer
	addr
	err error
}

func (rs *runeSlice) Size() int64 { return rs.size() }

func (rs *runeSlice) Rune(i int64) rune {
	switch {
	case i < 0 || i >= rs.size():
		panic("index out of bounds")
	case rs.err != nil:
		return -1
	}
	r, err := rs.buf.Rune(rs.from + i)
	if err != nil {
		rs.err = err
		return -1
	}
	return r
}

// Match returns the results of matching a regular experssion
// within an address range in an Editor.
func match(ed *Editor, at addr, re *re1.Regexp) ([][2]int64, error) {
	rs := &runeSlice{buf: ed.buf.runes, addr: at}
	m := re.Match(rs, 0)
	for i := range m {
		m[i][0] += at.from
		m[i][1] += at.from
	}
	return m, rs.err
}
