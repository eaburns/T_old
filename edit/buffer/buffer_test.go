package buffer

import (
	"io"
	"io/ioutil"
	"reflect"
	"testing"
)

const testBlockSize = 8

// Initializes a buffer with the text "01234567abcd!@#efghSTUVWXYZ"
// split across blocks of sizes: 8, 4, 3, 4, 8.
func makeTestBuffer(t *testing.T) *Buffer {
	b := New(testBlockSize)
	// Add 3 full blocks.
	n, err := b.Insert([]byte("01234567abcdefghSTUVWXYZ"), 0)
	if n != 24 || err != nil {
		b.Close()
		t.Fatalf(`Insert("01234567abcdefghSTUVWXYZ", 0)=%v,%v, want 24,nil`, n, err)
	}
	// Split block 1 in the middle.
	n, err = b.Insert([]byte("!@#"), 12)
	if n != 3 || err != nil {
		b.Close()
		t.Fatalf(`Insert("!@#", 12)=%v,%v, want 3,nil`, n, err)
	}
	ns := make([]int, len(b.blocks))
	for i, blk := range b.blocks {
		ns[i] = blk.n
	}
	if !reflect.DeepEqual(ns, []int{8, 4, 3, 4, 8}) {
		b.Close()
		t.Fatalf("blocks have sizes %v, want 8, 4, 3, 4, 8", ns)
	}
	return b
}

func TestReadAt(t *testing.T) {
	b := makeTestBuffer(t)
	defer b.Close()
	tests := []struct {
		n    int
		at   int64
		want string
		err  error
	}{
		{n: 1, at: 27, err: io.EOF},
		{n: 1, at: 28, err: io.EOF},
		{n: 1, at: -1, err: AddressError(-1)},
		{n: 1, at: -2, err: AddressError(-2)},

		{n: 0, at: 0, want: ""},
		{n: 1, at: 0, want: "0"},
		{n: 1, at: 26, want: "Z"},
		{n: 8, at: 19, want: "01234567"},
		{n: 8, at: 20, want: "1234567", err: io.EOF},
		{n: 8, at: 21, want: "234567", err: io.EOF},
		{n: 8, at: 22, want: "34567", err: io.EOF},
		{n: 8, at: 23, want: "4567", err: io.EOF},
		{n: 8, at: 24, want: "567", err: io.EOF},
		{n: 8, at: 25, want: "67", err: io.EOF},
		{n: 8, at: 26, want: "7", err: io.EOF},
		{n: 8, at: 27, want: "", err: io.EOF},
		{n: 11, at: 8, want: "abcd!@#efgh"},
		{n: 7, at: 12, want: "!@#efgh"},
		{n: 6, at: 13, want: "@#efgh"},
		{n: 5, at: 13, want: "#efgh"},
		{n: 4, at: 15, want: "efgh"},
		{n: 27, at: 0, want: "01234567abcd!@#efghSTUVWXYZ"},
		{n: 28, at: 0, want: "01234567abcd!@#efghSTUVWXYZ", err: io.EOF},
	}
	for _, test := range tests {
		bs := make([]byte, test.n)
		n, err := b.ReadAt(bs, test.at)
		if n != len(test.want) || err != test.err {
			t.Errorf("ReadAt(len=%v, %v)=%v,%v, want %v,%v",
				test.n, test.at, n, err, len(test.want), test.err)
		}
	}
}

func TestInsert(t *testing.T) {
	tests := []struct {
		init, add string
		at        int64
		want      string
		err       error
	}{
		{init: "", add: "0", at: -1, err: AddressError(-1)},
		{init: "", add: "0", at: 1, err: AddressError(1)},
		{init: "0", add: "1", at: 2, err: AddressError(2)},

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
		b := New(testBlockSize)
		defer b.Close()
		if len(test.init) > 0 {
			n, err := b.Insert([]byte(test.init), 0)
			if n != len(test.init) || err != nil {
				t.Errorf("%+v init failed: Insert(%v, 0)=%v,%v, want %v,nil",
					test, test.init, n, err, len(test.init))
				continue
			}
		}
		n, err := b.Insert([]byte(test.add), test.at)
		m := len(test.add)
		if test.err != nil {
			m = 0
		}
		if n != m || err != test.err {
			t.Errorf("%+v add failed: Insert(%v, %v)=%v,%v, want %v,%v",
				test, test.add, test.at, n, err, m, test.err)
			continue
		}
		if test.err != nil {
			continue
		}
		bs, err := ioutil.ReadAll(&reader{b: b})
		if s := string(bs); s != test.want || err != nil {
			t.Errorf("%+v read failed: ReadAll(·)=%v,%v, want %v,nil",
				test, s, err, test.want)
			continue
		}
	}
}

func TestDelete(t *testing.T) {
	tests := []struct {
		n    int
		at   int64
		want string
		err  error
	}{
		{n: 1, at: 27, err: AddressError(27)},
		{n: 1, at: -1, err: AddressError(-1)},
		{n: -1, at: 1, err: CountError(-1)},

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
		b := makeTestBuffer(t)
		defer b.Close()

		m := int(b.Size()) - len(test.want)
		if test.err != nil {
			m = 0
		}
		n, err := b.Delete(test.n, test.at)
		if n != m || err != test.err {
			t.Errorf("Delete(%v, %v)=%v,%v, want %v,%v",
				test.n, test.at, n, err, m, test.err)
			continue
		}
		if test.err != nil {
			continue
		}
		got, err := ioutil.ReadAll(&reader{b: b})
		if s := string(got); s != test.want || err != nil {
			t.Errorf("%+v read failed: ReadAll(·)=%v,%v want %v,nil",
				test, s, err, test.want)
		}
	}
}

func TestBlockAlloc(t *testing.T) {
	bs := []byte("αβξδφγθι")
	l := len(bs)
	if l <= testBlockSize {
		t.Fatalf("len(bs)=%d, want >%d", l, testBlockSize)
	}

	b := New(testBlockSize)
	defer b.Close()
	n, err := b.Insert(bs, 0)
	if n != l || err != nil {
		t.Fatalf(`Initial Insert(%v, 0)=%v,%v, want %v,nil`, bs, n, err, l)
	}
	if len(b.blocks) != 2 {
		t.Fatalf("After initial insert: len(b.blocks)=%v, want 2", len(b.blocks))
	}

	n, err = b.Delete(l, 0)
	if n != l || err != nil {
		t.Fatalf(`Delete(%v, 0)=%v,%v, want 5,nil`, l, n, err)
	}
	if len(b.blocks) != 0 {
		t.Fatalf("After delete: len(b.blocks)=%v, want 0", len(b.blocks))
	}
	if len(b.free) != 2 {
		t.Fatalf("After delete: len(b.free)=%v, want 2", len(b.free))
	}

	bs = bs[:testBlockSize/2]
	l = len(bs)

	n, err = b.Insert(bs, 0)
	if n != l || err != nil {
		t.Fatalf(`Second Insert(%v, 7)=%v,%v, want %v,nil`, bs, n, err, l)
	}
	if len(b.blocks) != 1 {
		t.Fatalf("After second insert: len(b.blocks)=%d, want 1", len(b.blocks))
	}
	if len(b.free) != 1 {
		t.Fatalf("After second insert: len(b.free)=%d, want 1", len(b.free))
	}
}

// TestInsertDeleteAndRead tests performing a few operations in sequence.
func TestInsertDeleteAndRead(t *testing.T) {
	b := New(testBlockSize)
	defer b.Close()

	const hiWorld = "Hello, World!"
	n, err := b.Insert([]byte(hiWorld), 0)
	if l := len(hiWorld); n != l || err != nil {
		t.Fatalf(`Insert(%s, 0)=%v,%v, want %v,nil`, hiWorld, n, err, l)
	}
	bs, err := ioutil.ReadAll(&reader{b: b})
	if s := string(bs); s != hiWorld || err != nil {
		t.Fatalf(`ReadAll(·)=%v,%v, want %s,nil`, s, err, hiWorld)
	}

	n, err = b.Delete(5, 7)
	if n != 5 || err != nil {
		t.Fatalf(`Delete(5, 7)=%v,%v, want 5,nil`, n, err)
	}
	bs, err = ioutil.ReadAll(&reader{b: b})
	if s := string(bs); s != "Hello, !" || err != nil {
		t.Fatalf(`ReadAll(·)=%v,%v, want "Hello, !",nil`, s, err)
	}

	const gophers = "Gophers"
	n, err = b.Insert([]byte(gophers), 7)
	if l := len(gophers); n != l || err != nil {
		t.Fatalf(`Insert(%s, 7)=%v,%v, want %v,nil`, gophers, n, err, l)
	}
	bs, err = ioutil.ReadAll(&reader{b: b})
	if s := string(bs); s != "Hello, Gophers!" || err != nil {
		t.Fatalf(`ReadAll(·)=%v,%v, want "Hello, Gophers!",nil`, s, err)
	}
}

type reader struct {
	b *Buffer
	q int64
}

func (r *reader) Read(bs []byte) (int, error) {
	if r.q < 0 || r.q >= r.b.Size() {
		// Make it out-of-range hereafter,
		// even if the buffer grows again.
		r.q = -1
		return 0, io.EOF
	}
	n, err := r.b.ReadAt(bs, r.q)
	r.q += int64(n)
	return n, err
}
