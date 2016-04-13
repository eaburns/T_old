// Copyright © 2016, The T Authors.

// Package editortest provides an editor server for use in tests.
package editortest

import (
	"net/http/httptest"
	"net/url"
	"path"

	"github.com/gorilla/mux"
)

// EditorServer is an interface implemented by editor.Server.
type EditorServer interface {
	RegisterHandlers(*mux.Router)
	Close() error
}

// Server is an HTTP editor server.
type Server struct {
	// URL is the URL of the server.
	URL *url.URL

	editorServer EditorServer
	httpServer   *httptest.Server
}

// NewServer returns a new, running Server.
func NewServer(editorServer EditorServer) *Server {
	router := mux.NewRouter()
	editorServer.RegisterHandlers(router)
	httpServer := httptest.NewServer(router)
	url, err := url.Parse(httpServer.URL)
	if err != nil {
		panic(err)
	}
	return &Server{
		URL:          url,
		editorServer: editorServer,
		httpServer:   httpServer,
	}
}

// PathURL returns the URL for the given path on this server.
func (s *Server) PathURL(elems ...string) *url.URL {
	v := *s.URL
	v.Path = path.Join(elems...)
	return &v
}

// Close closes the Server.
func (s *Server) Close() {
	s.httpServer.Close()
	s.editorServer.Close()
}
