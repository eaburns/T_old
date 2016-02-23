// Copyright © 2015, The T Authors.

/*
Package re1 is an implementation of a variant of Plan 9 regular expressions.
Plan 9 regular expressions are defined in the regexp(7) man page,
which can be found at http://swtch.com/plan9port/man/man7/regexp.html.
The following text is from regexp(7), modified to describe the re1 variant.

A regular expression specifies a set of strings of characters. A member of this set of strings is said to be matched by the regular expression. In many applications a delimiter character, commonly /, bounds a regular expression. In the following specification for regular expressions the word ‘character’ means any character (rune) but newline.

The syntax for a regular expression e0 is
	e3:    literal | charclass | '.' | '^' | '$' | '(' e0 ')'
	e2:    e3 | e2 REP
	REP:   '*' | '+' | '?'
	e1:    e2 | e1 e2
	e0:    e1 | e0 '|' e1
A literal is any non-metacharacter, or a metacharacter (one of .*+?[]()|\^$) or the delimiter or the letter n preceded by \. A literal delimiter can always be matched using a charclass (see below). \n is a literal newline.

A charclass is a nonempty string s bracketed [s] (or [^s]); it matches any character in (or not in) s. A negated character class never matches newline. A substring a−b, with a and b in ascending order, stands for the inclusive range of characters between a and b. In s, the metacharacters −, ], and an initial ^ must be preceded by a \; other metacharacters including the regular expression delimiter have no special meaning and may appear unescaped.

A . matches any character.

A ^ matches the beginning of a line; $ matches the end of the line.

The REP operators match zero or more (*), one or more (+), zero or one (?), instances respectively of the preceding regular expression e2.

A concatenated regular expression, e1e2, matches a match to e1 followed by a match to e2.

An alternative regular expression, e0|e1, matches either a match to e0 or a match to e1.

A match to any part of a regular expression extends as far as possible without preventing a match to the remainder of the regular expression.
*/
package re1

import (
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
)

// None indicates beginning or end of input text.
const None = -1

// Meta contains the re1 metacharacters.
const Meta = `.*+?[]()|\^$`

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

type dotLabel struct{}

func (dotLabel) accepts(_, c rune) bool { return c != '\n' && c != None }
func (dotLabel) consumes() bool         { return true }

type runeLabel rune

func (l runeLabel) accepts(_, c rune) bool { return c == rune(l) }
func (runeLabel) consumes() bool           { return true }

type bolLabel struct{}

func (bolLabel) accepts(p, _ rune) bool { return p == None || p == '\n' }
func (bolLabel) consumes() bool         { return false }

type eolLabel struct{}

func (eolLabel) accepts(_, c rune) bool { return c == None || c == '\n' }
func (eolLabel) consumes() bool         { return false }

type classLabel struct {
	runes  []rune
	ranges [][2]rune
	neg    bool
}

func (l *classLabel) accepts(_, c rune) bool {
	if c == None {
		return false
	}
	for _, r := range l.runes {
		if c == r {
			return !l.neg
		}
	}
	for _, r := range l.ranges {
		if r[0] <= c && c <= r[1] {
			return !l.neg
		}
	}
	return l.neg
}

func (classLabel) consumes() bool { return true }

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
	dot      token = -'.'
	star     token = -'*'
	plus     token = -'+'
	question token = -'?'
	obrace   token = -'['
	cbrace   token = -']'
	oparen   token = -'('
	cparen   token = -')'
	or       token = -'|'
	caret    token = -'^'
	dollar   token = -'$'
)

func (t token) String() string {
	if t == None {
		return "EOF"
	}
	if t < 0 {
		t = -t
	}
	return string([]rune{'\'', rune(t), '\''})
}

type parser struct {
	flags     Flags
	nSubexprs int
	delim     rune
	current   token
	rr        io.RuneReader
}

func newParser(rr io.RuneReader, flags []Flags) (*parser, error) {
	p := parser{nSubexprs: 1, rr: rr}
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
	case obrace:
		c, err := class(p)
		if err != nil {
			return nil, err
		}
		re.start.out[0].label = c
	case dot:
		re.start.out[0].label = dotLabel{}
	case caret:
		if p.flags&Reverse == 0 {
			re.start.out[0].label = bolLabel{}
		} else {
			re.start.out[0].label = eolLabel{}
		}
	case dollar:
		if p.flags&Reverse == 0 {
			re.start.out[0].label = eolLabel{}
		} else {
			re.start.out[0].label = bolLabel{}
		}
	case star, plus, question:
		return nil, errors.New("missing operand for " + p.current.String())
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
			c.neg = true
			c.runes = append(c.runes, '\n')
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
// The subexpression matches are the right-most, longest matches
// within the match of their containing expression.
// This means, in the case of nested subexpressions,
// an inner expression match is always within its outter expression match.
// For example,
// 	CompileString("((a*)b)*").MatchString("abb")=[[0 3] [2 3] [2 2]]
// 	// Subexpression 1, ((a*)b), matches [2 3].
// 	// The contained subexpression 2, (a*), matches [2 2],
// 	// the empty string at the beginning of the subexpression 1 match.
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
