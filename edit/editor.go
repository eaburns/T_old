// Copyright © 2015, The T Authors.

package edit

import (
	"bufio"
	"errors"
	"io"
	"os"
	"sync"

	"github.com/eaburns/T/edit/runes"
)

// MaxRunes is the maximum number of runes to read into memory.
const MaxRunes = 4096

// A Buffer is an editable rune buffer.
type Buffer struct {
	sync.Mutex
	runes      *runes.Buffer
	eds        []*Editor
	seq        int32
	undo, redo *log
}

// NewBuffer returns a new, empty Buffer.
func NewBuffer() *Buffer {
	return newBuffer(runes.NewBuffer(1 << 12))
}

func newBuffer(rs *runes.Buffer) *Buffer {
	return &Buffer{
		runes: rs,
		undo:  newLog(),
		redo:  newLog(),
	}
}

// Close closes the Buffer.
// After Close is called, the Buffer is no longer editable.
func (buf *Buffer) Close() error {
	buf.Lock()
	defer buf.Unlock()
	errs := []error{
		buf.runes.Close(),
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
	buf     *Buffer
	who     int32
	marks   map[rune]addr
	pending *log
}

// NewEditor returns an Editor that edits the given buffer.
func NewEditor(buf *Buffer) *Editor {
	buf.Lock()
	defer buf.Unlock()
	ed := &Editor{
		buf:     buf,
		marks:   make(map[rune]addr),
		pending: newLog(),
	}
	buf.eds = append(buf.eds, ed)
	return ed
}

// Close closes the editor.
func (ed *Editor) Close() error {
	ed.buf.Lock()
	defer ed.buf.Unlock()

	eds := ed.buf.eds
	for i := range eds {
		if eds[i] == ed {
			ed.buf.eds = append(eds[:i], eds[:i+1]...)
			return ed.pending.close()
		}
	}
	return errors.New("already closed")
}

// ReaderFrom returns an io.ReaderFrom
// that, when it's ReadFrom method is called,
// atomically evaluates the address,
// changes the addressed runes
// to those read from the io.Reader,
// and sets dot to the newly changed runes.
func (ed *Editor) ReaderFrom(a Address) io.ReaderFrom { return readerFrom{a: a, ed: ed} }

type readerFrom struct {
	a  Address
	ed *Editor
}

func (rf readerFrom) ReadFrom(r io.Reader) (int64, error) {
	cr := newCountingRuneReader(r)
	err := rf.ed.do(func() (addr, error) {
		at, err := rf.a.where(rf.ed)
		if err != nil {
			return addr{}, err
		}
		return at, pend(rf.ed, at, runes.RunesReader(cr))
	})
	return cr.n, err
}

type countingRuneReader struct {
	rr io.RuneReader
	n  int64
}

func newCountingRuneReader(r io.Reader) *countingRuneReader {
	if rr, ok := r.(io.RuneReader); ok {
		return &countingRuneReader{rr: rr}
	}
	return &countingRuneReader{rr: bufio.NewReader(r)}
}

func (cr *countingRuneReader) ReadRune() (rune, int, error) {
	r, n, err := cr.rr.ReadRune()
	cr.n += int64(n)
	return r, n, err
}

// WriterTo returns an io.WriterTo
// that, when it's WriteTo method is called,
// atomically evaluates the address,
// writes the addressed runes to an io.Writer,
// and sets dot to the address.
func (ed *Editor) WriterTo(a Address) io.WriterTo { return writerTo{a: a, ed: ed} }

type writerTo struct {
	a  Address
	ed *Editor
}

func (wt writerTo) WriteTo(w io.Writer) (int64, error) {
	cw := countingWriter{w: w}
	err := wt.ed.Do(Print(wt.a), &cw)
	return cw.n, err
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	cw.n += int64(n)
	return n, err
}

// Where returns rune offsets of the address.
func (ed *Editor) Where(a Address) (addr, error) {
	ed.buf.Lock()
	defer ed.buf.Unlock()
	at, err := a.where(ed)
	if err != nil {
		return addr{}, err
	}
	return at, err
}

// Do performs an Edit on the Editor's Buffer.
func (ed *Editor) Do(e Edit, w io.Writer) error {
	switch e := e.(type) {
	case undo:
		return ed.undoRedo(int(e), ed.undo1)
	case redo:
		return ed.undoRedo(int(e), ed.redo1)
	default:
		return ed.do(func() (addr, error) { return e.do(ed, w) })
	}
}

// Do applies changes to an Editor's Buffer.
//
// Changes are applied in two phases:
// Phase one logs the changes without modifying the Buffer.
// Phase two applies the changes to the Buffer.
// The two phases occur with the buffer Lock held.
//
// The f function performs phase one.
// It is called with the Editor's pending log cleared.
// It will typically append changes to the Editor's pending log
// and/or modify the Editor's marks.
// In the case of an error, the marks are restored
// to their values before any changes were made.
//
// The f function must return the address
// over which changes were computed.
// This address is used to compute and set dot
// after the changes are applied.
func (ed *Editor) do(f func() (addr, error)) error {
	ed.buf.Lock()
	defer ed.buf.Unlock()

	marks0 := make(map[rune]addr, len(ed.marks))
	for r, a := range ed.marks {
		marks0[r] = a
	}
	defer func() { ed.marks = marks0 }()

	if err := ed.pending.clear(); err != nil {
		return err
	}
	at, err := f()
	if err != nil {
		return err
	}

	if at, err = fixAddrs(at, ed.pending); err != nil {
		return err
	}

	if err := ed.buf.redo.clear(); err != nil {
		return err
	}

	for e := logFirst(ed.pending); !e.end(); e = e.next() {
		undoAt := addr{from: e.at.from, to: e.at.from + e.size}
		undoSrc := ed.buf.runes.Reader(e.at.from)
		undoSrc = runes.LimitReader(undoSrc, e.at.size())
		if err := ed.buf.undo.append(ed.buf.seq, undoAt, undoSrc); err != nil {
			return err
		}

		if err := ed.buf.change(e.at, e.data()); err != nil {
			// TODO(eaburns): Very bad; what should we do?
			return err
		}
	}

	ed.buf.seq++
	ed.marks['.'] = at
	marks0 = ed.marks
	return err
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

func pend(ed *Editor, at addr, src runes.Reader) error {
	return ed.pending.append(ed.buf.seq, at, src)
}

func (ed *Editor) undoRedo(n int, undoRedo1 func() (addr, error)) (err error) {
	ed.buf.Lock()
	defer ed.buf.Unlock()

	if n <= 0 {
		return os.ErrInvalid
	}

	marks0 := make(map[rune]addr, len(ed.marks))
	for r, a := range ed.marks {
		marks0[r] = a
	}
	defer func() { ed.marks = marks0 }()

	for i := 0; i < n; i++ {
		dot, err := undoRedo1()
		if err != nil {
			return err
		}
		ed.marks['.'] = dot
	}

	ed.buf.seq++
	marks0 = ed.marks
	return nil
}

// Undo1 undoes the most recent
// sequence of changes.
// A sequence of changes is one in which
// all changes have the same seq.
// It returns the address that covers
// all changes in the sequence.
// If nothing is undone, dot is returned.
func (ed *Editor) undo1() (addr, error) {
	e := logLast(ed.buf.undo)
	if e.end() {
		return ed.marks['.'], nil
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
			return addr{}, err
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
			return addr{}, err
		}
		e = e.next()
	}
	return all, start.pop()
}

// Redo1 redoes the most recent
// sequence of changes.
// A sequence of changes is one in which
// all changes have the same seq.
// It returns the address that covers
// all changes in the sequence.
// If nothing is undone, dot is returned.
func (ed *Editor) redo1() (addr, error) {
	e := logLast(ed.buf.redo)
	if e.end() {
		return ed.marks['.'], nil
	}

	all := addr{from: ed.buf.size() + 1, to: -1}
	for {
		undoAt := addr{from: e.at.from, to: e.at.from + e.size}
		undoSrc := ed.buf.runes.Reader(e.at.from)
		undoSrc = runes.LimitReader(undoSrc, e.at.size())
		if err := ed.buf.undo.append(ed.buf.seq, undoAt, undoSrc); err != nil {
			return addr{}, err
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
			return addr{}, err
		}
		if p := e.prev(); p.end() || p.seq != e.seq {
			break
		} else {
			e = p
		}
	}
	return all, e.pop()
}

func (ed *Editor) lines(at addr) (l0, l1 int64, err error) {
	var i int64
	l0 = int64(1) // line numbers are 1 based.
	for ; i < at.from; i++ {
		r, err := ed.buf.rune(i)
		if err != nil {
			return 0, 0, err
		} else if r == '\n' {
			l0++
		}
	}
	l1 = l0
	for ; i < at.to; i++ {
		r, err := ed.buf.rune(i)
		if err != nil {
			return 0, 0, err
		} else if r == '\n' && i < at.to-1 {
			l1++
		}
	}
	return l0, l1, nil
}
