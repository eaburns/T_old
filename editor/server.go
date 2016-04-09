// Copyright © 2016, The T Authors.

package editor

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eaburns/T/edit"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
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
	buffers map[string]*buffer
	editors map[string]*editor
	nextID  int
}

// NewServer returns a new Server.
func NewServer() *Server {
	return &Server{
		buffers: make(map[string]*buffer),
		editors: make(map[string]*editor),
	}
}

// Close closes the server and all of its buffers.
func (s *Server) Close() error {
	var errs []error
	for _, b := range s.buffers {
		errs = append(errs, b.close())
	}
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// RegisterHandlers registers handlers for the following paths and methods:
//  /buffers is the list of opened buffers.
//
// 	GET returns a Buffer list of the opened buffers.
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error.
//
// 	PUT creates a new, empty buffer and returns its Buffer.
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error.
//
//  /buffer/<ID> is the buffer with the given ID.
//
// 	GET returns the buffer's Buffer
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error.
// 	• Not Found if the buffer is not found.
//
// 	DELETE deletes the buffer and all of its editors.
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error.
// 	• Not Found if the buffer is not found.
//
// 	PUT creates a new editor for the buffer and returns its Editor.
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error.
// 	• Not Found if the buffer is not found.
//
//  /buffer/<ID>/changes is the buffer's change stream.
//
// 	GET upgrades the connection to a websocket.
// 	A ChangeList is sent on the websocket
// 	for each edit made to the buffer.
// 	Returns:
// 	• Internal Server Error on internal error.
// 	• Not Found if the buffer is not found.
//
//  /editor/<ID> is the editor with the given ID.
//
// 	GET returns the editor's Editor.
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error.
// 	• Not Found if the editor is not found.
//
// 	DELETE deletes the editor.
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error.
// 	• Not Found if the editor editor is not found.
//
//  /editor/<ID>/text is the text that the editor edits.
//
// 	GET returns the text of the editor's buffer.
// 	Parameters:
// 	• addr can optionally be set to an address string.
// 	  It must not appear multiple times, there can only be one addr.
// 	  If it is set, only the text within the address is returned.
//  	  Otherwise, all text is returned.
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error.
// 	• Not Found if the editor is not found.
// 	• Bad Request if the URL parameters or addr value are malformed.
// 	• Range Not Satisfiable if there is an error evaluating the address.
// 	  The response body will contain an error message.
//
// 	POST performs an atomic sequence of edits on the buffer.
// 	The body must be an ordered list of Edits.
// 	The response is an ordered list of EditResult.
// 	Returns:
// 	• OK on success.
// 	• Internal Server Error on internal error.
// 	• Not Found if the editor is not found.
// 	• Bad Request if the Edit list is malformed.
//
// Unless otherwise stated, the body of all error responses is the error message.
func (s *Server) RegisterHandlers(r *mux.Router) {
	r.HandleFunc("/buffers", s.listBuffers).Methods(http.MethodGet)
	r.HandleFunc("/buffers", s.newBuffer).Methods(http.MethodPut)
	r.HandleFunc("/buffer/{id}", s.bufferInfo).Methods(http.MethodGet)
	r.HandleFunc("/buffer/{id}", s.closeBuffer).Methods(http.MethodDelete)
	r.HandleFunc("/buffer/{id}", s.newEditor).Methods(http.MethodPut)
	r.HandleFunc("/buffer/{id}/changes", s.changes).Methods(http.MethodGet)
	r.HandleFunc("/editor/{id}", s.editorInfo).Methods(http.MethodGet)
	r.HandleFunc("/editor/{id}", s.closeEditor).Methods(http.MethodDelete)
	r.HandleFunc("/editor/{id}/text", s.read).Methods(http.MethodGet)
	r.HandleFunc("/editor/{id}/text", s.edit).Methods(http.MethodPost)
}

// respond JSON encodes resp to w, and sends an Internal Server Error on failure.
func respond(w http.ResponseWriter, resp interface{}) {
	body, err := json.Marshal(resp)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, err = w.Write(body)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) listBuffers(w http.ResponseWriter, req *http.Request) {
	s.RLock()
	var bufs []Buffer
	for _, b := range s.buffers {
		bufs = append(bufs, b.Buffer)
	}
	s.RUnlock()

	respond(w, bufs)
}

func (s *Server) newBuffer(w http.ResponseWriter, req *http.Request) {
	s.Lock()
	id := strconv.Itoa(s.nextID)
	s.nextID++
	buf := &buffer{
		Buffer: Buffer{
			ID:   id,
			Path: path.Join("/", "buffer", id),
		},
		buffer:  edit.NewBuffer(),
		editors: make(map[string]*editor),
		done:    make(chan struct{}),
	}
	s.buffers[buf.ID] = buf
	s.Unlock()

	respond(w, buf.Buffer)
}

func (s *Server) bufferInfo(w http.ResponseWriter, req *http.Request) {
	s.RLock()
	buf, ok := s.buffers[mux.Vars(req)["id"]]
	if !ok {
		s.RUnlock()
		http.NotFound(w, req)
		return
	}
	buf.RLock()
	info := buf.Buffer
	buf.RUnlock()
	s.RUnlock()

	respond(w, info)
}

func (s *Server) closeBuffer(w http.ResponseWriter, req *http.Request) {
	s.Lock()
	buf, ok := s.buffers[mux.Vars(req)["id"]]
	if !ok {
		s.Unlock()
		http.NotFound(w, req)
		return
	}
	buf.Lock()
	defer buf.Unlock()
	delete(s.buffers, buf.ID)
	for edID := range buf.editors {
		delete(s.editors, edID)
	}
	s.Unlock()

	if err := buf.close(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

var upgrader = websocket.Upgrader{
	HandshakeTimeout: wsTimeout,
	CheckOrigin:      func(*http.Request) bool { return true },
}

func (s *Server) changes(w http.ResponseWriter, req *http.Request) {
	s.Lock()
	buf, ok := s.buffers[mux.Vars(req)["id"]]
	if !ok {
		s.Unlock()
		http.NotFound(w, req)
		return
	}
	buf.Lock()
	s.Unlock()
	changes := make(chan []ChangeList, 1)
	buf.watchers = append(buf.watchers, changes)
	buf.Unlock()

	defer func() {
		buf.Lock()
		for i := range buf.watchers {
			if buf.watchers[i] == changes {
				buf.watchers = append(buf.watchers[:i], buf.watchers[i+1:]...)
				if buf.watcherRemoved != nil {
					buf.watcherRemoved <- struct{}{}
				}
				break
			}
		}
		buf.Unlock()
	}()

	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() {
		dl := time.Now().Add(wsTimeout)
		err := conn.WriteControl(websocket.CloseMessage, nil, dl)
		if err != nil && err != websocket.ErrCloseSent {
			log.Printf("error sending close to websocket: %v", err)
		}
		conn.Close()
	}()

	done := make(chan struct{})
	go func() {
		for {
			if _, _, err := conn.NextReader(); err != nil {
				close(done)
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		case <-buf.done:
			return
		case cls := <-changes:
			for _, cl := range cls {
				conn.SetWriteDeadline(time.Now().Add(wsTimeout))
				err := conn.WriteJSON(cl)
				if err != nil && err != websocket.ErrCloseSent {
					log.Printf("error sending to websocket: %v", err)
					return
				}
			}
		}
	}
}

func (s *Server) newEditor(w http.ResponseWriter, req *http.Request) {
	s.Lock()
	buf, ok := s.buffers[mux.Vars(req)["id"]]
	if !ok {
		s.Unlock()
		http.NotFound(w, req)
		return
	}
	buf.Lock()

	id := strconv.Itoa(s.nextID)
	s.nextID++
	ed := &editor{
		Editor: Editor{
			ID:         id,
			Path:       path.Join("/", "editor", id),
			BufferPath: buf.Path,
		},
		buffer: buf,
		Buffer: buf.buffer,
		marks:  make(map[rune]edit.Span),
	}
	s.editors[ed.ID] = ed
	buf.editors[ed.ID] = ed
	buf.Editors = append(buf.Editors, ed.Editor)

	buf.Unlock()
	s.Unlock()

	respond(w, ed.Editor)
}

func (s *Server) editorInfo(w http.ResponseWriter, req *http.Request) {
	s.RLock()
	ed, ok := s.editors[mux.Vars(req)["id"]]
	if !ok {
		s.RUnlock()
		http.NotFound(w, req)
		return
	}
	ed.buffer.RLock()
	info := ed.Editor
	ed.buffer.RUnlock()
	s.RUnlock()

	respond(w, info)
}

func (s *Server) closeEditor(w http.ResponseWriter, req *http.Request) {
	s.Lock()
	ed, ok := s.editors[mux.Vars(req)["id"]]
	if !ok {
		s.Unlock()
		http.NotFound(w, req)
		return
	}
	ed.buffer.Lock()

	delete(s.editors, ed.ID)
	delete(ed.buffer.editors, ed.ID)
	eds := ed.buffer.Editors
	for i := range eds {
		if eds[i].ID == ed.ID {
			ed.buffer.Editors = append(eds[:i], eds[i+1:]...)
			break
		}
	}

	ed.buffer.Unlock()
	s.Unlock()
}

func (s *Server) read(w http.ResponseWriter, req *http.Request) {
	s.Lock()
	ed, ok := s.editors[mux.Vars(req)["id"]]
	if !ok {
		s.Unlock()
		http.NotFound(w, req)
		return
	}
	ed.buffer.Lock()
	defer ed.buffer.Unlock()
	s.Unlock()

	addr := edit.All
	vars, err := url.ParseQuery(req.URL.RawQuery)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if a, ok := vars["addr"]; ok {
		if len(a) > 1 {
			http.Error(w, "addr can only be given once", http.StatusBadRequest)
			return
		}
		r := strings.NewReader(a[0])
		addr, err = edit.Addr(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if r.Len() != 0 {
			http.Error(w, "bad address: "+a[0], http.StatusBadRequest)
			return
		}
	}
	span, err := addr.Where(ed.Buffer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusRequestedRangeNotSatisfiable)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	if _, err = io.Copy(w, ed.Buffer.Reader(span)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) edit(w http.ResponseWriter, req *http.Request) {
	var edits []editRequest
	if err := json.NewDecoder(req.Body).Decode(&edits); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.Lock()
	ed, ok := s.editors[mux.Vars(req)["id"]]
	if !ok {
		s.Unlock()
		http.NotFound(w, req)
		return
	}
	ed.buffer.Lock()
	s.Unlock()

	var results []EditResult
	print := bytes.NewBuffer(nil)
	for _, e := range edits {
		print.Reset()
		err := e.Do(ed, print)
		ed.buffer.Sequence++
		result := EditResult{
			Sequence: ed.buffer.Sequence,
			Print:    print.String(),
		}
		if err != nil {
			result.Error = err.Error()
		}
		results = append(results, result)
	}

	ed.buffer.Unlock()

	respond(w, results)
}

type buffer struct {
	sync.RWMutex
	Buffer
	buffer *edit.Buffer

	editors map[string]*editor

	watchers []chan []ChangeList
	done     chan struct{}
	// watcherRemoved is for testing purposes.
	// If non-nil, an empty struct is sent when a watcher is removed.
	watcherRemoved chan struct{}
}

// Must be called with the write Lock held.
func (buf *buffer) close() error {
	close(buf.done)
	return buf.buffer.Close()
}

type editor struct {
	Editor
	*edit.Buffer
	buffer  *buffer
	marks   map[rune]edit.Span
	pending []Change
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

type changeReader struct {
	r      io.Reader
	nbytes int
	text   []byte
}

func (cr *changeReader) Read(d []byte) (int, error) {
	n, err := cr.r.Read(d)
	m := MaxInline - len(cr.text)
	if m > n {
		m = n
	}
	cr.text = append(cr.text, d[:m]...)
	cr.nbytes += n
	return n, err
}

func (ed *editor) Change(s edit.Span, r io.Reader) (int64, error) {
	cr := changeReader{r: r}
	n, err := ed.Buffer.Change(s, &cr)
	if err == nil {
		c := Change{Span: s, NewSize: n}
		if 0 < cr.nbytes && cr.nbytes <= MaxInline {
			c.Text = cr.text
		}
		ed.pending = append(ed.pending, c)
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
				if e == ed && m == '.' && c.Span[0] == s[0] {
					// We handle dot of the current editor specially.
					// If the change has the same start, grow dot.
					// Otherwise, update would simply leave it
					// as a point address and move it.
					dot := e.marks[m]
					dot[1] = dot.Update(c.Span, c.NewSize)[1]
					e.marks[m] = dot
				} else {
					e.marks[m] = e.marks[m].Update(c.Span, c.NewSize)
				}
			}
		}
	}
	cl := ChangeList{
		Sequence: ed.buffer.Sequence + 1,
		Changes:  ed.pending,
	}
	for _, c := range ed.buffer.watchers {
		select {
		case cls := <-c:
			c <- append(cls, cl)
		case c <- []ChangeList{cl}:
		}
	}
	ed.pending = nil
	return nil
}
