// Package edit provides sam-style editing of rune buffers.
// See sam(1) for an overview: http://swtch.com/plan9port/man/man1/sam.html.
package edit

import (
	"errors"
	"sync"
	"unicode"

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
	return newBufferRunes(runes.NewBuffer(blockSize))
}

// NewBufferRunes is like NewBuffer, but the buffer uses the given runes.
func newBufferRunes(r *runes.Buffer) *Buffer {
	return &Buffer{runes: r}
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
func (b *Buffer) rune(i int64) (rune, error) { return b.runes.Rune(i) }

// Change changes the runes in the range.
//
// The caller must hold b.lock.
func (b *Buffer) change(r addr, rs []rune) error {
	if _, err := b.runes.Delete(r.size(), r.from); err != nil {
		return err
	}
	if _, err := b.runes.Insert(rs, r.from); err != nil {
		return err
	}
	for _, e := range b.eds {
		e.update(r, int64(len(rs)))
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
	if err := ed.buf.change(r, rs); err != nil {
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

// Mark sets a mark to an address.
// The mark must be either a lower-case or upper-case letter or dot: [a-zA-Z.].
// Any other mark is an error.
// If the mark is . then dot is set to the address.
// Otherwise the named mark is set to the address.
func (ed *Editor) Mark(a Address, m rune) error {
	if !isMarkRune(m) && m != '.' {
		return errors.New("bad mark: " + string(m))
	}
	r, err := a.addr(ed)
	if err != nil {
		return err
	}
	ed.marks[m] = r
	return nil
}

// Edit parses a command and performs its edit on the buffer.
//
// In the following, text surrounded by / represents delimited text.
// The delimiter can be any character, it need not be /.
// Trailing delimiters may be elided, but the opening delimiter must be present.
// In delimited text, \ is an escape; the following character is interpreted literally,
// except \n which represents a literal newline.
// Items in {} are optional.
//
// Commands are:
// 	{addr} a/text/
//	or
//	{addr} a
//	lines of text
//	.	Appends after the addressed text.
//		If an address is not supplied, dot is used.
//	c
//	i	Just like a, but c changes the addressed text
//		and i inserts before the addressed text.
//	{addr} d
//		Deletes the addressed text.
//		If an address is not supplied, dot is used.
//	{addr} m {[a-zA-Z]}
//		Sets the named mark to the address.
//		If an address is not supplied, dot is used.
//		If a mark name is not given, dot is set.
func (ed *Editor) Edit(cmd []rune) error {
	addr, n, err := Addr(cmd)
	switch {
	case err != nil:
		return err
	case addr == nil:
		addr = Dot
	case len(cmd) == n:
		return errors.New("missing command")
	default:
		cmd = cmd[n:]
	}
	switch c := cmd[0]; c {
	case 'a':
		return ed.Append(addr, parseText(cmd[1:]))
	case 'c':
		return ed.Change(addr, parseText(cmd[1:]))
	case 'i':
		return ed.Insert(addr, parseText(cmd[1:]))
	case 'd':
		return ed.Delete(addr)
	case 'm':
		return ed.Mark(addr, parseMarkRune(cmd[1:]))
	default:
		return errors.New("unknown command: " + string(c))
	}
}

func parseText(cmd []rune) []rune {
	var rs []rune
	if len(cmd) > 0 && cmd[0] == '\n' {
		for i := 1; i < len(cmd); i++ {
			rs = append(rs, cmd[i])
			if i < len(cmd)-1 && cmd[i] == '\n' && cmd[i+1] == '.' {
				break
			}
		}
	} else if len(cmd) > 0 {
		d := cmd[0]
		for i := 1; i < len(cmd) && cmd[i] != d; i++ {
			if cmd[i] == '\\' && i < len(cmd)-1 {
				i++
				if cmd[i] == 'n' {
					cmd[i] = '\n'
				}
			}
			rs = append(rs, cmd[i])
		}
	}
	return rs
}

func parseMarkRune(cmd []rune) rune {
	var i int
	for ; i < len(cmd) && unicode.IsSpace(cmd[i]); i++ {
	}
	if i < len(cmd) && isMarkRune(cmd[i]) {
		return cmd[i]
	}
	return '.'
}

// Update updates the Editor's addresses
// given an addr that changed and its new size.
func (ed *Editor) update(r addr, n int64) {
	for m := range ed.marks {
		ed.marks[m] = ed.marks[m].update(r, n)
	}
}
