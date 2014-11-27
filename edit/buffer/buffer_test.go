package buffer

import (
	"io"
	"io/ioutil"
	"reflect"
	"testing"
)

const testBlockSize = 8

func TestReadAt(t *testing.T) {
	// Initializes a buffer with the text "01234567abcd!@#efghSTUVWXYZ"
	// split across blocks of sizes: 8, 4, 3, 4, 8.
	b := New(testBlockSize)
	defer b.Close()
	// Add 3 full blocks.
	n, err := b.AddAt([]byte("01234567abcdefghSTUVWXYZ"), 0)
	if n != 24 || err != nil {
		t.Fatalf(`AddAt("01234567abcdefghSTUVWXYZ", 0)=%v,%v, want 24,nil`, n, err)
	}
	// Split block 1 in the middle.
	n, err = b.AddAt([]byte("!@#"), 12)
	if n != 3 || err != nil {
		t.Fatalf(`AddAt("!@#", 12)=%v,%v, want 3,nil`, n, err)
	}
	ns := make([]int, len(b.blocks))
	for i, blk := range b.blocks {
		ns[i] = blk.n
	}
	if !reflect.DeepEqual(ns, []int{8, 4, 3, 4, 8}) {
		t.Fatalf("blocks have sizes %v, want 8, 4, 3, 4, 8", ns)
	}

	tests := []struct {
		n    int
		at   int64
		want string
		err  error
	}{
		{n: 1, at: 27, err: io.EOF},
		{n: 1, at: 28, err: io.EOF},
		{n: 1, at: -1, err: ErrBadAddress},
		{n: 1, at: -2, err: ErrBadAddress},

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

func TestAddAt(t *testing.T) {
	tests := []struct {
		init, add string
		at        int64
		want      string
		err       error
	}{
		{init: "", add: "0", at: -1, err: ErrBadAddress},
		{init: "", add: "0", at: 1, err: ErrBadAddress},
		{init: "0", add: "1", at: 2, err: ErrBadAddress},

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
			n, err := b.AddAt([]byte(test.init), 0)
			if n != len(test.init) || err != nil {
				t.Errorf("%+v init failed: AddAt(%v, 0)=%v,%v, want %v,nil",
					test, test.init, n, err, len(test.init))
				continue
			}
		}
		n, err := b.AddAt([]byte(test.add), test.at)
		wantn := len(test.add)
		if test.err != nil {
			wantn = 0
		}
		if n != wantn || err != test.err {
			t.Errorf("%+v add failed: AddAt(%v, %v)=%v,%v, want %v,%v",
				test, test.add, test.at, n, err, wantn, test.err)
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
