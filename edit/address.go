package edit

import "github.com/eaburns/T/edit/buffer"

const runeBytes = 4

// An AddressError records an error caused by an invalid address.
type AddressError Address

func (err AddressError) Error() string { return "invalid address" }

// An Address identifies a substring of a Buffer.
// The substring is the set of runes between From (inclusive) and To (exclusive).
type Address struct {
	From, To int64
}

// Size returns the number of runes identified by the Address.
func (a Address) Size() int64 {
	return a.To - a.From
}

// ByteSize returns the number of bytes identified by the Address.
func (a Address) byteSize() int64 {
	return (a.To - a.From) * runeBytes
}

// FromByte returns the byte offset into the rune buffer
// of the From portion of the Address.
func (a Address) fromByte() int64 {
	return a.From * runeBytes
}

func (a Address) bufferAddress() buffer.Address {
	return buffer.Address{
		From: a.fromByte(),
		To:   a.fromByte() + a.byteSize(),
	}
}
