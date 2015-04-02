package edit

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
	buf *runes
	// Last is the offset of the last header in the log.
	last int64
}

func newLog() *log { return &log{buf: newRunes(1 << 12)} }

func (l *log) close() error { return l.buf.close() }

func (l *log) clear() error {
	l.last = 0
	return l.buf.delete(l.buf.Size(), 0)
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
	// Who is a unique identifier
	// of the Editor that made the change.
	who int32
}

const headerRunes = 10

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
	rs[9] = h.who
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
	h.who = data[9]
}

func (l *log) append(seq, who int32, at addr, src source) error {
	h := header{
		prev: l.last,
		at:   at,
		size: src.size(),
		seq:  seq,
		who:  who,
	}
	l.last = l.buf.Size()
	if err := l.buf.insert(h.marshal(), l.last); err != nil {
		return err
	}
	return src.insert(l.buf, l.buf.Size())
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
	var data [headerRunes]rune
	if e.err = e.l.buf.read(data[:], e.offs); e.err != nil {
		e.offs = -1
	} else {
		e.header.unmarshal(data[:])
	}
}

// Store stores the entry's header back to the log.
// Store does nothing on the end entry
// or an entry with its error set
func (e *entry) store() error {
	if e.err != nil || e.offs < 0 {
		return nil
	}
	if err := e.l.buf.delete(headerRunes, e.offs); err != nil {
		return err
	}
	return e.l.buf.insert(e.header.marshal(), e.offs)
}

// Data returns a source for the entry's data.
func (e entry) data() source {
	if e.err != nil {
		panic("data called on the end iterator")
	}
	from := e.offs + headerRunes
	return &bufferSource{
		at:  addr{from: from, to: from + e.size},
		buf: e.l.buf,
	}
}
