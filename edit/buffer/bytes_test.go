package buffer

import (
	"io"
	"io/ioutil"
	"reflect"
	"testing"
	"unicode/utf8"
)

const testBlockSize = 8

func TestBytesWrite(t *testing.T) {
	tests := []struct {
		init  string
		write string
		at    Address
		want  string
		err   error
	}{
		{at: Address{From: 1, To: 2}, err: AddressError(Address{From: 1, To: 2})},
		{at: Address{From: -1, To: 0}, err: AddressError(Address{From: -1, To: 0})},
		{at: Address{From: 0, To: 1}, err: AddressError(Address{From: 0, To: 1})},
		{at: Address{From: 2, To: 1}, err: AddressError(Address{From: 2, To: 1})},

		{init: "", write: "", at: Address{}, want: ""},
		{init: "", write: "Hello, World!", at: Address{}, want: "Hello, World!"},
		{init: "", write: "Hello, 世界", at: Address{}, want: "Hello, 世界"},
		{init: "Hello, World!", write: "", at: Address{0, 13}, want: ""},
		{init: "Hello, !", write: "World", at: Address{7, 7}, want: "Hello, World!"},
		{init: "Hello, World", write: "! ☺", at: Address{12, 12}, want: "Hello, World! ☺"},
		{init: ", World!", write: "Hello", at: Address{0, 0}, want: "Hello, World!"},
	}
	for _, test := range tests {
		b := NewBytes(testBlockSize)
		defer b.Close()
		if err := b.Write([]byte(test.init), Address{}); err != nil {
			t.Errorf("init Write(%v, Address{})=%v, want nil", test.init, err)
			continue
		}
		if err := b.Write([]byte(test.write), test.at); err != test.err {
			t.Errorf("Write(%v, %v)=%v, want %v", test.write, test.at, err, test.err)
			continue
		}
		if test.err != nil {
			continue
		}
		bs, err := b.Read(Address{From: 0, To: b.Size()})
		if s := string(bs); s != test.want || err != nil {
			t.Errorf(`%+v Read(Address{0, %d})="%s",%v, want "%v",nil`,
				test, b.Size(), s, err, test.want)
			continue
		}
	}
}

func TestBytesRead(t *testing.T) {
	const hi = "Hello, 世界"
	tests := []struct {
		init string
		at   Address
		want string
		err  error
	}{
		{at: Address{From: 1, To: 2}, err: AddressError(Address{From: 1, To: 2})},
		{at: Address{From: -1, To: 0}, err: AddressError(Address{From: -1, To: 0})},
		{at: Address{From: 0, To: 1}, err: AddressError(Address{From: 0, To: 1})},
		{at: Address{From: 2, To: 1}, err: AddressError(Address{From: 2, To: 1})},

		{init: "", at: Address{}, want: ""},
		{init: "Hello, World!", at: Address{13, 13}, want: ""},
		{init: "Hello, World!", at: Address{0, 13}, want: "Hello, World!"},
		{init: hi, at: Address{0, int64(len(hi))}, want: hi},
		{init: hi, at: Address{1, int64(len(hi))}, want: "ello, 世界"},
		{init: hi, at: Address{0, int64(len(hi) - utf8.RuneLen('界'))}, want: "Hello, 世"},
		{init: hi, at: Address{1, int64(len(hi) - utf8.RuneLen('界'))}, want: "ello, 世"},
	}
	for _, test := range tests {
		b := NewBytes(testBlockSize)
		defer b.Close()
		if err := b.Write([]byte(test.init), Address{}); err != nil {
			t.Errorf("init Write(%v, Address{})=%v, want nil", test.init, err)
			continue
		}
		bs, err := b.Read(test.at)
		if s := string(bs); s != test.want || err != test.err {
			t.Errorf(`Read(%v)="%s",%v, want "%v",%v`,
				test.at, s, err, test.want, test.err)
			continue
		}
	}
}

func TestReadAt(t *testing.T) {
	b := makeTestBytes(t)
	defer b.Close()
	tests := []struct {
		n    int
		at   int64
		want string
		err  error
	}{
		{n: 1, at: 27, err: io.EOF},
		{n: 1, at: 28, err: io.EOF},
		{n: 1, at: -1, err: AddressError(Point(-1))},
		{n: 1, at: -2, err: AddressError(Point(-2))},

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

func TestEmptyReadAtEOF(t *testing.T) {
	b := NewBytes(testBlockSize)
	defer b.Close()

	if n, err := b.ReadAt([]byte{}, 0); n != 0 || err != nil {
		t.Errorf("empty buffer ReadAt([]byte{}, 0)=%v,%v, want 0,nil", n, err)
	}

	str := "Hello, World!"
	l := len(str)
	if n, err := b.insert([]byte(str), 0); n != l || err != nil {
		t.Fatalf("insert(%v, 0)=%v,%v, want %v,nil", str, n, err, l)
	}

	if n, err := b.ReadAt([]byte{}, 1); n != 0 || err != nil {
		t.Errorf("ReadAt([]byte{}, 1)=%v,%v, want 0,nil", n, err)
	}

	if n, err := b.delete(int64(l), 0); n != int64(l) || err != nil {
		t.Fatalf("delete(%v, 0)=%v,%v, want %v, nil", l, n, err, l)
	}
	if s := b.Size(); s != 0 {
		t.Fatalf("b.Size()=%d, want 0", s)
	}

	// The buffer should be empty, but we still don't want EOF when reading 0 bytes.
	if n, err := b.ReadAt([]byte{}, 0); n != 0 || err != nil {
		t.Errorf("deleted buffer ReadAt([]byte{}, 0)=%v,%v, want 0,nil", n, err)
	}
}

func TestInsert(t *testing.T) {
	tests := []struct {
		init, add string
		at        int64
		want      string
		err       error
	}{
		{init: "", add: "0", at: -1, err: AddressError(Point(-1))},
		{init: "", add: "0", at: 1, err: AddressError(Point(1))},
		{init: "0", add: "1", at: 2, err: AddressError(Point(2))},

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
		b := NewBytes(testBlockSize)
		defer b.Close()
		if len(test.init) > 0 {
			n, err := b.insert([]byte(test.init), 0)
			if n != len(test.init) || err != nil {
				t.Errorf("%+v init failed: insert(%v, 0)=%v,%v, want %v,nil",
					test, test.init, n, err, len(test.init))
				continue
			}
		}
		n, err := b.insert([]byte(test.add), test.at)
		m := len(test.add)
		if test.err != nil {
			m = 0
		}
		if n != m || err != test.err {
			t.Errorf("%+v add failed: insert(%v, %v)=%v,%v, want %v,%v",
				test, test.add, test.at, n, err, m, test.err)
			continue
		}
		if test.err != nil {
			continue
		}
		bs, err := ioutil.ReadAll(&bytesReader{b: b})
		if s := string(bs); s != test.want || err != nil {
			t.Errorf("%+v read failed: ReadAll(·)=%v,%v, want %v,nil",
				test, s, err, test.want)
			continue
		}
	}
}

func TestDelete(t *testing.T) {
	tests := []struct {
		n, at int64
		want  string
		err   error
	}{
		{n: 1, at: 27, err: AddressError(Point(27))},
		{n: 1, at: -1, err: AddressError(Point(-1))},
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
		b := makeTestBytes(t)
		defer b.Close()

		m := b.Size() - int64(len(test.want))
		if test.err != nil {
			m = 0
		}
		n, err := b.delete(test.n, test.at)
		if n != m || err != test.err {
			t.Errorf("delete(%v, %v)=%v,%v, want %v,%v",
				test.n, test.at, n, err, m, test.err)
			continue
		}
		if test.err != nil {
			continue
		}
		got, err := ioutil.ReadAll(&bytesReader{b: b})
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

	b := NewBytes(testBlockSize)
	defer b.Close()
	n, err := b.insert(bs, 0)
	if n != l || err != nil {
		t.Fatalf(`Initial insert(%v, 0)=%v,%v, want %v,nil`, bs, n, err, l)
	}
	if len(b.blocks) != 2 {
		t.Fatalf("After initial insert: len(b.blocks)=%v, want 2", len(b.blocks))
	}

	m, err := b.delete(int64(l), 0)
	if m != int64(l) || err != nil {
		t.Fatalf(`delete(%v, 0)=%v,%v, want 5,nil`, l, m, err)
	}
	if len(b.blocks) != 0 {
		t.Fatalf("After delete: len(b.blocks)=%v, want 0", len(b.blocks))
	}
	if len(b.free) != 2 {
		t.Fatalf("After delete: len(b.free)=%v, want 2", len(b.free))
	}

	bs = bs[:testBlockSize/2]
	l = len(bs)

	n, err = b.insert(bs, 0)
	if n != l || err != nil {
		t.Fatalf(`Second insert(%v, 7)=%v,%v, want %v,nil`, bs, n, err, l)
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
	b := NewBytes(testBlockSize)
	defer b.Close()

	const hiWorld = "Hello, World!"
	n, err := b.insert([]byte(hiWorld), 0)
	if l := len(hiWorld); n != l || err != nil {
		t.Fatalf(`insert(%s, 0)=%v,%v, want %v,nil`, hiWorld, n, err, l)
	}
	bs, err := ioutil.ReadAll(&bytesReader{b: b})
	if s := string(bs); s != hiWorld || err != nil {
		t.Fatalf(`ReadAll(·)=%v,%v, want %s,nil`, s, err, hiWorld)
	}

	m, err := b.delete(5, 7)
	if m != 5 || err != nil {
		t.Fatalf(`delete(5, 7)=%v,%v, want 5,nil`, m, err)
	}
	bs, err = ioutil.ReadAll(&bytesReader{b: b})
	if s := string(bs); s != "Hello, !" || err != nil {
		t.Fatalf(`ReadAll(·)=%v,%v, want "Hello, !",nil`, s, err)
	}

	const gophers = "Gophers"
	n, err = b.insert([]byte(gophers), 7)
	if l := len(gophers); n != l || err != nil {
		t.Fatalf(`insert(%s, 7)=%v,%v, want %v,nil`, gophers, n, err, l)
	}
	bs, err = ioutil.ReadAll(&bytesReader{b: b})
	if s := string(bs); s != "Hello, Gophers!" || err != nil {
		t.Fatalf(`ReadAll(·)=%v,%v, want "Hello, Gophers!",nil`, s, err)
	}
}

type bytesReader struct {
	b *Bytes
	q int64
}

func (r *bytesReader) Read(bs []byte) (int, error) {
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

// Initializes a buffer with the text "01234567abcd!@#efghSTUVWXYZ"
// split across blocks of sizes: 8, 4, 3, 4, 8.
func makeTestBytes(t *testing.T) *Bytes {
	b := NewBytes(testBlockSize)
	// Add 3 full blocks.
	n, err := b.insert([]byte("01234567abcdefghSTUVWXYZ"), 0)
	if n != 24 || err != nil {
		b.Close()
		t.Fatalf(`insert("01234567abcdefghSTUVWXYZ", 0)=%v,%v, want 24,nil`, n, err)
	}
	// Split block 1 in the middle.
	n, err = b.insert([]byte("!@#"), 12)
	if n != 3 || err != nil {
		b.Close()
		t.Fatalf(`insert("!@#", 12)=%v,%v, want 3,nil`, n, err)
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
