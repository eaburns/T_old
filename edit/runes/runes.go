// Copyright © 2015, The T Authors.

// Package runes provides unbounded, file-backed rune buffers
// and io-package-style interfaces for reading and writing rune slices.
package runes

import (
	"bytes"
	"io"
	"unicode/utf8"
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

type limitedReader struct {
	r        Reader
	n, limit int64
}

// LimitReader returns a Reader that reads no more than n runes from r.
func LimitReader(r Reader, n int64) Reader { return &limitedReader{r: r, limit: n} }

func (r *limitedReader) Read(p []rune) (int, error) {
	if r.n >= r.limit {
		return 0, io.EOF
	}
	n := len(p)
	if max := r.limit - r.n; max < int64(n) {
		n = int(max)
	}
	m, err := r.r.Read(p[:n])
	r.n += int64(m)
	return m, err
}

type stringReader struct {
	s string
}

// StringReader returns a Reader that reads runes from a string.
func StringReader(s string) Reader { return &stringReader{s} }

// Len returns the number of runes in the unread portion of the string.
func (r *stringReader) Len() int64 { return int64(utf8.RuneCountInString(r.s)) }

func (r *stringReader) Read(p []rune) (int, error) {
	for i := range p {
		if len(r.s) == 0 {
			return i, io.EOF
		}
		var w int
		p[i], w = utf8.DecodeRuneInString(r.s)
		r.s = r.s[w:]
	}
	return len(p), nil
}

type sliceReader struct {
	rs []rune
}

// SliceReader returns a Reader that reads runes from a slice.
func SliceReader(rs []rune) Reader { return &sliceReader{rs} }

// EmptyReader returns a Reader that is empty.
// All calls to read return 0, io.EOF.
func EmptyReader() Reader { return &sliceReader{} }

// Len returns the number of runes in the unread portion of the slice.
func (r *sliceReader) Len() int64 { return int64(len(r.rs)) }

func (r *sliceReader) Read(p []rune) (int, error) {
	if len(r.rs) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.rs)
	r.rs = r.rs[n:]
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
