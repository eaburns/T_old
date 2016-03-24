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

	"github.com/eaburns/T/edit"
)

// ErrNotFound indicates that a resource is not found.
var ErrNotFound = errors.New("not found")

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
func Close(url *url.URL) error { return request(url, http.MethodDelete, nil, nil) }

// BufferList does a GET and returns a list of Buffers from the response body.
// The URL is expected to point at an editor server's buffers list.
func BufferList(url *url.URL) ([]Buffer, error) {
	var list []Buffer
	if err := request(url, http.MethodGet, nil, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// NewBuffer does a PUT and returns a Buffer from the response body.
// The URL is expected to point at an editor server's buffers list.
func NewBuffer(url *url.URL) (Buffer, error) {
	var buf Buffer
	if err := request(url, http.MethodPut, nil, &buf); err != nil {
		return Buffer{}, err
	}
	return buf, nil
}

// BufferInfo does a GET and returns a Buffer from the response body.
// The URL is expected to point at a buffer path.
func BufferInfo(url *url.URL) (Buffer, error) {
	var buf Buffer
	if err := request(url, http.MethodGet, nil, &buf); err != nil {
		return Buffer{}, err
	}
	return buf, nil
}

// NewEditor does a PUT and returns an Editor from the response body.
// The URL is expected to point at a buffer path.
func NewEditor(url *url.URL) (Editor, error) {
	var ed Editor
	if err := request(url, http.MethodPut, nil, &ed); err != nil {
		return Editor{}, err
	}
	return ed, nil
}

// EditorInfo does a GET and returns an Editor from the response body.
// The URL is expected to point at an editor path.
func EditorInfo(url *url.URL) (Editor, error) {
	var ed Editor
	if err := request(url, http.MethodGet, nil, &ed); err != nil {
		return Editor{}, err
	}
	return ed, nil
}

// Do POSTs a sequence of edits and returns a list of the EditResults
// from the response body.
// The URL is expected to point at an editor path.
func Do(url *url.URL, edits ...edit.Edit) ([]EditResult, error) {
	var eds []editRequest
	for _, ed := range edits {
		eds = append(eds, editRequest{ed})
	}
	body := bytes.NewBuffer(nil)
	if err := json.NewEncoder(body).Encode(eds); err != nil {
		return nil, err
	}
	var results []EditResult
	if err := request(url, http.MethodPost, body, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func responseError(resp *http.Response) error {
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	data, _ := ioutil.ReadAll(resp.Body)
	return errors.New(resp.Status + ": " + string(data))
}
