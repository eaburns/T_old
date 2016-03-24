// Copyright © 2016, The T Authors.

// Package editor provides a server that serves an HTTP editor API,
// and functions convenient client access to the server.
package editor

import (
	"bytes"
	"errors"

	"github.com/eaburns/T/edit"
)

// A Buffer describes a buffer.
type Buffer struct {
	// ID is the ID of the buffer.
	ID string `json:"id"`

	// Path is the path to the buffer's resource.
	Path string `json:"path"`

	// Sequence is the sequence number of the last edit on the buffer.
	Sequence int `json:"sequence"`

	// Editors containts the buffer's editors.
	Editors []Editor `json:"editors"`
}

// An Editor describes an editor.
type Editor struct {
	// ID is the ID of the editor.
	ID string `json:"id"`

	// Path is the path to the editor's resource.
	Path string `json:"path"`

	// BufferPath is the path to the editor's buffer's resource.
	BufferPath string `json:"bufferPath"`
}

type editRequest struct{ edit.Edit }

func (e *editRequest) MarshalText() ([]byte, error) { return []byte(e.String()), nil }

func (e *editRequest) UnmarshalText(text []byte) error {
	var err error
	r := bytes.NewReader(text)
	if e.Edit, err = edit.Ed(r); err != nil {
		return err
	}
	if l := r.Len(); l != 0 {
		return errors.New("unexpected trailing text: " + string(text[l:]))
	}
	return nil
}

// An EditResult is result of performing an edito on a buffer.
type EditResult struct {
	// Sequence is the sequence number unique to the edit.
	Sequence int `json:"sequence"`

	// Print is any data that the edit printed.
	Print string `json:"print,omitempty"`

	// Error is any error that occurred.
	Error string `json:"error,omitempty"`
}
