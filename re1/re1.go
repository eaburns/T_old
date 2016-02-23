// Copyright © 2015, The T Authors.

/*
Package re1 is an extended variant of Plan 9 regular expressions.
Plan 9 regular expressions are defined in the regexp(7) man page,
which can be found at http://swtch.com/plan9port/man/man7/regexp.html.
The re1 variant has minor modifications
and includes some extensions common to
the Go standard regexp package, re2, Perl, etc.
The following text is modified from regexp(7) to describe the re1 variant.

A regular expression specifies a set of strings of characters.
A member of this set of strings is said to be matched by the regular expression.
In many applications a delimiter character, commonly /, bounds a regular expression.
In the following specification for regular expressions the word "character"
means any unicode character but newline.

Language

The syntax for a regular expression e0 is
	e3:    literal | charclass | '.' | '^' | '$' | '\' PERL | '(' e0 ')'
	PERL: 'd' | 'D' | 's' | 'S' | 'w' | 'W' | 'A' | 'z'
	e2:    e3 | e2 REP
	REP:   '*' | '+' | '?'
	e1:    e2 | e1 e2
	e0:    e1 | e0 '|' e1

A literal is any non-metacharacter that is not preceded by \;
a metacharacter (one of .*+?[]()|\^$) preceded by \;
the delimiter preceded by \;
the letter n preceded by \;
or any non-PERL-character-class character preceded by a \.
However, if the delimiter is a metacharacter,
when preceeded by \ it is interpreted as its meta form,
not its literal form.
A literal delimiter can always be matched using a charclass (see below).
\n is a literal newline.
A non-PERL-character-class character preceded by a \
is the character itself, without the \.


A charclass is a nonempty string s bracketed [s] (or [^s]);
it matches any character in (or not in) s.
A negated character class never matches newline.
A substring a−b, with a and b in ascending order,
stands for the inclusive range of characters between a and b.
In s, the metacharacters −, ], and an initial ^ must be preceded by a \;
other metacharacters including the regular expression delimiter
have no special meaning and may appear unescaped.

A . matches any character. It does not match newline.

A ^ matches the beginning of a line; $ matches the end of the line.

The PERL-style character classes are one of
d, D, s, S, w, W, b, B, A, or z, preceeded by a \.
	\d matches a unicode digit character.
	\D matches a unicode non-digit character.
	\s matches a unicode whitespace character.
	\S matches a unicode non-whitespace character.
	\w matches a "word character" (a unicode digit, letter, or _).
	\W matches any character but a word character.
	\b matches a "word boundary" (\w on one side and \W, \A, or \z on the other).
	\B matches a non word boundary.
	\A matches the beginning of the text.
	\z matches the end of the text.

The REP operators match
zero or more (*),
one or more (+),
zero or one (?),
instances respectively of the preceding regular expression e2.

A parenthesized subexpression, (e0), is a capturing group.
Caputring groups are numbered from 1 in the order of their (, left-to-right.
The indices of the e0 match within the outter expression are "captured"
and returned by the Regexp.Match method according to their number.

A concatenated regular expression, e1e2,
matches a match to e1 followed by a match to e2.

An alternative regular expression, e0|e1,
matches either a match to e0 or a match to e1.

Matches

Regular expression matches are left-most, longest matches.

Subexpression matches are right-most, longest matches
within the match of their containing expression.
This means, in the case of nested subexpressions,
an inner expression match is always within its outter expression match.
*/
package re1

import (
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// None indicates beginning or end of input text.
const None = -1

const (
	// Meta contains the re1 metacharacters.
	Meta = `.*+?[]()|\^$`
	// Perl are characters used in perl-style character classes.
	Perl = "dDsSwWbBAz"
)

// nCache is the maximum number of machines to cache.
const nCache = 2

// A Regexp is the compiled form of a regular expression.
type Regexp struct {
	start, end         *state
	nStates, nSubexprs int

	lock   sync.Mutex
	mcache []*machine
}

type state struct {
	n   int
	out [2]edge
	// sub==0 means no subexpression
	// sub>0 means start of subexpression sub-1
	// sub<0 means end of subexpression -(sub-1)
	subexpr int
}

type edge struct {
	label label
	to    *state
}

func (e *edge) consumes() bool         { return e.label != nil && e.label.consumes() }
func (e *edge) accepts(p, c rune) bool { return e.label == nil || e.label.accepts(p, c) }

type label interface {
	accepts(prev, cur rune) bool
	consumes() bool
}

type funcLabel func(rune) bool

func (f funcLabel) accepts(_, c rune) bool { return f(c) }
func (funcLabel) consumes() bool           { return true }

func isWord(r rune) bool { return unicode.IsDigit(r) || unicode.IsLetter(r) || r == '_' }
func isDot(r rune) bool  { return r >= 0 && r != '\n' }

type not struct{ label }

func (l not) accepts(p, c rune) bool { return c >= 0 && !l.label.accepts(p, c) }
func (l not) consumes() bool         { return l.label.consumes() }

type runeLabel rune

func (l runeLabel) accepts(_, c rune) bool { return c == rune(l) }
func (runeLabel) consumes() bool           { return true }

type classLabel struct {
	negated bool
	runes   []rune
	ranges  [][2]rune
}

func (l *classLabel) accepts(_, c rune) bool {
	if c < 0 {
		return false
	}
	for _, r := range l.runes {
		if c == r {
			return !l.negated
		}
	}
	for _, r := range l.ranges {
		if r[0] <= c && c <= r[1] {
			return !l.negated
		}
	}
	return l.negated
}

func (classLabel) consumes() bool { return true }

type beginLineLabel struct{}

func (beginLineLabel) accepts(p, _ rune) bool { return p == None || p == '\n' }
func (beginLineLabel) consumes() bool         { return false }

type endLineLabel struct{}

func (endLineLabel) accepts(_, c rune) bool { return c == None || c == '\n' }
func (endLineLabel) consumes() bool         { return false }

type beginTextLabel struct{}

func (beginTextLabel) accepts(p, _ rune) bool { return p == None }
func (beginTextLabel) consumes() bool         { return false }

type endTextLabel struct{}

func (endTextLabel) accepts(_, c rune) bool { return c == None }
func (endTextLabel) consumes() bool         { return false }

type wordBoundaryLabel struct{}

func (wordBoundaryLabel) accepts(p, c rune) bool {
	return isWord(p) && (!isWord(c) || c == None) ||
		isWord(c) && (!isWord(p) || p == None)
}

func (wordBoundaryLabel) consumes() bool { return false }

// Flags control the behavior of regular expression compilation.
type Flags int

const (
	// Delimited indicates that the first rune read by the compiler
	// should be interpreted as a delimiter.
	// Compilation will terminate either at the end of input
	// or the first un-escaped delimiter.
	Delimited Flags = 1 << iota
	// Literal indicates that the regular expression
	// should be compiled for a literal match.
	Literal
	// Reverse indicates that the regular expression
	// should be compiled for a reverse match.
	Reverse
)

func (flags Flags) String() string {
	var ss []string
	if flags == 0 {
		return "0"
	}
	if flags&Delimited != 0 {
		flags ^= Delimited
		ss = append(ss, "Delimited")
	}
	if flags&Literal != 0 {
		flags ^= Literal
		ss = append(ss, "Literal")
	}
	if flags&Reverse != 0 {
		flags ^= Reverse
		ss = append(ss, "Reverse")
	}
	if flags != 0 {
		ss = append(ss, "0x"+strconv.FormatInt(int64(flags), 16))
	}
	return strings.Join(ss, "|")
}

// Compile compiles a regular expression.
// The parse is terminated by EOF or an un-escaped closing delimiter.
func Compile(rr io.RuneReader, flags ...Flags) (*Regexp, error) {
	p, err := newParser(rr, flags)
	if err != nil {
		return nil, err
	}
	re, err := e0(p)
	if err != nil {
		return nil, err
	}
	if re == nil {
		re = &Regexp{start: new(state), end: new(state)}
		re.start.out[0].to = re.end
	}
	re = subexpr(re, 0)
	re.nSubexprs = p.nSubexprs
	numberStates(re)
	return re, nil
}

// NumberStates assigns a unique, small interger to each node
// and sets Regexp.n to the number of nodes in the automata.
func numberStates(re *Regexp) {
	var s *state
	stack := []*state{re.start}
	re.nStates++
	for len(stack) > 0 {
		s, stack = stack[len(stack)-1], stack[:len(stack)-1]
		for _, e := range s.out {
			t := e.to
			if t == nil || t == re.start || t.n > 0 {
				continue
			}
			t.n = re.nStates
			re.nStates++
			stack = append(stack, t)
		}
	}
}

type token rune

const (
	dot             token = -'.'
	star            token = -'*'
	plus            token = -'+'
	question        token = -'?'
	obrace          token = -'['
	cbrace          token = -']'
	oparen          token = -'('
	cparen          token = -')'
	or              token = -'|'
	caret           token = -'^'
	dollar          token = -'$'
	digit           token = -'d'
	notDigit        token = -'D'
	space           token = -'s'
	notSpace        token = -'S'
	word            token = -'w'
	notWord         token = -'W'
	wordBoundary    token = -'b'
	notWordBoundary token = -'B'
	beginText       token = -'A'
	endText         token = -'z'
)

type parser struct {
	flags     Flags
	nSubexprs int
	nest      int
	delim     rune
	current   token
	rr        io.RuneReader
}

func newParser(rr io.RuneReader, flags []Flags) (*parser, error) {
	p := parser{nSubexprs: 1, nest: -1, rr: rr}
	for _, f := range flags {
		p.flags |= f
	}
	if p.flags&Delimited != 0 {
		switch r, _, err := rr.ReadRune(); {
		case err == io.EOF:
			break // do nothing.
		case err != nil:
			return nil, err
		case r == '\\':
			return nil, errors.New("bad delimiter")
		default:
			p.delim = r
		}
	}
	return &p, p.next()
}

func (p *parser) read() (rune, error) {
	r, _, err := p.rr.ReadRune()
	return r, err
}

func (p *parser) next() error {
	switch r, err := p.read(); {
	case err == io.EOF:
		p.current = None
	case err != nil:
		return err
	case r == p.delim:
		p.current = None
	case r == '\\':
		switch r1, err := p.read(); {
		case err == io.EOF:
			p.current = '\\'
		case err != nil:
			return err
		case r1 == 'n':
			p.current = '\n'
		case strings.ContainsRune(Perl, r1):
			p.current = token(-r1)
		case r1 == p.delim && strings.ContainsRune(Meta, r1):
			p.current = token(-r1)
		default:
			p.current = token(r1)
		}
	case strings.ContainsRune(Meta, r):
		p.current = token(-r)
	default:
		p.current = token(r)
	}
	return nil
}

func e0(p *parser) (*Regexp, error) {
	p.nest++
	defer func() { p.nest-- }()

	l, err := e1(p)
	if l == nil || err != nil || p.current == None || p.current != or {
		return l, err
	}
	if err := p.next(); err != nil {
		return nil, err
	}
	switch r, err := e0(p); {
	case err != nil:
		return nil, err
	case r == nil:
		return nil, errors.New("missing operand for |")
	default:
		re := &Regexp{start: new(state), end: new(state)}
		re.start.out[0].to = l.start
		re.start.out[1].to = r.start
		l.end.out[0].to = re.end
		r.end.out[0].to = re.end
		return re, nil
	}
}

func e1(p *parser) (*Regexp, error) {
	l, err := e2(p)
	if l == nil || err != nil {
		return l, err
	}
	r, err := e1(p)
	if r == nil || err != nil {
		return l, err
	}
	if p.flags&Reverse != 0 {
		l, r = r, l
	}
	re := &Regexp{start: new(state)}
	re.start = l.start
	if l.end.subexpr == 0 {
		// Common case: if possible, re-use l's end state.
		*l.end = *r.start
	} else {
		l.end.out[0].to = r.start
	}
	re.end = r.end
	return re, nil
}

func e2(p *parser) (*Regexp, error) {
	l, err := e3(p)
	if err != nil || l == nil {
		return l, err
	}
	return e2p(l, p)
}

func e2p(l *Regexp, p *parser) (*Regexp, error) {
	if p.flags&Literal != 0 {
		return l, nil
	}
	re := &Regexp{start: new(state), end: new(state)}
	switch p.current {
	case None:
		return l, nil
	case star:
		if l.start.out[1].to == nil {
			// Common case: if possible, re-use l's start state.
			re.start = l.start
		} else {
			re.start.out[0].to = l.start
		}
		re.start.out[1].to = re.end
		l.end.out[0].to = l.start
		l.end.out[1].to = re.end
	case plus:
		re.start.out[0].to = l.start
		l.end.out[0].to = l.start
		l.end.out[1].to = re.end
	case question:
		re.start.out[0].to = l.start
		re.start.out[1].to = re.end
		l.end.out[0].to = re.end
	default:
		return l, nil
	}
	if err := p.next(); err != nil {
		return nil, err
	}
	return e2p(re, p)
}

func e3(p *parser) (*Regexp, error) {
	re := &Regexp{start: new(state), end: new(state)}
	re.start.out[0].to = re.end
	if p.flags&Literal != 0 {
		if p.current == None {
			return nil, nil
		}
		if p.current < 0 {
			p.current = -p.current
		}
		re.start.out[0].label = runeLabel(p.current)
		if err := p.next(); err != nil {
			return nil, err
		}
		return re, nil
	}
	switch p.current {
	case None:
		return nil, nil
	case oparen:
		if err := p.next(); err != nil {
			return nil, err
		}
		nSubexprs := p.nSubexprs
		p.nSubexprs++
		switch e, err := e0(p); {
		case err != nil:
			return nil, err
		case p.current == None:
			return nil, errors.New("unclosed (")
		case p.current != cparen:
			panic("impossible unclose")
		case e == nil:
			return nil, errors.New("missing operand for (")
		default:
			re = subexpr(e, nSubexprs)
		}
	case cparen:
		if p.nest == 0 {
			return nil, errors.New("unopened )")
		}
		return nil, nil
	case obrace:
		c, err := class(p)
		if err != nil {
			return nil, err
		}
		re.start.out[0].label = c
	case cbrace:
		return nil, errors.New("unopened ]")
	case dot:
		re.start.out[0].label = funcLabel(isDot)
	case digit:
		re.start.out[0].label = funcLabel(unicode.IsDigit)
	case notDigit:
		re.start.out[0].label = not{funcLabel(unicode.IsDigit)}
	case space:
		re.start.out[0].label = funcLabel(unicode.IsSpace)
	case notSpace:
		re.start.out[0].label = not{funcLabel(unicode.IsSpace)}
	case word:
		re.start.out[0].label = funcLabel(isWord)
	case notWord:
		re.start.out[0].label = not{funcLabel(isWord)}
	case wordBoundary:
		re.start.out[0].label = wordBoundaryLabel{}
	case notWordBoundary:
		re.start.out[0].label = not{wordBoundaryLabel{}}
	case caret:
		if p.flags&Reverse == 0 {
			re.start.out[0].label = beginLineLabel{}
		} else {
			re.start.out[0].label = endLineLabel{}
		}
	case dollar:
		if p.flags&Reverse == 0 {
			re.start.out[0].label = endLineLabel{}
		} else {
			re.start.out[0].label = beginLineLabel{}
		}
	case beginText:
		if p.flags&Reverse == 0 {
			re.start.out[0].label = beginTextLabel{}
		} else {
			re.start.out[0].label = endTextLabel{}
		}
	case endText:
		if p.flags&Reverse == 0 {
			re.start.out[0].label = endTextLabel{}
		} else {
			re.start.out[0].label = beginTextLabel{}
		}
	case star:
		return nil, errors.New("missing operand for *")
	case plus:
		return nil, errors.New("missing operand for +")
	case question:
		return nil, errors.New("missing operand for ?")
	default:
		if p.current < 0 { // Any other meta character.
			return nil, nil
		}
		re.start.out[0].label = runeLabel(p.current)
	}
	return re, p.next()
}

func class(p *parser) (label, error) {
	var c classLabel
	r, err := p.read()
	for {
		switch {
		case err == io.EOF:
			return nil, errors.New("unclosed ]")
		case err != nil:
			return nil, err
		case r == ']':
			if len(c.runes) == 0 && len(c.ranges) == 0 {
				return nil, errors.New("missing operand for [")
			}
			return &c, nil
		case r == '\\':
			switch r1, err := p.read(); {
			case err == io.EOF:
				return nil, errors.New("unclosed ]")
			case err != nil:
				return nil, err
			case r1 == p.delim && r1 == ']':
				if len(c.runes) == 0 && len(c.ranges) == 0 {
					return nil, errors.New("missing operand for [")
				}
				return &c, nil
			default:
				r = r1
			}
		case r == '^' && len(c.runes) == 0 && len(c.ranges) == 0:
			c.negated = true
			c.runes = append(c.runes, '\n')
			r, err = p.read()
			continue
		case r == '-':
			return nil, errors.New("range incomplete")
		}

		switch r1, err := p.read(); {
		case err == io.EOF:
			return nil, errors.New("unclosed ]")
		case err != nil:
			return nil, err
		case r1 == '-':
			switch r2, err := p.read(); {
			case err != nil && err != io.EOF:
				return nil, err
			case err == io.EOF || r2 == ']':
				return nil, errors.New("incomplete range")
			case r2 <= r:
				return nil, errors.New("range not ascending")
			default:
				c.ranges = append(c.ranges, [2]rune{r, r2})
				r, err = p.read()
			}
		default:
			c.runes = append(c.runes, r)
			r = r1
		}
	}
}

func subexpr(e *Regexp, n int) *Regexp {
	re := &Regexp{start: new(state), end: new(state)}
	re.start.out[0].to = e.start
	e.end.out[0].to = re.end
	re.start.subexpr = n + 1
	re.end.subexpr = -n - 1
	return re
}

// Match returns the byte indices of the regular expression
// and all subexpression matches.
//
// If the RuneReader only reads a substring of a larger text,
// prev and next give the previous and next rune to those of the RuneReader.
// Passing None for prev indicates no previous rune;
// the RuneReader begins at the beginning of the text.
// Passing None for next indicates no next rune;
// the RuneReader ends at the end of the text.
//
// The regular expression match is the left-most, longest match.
//
// The subexpression matches are right-most, longest matches
// within the match of their containing expression.
// This means, in the case of nested subexpressions,
// an inner expression match is always within its outter expression match.
// For example,
// Regexp "((a*)b)*" matching against the string "abb" gives [[0 3] [2 3] [2 2]].
// Subexpression 1, ((a*)b), matches [2 3].
// The contained subexpression 2, (a*), matches [2 2],
// the empty string at the beginning of the subexpression 1 match.
//
// The size of a match (m[i][1]-m[i][0]) is always non-negative.
func (re *Regexp) Match(prev rune, rr io.RuneReader, next rune) [][2]int64 {
	m := re.get()
	defer re.put(m)
	return m.match(prev, rr, next)
}

func (re *Regexp) get() *machine {
	re.lock.Lock()
	defer re.lock.Unlock()
	if len(re.mcache) == 0 {
		m := &machine{
			re:    re,
			q0:    newQueue(re.nStates),
			q1:    newQueue(re.nStates),
			stack: make([]*node, re.nStates),
			seen:  make([]bool, re.nStates),
			false: make([]bool, re.nStates),
		}
		if s := re.start.out[0].to; s.out[1].to == nil && s.out[0].consumes() {
			m.prefix = s.out[0].label
		}
		return m
	}
	m := re.mcache[0]
	re.mcache = re.mcache[1:]
	m.n = 0
	m.m = nil
	for p := m.q0.head; p != nil; p = p.next {
		m.put(p)
	}
	m.q0.head, m.q0.tail = nil, nil
	for p := m.q1.head; p != nil; p = p.next {
		m.put(p)
	}
	m.q1.head, m.q1.tail = nil, nil
	return m
}

func (re *Regexp) put(m *machine) {
	re.lock.Lock()
	defer re.lock.Unlock()
	if len(re.mcache) < nCache {
		re.mcache = append(re.mcache, m)
	}
}

type machine struct {
	re *Regexp
	n  int64
	m  [][2]int64
	// Prefix is a single-rune literal prefix of the regexp, or nil.
	prefix      label
	q0, q1      *queue
	stack       []*node
	seen, false []bool // false is to zero seen.
	free        *node
}

type node struct {
	state *state
	m     [][2]int64
	next  *node
}

func (m *machine) get(s *state) *node {
	if m.free == nil {
		return &node{state: s, m: make([][2]int64, m.re.nSubexprs)}
	}
	n := m.free
	m.free = m.free.next
	for i := range n.m {
		n.m[i] = [2]int64{}
	}
	n.state = s
	return n
}

func (m *machine) put(s *node) {
	s.next = m.free
	m.free = s
}

type queue struct {
	head, tail *node
	mem        []bool
}

func newQueue(n int) *queue { return &queue{mem: make([]bool, n)} }

func (q *queue) empty() bool { return q.head == nil }

func (q *queue) push(s *node) {
	if q.tail != nil {
		q.tail.next = s
	}
	if q.head == nil {
		q.head = s
	}
	q.tail = s
	s.next = nil
	q.mem[s.state.n] = true
}

func (q *queue) pop() *node {
	s := q.head
	q.head = q.head.next
	if q.head == nil {
		q.tail = nil
	}
	s.next = nil
	q.mem[s.state.n] = false
	return s
}

func (m *machine) match(p rune, rr io.RuneReader, n rune) [][2]int64 {
	var w int
	var c rune
	var err error
	for {
		m.n += int64(w)
		if c, w, err = rr.ReadRune(); err != nil {
			c, w = n, 0
		}
		for m.q0.empty() && m.prefix != nil && !m.prefix.accepts(p, c) && w > 0 {
			p = c
			m.n += int64(w)
			if c, w, err = rr.ReadRune(); err != nil {
				c, w = n, 0
			}
		}
		if m.m == nil && !m.q0.mem[m.re.start.n] {
			m.q0.push(m.get(m.re.start))
		}
		if m.q0.empty() {
			break
		}
		for !m.q0.empty() {
			s := m.q0.pop()
			m.step(s, p, c)
		}
		if w == 0 {
			break
		}
		p = c
		m.q0, m.q1 = m.q1, m.q0
	}
	return m.m
}

func (m *machine) step(s0 *node, p, c rune) {
	stack, seen := m.stack[:1], m.seen
	copy(seen, m.false)
	stack[0], seen[s0.state.n] = s0, true
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		switch subexpr := n.state.subexpr; {
		case subexpr > 0:
			n.m[subexpr-1][0] = m.n
			n.m[subexpr-1][1] = m.n
		case subexpr < 0:
			n.m[-subexpr-1][1] = m.n
		}

		if n.state == m.re.end && (m.m == nil || m.m[0][0] >= n.m[0][0]) {
			if m.m == nil {
				m.m = make([][2]int64, m.re.nSubexprs)
			}
			copy(m.m, n.m)
		}

		for i := range n.state.out {
			switch e := &n.state.out[i]; {
			case e.to == nil:
				continue
			case !e.consumes():
				if !seen[e.to.n] && e.accepts(p, c) {
					seen[e.to.n] = true
					t := m.get(e.to)
					copy(t.m, n.m)
					stack = append(stack, t)
				}
			case !m.q1.mem[e.to.n] && e.accepts(p, c):
				t := m.get(e.to)
				copy(t.m, n.m)
				m.q1.push(t)
			}
		}
		m.put(n)
	}
}
