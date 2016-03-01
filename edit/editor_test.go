// Copyright © 2015, The T Authors.

package edit

import (
	"io/ioutil"
	"strings"
	"testing"
)

// String returns a string containing the entire editor contents.
func (ed *Editor) String() string {
	data, err := ioutil.ReadAll(ed.Reader(Span{0, ed.Size()}))
	if err != nil {
		panic(err)
	}
	return string(data)
}

func TestEditorClose(t *testing.T) {
	buf := NewBuffer()
	ed := NewEditor(buf)
	if err := ed.Close(); err != nil {
		t.Fatalf("failed to close the editor: %v", err)
	}
	if err := ed.Close(); err == nil {
		t.Fatal("ed.Close()=nil, want error")
	}
	if err := buf.Close(); err != nil {
		t.Fatalf("failed to close the buffer: %v", err)
	}
}

func TestChangeOutOfSequence(t *testing.T) {
	ed := NewEditor(NewBuffer())
	defer ed.buf.Close()
	const init = "Hello, 世界"
	if err := ed.Change(Span{}, strings.NewReader(init)); err != nil {
		panic(err)
	}
	if err := ed.Apply(); err != nil {
		panic(err)
	}

	if err := ed.Change(Span{10, 20}, strings.NewReader("")); err != nil {
		panic(err)
	}
	if err := ed.Change(Span{0, 10}, strings.NewReader("")); err != nil {
		panic(err)
	}
	if err := ed.Apply(); err == nil {
		t.Error("ed.Apply()=<nil>, want error")
	}
}
