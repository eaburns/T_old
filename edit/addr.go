package edit

import (
	"errors"
	"strconv"
	"unicode/utf8"

	"github.com/eaburns/T/buffer"
	"github.com/eaburns/T/re1"
)

// An Address identifies a substring within a buffer.
type Address interface {
	rangeFrom(from int64, ed *Editor) (buffer.Address, error)
	String() string
	To(Address) Address
	Then(Address) Address
}

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

func (a compoundAddr) String() string {
	return a.a1.String() + string(a.op) + a.a2.String()
}

func (a compoundAddr) rangeFrom(from int64, ed *Editor) (buffer.Address, error) {
	a1, err := a.a1.rangeFrom(from, ed)
	if err != nil {
		return buffer.Address{}, err
	}
	switch a.op {
	case ',':
		a2, err := a.a2.rangeFrom(from, ed)
		if err != nil {
			return buffer.Address{}, err
		}
		return buffer.Address{From: a1.From, To: a2.To}, nil
	case ';':
		origDot := ed.dot
		ed.dot = a1
		a2, err := a.a2.rangeFrom(a1.To, ed)
		if err != nil {
			ed.dot = origDot // Restore dot on error.
			return buffer.Address{}, err
		}
		return buffer.Address{From: a1.From, To: a2.To}, nil
	default:
		panic("bad compound address")
	}
}

// An AdditiveAddress identifies a substring within a buffer.
// AdditiveAddresses are created using the + or - operation.
type AdditiveAddress interface {
	Address
	Plus(SimpleAddress) AdditiveAddress
	Minus(SimpleAddress) AdditiveAddress
}

type addAddr struct {
	op rune
	a1 AdditiveAddress
	a2 SimpleAddress
}

func (a addAddr) To(a2 Address) Address {
	return compoundAddr{op: ',', a1: a, a2: a2}
}

func (a addAddr) Then(a2 Address) Address {
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

func (a addAddr) rangeFrom(from int64, ed *Editor) (buffer.Address, error) {
	a1, err := a.a1.rangeFrom(from, ed)
	if err != nil {
		return buffer.Address{}, err
	}
	switch a.op {
	case '+':
		return a.a2.rangeFrom(a1.To, ed)
	case '-':
		return a.a2.reverse().rangeFrom(a1.From, ed)
	default:
		panic("bad additive address")
	}
}

// A SimpleAddress identifies a substring within a buffer.
// SimpleAddresses can be composed to form composite addresses.
type SimpleAddress interface {
	AdditiveAddress
	reverse() SimpleAddress
}

type simpAddrImpl interface {
	rangeFrom(from int64, ed *Editor) (buffer.Address, error)
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

func (a simpleAddr) Plus(a2 SimpleAddress) AdditiveAddress {
	return addAddr{op: '+', a1: a, a2: a2}
}

func (a simpleAddr) Minus(a2 SimpleAddress) AdditiveAddress {
	return addAddr{op: '-', a1: a, a2: a2}
}

type dotAddr struct{}

// Dot returns the address of the Editor's dot.
func Dot() SimpleAddress { return simpleAddr{dotAddr{}} }

func (dotAddr) String() string { return "." }

func (dotAddr) rangeFrom(_ int64, ed *Editor) (buffer.Address, error) {
	if ed.dot.From < 0 || ed.dot.To > ed.runes.Size() {
		return buffer.Address{}, errors.New("dot address out of range")
	}
	return ed.dot, nil
}

func (d dotAddr) reverse() SimpleAddress { return simpleAddr{d} }

type endAddr struct{}

// End returns the address of the empty string at the end of the buffer.
func End() SimpleAddress { return simpleAddr{endAddr{}} }

func (endAddr) String() string { return "$" }

func (endAddr) rangeFrom(_ int64, ed *Editor) (buffer.Address, error) {
	return buffer.Point(ed.runes.Size()), nil
}

func (e endAddr) reverse() SimpleAddress { return simpleAddr{e} }

type runeAddr int64

// Rune returns the address of the empty string after rune n.
func Rune(n int64) SimpleAddress { return simpleAddr{runeAddr(n)} }

func (n runeAddr) String() string {
	return "#" + strconv.FormatInt(int64(n), 10)
}

func (n runeAddr) rangeFrom(from int64, ed *Editor) (buffer.Address, error) {
	m := from + int64(n)
	if m < 0 || m > ed.runes.Size() {
		return buffer.Address{}, errors.New("rune address out of range")
	}
	return buffer.Point(m), nil
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

func (l lineAddr) rangeFrom(from int64, ed *Editor) (buffer.Address, error) {
	if l.neg {
		return l.rev(from, ed)
	}
	return l.fwd(from, ed)
}

func (l lineAddr) reverse() SimpleAddress {
	l.neg = !l.neg
	return simpleAddr{l}
}

func (l lineAddr) fwd(from int64, ed *Editor) (buffer.Address, error) {
	a := buffer.Address{From: from, To: from}
	if a.To > 0 {
		for a.To < ed.runes.Size() && ed.runes.Rune(a.To-1) != '\n' {
			a.To++
		}
		if l.n > 0 {
			a.From = a.To
		}
	}
	for l.n > 0 && a.To < ed.runes.Size() {
		r := ed.runes.Rune(a.To)
		a.To++
		if r == '\n' {
			l.n--
			if l.n > 0 {
				a.From = a.To
			}
		}
	}
	if l.n > 1 || l.n == 1 && a.To < ed.runes.Size() {
		return buffer.Address{}, errors.New("line address out of range")
	}
	return a, nil
}

func (l lineAddr) rev(from int64, ed *Editor) (buffer.Address, error) {
	a := buffer.Address{From: from, To: from}
	if a.From < ed.runes.Size() {
		for a.From > 0 && ed.runes.Rune(a.From-1) != '\n' {
			a.From--
		}
		a.To = a.From
	}
	for l.n > 0 && a.From > 0 {
		r := ed.runes.Rune(a.From - 1)
		a.From--
		if r == '\n' {
			l.n--
			a.To = a.From + 1
		} else if a.From == 0 {
			a.To = a.From
		}
	}
	if l.n > 1 {
		return buffer.Address{}, errors.New("line address out of range")
	}
	for a.From > 0 && ed.runes.Rune(a.From-1) != '\n' {
		a.From--
	}
	return a, nil
}

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
	return simpleAddr{reAddr{rev: re[0] == '?', re: re}}
}

func (r reAddr) String() string {
	var esc bool
	var rs []rune
	d, _ := utf8.DecodeRuneInString(r.re)
	for i, ru := range r.re {
		rs = append(rs, ru)
		// Ensure an unescaped trailing delimiter.
		if i == len(r.re)-utf8.RuneLen(ru) && (i == 0 || ru != d || esc) {
			rs = append(rs, d)
		}
		esc = !esc && ru == '\\'
	}
	return string(rs)
}

type reverse struct{ *buffer.Runes }

func (r reverse) Rune(i int64) rune {
	return r.Runes.Rune(r.Runes.Size() - i - 1)
}

func (r reAddr) rangeFrom(from int64, ed *Editor) (a buffer.Address, err error) {
	re, err := re1.Compile([]rune(r.re), re1.Options{Delimited: true, Reverse: r.rev})
	if err != nil {
		return a, err
	}
	runes := re1.Runes(ed.runes)
	if r.rev {
		runes = reverse{ed.runes}
		from = runes.Size() - from
	}
	defer buffer.RecoverRuneReadError(&err)
	match := re.Match(runes, from)
	if match == nil {
		return a, errors.New("no match")
	}
	a = buffer.Address{From: match[0][0], To: match[0][1]}
	if r.rev {
		a.From, a.To = runes.Size()-a.To, runes.Size()-a.From
	}
	return a, nil

}

func (r reAddr) reverse() SimpleAddress {
	r.rev = !r.rev
	return simpleAddr{r}
}
