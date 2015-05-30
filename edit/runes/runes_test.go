// Copyright © 2015, The T Authors.

package runes

import (
	"bytes"
	"io"
	"reflect"
	"strings"
	"testing"
)

var (
	helloWorldTestRunes = []rune("Hello, World! αβξ")
	helloWorldReadTests = readTests{
		{0, ""},
		{5, "Hello"},
		{2, ", "},
		{6, "World!"},
		{0, ""},
		{1, " "},
		{100, "αβξ"},
	}
)

// A Reader that is not special-cased for any fast paths.
type testReader struct{ Reader }

func (r testReader) Read(p []rune) (int, error) { return r.Reader.Read(p) }

// A Writer that is not special-cased for any fast paths.
type testWriter struct{ Writer }

func (w testWriter) Write(p []rune) (int, error) { return w.Writer.Write(p) }

func TestReadAll(t *testing.T) {
	manyRunes := make([]rune, MinRead*1.5)
	for i := range manyRunes {
		manyRunes[i] = rune(i)
	}
	tests := [][]rune{
		helloWorldTestRunes,
		manyRunes,
	}
	for _, test := range tests {
		r := SliceReader(test)
		rs, err := ReadAll(r)
		if !reflect.DeepEqual(rs, test) || err != nil {
			t.Errorf("ReadAll(·)=%q,%v, want %q,<nil>", string(rs), err, string(test))
		}
	}
}

func TestSliceReader(t *testing.T) {
	r := SliceReader(helloWorldTestRunes)
	helloWorldReadTests.run(t, r)
}

func TestStringReader(t *testing.T) {
	r := StringReader(string(helloWorldTestRunes))
	helloWorldReadTests.run(t, r)
}

func TestRunesReader(t *testing.T) {
	r := RunesReader(strings.NewReader(string(helloWorldTestRunes)))
	helloWorldReadTests.run(t, r)
}

// TestLimitedReaderBigReader tests the LimitedReader
// where the underlying reader is bigger than the limit.
func TestLimitedReaderBigReader(t *testing.T) {
	left := int64(len(helloWorldTestRunes))
	bigRunes := make([]rune, left*10)
	copy(bigRunes, helloWorldTestRunes)
	r := LimitReader(SliceReader(bigRunes), left)
	helloWorldReadTests.run(t, r)
}

// TestLimitedReaderSmallReader tests the LimitedReader
// where the underlying reader is smaller than the limit.
func TestLimitedReaderSmallReader(t *testing.T) {
	// Chop off the last 3 runes,
	// and the last readTest element.
	rs := helloWorldTestRunes[:len(helloWorldTestRunes)-3]
	tests := helloWorldReadTests[:len(helloWorldReadTests)-1]

	left := int64(len(helloWorldTestRunes))
	r := LimitReader(SliceReader(rs), left)
	tests.run(t, r)
}

type readTests []struct {
	n    int
	want string
}

func (tests readTests) run(t *testing.T, r Reader) {
	for _, test := range tests {
		w := []rune(test.want)
		p := make([]rune, test.n)
		m, err := r.Read(p)
		if m != len(w) || !reflect.DeepEqual(p[:m], w) || (err != nil && err != io.EOF) {
			t.Errorf("Read(len=%d)=%d,%v; %q want %d,<nil>; %q",
				test.n, m, err, string(p[:m]), len(w), test.want)
			return
		}
	}
	n, err := r.Read(make([]rune, 1))
	if n != 0 || err != io.EOF {
		t.Errorf("Read(len=1)=%d,%v, want 0,io.EOF", n, err)
	}
}

func TestUTF8Writer(t *testing.T) {
	tests := []struct {
		writes []string
		want   string
	}{
		{[]string{""}, ""},
		{[]string{"Hello,", "", " ", "", "World!"}, "Hello, World!"},
		{[]string{"Hello", ",", " ", "World!"}, "Hello, World!"},
		{[]string{"Hello", ",", " ", "世界!"}, "Hello, 世界!"},
	}
	for _, test := range tests {
		b := bytes.NewBuffer(nil)
		w := UTF8Writer(b)
		for _, write := range test.writes {
			rs := []rune(write)
			n, err := w.Write(rs)
			if n != len(rs) || err != nil {
				t.Errorf("w.Write(%q)=%d,%v, want %d,nil", test.writes, n, err, len(rs))
			}
		}
		if str := b.String(); str != test.want {
			t.Errorf("write %#v, want=%q, got %q", test.writes, str, test.want)
		}
	}
}

func TestCopy(t *testing.T) {
	for _, test := range insertTests {
		rs := []rune(test.add)
		n := int64(len(rs))
		bSrc := NewBuffer(testBlockSize)
		defer bSrc.Close()
		if err := bSrc.Insert(rs, 0); err != nil {
			t.Fatalf("b.Insert(%q, 0)=%v, want nil", rs, err)
		}
		srcs := []func() Reader{
			func() Reader { return StringReader(string(rs)) },
			func() Reader { return SliceReader(rs) },
			func() Reader { return bSrc.Reader(0) },
			func() Reader { return LimitReader(bSrc.Reader(0), n) },
		}
		// Fast path.
		for _, src := range srcs {
			bDst := NewBuffer(testBlockSize)
			defer bDst.Close()
			test.initBuffer(t, bDst)
			testCopy(t, test, bDst, bDst.Writer(test.at), src())
		}
		// Slow path.
		for _, src := range srcs {
			bDst := NewBuffer(testBlockSize)
			defer bDst.Close()
			test.initBuffer(t, bDst)
			testCopy(t, test, bDst, testWriter{bDst.Writer(test.at)}, src())
		}
	}
}

func TestCopySmallLimiterReader(t *testing.T) {
	srcRunes := []rune{'☺', '☹', '☹', '☹', '☹'}
	test := insertTest{
		init: "abcdef",
		add:  string(srcRunes[:1]),
		at:   3,
		want: "abc☺def",
		err:  "",
	}

	bSrc := NewBuffer(testBlockSize)
	defer bSrc.Close()
	if err := bSrc.Insert(srcRunes, 0); err != nil {
		t.Fatalf("b.Insert(%q, 0)=%v, want nil", srcRunes, err)
	}
	src := LimitReader(bSrc.Reader(0), 1)

	bDst := NewBuffer(testBlockSize)
	defer bDst.Close()
	test.initBuffer(t, bDst)
	dst := bDst.Writer(test.at)

	testCopy(t, test, bDst, dst, src)
}

func testCopy(t *testing.T, test insertTest, bDst *Buffer, dst Writer, src Reader) {
	n, err := Copy(dst, src)
	add := []rune(test.add)
	if !errMatch(test.err, err) || (n != int64(len(add)) && test.err == "") {
		t.Errorf("Copy(%#v, %#v)=%v,%v, want %v,%q",
			dst, src, n, err, len(test.add), test.err)
	}
	if test.err != "" {
		return
	}
	if s := readAll(bDst); s != test.want || err != nil {
		t.Errorf("Copy(%#v, %#v); readAll(·)=%q,%v, want %q,nil",
			dst, src, s, err, test.want)
		return
	}
}
