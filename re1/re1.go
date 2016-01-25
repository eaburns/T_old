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
	start, end *node
	// N is the number of states in the expression.
	n int
	// Nsub is the number of subexpressions,
	// counting the 0th, which is the entire expression.
	nsub int

	lock   sync.Mutex
	mcache []*machine
}

type node struct {
	n   int
	out [2]edge
	// sub==0 means no subexpression
	// sub>0 means start of subexpression sub-1
	// sub<0 means end of subexpression -(sub-1)
	sub int
}

type edge struct {
	label label
	to    *node
}

type label interface {
	ok(prev, cur rune) bool
	epsilon() bool
}

type dotLabel struct{}

func (dotLabel) ok(_, c rune) bool { return c != '\n' && c != eof }
func (dotLabel) epsilon() bool     { return false }

type runeLabel rune

func (l runeLabel) ok(_, c rune) bool { return c == rune(l) }
func (runeLabel) epsilon() bool       { return false }

type bolLabel struct{}

func (bolLabel) ok(p, _ rune) bool { return p == eof || p == '\n' }
func (bolLabel) epsilon() bool     { return true }

type eolLabel struct{}

func (eolLabel) ok(_, c rune) bool { return c == eof || c == '\n' }
func (eolLabel) epsilon() bool     { return true }

type classLabel struct {
	runes  []rune
	ranges [][2]rune
	neg    bool
}

func (l *classLabel) ok(_, c rune) bool {
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

func (classLabel) epsilon() bool { return false }

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
func Compile(rs io.RuneScanner, opts Options) (*Regexp, error) {
	p, err := newParser(rs, opts)
	if err != nil {
		return nil, err
	}
	re, err := e0(p)
	if err != nil {
		return nil, err
	}
	if re == nil {
		re = &Regexp{start: new(node), end: new(node)}
		re.start.out[0].to = re.end
	}
	re = subexpr(re, 0)
	re.nsub = p.nsub
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
	nsub    int
	delim   rune
	current token
	scanner io.RuneScanner
}

func newParser(rs io.RuneScanner, opts Options) (*parser, error) {
	p := parser{nsub: 1, Options: opts, scanner: rs}
	if opts.Delimited {
		switch r, _, err := rs.ReadRune(); {
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
	r, _, err := p.scanner.ReadRune()
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
		re := &Regexp{start: new(node), end: new(node)}
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
	re := &Regexp{start: new(node)}
	re.start = l.start
	if l.end.sub == 0 {
		// Common case: if possible, re-use l's end node.
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
	re := &Regexp{start: new(node), end: new(node)}
	switch p.current {
	case eof:
		return l, nil
	case star:
		if l.start.out[1].to == nil {
			// Common case: if possible, re-use l's start node.
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
	re := &Regexp{start: new(node), end: new(node)}
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
		nsub := p.nsub
		p.nsub++
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
			re = subexpr(e, nsub)
		}
	case obrace:
		c, err := charClass(p)
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

func charClass(p *parser) (label, error) {
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
	re := &Regexp{start: new(node), end: new(node)}
	re.start.out[0].to = e.start
	e.end.out[0].to = re.end
	re.start.sub = n + 1
	re.end.sub = -n - 1
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
	re          *Regexp
	at          int64
	cap         [][2]int64
	lit         label
	q0, q1      *queue
	stack       []*state
	seen, false []bool // false is to zero seen.
	free        *state
}

type state struct {
	node *node
	cap  [][2]int64
	next *state
}

func newMachine(re *Regexp) *machine {
	m := &machine{
		re:    re,
		q0:    newQueue(re.n),
		q1:    newQueue(re.n),
		stack: make([]*state, re.n),
		seen:  make([]bool, re.n),
		false: make([]bool, re.n),
	}
	if s := re.start.out[0].to; s.out[1].to == nil &&
		s.out[0].label != nil && !s.out[0].label.epsilon() {
		m.lit = s.out[0].label
	}
	return m
}

func (m *machine) init(from int64) {
	m.at = from
	m.cap = nil
	for p := m.q0.head; p != nil; p = p.next {
		m.put(p)
	}
	m.q0.head, m.q0.tail = nil, nil
	for p := m.q1.head; p != nil; p = p.next {
		m.put(p)
	}
	m.q1.head, m.q1.tail = nil, nil
}

func (m *machine) get(n *node) (s *state) {
	if m.free == nil {
		return &state{node: n, cap: make([][2]int64, m.re.nsub)}
	}
	s = m.free
	m.free = m.free.next
	for i := range s.cap {
		s.cap[i] = [2]int64{}
	}
	s.node = n
	return s
}

func (m *machine) put(s *state) {
	s.next = m.free
	m.free = s
}

type queue struct {
	head, tail *state
	mem        []bool
}

func newQueue(n int) *queue { return &queue{mem: make([]bool, n)} }

func (q *queue) empty() bool { return q.head == nil }

func (q *queue) push(s *state) {
	if q.tail != nil {
		q.tail.next = s
	}
	if q.head == nil {
		q.head = s
	}
	q.tail = s
	s.next = nil
	q.mem[s.node.n] = true
}

func (q *queue) pop() *state {
	s := q.head
	q.head = q.head.next
	if q.head == nil {
		q.tail = nil
	}
	s.next = nil
	q.mem[s.node.n] = false
	return s
}

func (m *machine) match(rs Runes, end int64) [][2]int64 {
	sz := rs.Size()
	p, c := runeOrEOF(rs, sz, m.at-1), runeOrEOF(rs, sz, m.at)
	for {
		for m.q0.empty() && m.lit != nil && !m.lit.ok(p, c) && m.at <= end {
			m.at++
			p, c = c, runeOrEOF(rs, sz, m.at)
		}

		if m.cap == nil && !m.q0.mem[m.re.start.n] && m.at <= end {
			m.q0.push(m.get(m.re.start))
		}
		if m.q0.empty() {
			break
		}
		for !m.q0.empty() {
			s := m.q0.pop()
			m.step(s, p, c)
		}
		m.at++
		p, c = c, runeOrEOF(rs, sz, m.at)
		m.q0, m.q1 = m.q1, m.q0
	}
	return m.cap
}

func runeOrEOF(rs Runes, sz, i int64) rune {
	if i < 0 || i >= sz {
		return eof
	}
	return rs.Rune(i)
}

func (m *machine) step(s0 *state, p, c rune) {
	stk, seen := m.stack[:1], m.seen
	copy(seen, m.false)
	stk[0], seen[s0.node.n] = s0, true
	for len(stk) > 0 {
		s := stk[len(stk)-1]
		stk = stk[:len(stk)-1]

		switch sub := s.node.sub; {
		case sub > 0:
			s.cap[sub-1][0] = m.at
		case sub < 0:
			s.cap[-sub-1][1] = m.at
		}

		if s.node == m.re.end && (m.cap == nil || m.cap[0][0] >= s.cap[0][0]) {
			if m.cap == nil {
				m.cap = make([][2]int64, m.re.nsub)
			}
			copy(m.cap, s.cap)
		}

		for i := range s.node.out {
			switch e := &s.node.out[i]; {
			case e.to == nil:
				continue
			case e.label == nil || e.label.epsilon():
				if !seen[e.to.n] && (e.label == nil || e.label.ok(p, c)) {
					seen[e.to.n] = true
					t := m.get(e.to)
					copy(t.cap, s.cap)
					stk = append(stk, t)
				}
			case !m.q1.mem[e.to.n] && e.label.ok(p, c):
				t := m.get(e.to)
				copy(t.cap, s.cap)
				m.q1.push(t)
			}
		}
		m.put(s)
	}
}
