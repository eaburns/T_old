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

// Ed parses and returns an Edit and the remaining, unparsed runes.
//
// In the following, text surrounded by / represents delimited text.
// The delimiter can be any character, it need not be /.
// Trailing delimiters may be elided, but the opening delimiter must be present.
// In delimited text, \ is an escape; the following character is interpreted literally,
// except \n which represents a literal newline.
// Items in {} are optional.
//
// The edit language is:
//	addr
//		Sets the address of Dot.
// 	{addr} a/text/
//	or
//	{addr} a
//	lines of text
//	.
//		Appends after the addressed text.
//		If an address is not supplied, dot is used.
//		Dot is set to the address.
//	{addr} c
//	{addr} i
//		Just like a, but c changes the addressed text
//		and i inserts before the addressed text.
//		Dot is set to the address.
//	{addr} d
//		Deletes the addressed text.
//		If an address is not supplied, dot is used.
//		Dot is set to the address.
//	{addr} t {addr}
//	{addr} m {addr}
//		Copies or moves runes from the first address to after the second.
//		Dot is set to the newly inserted or moved runes.
//	{addr} s{n}/regexp/text/{g}
//		Substitute substitutes text for the first match
// 		of the regular expression in the addressed range.
// 		When substituting, a backslash followed by a digit d
// 		stands for the string that matched the d-th subexpression.
//		\n is a literal newline.
//		A number n after s indicates we substitute the Nth match in the
//		address range. If n == 0 set n = 1.
// 		If the delimiter after the text is followed by the letter g
//		then all matches in the address range are substituted.
//		If a number n and the letter g are both present then the Nth match
//		and all subsequent matches in the address range are	substituted.
//		If an address is not supplied, dot is used.
//		Dot is set to the modified address.
//	{addr} k {[a-zA-Z]}
//		Sets the named mark to the address.
//		If an address is not supplied, dot is used.
//		If a mark name is not given, dot is set.
//		Dot is set to the address.
//	{addr} p
//		Returns the runes identified by the address.
//		If an address is not supplied, dot is used.
// 		It is an error to print more than MaxRunes runes.
//		Dot is set to the address.
//	{addr} ={#}
//		Without '#' returns the line offset(s) of the address.
//		With '#' returns the rune offsets of the address.
//		If an address is not supplied, dot is used.
//		Dot is set to the address.
func Ed(e []rune) (Edit, []rune, error) {
	edit, left, err := ed(e)
	for len(left) > 0 && unicode.IsSpace(left[0]) {
		var r rune
		r, left = left[0], left[1:]
		if r == '\n' {
			break
		}
	}
	return edit, left, err
}

func ed(e []rune) (edit Edit, left []rune, err error) {
	a, e, err := addrOrDot(e)
	switch {
	case err != nil:
		return nil, e, err
	case len(e) == 0 || e[0] == '\n':
		return Set(a, '.'), e, nil
	}
	switch c, e := e[0], e[1:]; c {
	case 'a', 'c', 'i':
		var rs []rune
		rs, e = parseText(e)
		switch c {
		case 'a':
			return Append(a, string(rs)), e, nil
		case 'c':
			return Change(a, string(rs)), e, nil
		case 'i':
			return Insert(a, string(rs)), e, nil
		}
		panic("unreachable")
	case 'd':
		return Delete(a), e, nil
	case 'k':
		mk, e, err := parseMarkRune(e)
		if err != nil {
			return nil, e, err
		}
		return Set(a, mk), e, nil
	case 'p':
		return Print(a), e, nil
	case '=':
		if len(e) == 0 || e[0] != '#' {
			return WhereLine(a), e, nil
		}
		return Where(a), e[1:], nil
	case 't', 'm':
		a1, e, err := addrOrDot(e)
		if err != nil {
			return nil, e, err
		}
		if c == 't' {
			return Copy(a, a1), e, nil
		}
		return Move(a, a1), e, nil
	case 's':
		n, e, err := parseNumber(e)
		if err != nil {
			return nil, e, err
		}
		exp, e, err := parseRegexp(e)
		if err != nil {
			return nil, e, err
		}
		if len(exp) < 2 || len(exp) == 2 && exp[0] == exp[1] {
			// len==1 is just the open delim.
			// len==2 && exp[0]==exp[1] is just open and close delim.
			return nil, e, errors.New("missing pattern")
		}
		repl, e := parseDelimited(exp[0], e)
		sub := Substitute{
			A:    a,
			RE:   string(exp),
			With: string(repl),
			From: n,
		}
		if len(e) > 0 && e[0] == 'g' {
			sub.Global = true
			e = e[1:]
		}
		return sub, e, nil
	default:
		return nil, e, errors.New("unknown command: " + string(c))
	}
}

func addrOrDot(e []rune) (Address, []rune, error) {
	a, e, err := parseCompoundAddr(e)
	switch {
	case err != nil:
		return nil, e, err
	case a == nil:
		a = Dot
	}
	return a, e, err
}

func parseText(e []rune) ([]rune, []rune) {
	var i int
	for i < len(e) && unicode.IsSpace(e[i]) {
		if e[i] == '\n' {
			return parseLines(e[i+1:])
		}
		i++
	}
	if i == len(e) {
		return nil, e
	}
	return parseDelimited(e[i], e[i+1:])
}

func parseLines(e []rune) ([]rune, []rune) {
	var i int
	var nl bool
	for i = 0; i < len(e); i++ {
		if nl && e[i] == '.' {
			switch {
			case i == len(e)-1:
				return e[:i], e[i+1:]
			case i < len(e)-1 && e[i+1] == '\n':
				return e[:i], e[i+2:]
			}
		}
		nl = e[i] == '\n'
	}
	return e, e[i:]
}

// ParseDelimited returns the runes
// up to the first unescaped delimiter,
// raw newline (rune 0xA),
// or the end of the slice
// and the remaining, unconsumed runes.
// A delimiter preceeded by \ is escaped and is non-terminating.
// The letter n preceeded by \ is a newline literal.
func parseDelimited(delim rune, e []rune) ([]rune, []rune) {
	var i int
	var rs []rune
	for i = 0; i < len(e); i++ {
		switch {
		case e[i] == delim || e[i] == '\n':
			return rs, e[i+1:]
		case i < len(e)-1 && e[i] == '\\' && e[i+1] == delim:
			rs = append(rs, delim)
			i++
		case i < len(e)-1 && e[i] == '\\' && e[i+1] == 'n':
			rs = append(rs, '\n')
			i++
		default:
			rs = append(rs, e[i])
		}
	}
	return rs, nil
}

func parseMarkRune(e []rune) (rune, []rune, error) {
	var i int
	if i < len(e) && isMarkRune(e[i]) {
		return e[i], e[i+1:], nil
	} else if i == len(e) {
		return '.', nil, nil
	}
	return ' ', e[i:], errors.New("bad mark: " + string(e[i]))
}

// parseNumber parses and returns a positive integer. The first returned
// value is the parsed number, the second is the number of runes parsed.
func parseNumber(e []rune) (int, []rune, error) {
	for len(e) > 0 && unicode.IsSpace(e[0]) && e[0] != '\n' {
		e = e[1:]
	}

	i := 0
	n := 1 // by default use the first instance
	var err error
	for len(e) > i && unicode.IsDigit(e[i]) {
		i++
	}
	if i != 0 {
		n, err = strconv.Atoi(string(e[:i]))
		if err != nil {
			return 0, e[:], err
		}
	}
	return n, e[i:], nil
}

func parseRegexp(e []rune) ([]rune, []rune, error) {
	// re1 doesn't special-case raw newlines.
	// We need them to terminate the regexp.
	// So, we split on newline (if any),
	// parse the first line with re1,
	// and rejoin the rest of the lines.
	var rest []rune
	for i, r := range e {
		if r == '\n' {
			e, rest = e[:i], e[i:]
			break
		}
	}
	for len(e) > 0 && unicode.IsSpace(e[0]) {
		e = e[1:]
	}

	re, err := re1.Compile(e, re1.Options{Delimited: true})
	if err != nil {
		return nil, e, err
	}
	exp := re.Expression()
	return exp, append(e[len(exp):], rest...), nil
}
