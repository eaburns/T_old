// Copyright © 2016, The T Authors.

package websocket

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strconv"
	"testing"
)

func TestDialNotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	s := httptest.NewServer(handler)
	defer s.Close()

	URL, err := url.Parse(s.URL)
	if err != nil {
		t.Fatalf("url.Parse(%q)=_,%v", s.URL, err)
	}
	URL.Scheme = "ws"
	URL.Path = path.Join("/", "notfound")

	conn, err := Dial(URL)
	if hsErr, ok := err.(HandshakeError); !ok || hsErr.StatusCode != http.StatusNotFound {
		t.Errorf("Dial(%s)=_,%v, want HandshakeError{StatusCode: 400}", URL, err)
		conn.Close()
	}
}

func TestEcho(t *testing.T) {
	const N = 10

	handler := http.HandlerFunc(echoUntilClose(t))
	s := httptest.NewServer(handler)
	defer s.Close()

	URL, err := url.Parse(s.URL)
	if err != nil {
		t.Fatalf("url.Parse(%q)=_,%v", s.URL, err)
	}
	URL.Scheme = "ws"
	conn, err := Dial(URL)
	if err != nil {
		t.Fatalf("Dial(%s)=_,%v", URL, err)
	}

	for i := 0; i < N; i++ {
		sent := strconv.Itoa(i)
		if err := conn.Send(sent); err != nil {
			t.Fatalf("client conn.Send(%q)=%v", sent, err)
		}
		var recvd string
		if err := conn.Recv(&recvd); err != nil {
			t.Fatalf("client conn.Recv(&recvd)=%v", err)
		}
		if recvd != sent {
			t.Errorf("recvd=%q, want %q", recvd, sent)
		}
	}
	if err := conn.Close(); err != nil {
		t.Errorf("client conn.Close()=%v", err)
	}
}

func TestRecvOnClosedConn(t *testing.T) {
	handler := http.HandlerFunc(recvUntilClose(t))
	s := httptest.NewServer(handler)
	defer s.Close()

	URL, err := url.Parse(s.URL)
	if err != nil {
		t.Fatalf("url.Parse(%q)=_,%v", s.URL, err)
	}
	URL.Scheme = "ws"
	conn, err := Dial(URL)
	if err != nil {
		t.Fatalf("Dial(%s)=_,%v", URL, err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("client conn.Close()=%v", err)
	}

	for i := 0; i < 3; i++ {
		if err := conn.Recv(nil); err != io.EOF {
			t.Errorf("client %d conv.Recv(nil)=%v, want %v", i, err, io.EOF)
		}
	}
}

func TestRecvNill(t *testing.T) {
	handler := http.HandlerFunc(echoUntilClose(t))
	s := httptest.NewServer(handler)
	defer s.Close()

	URL, err := url.Parse(s.URL)
	if err != nil {
		t.Fatalf("url.Parse(%q)=_,%v", s.URL, err)
	}
	URL.Scheme = "ws"
	conn, err := Dial(URL)
	if err != nil {
		t.Fatalf("Dial(%s)=_,%v", URL, err)
	}
	if err := conn.Send("abc"); err != nil {
		t.Fatalf("conn.Send(\"abc\")=%v", err)
	}
	if err := conn.Recv(nil); err != nil {
		t.Errorf("conn.Recv(nill)=%v, want nil", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("client conn.Close()=%v", err)
	}
}

func echoUntilClose(t *testing.T) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r)
		if err != nil {
			t.Fatalf("Upgrade(w, r)=%v", err)
		}
		for {
			var s string
			switch err := conn.Recv(&s); {
			case err == io.EOF:
				if err := conn.Close(); err != nil {
					t.Errorf("server conn.Close()=%v", err)
				}
				return
			case err != nil:
				t.Fatalf("server conn.Recv(&s)=%v", err)
			default:
				if err := conn.Send(s); err != nil {
					t.Fatalf("server conn.Send(%q)=%v", s, err)
				}
			}
		}
	})
}

func recvUntilClose(t *testing.T) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r)
		if err != nil {
			t.Fatalf("Upgrade(w, r)=%v", err)
		}
		for {
			switch err := conn.Recv(nil); {
			case err == nil:
				continue
			case err != io.EOF:
				t.Errorf("server conn.Recv(nill)=%v", err)
			}
			if err := conn.Close(); err != nil {
				t.Errorf("server conn.Close()=%v", err)
			}
		}
	})
}
