package edit

import (
	"errors"
	"reflect"
	"regexp"
	"testing"
)

const testBlockSize = 8

func TestRunesRune(t *testing.T) {
	rs := []rune("Hello, 世界!")
	b := newRunes(testBlockSize)
	defer b.close()
	if err := b.insert(rs, 0); err != nil {
		t.Fatalf(`b.insert("%s", 0)=%v, want nil`, string(rs), err)
	}
	for i, want := range rs {
		if got, err := b.Rune(int64(i)); err != nil || got != want {
			t.Errorf("b.Rune(%d)=%v,%v, want %v,nil", i, got, err, want)
		}
	}
}

func TestRunesRead(t *testing.T) {
	b := makeTestBytes(t)
	defer b.close()
	tests := []struct {
		n    int
		offs int64
		want string
		err  string
	}{
		{n: 1, offs: 27, err: "EOF"},
		{n: 1, offs: 28, err: "EOF"},
		{n: 1, offs: -1, err: "invalid offset"},
		{n: 1, offs: -2, err: "invalid offset"},

		{n: 0, offs: 0},
		{n: 1, offs: 0, want: "0"},
		{n: 1, offs: 26, want: "Z"},
		{n: 8, offs: 19, want: "STUVWXYZ"},
		{n: 8, offs: 20, err: "EOF"},
		{n: 8, offs: 21, err: "EOF"},
		{n: 8, offs: 22, err: "EOF"},
		{n: 8, offs: 23, err: "EOF"},
		{n: 8, offs: 24, err: "EOF"},
		{n: 8, offs: 25, err: "EOF"},
		{n: 8, offs: 26, err: "EOF"},
		{n: 8, offs: 27, err: "EOF"},
		{n: 11, offs: 8, want: "abcd!@#efgh"},
		{n: 7, offs: 12, want: "!@#efgh"},
		{n: 6, offs: 13, want: "@#efgh"},
		{n: 5, offs: 13, want: "@#efg"},
		{n: 4, offs: 15, want: "efgh"},
		{n: 27, offs: 0, want: "01234567abcd!@#efghSTUVWXYZ"},
		{n: 28, offs: 0, err: "EOF"},
	}
	for _, test := range tests {
		rs := make([]rune, test.n)
		err := b.read(rs, test.offs)
		if !errMatch(test.err, err) {
			t.Errorf("ReadAt(len=%v, %v)=%v, want %v", test.n, test.offs, err, test.err)
		}
		if str := string(rs); err == nil && str != test.want {
			t.Errorf("ReadAt(len=%v, %v) read %q, want %q", test.n, test.offs, str, test.want)
		}
	}
}

func TestRunesEmptyReadAtEOF(t *testing.T) {
	b := newRunes(testBlockSize)
	defer b.close()

	if err := b.read([]rune{}, 0); err != nil {
		t.Errorf("empty buffer Read([]rune{}, 0)=%v, want nil", err)
	}

	str := "Hello, World!"
	if err := b.insert([]rune(str), 0); err != nil {
		t.Fatalf("insert(%v, 0)=%v, want nil", str, err)
	}

	if err := b.read([]rune{}, 1); err != nil {
		t.Errorf("Read([]rune{}, 1)=%v, want nil", err)
	}

	l := len(str)
	if err := b.delete(int64(l), 0); err != nil {
		t.Fatalf("delete(%v, 0)=%v, want nil", l, err)
	}
	if s := b.Size(); s != 0 {
		t.Fatalf("b.Size()=%d, want 0", s)
	}

	// The buffer should be empty, but we still don't want EOF when reading 0 bytes.
	if err := b.read([]rune{}, 0); err != nil {
		t.Errorf("deleted buffer Read([]rune{}, 0)=%v, want nil", err)
	}
}

func TestRunesInsert(t *testing.T) {
	tests := []struct {
		init, add string
		at        int64
		want      string
		err       string
	}{
		{init: "", add: "0", at: -1, err: "invalid offset"},
		{init: "", add: "0", at: 1, err: "invalid offset"},
		{init: "0", add: "1", at: 2, err: "invalid offset"},

		{init: "", add: "", at: 0, want: ""},
		{init: "", add: "0", at: 0, want: "0"},
		{init: "", add: "012", at: 0, want: "012"},
		{init: "", add: "01234567", at: 0, want: "01234567"},
		{init: "", add: "012345670", at: 0, want: "012345670"},
		{init: "", add: "0123456701234567", at: 0, want: "0123456701234567"},
		{init: "1", add: "0", at: 0, want: "01"},
		{init: "2", add: "01", at: 0, want: "012"},
		{init: "0", add: "01234567", at: 0, want: "012345670"},
		{init: "01234567", add: "01234567", at: 0, want: "0123456701234567"},
		{init: "01234567", add: "01234567", at: 8, want: "0123456701234567"},
		{init: "0123456701234567", add: "01234567", at: 8, want: "012345670123456701234567"},
		{init: "02", add: "1", at: 1, want: "012"},
		{init: "07", add: "123456", at: 1, want: "01234567"},
		{init: "00", add: "1234567", at: 1, want: "012345670"},
		{init: "01234567", add: "abc", at: 1, want: "0abc1234567"},
		{init: "01234567", add: "abc", at: 2, want: "01abc234567"},
		{init: "01234567", add: "abc", at: 3, want: "012abc34567"},
		{init: "01234567", add: "abc", at: 4, want: "0123abc4567"},
		{init: "01234567", add: "abc", at: 5, want: "01234abc567"},
		{init: "01234567", add: "abc", at: 6, want: "012345abc67"},
		{init: "01234567", add: "abc", at: 7, want: "0123456abc7"},
		{init: "01234567", add: "abc", at: 8, want: "01234567abc"},
		{init: "01234567", add: "abcdefgh", at: 4, want: "0123abcdefgh4567"},
		{init: "01234567", add: "abcdefghSTUVWXYZ", at: 4, want: "0123abcdefghSTUVWXYZ4567"},
		{init: "0123456701234567", add: "abcdefgh", at: 8, want: "01234567abcdefgh01234567"},
	}
	for _, test := range tests {
		b := newRunes(testBlockSize)
		defer b.close()
		if len(test.init) > 0 {

			if err := b.insert([]rune(test.init), 0); err != nil {
				t.Errorf("%+v init failed: insert(%v, 0)=%v, want nil", test, test.init, err)
				continue
			}
		}

		err := b.insert([]rune(test.add), test.at)
		if !errMatch(test.err, err) {
			t.Errorf("%+v add failed: insert(%v, %v)=%v, want %v",
				test, test.add, test.at, err, test.err)
			continue
		}
		if test.err != "" {
			continue
		}
		if s := readAll(b); s != test.want || err != nil {
			t.Errorf("%+v read failed: readAll(·)=%v,%v, want %v,nil",
				test, s, err, test.want)
			continue
		}
	}
}

func TestRunesDelete(t *testing.T) {
	tests := []struct {
		n, at int64
		want  string
		err   string
	}{
		{n: 1, at: 27, err: "invalid offset"},
		{n: 1, at: -1, err: "invalid offset"},

		{n: 0, at: 0, want: "01234567abcd!@#efghSTUVWXYZ"},
		{n: 1, at: 0, want: "1234567abcd!@#efghSTUVWXYZ"},
		{n: 2, at: 0, want: "234567abcd!@#efghSTUVWXYZ"},
		{n: 3, at: 0, want: "34567abcd!@#efghSTUVWXYZ"},
		{n: 4, at: 0, want: "4567abcd!@#efghSTUVWXYZ"},
		{n: 5, at: 0, want: "567abcd!@#efghSTUVWXYZ"},
		{n: 6, at: 0, want: "67abcd!@#efghSTUVWXYZ"},
		{n: 7, at: 0, want: "7abcd!@#efghSTUVWXYZ"},
		{n: 8, at: 0, want: "abcd!@#efghSTUVWXYZ"},
		{n: 9, at: 0, want: "bcd!@#efghSTUVWXYZ"},
		{n: 26, at: 0, want: "Z"},
		{n: 27, at: 0, want: ""},

		{n: 0, at: 1, want: "01234567abcd!@#efghSTUVWXYZ"},
		{n: 1, at: 1, want: "0234567abcd!@#efghSTUVWXYZ"},
		{n: 1, at: 2, want: "0134567abcd!@#efghSTUVWXYZ"},
		{n: 1, at: 3, want: "0124567abcd!@#efghSTUVWXYZ"},
		{n: 1, at: 4, want: "0123567abcd!@#efghSTUVWXYZ"},
		{n: 1, at: 5, want: "0123467abcd!@#efghSTUVWXYZ"},
		{n: 1, at: 6, want: "0123457abcd!@#efghSTUVWXYZ"},
		{n: 1, at: 7, want: "0123456abcd!@#efghSTUVWXYZ"},
		{n: 1, at: 8, want: "01234567bcd!@#efghSTUVWXYZ"},
		{n: 1, at: 9, want: "01234567acd!@#efghSTUVWXYZ"},
		{n: 8, at: 1, want: "0bcd!@#efghSTUVWXYZ"},
		{n: 26, at: 1, want: "0"},
		{n: 25, at: 1, want: "0Z"},
	}
	for _, test := range tests {
		b := makeTestBytes(t)
		defer b.close()

		err := b.delete(test.n, test.at)
		if !errMatch(test.err, err) {
			t.Errorf("delete(%v, %v)=%v, want %v", test.n, test.at, err, test.err)
			continue
		}
		if test.err != "" {
			continue
		}
		if s := readAll(b); s != test.want || err != nil {
			t.Errorf("%+v read failed: ReadAll(·)=%v,%v want %v,nil", test, s, err, test.want)
		}
	}
}

func TestRunesBlockAlloc(t *testing.T) {
	rs := []rune("αβξδφγθιζ")
	l := len(rs)
	if l <= testBlockSize {
		t.Fatalf("len(rs)=%d, want >%d", l, testBlockSize)
	}

	b := newRunes(testBlockSize)
	defer b.close()

	if err := b.insert(rs, 0); err != nil {
		t.Fatalf(`Initial insert(%v, 0)=%v, want nil`, rs, err)
	}
	if len(b.blocks) != 2 {
		t.Fatalf("After initial insert: len(b.blocks)=%v, want 2", len(b.blocks))
	}

	if err := b.delete(int64(l), 0); err != nil {
		t.Fatalf(`delete(%v, 0)=%v, want nil`, l, err)
	}
	if len(b.blocks) != 0 {
		t.Fatalf("After delete: len(b.blocks)=%v, want 0", len(b.blocks))
	}
	if len(b.free) != 2 {
		t.Fatalf("After delete: len(b.free)=%v, want 2", len(b.free))
	}

	rs = rs[:testBlockSize/2]
	l = len(rs)

	if err := b.insert(rs, 0); err != nil {
		t.Fatalf(`Second insert(%v, 7)=%v, want nil`, rs, err)
	}
	if len(b.blocks) != 1 {
		t.Fatalf("After second insert: len(b.blocks)=%d, want 1", len(b.blocks))
	}
	if len(b.free) != 1 {
		t.Fatalf("After second insert: len(b.free)=%d, want 1", len(b.free))
	}
}

// TestInsertDeleteAndRead tests performing a few operations in sequence.
func TestRunesInsertDeleteAndRead(t *testing.T) {
	b := newRunes(testBlockSize)
	defer b.close()

	const hiWorld = "Hello, World!"
	err := b.insert([]rune(hiWorld), 0)
	if err != nil {
		t.Fatalf(`insert(%s, 0)=%v, want nil`, hiWorld, err)
	}
	if s := readAll(b); s != hiWorld || err != nil {
		t.Fatalf(`readAll(·)=%v,%v, want %s,nil`, s, err, hiWorld)
	}

	if err := b.delete(5, 7); err != nil {
		t.Fatalf(`delete(5, 7)=%v, want nil`, err)
	}
	if s := readAll(b); s != "Hello, !" || err != nil {
		t.Fatalf(`readAll(·)=%v,%v, want "Hello, !",nil`, s, err)
	}

	const gophers = "Gophers"
	err = b.insert([]rune(gophers), 7)
	if err != nil {
		t.Fatalf(`insert(%s, 7)=%v, want nil`, gophers, err)
	}
	if s := readAll(b); s != "Hello, Gophers!" || err != nil {
		t.Fatalf(`readAll(·)=%v,%v, want "Hello, Gophers!",nil`, s, err)
	}
}

func errMatch(re string, err error) bool {
	if err == nil {
		return re == ""
	}
	return regexp.MustCompile(re).Match([]byte(err.Error()))
}

func readAll(b *runes) string {
	rs := make([]rune, b.Size())
	if err := b.read(rs, 0); err != nil {
		panic(err)
	}
	return string(rs)
}

// Initializes a buffer with the text "01234567abcd!@#efghSTUVWXYZ"
// split across blocks of sizes: 8, 4, 3, 4, 8.
func makeTestBytes(t *testing.T) *runes {
	b := newRunes(testBlockSize)
	// Add 3 full blocks.

	if err := b.insert([]rune("01234567abcdefghSTUVWXYZ"), 0); err != nil {
		b.close()
		t.Fatalf(`insert("01234567abcdefghSTUVWXYZ", 0)=%v, want nil`, err)
	}
	// Split block 1 in the middle.
	if err := b.insert([]rune("!@#"), 12); err != nil {
		b.close()
		t.Fatalf(`insert("!@#", 12)=%v, want nil`, err)
	}
	ns := make([]int, len(b.blocks))
	for i, blk := range b.blocks {
		ns[i] = blk.n
	}
	if !reflect.DeepEqual(ns, []int{8, 4, 3, 4, 8}) {
		b.close()
		t.Fatalf("blocks have sizes %v, want 8, 4, 3, 4, 8", ns)
	}
	return b
}

type errReadWriterAt struct{ error }

func (e *errReadWriterAt) ReadAt([]byte, int64) (int, error)  { return 0, e.error }
func (e *errReadWriterAt) WriteAt([]byte, int64) (int, error) { return 0, e.error }
func (e *errReadWriterAt) Close() error                       { return e.error }

// TestErrors tests some error cases. It's not exhaustive.
func TestRunesErrors(t *testing.T) {
	str := []rune("Hello, World")
	f := &errReadWriterAt{nil}
	b := newRunesReaderWriterAt(len(str)/2, f)

	if err := b.insert(str, 0); err != nil {
		t.Fatalf("b.insert(…)=%v, want nil", err)
	}

	// From here on, all IO causes an error.
	f.error = errors.New("bad IO")

	if _, err := b.Rune(0); err != f.error {
		t.Errorf("b.Rune(0)=%v, want %v", err, f.error)
	}
	if err := b.insert(str, 3); err != f.error {
		t.Errorf("b.insert(…)=%v, want %v", err, f.error)
	}
	if err := b.delete(1, 0); err != f.error {
		t.Errorf("b.delete(…)=%v, want %v", err, f.error)
	}
	// The delete failed, so nothing should have been deleted.
	if sz := b.Size(); sz != int64(len(str)) {
		t.Errorf("b.Size()=%v, want %v", sz, len(str))
	}
	if err := b.read(make([]rune, b.Size()), 0); err != f.error {
		t.Errorf("b.read(…)=%v, want %v", err, f.error)
	}
	if err := b.close(); err != f.error {
		t.Errorf("b.close()=%v, want %v", err, f.error)
	}
}
