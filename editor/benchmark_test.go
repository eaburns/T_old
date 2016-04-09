// Copyright © 2016, The T Authors.

package editor

import (
	"testing"

	"github.com/eaburns/T/edit"
)

func BenchmarkDo(b *testing.B) {
	s := newServer()
	defer s.close()

	buffersURL := urlWithPath(s.url, "/", "buffers")
	buf, err := NewBuffer(buffersURL)
	if err != nil {
		panic(err)
	}
	bufferURL := urlWithPath(s.url, buf.Path)
	ed, err := NewEditor(bufferURL)
	if err != nil {
		panic(err)
	}

	textURL := urlWithPath(s.url, ed.Path, "text")
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
