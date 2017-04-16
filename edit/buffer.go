// Copyright © 2015, The T Authors.

package edit

import (
	"bufio"
	"io"
	"unicode/utf8"

	"github.com/eaburns/T/edit/runes"
)

// A Buffer implements the Editor interface,
// editing an unbounded sequence of runes.
type Buffer struct {
	runes               *runes.Buffer
	pending, undo, redo *log
	seq                 int32
	marks               map[rune]Span
}

// NewBuffer returns a new, empty Buffer.
func NewBuffer() *Buffer { return newBuffer(runes.NewBuffer(1 << 12)) }

func newBuffer(rs *runes.Buffer) *Buffer {
	return &Buffer{
		runes:   rs,
		undo:    newLog(),
		redo:    newLog(),
		pending: newLog(),
		marks:   make(map[rune]Span),
	}
}

// Close closes the Buffer and releases its resources.
func (buf *Buffer) Close() error {
	errs := []error{
		buf.runes.Close(),
		buf.pending.close(),
		buf.undo.close(),
		buf.redo.close(),
	}
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// Change changes the string identified by at
// to contain the runes from the Reader.
//
// This method must be called with the Lock held.
func (buf *Buffer) change(s Span, src runes.Reader) error {
	if err := buf.runes.Delete(s.Size(), s[0]); err != nil {
		return err
	}
	n, err := runes.Copy(buf.runes.Writer(s[0]), src)
	if err != nil {
		return err
	}
	for m := range buf.marks {
		buf.marks[m] = buf.marks[m].Update(s, n)
	}
	return nil
}

// Size implements the Size method of the Text interface.
//
// It returns the number of Runes in the Buffer.
func (buf *Buffer) Size() int64 { return buf.runes.Size() }

func (buf *Buffer) Mark(m rune) Span { return buf.marks[m] }

func (buf *Buffer) SetMark(m rune, s Span) error {
	if size := buf.Size(); s[0] < 0 || s[1] < 0 || s[0] > size || s[1] > size {
		return ErrInvalidArgument
	}
	buf.marks[m] = s
	return nil
}

type runeReader struct {
	span   Span
	buffer *Buffer
}

func (rr *runeReader) ReadRune() (r rune, w int, err error) {
	switch size := rr.span.Size(); {
	case size == 0:
		return 0, 0, io.EOF
	case size < 0:
		rr.span[0]--
		r, err = rr.buffer.runes.Rune(rr.span[0])
	default:
		r, err = rr.buffer.runes.Rune(rr.span[0])
		rr.span[0]++
	}
	return r, 1, err
}

type badRange struct{}

func (badRange) Read([]byte) (int, error)     { return 0, ErrInvalidArgument }
func (badRange) ReadRune() (rune, int, error) { return 0, 0, ErrInvalidArgument }

// RuneReader implements the Runes method of the Text interface.
//
// Each non-error ReadRune operation returns a width of 1.
func (buf *Buffer) RuneReader(s Span) io.RuneReader {
	if size := buf.Size(); s[0] < 0 || s[1] < 0 || s[0] > size || s[1] > size {
		return badRange{}
	}
	return &runeReader{span: s, buffer: buf}
}

func (buf *Buffer) Reader(s Span) io.Reader {
	if size := buf.Size(); s[0] < 0 || s[1] < 0 || s[0] > size || s[1] > size {
		return badRange{}
	}
	rr := runes.LimitReader(buf.runes.Reader(s[0]), s.Size())
	return runes.UTF8Reader(rr)
}

func (buf *Buffer) Change(s Span, r io.Reader) (n int64, err error) {
	if prev := logLast(buf.pending); !prev.end() && s[0] < prev.span[1] {
		err = ErrOutOfSequence
	} else {
		rr := runes.RunesReader(bufio.NewReader(r))
		n, err = buf.pending.append(buf.seq, s, rr)
	}
	if err != nil {
		buf.pending.reset()
	}
	return n, err
}

func (buf *Buffer) Apply() error {
	for e := logFirst(buf.pending); !e.end(); e = e.next() {
		undoSpan := Span{e.span[0], e.span[0] + e.size}
		undoSrc := buf.runes.Reader(e.span[0])
		undoSrc = runes.LimitReader(undoSrc, e.span.Size())
		if _, err := buf.undo.append(buf.seq, undoSpan, undoSrc); err != nil {
			return err
		}
	}
	dot := buf.marks['.']
	for e := logFirst(buf.pending); !e.end(); e = e.next() {
		if e.span[0] == dot[0] {
			// If they have the same start, grow dot.
			// Otherwise, update would simply leave it
			// as a point address and move it.
			dot[1] = dot.Update(e.span, e.size)[1]
		} else {
			dot = dot.Update(e.span, e.size)
		}
		for f := e.next(); !f.end(); f = f.next() {
			f.span = f.span.Update(e.span, e.size)
			if err := f.store(); err != nil {
				return err
			}
		}

		if err := buf.change(e.span, e.data()); err != nil {
			return err
		}
	}
	buf.pending.reset()
	buf.redo.reset()
	buf.marks['.'] = dot
	buf.seq++
	return nil
}

func (buf *Buffer) Undo() error {
	marks0 := make(map[rune]Span, len(buf.marks))
	for r, s := range buf.marks {
		marks0[r] = s
	}
	defer func() { buf.marks = marks0 }()

	all := Span{-1, 0}
	start := logLastFrame(buf.undo)
	if start.end() {
		return nil
	}
	for e := start; !e.end(); e = e.next() {
		redoSpan := Span{e.span[0], e.span[0] + e.size}
		redoSrc := buf.runes.Reader(e.span[0])
		redoSrc = runes.LimitReader(redoSrc, e.span.Size())
		if _, err := buf.redo.append(buf.seq, redoSpan, redoSrc); err != nil {
			return err
		}

		if all[0] < 0 {
			all[0] = e.span[0]
		}
		all[1] = e.span[0] + e.size
		if err := buf.change(e.span, e.data()); err != nil {
			return err
		}
	}
	buf.marks['.'] = all
	marks0 = buf.marks
	buf.seq++
	return start.pop()
}

func (buf *Buffer) Redo() error {
	marks0 := make(map[rune]Span, len(buf.marks))
	for r, s := range buf.marks {
		marks0[r] = s
	}
	defer func() { buf.marks = marks0 }()

	start := logLastFrame(buf.redo)
	if start.end() {
		return nil
	}
	for e := start; !e.end(); e = e.next() {
		undoSpan := Span{e.span[0], e.span[0] + e.size}
		undoSrc := buf.runes.Reader(e.span[0])
		undoSrc = runes.LimitReader(undoSrc, e.span.Size())
		if _, err := buf.undo.append(buf.seq, undoSpan, undoSrc); err != nil {
			return err
		}
	}

	all := Span{0, -1}
	for e := logLast(buf.redo); e != start.prev(); e = e.prev() {
		all[0] = e.span[0]
		if all[1] < 0 {
			all[1] = e.span[0] + e.size
		} else {
			all[1] += e.size - e.span.Size()
		}
		if err := buf.change(e.span, e.data()); err != nil {
			return err
		}
	}

	buf.marks['.'] = all
	marks0 = buf.marks
	buf.seq++
	return start.pop()
}

// A log holds a record of changes made to a buffer.
// It consists of an unbounded number of entries.
// Each entry has a header and zero or more runes of data.
// The header contains the Span of the change
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

func (l *log) reset() {
	l.last = 0
	l.buf.Reset()
}

type header struct {
	// Prev is the offset into the log
	// of the beginning of the previous entry's header.
	// If there is no previous entry, prev is -1.
	prev int64
	// Span is the original Span that changed.
	span Span
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
	rs[2] = int32(h.span[0] >> 32)
	rs[3] = int32(h.span[0] & 0xFFFFFFFF)
	rs[4] = int32(h.span[1] >> 32)
	rs[5] = int32(h.span[1] & 0xFFFFFFFF)
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
	h.span[0] = int64(data[2])<<32 | int64(data[3])
	h.span[1] = int64(data[4])<<32 | int64(data[5])
	h.size = int64(data[6])<<32 | int64(data[7])
	h.seq = data[8]
}

func (l *log) append(seq int32, s Span, src runes.Reader) (int64, error) {
	prev := l.last
	l.last = l.buf.Size()
	n, err := runes.Copy(l.buf.Writer(l.last), src)
	if err != nil {
		return n, err
	}
	// Insert the header before the data.
	h := header{
		prev: prev,
		span: s,
		size: n,
		seq:  seq,
	}
	return n, l.buf.Insert(h.marshal(), l.last)
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

// LogLastFrame returns the start of the last frame on the log.
// A frame is a sequence of log entries with the same seq.
// If the log is empty, logLastFrame returns the dummy end entry.
func logLastFrame(l *log) entry {
	e := logLast(l)
	for !e.end() {
		prev := e.prev()
		if prev.end() || prev.seq != e.seq {
			break
		}
		e = prev
	}
	return e
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

// Put writes the UTF8 encoding of the buffer to the io.Writer.
// The return value is the number of bytes written.
func (b *Buffer) Put(w io.Writer) (int, error) {
	const n = 512
	var tot int
	var at Address
	for at.From < b.Size() {
		at.To = at.From + blockRunes
		if at.To > b.Size() {
			at.To = b.Size()
		}
		rs, err := b.Read(at)
		if err != nil {
			return tot, err
		}

		var o int
		var bs [utf8.UTFMax * n]byte
		for _, r := range rs {
			o += utf8.EncodeRune(bs[o:], r)
		}
		m, err := w.Write(bs[:o])
		tot += m
		if err != nil {
			return tot, err
		}
		at.From = at.To
	}
	return tot, nil
}
