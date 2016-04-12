// Copyright © 2016, The T Authors.

package editor

import (
	"testing"

	"github.com/eaburns/T/edit"
	"github.com/eaburns/T/editor/editortest"
)

func BenchmarkDo(b *testing.B) {
	s := editortest.NewServer(NewServer())
	defer s.Close()

	buffersURL := s.PathURL("/", "buffers")
	buf, err := NewBuffer(buffersURL)
	if err != nil {
		panic(err)
	}
	bufferURL := s.PathURL(buf.Path)
	ed, err := NewEditor(bufferURL)
	if err != nil {
		panic(err)
	}

	textURL := s.PathURL(ed.Path, "text")
	if _, err := Do(textURL, edit.Change(edit.All, "Hello, World")); err != nil {
		panic(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Do(textURL, edit.Print(edit.All)); err != nil {
			panic(err)
		}
	}
}
