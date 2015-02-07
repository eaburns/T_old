// Package edit provides sam-style editing of rune buffers.
// See sam(1) for an overview: http://swtch.com/plan9port/man/man1/sam.html.
package edit

import (
	"errors"
	"sync"
	"unicode"

	"github.com/eaburns/T/re1"
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

// Substitute substitutes text for the first match
// of the regular expression in the addressed range.
// When substituting, a backslash followed by a digit d
// stands for the string that matched the d-th subexpression.
// \n is a literal newline.
// If g is true then all matches in the address range are substituted.
// Dot is set to the modified address.
func (ed *Editor) Substitute(a Address, re *re1.Regexp, sub []rune, g bool) error {
	ed.buf.lock.Lock()
	defer ed.buf.lock.Unlock()

	r, err := a.addr(ed)
	if err != nil {
		return err
	}
	r0 := r

	for {
		d, m, err := ed.sub1(r, re, sub)
		if err != nil {
			return err
		}
		if m == nil {
			break
		}
		r0.to += d
		r.to += d
		r.from = m[0][1] + d
		if !g {
			break
		}
	}
	ed.marks['.'] = r0
	return nil
}

func (ed *Editor) sub1(r addr, re *re1.Regexp, sub []rune) (int64, [][2]int64, error) {
	m, err := ed.subMatch(r, re)
	if err != nil || m == nil {
		return 0, m, err
	}
	repl, err := ed.subText(m, sub)
	if err != nil {
		return 0, nil, err
	}
	if err := ed.buf.change(addr{m[0][0], m[0][1]}, repl); err != nil {
		return 0, nil, err
	}
	return int64(len(repl)) - (m[0][1] - m[0][0]), m, err
}

func (ed *Editor) subText(m [][2]int64, sub []rune) ([]rune, error) {
	var rs []rune
	for i := 0; i < len(sub); i++ {
		d := escDigit(sub[i:])
		if d < 0 {
			rs = append(rs, sub[i])
			continue
		}
		sub, err := ed.subExpr(m, d)
		if err != nil {
			return nil, err
		}
		rs = append(rs, sub...)
		i++
	}
	return rs, nil
}

// EscDigit returns the digit from \[0-9]
// or -1 if the text does not represent an escaped digit.
func escDigit(sub []rune) int {
	if len(sub) >= 2 && sub[0] == '\\' && unicode.IsDigit(sub[1]) {
		return int(sub[1] - '0')
	}
	return -1
}

func (ed *Editor) subExpr(m [][2]int64, n int) ([]rune, error) {
	if n >= len(m) {
		return nil, nil
	}
	rs := make([]rune, m[n][1]-m[n][0])
	if _, err := ed.buf.runes.Read(rs, m[n][0]); err != nil {
		return nil, err
	}
	return rs, nil
}

type runeSlice struct {
	buf *runes.Buffer
	addr
	err error
}

func (rs *runeSlice) Size() int64 { return rs.size() }

func (rs *runeSlice) Rune(i int64) rune {
	switch {
	case i < 0 || i >= rs.size():
		panic("index out of bounds")
	case rs.err != nil:
		return -1
	}
	r, err := rs.buf.Rune(rs.from + i)
	if err != nil {
		rs.err = err
		return -1
	}
	return r
}

func (ed *Editor) subMatch(r addr, re *re1.Regexp) ([][2]int64, error) {
	rs := &runeSlice{buf: ed.buf.runes, addr: r}
	m := re.Match(rs, 0)
	for i := range m {
		m[i][0] += r.from
		m[i][1] += r.from
	}
	return m, rs.err
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
//	{addr} s/regexp/text/
//		Substitute substitutes text for the first match
// 		of the regular expression in the addressed range.
// 		When substituting, a backslash followed by a digit d
// 		stands for the string that matched the d-th subexpression.
//		\n is a literal newline.
// 		If the delimiter after the text is followed by the letter g
//		then all matches in the address range are substituted.
//		If an address is not supplied, dot is used.
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
	case 's':
		re, err := re1.Compile(cmd[1:], re1.Options{Delimited: true})
		if err != nil {
			return err
		}
		exp := re.Expression()
		cmd = cmd[1+len(exp):]
		sub := parseDelimited(exp[0], true, cmd)
		g := len(sub) < len(cmd)-1 && cmd[len(sub)+1] == 'g'
		return ed.Substitute(addr, re, sub, g)
	default:
		return errors.New("unknown command: " + string(c))
	}
}

func parseText(cmd []rune) []rune {
	switch {
	case len(cmd) > 0 && cmd[0] == '\n':
		return parseLines(cmd)
	case len(cmd) > 0:
		return parseDelimited(cmd[0], false, cmd[1:])
	default:
		return nil
	}
}

func parseLines(cmd []rune) []rune {
	var rs []rune
	for i := 1; i < len(cmd); i++ {
		rs = append(rs, cmd[i])
		if i < len(cmd)-1 && cmd[i] == '\n' && cmd[i+1] == '.' {
			break
		}
	}
	return rs
}

func parseDelimited(d rune, digits bool, cmd []rune) []rune {
	var rs []rune
	for i := 0; i < len(cmd) && cmd[i] != d; i++ {
		if cmd[i] == '\\' && i < len(cmd)-1 {
			if digits && unicode.IsDigit(cmd[i+1]) {
				rs = append(rs, '\\')
			}
			i++
			if cmd[i] == 'n' {
				cmd[i] = '\n'
			}
		}
		rs = append(rs, cmd[i])
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
