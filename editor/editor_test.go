package editor

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"reflect"
	"sort"
	"strings"
	"testing"

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

	const hi = "Hello, ���界"
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

func TestDo_Nohthing(t *testing.T) {
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
