/*
Package re1 is an implementation of Plan 9 regular expressions.
Plan 9 regular expressions are defined in the regexp(7) man page,
which can be found at http://swtch.com/plan9port/man/man7/regexp.html.

It reads:

A regular expression specifies a set of strings of characters. A member of this set of strings is said to be matched by the regular expression. In many applications a delimiter character, commonly /, bounds a regular expression. In the following specification for regular expressions the word ‘character’ means any character (rune) but newline.

The syntax for a regular expression e0 is
	e3:    literal | charclass | '.' | '^' | '$' | '(' e0 ')'
	e2:    e3 | e2 REP
	REP:   '*' | '+' | '?'
	e1:    e2 | e1 e2
	e0:    e1 | e0 '|' e1
A literal is any non-metacharacter, or a metacharacter (one of .*+?[]()|\^$), or the delimiter preceded by \.

A charclass is a nonempty string s bracketed [s] (or [^s]); it matches any character in (or not in) s. A negated character class never matches newline. A substring a−b, with a and b in ascending order, stands for the inclusive range of characters between a and b. In s, the metacharacters −, ], an initial ^, and the regular expression delimiter must be preceded by a \; other metacharacters have no special meaning and may appear unescaped.

A . matches any character.

A ^ matches the beginning of a line; $ matches the end of the line.

The REP operators match zero or more (*), one or more (+), zero or one (?), instances respectively of the preceding regular expression e2.

A concatenated regular expression, e1e2, matches a match to e1 followed by a match to e2.

An alternative regular expression, e0|e1, matches either a match to e0 or a match to e1.

A match to any part of a regular expression extends as far as possible without preventing a match to the remainder of the regular expression.
*/
package re1

import (
	"strconv"
)

// A Regexp is a compiled regular expression.
type Regexp struct{}

type token rune

// Meta tokens are negative numbers.
const (
	eof token = -1 - iota
	or
	dot
	star
	plus
	question
	dollar
	carrot
	oparen
	cparen
	obrace
	cbrace
)

func (t token) String() string {
	switch t {
	case eof:
		return "EOF"
	case or:
		return "|"
	case dot:
		return "."
	case star:
		return "*"
	case plus:
		return "+"
	case question:
		return "?"
	case dollar:
		return "$"
	case carrot:
		return "^"
	case oparen:
		return "("
	case cparen:
		return ")"
	case obrace:
		return "["
	case cbrace:
		return "]"
	default:
		return string([]rune{rune(t)})
	}
}

type parser struct {
	rs        []rune
	prev, pos int
	delim     rune // -1 for no delimiter.
	sub       int
}

func (p *parser) eof() bool {
	return p.pos == len(p.rs) || p.rs[p.pos] == p.delim
}

func (p *parser) back() {
	p.pos = p.prev
}

func (p *parser) peek() token {
	if p.eof() {
		return eof
	}
	t := p.next()
	p.back()
	return t
}

func (p *parser) next() (t token) {
	if p.eof() {
		return eof
	}
	p.prev = p.pos
	p.pos++
	switch r := p.rs[p.pos-1]; r {
	case '\\':
		switch {
		case p.pos == len(p.rs):
			return '\\'
		case p.rs[p.pos] == 'n':
			p.pos++
			return '\n'
		default:
			p.pos++
			return token(p.rs[p.pos-1])
		}
	case '.':
		return dot
	case '*':
		return star
	case '+':
		return plus
	case '?':
		return question
	case '[':
		return obrace
	case ']':
		return cbrace
	case '(':
		return oparen
	case ')':
		return cparen
	case '|':
		return or
	case '$':
		return dollar
	case '^':
		return carrot
	default:
		return token(r)
	}
}

// A ParseError records an error encountered while parsing a regular expression.
type ParseError struct {
	Position int
	Message  string
}

func (e ParseError) Error() string { return strconv.Itoa(e.Position) + ": " + e.Message }

// CompileDelim compiles the string into a regular expression.
// The first rune is assumed to be an opening delimiter.
// The regular expression is parsed until either
// the end of the input or an un-escaped closing delimiter.
// The return value is the regular expression and the number of runes consumed,
// including the closing delimiter if one was found.
func CompileDelim(rs []rune) (re Regexp, n int, err error) {
	re, n, err = compile(rs[1:], rs[0])
	if err != nil {
		return Regexp{}, 0, err
	}
	n++ // opening delimiter
	if n < len(rs) {
		if rs[n] != rs[0] {
			panic("stopped before closing delimiter")
		}
		n++
	}
	return re, n, err
}

// Compile compiles the string into a regular expression.
func Compile(rs []rune) (Regexp, error) {
	re, _, err := compile(rs, -1)
	return re, err
}

func compile(rs []rune, delim rune) (re Regexp, n int, err error) {
	defer func() {
		switch e := recover().(type) {
		case nil:
			return
		case ParseError:
			re, n, err = Regexp{}, 0, e
		default:
			panic(e)
		}
	}()
	p := parser{rs: rs, sub: 1, delim: delim}
	e := e0(&p)
	if e == nil {
		e = &Regexp{}
	}
	switch p.next() {
	case cparen:
		panic(ParseError{Position: p.pos - 1, Message: "unmatched ')'"})
	case cbrace:
		panic(ParseError{Position: p.pos - 1, Message: "unmatched ']'"})
	}
	return Regexp{}, p.pos, nil
}

func e0(p *parser) *Regexp {
	l := e1(p)
	if l == nil || p.peek() != or {
		return l
	}
	p.next()
	if p.eof() {
		panic(ParseError{Position: p.pos - 1, Message: "'|' has no right hand side"})
	}
	_ = e0(p)
	return &Regexp{}
}

func e1(p *parser) *Regexp {
	l := e2(p)
	if l == nil || p.eof() {
		return l
	}
	r := e1(p)
	if r == nil {
		return l
	}
	return &Regexp{}
}

func e2(p *parser) *Regexp {
	l := e3(p)
	if p.eof() || l == nil {
		return l
	}
	return e2p(*l, p)
}

func e2p(l Regexp, p *parser) *Regexp {
	switch p.next() {
	case star:
		return e2p(Regexp{}, p)
	case plus:
		return e2p(Regexp{}, p)
	case question:
		return e2p(Regexp{}, p)
	case eof:
		return &l
	default:
		p.back()
		return &l
	}
}

func e3(p *parser) *Regexp {
	switch t := p.next(); t {
	case oparen:
		o := p.pos - 1
		p.sub++
		if p.peek() == cparen {
			panic(ParseError{Position: o, Message: "missing operand for '('"})
		}
		_ = e0(p)
		if t = p.next(); t != cparen {
			panic(ParseError{Position: o, Message: "got " + t.String() + " wanted ')'"})
		}
		return &Regexp{}

	case obrace:
		panic("unimplemented")

	case dot:
	case carrot:
	case dollar:
	default:
		if t < 0 {
			p.back()
			return nil
		}
	}
	return &Regexp{}
}
