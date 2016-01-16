// Copyright © 2015, The T Authors.

// Package edit provides a language and functions for editing buffers of runes.
// This package is heavily inspired by the text editor Sam.
// In fact, the edit language of this package
// is a dialect of the edit language of Sam.
// For background, see the sam(1) manual page:
// http://swtch.com/plan9port/man/man1/sam.html.
//
// This package has four primary types: Buffer, Editor, Address, and Edit.
// These are described in detail below.
//
// Buffer
//
// Buffers are infinite-capacity, disk-backed, buffers of runes.
// New Buffers, created with the NewBuffer function, are empty.
// The only operation that can be done directly to a Buffer is closing it
// with the Buffer.Close method.
// All other operations are done using an Editor.
//
// Editor
//
// Editors view and modify Buffers.
// Multiple Editors may operate on a single Buffer concurrently.
// However, each Editor maintains its own state.
// This state includes both 'dot'
// (which is what T calls the position of the cursor)
// and 'marks' (which are like bookmarks into a Buffer).
//
// One common use of an Editor is to call its Do.
// The Do method performs an Edit,
// which views or changes the Buffer,
// typically at a particular Address.
// Editors can also make streaming changes.
// Streaming changes update the contents at an Address in the Buffer
// using the data read from an io.Reader.
// Likewise Editors can provide a streaming view of the Buffer
// by writing the contents at an Address to an io.Writer.
//
// Address
//
// Addresses identify a substring of runes within a buffer.
// They can be constructed in two different ways.
//
// The first way is by parsing a string in T's address language.
// This is intended for handling addresses from user input.
// An Address, specified by the user, can be parsed using the Addr function.
// The return value is the Address,
// the left over characters that weren't part of the parsed Address,
// and any errors encountered while parsing.
//
// For example:
// 	addr, leftOver, err := Addr([]rune("1+#3,/the/+8"))
//
// For details about the address language,
// see the document on the Addr function below.
//
// The second way is by using function and method calls.
// This is intended for creating Addresses programmatically or in source code.
// The functions and methods make it difficult to create an invalid address.
// Errors creating addresses this way
// are typically reported by the compiler
// at compile-time.
//
// For example:
//	addr := Line(1).Plus(Rune(3)).To(Regexp("/the/").Plus(Line(8)))
//
// Edit
//
// Edits describe an operation that an Editor can perform on a Buffer.
// Like with Addresses, they can be constructed in two different ways.
//
// The first way is by parsing a string in T's edit language.
// This is intended for handling edits from user input.
// It goes hand-in-hand with the address language.
// In fact, the address language is a subset of the edit language.
// An Edit, specified by the user, can be parsed using the Ed function.
// The return value is the Edit,
// the left over characters that weren't part of the parsed Edit,
// and any errors encountered while parsing.
//
// Here's an example:
//	edit, leftOver, err := Ed("1+#3,/the/+8 c/new text/")
//
// For details about the edit language,
// see the document on the Ed function below.
//
// The second way is by using function calls.
// This is intended for creating Edits programmatically or in source code.
// Like with Addresses, this technique makes it harder to create an invalid edit,
// and errors can be reported at compile-time.
//
// Here's example:
// 	addr := Line(1).Plus(Rune(3)).To(Regexp("/the/").Plus(Line(8)))
//	edit := Change(addr, "new text")
//
// Once created,
// regardless of whether by parsing the edit language or using functions,
// Edits can be applied using Editor.Do as described above.
// The Do method can either
// change the contents of the Buffer,
// change the Editor's state based on the contents of the Buffer,
// print runes from the Buffer or information about the Buffer to an io.Writer,
// or a mix of the above.
//
// Here are a few examples:
// 	ed := NewEditor(NewBuffer())
//	// Discards any printed output, of which there is none from the Insert Edit.
//	ed.Do(Insert(End, "Hello, World!"), ioutil.Discard)
//	// Prints the runes within the given address to standard output.
//	ed.Do(Print(Rune(1).To(Rune(10)), os.Stdout)
//	// Substitutes "World" with "世界". Nothing is printed.
//	ed.Do(Sub("/World/", "世界"), ioutil.Discard)
//	// Prints the address of dot, the cursor, to standard output.
//	ed.Do(Where(Dot), os.Stdout)
//
// A note on regular expressions
//
// The Go regexp package is very good,
// but it doesn't support reverse matching,
// which is required by T.
// T has its own regexp implementation
// called re1.
// Re1 is an implementation of Plan9 regular expressions.
// It is documented here:
// https://godoc.org/github.com/eaburns/T/re1.
package edit

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
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
// The mark m can by any rune.
// If the mark is . or whitespace then dot is set to a.
func Set(a Address, m rune) Edit {
	if unicode.IsSpace(m) {
		m = '.'
	}
	return set{a: a, m: m}
}

func (e set) String() string { return e.a.String() + "k" + string(e.m) }

func (e set) do(ed *Editor, _ io.Writer) (addr, error) {
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

type pipe struct {
	cmd      string
	a        Address
	to, from bool
}

// Pipe returns an Edit
// that sends the string at an address
// to the standard input of a command
// and replaces the string
// with the command's standard output.
// If the command outputs to standard error,
// that is written to the io.Writer
// supplied to Editor.Do.
//
// The command is executed through the shell
// as an argument to "-c".
// The shell is either the value of
// the SHELL environment variable
// or DefaultShell if SHELL is unset.
func Pipe(a Address, cmd string) Edit {
	return pipe{cmd: cmd, a: a, to: true, from: true}
}

// PipeTo returns an Edit like Pipe,
// but the standard output of the command
// is written to the writer,
// and does not overwrite the address a.
func PipeTo(a Address, cmd string) Edit {
	return pipe{cmd: cmd, a: a, to: true}
}

// PipeFrom returns an Edit like Pipe,
// but the standard input of the command
// is connected to an empty reader.
func PipeFrom(a Address, cmd string) Edit {
	return pipe{cmd: cmd, a: a, from: true}
}

func (e pipe) String() string {
	pipe := "|"
	if !e.to {
		pipe = "<"
	} else if !e.from {
		pipe = ">"
	}
	return e.a.String() + pipe + escNewlines(e.cmd) + "\n"
}

func escNewlines(s string) string {
	var esc string
	for _, r := range s {
		if r == '\n' {
			esc += `\n`
		} else {
			esc += string(r)
		}
	}
	return esc
}

// DefaultShell is the default shell
// which is used to execute commands
// if the SHELL environment variable
// is not set.
const DefaultShell = "/bin/sh"

func shell() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	return DefaultShell
}

func (e pipe) do(ed *Editor, w io.Writer) (addr, error) {
	at, err := e.a.where(ed)
	if err != nil {
		return addr{}, err
	}

	cmd := exec.Command(shell(), "-c", e.cmd)
	cmd.Stderr = w

	if e.to {
		r := ed.buf.runes.Reader(at.from)
		r = runes.LimitReader(r, at.size())
		cmd.Stdin = runes.UTF8Reader(r)
	}

	if !e.from {
		cmd.Stdout = w
		if err := cmd.Run(); err != nil {
			return addr{}, err
		}
		return at, nil
	}

	r, err := cmd.StdoutPipe()
	if err != nil {
		return addr{}, err
	}
	if err := cmd.Start(); err != nil {
		return addr{}, err
	}
	pendErr := pend(ed, at, runes.RunesReader(bufio.NewReader(r)))
	if err = cmd.Wait(); err != nil {
		return addr{}, err
	}
	if pendErr != nil {
		return addr{}, pendErr
	}
	return at, nil
}

type undo int

// Undo returns an Edit
// that undoes the n most recent changes
// made to the buffer,
// and sets dot to the address
// covering the last undone change.
// If n ≤ 0 then 1 change is undone.
func Undo(n int) Edit { return undo(n) }

func (e undo) String() string {
	if e <= 0 {
		return "u1"
	}
	return "u" + strconv.Itoa(int(e))
}

// Undo is special-cased by Editor.Do.
func (e undo) do(*Editor, io.Writer) (addr, error) { panic("unimplemented") }

type redo int

// Redo returns an Edit
// that redoes the n most recent changes
// undone by an Undo edit,
// and sets dot to the address
// covering the last redone change.
// If n ≤ 0 then 1 change is redone.
func Redo(n int) Edit { return redo(n) }

func (e redo) String() string {
	if e <= 0 {
		return "r1"
	}
	return "r" + strconv.Itoa(int(e))
}

// Redo is special-cased by Editor.Do.
func (e redo) do(*Editor, io.Writer) (addr, error) { panic("unimplemented") }

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
//	{addr} k {name}
//		Sets the named mark to the address.
//		If an address is not supplied, dot is used.
//		The name is any non-whitespace rune.
// 		If name is not supplied or is the rune . then dot is set.
//		Regardless of which mark is set,
// 		dot is also set to the address.
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
//	{addr} | cmd
//	{addr} < cmd
//	{addr} > cmd
//		| pipes the addressed string to standard input
//		of a shell command and overwrites the address
//		by the standard output of the command.
//		< and > are like |,
//		but < only overwrites with the command's standard output,
//		and > only pipes to the command's standard input.
//		If an address is not supplied, dot is used.
//		Dot is set to the address.
//
//	 	The command is passed as the argument of -c
//		to the shell in the SHELL environment variable.
//		If SHELL is unset, the value of DefaultShell is used.
//
//		Parsing of cmd is termiated by
//		either a newline or the end of input.
//		Within cmd, \n is interpreted as a newline literal.
//	u{n}
//		Undoes the n most recent changes
// 		made to the buffer by any editor.
//		If n is not specified, it defaults to 1.
//		Dot is set to the address covering
// 		the last undone change.
//	r{n}
//		Redoes the n most recent changes
//		undone by any editor.
//		If n is not specified, it defaults to 1.
//		Dot is set to the address covering
// 		the last redone change.
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
	a, e, err := parseCompoundAddr(e)
	switch {
	case err != nil:
		return nil, e, err
	case a == nil && len(e) > 0:
		switch e[0] {
		case 'u':
			n, e, err := parseNumber(e[1:])
			if err != nil {
				return nil, e, err
			}
			return Undo(n), e, nil

		case 'r':
			n, e, err := parseNumber(e[1:])
			if err != nil {
				return nil, e, err
			}
			return Redo(n), e, nil
		}
		fallthrough
	case a == nil:
		a = Dot
	}
	if len(e) == 0 || e[0] == '\n' {
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
		mk, e := parseMarkRune(e)
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
	case '|':
		cmd, e := parseCmd(e)
		return Pipe(a, cmd), e, nil
	case '>':
		cmd, e := parseCmd(e)
		return PipeTo(a, cmd), e, nil
	case '<':
		cmd, e := parseCmd(e)
		return PipeFrom(a, cmd), e, nil
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

func parseMarkRune(e []rune) (rune, []rune) {
	for len(e) > 0 && unicode.IsSpace(e[0]) {
		e = e[1:]
	}
	if len(e) == 0 {
		return '.', nil
	}
	return e[0], e[1:]
}

// ParseNumber parses and returns a positive integer.
// The first returned value is the number,
// the second is the number of runes parsed.
// If there is no error and 0 runes are parsed,
// the number returned is 1.
func parseNumber(e []rune) (int, []rune, error) {
	for len(e) > 0 && unicode.IsSpace(e[0]) && e[0] != '\n' {
		e = e[1:]
	}

	i := 0
	n := 1 // by default return 1
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

func parseCmd(e []rune) (string, []rune) {
	var cmd string
	for len(e) > 0 && unicode.IsSpace(e[0]) && e[0] != '\n' {
		e = e[1:]
	}
	for len(e) > 0 {
		var r rune
		switch r, e = e[0], e[1:]; {
		case r == '\\' && len(e) > 0 && e[0] == 'n':
			cmd += "\n"
			e = e[1:]
		case r == '\n':
			return cmd, e
		default:
			cmd += string(r)
		}
	}
	return cmd, e
}
