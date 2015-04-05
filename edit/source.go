package edit

import (
	"errors"

	"github.com/eaburns/T/edit/runes"
)

// A source is a source of runes that can be inserted into a buffer.
type source interface {
	size() int64
	insert(b *runes.Buffer, at int64) error
}

type emptySource struct{}

func (s emptySource) size() int64 { return 0 }

func (s emptySource) insert(*runes.Buffer, int64) error { return nil }

type sliceSource []rune

func (s sliceSource) size() int64 { return int64(len(s)) }

func (s sliceSource) insert(b *runes.Buffer, at int64) error { return b.Insert(s, at) }

type bufferSource struct {
	at  addr
	buf *runes.Buffer
}

func (s bufferSource) size() int64 { return s.at.size() }

// TODO(eaburns): Do this properly.
// Probably add runes.Append, which is rune-cache-aware.
// Whatever you do, don't read the entire thing into memory,
// that defeats the entire point.
func (s bufferSource) insert(b *runes.Buffer, at int64) error {
	if s.buf == nil {
		return nil
	}
	if s.at.size() > MaxRead {
		// This goes away when the TODO above is fixed.
		return errors.New("bufferSource insert too big")
	}
	rs, err := s.buf.Read(int(s.at.size()), s.at.from)
	if err != nil {
		return err
	}
	return b.Insert(rs, at)
}
