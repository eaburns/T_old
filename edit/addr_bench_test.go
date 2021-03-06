// Copyright © 2015, The T Authors.

// Copied from go/test/bench/go1/regexp_test.go,
// which has the following notice:
//
// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package edit

import (
	"bytes"
	"math/rand"
	"testing"
)

// benchmark based on regexp/exec_test.go

func makeEditor(n int) (buf *Buffer, lines int, runes int64) {
	rand.Seed(0) // For reproducibility.
	data := make([]byte, n)
	for i := 0; i < n; i++ {
		if rand.Intn(30) == 0 {
			lines++
			data[i] = '\n'
		} else {
			data[i] = byte(rand.Intn(0x7E+1-0x20) + 0x20)
		}
	}
	buf = NewBuffer()
	if _, err := buf.Change(Span{}, bytes.NewReader(data)); err != nil {
		panic(err)
	}
	if err := buf.Apply(); err != nil {
		panic(err)
	}
	return buf, lines, int64(n)
}

func benchmarkRune(b *testing.B, n int) {
	ed, _, runes := makeEditor(n)
	defer ed.Close()
	b.ResetTimer()
	b.SetBytes(int64(n))
	for i := 0; i < b.N; i++ {
		if _, err := Rune(runes).Where(ed); err != nil {
			b.Fatal(err.Error())
		}
	}
}

func BenchmarkRunex32(b *testing.B)  { benchmarkRune(b, 32<<0) }
func BenchmarkRunex1K(b *testing.B)  { benchmarkRune(b, 1<<10) }
func BenchmarkRunex1M(b *testing.B)  { benchmarkRune(b, 1<<20) }
func BenchmarkRunex32M(b *testing.B) { benchmarkRune(b, 32<<20) }

func benchmarkLine(b *testing.B, n int) {
	buf, lines, _ := makeEditor(n)
	defer buf.Close()
	if lines == 0 {
		b.Fatalf("too few lines: %d", lines)
	}
	b.ResetTimer()
	b.SetBytes(int64(n))
	for i := 0; i < b.N; i++ {
		if _, err := Line(lines).Where(buf); err != nil {
			b.Fatal(err.Error())
		}
	}
}

func BenchmarkLinex32(b *testing.B)  { benchmarkLine(b, 32<<0) }
func BenchmarkLinex1K(b *testing.B)  { benchmarkLine(b, 1<<10) }
func BenchmarkLinex1M(b *testing.B)  { benchmarkLine(b, 1<<20) }
func BenchmarkLinex32M(b *testing.B) { benchmarkLine(b, 32<<20) }

func benchmarkRegexp(b *testing.B, re string, n int) {
	buf, _, _ := makeEditor(n)
	defer buf.Close()
	b.ResetTimer()
	b.SetBytes(int64(n))
	for i := 0; i < b.N; i++ {
		switch _, err := Regexp(re).Where(buf); {
		case err == nil:
			panic("unexpected match")
		case err != ErrNoMatch:
			panic(err)
		}
	}
}

const (
	easy0  = "ABCDEFGHIJKLMNOPQRSTUVWXYZ$"
	easy1  = "A[AB]B[BC]C[CD]D[DE]E[EF]F[FG]G[GH]H[HI]I[IJ]J$"
	medium = "[XYZ]ABCDEFGHIJKLMNOPQRSTUVWXYZ$"
	hard   = "[ -~]*ABCDEFGHIJKLMNOPQRSTUVWXYZ$"
)

func BenchmarkRegexpEasy0x32(b *testing.B)   { benchmarkRegexp(b, easy0, 32<<0) }
func BenchmarkRegexpEasy0x1K(b *testing.B)   { benchmarkRegexp(b, easy0, 1<<10) }
func BenchmarkRegexpEasy0x1M(b *testing.B)   { benchmarkRegexp(b, easy0, 1<<20) }
func BenchmarkRegexpEasy0x32M(b *testing.B)  { benchmarkRegexp(b, easy0, 32<<20) }
func BenchmarkRegexpEasy1x32(b *testing.B)   { benchmarkRegexp(b, easy1, 32<<0) }
func BenchmarkRegexpEasy1x1K(b *testing.B)   { benchmarkRegexp(b, easy1, 1<<10) }
func BenchmarkRegexpEasy1x1M(b *testing.B)   { benchmarkRegexp(b, easy1, 1<<20) }
func BenchmarkRegexpEasy1x32M(b *testing.B)  { benchmarkRegexp(b, easy1, 32<<20) }
func BenchmarkRegexpMediumx32(b *testing.B)  { benchmarkRegexp(b, medium, 1<<0) }
func BenchmarkRegexpMediumx1K(b *testing.B)  { benchmarkRegexp(b, medium, 1<<10) }
func BenchmarkRegexpMediumx1M(b *testing.B)  { benchmarkRegexp(b, medium, 1<<20) }
func BenchmarkRegexpMediumx32M(b *testing.B) { benchmarkRegexp(b, medium, 32<<20) }
func BenchmarkRegexpHardx32(b *testing.B)    { benchmarkRegexp(b, hard, 32<<0) }
func BenchmarkRegexpHardx1K(b *testing.B)    { benchmarkRegexp(b, hard, 1<<10) }
func BenchmarkRegexpHardx1M(b *testing.B)    { benchmarkRegexp(b, hard, 1<<20) }
func BenchmarkRegexpHardx32M(b *testing.B)   { benchmarkRegexp(b, hard, 32<<20) }
