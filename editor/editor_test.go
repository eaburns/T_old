package editor

import (
	"io/ioutil"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/eaburns/T/edit"
	"github.com/gorilla/mux"
)

type testServer struct {
	editorServer *Server
	httpServer   *httptest.Server
	url          *url.URL
}

func newServer() *testServer {
	router := mux.NewRouter()
	editorServer := NewServer()
	editorServer.RegisterHandlers(router)
	httpServer := httptest.NewServer(router)
	url, err := url.Parse(httpServer.URL)
	if err != nil {
		panic(err)
	}
	return &testServer{
		editorServer: editorServer,
		httpServer:   httpServer,
		url:          url,
	}
}

func (s *testServer) close() {
	s.editorServer.Close()
	s.httpServer.Close()
}

type bufferSlice []Buffer

func (s bufferSlice) Len() int           { return len(s) }
func (s bufferSlice) Less(i, j int) bool { return s[i].ID < s[j].ID }
func (s bufferSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

type editorSlice []Editor

func (s editorSlice) Len() int           { return len(s) }
func (s editorSlice) Less(i, j int) bool { return s[i].ID < s[j].ID }
func (s editorSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func urlWithPath(u *url.URL, elems ...string) *url.URL {
	v := *u
	v.Path = path.Join(elems...)
	return &v
}

func TestBufferList(t *testing.T) {
	s := newServer()
	defer s.close()

	buffersURL := urlWithPath(s.url, "/", "buffers")

	// Empty.
	if bufs, err := BufferList(buffersURL); err != nil || len(bufs) != 0 {
		t.Errorf("BufferList(%q)=%v,%v, want [],nil", buffersURL, bufs, err)
	}

	var want []Buffer
	for i := 0; i < 3; i++ {
		buf, err := NewBuffer(buffersURL)
		if err != nil {
			t.Fatalf("NewBuffer(%q)=%v,%v, want _,nil", buffersURL, buf, err)
		}
		want = append(want, buf)
	}
	bufs, err := BufferList(buffersURL)
	sort.Sort(bufferSlice(bufs))
	if err != nil || !reflect.DeepEqual(bufs, want) {
		t.Errorf("BufferList(%q)=%v,%v, want %v,nil", buffersURL, bufs, err, want)
	}
}

func TestNewBuffer(t *testing.T) {
	s := newServer()
	defer s.close()

	buffersURL := urlWithPath(s.url, "/", "buffers")

	var bufs []Buffer
	for i := 0; i < 3; i++ {
		buf, err := NewBuffer(buffersURL)
		if err != nil {
			t.Errorf("NewBuffer(%q)=%v,%v, want _,nil", buffersURL, buf, err)
			continue
		}
		for j, b := range bufs {
			if b.ID == buf.ID {
				t.Errorf("bufs[%d].ID == %s == bufs[%d].ID", i, buf.ID, j)
			}
		}
		bufs = append(bufs, buf)
	}
}

func TestBufferInfo(t *testing.T) {
	s := newServer()
	defer s.close()

	buffersURL := urlWithPath(s.url, "/", "buffers")

	for i := 0; i < 3; i++ {
		buf, err := NewBuffer(buffersURL)
		if err != nil {
			t.Fatalf("NewBuffer(%q)=%v,%v, want _,nil", buffersURL, buf, err)
		}

		bufferURL := urlWithPath(s.url, buf.Path)
		got, err := BufferInfo(bufferURL)
		if err != nil || !reflect.DeepEqual(got, buf) {
			t.Errorf("BufferInfo(%q)=%v,%v, want %v,nil", bufferURL, got, err, buf)
		}
	}

	notFoundURL := urlWithPath(s.url, "/", "buffer", "notfound")
	buf, err := BufferInfo(notFoundURL)
	if err != ErrNotFound {
		t.Errorf("BufferInfo(%q)=%v,%v, want _,%v", notFoundURL, buf, err, ErrNotFound)
	}
}

func TestCloseBuffer(t *testing.T) {
	s := newServer()
	defer s.close()

	buffersURL := urlWithPath(s.url, "/", "buffers")

	var bufs []Buffer
	for i := 0; i < 3; i++ {
		buf, err := NewBuffer(buffersURL)
		if err != nil {
			t.Fatalf("NewBuffer(%q)=%v,%v, want _,nil", buffersURL, buf, err)
		}
		bufs = append(bufs, buf)
	}

	for _, buf := range bufs {
		bufferURL := urlWithPath(s.url, buf.Path)
		if err := Close(bufferURL); err != nil {
			t.Errorf("Close(%q)=%v, want nil", bufferURL, err)
		}
	}
	if got, err := BufferList(buffersURL); err != nil || len(got) != 0 {
		t.Errorf("BufferList(%q)=%v,%v, want [],nil", buffersURL, got, err)
	}

	notFoundURL := urlWithPath(s.url, "/", "buffer", "notfound")
	if err := Close(notFoundURL); err != ErrNotFound {
		t.Errorf("Close(%q)=%v, want %v", notFoundURL, err, ErrNotFound)
	}
}

func TestCloseBuffer_ClosesEditors(t *testing.T) {
	s := newServer()
	defer s.close()

	buffersURL := urlWithPath(s.url, "/", "buffers")

	buf, err := NewBuffer(buffersURL)
	if err != nil {
		t.Fatalf("NewBuffer(%q)=%v,%v, want _,nil", buffersURL, buf, err)
	}

	bufferURL := urlWithPath(s.url, buf.Path)
	ed, err := NewEditor(bufferURL)
	if err != nil {
		t.Fatalf("NewEditor(%q)=%v,%v, want _,nil", bufferURL, ed, err)
	}

	if err := Close(bufferURL); err != nil {
		t.Errorf("Close(%q)=%v, want nil", bufferURL, err)
	}

	if n := len(s.editorServer.editors); n != 0 {
		t.Errorf("len(s.editorServer.editors)=%d, want 0", n)
	}
}

func TestNewEditor(t *testing.T) {
	s := newServer()
	defer s.close()

	buffersURL := urlWithPath(s.url, "/", "buffers")
	buf, err := NewBuffer(buffersURL)
	if err != nil {
		t.Fatalf("NewBuffer(%q)=%v,%v, want _,nil", buffersURL, buf, err)
	}

	bufferURL := urlWithPath(s.url, buf.Path)
	var eds []Editor
	for i := 0; i < 3; i++ {
		ed, err := NewEditor(bufferURL)
		if err != nil {
			t.Errorf("NewEditor(%q)=%v,%v, want _,nil", bufferURL, ed, err)
		}
		for j, e := range eds {
			if e.ID == ed.ID {
				t.Errorf("eds[%d].ID == %s == eds[%d].ID", i, ed.ID, j)
			}
		}
		eds = append(eds, ed)
	}

	buf, err = BufferInfo(bufferURL)
	if err != nil {
		t.Errorf("BufferInfo(%q)=%v,%v, want _,nil", bufferURL, buf, err)
	}
	sort.Sort(editorSlice(buf.Editors))
	sort.Sort(editorSlice(eds))
	if !reflect.DeepEqual(buf.Editors, eds) {
		t.Errorf("buf.Editors=%v, want %v\n", buf.Editors, eds)
	}

	notFoundURL := urlWithPath(s.url, "/", "buffer", "notfound")
	if got, err := NewEditor(notFoundURL); err != ErrNotFound {
		t.Errorf("NewEditor(%q)=%v,%v, want _,%v", notFoundURL, got, err, ErrNotFound)
	}
}

func TestEditorInfo(t *testing.T) {
	s := newServer()
	defer s.close()

	buffersURL := urlWithPath(s.url, "/", "buffers")
	buf, err := NewBuffer(buffersURL)
	if err != nil {
		t.Fatalf("NewBuffer(%q)=%v,%v, want _,nil", buffersURL, buf, err)
	}

	bufferURL := urlWithPath(s.url, buf.Path)
	for i := 0; i < 3; i++ {
		ed, err := NewEditor(bufferURL)
		if err != nil {
			t.Fatalf("NewEditor(%q)=%v,%v, want _,nil", bufferURL, ed, err)
		}

		editorURL := urlWithPath(s.url, ed.Path)
		got, err := EditorInfo(editorURL)
		if err != nil || got != ed {
			t.Errorf("EditorInfo(%q)=%v,%v, want %v,nil", editorURL, got, err, ed)
		}
	}

	notFoundURL := urlWithPath(s.url, "/", "editor", "notfound")
	if got, err := EditorInfo(notFoundURL); err != ErrNotFound {
		t.Errorf("EditorInfo(%q)=%v,%v, want _,%v", notFoundURL, got, err, ErrNotFound)
	}
}

func TestCloseEditor(t *testing.T) {
	s := newServer()
	defer s.close()

	buffersURL := urlWithPath(s.url, "/", "buffers")
	buf, err := NewBuffer(buffersURL)
	if err != nil {
		t.Fatalf("NewBuffer(%q)=%v,%v, want _,nil", buffersURL, buf, err)
	}

	bufferURL := urlWithPath(s.url, buf.Path)
	var eds []Editor
	for i := 0; i < 3; i++ {
		ed, err := NewEditor(bufferURL)
		if err != nil {
			t.Fatalf("NewEditor(%q)=%v,%v, want _,nil", bufferURL, buf, err)
		}
		eds = append(eds, ed)
	}

	sort.Sort(editorSlice(eds))
	for i, ed := range eds {
		editorURL := urlWithPath(s.url, ed.Path)
		if err := Close(editorURL); err != nil {
			t.Errorf("Close(%q)=%v, want nil", editorURL, err)
		}
		buf, err := BufferInfo(bufferURL)
		if err != nil {
			t.Errorf("BufferInfo(%q)=%v,%v, want _,nil", bufferURL, buf, err)
		}
		sort.Sort(editorSlice(buf.Editors))
		if want := eds[i+1:]; !reflect.DeepEqual(buf.Editors, want) {
			t.Errorf("buf.Editors=%v, want %v\n", buf.Editors, want)
		}
	}

	notFoundURL := urlWithPath(s.url, "/", "editor", "notfound")
	if err := Close(notFoundURL); err != ErrNotFound {
		t.Errorf("Close(%q)=%v, want %v", notFoundURL, err, ErrNotFound)
	}
}

func TestDo(t *testing.T) {
	s := newServer()
	defer s.close()

	buffersURL := urlWithPath(s.url, "/", "buffers")
	buf, err := NewBuffer(buffersURL)
	if err != nil {
		t.Fatalf("NewBuffer(%q)=%v,%v, want _,nil", buffersURL, buf, err)
	}

	bufferURL := urlWithPath(s.url, buf.Path)
	ed, err := NewEditor(bufferURL)
	if err != nil {
		t.Fatalf("NewEditor(%q)=%v,%v, want _,nil", bufferURL, buf, err)
	}

	const hi = "Hello, �������界"
	edits := []edit.Edit{
		edit.Print(edit.Line(100)), // 1
		edit.Append(edit.All, hi),  // 2
		edit.Print(edit.All),       // 3
	}
	want := []EditResult{
		{Sequence: 1, Error: edit.RangeError(0).Error()},
		{Sequence: 2},
		{Sequence: 3, Print: hi},
	}
	textURL := urlWithPath(s.url, ed.Path, "text")
	got, err := Do(textURL, edits...)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Do(%q, %v...)=%v,%v, want %v,nil", textURL, edits, got, err, want)
	}
}

func TestDo_Nothing(t *testing.T) {
	s := newServer()
	defer s.close()

	buffersURL := urlWithPath(s.url, "/", "buffers")
	buf, err := NewBuffer(buffersURL)
	if err != nil {
		t.Fatalf("NewBuffer(%q)=%v,%v, want _,nil", buffersURL, buf, err)
	}

	bufferURL := urlWithPath(s.url, buf.Path)
	ed, err := NewEditor(bufferURL)
	if err != nil {
		t.Fatalf("NewEditor(%q)=%v,%v, want _,nil", bufferURL, buf, err)
	}

	textURL := urlWithPath(s.url, ed.Path, "text")
	got, err := Do(textURL)
	if err != nil || len(got) != 0 {
		t.Errorf("Do(%q)=%v,%v, want [],nil", textURL, got, err)
	}
}

func TestDo_NotFound(t *testing.T) {
	s := newServer()
	defer s.close()

	notFoundURL := urlWithPath(s.url, "/", "editor", "notfound", "text")
	if _, err := Do(notFoundURL); err != ErrNotFound {
		t.Errorf("Do(%q)=_,%v, want %v", notFoundURL, err, ErrNotFound)
	}
}

func TestDo_BadRequest(t *testing.T) {
	s := newServer()
	defer s.close()

	buffersURL := urlWithPath(s.url, "/", "buffers")
	buf, err := NewBuffer(buffersURL)
	if err != nil {
		t.Fatalf("NewBuffer(%q)=%v,%v, want _,nil", buffersURL, buf, err)
	}

	bufferURL := urlWithPath(s.url, buf.Path)
	ed, err := NewEditor(bufferURL)
	if err != nil {
		t.Fatalf("NewEditor(%q)=%v,%v, want _,nil", bufferURL, buf, err)
	}

	textURL := urlWithPath(s.url, ed.Path, "text")
	for _, e := range []string{`not json`, `["badEdit"]`, `["c/a/b/leftover"]`} {
		badEdit := strings.NewReader(e)
		req, err := http.NewRequest(http.MethodPost, textURL.String(), badEdit)
		if err != nil {
			t.Fatalf("http.NewRequest(%v, %q, nil)=_,%v, want _,nil", http.MethodPost, textURL, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusBadRequest {
			t.Errorf("http.DefaultClient.Do(%v %v)=%v,%v, want %v,nil",
				req.Method, req.URL, resp.StatusCode, err, http.StatusBadRequest)
		}
	}
}

func TestEditorEdit_UpdateMarks(t *testing.T) {
	s := newServer()
	defer s.close()

	buffersURL := urlWithPath(s.url, "/", "buffers")
	buf, err := NewBuffer(buffersURL)
	if err != nil {
		t.Fatalf("NewBuffer(%q)=%v,%v, want _,nil", buffersURL, buf, err)
	}

	bufferURL := urlWithPath(s.url, buf.Path)
	ed, err := NewEditor(bufferURL)
	if err != nil {
		t.Fatalf("NewEditor(%q)=%v,%v, want _,nil", bufferURL, buf, err)
	}

	const hi = "Hello, 世界!"
	edits := []edit.Edit{
		edit.Append(edit.All, hi),
		edit.Set(edit.Regexp("世界"), 'm'),
		edit.Change(edit.Regexp("Hello"), "hi"),
		edit.Print(edit.Mark('.')),
		edit.Print(edit.Mark('m')),
	}
	want := []EditResult{
		{Sequence: 1},
		{Sequence: 2},
		{Sequence: 3},
		{Sequence: 4, Print: "hi"},
		{Sequence: 5, Print: "世界"},
	}
	textURL := urlWithPath(s.url, ed.Path, "text")
	got, err := Do(textURL, edits...)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Do(%q, %v...)=%v,%v, want %v,nil", textURL, edits, got, err, want)
	}
}

func TestEditorEdit_MultipleEditors(t *testing.T) {
	s := newServer()
	defer s.close()

	buffersURL := urlWithPath(s.url, "/", "buffers")
	buf, err := NewBuffer(buffersURL)
	if err != nil {
		t.Fatalf("NewBuffer(%q)=%v,%v, want _,nil", buffersURL, buf, err)
	}

	bufferURL := urlWithPath(s.url, buf.Path)

	ed0, err := NewEditor(bufferURL)
	if err != nil {
		t.Fatalf("NewEditor(%q)=%v,%v, want _,nil", bufferURL, buf, err)
	}
	text0URL := urlWithPath(s.url, ed0.Path, "text")

	ed1, err := NewEditor(bufferURL)
	if err != nil {
		t.Fatalf("NewEditor(%q)=%v,%v, want _,nil", bufferURL, buf, err)
	}
	text1URL := urlWithPath(s.url, ed1.Path, "text")

	const hi = "Hello, 世界!"
	edits := []edit.Edit{
		edit.Append(edit.All, hi),        // 1
		edit.Set(edit.Regexp("世界"), 'm'), // 2
	}
	if _, err := Do(text0URL, edits...); err != nil {
		t.Errorf("Do(%q, %v...)=_,%v, want _,nil", text0URL, edits, err)
	}

	edits = []edit.Edit{
		edit.Change(edit.Regexp("Hello"), "hi"), // 3
	}
	if _, err := Do(text1URL, edits...); err != nil {
		t.Errorf("Do(%q, %v...)=_,%v, want _,nil", text1URL, edits, err)
	}

	edits = []edit.Edit{
		edit.Print(edit.Mark('.')),        // 4
		edit.Print(edit.Mark('m')),        // 5
		edit.Insert(edit.Line(0), "Oh, "), // 6
	}
	want := []EditResult{
		{Sequence: 4, Print: ", 世界!"},
		{Sequence: 5, Print: "世界"},
		{Sequence: 6},
	}
	if got, err := Do(text0URL, edits...); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Do(%q, %v...)=%v,%v, want %v,nil", text0URL, edits, got, err, want)
	}

	edits = []edit.Edit{
		edit.Print(edit.Mark('.')), // 7
		edit.Print(edit.All),       // 8
	}
	want = []EditResult{
		{Sequence: 7, Print: "hi"},
		{Sequence: 8, Print: "Oh, hi, 世界!"},
	}
	if got, err := Do(text1URL, edits...); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Do(%q, %v...)=%v,%v, want %v,nil", text1URL, edits, got, err, want)
	}
}

func TestReader(t *testing.T) {
	const line1 = "Hello, World\n"
	const hi = line1 + "☺☹\n←→\n"

	s := newServer()
	defer s.close()

	buffersURL := urlWithPath(s.url, "/", "buffers")
	buf, err := NewBuffer(buffersURL)
	if err != nil {
		t.Fatalf("NewBuffer(%q)=%v,%v, want _,nil", buffersURL, buf, err)
	}

	bufferURL := urlWithPath(s.url, buf.Path)
	ed, err := NewEditor(bufferURL)
	if err != nil {
		t.Fatalf("NewEditor(%q)=%v,%v, want _,nil", bufferURL, buf, err)
	}

	textURL := urlWithPath(s.url, ed.Path, "text")

	// Empty reader.
	r, err := Reader(textURL, nil)
	if err != nil {
		t.Fatalf("Reader(%v,nil)=_,%v, want _,nil", textURL, err)
	}
	data, err := ioutil.ReadAll(r)
	if err != nil || len(data) != 0 {
		t.Errorf("ioutil.ReadAll(r)=%v,%v, want [],nil", data, err)
	}

	edits := []edit.Edit{edit.Append(edit.All, hi)}
	if resp, err := Do(textURL, edits...); err != nil {
		t.Fatalf("Do(%q, %v...)=%v,%v, want _,nil", textURL, edits, resp, err)
	}

	// Read everything.
	r, err = Reader(textURL, nil)
	if err != nil {
		t.Fatalf("Reader(%v,nil)=_,%v, want _,nil", textURL, err)
	}
	data, err = ioutil.ReadAll(r)
	if str := string(data); err != nil || str != hi {
		t.Errorf("ioutil.ReadAll(r)=%q,%v, want %q,nil", str, err, hi)
	}
	r.Close()

	// Read from an address.
	r, err = Reader(textURL, edit.Line(1))
	if err != nil {
		t.Fatalf("Reader(%v,nil)=_,%v, want _,nil", textURL, err)
	}
	data, err = ioutil.ReadAll(r)
	if str := string(data); err != nil || str != line1 {
		t.Errorf("ioutil.ReadAll(r)=%q,%v, want %q,nil", str, err, line1)
	}
	r.Close()

	// Address requires escaping
	r, err = Reader(textURL, edit.Regexp("Hello")) // /Hello/, / must be escaped.
	if err != nil {
		t.Fatalf("Reader(%v,nil)=_,%v, want _,nil", textURL, err)
	}
	data, err = ioutil.ReadAll(r)
	if str := string(data); err != nil || str != "Hello" {
		t.Errorf("ioutil.ReadAll(r)=%q,%v, want %q,nil", str, err, "Hello")
	}
	r.Close()

	// Out of range.
	r, err = Reader(textURL, edit.Line(100))
	if err != ErrRange {
		t.Fatalf("Reader(%v,nil)=_,%v, want _,%v", textURL, err, ErrRange)
	}
	if err == nil {
		r.Close()
	}

	// Not found.
	notFoundURL := urlWithPath(s.url, "/", "editor", "notfound", "text")
	r, err = Reader(notFoundURL, nil)
	if err != ErrNotFound {
		t.Errorf("Do(%q)=_,%v, want %v", notFoundURL, err, ErrNotFound)
	}
	if err == nil {
		r.Close()
	}

	// Multiple Addrs.
	multiAddrURL := *textURL
	multiAddrURL.RawQuery = "addr=0"
	r, err = Reader(&multiAddrURL, edit.Line(1))
	if err == nil {
		r.Close()
		t.Fatalf("Reader(%v,nil)=_,%v, want _,<non-nil>", textURL, err)
	}

	// Bad addr.
	badAddrURL := *textURL
	badAddrURL.RawQuery = "addr=" + strconv.FormatInt(math.MaxInt64, 10) + "0"
	r, err = Reader(&badAddrURL, nil)
	if err == nil {
		r.Close()
		t.Fatalf("Reader(%v,nil)=_,%v, want _,<non-nil>", textURL, err)
	}

	// Leftover after addr.
	leftoverAddrURL := *textURL
	leftoverAddrURL.RawQuery = "addr=1hi"
	r, err = Reader(&leftoverAddrURL, nil)
	if err == nil {
		r.Close()
		t.Fatalf("Reader(%v,nil)=_,%v, want _,<non-nil>", textURL, err)
	}

	// Malformed parameters
	badParamsURL := *textURL
	badParamsURL.RawQuery = "addr=%G" // G is not valid hex.
	r, err = Reader(&badParamsURL, nil)
	if err == nil {
		r.Close()
		t.Fatalf("Reader(%v,nil)=_,%v, want _,<non-nil>", textURL, err)
	}
}

func TestChangeStream(t *testing.T) {
	s := newServer()
	defer s.close()

	buffersURL := urlWithPath(s.url, "/", "buffers")
	buf, err := NewBuffer(buffersURL)
	if err != nil {
		t.Fatalf("NewBuffer(%q)=%v,%v, want _,nil", buffersURL, buf, err)
	}

	bufferURL := urlWithPath(s.url, buf.Path)
	ed, err := NewEditor(bufferURL)
	if err != nil {
		t.Fatalf("NewEditor(%q)=%v,%v, want _,nil", bufferURL, buf, err)
	}

	changesURL := urlWithPath(s.url, buf.Path, "changes")
	changesURL.Scheme = "ws"
	var watchers [3]*ChangeStream
	for i := range watchers {
		var err error
		watchers[i], err = Changes(changesURL)
		if err != nil {
			t.Fatalf("Changes(%q)=_,%v, want _,nil", changesURL, err)
		}
		defer watchers[i].Close()
	}

	var hi = "Hello, 世界!" + strings.Repeat("x", MaxInline)
	eds := []edit.Edit{
		edit.Insert(edit.All, hi),               // 1
		edit.Change(edit.Regexp("世界"), "World"), // 2
		edit.SubGlobal(edit.All, ",|!", "."),    // 3
		edit.Delete(edit.All),                   // 4
	}
	textURL := urlWithPath(s.url, ed.Path, "text")
	if res, err := Do(textURL, eds...); err != nil {
		t.Fatalf("ed.Do(%q, %v...)=%v,%v want _,nil", textURL, eds, res, err)
	}

	wants := []ChangeList{
		ChangeList{
			Sequence: 1,
			Changes: []Change{
				{
					Span:    edit.Span{0: 0, 1: 0},
					NewSize: int64(utf8.RuneCountInString(hi)),
				},
			},
		},
		ChangeList{
			Sequence: 2,
			Changes: []Change{
				{
					Span:    edit.Span{0: 7, 1: 9},
					NewSize: int64(utf8.RuneCountInString("World")),
					Text:    []byte("World"),
				},
			},
		},
		ChangeList{
			Sequence: 3,
			Changes: []Change{
				{
					Span:    edit.Span{0: 5, 1: 6},
					NewSize: 1,
					Text:    []byte("."),
				},
				{
					Span:    edit.Span{0: 12, 1: 13},
					NewSize: 1,
					Text:    []byte("."),
				},
			},
		},
		ChangeList{
			Sequence: 4,
			Changes: []Change{
				{
					// +3, because 世界 changed to World.
					Span:    edit.Span{0: 0, 1: int64(utf8.RuneCountInString(hi)) + 3},
					NewSize: 0,
				},
			},
		},
	}
	for _, want := range wants {
		for i := range watchers {
			got, err := watchers[i].Next()
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Errorf("watchers[%d].Next()=%v,%v, want %v,nil", i, got, err, want)
			}
		}
	}
}

func TestChangeStream_Close(t *testing.T) {
	s := newServer()
	defer s.close()

	buffersURL := urlWithPath(s.url, "/", "buffers")
	buf, err := NewBuffer(buffersURL)
	if err != nil {
		t.Fatalf("NewBuffer(%q)=%v,%v, want _,nil", buffersURL, buf, err)
	}

	s.editorServer.buffers[buf.ID].watcherRemoved = make(chan struct{})

	changesURL := urlWithPath(s.url, buf.Path, "changes")
	changesURL.Scheme = "ws"
	changes, err := Changes(changesURL)
	if err != nil {
		t.Fatalf("Changes(%q)=_,%v, want _,nil", changesURL, err)
	}

	if err := changes.Close(); err != nil {
		t.Fatalf("changes.Close()=%v, want nil", err)
	}

	// Wait 1 second for the server to receive the close
	// and call the defer to remove the watcher.
	// This is suboptimal, but better than nothing.
	select {
	case <-s.editorServer.buffers[buf.ID].watcherRemoved:
	case <-time.After(1 * time.Second):
		t.Errorf("timed out waiting for watcher to close")
	}
}

func TestChangeStream_BufferClose(t *testing.T) {
	s := newServer()
	defer s.close()

	buffersURL := urlWithPath(s.url, "/", "buffers")
	buf, err := NewBuffer(buffersURL)
	if err != nil {
		t.Fatalf("NewBuffer(%q)=%v,%v, want _,nil", buffersURL, buf, err)
	}

	changesURL := urlWithPath(s.url, buf.Path, "changes")
	changesURL.Scheme = "ws"
	changes, err := Changes(changesURL)
	if err != nil {
		t.Fatalf("Changes(%q)=_,%v, want _,nil", changesURL, err)
	}

	bufferURL := urlWithPath(s.url, buf.Path)
	if err := Close(bufferURL); err != nil {
		t.Fatalf("buf.Close()=%v, want nil", err)
	}

	if _, err := changes.Next(); err == nil {
		t.Errorf("changes.Next()=_,%v, want non-nil", err)
	}
}

func TestChangeStream_NotFound(t *testing.T) {
	s := newServer()
	defer s.close()

	changesURL := urlWithPath(s.url, "buffer", "notfound", "changes")
	changesURL.Scheme = "ws"
	changes, err := Changes(changesURL)
	if err != ErrNotFound {
		t.Errorf("Changes(%q)=_,%v, want _,%v", changesURL, err, ErrNotFound)
	}
	if err == nil {
		changes.Close()
	}
}
