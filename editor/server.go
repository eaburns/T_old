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
	sync.Mutex
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

// GetBuffer returns the buffer with the bid from the URL
// or an error if the buffer is not found.
func getBuffer(s *Server, req *http.Request) (*buffer, error) {
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

// GetEditor returns the editor with the bid and eid from the URL
// or an error if the editor is not found.
func getEditor(s *Server, req *http.Request) (*editor, error) {
	buf, err := getBuffer(s, req)
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
	s.Lock()
	defer s.Unlock()

	var bufs []BufferInfo
	for _, b := range s.buffers {
		bufs = append(bufs, b.BufferInfo)
	}
	if err := json.NewEncoder(w).Encode(bufs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) newBuffer(w http.ResponseWriter, req *http.Request) {
	s.Lock()
	defer s.Unlock()

	buf := &buffer{
		BufferInfo: BufferInfo{ID: s.nextBufferID},
		Buffer:     edit.NewBuffer(),
		editors:    make(map[int]*editor),
	}
	s.buffers[buf.ID] = buf
	s.nextBufferID++
	if err := json.NewEncoder(w).Encode(buf.BufferInfo); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) bufferInfo(w http.ResponseWriter, req *http.Request) {
	s.Lock()
	defer s.Unlock()

	buf, err := getBuffer(s, req)
	if err != nil {
		notFound(w, err)
		return
	}
	buf.Lock()
	defer buf.Unlock()

	if err := json.NewEncoder(w).Encode(buf.BufferInfo); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) closeBuffer(w http.ResponseWriter, req *http.Request) {
	s.Lock()
	defer s.Unlock()

	buf, err := getBuffer(s, req)
	if err != nil {
		notFound(w, err)
		return
	}
	buf.Lock()
	defer buf.Unlock()

	delete(s.buffers, buf.ID)
	if err := buf.Close(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) listEditors(w http.ResponseWriter, req *http.Request) {
	s.Lock()
	defer s.Unlock()

	buf, err := getBuffer(s, req)
	if err != nil {
		notFound(w, err)
		return
	}
	buf.Lock()
	defer buf.Unlock()

	var eds []EditorInfo
	for _, ed := range buf.editors {
		eds = append(eds, ed.EditorInfo)
	}
	if err := json.NewEncoder(w).Encode(eds); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) newEditor(w http.ResponseWriter, req *http.Request) {
	s.Lock()
	defer s.Unlock()

	buf, err := getBuffer(s, req)
	if err != nil {
		notFound(w, err)
		return
	}
	buf.Lock()
	defer buf.Unlock()

	ed := &editor{
		EditorInfo: EditorInfo{ID: buf.nextEditorID, BufferID: buf.ID},
		buffer:     buf,
		marks:      make(map[rune]edit.Span),
	}
	buf.editors[ed.ID] = ed
	buf.nextEditorID++
	if err := json.NewEncoder(w).Encode(ed.EditorInfo); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) editorInfo(w http.ResponseWriter, req *http.Request) {
	s.Lock()
	defer s.Unlock()

	ed, err := getEditor(s, req)
	if err != nil {
		notFound(w, err)
		return
	}
	ed.buffer.Lock()
	defer ed.buffer.Unlock()

	if err := json.NewEncoder(w).Encode(ed.EditorInfo); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) closeEditor(w http.ResponseWriter, req *http.Request) {
	s.Lock()
	defer s.Unlock()

	ed, err := getEditor(s, req)
	if err != nil {
		notFound(w, err)
		return
	}
	ed.buffer.Lock()
	defer ed.buffer.Unlock()

	delete(ed.buffer.editors, ed.ID)
}

func (s *Server) edit(w http.ResponseWriter, req *http.Request) {
	s.Lock()
	defer s.Unlock()

	ed, err := getEditor(s, req)
	if err != nil {
		notFound(w, err)
		return
	}
	ed.buffer.Lock()
	defer ed.buffer.Unlock()

	var edits []EditRequest
	if err := json.NewDecoder(req.Body).Decode(&edits); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
	if err := json.NewEncoder(w).Encode(resps); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type buffer struct {
	sync.Mutex
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
