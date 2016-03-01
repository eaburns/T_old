// Copyright © 2015, The T Authors.

// Package edit provides a language and functions for editing text.
// This package is heavily inspired by the text editor Sam.
// In fact, the edit language of this package
// is a dialect of the edit language of Sam.
// For background, see the sam(1) manual page:
// http://swtch.com/plan9port/man/man1/sam.html.
//
//
// Text and Addresses
//
//
// The Text interface provides methods to operate on a read-only text.
// The text is accessed using Spans,
// defined by an inclusive start point
// and an exclusive end point.
// The units of a Span are unspecified,
// but are defined by a Text implementation,
// by way of its Size and ReadRune method.
//
// Addresses provide a high-level language for identifying Spans of a Text.
// They can be constructed in two different ways.
// The first way is by parsing a string in T's Address language.
// The Address language is parsed by the Addr function.
// Here's an example:
// 	// This Address identifies the Span from 3 runes past the end of line 1
// 	// until the end of 8th line following the next occurrence of "the".
// 	addr, err := Addr(strings.NewReader("1+#3,/the/+8"))
//
// See the documentation of the Addr function for more details.
//
// The second way to construct an Address is by using functions and methods.
// This is intended for creating Addresses programmatically or in source code.
// Unlike the Addr function, which reports errors at run-time,
// errors that occur while creating Addresses using these functions and methods
// are reported by the compiler at compile-time.
// Here's an example; it creates the same address as above:
//	addr := Line(1).Plus(Rune(3)).To(Regexp("the").Plus(Line(8)))
//
// Once created, whether by the Address language or using functions and methodts,
// Addresses can be evaluated on a Text using their Where method.
// The Where method returns the Span of the Text identified by the Address.
//
// Editor and Edits
//
// The Editor interface provides methods to operate on a read/write text.
// A text is modified with the Change, Apply, Undo, and Redo methods of the Editor.
// The Change method stages a change to a specified Span of the Text.
// It does not modify the Text itself.
// The Apply method modifies the Text by applying all staged changes in sequence.
// Undo and Redo, undo and redo batches of changes made with Apply.
//
// Edits provide a high-level language for modifying a Text.
// Like Addresses, they can be constructed in two different ways.
// The first way is by parsing a string in T's Edit language.
// The Edit language goes hand-in-hand with the Address language.
// In fact, the Address language is a subset of the Edit language.
// The Edit language is parsed by the Ed function.
// Here's an example:
// 	// This Edit changes the Span from 3 runes past the end of line 1
// 	// until the end of 8th line following the next occurrence of "the",
// 	// to have the text "new text".
//	e, err := Ed(strings.NewReader("1+#3,/the/+8 c/new text/"))
//
// See the documentation of the Ed function for more details.
//
// The second way to construct an Edit is by using functions.
// This is intended for creating Edits programmatically or in source code.
// Unlike the Ed function, which reports errors at run-time,
// errors that occur while creating Edits using these functions
// are reported by the compiler at compile-time.
// Here's example; it makes the same modification as above:
// 	addr := Line(1).Plus(Rune(3)).To(Regexp("the").Plus(Line(8)))
//	edit := Change(addr, "new text")
//
// Once created, whether by the Edit language or using functions,
// Edits can be applied to an Editor using their Do method.
// The Do method can either
// modify Text,
// change the Editor's state based on the contents of the Text,
// print text from the Text or information about the Text to an io.Writer,
// or a combination of the above of the above.
// It all depends on the Edit being performed.
//
// Buffer
//
// The Buffer type provides an implementation of the Editor interface.
// A Buffer is an infinite-capacity, disk-backed, buffers of runes.
package edit

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// An Edit is an operation that can be made on a Buffer by an Editor.
type Edit interface {
	// String returns the string representation of the edit.
	// The returned string will result in an equivalent edit
	// when parsed with Ed().
	String() string

	// Do performs the Edit on an Editor.
	// Anything printed by the Edit is written to the Writer.
	Do(Editor, io.Writer) error
}

type change struct {
	Address
	op  rune
	str string
}

// Change returns an Edit
// that changes the string at a to str,
// and sets dot to the changed runes.
func Change(a Address, str string) Edit { return change{Address: a, op: 'c', str: str} }

// Append returns an Edit
// that appends str after the string at a,
// and sets dot to the appended runes.
func Append(a Address, str string) Edit { return change{Address: a, op: 'a', str: str} }

// Insert returns an Edit
// that inserts str before the string at a,
// and sets dot to the inserted runes.
func Insert(a Address, str string) Edit { return change{Address: a, op: 'i', str: str} }

// Delete returns an Edit
// that deletes the string at a,
// and sets dot to the empty string
// that was deleted.
func Delete(a Address) Edit { return change{Address: a, op: 'd'} }

func (e change) String() string {
	if e.op == 'd' {
		return e.Address.String() + "d"
	}
	return e.Address.String() + string(e.op) + "/" + Escape(e.str, '/') + "/"
}

func (e change) Do(ed Editor, _ io.Writer) error {
	s, err := e.Where(ed)
	if err != nil {
		return err
	}
	switch e.op {
	case 'a':
		s[0] = s[1]
	case 'i':
		s[1] = s[0]
	}
	if err := ed.Change(s, strings.NewReader(e.str)); err != nil {
		return err
	}
	if err := ed.SetMark('.', s); err != nil {
		return err
	}
	return ed.Apply()
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

func (e move) Do(ed Editor, _ io.Writer) error {
	src, err := e.src.Where(ed)
	if err != nil {
		return err
	}
	dst, err := e.dst.Where(ed)
	if err != nil {
		return err
	}
	dst[0] = dst[1]

	if dst[0] > src[0] && dst[0] < src[1] {
		return errors.New("move addresses overlap")
	}

	if dst[0] >= src[1] {
		// Moving to after the source. Delete the source first.
		if err := ed.Change(src, strings.NewReader("")); err != nil {
			return err
		}
	}
	if err := ed.Change(dst, ed.Reader(src)); err != nil {
		return err
	}
	if dst[0] <= src[0] {
		// Moving to before the source. Delete the source second.
		if err := ed.Change(src, strings.NewReader("")); err != nil {
			return err
		}
	}
	if err := ed.SetMark('.', dst); err != nil {
		return err
	}
	return ed.Apply()

}

type copyEdit struct {
	src, dst Address
}

// Copy returns an Edit
// that copies runes from src to after dst
// and sets dot to the copied runes.
func Copy(src, dst Address) Edit { return copyEdit{src: src, dst: dst} }

func (e copyEdit) String() string { return e.src.String() + "t" + e.dst.String() }

func (e copyEdit) Do(ed Editor, _ io.Writer) error {
	src, err := e.src.Where(ed)
	if err != nil {
		return err
	}
	dst, err := e.dst.Where(ed)
	if err != nil {
		return err
	}
	dst[0] = dst[1]
	if err := ed.Change(dst, ed.Reader(src)); err != nil {
		return err
	}
	if err := ed.SetMark('.', dst); err != nil {
		return err
	}
	return ed.Apply()
}

type set struct {
	Address
	mark rune
}

// Set returns an Edit
// that sets the dot or mark m to a.
// The mark m can by any rune.
// If the mark is . or whitespace then dot is set to a.
func Set(a Address, m rune) Edit {
	if unicode.IsSpace(m) {
		m = '.'
	}
	return set{Address: a, mark: m}
}

func (e set) String() string { return e.Address.String() + "k" + string(e.mark) }

func (e set) Do(ed Editor, _ io.Writer) error {
	s, err := e.Where(ed)
	if err != nil {
		return err
	}
	return ed.SetMark(e.mark, s)
}

type print struct{ Address }

// Print returns an Edit
// that prints the string at a to an io.Writer
// and sets dot to the printed string.
func Print(a Address) Edit { return print{a} }

func (e print) String() string { return e.Address.String() + "p" }

func (e print) Do(ed Editor, print io.Writer) error {
	s, err := e.Where(ed)
	if err != nil {
		return err
	}
	if _, err := io.Copy(print, ed.Reader(s)); err != nil {
		return err
	}
	return ed.SetMark('.', s)
}

type where struct {
	Address
	line bool
}

// Where returns an Edit
// that prints the rune location of a
// to an io.Writer
// and sets dot to the a.
func Where(a Address) Edit { return where{Address: a} }

// WhereLine returns an Edit that prints both
// the rune address and the lines containing a
// to an io.Writer
// and sets dot to the a.
func WhereLine(a Address) Edit { return where{Address: a, line: true} }

func (e where) String() string {
	if e.line {
		return e.Address.String() + "="
	}
	return e.Address.String() + "=#"
}

func (e where) Do(ed Editor, print io.Writer) error {
	s, err := e.Where(ed)
	if err != nil {
		return err
	}
	if e.line {
		l0, l1, err := lines(ed, s)
		if err != nil {
			return err
		}
		if l0 == l1 {
			_, err = fmt.Fprintf(print, "%d", l0)
		} else {
			_, err = fmt.Fprintf(print, "%d,%d", l0, l1)
		}
	} else {
		if s.Size() == 0 {
			_, err = fmt.Fprintf(print, "#%d", s[0])
		} else {
			_, err = fmt.Fprintf(print, "#%d,#%d", s[0], s[1])
		}
	}
	if err != nil {
		return err
	}
	return ed.SetMark('.', s)
}

func lines(ed Editor, s Span) (l0, l1 int64, err error) {
	var i int64
	l0 = int64(1) // line numbers are 1 based.
	rr := ed.RuneReader(Span{0, ed.Size()})
	for ; i < s[0]; i++ {
		switch r, _, err := rr.ReadRune(); {
		case err != nil:
			return 0, 0, err
		case r == '\n':
			l0++
		}
	}
	l1 = l0
	for ; i < s[1]-1; i++ {
		switch r, _, err := rr.ReadRune(); {
		case err != nil:
			return 0, 0, err
		case r == '\n':
			l1++
		}
	}
	return l0, l1, nil
}

// Substitute is an Edit that substitutes regular expression matches.
type Substitute struct {
	// Address is the address in which to search for matches.
	// After performing the edit, Dot is set the modified address A.
	Address Address

	// Regexp is the regular expression to match.
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
	Regexp string

	// With is the template with which to replace each match of Regexp.
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
	return Substitute{Address: a, Regexp: re, With: with, From: 1}
}

// SubGlobal returns a Substitute Edit
// that substitutes the all occurrences
// of the regular expression within a
// and sets dot to the modified address a.
func SubGlobal(a Address, re, with string) Edit {
	return Substitute{Address: a, Regexp: re, With: with, Global: true, From: 1}
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
	return e.Address.String() + "s" + n + "/" + Escape(e.Regexp, '/') + "/" + Escape(e.With, '/') + "/" + g
}

func (e Substitute) Do(ed Editor, _ io.Writer) error {
	s, err := e.Address.Where(ed)
	if err != nil {
		return err
	}
	re, err := regexpCompile(e.Regexp)
	if err != nil {
		return err
	}

	var prev []int
	from := s[0]
	for from <= s[1] { // Allow one run on an empty input.
		m := match(re, Span{from, s[1]}, ed)
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
				return nil
			}
			if !e.Global {
				break
			}
		}
	}
	if err := ed.SetMark('.', s); err != nil {
		return err
	}
	return ed.Apply()

}

func regexpSub(re *regexp.Regexp, match []int, with string, ed Editor) error {
	dst := Span{int64(match[0]), int64(match[1])}
	src, err := ioutil.ReadAll(ed.Reader(dst))
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
	return ed.Change(dst, bytes.NewReader(repl))
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

func (e pipe) Do(ed Editor, print io.Writer) error {
	s, err := e.a.Where(ed)
	if err != nil {
		return err
	}

	cmd := exec.Command(shell(), "-c", e.cmd)
	cmd.Stderr = print

	if e.to {
		cmd.Stdin = ed.Reader(s)
	}

	if !e.from {
		cmd.Stdout = print
		if err := cmd.Run(); err != nil {
			return err
		}
		return ed.SetMark('.', s)
	}

	r, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	changeErr := ed.Change(s, r)
	if err = cmd.Wait(); err != nil {
		return err
	}
	if changeErr != nil {
		return changeErr
	}
	if err := ed.SetMark('.', s); err != nil {
		return err
	}
	return ed.Apply()
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

func (e undo) Do(ed Editor, _ io.Writer) error {
	if e <= 0 {
		e = 1
	}
	for i := 0; i < int(e); i++ {
		if err := ed.Undo(); err != nil {
			return err
		}
	}
	return nil
}

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

func (e redo) Do(ed Editor, _ io.Writer) error {
	if e <= 0 {
		e = 1
	}
	for i := 0; i < int(e); i++ {
		if err := ed.Redo(); err != nil {
			return err
		}
	}
	return nil
}

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
// 		made to the buffer by any Editor.
//		If n is not specified, it defaults to 1.
//		Dot is set to the address covering
// 		the last undone change.
//	r{n}
//		Redoes the n most recent changes
//		undone by any Editor.
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
		from, err := parseNumber(rs)
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
		sub := Substitute{Address: a, Regexp: re, With: with, From: from}
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
