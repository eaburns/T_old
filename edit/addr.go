package edit

import (
	"errors"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/eaburns/T/re1"
	"github.com/eaburns/T/runes"
)

// An Address identifies a substring within a buffer.
type Address interface {
	addrFrom(from int64, ed *Editor) (addr, error)
	String() string
	To(Address) Address
	Then(Address) Address
	Plus(SimpleAddress) Address
	Minus(SimpleAddress) Address
	addr(*Editor) (addr, error)
}

// A addr identifies a substring within a buffer
// by its inclusive start offset and its exclusive end offset.
type addr struct{ from, to int64 }

// Size returns the number of runes in
// the string identified by the range.
func (a addr) size() int64 { return a.to - a.from }

// All returns the address of the entire buffer: 0,$.
func All() Address { return Line(0).To(End()) }

type compoundAddr struct {
	op     rune
	a1, a2 Address
}

func (a compoundAddr) To(a2 Address) Address {
	return compoundAddr{op: ',', a1: a, a2: a2}
}

func (a compoundAddr) Then(a2 Address) Address {
	return compoundAddr{op: ';', a1: a, a2: a2}
}

func (a compoundAddr) Plus(a2 SimpleAddress) Address {
	return addAddr{op: '+', a1: a, a2: a2}
}

func (a compoundAddr) Minus(a2 SimpleAddress) Address {
	return addAddr{op: '-', a1: a, a2: a2}
}

func (a compoundAddr) String() string {
	return a.a1.String() + string(a.op) + a.a2.String()
}

func (a compoundAddr) addr(ed *Editor) (addr, error) {
	return a.addrFrom(0, ed)
}

func (a compoundAddr) addrFrom(from int64, ed *Editor) (addr, error) {
	a1, err := a.a1.addrFrom(from, ed)
	if err != nil {
		return addr{}, err
	}
	switch a.op {
	case ',':
		a2, err := a.a2.addrFrom(from, ed)
		if err != nil {
			return addr{}, err
		}
		return addr{from: a1.from, to: a2.to}, nil
	case ';':
		origDot := ed.dot
		ed.dot = a1
		a2, err := a.a2.addrFrom(a1.to, ed)
		if err != nil {
			ed.dot = origDot // Restore dot on error.
			return addr{}, err
		}
		return addr{from: a1.from, to: a2.to}, nil
	default:
		panic("bad compound address")
	}
}

type addAddr struct {
	op rune
	a1 Address
	a2 SimpleAddress
}

func (a addAddr) To(a2 Address) Address {
	return compoundAddr{op: ',', a1: a, a2: a2}
}

func (a addAddr) Then(a2 Address) Address {
	return compoundAddr{op: ';', a1: a, a2: a2}
}

func (a addAddr) Plus(a2 SimpleAddress) Address {
	return addAddr{op: '+', a1: a, a2: a2}
}

func (a addAddr) Minus(a2 SimpleAddress) Address {
	return addAddr{op: '-', a1: a, a2: a2}
}

func (a addAddr) String() string {
	return a.a1.String() + string(a.op) + a.a2.String()
}

func (a addAddr) addr(ed *Editor) (addr, error) {
	return a.addrFrom(0, ed)
}

func (a addAddr) addrFrom(from int64, ed *Editor) (addr, error) {
	a1, err := a.a1.addrFrom(from, ed)
	if err != nil {
		return addr{}, err
	}
	switch a.op {
	case '+':
		return a.a2.addrFrom(a1.to, ed)
	case '-':
		return a.a2.reverse().addrFrom(a1.from, ed)
	default:
		panic("bad additive address")
	}
}

// A SimpleAddress identifies a substring within a buffer.
// SimpleAddresses can be composed to form composite addresses.
type SimpleAddress interface {
	Address
	reverse() SimpleAddress
}

type simpAddrImpl interface {
	addrFrom(from int64, ed *Editor) (addr, error)
	String() string
	reverse() SimpleAddress
}

type simpleAddr struct {
	simpAddrImpl
}

func (a simpleAddr) To(a2 Address) Address {
	return compoundAddr{op: ',', a1: a, a2: a2}
}

func (a simpleAddr) Then(a2 Address) Address {
	return compoundAddr{op: ';', a1: a, a2: a2}
}

func (a simpleAddr) Plus(a2 SimpleAddress) Address {
	return addAddr{op: '+', a1: a, a2: a2}
}

func (a simpleAddr) Minus(a2 SimpleAddress) Address {
	return addAddr{op: '-', a1: a, a2: a2}
}

func (a simpleAddr) addr(ed *Editor) (addr, error) {
	return a.addrFrom(0, ed)
}

type dotAddr struct{}

// Dot returns the address of the Editor's dot.
func Dot() SimpleAddress { return simpleAddr{dotAddr{}} }

func (dotAddr) String() string { return "." }

func (dotAddr) addrFrom(_ int64, ed *Editor) (addr, error) {
	if ed.dot.from < 0 || ed.dot.to > ed.runes.Size() {
		return addr{}, errors.New("dot address out of range")
	}
	return ed.dot, nil
}

func (d dotAddr) reverse() SimpleAddress { return simpleAddr{d} }

type endAddr struct{}

// End returns the address of the empty string at the end of the buffer.
func End() SimpleAddress { return simpleAddr{endAddr{}} }

func (endAddr) String() string { return "$" }

func (endAddr) addrFrom(_ int64, ed *Editor) (addr, error) {
	return addr{from: ed.runes.Size(), to: ed.runes.Size()}, nil
}

func (e endAddr) reverse() SimpleAddress { return simpleAddr{e} }

type runeAddr int64

// Rune returns the address of the empty string after rune n.
func Rune(n int64) SimpleAddress { return simpleAddr{runeAddr(n)} }

func (n runeAddr) String() string {
	return "#" + strconv.FormatInt(int64(n), 10)
}

func (n runeAddr) addrFrom(from int64, ed *Editor) (addr, error) {
	m := from + int64(n)
	if m < 0 || m > ed.runes.Size() {
		return addr{}, errors.New("rune address out of range")
	}
	return addr{from: m, to: m}, nil
}

func (n runeAddr) reverse() SimpleAddress { return simpleAddr{runeAddr(-n)} }

type lineAddr struct {
	neg bool
	n   int
}

// Line returns the address of the nth full line.
func Line(n int) SimpleAddress {
	if n < 0 {
		return simpleAddr{lineAddr{neg: true, n: -n}}
	}
	return simpleAddr{lineAddr{n: n}}
}

func (l lineAddr) String() string {
	n := strconv.Itoa(int(l.n))
	if l.neg {
		return "-" + n
	}
	return n
}

func (l lineAddr) addrFrom(from int64, ed *Editor) (addr, error) {
	if l.neg {
		return l.rev(from, ed)
	}
	return l.fwd(from, ed)
}

func (l lineAddr) reverse() SimpleAddress {
	l.neg = !l.neg
	return simpleAddr{l}
}

func (l lineAddr) fwd(from int64, ed *Editor) (addr, error) {
	a := addr{from: from, to: from}
	if a.to > 0 {
		for a.to < ed.runes.Size() && ed.runes.Rune(a.to-1) != '\n' {
			a.to++
		}
		if l.n > 0 {
			a.from = a.to
		}
	}
	for l.n > 0 && a.to < ed.runes.Size() {
		r := ed.runes.Rune(a.to)
		a.to++
		if r == '\n' {
			l.n--
			if l.n > 0 {
				a.from = a.to
			}
		}
	}
	if l.n > 1 || l.n == 1 && a.to < ed.runes.Size() {
		return addr{}, errors.New("line address out of range")
	}
	return a, nil
}

func (l lineAddr) rev(from int64, ed *Editor) (addr, error) {
	a := addr{from: from, to: from}
	if a.from < ed.runes.Size() {
		for a.from > 0 && ed.runes.Rune(a.from-1) != '\n' {
			a.from--
		}
		a.to = a.from
	}
	for l.n > 0 && a.from > 0 {
		r := ed.runes.Rune(a.from - 1)
		a.from--
		if r == '\n' {
			l.n--
			a.to = a.from + 1
		} else if a.from == 0 {
			a.to = a.from
		}
	}
	if l.n > 1 {
		return addr{}, errors.New("line address out of range")
	}
	for a.from > 0 && ed.runes.Rune(a.from-1) != '\n' {
		a.from--
	}
	return a, nil
}

// ErrNoMatch is returned when a regular expression address fails to match.
var ErrNoMatch = errors.New("no match")

type reAddr struct {
	rev bool
	re  string
}

// Regexp returns a regular expression address.
// The regular expression must be a delimited regular expression
// using the syntax of the re1 package:
// http://godoc.org/github.com/eaburns/T/re1.
// If the delimiter is a ? then the regular expression is matched in reverse.
// The regular expression is not compiled until the address is computed
// on a buffer, so compilation errors will not be returned until that time.
func Regexp(re string) SimpleAddress {
	if len(re) == 0 {
		re = "/"
	}
	return simpleAddr{reAddr{rev: re[0] == '?', re: withTrailingDelim(re)}}
}

func withTrailingDelim(re string) string {
	var esc bool
	var rs []rune
	d, _ := utf8.DecodeRuneInString(re)
	for i, ru := range re {
		rs = append(rs, ru)
		// Ensure an unescaped trailing delimiter.
		if i == len(re)-utf8.RuneLen(ru) && (i == 0 || ru != d || esc) {
			rs = append(rs, d)
		}
		esc = !esc && ru == '\\'
	}
	return string(rs)
}

func (r reAddr) String() string { return r.re }

type reverse struct{ *runes.Buffer }

func (r reverse) Rune(i int64) rune {
	return r.Buffer.Rune(r.Buffer.Size() - i - 1)
}

func (r reAddr) addrFrom(from int64, ed *Editor) (a addr, err error) {
	re, err := re1.Compile([]rune(r.re), re1.Options{Delimited: true, Reverse: r.rev})
	if err != nil {
		return a, err
	}
	rs := re1.Runes(ed.runes)
	if r.rev {
		rs = reverse{ed.runes}
		from = rs.Size() - from
	}
	defer runes.RecoverRuneReadError(&err)
	match := re.Match(rs, from)
	if match == nil {
		return a, ErrNoMatch
	}
	a = addr{from: match[0][0], to: match[0][1]}
	if r.rev {
		a.from, a.to = rs.Size()-a.to, rs.Size()-a.from
	}
	return a, nil

}

func (r reAddr) reverse() SimpleAddress {
	r.rev = !r.rev
	return simpleAddr{r}
}

const (
	digits        = "0123456789"
	simpleFirst   = "#/?$." + digits
	additiveFirst = "+-" + simpleFirst
)

// Addr returns an Address parsed from a string.
//
// The address syntax for address a0 is:
//	a0:	{a0} ',' {a0} | {a0} ';' {a0} | {a0} '+' {a1} | {a0} '-' {a1} | a0 a1 | a1
//	a1:	'$' | '.'| '#'{n} | n | '/' regexp {'/'} | '?' regexp {'?'}
//	n:	'0' | '1' | '2' | '3' | '4' | '5' | '6' | '7' | '8' | '9' | n n
//	regexp:	<a valid re1 regular expression>
// All address operators are left-associative.
// The '+' and '-' operators are higher-precedence than ',' and ';'.
//
// Production a1 describes simple addresses:
//	$ is the empty string at the end of the buffer.
//	. is the current address of the editor, called dot.
//	#{n} is the empty string after rune number n. If n is missing then 1 is used.
//	n is the nth line in the buffer. 0 is the string before the first full line.
//	'/' regexp {'/'} is the first match of the regular expression.
//	'?' regexp {'?'} is the first match of the regular expression going in reverse.
//
// Production a0 describes compound addresses:
//	{a0} ',' {a0} is the string from the start of the first address to the end of the second.
//		If the first address is missing, 0 is used.
//		If the second address is missing, $ is used.
//	{a0} ';' {a0} is like the previous,
//		but with the second address evaluated from the end of the first
//		with dot set to the first address.
//		If the first address is missing, 0 is used.
//		If the second address is missing, $ is used.
//	{a0} '+' {a0} is the second address evaluated from the end of the first.
//		If the first address is missing, . is used.
//		If the second address is missing, 1 is used.
//	{a0} '-' {a0} is the second address evaluated in reverse from the start of the first.
//		If the first address is missing, . is used.
//		If the second address is missing, 1 is used.
// If two addresses of the form a0 a1 are present and distinct then a '+' is inserted, as in a0 '+' a1.
func Addr(rs []rune) (Address, int, error) {
	var a1 Address
	var tot int
	for {
		var r rune
		if len(rs) > 0 {
			r = rs[0]
		}
		var n int
		var err error
		switch {
		case unicode.IsSpace(r):
			n++

		case strings.ContainsRune(simpleFirst, r):
			var a2 SimpleAddress
			switch a2, n, err = parseSimpleAddr(rs); {
			case err != nil:
				return nil, n, err
			case a1 != nil:
				a1 = a1.Plus(a2)
			default:
				a1 = a2
			}
		case r == '+' || r == '-':
			n++
			if a1 == nil {
				a1 = Dot()
			}
			a2, m, err := parseSimpleAddr(rs[1:])
			n += m
			switch {
			case err != nil:
				return nil, tot + n, err
			case a2 == nil:
				a2 = Line(1)
			}
			if r == '+' {
				a1 = a1.Plus(a2)
			} else {
				a1 = a1.Minus(a2)
			}
		case r == ',' || r == ';':
			n++
			if a1 == nil {
				a1 = Line(0)
			}
			a2, m, err := Addr(rs[1:])
			n += m
			switch {
			case err != nil:
				return nil, tot + n, err
			case a2 == nil:
				a2 = End()
			}
			if r == ',' {
				a1 = a1.To(a2)
			} else {
				a1 = a1.Then(a2)
			}
		default:
			return a1, tot, nil
		}
		tot += n
		rs = rs[n:]
	}
}

func parseSimpleAddr(rs []rune) (SimpleAddress, int, error) {
	var n int
loop:
	for len(rs) > 0 {
		var m int
		var err error
		var a SimpleAddress
		switch r := rs[0]; {
		case unicode.IsSpace(r):
			n++
		case r == '#':
			a, m, err = parseRuneAddr(rs)
		case strings.ContainsRune(digits, r):
			a, m, err = parseLineAddr(rs)
		case r == '/' || r == '?':
			a, m, err = parseRegexpAddr(rs)
		case r == '$':
			a, m = End(), 1
		case r == '.':
			a, m = Dot(), 1
		default:
			break loop
		}
		n += m
		if a != nil {
			return a, n, err
		}
		rs = rs[n:]
	}
	return nil, n, nil
}

func parseRuneAddr(rs []rune) (SimpleAddress, int, error) {
	if rs[0] != '#' {
		panic("not a rune address")
	}
	var n int
	for n = 1; n < len(rs) && strings.ContainsRune(digits, rs[n]); n++ {
	}
	s := "1"
	if n > 1 {
		s = string(rs[1:n])
	}
	const base, bits = 10, 64
	r, err := strconv.ParseInt(s, base, bits)
	return Rune(r), n, err
}

func parseLineAddr(rs []rune) (SimpleAddress, int, error) {
	if !strings.ContainsRune(digits, rs[0]) {
		panic("not a line address")
	}
	var n int
	for n = 1; n < len(rs) && strings.ContainsRune(digits, rs[n]); n++ {
		n++
	}
	l, err := strconv.Atoi(string(rs[:n]))
	return Line(l), n, err
}

func parseRegexpAddr(rs []rune) (SimpleAddress, int, error) {
	if rs[0] != '/' && rs[0] != '?' {
		panic("not a regexp address")
	}
	opts := re1.Options{Delimited: true, Reverse: rs[0] == '?'}
	re, err := re1.Compile(rs, opts)
	if err != nil {
		return nil, 0, err
	}
	return Regexp(string(re.Expression())), len(re.Expression()), nil
}
