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

// A Regexp is the compiled form of a regular expression.
type Regexp struct {
	// Expr is the expression that compiled into this Regexp.
	expr       []rune
	start, end *node
	// N is the number of states in the expression.
	n int
}

// Expression returns the input expression
// that was compiled into this Regexp.
func (re *Regexp) Expression() []rune { return re.expr }

type node struct {
	n   int
	out [2]edge
}

type edge struct {
	label label
	to    *node
}

func (e *edge) epsilon() bool {
	return e.label == nil || e.label.epsilon()
}

func (e *edge) ok(p, c rune) bool {
	return e.label != nil && e.label.ok(p, c)
}

func (e *edge) String() string {
	var l string
	if e.label == nil {
		l = "ε"
	} else {
		l = e.label.String()
	}
	var t string
	if e.to == nil {
		t = "<nil>"
	} else {
		t = strconv.Itoa(e.to.n)
	}
	return "{ label: " + l + ", to: " + t + " }"
}

type label interface {
	ok(prev, cur rune) bool
	epsilon() bool
	String() string
}

type dotLabel struct{}

func (dotLabel) ok(_, c rune) bool { return c != '\n' }
func (dotLabel) epsilon() bool     { return false }
func (dotLabel) String() string    { return "." }

type runeLabel rune

func (r runeLabel) ok(_, c rune) bool { return c == rune(r) }
func (r runeLabel) epsilon() bool     { return false }
func (r runeLabel) String() string    { return string([]rune{rune(r)}) }

type bolLabel struct{}

func (r bolLabel) ok(p, _ rune) bool { return p == rune(eof) || p == '\n' }
func (r bolLabel) epsilon() bool     { return true }
func (r bolLabel) String() string    { return "<bol>" }

type eolLabel struct{}

func (r eolLabel) ok(_, c rune) bool { return c == rune(eof) || c == '\n' }
func (r eolLabel) epsilon() bool     { return true }
func (r eolLabel) String() string    { return "<eol>" }

// A ParseError records an error encountered while parsing a regular expression.
type ParseError struct {
	Position int
	Message  string
}

func (e ParseError) Error() string { return strconv.Itoa(e.Position) + ": " + e.Message }

// Options are compile-time options for regular expressions.
type Options struct {
	// Delimited states whether the first character
	// in the string should be interpreted as a delimiter.
	Delimited bool
	// Reverse states whether the regular expression
	// should be compiled for reverse match.
	Reverse bool
	// Literal states whether metacharacters should be interpreted as literals.
	Literal bool
}

// Compile compiles a regular expression using the options.
// The regular expression is parsed until either
// the end of the input or an un-escaped closing delimiter.
func Compile(rs []rune, opts Options) (re *Regexp, err error) {
	defer func() {
		switch e := recover().(type) {
		case nil:
			return
		case ParseError:
			re, err = nil, e
		default:
			panic(e)
		}
	}()

	p := parser{rs: rs, reverse: opts.Reverse, literal: opts.Literal}
	var n int
	if opts.Delimited {
		p.delim = p.rs[0]
		p.rs = p.rs[1:]
		n++
	}

	re = e0(&p)
	n += p.pos
	if re == nil {
		re = &Regexp{start: new(node), end: new(node)}
		re.start.out[0].to = re.end
	}

	switch p.peek() {
	case cparen:
		panic(ParseError{Position: p.pos, Message: "unmatched ')'"})
	case cbrace:
		panic(ParseError{Position: p.pos, Message: "unmatched ']'"})
	}

	if opts.Delimited && n < len(rs) {
		if rs[n] != rs[0] {
			panic("stopped before closing delimiter")
		}
		n++
	}
	re.expr = rs[:n]
	numberStates(re)
	return re, nil
}

// NumberStates assigns a unique, small interger to each state
// and sets Regexp.n to the number of states in the automata.
func numberStates(re *Regexp) {
	var s *node
	stk := []*node{re.start}
	re.n++
	for len(stk) > 0 {
		s, stk = stk[len(stk)-1], stk[:len(stk)-1]
		for _, e := range s.out {
			t := e.to
			if t == nil || t == re.start || t.n > 0 {
				continue
			}
			t.n = re.n
			re.n++
			stk = append(stk, t)
		}
	}
}

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
	rs               []rune
	prev, pos        int
	delim            rune // -1 for no delimiter.
	reverse, literal bool
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
	r := p.rs[p.pos-1]
	if p.literal {
		return token(r)
	}
	switch r {
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

func e0(p *parser) *Regexp {
	l := e1(p)
	if l == nil || p.peek() != or {
		return l
	}
	p.next()
	if p.eof() {
		panic(ParseError{Position: p.pos - 1, Message: "'|' has no right hand side"})
	}
	r := e0(p)
	re := &Regexp{start: new(node), end: new(node)}
	re.start.out[0].to = l.start
	re.start.out[1].to = r.start
	l.end.out[0].to = re.end
	r.end.out[0].to = re.end
	return re
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
	if p.reverse {
		l, r = r, l
	}
	re := &Regexp{start: new(node)}
	re.start = l.start
	l.end.out[0].to = r.start
	re.end = r.end
	return re
}

func e2(p *parser) *Regexp {
	l := e3(p)
	if p.eof() || l == nil {
		return l
	}
	return e2p(l, p)
}

func e2p(l *Regexp, p *parser) *Regexp {
	re := &Regexp{start: new(node), end: new(node)}
	switch p.next() {
	case star:
		re.start.out[1].to = l.end
		fallthrough
	case plus:
		re.start.out[0].to = l.start
		l.end.out[0].to = l.start
		l.end.out[1].to = re.end
		break
	case question:
		re.start.out[0].to = l.start
		re.start.out[1].to = l.end
		re.end = l.end
		break
	case eof:
		return l
	default:
		p.back()
		return l
	}
	return e2p(re, p)
}

func e3(p *parser) *Regexp {
	re := &Regexp{start: new(node), end: new(node)}
	re.start.out[0].to = re.end

	switch t := p.next(); {
	case t == oparen:
		o := p.pos - 1
		if p.peek() == cparen {
			panic(ParseError{Position: o, Message: "missing operand for '('"})
		}
		e := e0(p)
		if t = p.next(); t != cparen {
			panic(ParseError{Position: o, Message: "got " + t.String() + " wanted ')'"})
		}
		re.start.out[0].to = e.start
		e.end.out[0].to = re.end
	case t == obrace:
		panic("unimplemented")
	case t == dot:
		re.start.out[0].label = dotLabel{}
	case t == carrot && !p.reverse || t == dollar && p.reverse:
		re.start.out[0].label = bolLabel{}
	case t == carrot && p.reverse || t == dollar && !p.reverse:
		re.start.out[0].label = eolLabel{}
	default:
		if t < 0 {
			p.back()
			return nil
		}
		re.start.out[0].label = runeLabel(t)
	}
	return re
}
