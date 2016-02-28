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
// 	addr, leftOver, err := Addr(strings.NewReader("1+#3,/the/+8"))
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
//	addr := Line(1).Plus(Rune(3)).To(Regexp("the").Plus(Line(8)))
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
//	edit, leftOver, err := Ed(strings.NewReader("1+#3,/the/+8 c/new text/"))
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
// 	addr := Line(1).Plus(Rune(3)).To(Regexp("the").Plus(Line(8)))
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
package edit

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"unicode"
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

func (e change) String() string {
	if e.op == 'd' {
		return e.a.String() + "d"
	}
	return e.a.String() + string(e.op) + "/" + Escape(e.str, '/') + "/"
}

func (e change) do(ed *Editor, _ io.Writer) (addr, error) {
	at, err := ed.where(e.a)
	if err != nil {
		return addr{}, err
	}
	switch e.op {
	case 'a':
		at.from = at.to
	case 'i':
		at.to = at.from
	}
	return at, pend(ed, at, runes.StringReader(e.str))
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
	s, err := ed.where(e.src)
	if err != nil {
		return addr{}, err
	}
	d, err := ed.where(e.dst)
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
	s, err := ed.where(e.src)
	if err != nil {
		return addr{}, err
	}
	d, err := ed.where(e.dst)
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
	at, err := ed.where(e.a)
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
	at, err := ed.where(e.a)
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
	at, err := ed.where(e.a)
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
	//
	// The regular expression syntax is that of the standard library regexp package.
	// The syntax is documented here: https://github.com/google/re2/wiki/Syntax.
	// All regular expressions are wrapped in (?m:<re>), making them multi-line by default.
	// The beginning and end of the address A
	// are the beginning and end of text for the regexp match.
	// So given:
	// 	xyzabc123
	// The substitution #3,#6s/^abc$/αβξ will result in:
	// 	xyzαβξ123
	RE string
	// With is the template with which to replace each match of RE.
	// The syntax is that of the standard regexp package's Regexp.Expand method
	// described here: https://golang.org/pkg/regexp/#Regexp.Expand.
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
func Sub(a Address, re, with string) Edit {
	return Substitute{A: a, RE: re, With: with, From: 1}
}

// SubGlobal returns a Substitute Edit
// that substitutes the all occurrences
// of the regular expression within a
// and sets dot to the modified address a.
func SubGlobal(a Address, re, with string) Edit {
	return Substitute{A: a, RE: re, With: with, Global: true, From: 1}
}

func (e Substitute) String() string {
	var n string
	if e.From > 1 {
		n = strconv.Itoa(e.From)
	}
	var g string
	if e.Global {
		g = "g"
	}
	return e.A.String() + "s" + n + "/" + Escape(e.RE, '/') + "/" + Escape(e.With, '/') + "/" + g
}

func (e Substitute) do(ed *Editor, _ io.Writer) (addr, error) {
	at, err := ed.where(e.A)
	if err != nil {
		return addr{}, err
	}
	re, err := regexpCompile(e.RE)
	if err != nil {
		return addr{}, err
	}

	var prev []int
	from := at.from
	for from <= at.to { // Allow one run on an empty input.
		m := match(re, Span{from, at.to}, ed)
		if len(m) < 2 {
			break
		}
		if m[0] == m[1] {
			from++
		} else {
			from = int64(m[1])
		}

		if len(prev) >= 2 && m[0] == m[1] && m[1] == prev[1] {
			// Skip an empty match immediately following the previous match.
			prev = m
			continue
		}
		prev = m

		e.From--
		if e.From <= 0 {
			if err := regexpSub(re, m, e.With, ed); err != nil {
				return addr{}, nil
			}
			if !e.Global {
				break
			}
		}
	}
	return at, nil
}

func regexpSub(re *regexp.Regexp, match []int, with string, ed *Editor) error {
	m := addr{from: int64(match[0]), to: int64(match[1])}
	r := runes.LimitReader(ed.buf.runes.Reader(m.from), m.size())
	src, err := ioutil.ReadAll(runes.UTF8Reader(r))
	if err != nil {
		return err
	}

	matchSrc := make([]int, len(match))
	var bi, ri int
	for {
		for i := range match {
			if match[i]-match[0] == ri {
				matchSrc[i] = bi
			}
		}
		if bi >= len(src) {
			break
		}
		_, w := utf8.DecodeRune(src[bi:])
		bi += w
		ri++
	}

	repl := re.Expand(nil, []byte(with), src, matchSrc)
	return pend(ed, m, runes.ByteReader(repl))
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
	var esc []rune
	for _, r := range s {
		if r == '\n' {
			esc = append(esc, '\\', 'n')
		} else {
			esc = append(esc, r)
		}
	}
	return string(esc)
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
	at, err := ed.where(e.a)
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

// Ed parses and returns an Edit.
//
// Edits are terminated by a newline, end of input, or the end of the edit.
// For example:
// 	1,5
// 	d
// 		Is terminated at the newline precceding d.
// 		The newline is not consumed.
//
//	1,5a/xyz
// 		Is terminated at z at the end of the input.
//
// 	1,5dabc
// 		Is terminated at d, the end of the edit.
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
//		Appends text after the address.
// 		In the text, all \, raw newlines, and / must be escaped with \.
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
//		Substitute substitutes matches of regexp within the address.
//
// 		The regexp uses the syntax of the standard library regexp package.
// 		The regexp is wrapped in (?m:<regexp>), making it multi-line by default.
// 		The replacement text uses the systax of Regexp.Expand method,
// 		described here: https://golang.org/pkg/regexp/#Regexp.Expand.,
// 		The runes \, raw newlines, and / must be escaped with \
// 		in both the regexp and replacement text.
// 		For example, ,s/\\s+/\\/g replaces runs of whitespace with \.
//
//		A number n after s indicates we substitute the Nth match in the
//		address range. If n == 0 set n = 1.
// 		If the delimiter after the text is followed by the letter g
//		then all matches in the address range are substituted.
//		If a number n and the letter g are both present then the Nth match
//		and all subsequent matches in the address range are substituted.
//
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
func Ed(rs io.RuneScanner) (Edit, error) {
	a, err := Addr(rs)
	switch {
	case err != nil:
		return nil, err
	case a == nil:
		switch r, _, err := rs.ReadRune(); {
		case err == io.EOF:
			return Set(Dot, '.'), nil
		case err != nil:
			return nil, err
		case r == 'u':
			n, err := parseNumber(rs)
			if err != nil {
				return nil, err
			}
			return Undo(n), nil
		case r == 'r':
			n, err := parseNumber(rs)
			if err != nil {
				return nil, err
			}
			return Redo(n), nil
		default:
			if err := rs.UnreadRune(); err != nil {
				return nil, err
			}
			a = Dot
		}
	}
	switch r, _, err := rs.ReadRune(); {
	case err != nil && err != io.EOF:
		return nil, err
	case err == nil && r == '\n':
		if err := rs.UnreadRune(); err != nil {
			return nil, err
		}
		fallthrough
	case err == io.EOF:
		return Set(a, '.'), nil
	case r == 'a' || r == 'c' || r == 'i':
		text, err := parseText(rs)
		if err != nil {
			return nil, err
		}
		switch r {
		case 'a':
			return Append(a, text), nil
		case 'c':
			return Change(a, text), nil
		default: // case 'i'
			return Insert(a, text), nil
		}
	case r == 'd':
		return Delete(a), nil
	case r == 'k':
		m, err := parseMarkRune(rs)
		if err != nil {
			return nil, err
		}
		return Set(a, m), nil
	case r == 'p':
		return Print(a), nil
	case r == '=':
		switch r, _, err := rs.ReadRune(); {
		case err == io.EOF:
			return WhereLine(a), nil
		case err != nil:
			return nil, err
		case r == '#':
			return Where(a), nil
		default:
			if err := rs.UnreadRune(); err != nil {
				return nil, err
			}
			return WhereLine(a), nil
		}
	case r == 't' || r == 'm':
		a1, err := parseAddrOrDot(rs)
		if err != nil {
			return nil, err
		}
		if r == 't' {
			return Copy(a, a1), nil
		}
		return Move(a, a1), nil
	case r == 's':
		n, err := parseNumber(rs)
		if err != nil {
			return nil, err
		}
		if err := skipSpace(rs); err != nil {
			return nil, err
		}
		delim, _, err := rs.ReadRune()
		switch {
		case err != nil && err != io.EOF:
			return nil, err
		case err == io.EOF:
			return Sub(a, "", ""), nil
		case delim == '\n':
			return Sub(a, "", ""), rs.UnreadRune()
		}
		re, err := parseDelimited(delim, rs)
		if err != nil {
			return nil, err
		}
		if _, err := regexpCompile(re); err != nil {
			return nil, err
		}
		with, err := parseDelimited(delim, rs)
		if err != nil {
			return nil, err
		}
		sub := Substitute{A: a, RE: re, With: with, From: n}
		switch r, _, err := rs.ReadRune(); {
		case err == io.EOF:
			return sub, nil
		case err != nil:
			return nil, err
		case r == 'g':
			sub.Global = true
		default:
			if err := rs.UnreadRune(); err != nil {
				return nil, err
			}
		}
		return sub, nil
	case r == '|' || r == '>' || r == '<':
		c, err := parseCmd(rs)
		if err != nil {
			return nil, err
		}
		switch r {
		case '|':
			return Pipe(a, c), nil
		case '>':
			return PipeTo(a, c), nil
		default: // case '<'
			return PipeFrom(a, c), nil
		}
	default:
		return nil, errors.New("unknown command: " + string(r))
	}
}

func parseAddrOrDot(rs io.RuneScanner) (Address, error) {
	switch a, err := Addr(rs); {
	// parseCompoundAddr returns never returns io.EOF, but nil, nil.
	case err != nil:
		return nil, err
	case a == nil:
		return Dot, nil
	default:
		return a, err
	}
}

func parseText(rs io.RuneScanner) (string, error) {
	for {
		switch r, _, err := rs.ReadRune(); {
		case err == io.EOF:
			return "", nil
		case err != nil:
			return "", err
		case r == '\n':
			return parseLines(rs)
		case unicode.IsSpace(r):
			continue
		default:
			return parseDelimited(r, rs)
		}
	}
}

func parseLines(rs io.RuneScanner) (string, error) {
	var s []rune
	var nl bool
	for {
		switch r, _, err := rs.ReadRune(); {
		case err == io.EOF:
			return string(s), nil
		case err != nil:
			return "", err
		case nl && r == '.':
			return string(s), nil
		default:
			s = append(s, r)
			nl = r == '\n'
		}
	}
}

// ParseDelimited returns the unescaped string of runes
// up to the first non-escaped delimiter, raw newline, or EOF.
func parseDelimited(delim rune, rs io.RuneScanner) (string, error) {
	var s []rune
	var esc bool
	for {
		switch r, _, err := rs.ReadRune(); {
		case err != nil && err != io.EOF:
			return "", err
		case err == io.EOF || !esc && r == delim:
			return Unescape(string(s)), nil
		case r == '\n':
			return Unescape(string(s)), rs.UnreadRune()
		default:
			s = append(s, r)
			esc = !esc && r == '\\'
		}
	}
}

// Escape returns the string,
// with \ inserted before all occurrences of
// \, raw newlines, and runes in esc.
func Escape(str string, esc ...rune) string {
	// Always escape \ and raw newlines.
	esc = append(esc, '\\')
	var s []rune
	for _, r := range str {
		if r == '\n' {
			s = append(s, '\\', 'n')
			continue
		}
		for _, e := range esc {
			if r == e {
				s = append(s, '\\')
				break
			}
		}
		s = append(s, r)
	}
	return string(s)
}

// Unescape returns the string,
// with all occurrences of \n replaced by a raw newline,
// and all occurrences of \ followed by any other rune with the rune.
//
// If the last rune is \ that is not preceded by a \,
// it remains unchanged as a trailing \.
func Unescape(str string) string {
	var s []rune
	var esc bool
	for _, r := range str {
		if !esc && r == '\\' {
			esc = true
			continue
		}
		if esc && r == 'n' {
			s = append(s, '\n')
		} else {
			s = append(s, r)
		}
		esc = false
	}
	if esc {
		s = append(s, '\\')
	}
	return string(s)
}

func parseMarkRune(rs io.RuneScanner) (rune, error) {
	for {
		switch r, _, err := rs.ReadRune(); {
		case err == io.EOF:
			return '.', nil
		case err != nil:
			return 0, err
		case unicode.IsSpace(r):
			continue
		default:
			return r, nil
		}
	}
}

// ParseNumber parses and returns a positive integer.
// Leading spaces are ignored.
// If EOF is reached before any digits are encountered, 1 is returned.
func parseNumber(rs io.RuneScanner) (int, error) {
	if err := skipSpace(rs); err != nil {
		return 0, err
	}
	var s []rune
	for {
		switch r, _, err := rs.ReadRune(); {
		case err == io.EOF:
			break
		case err != nil:
			return 0, err
		case unicode.IsDigit(r):
			s = append(s, r)
			continue
		default:
			if err := rs.UnreadRune(); err != nil {
				return 0, err
			}
		}

		if len(s) == 0 {
			return 1, nil
		}
		return strconv.Atoi(string(s))
	}
}

func parseCmd(rs io.RuneScanner) (string, error) {
	if err := skipSpace(rs); err != nil {
		return "", err
	}
	var esc bool
	var s []rune
	for {
		switch r, _, err := rs.ReadRune(); {
		case err == io.EOF:
			return string(s), nil
		case err != nil:
			return "", nil
		case r == '\n':
			return string(s), rs.UnreadRune()
		case r == '\\':
			esc = true
		case esc && r == 'n':
			s = append(s, '\n')
			esc = false
		default:
			if esc {
				s = append(s, '\\')
			}
			s = append(s, r)
			esc = false
		}
	}
}

// SkipSpace consumes and ignores non-newline whitespace.
// Terminates if a newline is encountered.
// The terminating newline remains consumed.
func skipSpace(rs io.RuneScanner) error {
	for {
		switch r, _, err := rs.ReadRune(); {
		case err == io.EOF:
			return nil
		case err != nil:
			return err
		case r != '\n' && unicode.IsSpace(r):
			continue
		default:
			return rs.UnreadRune()
		}
	}
}
