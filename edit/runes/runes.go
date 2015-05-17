// Copyright © 2015, The T Authors.

// Package runes provides unbounded, file-backed rune buffers
// and io-package-style interfaces for reading and writing rune slices.
package runes

import (
	"bytes"
	"io"
)

// MinRead is the minimum rune buffer size
// passed to a Read call.
const MinRead = bytes.MinRead

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

// ReaderFrom wraps the ReadFrom method.
// It reads runes from the reader
// until there are no more runes to read.
type ReaderFrom interface {
	ReadFrom(Reader) (int64, error)
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

func (r *SliceReader) readSize() int64 { return int64(len(r.Rs)) }

func (r *SliceReader) Read(p []rune) (int, error) {
	if len(r.Rs) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.Rs)
	r.Rs = r.Rs[n:]
	return n, nil
}

// ReadAll reads runes from the reader
// until an error or io.EOF is encountered.
// It returns all of the runes read.
// On success, the error is nil, not io.EOF.
func ReadAll(r Reader) ([]rune, error) {
	var rs []rune
	p := make([]rune, MinRead)
	for {
		n, err := r.Read(p)
		rs = append(rs, p[:n]...)
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return rs, err
		}
	}
}

// Copy copies from src into dst until either EOF is reached or an error occurs.
// It returns the number of bytes written or the first error encountered, if any.
//
// A successful Copy returns err == nil, not err == io.EOF.
//
// If dst implements the ReaderFrom interface,
// the copy is implemented by calling dst.ReadFrom.
func Copy(dst Writer, src Reader) (int64, error) {
	if rf, ok := dst.(ReaderFrom); ok {
		return rf.ReadFrom(src)
	}
	return slowCopy(dst, src)
}

func slowCopy(dst Writer, src Reader) (int64, error) {
	var tot int64
	var buf [MinRead]rune
	for {
		nr, rerr := src.Read(buf[:])
		nw, werr := dst.Write(buf[:nr])
		tot += int64(nw)
		switch {
		case rerr != nil && rerr != io.EOF:
			return tot, rerr
		case werr != nil:
			return tot, werr
		case rerr == io.EOF:
			return tot, nil
		}
	}
}
