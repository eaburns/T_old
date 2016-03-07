package editor

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"path"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/eaburns/T/edit"
	"github.com/gorilla/mux"
)

const tooBigInt = "92233720368547758070"

// badIDs is a slice of bad IDs for buffers and editors.
var badIDs = []string{
	"-1",
	"badID",
	tooBigInt,
}

type testServer struct {
	editorServer *Server
	httpServer   *httptest.Server
	host         string
}

func newServer() *testServer {
	router := mux.NewRouter()
	editorServer := NewServer()
	editorServer.RegisterHandlers(router)
	httpServer := httptest.NewServer(router)
	return &testServer{
		editorServer: editorServer,
		httpServer:   httpServer,
		host:         httpServer.URL[len("http://"):],
	}
}

func (s *testServer) close() {
	s.editorServer.Close()
	s.httpServer.Close()
}

func putRequest(c *Client, elems ...interface{}) *http.Request {
	url := url(c, elems...)
	req, err := http.NewRequest(http.MethodPut, url, nil)
	if err != nil {
		panic(err)
	}
	return req
}

func deleteRequest(c *Client, elems ...interface{}) *http.Request {
	url := url(c, elems...)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		panic(err)
	}
	return req
}

func responseBody(resp *http.Response) string {
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	return string(bytes.TrimSpace(b))
}

// testBadBufferIDs tests a bunch of invalid buffer IDs, and expects NotFound.
// pathSuffix is joined after after /buffer/<N>.
func testBadBufferIDs(t *testing.T, c *Client, method, pathSuffix, body string) {
	for _, id := range badIDs {
		bufferPath := path.Join("/", "buffer", id)
		u := url(c, path.Join(bufferPath, pathSuffix))
		req, err := http.NewRequest(method, u, strings.NewReader(body))
		if err != nil {
			t.Fatalf("http.NewRequest(%v, %q, nil)=_,%v, want _,nil", method, u, err)
		}
		resp, err := c.http.Do(req)
		if err != nil || resp.StatusCode != http.StatusNotFound {
			t.Errorf("c.http.Do(%v %v)=%v,%v, want %v,nil",
				req.Method, req.URL, resp.StatusCode, err, http.StatusNotFound)
		}
		if got := responseBody(resp); got != bufferPath {
			t.Errorf("response body %s want %s", got, bufferPath)
		}
	}
}

// testBadBufferIDs tests a bunch of invalid editor IDs, and expects NotFound.
// pathSuffix is joined after after /buffer/<N>/editor/<M>.
func testBadEditorIDs(t *testing.T, buf *Buffer, method, pathSuffix, body string) {
	bufID := strconv.Itoa(buf.id)
	for _, id := range badIDs {
		editorPath := path.Join("/", "buffer", bufID, "editor", id)
		u := url(buf.client, path.Join(editorPath, pathSuffix))
		req, err := http.NewRequest(method, u, strings.NewReader(body))
		if err != nil {
			t.Fatalf("http.NewRequest(%v, %q, nil)=_,%v, want _,nil", method, u, err)
		}
		resp, err := buf.client.http.Do(req)
		if err != nil || resp.StatusCode != http.StatusNotFound {
			t.Errorf("buf.client.http.Do(%v %v)=%v,%v, want %v,nil",
				req.Method, req.URL, resp.StatusCode, err, http.StatusNotFound)
		}
		if got := responseBody(resp); got != editorPath {
			t.Errorf("response body %s want %s", got, editorPath)
		}
	}
}

func TestBuffers(t *testing.T) {
	s := newServer()
	defer s.close()
	c := NewClient(s.host)

	// Empty.
	if bufs, err := c.Buffers(); err != nil || len(bufs) != 0 {
		t.Errorf("c.Buffers()=%v,%v, want len(·)=0,nil", bufs, err)
	}

	var want []BufferInfo
	for i := 0; i < 3; i++ {
		buf, err := c.NewBuffer()
		if err != nil {
			t.Fatalf("c.NewBuffer()=%v,%v, want _,nil", buf, err)
		}
		info, err := buf.Info()
		if err != nil {
			t.Fatalf("buf.Info()=%v,%v, want _,nil", info, err)
		}
		want = append(want, info)
	}

	if bufs, err := c.Buffers(); err != nil || !reflect.DeepEqual(bufs, want) {
		t.Errorf("c.Buffers()=%v,%v, want %v,nil", bufs, err, want)
	}
}

func TestNewBuffer(t *testing.T) {
	s := newServer()
	defer s.close()
	c := NewClient(s.host)

	for i := 0; i < 3; i++ {
		if buf, err := c.NewBuffer(); err != nil {
			t.Errorf("c.NewBuffer()=%v,%v, want _,nil", buf, err)
		}
	}
}

func TestBufferInfo(t *testing.T) {
	s := newServer()
	defer s.close()
	c := NewClient(s.host)

	for i := 0; i < 3; i++ {
		buf, err := c.NewBuffer()
		if err != nil {
			t.Fatalf("c.NewBuffer()=%v,%v, want _,nil", buf, err)
		}
		info, err := buf.Info()
		if err != nil || info.ID != buf.id {
			t.Errorf("buf.Info()=%v,%v, want BufferInfo{ID: %d, …},nil", info, err, buf.id)
		}
	}
}

func TestBufferInfo_NotFound(t *testing.T) {
	s := newServer()
	defer s.close()
	c := NewClient(s.host)

	info, err := c.Buffer(100).Info()
	if want := NotFoundError("/buffer/100"); err != want {
		t.Errorf("c.Buffer(100).Info()=%v,%v, want BufferInfo{},%v", info, err, want)
	}
	testBadBufferIDs(t, c, http.MethodGet, "", "")
}

func TestBufferClose(t *testing.T) {
	s := newServer()
	defer s.close()
	c := NewClient(s.host)

	var buffers []*Buffer
	for i := 0; i < 3; i++ {
		buf, err := c.NewBuffer()
		if err != nil {
			t.Fatalf("c.NewBuffer()=%v,%v, want _,nil", buf, err)
		}
		buffers = append(buffers, buf)
	}

	for _, buf := range buffers {
		if err := buf.Close(); err != nil {
			t.Errorf("buf.Close()=%v, want nil", err)
		}
	}
	if got, err := c.Buffers(); err != nil || len(got) != 0 {
		t.Errorf("c.Buffers()=%v,%v, want len(·)=0,nil", got, err)
	}
}

func TestBufferClose_NotFound(t *testing.T) {
	s := newServer()
	defer s.close()
	c := NewClient(s.host)

	got := c.Buffer(100).Close()
	if want := NotFoundError("/buffer/100"); got != want {
		t.Errorf("c.Buffer(100).Close()=%v, want %v", got, want)
	}

	testBadBufferIDs(t, c, http.MethodDelete, "", "")
}

func TestEditors(t *testing.T) {
	s := newServer()
	defer s.close()
	c := NewClient(s.host)
	buf, err := c.NewBuffer()
	if err != nil {
		t.Fatalf("c.NewBuffer()=%v,%v, want _,nil", buf, err)
	}

	// Empty.
	if eds, err := buf.Editors(); err != nil || len(eds) != 0 {
		t.Errorf("buf.Editors()=%v,%v, want len(·)=0,nil", eds, err)
	}

	var want []EditorInfo
	for i := 0; i < 3; i++ {
		ed, err := buf.NewEditor()
		if err != nil {
			t.Fatalf("buf.NewEditor()=%v,%v, want _,nil", ed, err)
		}
		info, err := ed.Info()
		if err != nil {
			t.Fatalf("ed.Info()=%v,%v, want _,nil", info, err)
		}
		want = append(want, info)
	}

	if eds, err := buf.Editors(); err != nil || !reflect.DeepEqual(eds, want) {
		t.Errorf("buf.Editors()=%v,%v, want %v,nil", eds, err, want)
	}
}

func TestEditors_NotFound(t *testing.T) {
	s := newServer()
	defer s.close()
	c := NewClient(s.host)

	info, err := c.Buffer(100).Editors()
	if want := NotFoundError("/buffer/100"); err != want {
		t.Errorf("c.Buffer(100).Editors()=%v,%v, want BufferInfo{},%v", info, err, want)
	}

	testBadBufferIDs(t, c, http.MethodGet, "editor", "")
}

func TestNewEditor(t *testing.T) {
	s := newServer()
	defer s.close()
	c := NewClient(s.host)
	buf, err := c.NewBuffer()
	if err != nil {
		t.Fatalf("c.NewBuffer()=%v,%v, want _,nil", buf, err)
	}

	for i := 0; i < 3; i++ {
		if ed, err := buf.NewEditor(); err != nil {
			t.Errorf("buf.NewEditor()=%v,%v, want _,nil", ed, err)
		}
	}
}

func TestNewEditor_Notfound(t *testing.T) {
	s := newServer()
	defer s.close()
	c := NewClient(s.host)

	info, err := c.Buffer(100).NewEditor()
	if want := NotFoundError("/buffer/100"); err != want {
		t.Errorf("c.Buffer(100).NewEditor()=%v,%v, want BufferInfo{},%v", info, err, want)
	}

	testBadBufferIDs(t, c, http.MethodPut, "editor", "")
}

func TestEditorInfo(t *testing.T) {
	s := newServer()
	defer s.close()
	c := NewClient(s.host)
	buf, err := c.NewBuffer()
	if err != nil {
		t.Fatalf("c.NewBuffer()=%v,%v, want _,nil", buf, err)
	}

	for i := 0; i < 3; i++ {
		ed, err := buf.NewEditor()
		if err != nil {
			t.Fatalf("buf.NewEDitor()=%v,%v, want _,nil", ed, err)
		}
		info, err := ed.Info()
		if err != nil || info.ID != ed.id {
			t.Errorf("ed.Info()=%v,%v, want EditorInfo{ID: %d, …},nil", info, err, ed.id)
		}
	}
}

func TestEditorInfo_NotFound(t *testing.T) {
	s := newServer()
	defer s.close()
	c := NewClient(s.host)
	buf, err := c.NewBuffer()
	if err != nil {
		t.Fatalf("c.NewBuffer()=%v,%v, want _,nil", buf, err)
	}

	info, err := c.Buffer(100).Editor(100).Info()
	if want := NotFoundError("/buffer/100"); err != want {
		t.Errorf("c.Buffer(100).Editor(100).Info()=%v,%v, want _,%v", info, err, want)
	}
	info, err = buf.Editor(100).Info()
	if want := NotFoundError("/buffer/0/editor/100"); err != want {
		t.Errorf("buf.Editor(100).Info()=%v,%v, want _,%v", info, err, want)
	}

	testBadBufferIDs(t, c, http.MethodGet, "editor/100", "")
	testBadEditorIDs(t, buf, http.MethodGet, "", "")
}

func TestEditorClose(t *testing.T) {
	s := newServer()
	defer s.close()
	c := NewClient(s.host)
	buf, err := c.NewBuffer()
	if err != nil {
		t.Fatalf("c.NewBuffer()=%v,%v, want _,nil", buf, err)
	}

	var editors []*Editor
	for i := 0; i < 3; i++ {
		ed, err := buf.NewEditor()
		if err != nil {
			t.Fatalf("buf.NewEditor()=%v,%v, want _,nil", buf, err)
		}
		editors = append(editors, ed)
	}

	for _, ed := range editors {
		if err := ed.Close(); err != nil {
			t.Errorf("ed.Close()=%v, want nil", err)
		}
	}
	if got, err := buf.Editors(); err != nil || len(got) != 0 {
		t.Errorf("buf.Editors()=%v,%v, want len(·)=0,nil", got, err)
	}
}

func TestEditorClose_NotFound(t *testing.T) {
	s := newServer()
	defer s.close()
	c := NewClient(s.host)
	buf, err := c.NewBuffer()
	if err != nil {
		t.Fatalf("c.NewBuffer()=%v,%v, want _,nil", buf, err)
	}

	err = c.Buffer(100).Editor(100).Close()
	if want := NotFoundError("/buffer/100"); err != want {
		t.Errorf("c.Buffer(100).Editor(100).Close()=%v, want _,%v", err, want)
	}
	err = buf.Editor(100).Close()
	if want := NotFoundError("/buffer/0/editor/100"); err != want {
		t.Errorf("buf.Editor(100).Close()=%v, want _,%v", err, want)
	}

	testBadBufferIDs(t, c, http.MethodDelete, "editor/100", "")
	testBadEditorIDs(t, buf, http.MethodDelete, "", "")
}

func TestEditorEdit_NotFound(t *testing.T) {
	s := newServer()
	defer s.close()
	c := NewClient(s.host)
	buf, err := c.NewBuffer()
	if err != nil {
		t.Fatalf("c.NewBuffer()=%v,%v, want _,nil", buf, err)
	}

	resp, err := c.Buffer(100).Editor(100).Edit()
	if want := NotFoundError("/buffer/100"); err != want {
		t.Errorf("c.Buffer(100).Editor(100).Edit()=%v,%v, want _,%v", resp, err, want)
	}
	resp, err = buf.Editor(100).Edit()
	if want := NotFoundError("/buffer/0/editor/100"); err != want {
		t.Errorf("buf.Editor(100).Edit()=%v,%v, want _,%v", resp, err, want)
	}

	testBadBufferIDs(t, c, http.MethodPost, "editor/100", "[]")
	testBadEditorIDs(t, buf, http.MethodPost, "", "[]")
}

func TestEditorEdit(t *testing.T) {
	s := newServer()
	defer s.close()
	c := NewClient(s.host)
	buf, err := c.NewBuffer()
	if err != nil {
		t.Fatalf("c.NewBuffer()=%v,%v, want _,nil", buf, err)
	}
	ed, err := buf.NewEditor()
	if err != nil {
		t.Fatalf("buf.NewEditor()=%v,%v, want _,nil", ed, err)
	}

	const hi = "Hello, 世界"
	edits := []edit.Edit{
		edit.Print(edit.Line(100)), // 1
		edit.Append(edit.All, hi),  // 2
		edit.Print(edit.All),       // 3
	}
	want := []EditResponse{
		{Sequence: 1, Error: edit.RangeError(0).Error()},
		{Sequence: 2},
		{Sequence: 3, Print: hi},
	}
	got, err := ed.Edit(edits...)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("ed.Edit(%v...)=%v,%v, want %v,nil", edits, got, err, want)
	}
}

func TestEditorEdit_BadRequest(t *testing.T) {
	s := newServer()
	defer s.close()
	c := NewClient(s.host)
	buf, err := c.NewBuffer()
	if err != nil {
		t.Fatalf("c.NewBuffer()=%v,%v, want _,nil", buf, err)
	}
	ed, err := buf.NewEditor()
	if err != nil {
		t.Fatalf("buf.NewEditor()=%v,%v, want _,nil", ed, err)
	}

	for _, e := range []string{`not json`, `["badEdit"]`, `["c/a/b/leftover"]`} {
		badEdit := strings.NewReader(e)
		u := url(c, "/", "buffer", buf.id, "editor", ed.id)
		req, err := http.NewRequest(http.MethodPost, u, badEdit)
		if err != nil {
			t.Fatalf("http.NewRequest(%v, %q, nil)=_,%v, want _,nil", http.MethodPost, u, err)
		}
		resp, err := c.http.Do(req)
		if err != nil || resp.StatusCode != http.StatusBadRequest {
			t.Errorf("c.http.Do(%v %v)=%v,%v, want %v,nil",
				req.Method, req.URL, resp.StatusCode, err, http.StatusBadRequest)
		}
	}
}

func TestEditorEdit_UpdateMarks(t *testing.T) {
	s := newServer()
	defer s.close()
	c := NewClient(s.host)
	buf, err := c.NewBuffer()
	if err != nil {
		t.Fatalf("c.NewBuffer()=%v,%v, want _,nil", buf, err)
	}
	ed, err := buf.NewEditor()
	if err != nil {
		t.Fatalf("buf.NewEditor()=%v,%v, want _,nil", ed, err)
	}

	const hi = "Hello, 世界!"
	edits := []edit.Edit{
		edit.Append(edit.All, hi),
		edit.Set(edit.Regexp("世界"), 'm'),
		edit.Change(edit.Regexp("Hello"), "hi"),
		edit.Print(edit.Mark('.')),
		edit.Print(edit.Mark('m')),
	}
	want := []EditResponse{
		{Sequence: 1},
		{Sequence: 2},
		{Sequence: 3},
		{Sequence: 4, Print: "hi"},
		{Sequence: 5, Print: "世界"},
	}
	got, err := ed.Edit(edits...)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("ed.Edit(%v...)=%v,%v, want %v,nil", edits, got, err, want)
	}
}

func TestEditorEdit_MultipleEditors(t *testing.T) {
	s := newServer()
	defer s.close()
	c := NewClient(s.host)
	buf, err := c.NewBuffer()
	if err != nil {
		t.Fatalf("c.NewBuffer()=%v,%v, want _,nil", buf, err)
	}
	ed0, err := buf.NewEditor()
	if err != nil {
		t.Fatalf("buf.NewEditor()=%v,%v, want _,nil", ed0, err)
	}
	ed1, err := buf.NewEditor()
	if err != nil {
		t.Fatalf("buf.NewEditor()=%v,%v, want _,nil", ed1, err)
	}

	const hi = "Hello, 世界!"
	edits := []edit.Edit{
		edit.Append(edit.All, hi),        // 1
		edit.Set(edit.Regexp("世界"), 'm'), // 2
	}
	if _, err := ed0.Edit(edits...); err != nil {
		t.Errorf("ed0.Edit(%v...)=_,%v, want _,nil", edits, err)
	}

	edits = []edit.Edit{
		edit.Change(edit.Regexp("Hello"), "hi"), // 3
	}
	if _, err := ed1.Edit(edits...); err != nil {
		t.Errorf("ed1.Edit(%v...)=_,%v, want _,nil", edits, err)
	}

	edits = []edit.Edit{
		edit.Print(edit.Mark('.')),        // 4
		edit.Print(edit.Mark('m')),        // 5
		edit.Insert(edit.Line(0), "Oh, "), // 6
	}
	want := []EditResponse{
		{Sequence: 4, Print: ", 世界!"},
		{Sequence: 5, Print: "世界"},
		{Sequence: 6},
	}
	if got, err := ed0.Edit(edits...); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("ed0.Edit(%v...)=%v,%v, want %v,nil", edits, got, err, want)
	}

	edits = []edit.Edit{
		edit.Print(edit.Mark('.')), // 7
		edit.Print(edit.All),       // 8
	}
	want = []EditResponse{
		{Sequence: 7, Print: "hi"},
		{Sequence: 8, Print: "Oh, hi, 世界!"},
	}
	if got, err := ed1.Edit(edits...); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("ed1.Edit(%v...)=%v,%v, want %v,nil", edits, got, err, want)
	}
}
