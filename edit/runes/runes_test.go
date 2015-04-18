// Copyright © 2015, The T Authors.

package runes

import (
	"io"
	"reflect"
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

func TestSliceReader(t *testing.T) {
	r := &SliceReader{helloWorldTestRunes}
	helloWorldReadTests.run(t, r)
}

// TestLimitedReaderBigReader tests the LimitedReader
// where the underlying reader is bigger than the limit.
func TestLimitedReaderBigReader(t *testing.T) {
	left := int64(len(helloWorldTestRunes))
	bigRunes := make([]rune, left*10)
	copy(bigRunes, helloWorldTestRunes)
	r := &LimitedReader{Reader: &SliceReader{bigRunes}, N: left}
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
	r := &LimitedReader{Reader: &SliceReader{rs}, N: left}
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
