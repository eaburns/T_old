// Pieces copied from go/test/bench/go1/regexp_test.go,
// which requires the following notice:
// Copyright 2013 The Go Authors. All rights reserved.

package runes

import (
	"math/rand"
	"testing"
)

const benchBlockSize = 4096

func randomRunes(n int) []rune {
	rand.Seed(0)
	rs := make([]rune, n)
	for i := range rs {
		rs[i] = rune(rand.Int())
	}
	return rs
}

func writeBench(b *testing.B, n int) {
	r := NewBuffer(benchBlockSize)
	defer r.Close()
	rs := randomRunes(n)
	b.SetBytes(int64(n * runeBytes))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Insert(rs, 0)
		r.Delete(int64(n), 0)
	}
}

func BenchmarkWrite1(b *testing.B)   { writeBench(b, 1) }
func BenchmarkWrite1k(b *testing.B)  { writeBench(b, 1024) }
func BenchmarkWrite4k(b *testing.B)  { writeBench(b, 4096) }
func BenchmarkWrite10k(b *testing.B) { writeBench(b, 1048576) }

func readBench(b *testing.B, n int) {
	r := NewBuffer(benchBlockSize)
	defer r.Close()
	r.Insert(randomRunes(n), 0)
	rs := make([]rune, n)
	b.SetBytes(int64(n * runeBytes))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Read(rs, 0)
	}
}

func BenchmarkRead1(b *testing.B)   { readBench(b, 1) }
func BenchmarkRead1k(b *testing.B)  { readBench(b, 1024) }
func BenchmarkRead4k(b *testing.B)  { readBench(b, 4096) }
func BenchmarkRead10k(b *testing.B) { readBench(b, 1048576) }

func benchmarkRune(b *testing.B, n int, rnd bool) {
	r := NewBuffer(benchBlockSize)
	defer r.Close()
	r.Insert(randomRunes(n), 0)

	inds := make([]int64, 4096)
	for i := range inds {
		if rnd {
			inds[i] = rand.Int63n(int64(n))
		} else {
			inds[i] = int64(i % n)
		}
	}

	b.SetBytes(runeBytes)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Rune(inds[i%len(inds)])
	}

}

func BenchmarkRune10kRand(b *testing.B)   { benchmarkRune(b, 1048576, true) }
func BenchmarkRune10kScan(b *testing.B)   { benchmarkRune(b, 1048576, false) }
func BenchmarkRuneCacheRand(b *testing.B) { benchmarkRune(b, benchBlockSize, true) }
func BenchmarkRuneCacheScan(b *testing.B) { benchmarkRune(b, benchBlockSize, false) }
