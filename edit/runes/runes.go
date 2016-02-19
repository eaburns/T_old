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
// instead of a slice of bytes,
// and returns the number of runes read
// instead of the number of bytes read.
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

type utf8Reader struct {
	r   Reader
	buf *bytes.Buffer
}

// UTF8Reader returns a buffering io.Reader that reads UTF8 from r.
func UTF8Reader(r Reader) io.Reader {
	return utf8Reader{r: r, buf: bytes.NewBuffer(nil)}
}

func (r utf8Reader) Read(p []byte) (int, error) {
	if err := r.fill(len(p)); err != nil {
		return 0, err
	}
	return r.buf.Read(p)
}

func (r utf8Reader) fill(min int) error {
	if r.buf.Len() > 0 {
		return nil
	}
	if min < MinRead {
		min = MinRead
	}
	rs := make([]rune, min)
	n, err := r.r.Read(rs)
	for i := 0; i < n; i++ {
		if _, err := r.buf.WriteRune(rs[i]); err != nil {
			return err
		}
	}
	if err != nil && err != io.EOF {
		return err
	}
	return nil
}

type utf8Writer struct{ w io.Writer }

// UTF8Writer returns a Writer that writes UTF8 to w.
func UTF8Writer(w io.Writer) Writer { return utf8Writer{w} }

func (w utf8Writer) Write(p []rune) (int, error) {
	for i, r := range p {
		var e [utf8.UTFMax]byte
		sz := utf8.EncodeRune(e[:], r)
		switch n, err := w.w.Write(e[:sz]); {
		case n < sz:
			return i, err
		case err != nil:
			return i + 1, err
		}
	}
	return len(p), nil
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

type byteReader struct {
	s []byte
}

// ByteReader returns a Reader that reads runes from a []byte.
func ByteReader(s []byte) Reader { return &byteReader{s} }

// Len returns the number of runes in the unread portion of the string.
func (r *byteReader) Len() int64 { return int64(utf8.RuneCount(r.s)) }

func (r *byteReader) Read(p []rune) (int, error) {
	for i := range p {
		if len(r.s) == 0 {
			return i, io.EOF
		}
		var w int
		p[i], w = utf8.DecodeRune(r.s)
		r.s = r.s[w:]
	}
	return len(p), nil
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

type runesReader struct{ r io.RuneReader }

// RunesReader returns a Reader that reads from an io.RuneReader.
func RunesReader(r io.RuneReader) Reader { return runesReader{r} }

func (r runesReader) Read(p []rune) (int, error) {
	for i := range p {
		ru, _, err := r.r.ReadRune()
		if err != nil {
			return i, err
		}
		p[i] = ru
	}
	return len(p), nil
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
// It returns the number of runes written or the first error encountered, if any.
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
