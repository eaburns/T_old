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

// ErrNoMatch is returned when a regular expression fails to match.
var ErrNoMatch = errors.New("no match")

var (
	// All is the Address of the entire Text: 0,$.
	All = Line(0).To(End)

	// Dot is the Address of the dot mark.
	Dot SimpleAddress = Mark('.')

	// End is the Address of the empty string at the end of the Text.
	End SimpleAddress = end{}
)

// An Address identifies a Span within a Text.
type Address interface {
	// String returns the string representation of the Address.
	// The returned string will result in an equivalent Address
	// when parsed with Addr().
	String() string

	// To returns an Address identifying the string
	// from the start of the receiver to the end of the argument.
	To(AdditiveAddress) Address

	// Then returns an Address like To,
	// but with dot set to the receiver Address
	// during evaluation of the argument.
	Then(AdditiveAddress) Address

	// Where returns the Span of the Address evaluated on a Text.
	Where(Text) (Span, error)
}

type to struct {
	left  Address
	right AdditiveAddress
}

func (a to) String() string                 { return a.left.String() + "," + a.right.String() }
func (a to) To(b AdditiveAddress) Address   { return to{left: a, right: b} }
func (a to) Then(b AdditiveAddress) Address { return then{left: a, right: b} }

func (a to) Where(text Text) (Span, error) {
	left, err := a.left.Where(text)
	if err != nil {
		return Span{}, err
	}
	right, err := a.right.Where(text)
	if err != nil {
		return Span{}, err
	}
	return Span{left[0], right[1]}, nil
}

type then struct {
	left  Address
	right AdditiveAddress
}

func (a then) String() string                 { return a.left.String() + ";" + a.right.String() }
func (a then) To(b AdditiveAddress) Address   { return to{left: a, right: b} }
func (a then) Then(b AdditiveAddress) Address { return then{left: a, right: b} }

type withDot struct {
	Text
	dot Span
}

func (text withDot) Mark(r rune) Span {
	if r == '.' || unicode.IsSpace(r) {
		return text.dot
	}
	return text.Text.Mark(r)
}

func (a then) Where(text Text) (Span, error) {
	left, err := a.left.Where(text)
	if err != nil {
		return Span{}, err
	}
	right, err := a.right.Where(withDot{Text: text, dot: left})
	if err != nil {
		return Span{}, err
	}
	return Span{left[0], right[1]}, nil
}

// A AdditiveAddress identifies a Span within a Text.
// AdditiveAddress can be composed
// using the methods of the Address interface,
// and the Plus and Minus methods
// to form more-complex Addresses.
type AdditiveAddress interface {
	Address
	Plus(SimpleAddress) AdditiveAddress
	Minus(SimpleAddress) AdditiveAddress
	where(int64, Text) (Span, error)
}

type plus struct {
	left  AdditiveAddress
	right SimpleAddress
}

func (a plus) String() string                        { return a.left.String() + "+" + a.right.String() }
func (a plus) To(b AdditiveAddress) Address          { return to{left: a, right: b} }
func (a plus) Then(b AdditiveAddress) Address        { return then{left: a, right: b} }
func (a plus) Plus(b SimpleAddress) AdditiveAddress  { return plus{left: a, right: b} }
func (a plus) Minus(b SimpleAddress) AdditiveAddress { return minus{left: a, right: b} }
func (a plus) Where(text Text) (Span, error)         { return a.where(0, text) }

func (a plus) where(from int64, text Text) (Span, error) {
	left, err := a.left.where(from, text)
	if err != nil {
		return Span{}, err
	}
	return a.right.where(left[1], text)
}

type minus struct {
	left  AdditiveAddress
	right SimpleAddress
}

func (a minus) String() string                        { return a.left.String() + "-" + a.right.String() }
func (a minus) To(b AdditiveAddress) Address          { return to{left: a, right: b} }
func (a minus) Then(b AdditiveAddress) Address        { return then{left: a, right: b} }
func (a minus) Plus(b SimpleAddress) AdditiveAddress  { return plus{left: a, right: b} }
func (a minus) Minus(b SimpleAddress) AdditiveAddress { return minus{left: a, right: b} }
func (a minus) Where(text Text) (Span, error)         { return a.where(0, text) }

func (a minus) where(from int64, text Text) (Span, error) {
	left, err := a.left.where(from, text)
	if err != nil {
		return Span{}, err
	}
	return a.right.reverse().where(left[0], text)
}

// A SimpleAddress identifies a Span within a Text.
// SimpleAddresses can be composed
// using the methods of the AdditiveAddress interface
// to form more-complex Addresses.
type SimpleAddress interface {
	AdditiveAddress
	reverse() SimpleAddress
}

type end struct{}

func (a end) String() string                        { return "$" }
func (a end) To(b AdditiveAddress) Address          { return to{left: a, right: b} }
func (a end) Then(b AdditiveAddress) Address        { return then{left: a, right: b} }
func (a end) Plus(b SimpleAddress) AdditiveAddress  { return plus{left: a, right: b} }
func (a end) Minus(b SimpleAddress) AdditiveAddress { return minus{left: a, right: b} }
func (a end) reverse() SimpleAddress                { return a }
func (a end) Where(text Text) (Span, error)         { return a.where(0, text) }

func (a end) where(from int64, text Text) (Span, error) {
	size := text.Size()
	return Span{size, size}, nil
}

type line int

// Line returns the Address of the nth full line.
// A negative n is interpreted as n=0.
func Line(n int) SimpleAddress {
	if n < 0 {
		return line(0)
	}
	return line(n)
}

func (a line) String() string                        { return strconv.Itoa(int(a)) }
func (a line) To(b AdditiveAddress) Address          { return to{left: a, right: b} }
func (a line) Then(b AdditiveAddress) Address        { return then{left: a, right: b} }
func (a line) Plus(b SimpleAddress) AdditiveAddress  { return plus{left: a, right: b} }
func (a line) Minus(b SimpleAddress) AdditiveAddress { return minus{left: a, right: b} }
func (a line) reverse() SimpleAddress                { return line(int(-a)) }
func (a line) Where(text Text) (Span, error)         { return a.where(0, text) }

func (a line) where(from int64, text Text) (Span, error) {
	if a < 0 {
		return lineBackward(int(-a), from, text)
	}
	return lineForward(int(a), from, text)
}

func lineForward(n int, from int64, text Text) (Span, error) {
	s := Span{from, from}
	if from > 0 {
		// Position s1 at the beginning of the next full line.
		// If s1 is already at the beginning of a full line, we've got it.
		// Start by going back 1 rune.
		_, w, err := text.RuneReader(Span{from, 0}).ReadRune()
		if err != nil {
			return Span{}, err
		}
		s[1] -= int64(w)
		rr := text.RuneReader(Span{s[1], text.Size()})
	loop:
		for {
			switch r, w, err := rr.ReadRune(); {
			case err != nil && err != io.EOF:
				return Span{}, err
			case err == io.EOF:
				break loop
			case r == '\n':
				_, w, err := rr.ReadRune()
				if err != nil {
					return Span{}, err
				}
				s[1] += int64(w)
				break loop
			default:
				s[1] += int64(w)
			}
		}
		if n > 0 {
			s[0] = s[1]
		}
	}

	rr := text.RuneReader(Span{s[1], text.Size()})
	for n > 0 && s[1] < text.Size() {
		r, w, err := rr.ReadRune()
		if err != nil { // The error can't be EOF; s[1] < text.Size().
			return Span{}, err
		}
		s[1] += int64(w)
		if r == '\n' {
			n--
			if n > 0 {
				s[0] = s[1]
			}
		}
	}
	if n > 1 || n == 1 && s[1] < text.Size() {
		s = Span{text.Size(), text.Size()}
	}
	return s, nil
}

func lineBackward(n int, from int64, text Text) (Span, error) {
	s := Span{from, from}
	if s[0] < text.Size() {
		rr := text.RuneReader(Span{from, 0})
	loop:
		for {
			switch r, w, err := rr.ReadRune(); {
			case err != nil && err != io.EOF:
				return Span{}, err
			case err == io.EOF || r == '\n':
				break loop
			default:
				s[0] -= int64(w)
			}
		}
		s[1] = s[0]
	}
	rr := text.RuneReader(Span{s[0], 0})
	for n > 0 {
		r, w, err := rr.ReadRune()
		if err != nil && err != io.EOF {
			return Span{}, err
		}
		s[0] -= int64(w)
		if r == '\n' {
			n--
			s[1] = s[0] + 1
		} else if s[0] == 0 {
			s[1] = s[0]
		}
		if err == io.EOF {
			break
		}
	}
	if n > 1 {
		return Span{}, nil
	}
	for {
		r, w, err := rr.ReadRune()
		if err != nil && err != io.EOF {
			return Span{}, err
		} else if r == '\n' {
			break
		}
		s[0] -= int64(w)
		if err == io.EOF {
			break
		}
	}
	return s, nil
}

type mark rune

// Mark returns the Address of the named mark rune.
// If the rune is a space character, . is used.
func Mark(r rune) SimpleAddress {
	if unicode.IsSpace(r) {
		r = '.'
	}
	return mark(r)
}

func (a mark) String() string {
	if a == '.' {
		return "."
	}
	return "'" + string(rune(a))
}

func (a mark) To(b AdditiveAddress) Address           { return to{left: a, right: b} }
func (a mark) Then(b AdditiveAddress) Address         { return then{left: a, right: b} }
func (a mark) Plus(b SimpleAddress) AdditiveAddress   { return plus{left: a, right: b} }
func (a mark) Minus(b SimpleAddress) AdditiveAddress  { return minus{left: a, right: b} }
func (a mark) reverse() SimpleAddress                 { return a }
func (a mark) Where(text Text) (Span, error)          { return a.where(0, text) }
func (a mark) where(_ int64, text Text) (Span, error) { return text.Mark(rune(a)), nil }

type regexpAddr struct {
	regexp string
	rev    bool
}

// Regexp returns an Address identifying the next match of a regular expression.
// If Regexp is the right-hand operand of + or -, next is relative to the left-hand operand.
// Otherwise, next is relative to the . mark.
//
// If a forward search reaches the end of the Text without finding a match,
// it wraps to the beginning of the Text.
// If a reverse search reaches the beginning of the Text without finding a match,
// it wraps to the end of the Text.
//
// The regular expression syntax is that of the standard library regexp package.
// The syntax is documented here: https://github.com/google/re2/wiki/Syntax.
// All regular expressions are wrapped in (?m:<re>), making them multi-line by default.
// In a forward search, the relative start location
// (the . mark or the right-hand operand of +)
// is considered to be the beginning of text.
// So, for example, given:
// 	abcabc
// 	abc
// The address #3+/^abc will match the runes #3,#6,
// the second "abc" in the first line.
// Likewise, in a reverse search, the relative start location
// is considered to be the end of text.
func Regexp(regexp string) SimpleAddress                   { return regexpAddr{regexp: regexp} }
func (a regexpAddr) String() string                        { return "/" + Escape(a.regexp, '/') + "/" }
func (a regexpAddr) To(b AdditiveAddress) Address          { return to{left: a, right: b} }
func (a regexpAddr) Then(b AdditiveAddress) Address        { return then{left: a, right: b} }
func (a regexpAddr) Plus(b SimpleAddress) AdditiveAddress  { return plus{left: a, right: b} }
func (a regexpAddr) Minus(b SimpleAddress) AdditiveAddress { return minus{left: a, right: b} }

func (a regexpAddr) reverse() SimpleAddress {
	a.rev = !a.rev
	return a
}

func (a regexpAddr) Where(text Text) (Span, error) { return a.where(text.Mark('.')[1], text) }

func (a regexpAddr) where(from int64, text Text) (Span, error) {
	re, err := regexpCompile(a.regexp)
	if err != nil {
		return Span{}, err
	}
	var m []int
	if a.rev {
		m = prevMatch(re, from, text, true)
	} else {
		m = nextMatch(re, from, text, true)
	}
	if len(m) < 2 {
		return Span{}, ErrNoMatch
	}
	return Span{int64(m[0]), int64(m[1])}, nil
}

func match(re *regexp.Regexp, s Span, text Text) []int {
	m := re.FindReaderSubmatchIndex(text.RuneReader(s))
	for i := range m {
		m[i] += int(s[0])
	}
	return m
}

func nextMatch(re *regexp.Regexp, from int64, text Text, wrap bool) []int {
	m := match(re, Span{from, text.Size()}, text)
	if len(m) >= 2 && m[0] < m[1] {
		return m
	}
	if from > 0 && wrap {
		return nextMatch(re, 0, text, false)
	}
	return nil
}

func prevMatch(re *regexp.Regexp, from int64, text Text, wrap bool) []int {
	var prev []int
	for {
		span := Span{0, from}
		if len(prev) >= 2 {
			span[0] = int64(prev[1])
		}
		cur := match(re, span, text)
		if len(cur) < 2 || cur[0] >= cur[1] {
			break
		}
		prev = cur
	}
	if prev != nil {
		return prev
	}
	if size := text.Size(); from < size && wrap {
		return prevMatch(re, size, text, false)
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

type runeAddr int64

// Rune returns the Address of the empty Span after rune n.
// A negative n is interpreted as n=0.
func Rune(n int64) SimpleAddress {
	if n < 0 {
		return runeAddr(0)
	}
	return runeAddr(n)
}

func (a runeAddr) String() string                        { return "#" + strconv.FormatInt(int64(a), 10) }
func (a runeAddr) To(b AdditiveAddress) Address          { return to{left: a, right: b} }
func (a runeAddr) Then(b AdditiveAddress) Address        { return then{left: a, right: b} }
func (a runeAddr) Plus(b SimpleAddress) AdditiveAddress  { return plus{left: a, right: b} }
func (a runeAddr) reverse() SimpleAddress                { return runeAddr(-a) }
func (a runeAddr) Minus(b SimpleAddress) AdditiveAddress { return minus{left: a, right: b} }
func (a runeAddr) Where(text Text) (Span, error)         { return a.where(0, text) }

func (a runeAddr) where(from int64, text Text) (Span, error) {
	delta := 1
	s := Span{from, text.Size()}
	if a < 0 {
		a = -a
		delta = -1
		s[1] = 0
	}
	rr := text.RuneReader(s)
	for a > 0 {
		switch _, w, err := rr.ReadRune(); {
		case err == io.EOF:
			if delta < 0 {
				return Span{}, nil
			}
			return Span{text.Size(), text.Size()}, nil
		case err != nil:
			return Span{}, err
		default:
			from += int64(w * delta)
			a--
		}
	}
	return Span{from, from}, nil
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
// 		but with dot set to the receiver Address
// 		during evaluation of the argument.
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
		re, err := parseDelimited(r, rs)
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
