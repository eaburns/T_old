// Copyright © 2016, The T Authors.

package editor

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/eaburns/T/edit"
	"github.com/gorilla/websocket"
)

var (
	// ErrNotFound indicates that a resource is not found.
	ErrNotFound = errors.New("not found")

	// ErrRange indicates an out-of-range Address.
	ErrRange = errors.New("bad range")
)

func request(url *url.URL, method string, body io.Reader, resp interface{}) error {
	httpReq, err := http.NewRequest(method, url.String(), body)
	if err != nil {
		return err
	}
	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		return responseError(httpResp)
	}
	if resp == nil {
		return nil
	}
	return json.NewDecoder(httpResp.Body).Decode(resp)
}

// Close does a DELETE.
// The URL is expected to point at either a buffer path or an editor path.
func Close(URL *url.URL) error { return request(URL, http.MethodDelete, nil, nil) }

// BufferList does a GET and returns a list of Buffers from the response body.
// The URL is expected to point at an editor server's buffers list.
func BufferList(URL *url.URL) ([]Buffer, error) {
	var list []Buffer
	if err := request(URL, http.MethodGet, nil, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// NewBuffer does a PUT and returns a Buffer from the response body.
// The URL is expected to point at an editor server's buffers list.
func NewBuffer(URL *url.URL) (Buffer, error) {
	var buf Buffer
	if err := request(URL, http.MethodPut, nil, &buf); err != nil {
		return Buffer{}, err
	}
	return buf, nil
}

// BufferInfo does a GET and returns a Buffer from the response body.
// The URL is expected to point at a buffer path.
func BufferInfo(URL *url.URL) (Buffer, error) {
	var buf Buffer
	if err := request(URL, http.MethodGet, nil, &buf); err != nil {
		return Buffer{}, err
	}
	return buf, nil
}

// A ChangeStream reads changes made to a buffer.
type ChangeStream struct {
	conn *websocket.Conn
}

// Close closes the stream.
// The ChangeStream should not be used after being closed.
func (s *ChangeStream) Close() error {
	dl := time.Now().Add(wsTimeout)
	s.conn.WriteControl(websocket.CloseMessage, nil, dl)
	// TODO(eaburns): Read the close response before dropping the connection.
	return s.conn.Close()
}

// Next returns the next ChangeList from the stream.
func (s *ChangeStream) Next() (ChangeList, error) {
	var cl ChangeList
	return cl, s.conn.ReadJSON(&cl)
}

// Changes returns a ChangeStream that reads changes made to a buffer.
// The URL is expected to point at the changes file of a buffer.
// Note that the changes file is a websocket, and must use a ws scheme:
// 	ws://host:port/buffer/<ID>/changes
func Changes(URL *url.URL) (*ChangeStream, error) {
	conn, resp, err := websocket.DefaultDialer.Dial(URL.String(), make(http.Header))
	switch {
	case err == websocket.ErrBadHandshake:
		if resp.StatusCode != http.StatusOK {
			return nil, responseError(resp)
		}
		fallthrough
	case err != nil:
		return nil, err
	}
	return &ChangeStream{conn: conn}, nil
}

// NewEditor does a PUT and returns an Editor from the response body.
// The URL is expected to point at a buffer path.
func NewEditor(URL *url.URL) (Editor, error) {
	var ed Editor
	if err := request(URL, http.MethodPut, nil, &ed); err != nil {
		return Editor{}, err
	}
	return ed, nil
}

// EditorInfo does a GET and returns an Editor from the response body.
// The URL is expected to point at an editor path.
func EditorInfo(URL *url.URL) (Editor, error) {
	var ed Editor
	if err := request(URL, http.MethodGet, nil, &ed); err != nil {
		return Editor{}, err
	}
	return ed, nil
}

// Reader returns an io.ReadCloser that reads the text from a given Address.
// If non-nil, the returned io.ReadCloser must be closed by the caller.
// If the Address is non-nil, it is set as the value of the addr URL parameter.
// The URL is expected to point at an editor's text path.
func Reader(URL *url.URL, addr edit.Address) (io.ReadCloser, error) {
	urlCopy := *URL
	if addr != nil {
		vals := make(url.Values)
		vals["addr"] = []string{addr.String()}
		urlCopy.RawQuery += "&" + vals.Encode()
	}

	httpResp, err := http.Get(urlCopy.String())
	if err != nil {
		return nil, err
	}
	if httpResp.StatusCode != http.StatusOK {
		defer httpResp.Body.Close()
		return nil, responseError(httpResp)
	}
	return httpResp.Body, nil
}

// Do POSTs a sequence of edits and returns a list of the EditResults
// from the response body.
// The URL is expected to point at an editor path.
func Do(URL *url.URL, edits ...edit.Edit) ([]EditResult, error) {
	var eds []editRequest
	for _, ed := range edits {
		eds = append(eds, editRequest{ed})
	}
	body := bytes.NewBuffer(nil)
	if err := json.NewEncoder(body).Encode(eds); err != nil {
		return nil, err
	}
	var results []EditResult
	if err := request(URL, http.MethodPost, body, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func responseError(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusRequestedRangeNotSatisfiable:
		return ErrRange
	default:
		data, _ := ioutil.ReadAll(resp.Body)
		return errors.New(resp.Status + ": " + string(data))
	}
}
