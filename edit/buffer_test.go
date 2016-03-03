// Copyright © 2015, The T Authors.

package edit

import (
	"errors"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/eaburns/T/edit/runes"
)

// String returns a string containing the entire editor contents.
func (buf *Buffer) String() string {
	data, err := ioutil.ReadAll(buf.Reader(Span{0, buf.Size()}))
	if err != nil {
		panic(err)
	}
	return string(data)
}

func TestBufferClose(t *testing.T) {
	if err := NewBuffer().Close(); err != nil {
		t.Fatalf("failed to close the buffer: %v", err)
	}
}

var badSpans = []Span{
	Span{-1, 0},
	Span{0, -1},
	Span{1, 0},
	Span{0, 1},
}

func TestBufferBadSetMark(t *testing.T) {
	for _, s := range badSpans {
		buf := NewBuffer()
		defer buf.Close()
		if err := buf.SetMark('.', s); err != ErrInvalidArgument {
			t.Errorf("buf.SetMark('.', %v)=%v, want %v", s, err, ErrInvalidArgument)
		}
	}
}

func TestBufferBadRuneReader(t *testing.T) {
	for _, s := range badSpans {
		buf := NewBuffer()
		defer buf.Close()
		if _, _, err := buf.RuneReader(s).ReadRune(); err != ErrInvalidArgument {
			t.Errorf("buf.RuneReader(%v).ReadRune()=_,_,%v, want %v", s, err, ErrInvalidArgument)
		}
	}
}

func TestBufferBadReader(t *testing.T) {
	for _, s := range badSpans {
		buf := NewBuffer()
		defer buf.Close()
		var d [1]byte
		if _, err := buf.Reader(s).Read(d[:]); err != ErrInvalidArgument {
			t.Errorf("buf.Reader(%v).Read(·)=_,%v, want %v", s, err, ErrInvalidArgument)
		}
	}
}

func TestBufferChangeOutOfSequence(t *testing.T) {
	buf := NewBuffer()
	defer buf.Close()
	const init = "Hello, 世界"
	if err := buf.Change(Span{}, strings.NewReader(init)); err != nil {
		panic(err)
	}
	if err := buf.Apply(); err != nil {
		panic(err)
	}

	if err := buf.Change(Span{10, 20}, strings.NewReader("")); err != nil {
		panic(err)
	}
	if err := buf.Change(Span{0, 10}, strings.NewReader("")); err != nil {
		panic(err)
	}
	if err := buf.Apply(); err != ErrOutOfSequence {
		t.Errorf("buf.Apply()=%v, want %v", err, ErrOutOfSequence)
	}
}

func TestLogEntryEmpty(t *testing.T) {
	l := newLog()
	defer l.close()
	if !logFirst(l).end() {
		t.Errorf("empty logFirst(l).end()=false, want true")
	}
	if !logFirst(l).prev().end() {
		t.Errorf("empty logFirst(l).prev().end()=false, want true")
	}
	if !logLast(l).end() {
		t.Errorf("empty logLast(l).end()=false, want true")
	}
	if !logLast(l).next().end() {
		t.Errorf("empty logLast(l).next().end()=false, want true")
	}
}

func TestLogEntryWrap(t *testing.T) {
	entries := []testEntry{
		{seq: 0, span: Span{0, 0}, str: "Hello, World!"},
	}
	l := initTestLog(t, entries)
	defer l.close()

	it := logFirst(l)
	if it != logLast(l) {
		t.Errorf("it != logLast")
	}
	if !it.next().end() {
		t.Errorf("it.next().end()=false, want true")
	}
	if it.next().next() != logFirst(l) {
		t.Errorf("it.next().next() != logFirst(l)")
	}
	if !it.prev().end() {
		t.Errorf("it.prev().end()=false, want true")
	}
	if it.prev().prev() != logLast(l) {
		t.Errorf("it.prev().prev() != logLast(l)")
	}
}

func TestLogEntryBackAndForth(t *testing.T) {
	entries := []testEntry{
		{seq: 0, span: Span{0, 0}, str: "Hello, World!"},
		{seq: 1, span: Span{0, 5}, str: "Foo, Bar, Baz"},
		{seq: 1, span: Span{8, 10}, str: "Ms. Pepper"},
		{seq: 2, span: Span{20, 50}, str: "Hello, 世界"},
	}
	l := initTestLog(t, entries)
	defer l.close()

	// Go forward.
	it := logFirst(l)
	for i := range entries {
		checkEntry(t, i, entries, it)
		it = it.next()
	}
	if !it.end() {
		t.Fatalf("end: it.end()=false, want true")
	}

	// Then go back.
	it = it.prev()
	for i := len(entries) - 1; i >= 0; i-- {
		checkEntry(t, i, entries, it)
		it = it.prev()
	}
	if !it.end() {
		t.Fatalf("start: it.start()=false, want true")
	}
}

func TestLogAt(t *testing.T) {
	entries := []testEntry{
		{seq: 0, span: Span{0, 0}, str: "Hello, World!"},
		{seq: 1, span: Span{0, 5}, str: "Foo, Bar, Baz"},
		{seq: 1, span: Span{8, 10}, str: "Ms. Pepper"},
		{seq: 2, span: Span{20, 50}, str: "Hello, 世界"},
	}
	l := initTestLog(t, entries)
	defer l.close()

	// Get the location of each entry.
	var locs []int64
	for it := logFirst(l); !it.end(); it = it.next() {
		locs = append(locs, it.offs)
	}
	if len(locs) != len(entries) {
		t.Fatalf("len(locs)=%v, want %v\n", len(locs), len(entries))
	}

	// Test logAt.
	for i := range entries {
		checkEntry(t, i, entries, logAt(l, locs[i]))
	}
}

func TestLogEntryError(t *testing.T) {
	entries := []testEntry{
		{seq: 0, span: Span{0, 0}, str: "Hello, World!"},
		{seq: 1, span: Span{0, 5}, str: "Foo, Bar, Baz"},
		{seq: 1, span: Span{8, 10}, str: "Ms. Pepper"},
		{seq: 2, span: Span{20, 50}, str: "Hello, 世界"},
	}
	l := initTestLog(t, entries)
	defer l.close()
	it := logFirst(l)
	it.err = errors.New("test error")
	if !it.next().end() {
		t.Errorf("!it.next().end()")
	}
	if !it.prev().end() {
		t.Errorf("!it.prev().end()")
	}
}

func TestLogEntryStore(t *testing.T) {
	entries := []testEntry{
		{seq: 0, span: Span{0, 0}, str: "Hello, World!"},
		{seq: 1, span: Span{0, 5}, str: "Foo, Bar, Baz"},
		{seq: 1, span: Span{8, 10}, str: "Ms. Pepper"},
		{seq: 2, span: Span{20, 50}, str: "Hello, 世界"},
	}
	l := initTestLog(t, entries)
	defer l.close()

	// Modify an entry.
	e1 := logFirst(l).next()
	if err := e1.store(); err != nil {
		t.Fatalf("e1.store()=%v, want nil", err)
	}

	// Check that the entry is modified and others are not.
	e := logFirst(l)
	for i := range entries {
		checkEntry(t, i, entries, e)
		e = e.next()
	}
}

func TestLogEntryPop(t *testing.T) {
	entries := []testEntry{
		{seq: 0, str: "Hello, World"},
		{seq: 1, str: "☹☺"},
		{seq: 1, str: "---"},
		{seq: 1, str: "aeu"},
		{seq: 2, str: "Testing 123"},
	}
	l := initTestLog(t, entries)
	defer l.close()

	seq2 := logLast(l)
	if err := seq2.pop(); err != nil {
		t.Fatalf("seq2.pop()=%v, want nil", err)
	}
	// {seq: 1, str: "aeu"}
	checkEntry(t, len(entries)-2, entries, logLast(l))

	seq1 := logLast(l).prev().prev()
	// {seq: 1, str: "☹☺"}
	checkEntry(t, 1, entries, seq1)
	if err := seq1.pop(); err != nil {
		t.Fatalf("seq1.pop()=%v, want nil", err)
	}
	// {seq: 0, str: "Hello, World"}
	checkEntry(t, 0, entries, logLast(l))

	seq0 := logLast(l)
	if err := seq0.pop(); err != nil {
		t.Fatalf("seq0.pop()=%v, want nil", err)
	}
	if !logLast(l).end() {
		t.Fatal("popped last entry, log is not empty")
	}

	end := logLast(l)
	if err := end.pop(); err != nil {
		t.Fatalf("end.pop()=%v, want nil", err)
	}
	if !logLast(l).end() {
		t.Fatal("popped end entry, log is not empty")
	}
}

type testEntry struct {
	seq  int32
	span Span
	str  string
}

func checkEntry(t *testing.T, i int, entries []testEntry, e entry) {
	te := entries[i]
	if got := readSource(e.data()); got != te.str {
		t.Fatalf("entry %d: e.data()=%q, want %q\n", i, got, te.str)
	}
	if e.seq != te.seq {
		t.Fatalf("entry %d: e.h.seq=%v, want %v\n", i, e.seq, te.seq)
	}
	if e.span != te.span {
		t.Fatalf("entry %d: e.h.span=%v, want %v\n", i, e.span, te.span)
	}
	t.Logf("entry %d ok", i)
}

func initTestLog(t *testing.T, entries []testEntry) *log {
	l := newLog()
	for _, e := range entries {
		r := runes.StringReader(e.str)
		if err := l.append(e.seq, e.span, r); err != nil {
			t.Fatalf("l.append(%v, %v, %q)=%v", e.seq, e.span, e.str, err)
		}
	}
	return l
}

func readSource(src runes.Reader) string {
	rs, err := runes.ReadAll(src)
	if err != nil {
		panic(err)
	}
	return string(rs)
}
