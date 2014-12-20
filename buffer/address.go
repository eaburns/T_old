package buffer

import "strconv"

// An Address identifies a substring of a Buffer.
// The substring is the set of runes between From (inclusive) and To (exclusive).
type Address struct {
	From, To int64
}

// Size returns the length of the string identified by the address.
func (a Address) Size() int64 {
	return a.To - a.From
}

// AsBytes returns the byte Address of a rune Address.
func (a Address) asBytes() Address {
	return Address{From: a.From * runeBytes, To: a.To * runeBytes}
}

func (a Address) String() string {
	return strconv.FormatInt(a.From, 10) + ", " + strconv.FormatInt(a.To, 10)
}

// Point returns the address of the empty string at the given point.
func Point(at int64) Address {
	return Address{From: at, To: at}
}

// An AddressError records an error caused by an out-of-bounds address.
type AddressError Address

func (err AddressError) Error() string {
	return "invalid address: " + Address(err).String()
}
