// Copyright © 2016, The T Authors.

package edit

import "io"

// A Text provides a read-only view of a sequence of text.
//
// Strings of the text are identified by Spans.
// The unit of measurement for a Span is unspecified;
// it is determined by the implementation
// using the Size method and the width return of RuneReader.RuneRead.
type Text interface {
	// Size returns the size of the Text.
	Size() int64

	// Mark returns the Span of a mark.
	// If the range was never set, Mark returns Span{}.
	Mark(rune) Span

	// RuneReader returns a RuneReader that reads runes from the given Span.
	//
	// If the Size of the Span is negative, the reader returns runes in reverse.
	//
	// If either endpoint of the Span is negative or greater than the Size of the Text,
	// an error is retured by the RuneReader.ReadRune method.
	RuneReader(Span) io.RuneReader
}

// A Span identifies a string within a Text.
type Span [2]int64

// Size returns the size of the Span.
func (s Span) Size() int64 { return s[1] - s[0] }

// Update updates s to account for t changing to size n.
func (s Span) Update(t Span, n int64) Span {
	// Clip, unless t is entirely within s.
	if s[0] >= t[0] || t[1] > s[1] {
		if t.Contains(s[0]) {
			s[0] = t[1]
		}
		if t.Contains(s[1] - 1) {
			s[1] = t[0]
		}
		if s[0] > s[1] {
			s[1] = s[0]
		}
	}
	// Move.
	d := n - t.Size()
	if s[1] >= t[1] {
		s[1] += d
	}
	if s[0] >= t[1] {
		s[0] += d
	}
	return s
}

// Contains returns whether a location is within the Span.
func (s Span) Contains(l int64) bool { return s[0] <= l && l < s[1] }
