package edit

import "github.com/eaburns/T/edit/runes"

// A source is a source of runes that can be inserted into a buffer.
type source interface {
	size() int64
	insert(b *runes.Buffer, at int64) error
}

type sliceSource []rune

func (s sliceSource) size() int64 { return int64(len(s)) }

func (s sliceSource) insert(b *runes.Buffer, at int64) error {
	_, err := b.Insert(s, at)
	return err
}

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
	rs := make([]rune, s.at.size())
	_, err := s.buf.Read(rs, s.at.from)
	if err != nil {
		return err
	}
	_, err = b.Insert(rs, at)
	return err
}
