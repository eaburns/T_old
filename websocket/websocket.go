// Copyright © 2016, The T Authors.

// Package websocket provides a wrapper for github.com/gorilla/websocket.
// The wrapper has limited features; the point is ease of use for some common cases.
// It does NOT check the request Origin header.
// All of its methods are safe for concurrent use.
// It automatically applies a send timeout.
// It transparently handles the closing handshake.
package websocket

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// SendTimeout is the amount of time to wait on a Send before giving up.
	SendTimeout = 5 * time.Second

	// CloseRecvTimeout is the amount of time waiting during Close
	// for the remote peer to send a Close message
	// before shutting down the connection.
	CloseRecvTimeout = 5 * time.Second

	// HandshakeTimeout is the amount of time to wait
	// for the connection handshake to complete.
	HandshakeTimeout = 5 * time.Second
)

// A HandshakeError is returned if Dial fails the handshake.
type HandshakeError struct {
	// Status is the string representation of the HTTP response status code.
	Status string
	// StatusCode is the numeric HTTP response status code.
	StatusCode int
}

func (err HandshakeError) Error() string { return err.Status }

var upgrader = websocket.Upgrader{
	HandshakeTimeout: HandshakeTimeout,
	CheckOrigin:      func(*http.Request) bool { return true },
}

// A Conn is a websocket connection.
type Conn struct {
	conn           *websocket.Conn
	send           chan sendReq
	recv           chan recvMsg
	sendCloseOnce  sync.Once
	sendCloseError error
}

// Dial dials a websocket and returns a new Conn.
//
// If the handshake fails, a HandshakeError is returned.
func Dial(URL *url.URL) (*Conn, error) {
	hdr := make(http.Header)
	conn, resp, err := websocket.DefaultDialer.Dial(URL.String(), hdr)
	if err == websocket.ErrBadHandshake && resp.StatusCode != http.StatusOK {
		return nil, HandshakeError{Status: resp.Status, StatusCode: resp.StatusCode}
	}
	if err != nil {
		return nil, err
	}
	return newConn(conn), nil
}

// Upgrade upgrades an HTTP handler and returns an new *Conn.
func Upgrade(w http.ResponseWriter, req *http.Request) (*Conn, error) {
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		return nil, err
	}
	return newConn(conn), nil
}

func newConn(conn *websocket.Conn) *Conn {
	c := &Conn{
		conn: conn,
		send: make(chan sendReq, 10),
		recv: make(chan recvMsg, 10),
	}
	go c.goSend()
	go c.goRecv()
	return c
}

// Close closes the websocket connection,
// unblocking any blocked calls to Recv or Send,
// and blocks until the closing handshake completes
// or CloseRecvTimeout timeout expires.
//
// Close should not be called more than once.
func (c *Conn) Close() error {
	close(c.send)

	err := c.sendClose()
	timer := time.NewTimer(CloseRecvTimeout)
	if err != nil {
		timer.Stop()
		c.conn.Close()
	}

	for {
		select {
		case _, ok := <-c.recv:
			if !ok {
				if timer.Stop() {
					err = c.conn.Close()
				}
				return err
			}
		case <-timer.C:
			err = c.conn.Close()
		}
	}
}

// Send sends a JSON-encoded message.
//
// Send must not be called on a closed connection.
func (c *Conn) Send(msg interface{}) error {
	result := make(chan error)
	c.send <- sendReq{msg: msg, result: result}
	return <-result
}

type sendReq struct {
	msg    interface{}
	result chan<- error
}

func (c *Conn) goSend() {
	for req := range c.send {
		dl := time.Now().Add(SendTimeout)
		c.conn.SetWriteDeadline(dl)
		err := c.conn.WriteJSON(req.msg)
		req.result <- err
	}
}

// Recv receives the next JSON-encoded message into msg.
// If msg is nill, the received message is discarded.
//
// This function must be called continually until Close() is called,
// otherwise the connection will not respond to ping/pong messages.
//
// Calling Recv on a closed connection returns io.EOF.
func (c *Conn) Recv(msg interface{}) error {
	r, ok := <-c.recv
	if !ok {
		return io.EOF
	}
	if r.err != nil {
		return r.err
	}
	if msg == nil {
		return nil
	}
	return json.Unmarshal(r.p, msg)
}

type recvMsg struct {
	p   []byte
	err error
}

func (c *Conn) goRecv() {
	defer close(c.recv)

	for {
		messageType, p, err := c.conn.ReadMessage()
		if messageType == websocket.TextMessage {
			c.recv <- recvMsg{p: p, err: err}
		}
		if err != nil {
			// If this errors, a subsequent call to Close will return the error.
			c.sendClose()
			// ReadMessage cannot receive messages after it returns an error.
			// So give up on waiting for a Close from the peer.
			return
		}
	}
}

func (c *Conn) sendClose() error {
	c.sendCloseOnce.Do(func() {
		dl := time.Now().Add(SendTimeout)
		c.sendCloseError = c.conn.WriteControl(websocket.CloseMessage, nil, dl)
		// If we receive a Close from the peer,
		// gorilla will send the Close response for us.
		// We don't bother tracking this, so just ignore this. error.
		if c.sendCloseError == websocket.ErrCloseSent {
			c.sendCloseError = nil
		}
	})
	return c.sendCloseError
}
