// Copyright © 2016, The T Authors.

package editor

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"

	"github.com/eaburns/T/edit"
	"github.com/gorilla/mux"
)

// Server implements http.Handler, serving an HTTP text editor.
// It provides an HTTP API for creating buffers of text
// and editors to read and modify those buffers.
//
// Buffers and editors
//
// A buffer is an un-bounded sequence of runes.
// Buffers can be viewed and modified using editors.
// A buffer can have multiple editors,
// but each editor edits only a single buffer.
//
// An editor can view and modify a buffer
// using the T edit language documented here:
// https://godoc.org/github.com/eaburns/T/edit#Ed.
// While multiple editors can edit the same buffer concurrently,
// each editor maintains its own local state.
type Server struct {
	sync.RWMutex
	buffers      map[int]*buffer
	nextBufferID int
}

// NewServer returns a new Server.
func NewServer() *Server { return &Server{buffers: make(map[int]*buffer)} }

// Close closes the server and all of its buffers.
func (s *Server) Close() error {
	var errs []error
	for _, b := range s.buffers {
		errs = append(errs, b.Buffer.Close())
	}
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// RegisterHandlers registeres handlers for the following paths and methods:
//  /buffer is the list of opened buffers
//
// 	GET returns a BufferInfo list of the opened buffers.
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error.
//
// 	PUT creates a new, empty buffer and returns its BufferInfo.
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error.
//
//  /buffer/<N> is the buffer with ID N
//
// 	GET returns the buffer's BufferInfo
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error.
// 	• Not Found if the buffer is not found.
// 	  The body is the path to the buffer.
//
// 	DELETE deletes the buffer and all of its editors.
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error.
// 	• Not Found if the buffer is not found.
// 	  The body is the path to the buffer.
//
//  /buffer/<N>/editor is the list of the buffer's editors.
//
// 	GET returns an EditorInfo list of the buffer's opened editors.
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error.
// 	• Not Found if the buffer is not found.
// 	  The body is the path to the buffer.
//
// 	PUT creates a new editor for the buffer and returns its EditorInfo.
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error.
// 	• Not Found if the buffer is not found.
// 	  The body is the path to the buffer.
//
//  /buffer/<N>/editor/<M> is the editor M of buffer N.
//
// 	GET returns the editor's EditorInfo.
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error.
// 	• Not Found if either the buffer or editor is not found.
// 	  The body is the path to the buffer or editor.
//
// 	DELETE deletes the editor.
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error.
// 	• Not Found if either the buffer or editor is not found.
// 	  The body is the path to the buffer or editor.
//
// 	POST performs an atomic sequence of edits on the buffer.
// 	The body must be an ordered list of EditRequests.
// 	The response is an ordered list of EditResponses.
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error.
// 	• Not Found if either the buffer or editor is not found.
// 	  The body is the path to the buffer or editor.
// 	• Bad Request if the EditRequest list is malformed.
//
// Unless otherwise stated, the body of all error responses is the error message.
func (s *Server) RegisterHandlers(r *mux.Router) {
	r.HandleFunc("/buffer", s.listBuffers).Methods(http.MethodGet)
	r.HandleFunc("/buffer", s.newBuffer).Methods(http.MethodPut)
	r.HandleFunc(`/buffer/{bid}`, s.bufferInfo).Methods(http.MethodGet)
	r.HandleFunc(`/buffer/{bid}`, s.closeBuffer).Methods(http.MethodDelete)
	r.HandleFunc(`/buffer/{bid}/editor`, s.listEditors).Methods(http.MethodGet)
	r.HandleFunc(`/buffer/{bid}/editor`, s.newEditor).Methods(http.MethodPut)
	r.HandleFunc(`/buffer/{bid}/editor/{eid}`, s.editorInfo).Methods(http.MethodGet)
	r.HandleFunc(`/buffer/{bid}/editor/{eid}`, s.closeEditor).Methods(http.MethodDelete)
	r.HandleFunc(`/buffer/{bid}/editor/{eid}`, s.edit).Methods(http.MethodPost)
}

func notFound(w http.ResponseWriter, err error) { http.Error(w, err.Error(), http.StatusNotFound) }

// getBufferRLocked returns the opened, read-locked buffer
// with the ID from the URL's bid variable,
// or an error if the buffer is not found.
// The buffer is opened and will not be closed until the lock is released.
func getBufferRLocked(s *Server, req *http.Request) (*buffer, error) {
	s.RLock()
	defer s.RUnlock()
	buf, err := getBufferUnsafe(s, req)
	if err == nil {
		buf.RLock()
	}
	return buf, err
}

// getBufferLocked returns the opened, write-locked buffer
// with the ID from the URL's bid variable,
// or an error if the buffer is not found.
// The buffer is opened and will not be closed until the lock is released.
func getBufferLocked(s *Server, req *http.Request) (*buffer, error) {
	s.RLock()
	defer s.RUnlock()
	buf, err := getBufferUnsafe(s, req)
	if err == nil {
		buf.Lock()
	}
	return buf, err
}

// GetBufferUnsafe is like GetBuffer,
// but requires the caller to take the server lock.
func getBufferUnsafe(s *Server, req *http.Request) (*buffer, error) {
	bid := mux.Vars(req)["bid"]
	id, err := strconv.Atoi(bid)
	if err != nil {
		return nil, errors.New("/buffer/" + bid)
	}
	buf, ok := s.buffers[id]
	if !ok {
		return nil, errors.New("/buffer/" + bid)
	}
	return buf, nil
}

// getEditorRLocked returns the editor with the eid from the URL
// for the open, read-locked buffer with bid from the URL
// or an error if the editor is not found.
func getEditorRLocked(s *Server, req *http.Request) (*editor, error) {
	s.RLock()
	defer s.RUnlock()
	ed, err := getEditorUnsafe(s, req)
	if err == nil {
		ed.buffer.RLock()
	}
	return ed, err
}

// getEditorLocked returns the editor with the eid from the URL
// for the open, write-locked buffer with bid from the URL
// or an error if the editor is not found.
func getEditorLocked(s *Server, req *http.Request) (*editor, error) {
	s.RLock()
	defer s.RUnlock()
	ed, err := getEditorUnsafe(s, req)
	if err == nil {
		ed.buffer.Lock()
	}
	return ed, err
}

// getEditorUnsafe returns the editor with bid and eid from the URL
// or an error if the editor is not found.
// The caller must hold the buffer's Lock or RLock.
func getEditorUnsafe(s *Server, req *http.Request) (*editor, error) {
	buf, err := getBufferUnsafe(s, req)
	if err != nil {
		return nil, err
	}
	eid := mux.Vars(req)["eid"]
	id, err := strconv.Atoi(eid)
	if err != nil {
		return nil, errors.New("/buffer/" + strconv.Itoa(buf.ID) + "/editor/" + eid)
	}
	ed, ok := buf.editors[id]
	if !ok {
		return nil, errors.New("/buffer/" + strconv.Itoa(buf.ID) + "/editor/" + eid)
	}
	return ed, nil
}

func (s *Server) listBuffers(w http.ResponseWriter, req *http.Request) {
	s.RLock()
	var bufs []BufferInfo
	for _, b := range s.buffers {
		bufs = append(bufs, b.BufferInfo)
	}
	s.RUnlock()

	if err := json.NewEncoder(w).Encode(bufs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) newBuffer(w http.ResponseWriter, req *http.Request) {
	s.Lock()
	buf := &buffer{
		BufferInfo: BufferInfo{ID: s.nextBufferID},
		Buffer:     edit.NewBuffer(),
		editors:    make(map[int]*editor),
	}
	s.buffers[buf.ID] = buf
	s.nextBufferID++
	s.Unlock()

	if err := json.NewEncoder(w).Encode(buf.BufferInfo); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) bufferInfo(w http.ResponseWriter, req *http.Request) {
	buf, err := getBufferRLocked(s, req)
	if err != nil {
		notFound(w, err)
		return
	}
	info := buf.BufferInfo
	buf.RUnlock()

	if err := json.NewEncoder(w).Encode(info); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) closeBuffer(w http.ResponseWriter, req *http.Request) {
	s.Lock()
	buf, err := getBufferUnsafe(s, req)
	if err != nil {
		s.Unlock()
		notFound(w, err)
		return
	}
	delete(s.buffers, buf.ID)
	s.Unlock()

	buf.Lock()
	defer buf.Unlock()
	if err := buf.Close(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) listEditors(w http.ResponseWriter, req *http.Request) {
	buf, err := getBufferRLocked(s, req)
	if err != nil {
		notFound(w, err)
		return
	}
	var eds []EditorInfo
	for _, ed := range buf.editors {
		eds = append(eds, ed.EditorInfo)
	}
	buf.RUnlock()

	if err := json.NewEncoder(w).Encode(eds); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) newEditor(w http.ResponseWriter, req *http.Request) {
	buf, err := getBufferLocked(s, req)
	if err != nil {
		notFound(w, err)
		return
	}
	ed := &editor{
		EditorInfo: EditorInfo{ID: buf.nextEditorID, BufferID: buf.ID},
		buffer:     buf,
		marks:      make(map[rune]edit.Span),
	}
	buf.editors[ed.ID] = ed
	buf.nextEditorID++
	defer buf.Unlock()

	if err := json.NewEncoder(w).Encode(ed.EditorInfo); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) editorInfo(w http.ResponseWriter, req *http.Request) {
	ed, err := getEditorRLocked(s, req)
	if err != nil {
		notFound(w, err)
		return
	}
	info := ed.EditorInfo
	ed.buffer.RUnlock()

	if err := json.NewEncoder(w).Encode(info); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) closeEditor(w http.ResponseWriter, req *http.Request) {
	ed, err := getEditorLocked(s, req)
	if err != nil {
		notFound(w, err)
		return
	}
	delete(ed.buffer.editors, ed.ID)
	ed.buffer.Unlock()
}

func (s *Server) edit(w http.ResponseWriter, req *http.Request) {
	var edits []EditRequest
	if err := json.NewDecoder(req.Body).Decode(&edits); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ed, err := getEditorLocked(s, req)
	if err != nil {
		notFound(w, err)
		return
	}
	var resps []EditResponse
	print := bytes.NewBuffer(nil)
	for _, e := range edits {
		print.Reset()
		err := e.Do(ed, print)
		ed.buffer.Sequence++
		resp := EditResponse{
			Sequence: ed.buffer.Sequence,
			Print:    print.String(),
		}
		if err != nil {
			resp.Error = err.Error()
		}
		resps = append(resps, resp)
	}
	ed.buffer.Unlock()

	if err := json.NewEncoder(w).Encode(resps); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type buffer struct {
	sync.RWMutex
	BufferInfo
	*edit.Buffer
	editors      map[int]*editor
	nextEditorID int
}

type editor struct {
	EditorInfo
	*buffer
	marks   map[rune]edit.Span
	pending []change
}

type change struct {
	span edit.Span
	size int64
}

func (ed *editor) Mark(m rune) edit.Span { return ed.marks[m] }

func (ed *editor) SetMark(m rune, s edit.Span) error {
	if size := ed.Size(); s[0] < 0 || s[1] < 0 || s[0] > size || s[1] > size {
		return edit.ErrInvalidArgument
	}
	ed.marks[m] = s
	return nil
}

func (ed *editor) Change(s edit.Span, r io.Reader) (int64, error) {
	n, err := ed.Buffer.Change(s, r)
	if err == nil {
		ed.pending = append(ed.pending, change{span: s, size: n})
	}
	return n, err
}

func (ed *editor) Apply() error {
	if err := ed.Buffer.Apply(); err != nil {
		return err
	}
	for _, c := range ed.pending {
		for _, e := range ed.buffer.editors {
			for m, s := range e.marks {
				if e == ed && m == '.' && c.span[0] == s[0] {
					// We handle dot of the current editor specially.
					// If the change has the same start, grow dot.
					// Otherwise, update would simply leave it
					// as a point address and move it.
					dot := e.marks[m]
					dot[1] = dot.Update(c.span, c.size)[1]
					e.marks[m] = dot
				} else {
					e.marks[m] = e.marks[m].Update(c.span, c.size)
				}
			}
		}
	}
	ed.pending = ed.pending[:0]
	return nil
}
