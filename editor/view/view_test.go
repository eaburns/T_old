// Copyright © 2016, The T Authors.

package view

import (
	"net/url"
	"path"
	"reflect"
	"testing"
	"time"

	"github.com/eaburns/T/edit"
	"github.com/eaburns/T/editor"
	"github.com/eaburns/T/editor/editortest"
)

func TestNew(t *testing.T) {
	bufferURL, close := testBuffer()
	defer close()
	setText(bufferURL, "1\n2\n3\n")

	v, err := New(bufferURL)
	if err != nil {
		t.Fatalf("New(%q)=_,%v, want _,nil", bufferURL, err)
	}
	defer v.Close()

	wantMarks := []Mark{{Name: ViewMark}}
	v.View(func(text []byte, marks []Mark) {
		if len(text) != 0 || !reflect.DeepEqual(wantMarks, marks) {
			t.Errorf("v.View(·)=%v,%v, want {},%v", text, marks, wantMarks)
		}
	})
}

func TestResizeScroll(t *testing.T) {
	const lines = "1\n2\n3\n"
	tests := []struct {
		size   int
		scroll []int
		want   string
	}{
		{size: -1, want: ""},
		{size: 0, want: ""},
		{size: 1, want: "1\n"},
		{size: 2, want: "1\n2\n"},
		{size: 3, want: "1\n2\n3\n"},
		{size: 4, want: "1\n2\n3\n"},
		{size: 100, want: "1\n2\n3\n"},

		{size: 1, scroll: []int{-1}, want: "1\n"},
		{size: 1, scroll: []int{0}, want: "1\n"},
		{size: 1, scroll: []int{1}, want: "2\n"},
		{size: 1, scroll: []int{2}, want: "3\n"},
		{size: 1, scroll: []int{3}, want: ""},
		{size: 1, scroll: []int{100}, want: ""},

		{size: 2, scroll: []int{-1}, want: "1\n2\n"},
		{size: 2, scroll: []int{0}, want: "1\n2\n"},
		{size: 2, scroll: []int{1}, want: "2\n3\n"},
		{size: 2, scroll: []int{2}, want: "3\n"},
		{size: 2, scroll: []int{3}, want: ""},
		{size: 2, scroll: []int{100}, want: ""},

		{size: 1, scroll: []int{-1, -1}, want: "1\n"},
		{size: 1, scroll: []int{0, -1}, want: "1\n"},
		{size: 1, scroll: []int{1, -1}, want: "1\n"},
		{size: 1, scroll: []int{2, -1}, want: "2\n"},
		{size: 1, scroll: []int{3, -1}, want: "3\n"},
		{size: 1, scroll: []int{4, -1}, want: "3\n"},

		{size: 100, scroll: []int{100}, want: ""},
		{size: 100, scroll: []int{-100}, want: "1\n2\n3\n"},
		{size: 100, scroll: []int{100, -50}, want: "1\n2\n3\n"},
		{size: 100, scroll: []int{-100, 50}, want: ""},
	}

	bufferURL, close := testBuffer()
	defer close()
	setText(bufferURL, lines)

	for _, test := range tests {
		v, err := New(bufferURL)
		if err != nil {
			t.Fatalf("New(%q)=_,%v, want _,nil", bufferURL, err)
		}
		if v.Resize(test.size) {
			wait(v)
		}
		for _, s := range test.scroll {
			if s != 0 {
				v.Scroll(s)
				wait(v)
			}
		}
		v.View(func(text []byte, marks []Mark) {
			if str := string(text); str != test.want {
				t.Errorf("size %d, scroll %v: v.View(·)=%q,%v, want %q,_",
					test.size, test.scroll, str, marks, test.want)
			}
		})
		if err := v.Close(); err != nil {
			t.Fatalf("v.Close()=%v\n", err)
		}
	}
}

func TestWarp(t *testing.T) {
	const lines = "1\n2\n3\n4\n5\n6\n7\n8\n9\n0\n"
	tests := []struct {
		size int
		warp edit.Address
		want string
	}{
		{size: 1, warp: edit.Rune(0), want: "1\n"},
		{size: 1, warp: edit.All, want: "1\n"},
		{size: 1, warp: edit.End, want: ""},
		{size: 1, warp: edit.Line(0), want: "1\n"},
		{size: 1, warp: edit.Line(1), want: "1\n"},
		{size: 1, warp: edit.Line(2), want: "2\n"},
		{size: 1, warp: edit.Line(8), want: "8\n"},
		{size: 1, warp: edit.Line(9), want: "9\n"},
		{size: 1, warp: edit.Line(10), want: "0\n"},
		{size: 1, warp: edit.Clamp(edit.Line(11)), want: ""},
		{size: 1, warp: edit.Regexp("5"), want: "5\n"},
		{size: 1, warp: edit.Regexp("5\n6\n7"), want: "5\n"},
	}

	bufferURL, close := testBuffer()
	defer close()
	setText(bufferURL, lines)

	for _, test := range tests {
		v, err := New(bufferURL)
		if err != nil {
			t.Fatalf("New(%q)=_,%v, want _,nil", bufferURL, err)
		}
		if v.Resize(test.size) {
			wait(v)
		}
		v.Warp(test.warp)
		wait(v)
		v.View(func(text []byte, marks []Mark) {
			if str := string(text); str != test.want {
				t.Errorf("size %d, warp %s: v.View(·)=%q,%v, want %q,_",
					test.size, test.warp, str, marks, test.want)
			}
		})
		if err := v.Close(); err != nil {
			t.Fatalf("v.Close()=%v\n", err)
		}
	}
}

type doTest struct {
	name               string
	init               string
	size, scroll       int
	do                 edit.Edit
	want, error, print string
}

var doTests = []doTest{
	{
		name: "change all",
		init: "1\n2\n3\n4\n5\n6\n7\n8\n9\n0\n",
		size: 1,
		do:   edit.Change(edit.All, "Hello, World\n"),
		want: "Hello, World\n",
	},
	{
		name: "delete all",
		init: "1\n2\n3\n4\n5\n6\n7\n8\n9\n0\n",
		size: 1,
		do:   edit.Delete(edit.All),
		want: "",
	},
	{
		name: "change in view",
		init: "1\n2\n3\n4\n5\n6\n7\n8\n9\n0\n",
		size: 100,
		do:   edit.Change(edit.Regexp("4\n5\n6"), "6\n5\n4"),
		want: "1\n2\n3\n6\n5\n4\n7\n8\n9\n0\n",
	},
	{
		name: "delete in view",
		init: "1\n2\n3\n4\n5\n6\n7\n8\n9\n0\n",
		size: 100,
		do:   edit.Delete(edit.Line(2).To(edit.Line(9))),
		want: "1\n0\n",
	},
	{
		name:   "delete before",
		init:   "1\n2\n3\n4\n5\n6\n7\n8\n9\n0\n",
		size:   1,
		scroll: 1,
		do:     edit.Delete(edit.Line(1)),
		want:   "2\n",
	},
	{
		name:   "delete preceding newline",
		init:   "1\n2\n3\n4\n5\n6\n7\n8\n9\n0\n",
		size:   3,
		scroll: 1,
		do:     edit.Change(edit.Regexp(`1\n`), "1"),
		want:   "12\n3\n4\n",
	},
}

func TestDo(t *testing.T) {
	tests := append(doTests,
		doTest{
			name:  "error",
			init:  "1\n2\n3\n4\n5\n6\n7\n8\n9\n0\n",
			size:  1,
			do:    edit.Change(edit.Regexp("no match"), "Hello, World"),
			want:  "1\n",
			error: "no match",
		},
		doTest{
			name:  "print",
			init:  "1\n2\n3\n4\n5\n6\n7\n8\n9\n0\n",
			size:  1,
			do:    edit.Print(edit.Line(5).To(edit.Line(7))),
			want:  "1\n",
			print: "5\n6\n7\n",
		})

	bufferURL, close := testBuffer()
	defer close()

	for _, test := range tests {
		setText(bufferURL, test.init)

		v, err := New(bufferURL)
		if err != nil {
			t.Fatalf("New(%q)=_,%v, want _,nil", bufferURL, err)
		}

		if v.Resize(test.size) {
			wait(v)
		}

		v.Scroll(test.scroll)
		if test.scroll != 0 {
			wait(v)
		}

		result, err := v.Do(test.do)
		if err != nil {
			t.Fatalf("%s: v.Do(%q)=%v,%v, want _,nil", test.name, test.do, result, err)
		}

		wait(v)
		v.View(func(text []byte, marks []Mark) {
			if str := string(text); str != test.want {
				t.Errorf("%s: v.View(·)=%q,%v, want %q,_", test.name, str, marks, test.want)
			}
		})
		if len(result) != 1 {
			t.Fatalf("%s: len(result)=%d, want 1", test.name, len(result))
		}
		r := result[0]
		if r.Print != test.print {
			t.Errorf("%s: r.Print=%q, want %q", test.name, r.Print, test.print)
		}
		if r.Error != test.error {
			t.Errorf("%s: r.Error=%q, want %q", test.name, r.Error, test.error)
		}
		if err := v.Close(); err != nil {
			t.Fatalf("v.Close()=%v\n", err)
		}
	}
}

func TestConcurrentChange(t *testing.T) {
	bufferURL, close := testBuffer()
	defer close()

	for _, test := range doTests {
		if test.error != "" || test.print != "" {
			panic(test.name + " error and print must not be set")
		}

		setText(bufferURL, test.init)

		v, err := New(bufferURL)
		if err != nil {
			t.Fatalf("New(%q)=_,%v, want _,nil", bufferURL, err)
		}

		if v.Resize(test.size) {
			wait(v)
		}

		v.Scroll(test.scroll)
		if test.scroll != 0 {
			wait(v)
		}

		// Make a change using a different editor.
		do(bufferURL, test.do)

		wait(v)
		v.View(func(text []byte, marks []Mark) {
			if str := string(text); str != test.want {
				t.Errorf("%s: v.View(·)=%q,%v, want %q,_", test.name, str, marks, test.want)
			}
		})

		if err := v.Close(); err != nil {
			t.Fatalf("v.Close()=%v\n", err)
		}
	}
}

func TestTrackMarks(t *testing.T) {
	bufferURL, close := testBuffer()
	defer close()

	const lines = "1\n2\n3\n4\n5\n6\n7\n8\n9\n0\n"
	setText(bufferURL, lines)

	v, err := New(bufferURL, 'm', '.')
	if err != nil {
		t.Fatalf("New(%q, 'm', '.')=_,%v, want _,nil", bufferURL, err)
	}
	defer v.Close()

	if v.Resize(4) {
		wait(v)
	}

	v.DoAsync(edit.Set(edit.Rune(5), 'm'), edit.Set(edit.Rune(10), '.'))
	wait(v)

	want := [2]int64{5, 5}
	if got, ok := markAddr(v, 'm'); !ok || got != want {
		t.Errorf("mark['m']=%v,%v, want %v,true", got, ok, want)
	}

	want = [2]int64{10, 10}
	if got, ok := markAddr(v, '.'); !ok || got != want {
		t.Errorf("mark['.']=%v,%v, want %v,true", got, ok, want)
	}

	v.DoAsync(edit.Delete(edit.Rune(1).To(edit.Rune(2))))
	wait(v)

	want = [2]int64{4, 4}
	if got, ok := markAddr(v, 'm'); !ok || got != want {
		t.Errorf("mark['m']=%v,%v, want %v,true", got, ok, want)
	}

	want = [2]int64{1, 1}
	if got, ok := markAddr(v, '.'); !ok || got != want {
		t.Errorf("mark['.']=%v,%v, want %v,true", got, ok, want)
	}
}

// TestMaintainDot checks that, whatever we do under the hood, we don't affect dot.
func TestMaintainDot(t *testing.T) {
	bufferURL, close := testBuffer()
	defer close()

	v, err := New(bufferURL, 'm')
	if err != nil {
		t.Fatalf("New(%q, 'm')=_,%v, want _,nil", bufferURL, err)
	}
	defer v.Close()

	if v.Resize(1) {
		wait(v)
	}

	str := "Hello, 世界\n"
	v.DoAsync(edit.Change(edit.All, str))
	wait(v)

	e := edit.Print(edit.Dot)
	result, err := v.Do(e)
	if err != nil || len(result) != 1 || result[0].Print != str {
		t.Errorf("v.Do(%q)=%v,%v, want []EditResult{{Print: %q}}, nil", e, result, err, str)
	}
}

func TestMalformedEditError(t *testing.T) {
	bufferURL, close := testBuffer()
	defer close()

	v, err := New(bufferURL, 'm')
	if err != nil {
		t.Fatalf("New(%q, 'm')=_,%v, want _,nil", bufferURL, err)
	}
	defer v.Close()

	// The regexp is malformed.
	badEdit := edit.Print(edit.Regexp(`][`))
	res, err := v.Do(badEdit)
	if err == nil {
		t.Errorf("v.Do(%q)=%v,%v, want _,non-nil", badEdit, res, err)
	}
}

func markAddr(v *View, name rune) ([2]int64, bool) {
	var ok bool
	var where [2]int64
	v.View(func(_ []byte, marks []Mark) {
		for _, m := range marks {
			if m.Name == name {
				ok = true
				where = m.Where
			}
		}
	})
	return where, ok
}

func setText(bufferURL *url.URL, str string) {
	do(bufferURL, edit.Change(edit.All, str))
}

func do(bufferURL *url.URL, e edit.Edit) {
	ed, err := editor.NewEditor(bufferURL)
	if err != nil {
		panic(err)
	}
	editorURL := *bufferURL
	editorURL.Path = ed.Path
	defer editor.Close(&editorURL)

	textURL := editorURL
	textURL.Path = path.Join(ed.Path, "text")
	res, err := editor.Do(&textURL, e)
	if err != nil {
		panic(err)
	}
	if len(res) != 1 {
		panic("expected 1 result")
	}
	if res[0].Error != "" {
		panic(res[0].Error)
	}
}

func wait(v *View) {
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case <-v.Notify:
	case <-timer.C:
		panic("timed out")
	}
}

func testBuffer() (bufferURL *url.URL, close func()) {
	s := editortest.NewServer(editor.NewServer())
	b, err := editor.NewBuffer(s.PathURL("/", "buffers"))
	if err != nil {
		panic(err)
	}
	return s.PathURL(b.Path), s.Close
}
