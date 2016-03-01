// Copyright © 2015, The T Authors.

package edit

import (
	"bufio"
	"errors"
	"io"
	"sync"

	"github.com/eaburns/T/edit/runes"
)

// MaxRunes is the maximum number of runes to read into memory.
const MaxRunes = 4096

// A Buffer is an editable rune buffer.
type Buffer struct {
	sync.Mutex
	runes               *runes.Buffer
	eds                 []*Editor
	seq                 int32
	pending, undo, redo *log
}

// NewBuffer returns a new, empty Buffer.
func NewBuffer() *Buffer {
	return newBuffer(runes.NewBuffer(1 << 12))
}

func newBuffer(rs *runes.Buffer) *Buffer {
	return &Buffer{
		runes:   rs,
		undo:    newLog(),
		redo:    newLog(),
		pending: newLog(),
	}
}

// Close closes the Buffer and any Editors editing it.
// After Close is called, the Buffer is no longer editable.
func (buf *Buffer) Close() error {
	buf.Lock()
	defer buf.Unlock()
	errs := []error{
		buf.runes.Close(),
		buf.pending.close(),
		buf.undo.close(),
		buf.redo.close(),
	}
	for len(buf.eds) > 0 {
		errs = append(errs, buf.eds[0].close())
	}
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// Size returns the number of runes in the Buffer.
//
// This method must be called with the Lock held.
func (buf *Buffer) size() int64 { return buf.runes.Size() }

// Rune returns the ith rune in the Buffer.
//
// This method must be called with the Lock held.
func (buf *Buffer) rune(i int64) (rune, error) { return buf.runes.Rune(i) }

// Change changes the string identified by at
// to contain the runes from the Reader.
//
// This method must be called with the Lock held.
func (buf *Buffer) change(at addr, src runes.Reader) error {
	if err := buf.runes.Delete(at.size(), at.from); err != nil {
		return err
	}
	n, err := runes.Copy(buf.runes.Writer(at.from), src)
	if err != nil {
		return err
	}
	for _, ed := range buf.eds {
		for m := range ed.marks {
			ed.marks[m] = ed.marks[m].update(at, n)
		}
	}
	return nil
}

// An Editor edits a Buffer of runes.
type Editor struct {
	buf   *Buffer
	marks map[rune]addr
}

// NewEditor returns an Editor that edits the given buffer.
func NewEditor(buf *Buffer) *Editor {
	buf.Lock()
	defer buf.Unlock()
	ed := &Editor{
		buf:   buf,
		marks: make(map[rune]addr),
	}
	buf.eds = append(buf.eds, ed)
	return ed
}

// Close closes the editor.
func (ed *Editor) Close() error {
	ed.buf.Lock()
	defer ed.buf.Unlock()
	return ed.close()
}

func (ed *Editor) close() error {
	for i := range ed.buf.eds {
		if ed.buf.eds[i] == ed {
			ed.buf.eds = append(ed.buf.eds[:i], ed.buf.eds[i+1:]...)
			return nil
		}
	}
	return errors.New("already closed")
}

// Do performs an Edit on the Editor's Buffer.
func (ed *Editor) Do(e Edit, print io.Writer) error { return e.Do(ed, print) }

func pend(ed *Editor, at addr, src runes.Reader) error {
	return ed.Change(Span{at.from, at.to}, runes.UTF8Reader(src))
}

// A addr identifies a substring within a buffer
// by its inclusive start offset and its exclusive end offset.
type addr struct{ from, to int64 }

// Size returns the number of runes in
// the string identified by the range.
func (a addr) size() int64 { return a.to - a.from }

// Update returns a, updated to account for b changing to size n.
func (a addr) update(b addr, n int64) addr {
	// Clip, unless b is entirely within a.
	if a.from >= b.from || b.to > a.to {
		if b.contains(a.from) {
			a.from = b.to
		}
		if b.contains(a.to - 1) {
			a.to = b.from
		}
		if a.from > a.to {
			a.to = a.from
		}
	}

	// Move.
	d := n - b.size()
	if a.to >= b.to {
		a.to += d
	}
	if a.from >= b.to {
		a.from += d
	}
	return a
}

func (a addr) contains(p int64) bool { return a.from <= p && p < a.to }

// BUG(eaburns): This is temporary to ensure that Editor implements editor.
// Remove it once Editor goes away.
var _ editor = &Editor{}

// Size implements the Size method of the Text interface.
//
// It returns the number of Runes in the Buffer.
func (ed *Editor) Size() int64 { return ed.buf.size() }

func (ed *Editor) Mark(m rune) Span {
	at := ed.marks[m]
	return Span{at.from, at.to}
}

func (ed *Editor) SetMark(m rune, s Span) error {
	if size := ed.Size(); s[0] < 0 || s[1] < 0 || s[0] > size || s[1] > size {
		return ErrMarkRange
	}
	ed.marks[m] = addr{from: s[0], to: s[1]}
	return nil
}

type runeReader struct {
	span   Span
	editor *Editor
}

func (rr *runeReader) ReadRune() (r rune, w int, err error) {
	switch size := rr.span.Size(); {
	case size == 0:
		return 0, 0, io.EOF
	case size < 0:
		rr.span[0]--
		r, err = rr.editor.buf.rune(rr.span[0])
	default:
		r, err = rr.editor.buf.rune(rr.span[0])
		rr.span[0]++
	}
	return r, 1, err
}

// RuneReader implements the Runes method of the Text interface.
//
// Each non-error ReadRune operation returns a width of 1.
func (ed *Editor) RuneReader(span Span) io.RuneReader {
	return &runeReader{span: span, editor: ed}
}

func (ed *Editor) Reader(span Span) io.Reader {
	rr := ed.buf.runes.Reader(span[0])
	rr = runes.LimitReader(rr, span.Size())
	return runes.UTF8Reader(rr)
}

func (ed *Editor) Change(s Span, r io.Reader) error {
	rr := runes.RunesReader(bufio.NewReader(r))
	return ed.buf.pending.append(ed.buf.seq, addr{from: s[0], to: s[1]}, rr)
}

func (ed *Editor) Apply() error {
	dot, err := fixAddrs(ed.marks['.'], ed.buf.pending)
	if err != nil {
		return err
	}

	if err := ed.buf.redo.clear(); err != nil {
		return err
	}
	for e := logFirst(ed.buf.pending); !e.end(); e = e.next() {
		undoAt := addr{from: e.at.from, to: e.at.from + e.size}
		undoSrc := ed.buf.runes.Reader(e.at.from)
		undoSrc = runes.LimitReader(undoSrc, e.at.size())
		if err := ed.buf.undo.append(ed.buf.seq, undoAt, undoSrc); err != nil {
			// TODO(eaburns): Very bad; what should we do?
			return err
		}
		if err := ed.buf.change(e.at, e.data()); err != nil {
			// TODO(eaburns): Very bad; what should we do?
			return err
		}
	}
	if err := ed.buf.pending.clear(); err != nil {
		// TODO(eaburns): Very bad; what should we do?
		return err
	}

	ed.marks['.'] = dot
	ed.buf.seq++
	return nil
}

func fixAddrs(at addr, l *log) (addr, error) {
	if !inSequence(l) {
		return addr{}, errors.New("changes not in sequence")
	}
	for e := logFirst(l); !e.end(); e = e.next() {
		if e.at.from == at.from {
			// If they have the same from, grow at.
			// This grows at, even if it's a point address,
			// to include the change made by e.
			// Otherwise, update would simply leave it
			// as a point address and move it.
			at.to = at.update(e.at, e.size).to
		} else {
			at = at.update(e.at, e.size)
		}
		for f := e.next(); !f.end(); f = f.next() {
			f.at = f.at.update(e.at, e.size)
			if err := f.store(); err != nil {
				return addr{}, err
			}
		}
	}
	return at, nil
}

func inSequence(l *log) bool {
	e := logFirst(l)
	for !e.end() {
		f := e.next()
		if f.at != e.at && f.at.from < e.at.to {
			return false
		}
		e = f
	}
	return true
}

func (ed *Editor) Cancel() error { return ed.buf.pending.clear() }

func (ed *Editor) Undo() error {
	marks0 := make(map[rune]addr, len(ed.marks))
	for r, a := range ed.marks {
		marks0[r] = a
	}
	defer func() { ed.marks = marks0 }()

	e := logLast(ed.buf.undo)
	if e.end() {
		return nil
	}
	for {
		prev := e.prev()
		if prev.end() || prev.seq != e.seq {
			break
		}
		e = prev
	}

	all := addr{from: ed.buf.size() + 1, to: -1}
	start := e
	for !e.end() {
		redoAt := addr{from: e.at.from, to: e.at.from + e.size}
		redoSrc := ed.buf.runes.Reader(e.at.from)
		redoSrc = runes.LimitReader(redoSrc, e.at.size())
		if err := ed.buf.redo.append(ed.buf.seq, redoAt, redoSrc); err != nil {
			return err
		}

		// There is no need to call all.update,
		// because changes always apply
		// in sequence of increasing addresses.
		if e.at.from < all.from {
			all.from = e.at.from
		}
		if to := e.at.from + e.size; to > all.to {
			all.to = to
		}

		if err := ed.buf.change(e.at, e.data()); err != nil {
			return err
		}
		e = e.next()
	}
	ed.marks['.'] = all
	marks0 = ed.marks
	ed.buf.seq++
	return start.pop()
}

func (ed *Editor) Redo() error {
	marks0 := make(map[rune]addr, len(ed.marks))
	for r, a := range ed.marks {
		marks0[r] = a
	}
	defer func() { ed.marks = marks0 }()

	e := logLast(ed.buf.redo)
	if e.end() {
		return nil
	}

	all := addr{from: ed.buf.size() + 1, to: -1}
	for {
		undoAt := addr{from: e.at.from, to: e.at.from + e.size}
		undoSrc := ed.buf.runes.Reader(e.at.from)
		undoSrc = runes.LimitReader(undoSrc, e.at.size())
		if err := ed.buf.undo.append(ed.buf.seq, undoAt, undoSrc); err != nil {
			return err
		}

		// There is no need to call all.update,
		// because changes always apply
		// in sequence of increasing addresses.
		if e.at.from < all.from {
			all.from = e.at.from
		}
		if to := e.at.from + e.size; to > all.to {
			all.to = to
		}

		if err := ed.buf.change(e.at, e.data()); err != nil {
			return err
		}
		if p := e.prev(); p.end() || p.seq != e.seq {
			break
		} else {
			e = p
		}
	}
	ed.marks['.'] = all
	marks0 = ed.marks
	ed.buf.seq++
	return e.pop()
}
