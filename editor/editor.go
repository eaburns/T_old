// Copyright © 2016, The T Authors.

// Package editor provides a server that serves an HTTP editor API,
// and a client for convenient access to the server.
package editor

import (
	"bytes"
	"errors"

	"github.com/eaburns/T/edit"
)

// A BufferInfo contains meta-information about a buffer.
type BufferInfo struct {
	// ID is the buffer's unique ID.
	ID int `json:"id"`

	// Sequence is the sequence number of the last edit on the buffer.
	Sequence int `json:"sequence"`
}

// An EditorInfo contains meta-information about an editor of a buffer.
type EditorInfo struct {
	// ID is the editor's unique ID.
	ID int `json:"id"`

	// BufferID is the ID of the edited buffer.
	BufferID int `json:"bufferId"`
}

// An EditRequest requests that an editor perform an edit on its buffer.
type EditRequest struct {
	edit.Edit `json:"edit"`
}

func (req *EditRequest) MarshalText() ([]byte, error) { return []byte(req.String()), nil }

func (req *EditRequest) UnmarshalText(text []byte) error {
	r := bytes.NewReader(text)
	e, err := edit.Ed(r)
	if err != nil {
		return err
	}
	if l := r.Len(); l != 0 {
		return errors.New("unexpected trailing text: " + string(text[l:]))
	}
	req.Edit = e
	return nil
}

// An EditResponse contians the result of an edit performed by an editor.
type EditResponse struct {
	// Sequence is the sequence number unique to the edit.
	Sequence int `json:"sequence"`

	// Print is any data that the edit printed.
	Print string `json:"print,omitempty"`

	// Error is any error that occurred.
	Error string `json:"error,omitempty"`
}
