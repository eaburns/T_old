// Copyright © 2015, The T Authors.

// Package edit provides sam-style editing of rune buffers.
// See sam(1) for an overview: http://swtch.com/plan9port/man/man1/sam.html.
package edit

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"unicode"

	"github.com/eaburns/T/edit/runes"
	"github.com/eaburns/T/re1"
)

// MaxRunes is the maximum number of runes to read
// into memory for operations like Print.
const MaxRunes = 4096

// A Buffer is an editable rune buffer.
type Buffer struct {
	lock     sync.RWMutex
	runes    *runes.Buffer
	eds      []*Editor
	seq, who int32
}

// NewBuffer returns a new, empty Buffer.
func NewBuffer() *Buffer {
	return newBuffer(runes.NewBuffer(1 << 12))
}

func newBuffer(rs *runes.Buffer) *Buffer { return &Buffer{runes: rs} }

// Close closes the Buffer.
// After Close is called, the Buffer is no longer editable.
func (buf *Buffer) Close() error {
	buf.lock.Lock()
	defer buf.lock.Unlock()
	return buf.runes.Close()
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
	var marks map[rune]addr
	defer func() { ed.marks = marks }()
retry:
	marks = make(map[rune]addr, len(ed.marks))
	for r, a := range ed.marks {
		marks[r] = a
	}
	seq, at, err := pendChanges(ed, f)
	if err != nil {
		return err
	}
	if at, err = fixAddrs(at, ed.pending); err != nil {
		return err
	}
	switch retry, err := applyChanges(ed, seq); {
	case err != nil:
		return err
	case retry:
		goto retry
	}
	ed.marks['.'] = at
	marks = ed.marks
	return err
}

func pendChanges(ed *Editor, f func() (addr, error)) (int32, addr, error) {
	if err := ed.pending.clear(); err != nil {
		return 0, addr{}, err
	}

	ed.buf.lock.RLock()
	defer ed.buf.lock.RUnlock()
	seq := ed.buf.seq
	at, err := f()
	return seq, at, err
}

func applyChanges(ed *Editor, seq int32) (bool, error) {
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
	return ed.pending.append(ed.buf.seq, ed.who, at, src)
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
// It is an error to print more than MaxRunes runes.
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
	if at.size() > MaxRunes {
		return nil, addr{}, errors.New("print too big")
	}
	rs, err := ed.buf.runes.Read(int(at.size()), at.from)
	if err != nil {
		return nil, addr{}, err
	}
	ed.marks['.'] = at
	return rs, at, nil
}

// Where returns the rune offsets and line offsets of an address.
// The from offset is inclusive and to is exclusive.
// Dot is set to the address.
func (ed *Editor) Where(a Address) (rfrom, rto, lfrom, lto int64, err error) {
	ed.buf.lock.RLock()
	defer ed.buf.lock.RUnlock()
	at, err := whereRune(ed, a)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	lfrom, lto, err = whereLine(ed, a)
	return at.from, at.to, lfrom, lto, err
}

func whereRune(ed *Editor, a Address) (addr, error) {
	at, err := a.addr(ed)
	if err != nil {
		return addr{}, err
	}
	ed.marks['.'] = at
	return at, nil
}

func whereLine(ed *Editor, a Address) (lfrom, lto int64, err error) {
	at, err := a.addr(ed)
	if err != nil {
		return 0, 0, err
	}
	var i int64
	lfrom = 1 // line numbers are 1 based.
	for ; i < at.from; i++ {
		r, err := ed.buf.rune(i)
		if err != nil {
			return 0, 0, err
		} else if r == '\n' {
			lfrom++
		}
	}
	lto = lfrom
	for ; i < at.to; i++ {
		r, err := ed.buf.rune(i)
		if err != nil {
			return 0, 0, err
		} else if r == '\n' && i < at.to-1 {
			lto++
		}
	}
	return
}

// Change reads runes from r until end of file
// and changing the Address to the runes read.
// On success, dot is set to the changed runes.
//
// Change can be used to insert, append, and delete:
// 	Change(a.Minus(Rune(0)), r) // Insert
// 	Change(a.Plus(Rune(0)), r) // Append
// 	Change(a, runes.EmptyReader()) // Delete
func (ed *Editor) Change(a Address, r io.RuneReader) error {
	return ed.do(func() (addr, error) { return change(ed, a, runes.RunesReader(r)) })
}

func change(ed *Editor, a Address, r runes.Reader) (addr, error) {
	at, err := a.addr(ed)
	if err != nil {
		return addr{}, err
	}
	return at, pend(ed, at, r)
}

// Copy copies the runes from the source address
// after the destination address.
// Dot is set to the copied runes.
func (ed *Editor) Copy(src, dst Address) error {
	return ed.do(func() (addr, error) { return cpy(ed, src, dst) })
}

func cpy(ed *Editor, src, dst Address) (addr, error) {
	s, err := src.addr(ed)
	if err != nil {
		return addr{}, err
	}
	d, err := dst.addr(ed)
	if err != nil {
		return addr{}, err
	}
	d.from = d.to
	r := runes.LimitReader(ed.buf.runes.Reader(s.from), s.size())
	return d, pend(ed, d, r)
}

// Move moves the runes from the source address
// to after the destination address.
// Dot is set to the copied runes.
// It is an error the end of the destination
// to be within the source.
func (ed *Editor) Move(src, dst Address) error {
	return ed.do(func() (addr, error) { return move(ed, src, dst) })
}

func move(ed *Editor, src, dst Address) (addr, error) {
	s, err := src.addr(ed)
	if err != nil {
		return addr{}, err
	}
	d, err := dst.addr(ed)
	if err != nil {
		return addr{}, err
	}
	d.from = d.to

	if d.from > s.from && d.from < s.to {
		return addr{}, errors.New("addresses overlap")
	}

	if d.from >= s.to {
		// Moving to after the source. Delete the source first.
		if err := pend(ed, s, runes.EmptyReader()); err != nil {
			return addr{}, err
		}
	}
	r := runes.LimitReader(ed.buf.runes.Reader(s.from), s.size())
	if err := pend(ed, d, r); err != nil {
		return addr{}, err
	}
	if d.from <= s.from {
		// Moving to before the source. Delete the source second.
		if err := pend(ed, s, runes.EmptyReader()); err != nil {
			return addr{}, err
		}
	}
	return d, nil
}

// Substitute substitutes text for the first match
// of the regular expression in the addressed range.
// When substituting, a backslash followed by a digit d
// stands for the string that matched the d-th subexpression.
// It is an error if such a subexpression has more than MaxRunes.
// \n is a literal newline.
// If g is true then all matches in the address range are substituted.
// Dot is set to the modified address.
func (ed *Editor) Substitute(a Address, re *re1.Regexp, repl []rune, g bool) error {
	return ed.do(func() (addr, error) { return sub(ed, a, re, repl, g, 1) })
}

// SubstituteFrom is the same as Substitute but with an added option,
// n, which will skip the first n-1 matches.
func (ed *Editor) SubstituteFrom(a Address, re *re1.Regexp, repl []rune, g bool, n int) error {
	return ed.do(func() (addr, error) { return sub(ed, a, re, repl, g, n) })
}

func sub(ed *Editor, a Address, re *re1.Regexp, repl []rune, g bool, n int) (addr, error) {
	if n < 0 {
		return addr{}, errors.New("match number out of range: " + strconv.Itoa(n))
	}
	at, err := a.addr(ed)
	if err != nil {
		return addr{}, err
	}

	from := at.from
	for {
		m, err := subSingle(ed, addr{from, at.to}, re, repl, n)
		if err != nil {
			return addr{}, err
		}
		if !g || m == nil || m[0][1] == at.to {
			break
		}
		from = m[0][1]
		n = 1 // reset n to 1, so that on future iterations of this loop we get the next instance.
	}
	return at, nil
}

// SubSingle substitutes the Nth match of the regular expression
// with the replacement specifier.
func subSingle(ed *Editor, at addr, re *re1.Regexp, repl []rune, n int) ([][2]int64, error) {
	m, err := nthMatch(ed, at, re, n)
	if err != nil || m == nil {
		return m, err
	}
	rs, err := replRunes(ed, m, repl)
	if err != nil {
		return nil, err
	}
	at = addr{m[0][0], m[0][1]}
	return m, pend(ed, at, runes.SliceReader(rs))
}

// nthMatch skips past the first n-1 matches of the regular expression
func nthMatch(ed *Editor, at addr, re *re1.Regexp, n int) ([][2]int64, error) {
	var err error
	var m [][2]int64
	if n == 0 {
		n = 1
	}
	for i := 0; i < n; i++ {
		m, err = match(ed, at, re)
		if err != nil || m == nil {
			return nil, err
		}
		at.from = m[0][1]
	}
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
	n := m[i][1] - m[i][0]
	if n > MaxRunes {
		return nil, errors.New("subexpression too big")
	}
	rs, err := ed.buf.runes.Read(int(n), m[i][0])
	if err != nil {
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
// The returned rune slice is the result of commands that output,
// such as p and =#.
//
// In the following, text surrounded by / represents delimited text.
// The delimiter can be any character, it need not be /.
// Trailing delimiters may be elided, but the opening delimiter must be present.
// In delimited text, \ is an escape; the following character is interpreted literally,
// except \n which represents a literal newline.
// Items in {} are optional.
//
// Commands are:
//	addr
//		Sets the address of Dot.
// 	{addr} a/text/
//	or
//	{addr} a
//	lines of text
//	.
//		Appends after the addressed text.
//		If an address is not supplied, dot is used.
//		Dot is set to the address.
//	{addr} c
//	{addr} i
//		Just like a, but c changes the addressed text
//		and i inserts before the addressed text.
//		Dot is set to the address.
//	{addr} d
//		Deletes the addressed text.
//		If an address is not supplied, dot is used.
//		Dot is set to the address.
//	{addr} t {addr}
//	{addr} m {addr}
//		Copies or moves runes from the first address to after the second.
//		Dot is set to the newly inserted or moved runes.
//	{addr} s{n}/regexp/text/{g}
//		Substitute substitutes text for the first match
// 		of the regular expression in the addressed range.
// 		When substituting, a backslash followed by a digit d
// 		stands for the string that matched the d-th subexpression.
// 		It is an error if such a subexpression has more than MaxRunes.
//		\n is a literal newline.
//		A number n after s indicates we substitute the Nth match in the
//		address range. If n == 0 set n = 1.
// 		If the delimiter after the text is followed by the letter g
//		then all matches in the address range are substituted.
//		If a number n and the letter g are both present then the Nth match
//		and all subsequent matches in the address range are	substituted.
//		If an address is not supplied, dot is used.
//		Dot is set to the modified address.
//	{addr} k {[a-zA-Z]}
//		Sets the named mark to the address.
//		If an address is not supplied, dot is used.
//		If a mark name is not given, dot is set.
//		Dot is set to the address.
//	{addr} p
//		Returns the runes identified by the address.
//		If an address is not supplied, dot is used.
// 		It is an error to print more than MaxRunes runes.
//		Dot is set to the address.
//	{addr} ={#}
//		Without '#' returns the line offset(s) of the address.
//		With '#' returns the rune offsets of the address.
//		If an address is not supplied, dot is used.
//		Dot is set to the address.
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
		at, err := a.addr(ed)
		if err != nil {
			return nil, addr{}, err
		}
		ed.marks['.'] = at
		return nil, at, err
	default:
		cmd = cmd[n:]
	}
	switch c := cmd[0]; c {
	case 'a':
		r := runes.SliceReader(parseText(cmd[1:]))
		at, err := change(ed, a.Plus(Rune(0)), r)
		return nil, at, err
	case 'c':
		r := runes.SliceReader(parseText(cmd[1:]))
		at, err := change(ed, a, r)
		return nil, at, err
	case 'i':
		r := runes.SliceReader(parseText(cmd[1:]))
		at, err := change(ed, a.Minus(Rune(0)), r)
		return nil, at, err
	case 'd':
		at, err := change(ed, a, runes.EmptyReader())
		return nil, at, err
	case 'k':
		mk, err := parseMarkRune(cmd[1:])
		if err != nil {
			return nil, addr{}, err
		}
		at, err := mark(ed, a, mk)
		return nil, at, err
	case 'p':
		return print(ed, a)
	case '=':
		var ret string
		at, err := whereRune(ed, a)
		if err != nil {
			return nil, addr{}, err
		}
		if len(cmd) == 1 || cmd[1] != '#' {
			lineFrom, lineTo, err := whereLine(ed, a)
			if err != nil {
				return nil, addr{}, err
			}
			if lineFrom == lineTo {
				ret = fmt.Sprintf("%d", lineFrom)
			} else {
				ret = fmt.Sprintf("%d,%d", lineFrom, lineTo)
			}
		} else {
			ret = fmt.Sprintf("#%d,#%d", at.from, at.to)
		}
		return []rune(ret), at, nil
	case 's':
		n, cmd, err := parseNumber(cmd[1:])
		if err != nil {
			return nil, addr{}, err
		}
		re, err := re1.Compile(cmd[:], re1.Options{Delimited: true})
		if err != nil {
			return nil, addr{}, err
		}
		exp := re.Expression()
		cmd = cmd[len(exp):]
		repl := parseDelimited(exp[0], true, cmd)
		g := len(repl) < len(cmd)-1 && cmd[len(repl)+1] == 'g'
		at, err := sub(ed, a, re, repl, g, n)
		return nil, at, err
	case 't', 'm':
		a1, _, err := Addr(cmd[1:])
		switch {
		case err != nil:
			return nil, addr{}, err
		case a1 == nil:
			a1 = Dot
		}
		var at addr
		if c == 't' {
			at, err = cpy(ed, a, a1)
		} else {
			at, err = move(ed, a, a1)
		}
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

func parseMarkRune(cmd []rune) (rune, error) {
	var i int
	for ; i < len(cmd) && unicode.IsSpace(cmd[i]); i++ {
	}
	if i < len(cmd) && isMarkRune(cmd[i]) {
		return cmd[i], nil
	} else if i == len(cmd) {
		return '.', nil
	}
	return ' ', errors.New("bad mark: " + string(cmd[i]))
}

// parseNumber parses and returns a positive integer. The first returned
// value is the parsed number, the second is the number of runes parsed.
func parseNumber(cmd []rune) (int, []rune, error) {
	i := 0
	n := 1 // by default use the first instance
	var err error
	for len(cmd) > i && unicode.IsDigit(cmd[i]) {
		i++
	}
	if i != 0 {
		n, err = strconv.Atoi(string(cmd[:i]))
		if err != nil {
			return 0, cmd[:], err
		}
	}
	return n, cmd[i:], nil
}
