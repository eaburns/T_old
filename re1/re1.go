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
	"strings"
	"sync"
)

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

func (dotLabel) accepts(_, c rune) bool { return c != '\n' && c != eof }
func (dotLabel) consumes() bool         { return true }

type runeLabel rune

func (l runeLabel) accepts(_, c rune) bool { return c == rune(l) }
func (runeLabel) consumes() bool           { return true }

type bolLabel struct{}

func (bolLabel) accepts(p, _ rune) bool { return p == eof || p == '\n' }
func (bolLabel) consumes() bool         { return false }

type eolLabel struct{}

func (eolLabel) accepts(_, c rune) bool { return c == eof || c == '\n' }
func (eolLabel) consumes() bool         { return false }

type classLabel struct {
	runes  []rune
	ranges [][2]rune
	neg    bool
}

func (l *classLabel) accepts(_, c rune) bool {
	if c == eof {
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

// Options are compile-time options for regular expressions.
type Options struct {
	// Delimited nodes whether the first character
	// in the string should be interpreted as a delimiter.
	Delimited bool
	// Reverse nodes whether the regular expression
	// should be compiled for reverse match.
	Reverse bool
	// Literal nodes whether metacharacters should be interpreted as literals.
	Literal bool
}

// Compile compiles a regular expression using the options.
// The regular expression is parsed until either
// the end of the input or an un-escaped closing delimiter.
func Compile(rr io.RuneReader, opts Options) (*Regexp, error) {
	p, err := newParser(rr, opts)
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
	eof            = -1
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
	if t == eof {
		return "EOF"
	}
	if t < 0 {
		t = -t
	}
	return string([]rune{'\'', rune(t), '\''})
}

type parser struct {
	Options
	nSubexprs int
	delim     rune
	current   token
	rr        io.RuneReader
}

func newParser(rr io.RuneReader, opts Options) (*parser, error) {
	p := parser{nSubexprs: 1, Options: opts, rr: rr}
	if opts.Delimited {
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
		p.current = eof
	case err != nil:
		return err
	case r == p.delim:
		p.current = eof
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
	if l == nil || err != nil || p.current == eof || p.current != or {
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
	if p.Reverse {
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
	if p.Literal {
		return l, nil
	}
	re := &Regexp{start: new(state), end: new(state)}
	switch p.current {
	case eof:
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
		re.start.out[1].to = l.end
		re.end = l.end
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
	if p.Literal {
		if p.current == eof {
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
	case eof:
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
		case p.current == eof:
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
		if !p.Reverse {
			re.start.out[0].label = bolLabel{}
		} else {
			re.start.out[0].label = eolLabel{}
		}
	case dollar:
		if !p.Reverse {
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
			case err == io.EOF:
				return nil, errors.New("incomplete range")
			case err != nil:
				return nil, err
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

// Runes generalizes a slice or array of runes.
type Runes interface {
	// Rune returns the rune at a given index.
	// If the index is out of bounds, i.e. < 0 or ≥ Size(), Rune panics.
	Rune(int64) rune
	// Size returns the number of runes.
	Size() int64
}

// Match returns the left-most longest match beginning at from
// and wrapping around if no match is found going forward.
//
// The return value is nil if the expression did not match anything.
// Otherwise, the return value has as entry for each subexpression,
// with the entry for subexpression 0 being the entire regular expression.
// The 0th element of a subexpression entry is the inclusive start offset
// of the subexpression match and the 1st entry is the exclusive end offset.
// If the interval is empty then the subexpression did not match.
//
// The empty regular expression returns non-nil with an empty interval
// for subexpression 0.
func (re *Regexp) Match(rs Runes, from int64) [][2]int64 {
	m := re.get()
	defer re.put(m)
	m.init(from)
	ms := m.match(rs, rs.Size())
	if ms == nil {
		m.init(0)
		ms = m.match(rs, from)
	}
	return ms
}

func (re *Regexp) get() *machine {
	re.lock.Lock()
	defer re.lock.Unlock()
	if len(re.mcache) == 0 {
		return newMachine(re)
	}
	m := re.mcache[0]
	re.mcache = re.mcache[1:]
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

func newMachine(re *Regexp) *machine {
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

func (m *machine) init(from int64) {
	m.n = from
	m.m = nil
	for p := m.q0.head; p != nil; p = p.next {
		m.put(p)
	}
	m.q0.head, m.q0.tail = nil, nil
	for p := m.q1.head; p != nil; p = p.next {
		m.put(p)
	}
	m.q1.head, m.q1.tail = nil, nil
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

func (m *machine) match(rs Runes, end int64) [][2]int64 {
	sz := rs.Size()
	p, c := runeOrEOF(rs, sz, m.n-1), runeOrEOF(rs, sz, m.n)
	for {
		for m.q0.empty() && m.prefix != nil && !m.prefix.accepts(p, c) && m.n <= end {
			m.n++
			p, c = c, runeOrEOF(rs, sz, m.n)
		}

		if m.m == nil && !m.q0.mem[m.re.start.n] && m.n <= end {
			m.q0.push(m.get(m.re.start))
		}
		if m.q0.empty() {
			break
		}
		for !m.q0.empty() {
			s := m.q0.pop()
			m.step(s, p, c)
		}
		m.n++
		p, c = c, runeOrEOF(rs, sz, m.n)
		m.q0, m.q1 = m.q1, m.q0
	}
	return m.m
}

func runeOrEOF(rs Runes, sz, i int64) rune {
	if i < 0 || i >= sz {
		return eof
	}
	return rs.Rune(i)
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
