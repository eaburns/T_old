// Copyright © 2016, The T Authors.

package edit

import (
	"errors"
	"io"
)

var (
	// ErrInvalidArgument indicates an invalid, out-of-range Span.
	ErrInvalidArgument = errors.New("invalid argument")

	// ErrOutOfSequence indicates that a change modifies text
	// overlapping or preceeding the previous, staged change.
	ErrOutOfSequence = errors.New("out of sequence")
)

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
	// an ErrInvalidArgument is retured by the RuneReader.ReadRune method.
	RuneReader(Span) io.RuneReader

	// Reader returns a Reader that reads the Span as bytes.
	//
	// An ErrInvalidArgument error is returned by the Reader.Read method if
	// either endpoint of the Span is negative or greater than the Size of the Text,
	// or if the Size of the Span is negative.
	Reader(Span) io.Reader
}

// An Editor provides a read-write view of a sequence of text.
//
// An Editor changes the Text using a two-step procedure.
//
// The first step is to stage a batch of changes
// using repeated calls to the Change method.
// The Change method does not modify the Text,
// but logs the desired change to a staging buffer.
//
// The second step is to apply the staged changes
// by calling the Apply method.
// The Apply method applies the changes to the Text
// in the order that they were added to the staging log.
//
// An Editor also has an Undo stack and a Redo stack.
// The stacks hold batches of changes,
// providing support for unlimited undoing and redoing
// of changes made by calls to Apply, Undo, and Redo.
type Editor interface {
	Text

	// SetMark sets the Span of a mark.
	//
	// ErrInvalidArgument is returned
	// if either endpoint of the Span is negative
	// or greater than the Size of the Text.
	SetMark(rune, Span) error

	// Change stages a change that modifies a Span of text
	// to contain the data from a Reader,
	// to be applied on the next call to Apply,
	// and returns the size of text read from the Reader.
	// This method does not modify the Text.
	//
	// It is an error if a change modifies text
	// overlapping or preceding a previously staged, unapplied change.
	// In such a case, ErrOutOfSequence is returned.
	//
	// If an error is returned, previously staged changes are canceled.
	// They will not be performed on the next call to Apply.
	Change(Span, io.Reader) (int64, error)

	// Apply applies all changes since the previous call to Apply,
	// updates all marks to reflect the changes,
	// logs the applied changes to the Undo stack,
	// and clears the Redo stack.
	Apply() error

	// Undo undoes the changes at the top of the Undo stack.
	// It updates all marks to reflect the changes,
	// and logs the undone changes to the Redo stack.
	Undo() error

	// Redo redoes the changes at the top of the Redo stack.
	// It updates all marks to reflect the changes,
	// and logs the redone changes to the Undo stack.
	Redo() error
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
