// Copyright © 2015, The T Authors.

// Copied from go/test/bench/go1/regexp_test.go,
// which has the following notice:
//
// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package edit

import (
	"math/rand"
	"testing"
)

// benchmark based on regexp/exec_test.go

func makeEditor(n int) (*Editor, int) {
	rand.Seed(0) // For reproducibility.
	rs := make([]rune, n)
	var lines int
	for i := 0; i < n; i++ {
		if rand.Intn(30) == 0 {
			lines++
			rs[i] = '\n'
		} else {
			rs[i] = rune(rand.Intn(0x7E+1-0x20) + 0x20)
		}
	}
	ed := NewEditor(NewBuffer())
	if err := ed.buf.runes.Insert(rs, 0); err != nil {
		panic(err)
	}
	return ed, lines
}

func benchmarkLine(b *testing.B, n int) {
	ed, lines := makeEditor(n)
	defer ed.buf.Close()
	if lines == 0 {
		b.Fatalf("too few lines: %d", lines)
	}
	b.ResetTimer()
	b.SetBytes(int64(n))
	for i := 0; i < b.N; i++ {
		if _, err := Line(i % lines).where(ed); err != nil {
			b.Fatal(err.Error())
		}
	}
}

func BenchmarkLinex32(b *testing.B) { benchmarkLine(b, 32<<0) }
func BenchmarkLinex1K(b *testing.B) { benchmarkLine(b, 1<<10) }

func benchmarkRegexp(b *testing.B, re string, n int) {
	ed, _ := makeEditor(n)
	b.ResetTimer()
	b.SetBytes(int64(n))
	for i := 0; i < b.N; i++ {
		switch _, err := Regexp(re).where(ed); {
		case err == nil:
			panic("unexpected match")
		case err != ErrNoMatch:
			panic(err)
		}
	}
}

const (
	easy0  = "/ABCDEFGHIJKLMNOPQRSTUVWXYZ$"
	easy1  = "/A[AB]B[BC]C[CD]D[DE]E[EF]F[FG]G[GH]H[HI]I[IJ]J$"
	medium = "/[XYZ]ABCDEFGHIJKLMNOPQRSTUVWXYZ$"
	hard   = "/[ -~]*ABCDEFGHIJKLMNOPQRSTUVWXYZ$"
)

func BenchmarkRegexpEasy0x32(b *testing.B)  { benchmarkRegexp(b, easy0, 32<<0) }
func BenchmarkRegexpEasy0x1K(b *testing.B)  { benchmarkRegexp(b, easy0, 1<<10) }
func BenchmarkRegexpEasy1x32(b *testing.B)  { benchmarkRegexp(b, easy1, 32<<0) }
func BenchmarkRegexpEasy1x1K(b *testing.B)  { benchmarkRegexp(b, easy1, 1<<10) }
func BenchmarkRegexpMediumx32(b *testing.B) { benchmarkRegexp(b, medium, 1<<0) }
func BenchmarkRegexpMediumx1K(b *testing.B) { benchmarkRegexp(b, medium, 1<<10) }
func BenchmarkRegexpHardx32(b *testing.B)   { benchmarkRegexp(b, hard, 32<<0) }
func BenchmarkRegexpHardx1K(b *testing.B)   { benchmarkRegexp(b, hard, 1<<10) }
