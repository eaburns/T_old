// Copyright © 2015, The T Authors.

package edit

import "github.com/eaburns/T/edit/runes"

// A log holds a record of changes made to a buffer.
// It consists of an unbounded number of entries.
// Each entry has a header and zero or more runes of data.
// The header contains the addr of the change
// in the original unchanged buffer.
// The header also contains the size of the data,
// and a meta fields used for navigating the log.
// The data following the header is a sequence of runes
// which the change uses to replace the runes
// in the string addressed in the header.
type log struct {
	buf *runes.Buffer
	// Last is the offset of the last header in the log.
	last int64
}

func newLog() *log { return &log{buf: runes.NewBuffer(1 << 12)} }

func (l *log) close() error { return l.buf.Close() }

func (l *log) clear() error {
	l.last = 0
	return l.buf.Delete(l.buf.Size(), 0)
}

type header struct {
	// Prev is the offset into the log
	// of the beginning of the previous entry's header.
	// If there is no previous entry, prev is -1.
	prev int64
	// At is the original address that changed.
	at addr
	// Size is the new size of the address after the change.
	// The header is followed by size runes containing
	// the new contents of the changed address.
	size int64
	// Seq is a sequence number that uniqely identifies
	// the edit that made this change.
	seq int32
}

const headerRunes = 9

func (h *header) marshal() []rune {
	var rs [headerRunes]int32
	rs[0] = int32(h.prev >> 32)
	rs[1] = int32(h.prev & 0xFFFFFFFF)
	rs[2] = int32(h.at.from >> 32)
	rs[3] = int32(h.at.from & 0xFFFFFFFF)
	rs[4] = int32(h.at.to >> 32)
	rs[5] = int32(h.at.to & 0xFFFFFFFF)
	rs[6] = int32(h.size >> 32)
	rs[7] = int32(h.size & 0xFFFFFFFF)
	rs[8] = h.seq
	return rs[:]
}

func (h *header) unmarshal(data []rune) {
	if len(data) < headerRunes {
		panic("bad log")
	}
	h.prev = int64(data[0])<<32 | int64(data[1])
	h.at.from = int64(data[2])<<32 | int64(data[3])
	h.at.to = int64(data[4])<<32 | int64(data[5])
	h.size = int64(data[6])<<32 | int64(data[7])
	h.seq = data[8]
}

func (l *log) append(seq int32, at addr, src runes.Reader) error {
	prev := l.last
	l.last = l.buf.Size()
	n, err := runes.Copy(l.buf.Writer(l.last), src)
	if err != nil {
		return err
	}
	// Insert the header before the data.
	h := header{
		prev: prev,
		at:   at,
		size: n,
		seq:  seq,
	}
	return l.buf.Insert(h.marshal(), l.last)
}

type entry struct {
	l *log
	header
	offs int64
	err  error
}

// LogFirst returns the first entry in the log.
// If the log is empty, logFirst returns the dummy end entry.
func logFirst(l *log) entry { return logAt(l, 0) }

// LogLast returns the last entry in the log.
// If the log is empty, logLast returns the dummy end entry.
func logLast(l *log) entry { return logAt(l, l.last) }

// LogAt returns the entry at the given log offset.
// If the log is empty, logAt returns the dummy end entry.
func logAt(l *log, offs int64) entry {
	if l.buf.Size() == 0 {
		return entry{l: l, offs: -1}
	}
	it := entry{l: l, offs: offs}
	it.load()
	return it
}

// End returns whether the entry is the dummy end entry.
func (e entry) end() bool { return e.offs < 0 }

// Next returns the next entry in the log.
// Calling next on the dummy end entry returns
// the first entry in the log.
// On error, the end entry is returned with the error set.
// Calling next on an entry with the error set returns
// the same entry.
func (e entry) next() entry {
	switch {
	case e.err != nil:
		return entry{l: e.l, offs: -1, err: e.err}
	case e.end():
		return logFirst(e.l)
	case e.offs == e.l.last:
		e.offs = -1
	default:
		e.offs += headerRunes + e.size
	}
	e.load()
	return e
}

// Prev returns the previous entry in the log.
// Calling prev on the dummy end entry returns
// the last entry in the log.
// On error, the end entry is returned with the error set.
// Calling prev on an entry with the error set returns
// the same entry.
func (e entry) prev() entry {
	switch {
	case e.err != nil:
		return entry{l: e.l, offs: -1, err: e.err}
	case e.end():
		return logLast(e.l)
	case e.offs == 0:
		e.offs = -1
	default:
		e.offs = e.header.prev
	}
	e.load()
	return e
}

// Load loads the entry's header.
// If the log is empty, loading an entry
// makes the entry the dummy end entry.
func (e *entry) load() {
	if e.err != nil || e.offs < 0 {
		return
	}
	var data []rune
	data, e.err = e.l.buf.Read(headerRunes, e.offs)
	if e.err != nil {
		e.offs = -1
	} else {
		e.header.unmarshal(data)
	}
}

// Store stores the entry's header back to the log.
// Store does nothing on the end entry
// or an entry with its error set
func (e *entry) store() error {
	if e.err != nil || e.offs < 0 {
		return nil
	}
	if err := e.l.buf.Delete(headerRunes, e.offs); err != nil {
		return err
	}
	return e.l.buf.Insert(e.header.marshal(), e.offs)
}

// Data returns the entry's data.
func (e entry) data() runes.Reader {
	if e.err != nil {
		panic("data called on the end iterator")
	}
	from := e.offs + headerRunes
	return runes.LimitReader(e.l.buf.Reader(from), e.size)
}

// Pop removes this entry and all following it from the log.
// Popping the end entry is a no-op.
func (e entry) pop() error {
	if e.end() {
		return nil
	}
	l := e.l
	if p := e.prev(); p.end() {
		l.last = 0
	} else {
		l.last = p.offs
	}
	return e.l.buf.Delete(l.buf.Size()-e.offs, e.offs)
}
