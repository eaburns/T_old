// Copyright © 2015, The T Authors.

package edit

import (
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	// All is the address of the entire buffer: 0,$.
	All = Line(0).To(End)
	// Dot is the address of the Editor's dot.
	Dot SimpleAddress = simpleAddr{dotAddr{}}
	// End is the address of the empty string at the end of the buffer.
	End SimpleAddress = simpleAddr{endAddr{}}
)

// An Address identifies a substring within a buffer.
type Address interface {
	// String returns the string representation of the address.
	// The returned string will result in an equivalent address
	// when parsed with Addr().
	String() string
	// To returns an address identifying the string
	// from the start of the receiver to the end of the argument.
	To(AdditiveAddress) Address
	// Then returns an address like To,
	// but with dot set to the receiver address
	// and with the argument evaluated from the end of the reciver
	Then(AdditiveAddress) Address
	// Where returns the addr of the Address.
	where(*Editor) (addr, error)
}

// A addr identifies a substring within a buffer
// by its inclusive start offset and its exclusive end offset.
type addr struct{ from, to int64 }

// Size returns the number of runes in
// the string identified by the range.
func (a addr) size() int64 { return a.to - a.from }

// Update returns a, updated to account for b changing to size n.
func (a addr) update(b addr, n int64) addr {
	// Clip, unless b is entirely within a.
	if a.from >= b.from || b.to > a.to {
		if b.contains(a.from) {
			a.from = b.to
		}
		if b.contains(a.to - 1) {
			a.to = b.from
		}
		if a.from > a.to {
			a.to = a.from
		}
	}

	// Move.
	d := n - b.size()
	if a.to >= b.to {
		a.to += d
	}
	if a.from >= b.to {
		a.from += d
	}
	return a
}

func (a addr) contains(p int64) bool { return a.from <= p && p < a.to }

type compoundAddr struct {
	op     rune
	a1, a2 Address
}

func (a compoundAddr) To(a2 AdditiveAddress) Address {
	return compoundAddr{op: ',', a1: a, a2: a2}
}

func (a compoundAddr) Then(a2 AdditiveAddress) Address {
	return compoundAddr{op: ';', a1: a, a2: a2}
}

func (a compoundAddr) String() string {
	return a.a1.String() + string(a.op) + a.a2.String()
}

func (a compoundAddr) where(ed *Editor) (addr, error) {
	a1, err := a.a1.where(ed)
	if err != nil {
		return addr{}, err
	}
	if a.op == ',' {
		a2, err := a.a2.where(ed)
		if err != nil {
			return addr{}, err
		}
		return addr{from: a1.from, to: a2.to}, nil
	}
	origDot := ed.marks['.']
	ed.marks['.'] = a1
	a2, err := a.a2.where(ed)
	if err != nil {
		ed.marks['.'] = origDot // Restore dot on error.
		return addr{}, err
	}
	return addr{from: a1.from, to: a2.to}, nil
}

// A AdditiveAddress identifies a substring within a buffer.
// AdditiveAddress can be composed
// using the methods of the Address interface,
// and the Plus and Minus methods
// to form more-complex, composite addresses.
type AdditiveAddress interface {
	Address
	// Plus returns an address identifying the string
	// of the argument address evaluated from the end of the receiver.
	Plus(SimpleAddress) AdditiveAddress
	// Minus returns an address identifying the string
	// of the argument address evaluated in reverse
	// from the start of the receiver.
	Minus(SimpleAddress) AdditiveAddress
	// WhereFrom returns the addr of the AdditiveAddress,
	// relative to the from point.
	whereFrom(from int64, ed *Editor) (addr, error)
}

type addAddr struct {
	op rune
	a1 AdditiveAddress
	a2 SimpleAddress
}

func (a addAddr) To(a2 AdditiveAddress) Address {
	return compoundAddr{op: ',', a1: a, a2: a2}
}

func (a addAddr) Then(a2 AdditiveAddress) Address {
	return compoundAddr{op: ';', a1: a, a2: a2}
}

func (a addAddr) Plus(a2 SimpleAddress) AdditiveAddress {
	return addAddr{op: '+', a1: a, a2: a2}
}

func (a addAddr) Minus(a2 SimpleAddress) AdditiveAddress {
	return addAddr{op: '-', a1: a, a2: a2}
}

func (a addAddr) String() string {
	return a.a1.String() + string(a.op) + a.a2.String()
}

func (a addAddr) where(ed *Editor) (addr, error) { return a.whereFrom(0, ed) }

func (a addAddr) whereFrom(from int64, ed *Editor) (addr, error) {
	a1, err := a.a1.whereFrom(from, ed)
	if err != nil {
		return addr{}, err
	}
	if a.op == '+' {
		return a.a2.whereFrom(a1.to, ed)
	}
	return a.a2.reverse().whereFrom(a1.from, ed)
}

// A SimpleAddress identifies a substring within a buffer.
// SimpleAddresses can be composed
// using the methods of the AdditiveAddress interface
// to form more-complex, composite addresses.
type SimpleAddress interface {
	AdditiveAddress
	reverse() SimpleAddress
}

type simpAddrImpl interface {
	where(*Editor) (addr, error)
	whereFrom(from int64, ed *Editor) (addr, error)
	String() string
	reverse() SimpleAddress
}

type simpleAddr struct {
	simpAddrImpl
}

func (a simpleAddr) To(a2 AdditiveAddress) Address {
	return compoundAddr{op: ',', a1: a, a2: a2}
}

func (a simpleAddr) Then(a2 AdditiveAddress) Address {
	return compoundAddr{op: ';', a1: a, a2: a2}
}

func (a simpleAddr) Plus(a2 SimpleAddress) AdditiveAddress {
	return addAddr{op: '+', a1: a, a2: a2}
}

func (a simpleAddr) Minus(a2 SimpleAddress) AdditiveAddress {
	return addAddr{op: '-', a1: a, a2: a2}
}

type dotAddr struct{}

func (dotAddr) String() string { return "." }

func (a dotAddr) where(ed *Editor) (addr, error) {
	return a.whereFrom(0, ed)
}

func (dotAddr) whereFrom(_ int64, ed *Editor) (addr, error) {
	a := ed.marks['.']
	if a.from < 0 || a.to < a.from || a.to > ed.buf.size() {
		panic("bad dot")
	}
	return ed.marks['.'], nil
}

func (d dotAddr) reverse() SimpleAddress { return simpleAddr{d} }

type endAddr struct{}

func (endAddr) String() string { return "$" }

func (a endAddr) where(ed *Editor) (addr, error) { return a.whereFrom(0, ed) }

func (endAddr) whereFrom(_ int64, ed *Editor) (addr, error) {
	return addr{from: ed.buf.size(), to: ed.buf.size()}, nil
}

func (e endAddr) reverse() SimpleAddress { return simpleAddr{e} }

type markAddr rune

// Mark returns the address of the named mark rune.
// The rune must be a lower-case or upper-case letter or dot: [a-zA-Z.].
// An invalid mark rune results in an address that returns an error when evaluated.
func Mark(r rune) SimpleAddress {
	if unicode.IsSpace(r) {
		r = '.'
	}
	return simpleAddr{markAddr(r)}
}

func (m markAddr) String() string { return "'" + string(rune(m)) }

func (m markAddr) where(ed *Editor) (addr, error) { return m.whereFrom(0, ed) }

func (m markAddr) whereFrom(_ int64, ed *Editor) (addr, error) {
	a := ed.marks[rune(m)]
	if a.from < 0 || a.to < a.from || a.to > ed.buf.size() {
		panic("bad mark")
	}
	return a, nil
}

func (m markAddr) reverse() SimpleAddress { return simpleAddr{m} }

type runeAddr int64

// Rune returns the address of the empty string after rune n.
// If n is negative, this is equivalent to the compound address -#n.
func Rune(n int64) SimpleAddress { return simpleAddr{runeAddr(n)} }

func (n runeAddr) String() string {
	if n < 0 {
		return "-#" + strconv.FormatInt(int64(-n), 10)
	}
	return "#" + strconv.FormatInt(int64(n), 10)
}

func (n runeAddr) where(ed *Editor) (addr, error) {
	if n < 0 {
		return n.whereFrom(ed.marks['.'].from, ed)
	}
	return n.whereFrom(0, ed)
}

func (n runeAddr) whereFrom(from int64, ed *Editor) (addr, error) {
	m := from + int64(n)
	if m < 0 || m > ed.buf.size() {
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
// If n is negative, this is equivalent to the compound address -n.
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

func (l lineAddr) where(ed *Editor) (addr, error) {
	if l.neg {
		return l.whereFrom(ed.marks['.'].from, ed)
	}
	return l.whereFrom(0, ed)
}

func (l lineAddr) whereFrom(from int64, ed *Editor) (addr, error) {
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
		for a.to < ed.buf.size() {
			r, err := ed.buf.rune(a.to - 1)
			if err != nil {
				return a, err
			} else if r == '\n' {
				break
			}
			a.to++
		}
		if l.n > 0 {
			a.from = a.to
		}
	}
	for l.n > 0 && a.to < ed.buf.size() {
		r, err := ed.buf.rune(a.to)
		if err != nil {
			return a, err
		}
		a.to++
		if r == '\n' {
			l.n--
			if l.n > 0 {
				a.from = a.to
			}
		}
	}
	if l.n > 1 || l.n == 1 && a.to < ed.buf.size() {
		return addr{}, errors.New("line address out of range")
	}
	return a, nil
}

func (l lineAddr) rev(from int64, ed *Editor) (addr, error) {
	a := addr{from: from, to: from}
	if a.from < ed.buf.size() {
		for a.from > 0 {
			r, err := ed.buf.rune(a.from - 1)
			if err != nil {
				return a, err
			} else if r == '\n' {
				break
			}
			a.from--
		}
		a.to = a.from
	}
	for l.n > 0 && a.from > 0 {
		r, err := ed.buf.rune(a.from - 1)
		if err != nil {
			return a, err
		}
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
	for a.from > 0 {
		r, err := ed.buf.rune(a.from - 1)
		if err != nil {
			return a, err
		} else if r == '\n' {
			break
		}
		a.from--
	}
	return a, nil
}

// ErrNoMatch is returned when a regular expression address fails to match.
var ErrNoMatch = errors.New("no match")

type regexpAddr struct {
	regexp string
	// Previous is whether to return the previous match instead of the next.
	// It can only be true if the Regexp is the right-hand operand of a - address.
	previous bool
}

// Regexp returns an address identifying the next match of a regular expression.
// If Regexp is the right-hand operand of + or -, next is relative to the left-hand operand.
// Otherwise, next is relative to dot.
//
// If a forward search reaches the end of the buffer without finding a match,
// it wraps to the beginning of the buffer.
// If a reverse search reaches the beginning of the buffer without finding a match,
// it wraps to the end of the buffer.
//
// The regular expression syntax is that of the standard library regexp package.
// The syntax is documented here: https://github.com/google/re2/wiki/Syntax.
// All regular expressions are wrapped in (?m:<re>), making them multi-line by default.
// In a forward search, the relative start location
// (dot or the right-hand operand of +)
// is considered to be the beginning of text.
// So, for example, given:
// 	abcabc
// 	abc
// The address #3+/^abc will match the runes #3,#6,
// the second "abc" in the first line.
// Likewise, in a reverse search, the relative start location
// is considered to be the end of text.
func Regexp(re string) SimpleAddress {
	return simpleAddr{regexpAddr{regexp: re}}
}

func (a regexpAddr) String() string {
	return "/" + Escape(a.regexp, '/') + "/"
}

func (a regexpAddr) where(ed *Editor) (addr, error) {
	return a.whereFrom(ed.marks['.'].to, ed)
}

func (a regexpAddr) whereFrom(from int64, ed *Editor) (addr, error) {
	re, err := regexpCompile(a.regexp)
	if err != nil {
		return addr{}, err
	}
	var m []int
	if a.previous {
		m = prevMatch(re, from, ed, true)
	} else {
		m = nextMatch(re, from, ed, true)
	}
	if len(m) < 2 {
		return addr{}, ErrNoMatch
	}
	return addr{from: int64(m[0]), to: int64(m[1])}, nil
}

type runeReader struct {
	addr
	ed *Editor
}

func (rr *runeReader) ReadRune() (rune, int, error) {
	switch {
	case rr.size() <= 0:
		return 0, 0, io.EOF
	case rr.from < 0 || rr.from >= rr.ed.buf.size():
		return 0, 0, errors.New("out of range")
	}
	r, err := rr.ed.buf.runes.Rune(rr.from)
	rr.from++
	return r, 1, err
}

func rangeMatch(re *regexp.Regexp, at addr, ed *Editor) []int {
	rr := &runeReader{addr: at, ed: ed}
	m := re.FindReaderSubmatchIndex(rr)
	for i := range m {
		m[i] += int(at.from)
	}
	return m
}

func nextMatch(re *regexp.Regexp, from int64, ed *Editor, wrap bool) []int {
	m := rangeMatch(re, addr{from: from, to: ed.buf.size()}, ed)
	if len(m) >= 2 && m[0] < m[1] {
		return m
	}
	if from > 0 && wrap {
		return nextMatch(re, 0, ed, false)
	}
	return nil
}

func prevMatch(re *regexp.Regexp, from int64, ed *Editor, wrap bool) []int {
	var prev []int
	for {
		at := addr{to: from}
		if len(prev) >= 2 {
			at.from = int64(prev[1])
		}
		cur := rangeMatch(re, at, ed)
		if len(cur) < 2 || cur[0] >= cur[1] {
			break
		}
		prev = cur
	}
	if prev != nil {
		return prev
	}
	if from < ed.buf.size() && wrap {
		return prevMatch(re, ed.buf.size(), ed, false)
	}
	return nil
}

func regexpCompile(re string) (*regexp.Regexp, error) {
	if re == "\\" || len(re) > 2 && re[len(re)-1] == '\\' && re[len(re)-2] != '\\' {
		// Escape a trailing, unescaped \.
		re = re + "\\"
	}
	return regexp.Compile("(?m:" + re + ")")
}

func (a regexpAddr) reverse() SimpleAddress {
	a.previous = !a.previous
	return simpleAddr{a}
}

const (
	digits      = "0123456789"
	simpleFirst = "#/$.'" + digits
)

// Addr parses and returns an address.
//
// The address syntax for address a is:
// 	a: {a} , {aa} | {a} ; {aa} | {aa}
// 	aa: {aa} + {sa} | {aa} - {sa} | {aa} {sa} | {sa}
// 	sa: $ | . | 'r | #{n} | n | / regexp {/}
// 	n: [0-9]+
// 	r: any non-space rune
// 	regexp: any valid re1 regular expression
// All operators are left-associative.
//
// Production sa describes a simple addresse:
//	$ is the empty string at the end of the buffer.
//	. is the current address of the editor, called dot.
//	'{r} is the address of the non-space rune, r. If r is missing, . is used.
//	#{n} is the empty string after rune number n. If n is missing then 1 is used.
//	n is the nth line in the buffer. 0 is the string before the first full line.
//	'/' regexp {'/'} is the first match of the regular expression.
// 		The regexp uses the syntax of the standard library regexp package,
// 		except that \, raw newlines, and / must be escaped with \.
// 		The regexp is wrapped in (?m:<regexp>), making it multi-line by default.
//
// Production aa describes an additive address:
//	{aa} '+' {sa} is the second address evaluated from the end of the first.
//		If the first address is missing, . is used.
//		If the second address is missing, 1 is used.
//	{aa} '-' {sa} is the second address evaluated in reverse from the start of the first.
//		If the first address is missing, . is used.
//		If the second address is missing, 1 is used.
// 	If two addresses of the form aa sa are present and distinct
// 	then a '+' is inserted, as in aa + as.
//
// Production a describes a range address:
//	{a} ',' {aa} is the string from the start of the first address to the end of the second.
//		If the first address is missing, 0 is used.
//		If the second address is missing, $ is used.
//	{a} ';' {aa} is like the previous,
//		but with the second address evaluated from the end of the first
//		with dot set to the first address.
//		If the first address is missing, 0 is used.
//		If the second address is missing, $ is used.
//
// Addresses are terminated by a newline, end of input,
// or end of the address.
// For example:
// 	1,5
// 	-1
// 		Is terminated at the newline precceding -.
// 		The newline is not consumed.
//
//	1,5-6
// 		Is terminated at 6 at the end of the input.
//
// 	1,5dabc
// 		Is terminated at 5, the end of the address.
func Addr(rs io.RuneScanner) (Address, error) {
	aa, err := parseAdditiveAddress(rs)
	if err != nil {
		return nil, err
	}
	a, err := parseAddressTail(aa, rs)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func parseAddressTail(left Address, rs io.RuneScanner) (Address, error) {
	if err := skipSpace(rs); err != nil {
		return nil, err
	}
	switch r, _, err := rs.ReadRune(); {
	case err == io.EOF:
		break
	case err != nil:
		return nil, err
	case r == ',' || r == ';':
		if left == nil {
			left = Line(0)
		}
		right, err := parseAdditiveAddress(rs)
		if err != nil {
			return nil, err
		}
		if right == nil {
			right = End
		}
		var a Address
		if r == ',' {
			a = left.To(right)
		} else {
			a = left.Then(right)
		}
		return parseAddressTail(a, rs)
	default:
		return left, rs.UnreadRune()
	}
	return left, nil
}

func parseAdditiveAddress(rs io.RuneScanner) (AdditiveAddress, error) {
	a, err := parseSimpleAddress(rs)
	if err != nil {
		return nil, err
	}
	return parseAdditiveAddressTail(a, rs)
}

func parseAdditiveAddressTail(left AdditiveAddress, rs io.RuneScanner) (AdditiveAddress, error) {
	if err := skipSpace(rs); err != nil {
		return nil, err
	}
	switch r, _, err := rs.ReadRune(); {
	case err == io.EOF:
		break
	case err != nil:
		return nil, err
	case r == '-' || r == '+':
		if left == nil {
			left = Dot
		}
		right, err := parseSimpleAddress(rs)
		if err != nil {
			return nil, err
		}
		if right == nil {
			right = Line(1)
		}
		var a AdditiveAddress
		if r == '+' {
			a = left.Plus(right)
		} else {
			a = left.Minus(right)
		}
		return parseAdditiveAddressTail(a, rs)
	case strings.ContainsRune(simpleFirst, r):
		if err := rs.UnreadRune(); err != nil {
			return nil, err
		}
		right, err := parseSimpleAddress(rs)
		if err != nil {
			return nil, err
		}
		// Left cannot be nil.
		// We either came from parseAdditiveAddress or a recursive call.
		// In the first case, parseSimpleAddress would return an error, not nil.
		// In the sceond case, we are always called with non-nill left.
		//
		// Right cannot be nil.
		// parseSimpleAddress never returns nil when the first rune is in simpleFirst.
		if left == nil || right == nil {
			panic("impossible")
		}
		return parseAdditiveAddressTail(left.Plus(right), rs)
	default:
		return left, rs.UnreadRune()
	}
	return left, nil
}

func parseSimpleAddress(rs io.RuneScanner) (SimpleAddress, error) {
	if err := skipSpace(rs); err != nil {
		return nil, err
	}
	switch r, _, err := rs.ReadRune(); {
	case err == io.EOF:
		return nil, nil
	case err != nil:
		return nil, err
	case r == '\'':
		return parseMarkAddr(rs)
	case r == '#':
		return parseRuneAddr(rs)
	case strings.ContainsRune(digits, r):
		return parseLineAddr(r, rs)
	case r == '/':
		re, err := parseDelimitedNew(r, rs)
		if err != nil {
			return nil, err
		}
		if _, err := regexpCompile(re); err != nil {
			return nil, err
		}
		return Regexp(re), nil
	case r == '$':
		return End, nil
	case r == '.':
		return Dot, nil
	default:
		return nil, rs.UnreadRune()
	}
}

// ParseDelimited returns the unescaped string of runes
// up to the first non-escaped delimiter, raw newline, or EOF.
func parseDelimitedNew(delim rune, rs io.RuneScanner) (string, error) {
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

func parseMarkAddr(rs io.RuneScanner) (SimpleAddress, error) {
	for {
		switch r, _, err := rs.ReadRune(); {
		case err == io.EOF || err == nil && r == '\n':
			return Mark('.'), nil
		case err != nil:
			return nil, err
		case !unicode.IsSpace(r):
			return Mark(r), nil
		}
	}
}

func parseRuneAddr(rs io.RuneScanner) (SimpleAddress, error) {
	s, err := scanDigits(rs)
	if err != nil {
		return nil, err
	}
	if len(s) == 0 {
		s = "1"
	}
	const base, bits = 10, 64
	r, err := strconv.ParseInt(s, base, bits)
	return Rune(r), err
}

func parseLineAddr(r rune, rs io.RuneScanner) (SimpleAddress, error) {
	s, err := scanDigits(rs)
	if err != nil {
		return nil, err
	}
	l, err := strconv.Atoi(string(r) + s)
	return Line(l), err
}

func scanDigits(rs io.RuneScanner) (string, error) {
	var s []rune
	for {
		switch r, _, err := rs.ReadRune(); {
		case err == io.EOF:
			return string(s), nil
		case err != nil:
			return "", err
		case !unicode.IsDigit(r):
			return string(s), rs.UnreadRune()
		default:
			s = append(s, r)
		}
	}
}
