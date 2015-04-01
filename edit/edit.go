// Package edit provides sam-style editing of rune buffers.
// See sam(1) for an overview: http://swtch.com/plan9port/man/man1/sam.html.
package edit

import (
	"errors"
	"fmt"
	"sync"
	"unicode"

	"github.com/eaburns/T/re1"
)

// A Buffer is an editable rune buffer.
type Buffer struct {
	lock     sync.RWMutex
	runes    *runes
	eds      []*Editor
	seq, who int32
}

// NewBuffer returns a new, empty Buffer.
func NewBuffer() *Buffer {
	return newBuffer(newRunes(1 << 12))
}

func newBuffer(rs *runes) *Buffer { return &Buffer{runes: rs} }

// Close closes the Buffer.
// After Close is called, the Buffer is no longer editable.
func (buf *Buffer) Close() error {
	buf.lock.Lock()
	defer buf.lock.Unlock()
	return buf.runes.close()
}

// Size returns the number of runes in the Buffer.
//
// This method must be called with the RLock held.
func (buf *Buffer) size() int64 { return buf.runes.Size() }

// Rune returns the ith rune in the Buffer.
//
// This method must be called with the RLock held.
func (buf *Buffer) rune(i int64) (rune, error) { return buf.runes.Rune(i) }

// Change changes the string identified by at
// to contain the runes from the source.
//
// This method must be called with the Lock held.
func (buf *Buffer) change(at addr, rs source) error {
	if err := buf.runes.delete(at.size(), at.from); err != nil {
		return err
	}
	if err := rs.insert(buf.runes, at.from); err != nil {
		return err
	}
	for _, ed := range buf.eds {
		for m := range ed.marks {
			ed.marks[m] = ed.marks[m].update(at, rs.size())
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
	buf.lock.Lock()
	defer buf.lock.Unlock()
	ed := &Editor{
		buf:     buf,
		who:     buf.who,
		marks:   make(map[rune]addr),
		pending: newLog(),
	}
	buf.who++
	buf.eds = append(buf.eds, ed)
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
			return ed.pending.close()
		}
	}
	return errors.New("already closed")
}

// Do applies changes to an Editor's Buffer.
//
// Changes are applied in two phases:
// Phase one logs the changes without modifying the Buffer.
// Phase two applies the changes to the Buffer.
// If the Buffer is modified between phases one and two,
// no changes are applied, and the proceedure restarts
// from phase one.
//
// The f function performs phase one.
// It is called with the Buffer's RLock held
// and the Editor's pending log cleared.
// f appends the desired changes to the Editor's pending log
// and returns the address over which they were computed.
// The returned address is used to compute and set dot
// after the change is applied.
// In the face of retries, f is called multiple times,
// so it must be idempotent.
func (ed *Editor) do(f func() (addr, error)) error {
retry:
	marks := make(map[rune]addr, len(ed.marks))
	for r, a := range ed.marks {
		marks[r] = a
	}
	seq, at, err := pend(ed, f)
	if at, err = fixAddrs(at, ed.pending); err != nil {
		return err
	}
	switch retry, err := apply(ed, seq); {
	case err != nil:
		return err
	case retry:
		ed.marks = marks
		goto retry
	}
	ed.marks['.'] = at
	return err
}

func pend(ed *Editor, f func() (addr, error)) (int32, addr, error) {
	if err := ed.pending.clear(); err != nil {
		return 0, addr{}, err
	}

	ed.buf.lock.RLock()
	defer ed.buf.lock.RUnlock()
	seq := ed.buf.seq
	at, err := f()
	return seq, at, err
}

func apply(ed *Editor, seq int32) (bool, error) {
	ed.buf.lock.Lock()
	defer ed.buf.lock.Unlock()
	if ed.buf.seq != seq {
		return true, nil
	}
	for e := logFirst(ed.pending); !e.end(); e = e.next() {
		if err := ed.buf.change(e.at, e.data()); err != nil {
			// TODO(eaburns): Very bad; what should we do?
			return false, err
		}
	}
	ed.buf.seq++
	return false, nil
}

func fixAddrs(at addr, l *log) (addr, error) {
	if !inSequence(l) {
		return addr{}, errors.New("changes not in sequence")
	}
	for e := logFirst(l); !e.end(); e = e.next() {
		if e.at.from < at.from || e.at.to > at.to {
			panic("change not contained in address")
		}
		at.to = at.update(e.at, e.data().size()).to
		for f := e.next(); !f.end(); f = f.next() {
			f.at = f.at.update(e.at, e.data().size())
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

// Change changes the Address to the given runes
// and sets dot to the Address of the changed runes.
func (ed *Editor) Change(a Address, rs []rune) error {
	return ed.do(func() (addr, error) { return change(ed, a, rs) })
}

func change(ed *Editor, a Address, rs []rune) (addr, error) {
	at, err := a.addr(ed)
	if err != nil {
		return addr{}, err
	}
	err = ed.pending.append(ed.buf.seq, ed.who, at, sliceSource(rs))
	return at, err
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
	ed.buf.lock.RLock()
	defer ed.buf.lock.RUnlock()
	_, err := mark(ed, a, m)
	return err
}

func mark(ed *Editor, a Address, m rune) (addr, error) {
	if !isMarkRune(m) && m != '.' {
		return addr{}, errors.New("bad mark: " + string(m))
	}
	at, err := a.addr(ed)
	if err != nil {
		return addr{}, err
	}
	ed.marks[m] = at
	return at, nil
}

// Print returns the runes identified by the address.
// Dot is set to the address.
func (ed *Editor) Print(a Address) ([]rune, error) {
	ed.buf.lock.RLock()
	defer ed.buf.lock.RUnlock()
	pr, _, err := print(ed, a)
	return pr, err
}

func print(ed *Editor, a Address) ([]rune, addr, error) {
	at, err := a.addr(ed)
	if err != nil {
		return nil, addr{}, err
	}
	rs := make([]rune, at.size())
	if err := ed.buf.runes.read(rs, at.from); err != nil {
		return nil, addr{}, err
	}
	ed.marks['.'] = at
	return rs, at, nil
}

// Where returns the rune offsets of an address.
// The from offset is inclusive and to is exclusive.
// Dot is set to the address.
func (ed *Editor) Where(a Address) (from, to int64, err error) {
	ed.buf.lock.RLock()
	defer ed.buf.lock.RUnlock()
	at, err := where(ed, a)
	return at.from, at.to, err
}

func where(ed *Editor, a Address) (addr, error) {
	at, err := a.addr(ed)
	if err != nil {
		return addr{}, err
	}
	ed.marks['.'] = at
	return at, nil
}

// Substitute substitutes text for the first match
// of the regular expression in the addressed range.
// When substituting, a backslash followed by a digit d
// stands for the string that matched the d-th subexpression.
// \n is a literal newline.
// If g is true then all matches in the address range are substituted.
// Dot is set to the modified address.
func (ed *Editor) Substitute(a Address, re *re1.Regexp, repl []rune, g bool) error {
	return ed.do(func() (addr, error) { return sub(ed, a, re, repl, g) })
}

func sub(ed *Editor, a Address, re *re1.Regexp, repl []rune, g bool) (addr, error) {
	at, err := a.addr(ed)
	if err != nil {
		return addr{}, err
	}
	from := at.from
	for {
		m, err := sub1(ed, addr{from, at.to}, re, repl)
		if err != nil {
			return addr{}, err
		}
		if !g || m == nil || m[0][1] == at.to {
			break
		}
		from = m[0][1]
	}
	return at, nil
}

// Sub1 substitutes the first match of the regular expression
// with the replacement specifier.
func sub1(ed *Editor, at addr, re *re1.Regexp, repl []rune) ([][2]int64, error) {
	m, err := match(ed, at, re)
	if err != nil || m == nil {
		return m, err
	}
	rs, err := replRunes(ed, m, repl)
	if err != nil {
		return nil, err
	}
	at = addr{m[0][0], m[0][1]}
	err = ed.pending.append(ed.buf.seq, ed.who, at, sliceSource(rs))
	return m, err
}

// ReplRunes returns the runes that replace a matched regexp.
func replRunes(ed *Editor, m [][2]int64, repl []rune) ([]rune, error) {
	var rs []rune
	for i := 0; i < len(repl); i++ {
		d := escDigit(repl[i:])
		if d < 0 {
			rs = append(rs, repl[i])
			continue
		}
		sub, err := subExprMatch(ed, m, d)
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

// SubExprMatch returns the runes of a matched subexpression.
func subExprMatch(ed *Editor, m [][2]int64, i int) ([]rune, error) {
	if i < 0 || i >= len(m) {
		return []rune{}, nil
	}
	rs := make([]rune, m[i][1]-m[i][0])
	if err := ed.buf.runes.read(rs, m[i][0]); err != nil {
		return nil, err
	}
	return rs, nil
}

type runeSlice struct {
	buf *runes
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

// Match returns the results of matching a regular experssion
// within an address range in an Editor.
func match(ed *Editor, at addr, re *re1.Regexp) ([][2]int64, error) {
	rs := &runeSlice{buf: ed.buf.runes, addr: at}
	m := re.Match(rs, 0)
	for i := range m {
		m[i][0] += at.from
		m[i][1] += at.from
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
//		Dot is set to the address.
//	c
//	i	Just like a, but c changes the addressed text
//		and i inserts before the addressed text.
//		Dot is set to the address.
//	{addr} d
//		Deletes the addressed text.
//		If an address is not supplied, dot is used.
//		Dot is set to the address.
//	{addr} m {[a-zA-Z]}
//		Sets the named mark to the address.
//		If an address is not supplied, dot is used.
//		If a mark name is not given, dot is set.
//		Dot is set to the address.
//	{addr} p
//		Returns the runes identified by the address.
//		If an address is not supplied, dot is used.
//		Dot is set to the address.
//	{addr} =#
//		Returns the runes offsets of the address.
//		If an address is not supplied, dot is used.
//		Dot is set to the address.
//	{addr} s/regexp/text/
//		Substitute substitutes text for the first match
// 		of the regular expression in the addressed range.
// 		When substituting, a backslash followed by a digit d
// 		stands for the string that matched the d-th subexpression.
//		\n is a literal newline.
// 		If the delimiter after the text is followed by the letter g
//		then all matches in the address range are substituted.
//		If an address is not supplied, dot is used.
//		Dot is set to the modified address.
func (ed *Editor) Edit(cmd []rune) ([]rune, error) {
	var pr []rune
	err := ed.do(func() (at addr, err error) {
		pr, at, err = edit(ed, cmd)
		return at, err
	})
	return pr, err
}

func edit(ed *Editor, cmd []rune) ([]rune, addr, error) {
	a, n, err := Addr(cmd)
	switch {
	case err != nil:
		return nil, addr{}, err
	case a == nil:
		a = Dot
	case len(cmd) == n:
		return nil, addr{}, errors.New("missing command")
	default:
		cmd = cmd[n:]
	}
	switch c := cmd[0]; c {
	case 'a':
		at, err := change(ed, a.Plus(Rune(0)), parseText(cmd[1:]))
		return nil, at, err
	case 'c':
		at, err := change(ed, a, parseText(cmd[1:]))
		return nil, at, err
	case 'i':
		at, err := change(ed, a.Minus(Rune(0)), parseText(cmd[1:]))
		return nil, at, err
	case 'd':
		at, err := change(ed, a, []rune{})
		return nil, at, err
	case 'm':
		at, err := mark(ed, a, parseMarkRune(cmd[1:]))
		return nil, at, err
	case 'p':
		return print(ed, a)
	case '=':
		if cmd[1] != '#' {
			return nil, addr{}, errors.New("unknown command: " + string(c))
		}
		at, err := where(ed, a)
		if err != nil {
			return nil, addr{}, err
		}
		return []rune(fmt.Sprintf("#%d,#%d", at.from, at.to)), at, nil
	case 's':
		re, err := re1.Compile(cmd[1:], re1.Options{Delimited: true})
		if err != nil {
			return nil, addr{}, err
		}
		exp := re.Expression()
		cmd = cmd[1+len(exp):]
		repl := parseDelimited(exp[0], true, cmd)
		g := len(repl) < len(cmd)-1 && cmd[len(repl)+1] == 'g'
		at, err := sub(ed, a, re, repl, g)
		return nil, at, err
	default:
		return nil, addr{}, errors.New("unknown command: " + string(c))
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
