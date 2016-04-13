// Copyright © 2016, The T Authors.

// Package ui contains the T user interface.
package ui

import (
	"encoding/json"
	"image"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"sync"

	"github.com/gorilla/mux"
	"golang.org/x/exp/shiny/screen"
)

// Server is a T user interface server
type Server struct {
	screen    screen.Screen
	editorURL *url.URL
	windows   map[string]*window
	sheets    map[string]*sheet
	nextID    int
	done      func()
	sync.RWMutex
}

// NewServer returns a new Server for the given Screen.
// The editorURL must be the root URL of an editor server.
// Column and sheet tags use buffers created on this editor server.
func NewServer(scr screen.Screen, editorURL *url.URL) *Server {
	editorURL.Path = "/"
	return &Server{
		screen:    scr,
		editorURL: editorURL,
		windows:   make(map[string]*window),
		sheets:    make(map[string]*sheet),
		done:      func() {},
	}
}

// SetDoneHandler sets the function which is called if the last window is closed.
// By default, the done handler is a no-op.
func (s *Server) SetDoneHandler(f func()) {
	s.Lock()
	s.done = f
	s.Unlock()
}

// Close closes all windows.
// The server should not be used after calling Close.
func (s *Server) Close() error {
	s.Lock()
	defer s.Unlock()
	for _, w := range s.windows {
		w.close()
	}
	s.windows = nil
	s.sheets = nil
	return nil
}

// RegisterHandlers registers handlers for the following paths and methods:
//
//  /windows is the list of opened windows.
//
// 	GET returns a Window list of the opened windows.
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error.
//
// 	PUT creates a new window with a single column and returns its Window.
// 	The body must be a NewWindowRequest.
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error.
// 	• Bad Request if the WindowRequest is malformed.
//
//  /window/<ID> is the window with the given ID.
//
// 	DELETE deletes the window and all of its sheets.
// 	The server process exits when the last window is deleted.
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error.
// 	• Not Found if the buffer is not found.
//
//  /window/<ID>/columns is the list of the window's columns.
//
// 	PUT adds a column to the window.
// 	The body must be a NewColumnRequest.
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error
// 	  or if a new column cannot fit on the window.
// 	• Not Found if the window is not found.
// 	• Bad Request if the WindowRequest is malformed.
//
//  /window/<ID>/sheets is the list of the window's sheets.
//
// 	PUT adds a sheet to the left-most column of the window
// 	and returns its Sheet.
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error
// 	  or if a new sheet cannot fit in the column.
// 	• Not Found if the window is not found.
//
//  /sheets is the list of opened sheets.
//
// 	GET returns a Sheet list of the opened sheets.
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error.
//
//  /sheet/<ID> is the sheet with the given ID.
//
// 	DELETE deletes the sheet.
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error.
// 	• Not Found if the sheet is not found.
//
// Unless otherwise stated, the body of all error responses is the error message.
func (s *Server) RegisterHandlers(r *mux.Router) {
	r.HandleFunc("/windows", s.listWindows).Methods(http.MethodGet)
	r.HandleFunc("/windows", s.newWindow).Methods(http.MethodPut)
	r.HandleFunc("/window/{id}", s.deleteWindow).Methods(http.MethodDelete)
	r.HandleFunc("/window/{id}/columns", s.newColumn).Methods(http.MethodPut)
	r.HandleFunc("/window/{id}/sheets", s.newSheet).Methods(http.MethodPut)
	r.HandleFunc("/sheets", s.listSheets).Methods(http.MethodGet)
	r.HandleFunc("/sheet/{id}", s.deleteSheet).Methods(http.MethodDelete)
}

// respond JSON encodes resp to w, and sends an Internal Server Error on failure.
func respond(w http.ResponseWriter, resp interface{}) {
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// MakeWindow returns a Window for the corresponding window.
// It must be called with the server lock held.
func makeWindow(w *window) Window {
	return Window{
		ID:   w.id,
		Path: path.Join("/", "window", w.id),
	}
}

func (s *Server) listWindows(w http.ResponseWriter, req *http.Request) {
	s.RLock()
	var wins []Window
	for _, w := range s.windows {
		wins = append(wins, makeWindow(w))
	}
	s.RUnlock()
	respond(w, wins)
}

func (s *Server) newWindow(w http.ResponseWriter, req *http.Request) {
	var wreq NewWindowRequest
	if err := json.NewDecoder(req.Body).Decode(&wreq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.Lock()
	id := strconv.Itoa(s.nextID)
	s.nextID++
	s.Unlock()
	win, err := newWindow(id, s, image.Pt(wreq.Width, wreq.Height))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.Lock()
	s.windows[id] = win
	resp := makeWindow(win)
	s.Unlock()
	respond(w, resp)
}

func (s *Server) deleteWindow(w http.ResponseWriter, req *http.Request) {
	if !s.delWin(mux.Vars(req)["id"]) {
		http.NotFound(w, req)
	}
}

func (s *Server) delWin(winID string) bool {
	s.Lock()
	defer s.Unlock()
	w, ok := s.windows[winID]
	if !ok {
		return false
	}
	delete(s.windows, w.id)
	w.close()
	if len(s.windows) == 0 {
		s.done()
	}
	return true
}

func (s *Server) newColumn(w http.ResponseWriter, req *http.Request) {
	var creq NewColumnRequest
	if err := json.NewDecoder(req.Body).Decode(&creq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.Lock()
	win, ok := s.windows[mux.Vars(req)["id"]]
	if !ok {
		s.Unlock()
		http.NotFound(w, req)
		return
	}
	errChan := make(chan error)
	win.Send(func() {
		c, err := newColumn(win)
		if err == nil {
			win.addColumn(creq.X, c)
		}
		errChan <- err
	})
	s.Unlock()
	if err := <-errChan; err != nil {
		// TODO(eaburns): this may be an http error, propogate it.
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) newSheet(w http.ResponseWriter, req *http.Request) {
	var sreq NewSheetRequest
	if err := json.NewDecoder(req.Body).Decode(&sreq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	URL, err := url.Parse(sreq.URL)
	if err != nil {
		http.Error(w, "bad URL: "+sreq.URL, http.StatusBadRequest)
		return
	}

	s.Lock()
	win, ok := s.windows[mux.Vars(req)["id"]]
	if !ok {
		s.Unlock()
		http.NotFound(w, req)
		return
	}

	f, err := newSheet(strconv.Itoa(s.nextID), URL, win)
	if err != nil {
		s.Unlock()
		// TODO(eaburns): This may be an http response error.
		// Return that status and message, not StatusInternalServerError.
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.nextID++
	s.sheets[f.id] = f
	resp := makeSheet(f)
	win.Send(func() { win.addFrame(f) })
	s.Unlock()
	respond(w, resp)
}

// MakeSheet returns a Sheet for the corresponding sheet.
// It must be called with the server lock held.
func makeSheet(h *sheet) Sheet {
	return Sheet{
		ID:         h.id,
		Path:       path.Join("/", "sheet", h.id),
		WindowPath: path.Join("/", "window", h.win.id),
	}
}

func (s *Server) listSheets(w http.ResponseWriter, req *http.Request) {
	s.RLock()
	var sheets []Sheet
	for _, h := range s.sheets {
		sheets = append(sheets, makeSheet(h))
	}
	s.RUnlock()
	respond(w, sheets)
}

func (s *Server) deleteSheet(w http.ResponseWriter, req *http.Request) {
	if !s.delSheet(mux.Vars(req)["id"]) {
		http.NotFound(w, req)
	}
}

func (s *Server) delSheet(sheetID string) bool {
	s.Lock()
	defer s.Unlock()
	f, ok := s.sheets[sheetID]
	if !ok {
		return false
	}
	delete(s.sheets, sheetID)
	f.win.Send(func() { f.win.deleteFrame(f) })
	return true
}
