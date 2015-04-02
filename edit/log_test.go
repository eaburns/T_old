package edit

import (
	"errors"
	"testing"
)

func TestEntryEmpty(t *testing.T) {
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

func TestEntryWrap(t *testing.T) {
	entries := []testEntry{
		{seq: 0, who: 0, at: addr{0, 0}, str: "Hello, World!"},
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

func TestEntryBackAndForth(t *testing.T) {
	entries := []testEntry{
		{seq: 0, who: 0, at: addr{0, 0}, str: "Hello, World!"},
		{seq: 1, who: 0, at: addr{0, 5}, str: "Foo, Bar, Baz"},
		{seq: 1, who: 0, at: addr{8, 10}, str: "Ms. Pepper"},
		{seq: 2, who: 1, at: addr{20, 50}, str: "Hello, 世界"},
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
		{seq: 0, who: 0, at: addr{0, 0}, str: "Hello, World!"},
		{seq: 1, who: 0, at: addr{0, 5}, str: "Foo, Bar, Baz"},
		{seq: 1, who: 0, at: addr{8, 10}, str: "Ms. Pepper"},
		{seq: 2, who: 1, at: addr{20, 50}, str: "Hello, 世界"},
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

func TestEntryError(t *testing.T) {
	entries := []testEntry{
		{seq: 0, who: 0, at: addr{0, 0}, str: "Hello, World!"},
		{seq: 1, who: 2, at: addr{0, 5}, str: "Foo, Bar, Baz"},
		{seq: 1, who: 2, at: addr{8, 10}, str: "Ms. Pepper"},
		{seq: 2, who: 1, at: addr{20, 50}, str: "Hello, 世界"},
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

func TestEntryStore(t *testing.T) {
	entries := []testEntry{
		{seq: 0, who: 0, at: addr{0, 0}, str: "Hello, World!"},
		{seq: 1, who: 2, at: addr{0, 5}, str: "Foo, Bar, Baz"},
		{seq: 1, who: 2, at: addr{8, 10}, str: "Ms. Pepper"},
		{seq: 2, who: 1, at: addr{20, 50}, str: "Hello, 世界"},
	}
	l := initTestLog(t, entries)

	// Modify an entry.
	e1 := logFirst(l).next()
	e1.who = 123
	if err := e1.store(); err != nil {
		t.Fatalf("e1.store()=%v, want nil", err)
	}

	// Check that the entry is modified and others are not.
	entries[1].who = 123
	e := logFirst(l)
	for i := range entries {
		checkEntry(t, i, entries, e)
		e = e.next()
	}
}

type testEntry struct {
	seq, who int32
	at       addr
	str      string
}

func checkEntry(t *testing.T, i int, entries []testEntry, e entry) {
	te := entries[i]
	if got := readSource(e.data()); got != te.str {
		t.Fatalf("entry %d: e.data()=%q, want %q\n", i, got, te.str)
	}
	if e.seq != te.seq {
		t.Fatalf("entry %d: e.h.seq=%v, want %v\n", i, e.seq, te.seq)
	}
	if e.who != te.who {
		t.Fatalf("entry %d: e.h.who=%v, want %v\n", i, e.who, te.who)
	}
	if e.at != te.at {
		t.Fatalf("entry %d: e.h.at=%v, want %v\n", i, e.at, te.at)
	}
	t.Logf("entry %d ok", i)
}

func initTestLog(t *testing.T, entries []testEntry) *log {
	l := newLog()
	for _, e := range entries {
		if err := l.append(e.seq, e.who, e.at, sliceSource([]rune(e.str))); err != nil {
			t.Fatalf("l.append(%v, %v, %q)=%v", e.seq, e.at, e.str, err)
		}
	}
	return l
}

func readSource(src source) string {
	b := newRunes(int(src.size()))
	defer b.close()
	if err := src.insert(b, b.Size()); err != nil {
		panic(err)
	}
	rs := make([]rune, src.size())
	if err := b.read(rs, 0); err != nil {
		panic(err)
	}
	return string(rs)
}
