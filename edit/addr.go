package edit

import (
	"errors"
	"strconv"

	"github.com/eaburns/T/buffer"
	"github.com/eaburns/T/re1"
)

// An Address identifies a substring within a buffer.
type Address interface {
	Range(*Editor) (buffer.Address, error)
	String() string
}

// A SimpleAddress identifies a substring within a buffer.
// SimpleAddresses can be composed to form composite addresses.
type SimpleAddress struct{ simpleAddress }

type simpleAddress interface {
	Address
	rangeFrom(from int64, ed *Editor) (buffer.Address, error)
	reverse() SimpleAddress
}

type dotAddr struct{}

// Dot returns the address of the Editor's dot.
func Dot() SimpleAddress { return SimpleAddress{dotAddr{}} }

func (dotAddr) String() string { return "." }

func (d dotAddr) Range(ed *Editor) (buffer.Address, error) {
	return d.rangeFrom(0, ed)
}

func (dotAddr) rangeFrom(_ int64, ed *Editor) (buffer.Address, error) {
	if ed.dot.From < 0 || ed.dot.To > ed.runes.Size() {
		return buffer.Address{}, errors.New("dot address out of range")
	}
	return ed.dot, nil
}

func (d dotAddr) reverse() SimpleAddress { return SimpleAddress{d} }

type endAddr struct{}

// End returns the address of the empty string at the end of the buffer.
func End() SimpleAddress { return SimpleAddress{endAddr{}} }

func (endAddr) String() string { return "$" }

func (e endAddr) Range(ed *Editor) (buffer.Address, error) {
	return e.rangeFrom(0, ed)
}

func (endAddr) rangeFrom(_ int64, ed *Editor) (buffer.Address, error) {
	return buffer.Point(ed.runes.Size()), nil
}

func (e endAddr) reverse() SimpleAddress { return SimpleAddress{e} }

type runeAddr int64

// Rune returns the address of the empty string after rune n.
func Rune(n int64) SimpleAddress { return SimpleAddress{runeAddr(n)} }

func (n runeAddr) String() string {
	return "#" + strconv.FormatInt(int64(n), 10)
}

func (n runeAddr) Range(ed *Editor) (buffer.Address, error) {
	return n.rangeFrom(0, ed)
}

func (n runeAddr) rangeFrom(from int64, ed *Editor) (buffer.Address, error) {
	m := from + int64(n)
	if m < 0 || m > ed.runes.Size() {
		return buffer.Address{}, errors.New("rune address out of range")
	}
	return buffer.Point(m), nil
}

func (n runeAddr) reverse() SimpleAddress { return SimpleAddress{runeAddr(-n)} }

type lineAddr struct {
	neg bool
	n   int
}

// Line returns the address of the nth full line.
func Line(n int) SimpleAddress {
	if n < 0 {
		return SimpleAddress{lineAddr{neg: true, n: -n}}
	}
	return SimpleAddress{lineAddr{n: n}}
}

func (l lineAddr) String() string {
	n := strconv.Itoa(int(l.n))
	if l.neg {
		return "-" + n
	}
	return n
}

func (l lineAddr) Range(ed *Editor) (buffer.Address, error) {
	return l.rangeFrom(0, ed)
}

func (l lineAddr) rangeFrom(from int64, ed *Editor) (buffer.Address, error) {
	if l.neg {
		return l.rev(from, ed)
	}
	return l.fwd(from, ed)
}

func (l lineAddr) reverse() SimpleAddress {
	l.neg = !l.neg
	return SimpleAddress{l}
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
		if !nl && a.From <= 0 {
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
func Regexp(re string) SimpleAddress { return SimpleAddress{reAddr(re)} }

func (r reAddr) String() string {
	if len(r) > 0 && r[len(r)-1] == r[0] {
		r = r[:len(r)-1]
	}
	return string(r)
}

func (r reAddr) Range(ed *Editor) (buffer.Address, error) {
	return r.rangeFrom(0, ed)
}

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
		return SimpleAddress{r}
	case r[0] == '?':
		s := []rune(string(r))
		s[0] = '/'
		return SimpleAddress{reAddr(string(s))}
	case r[0] == '/':
		s := []rune(string(r))
		s[0] = '?'
		return SimpleAddress{reAddr(string(s))}
	default:
		panic("malformed regexp")
	}
}
