// Copyright © 2015, The T Authors.

package edit

import (
	"errors"
	"io"
	"strconv"
	"sync"
	"unicode"

	"github.com/eaburns/T/edit/runes"
	"github.com/eaburns/T/re1"
)

// MaxRunes is the maximum number of runes to read into memory.
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

// Where returns rune offsets of the address.
func (ed *Editor) Where(a Address) (addr, error) {
	ed.buf.lock.RLock()
	defer ed.buf.lock.RUnlock()
	at, err := a.where(ed)
	if err != nil {
		return addr{}, err
	}
	return at, err
}

// Do performs an Edit on the Editor's Buffer.
func (ed *Editor) Do(e Edit, w io.Writer) error {
	return ed.do(func() (addr, error) { return e.do(ed, w) })
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

// Edit parses a command and performs its edit on the buffer.
// Commands that output, such as p and =, write their output to the writer.
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
func (ed *Editor) Edit(cmd []rune, w io.Writer) error {
	return ed.do(func() (at addr, err error) { return edit(ed, cmd, w) })
}

func edit(ed *Editor, cmd []rune, w io.Writer) (addr, error) {
	a, n, err := Addr(cmd)
	switch {
	case err != nil:
		return addr{}, err
	case a == nil:
		a = Dot
	case len(cmd) == n:
		at, err := a.where(ed)
		if err != nil {
			return addr{}, err
		}
		ed.marks['.'] = at
		return at, err
	default:
		cmd = cmd[n:]
	}
	switch c := cmd[0]; c {
	case 'a':
		return change{a: a, op: 'a', str: string(parseText(cmd[1:]))}.do(ed, w)
	case 'c':
		return change{a: a, op: 'c', str: string(parseText(cmd[1:]))}.do(ed, w)
	case 'i':
		return change{a: a, op: 'i', str: string(parseText(cmd[1:]))}.do(ed, w)
	case 'd':
		return change{a: a, op: 'd'}.do(ed, w)
	case 'k':
		mk, err := parseMarkRune(cmd[1:])
		if err != nil {
			return addr{}, err
		}
		return set{a: a, m: mk}.do(ed, w)
	case 'p':
		return print{a: a}.do(ed, w)
	case '=':
		return where{a: a, line: len(cmd) == 1 || cmd[1] != '#'}.do(ed, w)
	case 's':
		n, cmd, err := parseNumber(cmd[1:])
		if err != nil {
			return addr{}, err
		}
		re, err := re1.Compile(cmd[:], re1.Options{Delimited: true})
		if err != nil {
			return addr{}, err
		}
		exp := re.Expression()
		cmd = cmd[len(exp):]
		repl := parseDelimited(exp[0], cmd)
		g := len(repl) < len(cmd)-1 && cmd[len(repl)+1] == 'g'
		return Substitute{
			A:      a,
			RE:     string(exp),
			With:   string(repl),
			Global: g,
			From:   n,
		}.do(ed, w)
	case 't', 'm':
		a1, _, err := Addr(cmd[1:])
		switch {
		case err != nil:
			return addr{}, err
		case a1 == nil:
			a1 = Dot
		}
		if c == 't' {
			return cpy{src: a, dst: a1}.do(ed, w)
		}
		return move{src: a, dst: a1}.do(ed, w)
	default:
		return addr{}, errors.New("unknown command: " + string(c))
	}
}

func parseText(e []rune) []rune {
	var i int
	for i < len(e) && unicode.IsSpace(e[i]) {
		if e[i] == '\n' {
			return parseLines(e[i+1:])
		}
		i++
	}
	if i == len(e) {
		return nil
	}
	return parseDelimited(e[i], e[i+1:])
}

func parseLines(e []rune) []rune {
	var i int
	var nl bool
	for i = 0; i < len(e); i++ {
		if nl && e[i] == '.' && (i == len(e)-1 || i < len(e)-1 && e[i+1] == '\n') {
			return e[:i]
		}
		nl = e[i] == '\n'
	}
	return e
}

// ParseDelimited returns the runes
// up to the first unescaped delimiter,
// or the end of the slice.
// A delimiter preceeded by \ is escaped and is non-terminating.
// The letter n preceeded by \ is a newline literal.
func parseDelimited(delim rune, e []rune) []rune {
	var i int
	var rs []rune
	for i = 0; i < len(e); i++ {
		switch {
		case e[i] == delim:
			return rs
		case i < len(e)-1 && e[i] == '\\' && e[i+1] == delim:
			rs = append(rs, delim)
			i++
		case i < len(e)-1 && e[i] == '\\' && e[i+1] == 'n':
			rs = append(rs, '\n')
			i++
		default:
			rs = append(rs, e[i])
		}
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
