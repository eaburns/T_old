// Package edit provides sam-style editing of rune buffers.
// See sam(1) for an overview: http://swtch.com/plan9port/man/man1/sam.html.
package edit

import "github.com/eaburns/T/runes"

// An Editor provides sam-like editing functionality on a buffer of runes.
type Editor struct {
	runes *runes.Buffer
	dot   addr
}
