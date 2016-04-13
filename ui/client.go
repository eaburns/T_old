// Copyright © 2016, The T Authors.

package ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
)

// ErrNotFound indicates that a resource is not found.
var ErrNotFound = errors.New("not found")

// Close does a DELETE.
// The URL is expected to point at either a window path or a sheet path.
func Close(URL *url.URL) error { return request(URL, http.MethodDelete, nil, nil) }

// WindowList goes a GET and returns a list of Windows from the response body.
// The URL is expected to point to the server's windows list.
func WindowList(URL *url.URL) ([]Window, error) {
	var list []Window
	if err := request(URL, http.MethodGet, nil, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// NewWindow PUTs a NewWindowRequest
// with Width and Height set to size.X and size.Y respectively,
// and returns a Window from the response body.
// The URL is expected to point at the server's windows list.
func NewWindow(URL *url.URL, size image.Point) (Window, error) {
	req := NewWindowRequest{
		Width:  size.X,
		Height: size.Y,
	}
	var win Window
	if err := request(URL, http.MethodPut, req, &win); err != nil {
		return Window{}, err
	}
	return win, nil
}

// NewColumn PUTs a NewColumnRequest.
// If the response status code is NotFound, ErrNotFound is returned.
// The URL is expected to point to a window's columns list.
func NewColumn(URL *url.URL, x float64) error {
	req := NewColumnRequest{X: x}
	return request(URL, http.MethodPut, req, nil)
}

// NewSheet does a PUT and areturns a Sheet from the response body.
// If the response status code is NotFound, ErrNotFound is returned.
// The URL is expected to point to a window's sheets list.
func NewSheet(uiURL *url.URL, editorOrBufferURL *url.URL) (Sheet, error) {
	req := NewSheetRequest{
		URL: editorOrBufferURL.String(),
	}
	var sheet Sheet
	if err := request(uiURL, http.MethodPut, req, &sheet); err != nil {
		return Sheet{}, err
	}
	return sheet, nil
}

// SheetList goes a GET and returns a list of Sheets from the response body.
// The URL is expected to point to the server's sheets list.
func SheetList(URL *url.URL) ([]Sheet, error) {
	var list []Sheet
	if err := request(URL, http.MethodGet, nil, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// Request makes an HTTP request to the given URL.
// req is the body of the request.
// If it implements io.Reader it is used directly as the body,
// otherwise it is JSON-encoded.
func request(url *url.URL, method string, req interface{}, resp interface{}) error {
	var body io.Reader
	if req != nil {
		if r, ok := req.(io.Reader); ok {
			body = r
		} else {
			d, err := json.Marshal(req)
			if err != nil {
				return err
			}
			body = bytes.NewReader(d)
		}
	}
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

func responseError(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusNotFound:
		return ErrNotFound
	default:
		data, _ := ioutil.ReadAll(resp.Body)
		return errors.New(resp.Status + ": " + string(data))
	}
}
