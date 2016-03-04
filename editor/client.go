// Copyright © 2016, The T Authors.

package editor

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"path"
	"sort"
	"strconv"

	"github.com/eaburns/T/edit"
)

// A NotFoundError indicates
// that a Buffer or Editor with the given ID
// is not found on the server.
type NotFoundError string

func (err NotFoundError) Error() string { return "not found: " + string(err) }

// A Client represents a client of an editor Server.
type Client struct {
	host string
	http http.Client
}

// A Buffer is a client-handle to a buffer.
type Buffer struct {
	id     int
	client *Client
}

// An Editor is a client-handle to an editor.
type Editor struct {
	id     int
	buffer *Buffer
}

// NewClient returns a new client for a Server at the given host.
func NewClient(host string) *Client { return &Client{host: host} }

func url(c *Client, elems ...interface{}) string {
	var strs []string
	for _, p := range elems {
		switch p := p.(type) {
		case string:
			strs = append(strs, p)
		case int:
			strs = append(strs, strconv.Itoa(p))
		default:
			panic("bad type")
		}
	}
	return "http://" + c.host + path.Join(strs...)
}

type bufferInfoSlice []BufferInfo

func (s bufferInfoSlice) Len() int           { return len(s) }
func (s bufferInfoSlice) Less(i, j int) bool { return s[i].ID < s[j].ID }
func (s bufferInfoSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// Buffers returns information about all buffers on the server.
// The returned list is sorted in ascending order of buffer ID.
func (c *Client) Buffers() ([]BufferInfo, error) {
	url := url(c, "/", "buffer")
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, respError(resp)
	}
	var bufs []BufferInfo
	if err := json.NewDecoder(resp.Body).Decode(&bufs); err != nil {
		return nil, err
	}
	sort.Sort(bufferInfoSlice(bufs))
	return bufs, nil
}

// NewBuffer returns a handle to a new buffer.
func (c *Client) NewBuffer() (*Buffer, error) {
	url := url(c, "/", "buffer")
	req, err := http.NewRequest(http.MethodPut, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, respError(resp)
	}
	var buf BufferInfo
	if err := json.NewDecoder(resp.Body).Decode(&buf); err != nil {
		return nil, err
	}
	return c.Buffer(buf.ID), nil
}

// Buffer returns a handle to the buffer with the given ID.
func (c *Client) Buffer(bufferID int) *Buffer { return &Buffer{id: bufferID, client: c} }

// Info returns information about the buffer.
func (buf *Buffer) Info() (BufferInfo, error) {
	url := url(buf.client, "/", "buffer", buf.id)
	resp, err := buf.client.http.Get(url)
	if err != nil {
		return BufferInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return BufferInfo{}, respError(resp)
	}
	var info BufferInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return BufferInfo{}, err
	}
	return info, nil
}

// Close closes the buffer.
func (buf *Buffer) Close() error {
	url := url(buf.client, "/", "buffer", buf.id)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	resp, err := buf.client.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return respError(resp)
	}
	return nil
}

type editorInfoSlice []EditorInfo

func (s editorInfoSlice) Len() int           { return len(s) }
func (s editorInfoSlice) Less(i, j int) bool { return s[i].ID < s[j].ID }
func (s editorInfoSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// Editors returns information about all of the editors on the buffer.
// The returned list is sorted in ascending order of editor ID.
func (buf *Buffer) Editors() ([]EditorInfo, error) {
	url := url(buf.client, "/", "buffer", buf.id, "editor")
	resp, err := buf.client.http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, respError(resp)
	}
	var eds []EditorInfo
	if err := json.NewDecoder(resp.Body).Decode(&eds); err != nil {
		return nil, err
	}
	sort.Sort(editorInfoSlice(eds))
	return eds, nil
}

// NewEditor returns a handle to a new editor.
func (buf *Buffer) NewEditor() (*Editor, error) {
	url := url(buf.client, "/", "buffer", buf.id, "editor")
	req, err := http.NewRequest(http.MethodPut, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := buf.client.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, respError(resp)
	}
	var ed EditorInfo
	if err := json.NewDecoder(resp.Body).Decode(&ed); err != nil {
		return nil, err
	}
	return buf.Editor(ed.ID), nil
}

// Editor returns a handle to the editor with the given ID.
func (buf *Buffer) Editor(editorID int) *Editor { return &Editor{id: editorID, buffer: buf} }

// Info returns information about the editor.
func (ed *Editor) Info() (EditorInfo, error) {
	url := url(ed.buffer.client, "/", "buffer", ed.buffer.id, "editor", ed.id)
	resp, err := ed.buffer.client.http.Get(url)
	if err != nil {
		return EditorInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return EditorInfo{}, respError(resp)
	}
	var info EditorInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return EditorInfo{}, err
	}
	return info, nil
}

// Close closes the editor.
func (ed *Editor) Close() error {
	url := url(ed.buffer.client, "/", "buffer", ed.buffer.id, "editor", ed.id)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	resp, err := ed.buffer.client.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return respError(resp)
	}
	return nil
}

// Edit performs an atomic sequence of edits to the editor's buffer.
func (ed *Editor) Edit(edits ...edit.Edit) ([]EditResponse, error) {
	buf := bytes.NewBuffer(nil)
	var eds []EditRequest
	for _, e := range edits {
		eds = append(eds, EditRequest{e})
	}
	if err := json.NewEncoder(buf).Encode(eds); err != nil {
		return nil, err
	}
	const mime = "application/json"
	url := url(ed.buffer.client, "/", "buffer", ed.buffer.id, "editor", ed.id)
	resp, err := ed.buffer.client.http.Post(url, mime, buf)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, respError(resp)
	}
	var edResp []EditResponse
	if err := json.NewDecoder(resp.Body).Decode(&edResp); err != nil {
		return nil, err
	}
	return edResp, nil
}

func respError(resp *http.Response) error {
	var msg string
	if bs, err := ioutil.ReadAll(resp.Body); err != nil {
		msg = "failed to read response body: " + err.Error()
	} else {
		msg = string(bytes.TrimSpace(bs))
	}
	switch resp.StatusCode {
	case http.StatusNotFound:
		return NotFoundError(msg)
	default:
		return errors.New(msg)
	}
}
