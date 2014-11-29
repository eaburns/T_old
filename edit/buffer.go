// Package edit provides an API for ed-like editing of file-backed buffers.
package edit

import (
	"encoding/binary"

	"github.com/eaburns/T/edit/buffer"
)

// A Buffer is an unbounded buffer of runes.
type Buffer struct {
	bytes *buffer.Buffer
}

// NewBuffer returns a new Buffer.
// The buffer caches no more than blockSize runes in memory.
func NewBuffer(blockSize int) *Buffer {
	return &Buffer{bytes: buffer.New(blockSize * runeBytes)}
}

// Close closes the buffer, freeing its resources.
func (b *Buffer) Close() error {
	return b.bytes.Close()
}

// Size returns the number of runes in the Buffer.
func (b *Buffer) Size() int64 {
	return b.bytes.Size() / runeBytes
}

// Read returns the runes in the range of an Address in the buffer.
func (b *Buffer) Read(addr Address) ([]rune, error) {
	if addr.From < 0 || addr.From > addr.To || addr.To > b.Size() {
		return nil, AddressError(addr)
	}

	bs := make([]byte, addr.byteSize())
	if _, err := b.bytes.ReadAt(bs, addr.fromByte()); err != nil {
		return nil, err
	}

	rs := make([]rune, 0, addr.Size())
	for len(bs) > 0 {
		r := rune(binary.LittleEndian.Uint32(bs))
		rs = append(rs, r)
		bs = bs[runeBytes:]
	}
	return rs, nil
}

// Write writes runes to the range of an Address in the buffer.
func (b *Buffer) Write(rs []rune, addr Address) error {
	if addr.From < 0 || addr.From > addr.To || addr.To > b.Size() {
		return AddressError(addr)
	}

	if _, err := b.bytes.Delete(addr.byteSize(), addr.fromByte()); err != nil {
		return err
	}

	bs := make([]byte, len(rs)*runeBytes)
	for i, r := range rs {
		binary.LittleEndian.PutUint32(bs[i*runeBytes:], uint32(r))
	}

	_, err := b.bytes.Insert(bs, addr.fromByte())
	return err
}
