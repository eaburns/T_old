package buffer

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
	r := NewRunes(benchBlockSize)
	defer r.Close()
	rs := randomRunes(n)
	b.SetBytes(int64(n * runeBytes))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Put(rs, Address{})
		r.Put([]rune{}, Address{From: 0, To: r.Size()})
	}
}

func BenchmarkWrite1(b *testing.B)   { writeBench(b, 1) }
func BenchmarkWrite1k(b *testing.B)  { writeBench(b, 1024) }
func BenchmarkWrite4k(b *testing.B)  { writeBench(b, 4096) }
func BenchmarkWrite10k(b *testing.B) { writeBench(b, 1048576) }

func readBench(b *testing.B, n int) {
	r := NewRunes(benchBlockSize)
	defer r.Close()
	r.Put(randomRunes(n), Address{})
	b.SetBytes(int64(n * runeBytes))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Get(Address{From: 0, To: r.Size()})
	}
}

func BenchmarkRead1(b *testing.B)   { readBench(b, 1) }
func BenchmarkRead1k(b *testing.B)  { readBench(b, 1024) }
func BenchmarkRead4k(b *testing.B)  { readBench(b, 4096) }
func BenchmarkRead10k(b *testing.B) { readBench(b, 1048576) }

func BenchmarkRune(b *testing.B) {
	r := NewRunes(benchBlockSize)
	defer r.Close()
	const nRunes = 1048576
	r.Put(randomRunes(nRunes), Address{})

	inds := make([]int64, 4096)
	for i := range inds {
		inds[i] = rand.Int63n(nRunes)
	}

	b.SetBytes(runeBytes)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Rune(inds[i%len(inds)])
	}
}
