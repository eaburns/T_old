// Copyright © 2016, The T Authors.

package ui

import (
	"image"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/eaburns/T/editor"
	"github.com/eaburns/T/editor/editortest"
	"github.com/gorilla/mux"
	"golang.org/x/exp/shiny/screen"
)

func TestWindowList(t *testing.T) {
	s := newServer(new(stubScreen))
	defer s.close()

	winsURL := urlWithPath(s.url, "/", "windows")

	// Empty.
	if wins, err := WindowList(winsURL); err != nil || len(wins) != 0 {
		t.Errorf("WindowList(%q)=%v,%v, want [],nil", winsURL, wins, err)
	}

	var want []Window
	for i := 0; i < 3; i++ {
		win, err := NewWindow(winsURL, image.Pt(800, 600))
		if err != nil {
			t.Fatalf("NewWindow(%q)=%v,%v, want _,nil", winsURL, win, err)
		}
		want = append(want, win)
	}
	wins, err := WindowList(winsURL)
	sort.Sort(windowSlice(wins))
	sort.Sort(windowSlice(want))
	if err != nil || !reflect.DeepEqual(wins, want) {
		t.Errorf("WindowList(%q)=%v,%v, want %v,nil", winsURL, wins, err, want)
	}
}

func TestNewWindow(t *testing.T) {
	s := newServer(new(stubScreen))
	defer s.close()

	winsURL := urlWithPath(s.url, "/", "windows")

	var wins []Window
	for i := 0; i < 3; i++ {
		win, err := NewWindow(winsURL, image.Pt(800, 600))
		if err != nil {
			t.Fatalf("NewWindow(%q)=%v,%v, want _,nil", winsURL, win, err)
		}
		for j, w := range wins {
			if w.ID == win.ID {
				t.Errorf("%d win.ID= %s = wins[%d].ID", i, w.ID, j)
			}
		}
		wins = append(wins, win)
	}
}

// TODO(eaburns): test that we are actually getting BadRequest errors.
func TestNewWindow_BadRequest(t *testing.T) {
	s := newServer(new(stubScreen))
	defer s.close()
	winsURL := urlWithPath(s.url, "/", "windows")
	var win Window
	req := strings.NewReader("bad request")
	if err := request(winsURL, http.MethodPut, req, &win); err == nil {
		t.Errorf("request(%q, PUT, \"bad request\", &win)=%v, want bad request", winsURL, err)
	}
}

func TestCloseWindow(t *testing.T) {
	s := newServer(new(stubScreen))
	defer s.close()

	winsURL := urlWithPath(s.url, "/", "windows")

	var wins []Window
	for i := 0; i < 3; i++ {
		win, err := NewWindow(winsURL, image.Pt(800, 600))
		if err != nil {
			t.Fatalf("NewWindow(%q)=%v,%v, want _,nil", winsURL, win, err)
		}
		wins = append(wins, win)
	}

	for i, win := range wins {
		winURL := urlWithPath(s.url, win.Path)
		if err := Close(winURL); err != nil {
			t.Errorf("Close(%q)=%v, want nil", winURL, err)
		}
		wantDone := i == len(wins)-1
		if s.done != wantDone {
			t.Errorf("s.done=%v, want %v", s.done, wantDone)
		}
	}

	if got, err := WindowList(winsURL); err != nil || len(got) != 0 {
		t.Errorf("WindowList(%q)=%v,%v, want [],nil", winsURL, got, err)
	}
}

func TestCloseWindow_NotFound(t *testing.T) {
	s := newServer(new(stubScreen))
	defer s.close()
	notFoundURL := urlWithPath(s.url, "/", "window", "notfound")
	if err := Close(notFoundURL); err != ErrNotFound {
		t.Errorf("Close(%q)=%v, want %v", notFoundURL, err, ErrNotFound)
	}
}

func TestNewColumn(t *testing.T) {
	const N = 3
	s := newServer(new(stubScreen))
	defer s.close()

	winsURL := urlWithPath(s.url, "/", "windows")
	win, err := NewWindow(winsURL, image.Pt(800, 600))
	if err != nil {
		t.Fatalf("NewWindow(%q)=%v,%v, want _,nil", winsURL, win, err)
	}
	colsURL := urlWithPath(s.url, win.Path, "columns")
	for i := 0; i < N; i++ {
		if err := NewColumn(colsURL, 0.5); err != nil {
			t.Errorf("NewColumn(%q, 0.5)=%v, want nil", colsURL, err)
		}
	}
	w := s.uiServer.windows[win.ID]
	wait(w)
	// N+1, because the window starts with 1 column, and we added N.
	if len(w.columns) != N+1 {
		t.Errorf("len(w.columns)=%d, want %d", len(w.columns), N+1)
	}
}

func TestNewColumn_NotFound(t *testing.T) {
	s := newServer(new(stubScreen))
	defer s.close()
	notFoundURL := urlWithPath(s.url, "/", "window", "notfound", "columns")
	if err := NewColumn(notFoundURL, 0.5); err != ErrNotFound {
		t.Errorf("NewColumn(%q, 0.5)=%v, want %v", notFoundURL, err, ErrNotFound)
	}
}

// TODO(eaburns): test that we are actually getting BadRequest errors.
func TestNewColumn_BadRequest(t *testing.T) {
	s := newServer(new(stubScreen))
	defer s.close()

	winsURL := urlWithPath(s.url, "/", "windows")
	win, err := NewWindow(winsURL, image.Pt(800, 600))
	if err != nil {
		t.Fatalf("NewWindow(%q)=%v,%v, want _,nil", winsURL, win, err)
	}
	colsURL := urlWithPath(s.url, win.Path, "columns")
	req := strings.NewReader("bad request")
	if err := request(colsURL, http.MethodPut, req, nil); err == nil {
		t.Errorf("request(%q, PUT, \"bad request\", nil)=%v, want bad request", colsURL, err)
	}
}

func TestNewColumn_WindowEdges(t *testing.T) {
	s := newServer(new(stubScreen))
	defer s.close()

	winsURL := urlWithPath(s.url, "/", "windows")
	win, err := NewWindow(winsURL, image.Pt(800, 600))
	if err != nil {
		t.Fatalf("NewWindow(%q)=%v,%v, want _,nil", winsURL, win, err)
	}
	colsURL := urlWithPath(s.url, win.Path, "columns")

	if err := NewColumn(colsURL, -1.0); err != nil {
		t.Errorf("NewColumn(%q, -1.0)=%v, want nil", colsURL, err)
	}
	if err := NewColumn(colsURL, 0.0); err != nil {
		t.Errorf("NewColumn(%q, 0.0)=%v, want nil", colsURL, err)
	}
	if err := NewColumn(colsURL, 0.01); err != nil {
		t.Errorf("NewColumn(%q, 0.01)=%v, want nil", colsURL, err)
	}
	if err := NewColumn(colsURL, 2.0); err != nil {
		t.Errorf("NewColumn(%q, 2.0)=%v, want nil", colsURL, err)
	}
	if err := NewColumn(colsURL, 1.0); err != nil {
		t.Errorf("NewColumn(%q, 1.0)=%v, want nil", colsURL, err)
	}
	if err := NewColumn(colsURL, 0.99); err != nil {
		t.Errorf("NewColumn(%q, 0.99)=%v, want nil", colsURL, err)
	}

	w := s.uiServer.windows[win.ID]
	wait(w)

	min := 0.0
	max := float64(w.Dx()-minFrameSize) / float64(w.Dx())

	// 1 original, plus 6 added.
	const N = 7
	if len(w.columns) != N {
		t.Errorf("len(w.columns)=%d, want %d", len(w.columns), N)
	}
	for i, x := range w.xs {
		if x < min {
			t.Errorf("w.xs[%d]=%f, want ≥ %f", i, x, min)
		}
		if x > max {
			t.Errorf("w.xs[%d]=%f, want ≤ %f", i, x, max)
		}
	}
}

func TestNewColumn_DoesNotFit(t *testing.T) {
	s := newServer(new(stubScreen))
	defer s.close()

	winsURL := urlWithPath(s.url, "/", "windows")
	// MinFrameSize only fits one column.
	win, err := NewWindow(winsURL, image.Pt(minFrameSize, 600))
	if err != nil {
		t.Fatalf("NewWindow(%q)=%v,%v, want _,nil", winsURL, win, err)
	}

	colsURL := urlWithPath(s.url, win.Path, "columns")
	if err := NewColumn(colsURL, 0.99); err != nil {
		t.Errorf("NewColumn(%q, 0.99)=%v, want nil", colsURL, err)
	}

	w := s.uiServer.windows[win.ID]
	wait(w)

	if len(w.columns) != 1 {
		t.Errorf("len(w.columns)=%d, want 1", len(w.columns))
	}
}

func TestNewSheet(t *testing.T) {
	s := newServer(new(stubScreen))
	defer s.close()

	winsURL := urlWithPath(s.url, "/", "windows")
	win, err := NewWindow(winsURL, image.Pt(800, 600))
	if err != nil {
		t.Fatalf("NewWindow(%q)=%v,%v, want _,nil", winsURL, win, err)
	}
	sheetsURL := urlWithPath(s.url, win.Path, "sheets")
	var sheets []Sheet
	for i := 0; i < 3; i++ {
		editorURL := s.editorServer.PathURL("/")
		sheet, err := NewSheet(sheetsURL, editorURL)
		if err != nil {
			t.Errorf("NewSheet(%q, %q)=%v,%v, want _, nil",
				sheetsURL, editorURL, sheet, err)
		}
		for j, h := range sheets {
			if h.ID == sheet.ID {
				t.Errorf("%d sheet.ID= %s = sheets[%d].ID", i, h.ID, j)
			}
		}
		sheets = append(sheets, sheet)
	}
}

func TestNewSheet_NotFound(t *testing.T) {
	s := newServer(new(stubScreen))
	defer s.close()

	winsURL := urlWithPath(s.url, "/", "windows")
	win, err := NewWindow(winsURL, image.Pt(800, 600))
	if err != nil {
		t.Fatalf("NewWindow(%q)=%v,%v, want _,nil", winsURL, win, err)
	}

	// Request a sheet for a window that is not found.
	notFoundURL := urlWithPath(s.url, "/", "window", "notfound", "sheets")
	editorURL := s.editorServer.PathURL("/")
	if h, err := NewSheet(notFoundURL, editorURL); err != ErrNotFound {
		t.Errorf("NewSheet(%q, %q)=%v,%v, want %v",
			notFoundURL, editorURL, h, err, ErrNotFound)
	}

	// Request a sheet for a buffer that is not found.
	sheetsURL := urlWithPath(s.url, win.Path, "sheets")
	// TODO(eaburns): This should be an ErrNotFound, but currently it's an internal server error.
	notFoundURL = s.editorServer.PathURL("/", "buffer", "notfound")
	if h, err := NewSheet(sheetsURL, notFoundURL); err == nil {
		t.Errorf("NewSheet(%q, %q)=%v,%v, want non-nil",
			sheetsURL, notFoundURL, h, err)
	}

	// Certainly no editor is serving on this port.
	editorURL.Host = "localhost:1"
	if h, err := NewSheet(sheetsURL, editorURL); err == nil {
		t.Errorf("NewSheet(%q, %q)=%v,%v, want connection refused",
			sheetsURL, editorURL, h, err)
	}
}

// TODO(eaburns): test that we are actually getting BadRequest errors.
func TestNewSheet_BadRequest(t *testing.T) {
	s := newServer(new(stubScreen))
	defer s.close()

	winsURL := urlWithPath(s.url, "/", "windows")
	win, err := NewWindow(winsURL, image.Pt(800, 600))
	if err != nil {
		t.Fatalf("NewWindow(%q)=%v,%v, want _,nil", winsURL, win, err)
	}

	sheetsURL := urlWithPath(s.url, win.Path, "sheets")
	badBufferURL := s.editorServer.PathURL("/", "not", "buffer", "path")
	if h, err := NewSheet(sheetsURL, badBufferURL); err == nil {
		t.Errorf("NewSheet(%q, %q)=%v,%v, want bad request",
			sheetsURL, badBufferURL, h, err)
	}

	bad := "bad json"
	req := strings.NewReader(bad)
	if err := request(sheetsURL, http.MethodPut, req, &win); err == nil {
		t.Errorf("request(%q, PUT, %q, &win)=%v, want bad request", sheetsURL, bad, err)
	}

	bad = `{"url":"%ZZ"}` // ZZ is not hex, but % esc wants hex — URL parse fails.
	req = strings.NewReader(bad)
	if err := request(sheetsURL, http.MethodPut, req, &win); err == nil {
		t.Errorf("request(%q, PUT, %q, &win)=%v, want bad request", sheetsURL, bad, err)
	}
}

func TestNewSheet_DoesNotFit(t *testing.T) {
	s := newServer(new(stubScreen))
	defer s.close()

	winsURL := urlWithPath(s.url, "/", "windows")
	// MinFrameSize can only fit 1 sheet.
	win, err := NewWindow(winsURL, image.Pt(800, minFrameSize))
	if err != nil {
		t.Fatalf("NewWindow(%q)=%v,%v, want _,nil", winsURL, win, err)
	}

	sheetsURL := urlWithPath(s.url, win.Path, "sheets")
	editorURL := s.editorServer.PathURL("/")
	if sheet, err := NewSheet(sheetsURL, editorURL); err != nil {
		t.Errorf("NewSheet(%q, %q)=%v,%v, want _, nil", sheetsURL, editorURL, sheet, err)
	}
	if sheet, err := NewSheet(sheetsURL, editorURL); err != nil {
		t.Errorf("NewSheet(%q, %q)=%v,%v, want _, nil", sheetsURL, editorURL, sheet, err)
	}

	w := s.uiServer.windows[win.ID]
	wait(w)

	// We expect 2 frames: the column tag and our first sheet;
	// the second sheet shouldn't fit.
	if n := len(w.columns[0].frames); n != 2 {
		t.Errorf("len(w.columns[0].frames)=%d, want 2", n)
	}
}

func TestCloseSheet(t *testing.T) {
	s := newServer(new(stubScreen))
	defer s.close()

	winsURL := urlWithPath(s.url, "/", "windows")
	win, err := NewWindow(winsURL, image.Pt(800, 600))
	if err != nil {
		t.Fatalf("NewWindow(%q)=%v,%v, want _,nil", winsURL, win, err)
	}
	editorURL := s.editorServer.PathURL("/")
	sheetsURL := urlWithPath(s.url, win.Path, "sheets")
	var sheets []Sheet
	for i := 0; i < 3; i++ {
		sheet, err := NewSheet(sheetsURL, editorURL)
		if err != nil {
			t.Errorf("NewSheet(%q, %q)=%v,%v, want _, nil", sheetsURL, editorURL, sheet, err)
		}
		sheets = append(sheets, sheet)
	}

	for _, h := range sheets {
		sheetURL := urlWithPath(s.url, h.Path)
		if err := Close(sheetURL); err != nil {
			t.Errorf("Close(%q)=%v, want nil", sheetURL, err)
		}
	}

	sheetListURL := urlWithPath(s.url, "sheets")
	if got, err := SheetList(sheetListURL); err != nil || len(got) != 0 {
		t.Errorf("SheetList(%q)=%v,%v, want [],nil", sheetListURL, got, err)
	}
}

func TestCloseSheet_NotFound(t *testing.T) {
	s := newServer(new(stubScreen))
	defer s.close()
	notFoundURL := urlWithPath(s.url, "/", "sheet", "notfound")
	if err := Close(notFoundURL); err != ErrNotFound {
		t.Errorf("Close(%q)=%v, want %v", notFoundURL, err, ErrNotFound)
	}
}

func TestSheetList(t *testing.T) {
	s := newServer(new(stubScreen))
	defer s.close()

	// Empty.
	sheetListURL := urlWithPath(s.url, "sheets")
	if got, err := SheetList(sheetListURL); err != nil || len(got) != 0 {
		t.Errorf("SheetList(%q)=%v,%v, want [],nil", sheetListURL, got, err)
	}

	winsURL := urlWithPath(s.url, "/", "windows")
	win, err := NewWindow(winsURL, image.Pt(800, 600))
	if err != nil {
		t.Fatalf("NewWindow(%q)=%v,%v, want _,nil", winsURL, win, err)
	}

	editorURL := s.editorServer.PathURL("/")
	sheetsURL := urlWithPath(s.url, win.Path, "sheets")
	var want []Sheet
	for i := 0; i < 3; i++ {
		sheet, err := NewSheet(sheetsURL, editorURL)
		if err != nil {
			t.Errorf("NewSheet(%q, %q)=%v,%v, want _, nil", sheetsURL, editorURL, sheet, err)
		}
		want = append(want, sheet)
	}

	sheets, err := SheetList(sheetListURL)
	sort.Sort(sheetSlice(sheets))
	sort.Sort(sheetSlice(want))
	if err != nil || !reflect.DeepEqual(sheets, want) {
		t.Errorf("SheetList(%q)=%v,%v, want %v,nil", sheetListURL, sheets, err, want)
	}
}

type testServer struct {
	scr          screen.Screen
	editorServer *editortest.Server
	uiServer     *Server
	httpServer   *httptest.Server
	url          *url.URL
	done         bool
}

func newServer(scr screen.Screen) *testServer {
	editorServer := editortest.NewServer(editor.NewServer())
	router := mux.NewRouter()
	uiServer := NewServer(scr, editorServer.PathURL("/"))
	uiServer.RegisterHandlers(router)
	httpServer := httptest.NewServer(router)
	url, err := url.Parse(httpServer.URL)
	if err != nil {
		panic(err)
	}
	ts := &testServer{
		editorServer: editorServer,
		uiServer:     uiServer,
		httpServer:   httpServer,
		url:          url,
	}
	uiServer.SetDoneHandler(func() { ts.done = true })
	return ts
}

func (s *testServer) close() {
	s.httpServer.Close()
	s.uiServer.Close()
	s.editorServer.Close()
}

type windowSlice []Window

func (s windowSlice) Len() int           { return len(s) }
func (s windowSlice) Less(i, j int) bool { return s[i].ID < s[j].ID }
func (s windowSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

type sheetSlice []Sheet

func (s sheetSlice) Len() int           { return len(s) }
func (s sheetSlice) Less(i, j int) bool { return s[i].ID < s[j].ID }
func (s sheetSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func urlWithPath(u *url.URL, elems ...string) *url.URL {
	v := *u
	v.Path = path.Join(elems...)
	return &v
}
