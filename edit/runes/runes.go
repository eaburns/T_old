// Copyright © 2015, The T Authors.

// Package runes provides unbounded, file-backed rune buffers
// and io-package-style interfaces for reading and writing rune slices.
package runes

import "io"

// Reader wraps the basic Read method.
// It behaves like io.Reader
// but it accepts a slice of runes
// instead of a slice of bytes.
type Reader interface {
	Read([]rune) (int, error)
}

// Writer wraps the basic Write method.
// It behaves like io.Writer
// but it accepts a slice of runes
// instead of a slice of bytes.
type Writer interface {
	Write([]rune) (int, error)
}

// LimitedReader wraps a Reader,
// limiting the number of runes read.
// When the limit is reached, io.EOF is returned.
type LimitedReader struct {
	Reader
	// N is the the number of bytes to read.
	// It should not be changed after calling Read.
	N int64
	n int64
}

func (r *LimitedReader) Read(p []rune) (int, error) {
	if r.n >= r.N {
		return 0, io.EOF
	}
	n := len(p)
	if max := r.N - r.n; max < int64(n) {
		n = int(max)
	}
	m, err := r.Reader.Read(p[:n])
	r.n += int64(m)
	return m, err
}

// A SliceReader is a Reader
// that reads from a rune slice.
type SliceReader struct {
	Rs []rune
}

func (r *SliceReader) Read(p []rune) (int, error) {
	if len(r.Rs) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.Rs)
	r.Rs = r.Rs[n:]
	return n, nil
}
