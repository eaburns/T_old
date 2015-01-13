// Package edit provides sam-style editing of rune buffers.
// See sam(1) for an overview: http://swtch.com/plan9port/man/man1/sam.html.
package edit

import (
	"errors"
	"sync"

	"github.com/eaburns/T/runes"
)

// TODO(eaburns): Find a good size.
const blockSize = 1 << 20

// A Buffer is an editable rune buffer.
type Buffer struct {
	runes *runes.Buffer
	lock  sync.Mutex
	eds   []*Editor
}

// NewBuffer returns a new, empty buffer.
func NewBuffer() *Buffer {
	return &Buffer{runes: runes.NewBuffer(blockSize)}
}

// Close closes the buffer and all of its editors.
// After Close is called, the buffer is no longer editable.
func (b *Buffer) Close() error {
	b.lock.Lock()
	defer b.lock.Unlock()
	return b.runes.Close()
}

// Size returns the number of runes in the buffer.
// If the rune is out of range it panics.
// If there is an error reading, it panics a RuneReadError containing the error.
//
// The caller must hold b.lock.
func (b *Buffer) size() int64 { return b.runes.Size() }

// Returns the ith rune in the buffer.
//
// The caller must hold b.lock.
func (b *Buffer) rune(i int64) rune { return b.runes.Rune(i) }

// Change changes the runes in the range.
//
// The caller must hold b.lock.
func (b *Buffer) change(ed *Editor, r addr, rs []rune) error {
	if _, err := b.runes.Delete(r.size(), r.from); err != nil {
		return err
	}
	if _, err := b.runes.Insert(rs, r.from); err != nil {
		return err
	}
	for _, e := range b.eds {
		if e != ed {
			e.update(r, int64(len(rs)))
		}
	}
	return nil
}

// An Editor provides sam-like editing functionality on a buffer of runes.
type Editor struct {
	buf   *Buffer
	marks map[rune]addr
}

// NewEditor returns a new editor that edits the buffer.
func (b *Buffer) NewEditor() *Editor {
	b.lock.Lock()
	defer b.lock.Unlock()

	ed := &Editor{buf: b, marks: make(map[rune]addr, 2)}
	b.eds = append(b.eds, ed)
	return ed
}

// Close closes the editor.
func (ed *Editor) Close() error {
	ed.buf.lock.Lock()
	defer ed.buf.lock.Unlock()

	eds := ed.buf.eds
	for i := range eds {
		if eds[i] == ed {
			ed.buf.eds = append(eds[:i], eds[:i+1]...)
			return nil
		}
	}
	return errors.New("already closed")
}

// Read returns the runes at an address in the buffer
// and sets dot to the address.
func (ed *Editor) Read(a Address) ([]rune, error) {
	ed.buf.lock.Lock()
	defer ed.buf.lock.Unlock()

	r, err := a.addr(ed)
	if err != nil {
		return nil, err
	}
	ed.marks['.'] = r
	rs := make([]rune, r.size())
	if _, err := ed.buf.runes.Read(rs, r.from); err != nil {
		return nil, err
	}
	return rs, nil
}

// Change changes the range to the given runes
// and sets dot to the address of the changed runes.
func (ed *Editor) Change(a Address, rs []rune) error {
	ed.buf.lock.Lock()
	defer ed.buf.lock.Unlock()

	r, err := a.addr(ed)
	if err != nil {
		return err
	}
	if err := ed.buf.change(ed, r, rs); err != nil {
		return err
	}
	ed.marks['.'] = addr{from: r.from, to: r.from + int64(len(rs))}
	return nil
}

// Append inserts the runes after the address
// and sets dot to the address of the appended runes.
func (ed *Editor) Append(ad Address, rs []rune) error {
	return ed.Change(ad.Plus(Rune(0)), rs)
}

// Delete deletes the runes at the address
// and sets dot to the address of the empty string
// where the runes were deleted.
func (ed *Editor) Delete(a Address) error {
	return ed.Change(a, []rune{})
}

// Insert inserts the runes before the address
// and sets dot to the address of the inserted runes.
func (ed *Editor) Insert(ad Address, rs []rune) error {
	return ed.Change(ad.Minus(Rune(0)), rs)
}

// Update updates the Editor's addresses
// given an addr that changed and its new size.
func (ed *Editor) update(r addr, n int64) {
	for m := range ed.marks {
		a := ed.marks[m]
		a.update(r, n)
		ed.marks[m] = a
	}
}
