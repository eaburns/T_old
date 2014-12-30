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
A literal is any non-metacharacter, or a metacharacter (one of .*+?[]()|\^$) or the delimiter or the letter n preceded by \. An exception is made if the delimiter is a metacharacter; in that case, when preceeded by \ it is interpreted as its meta form. A literal delimiter can always be matched using a charclass (see below). \n is a literal newline.

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
	"strconv"
	"sync"
)

// nCache is the maximum number of machines to cache.
const nCache = 2

// A Regexp is the compiled form of a regular expression.
type Regexp struct {
	// Expr is the expression that compiled into this Regexp.
	expr       []rune
	start, end *node
	// N is the number of states in the expression.
	n int
	// Nsub is the number of subexpressions,
	// counting the 0th, which is the entire expression.
	nsub int

	lock   sync.Mutex
	mcache []*machine
}

// Expression returns the input expression
// that was compiled into this Regexp.
func (re *Regexp) Expression() []rune { return re.expr }

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

	p := parser{rs: rs, nsub: 1, reverse: opts.Reverse, literal: opts.Literal}
	var n int
	if opts.Delimited {
		p.delim = p.rs[0]
		p.pos = 1
	}

	re = e0(&p)
	n += p.pos
	if re == nil {
		re = &Regexp{start: new(node), end: new(node)}
		re.start.out[0].to = re.end
	}
	re = subexpr(re, 0)
	re.nsub = p.nsub

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
	eof rune  = -1
	or  token = -2 - iota
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

type parser struct {
	rs               []rune
	prev, pos        int
	nsub             int
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
		return token(eof)
	}
	t := p.next()
	p.back()
	return t
}

func (p *parser) next() (t token) {
	if p.eof() {
		return token(eof)
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
	if l.end.sub == 0 {
		// Common case: if possible, re-use l's end node.
		*l.end = *r.start
	} else {
		l.end.out[0].to = r.start
	}
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
	case token(eof):
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
			panic(ParseError{Position: o, Message: "unclosed ')'"})
		}
		re = subexpr(e, p.nsub)
		p.nsub++
	case t == obrace:
		re.start.out[0].label = charClass(p)
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

func subexpr(e *Regexp, n int) *Regexp {
	re := &Regexp{start: new(node), end: new(node)}
	re.start.out[0].to = e.start
	e.end.out[0].to = re.end
	re.start.sub = n + 1
	re.end.sub = -n - 1
	return re
}

func charClass(p *parser) label {
	var c classLabel
	p0 := p.pos - 1
	if p.pos < len(p.rs) && p.rs[p.pos] == '^' {
		c.neg = true
		p.pos++
	}
	for {
		r := eof
		if p.pos < len(p.rs) {
			r = p.rs[p.pos]
			p.pos++
		}
		switch {
		case r == ']':
			if len(c.runes) == 0 && len(c.ranges) == 0 {
				panic(ParseError{Position: p0, Message: "missing operand for '['"})
			}
			if c.neg {
				c.runes = append(c.runes, '\n')
			}
			return &c
		case r == eof:
			panic(ParseError{Position: p0, Message: "unclosed ]"})
		case r == '-':
			panic(ParseError{Position: p.pos - 1, Message: "malformed []"})
		case r == '\\' && p.pos < len(p.rs):
			r = p.rs[p.pos]
			p.pos++
		}
		if p.pos >= len(p.rs) || p.rs[p.pos] != '-' {
			c.runes = append(c.runes, r)
			continue
		}
		p.pos++
		if p.pos >= len(p.rs) {
			panic(ParseError{Position: p.pos - 1, Message: "range incomplete"})
		}
		u := p.rs[p.pos]
		if u <= r {
			panic(ParseError{Position: p.pos, Message: "range not ascending"})
		}
		p.pos++
		c.ranges = append(c.ranges, [2]rune{r, u})
	}
}

// Runes generalizes a slice or array of runes.
type Runes interface {
	Rune(int64) rune
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
	matches := m.match(rs, rs.Size())
	if matches == nil {
		m.init(0)
		matches = m.match(rs, from)
	}
	return matches
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
	prev := eof
	if p := m.at - 1; p >= 0 && p < sz {
		prev = rs.Rune(p)
	}
	cur := eof
	if m.at >= 0 && m.at < sz {
		cur = rs.Rune(m.at)
	}
	for {
		for m.q0.empty() && m.lit != nil && !m.lit.ok(prev, cur) && m.at <= end {
			m.at++
			prev, cur = cur, eof
			if m.at >= 0 && m.at < sz {
				cur = rs.Rune(m.at)
			}
		}
		if m.cap == nil && !m.q0.mem[m.re.start.n] && m.at <= end {
			m.q0.push(m.get(m.re.start))
		}
		if m.q0.empty() {
			break
		}
		for !m.q0.empty() {
			s := m.q0.pop()
			m.step(s, prev, cur)
		}
		m.at++
		prev, cur = cur, eof
		if m.at >= 0 && m.at < sz {
			cur = rs.Rune(m.at)
		}
		m.q0, m.q1 = m.q1, m.q0
	}
	return m.cap
}

func (m *machine) step(s0 *state, prev, cur rune) {
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
				if !seen[e.to.n] && (e.label == nil || e.label.ok(prev, cur)) {
					seen[e.to.n] = true
					t := m.get(e.to)
					copy(t.cap, s.cap)
					stk = append(stk, t)
				}
			case !m.q1.mem[e.to.n] && e.label.ok(prev, cur):
				t := m.get(e.to)
				copy(t.cap, s.cap)
				m.q1.push(t)
			}
		}
		m.put(s)
	}
}
