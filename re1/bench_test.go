// Copyright © 2015, The T Authors.

// Copied from go/test/bench/go1/regexp_test.go,
// which requires the following notice:
// Copyright 2013 The Go Authors. All rights reserved.

package re1

import (
	"math/rand"
	"strings"
	"testing"
)

// benchmark based on regexp/exec_test.go

var regexpText []byte

func makeRegexpText(n int) []byte {
	rand.Seed(0) // For reproducibility.
	if len(regexpText) >= n {
		return regexpText[:n]
	}
	regexpText = make([]byte, n)
	for i := range regexpText {
		if rand.Intn(30) == 0 {
			regexpText[i] = '\n'
		} else {
			regexpText[i] = byte(rand.Intn(0x7E+1-0x20) + 0x20)
		}
	}
	return regexpText
}

func benchmark(b *testing.B, re string, n int) {
	r, err := Compile(strings.NewReader(re))
	if err != nil {
		panic(err)
	}
	t := makeRegexpText(n)
	rs := sliceRunes([]rune(string(t)))
	b.ResetTimer()
	b.SetBytes(int64(n))
	for i := 0; i < b.N; i++ {
		if r.Match(rs, 0) != nil {
			b.Fatal("match!")
		}
	}
}

const (
	easy0  = "ABCDEFGHIJKLMNOPQRSTUVWXYZ$"
	easy1  = "A[AB]B[BC]C[CD]D[DE]E[EF]F[FG]G[GH]H[HI]I[IJ]J$"
	medium = "[XYZ]ABCDEFGHIJKLMNOPQRSTUVWXYZ$"
	hard   = "[ -~]*ABCDEFGHIJKLMNOPQRSTUVWXYZ$"
)

func BenchmarkRegexpMatchEasy0x32(b *testing.B)   { benchmark(b, easy0, 32<<0) }
func BenchmarkRegexpMatchEasy0x1K(b *testing.B)   { benchmark(b, easy0, 1<<10) }
func BenchmarkRegexpMatchEasy0x1M(b *testing.B)   { benchmark(b, easy0, 1<<20) }
func BenchmarkRegexpMatchEasy0x32M(b *testing.B)  { benchmark(b, easy0, 32<<20) }
func BenchmarkRegexpMatchEasy1x32(b *testing.B)   { benchmark(b, easy1, 32<<0) }
func BenchmarkRegexpMatchEasy1x1K(b *testing.B)   { benchmark(b, easy1, 1<<10) }
func BenchmarkRegexpMatchEasy1x1M(b *testing.B)   { benchmark(b, easy1, 1<<20) }
func BenchmarkRegexpMatchEasy1x32M(b *testing.B)  { benchmark(b, easy1, 32<<20) }
func BenchmarkRegexpMatchMediumx32(b *testing.B)  { benchmark(b, medium, 1<<0) }
func BenchmarkRegexpMatchMediumx1K(b *testing.B)  { benchmark(b, medium, 1<<10) }
func BenchmarkRegexpMatchMediumx1M(b *testing.B)  { benchmark(b, medium, 1<<20) }
func BenchmarkRegexpMatchMediumx32M(b *testing.B) { benchmark(b, medium, 32<<20) }
func BenchmarkRegexpMatchHardx32(b *testing.B)    { benchmark(b, hard, 32<<0) }
func BenchmarkRegexpMatchHardx1K(b *testing.B)    { benchmark(b, hard, 1<<10) }
func BenchmarkRegexpMatchHardx1M(b *testing.B)    { benchmark(b, hard, 1<<20) }
func BenchmarkRegexpMatchHardx32M(b *testing.B)   { benchmark(b, hard, 32<<20) }
