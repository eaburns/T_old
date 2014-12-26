package edit

import (
	"errors"
	"strconv"

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
	switch {
	case l.n == 0 && from == 0:
		return buffer.Point(0), nil
	case from == 0 || ed.runes.Rune(from-1) == '\n':
		l.n-- // Already at the start of a line.
	}
	var err error
	var nl bool
	a := buffer.Address{From: from, To: from}
	for i := 0; i < l.n+1; i++ {
		if !nl && a.To >= ed.runes.Size() && l.n > 0 {
			return buffer.Address{}, errors.New("line address out of range")
		}
		a.From = a.To
		a.To, nl, err = l.nextLine(a.From, +1, ed)
		if err != nil {
			return buffer.Address{}, err
		}
	}
	return a, nil
}

func (l lineAddr) rev(from int64, ed *Editor) (buffer.Address, error) {
	var err error
	var nl bool
	a := buffer.Address{From: from, To: from}
	for i := 0; i < l.n+1; i++ {
		if a.From == 0 && i == l.n {
			a = buffer.Point(0)
			break
		}
		if i > 0 && a.From <= 0 {
			return buffer.Address{}, errors.New("line address out of range")
		}
		a.To = a.From
		if nl {
			a.From--
		}
		a.From, nl, err = l.nextLine(a.From, -1, ed)
		if err != nil {
			return buffer.Address{}, err
		}
	}
	return a, nil
}

// NextLine returns the address of the empty string
// immediately after the next \n or 0
// or the address of the end of the buffer.
func (lineAddr) nextLine(from, delta int64, ed *Editor) (int64, bool, error) {
	var i int64
	for i = from; delta > 0 && i < ed.runes.Size() || delta < 0 && i > 0; i += delta {
		switch {
		case delta < 0 && ed.runes.Rune(i-1) == '\n':
			return i, true, nil
		case delta > 0 && ed.runes.Rune(i) == '\n':
			return i + 1, true, nil
		}
	}
	return i, false, nil
}

type reAddr string

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
	return simpleAddr{reAddr(re)}
}

func (r reAddr) String() string { return string(r) + string(r[0]) }

type reverse struct{ *buffer.Runes }

func (r reverse) Rune(i int64) rune {
	return r.Runes.Rune(r.Runes.Size() - i - 1)
}

func (r reAddr) rangeFrom(from int64, ed *Editor) (a buffer.Address, err error) {
	rev := len(r) > 0 && r[0] == '?'
	re, err := re1.Compile([]rune(string(r)), re1.Options{Delimited: true, Reverse: rev})
	if err != nil {
		return a, err
	}
	runes := re1.Runes(ed.runes)
	if rev {
		runes = reverse{ed.runes}
		from = runes.Size() - from
	}
	defer buffer.RecoverRuneReadError(&err)
	match := re.Match(runes, from)
	if match == nil {
		return a, errors.New("no match")
	}
	a = buffer.Address{From: match[0][0], To: match[0][1]}
	if rev {
		a.From, a.To = runes.Size()-a.To, runes.Size()-a.From
	}
	return a, nil

}

func (r reAddr) reverse() SimpleAddress {
	switch {
	case len(r) == 0:
		return simpleAddr{r}
	case r[0] == '?':
		s := []rune(string(r))
		s[0] = '/'
		return simpleAddr{reAddr(string(s))}
	case r[0] == '/':
		s := []rune(string(r))
		s[0] = '?'
		return simpleAddr{reAddr(string(s))}
	default:
		panic("malformed regexp")
	}
}
