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
	req, err := json.Marshal(NewWindowRequest{
		Width:  size.X,
		Height: size.Y,
	})
	if err != nil {
		return Window{}, err
	}
	var win Window
	if err := request(URL, http.MethodPut, bytes.NewReader(req), &win); err != nil {
		return Window{}, err
	}
	return win, nil
}

// NewColumn PUTs a NewColumnRequest.
// If the response status code is NotFound, ErrNotFound is returned.
// The URL is expected to point to a window's columns list.
func NewColumn(URL *url.URL, x float64) error {
	req, err := json.Marshal(NewColumnRequest{X: x})
	if err != nil {
		return err
	}
	return request(URL, http.MethodPut, bytes.NewReader(req), nil)
}

// NewSheet does a PUT and areturns a Sheet from the response body.
// If the response status code is NotFound, ErrNotFound is returned.
// The URL is expected to point to a window's sheets list.
func NewSheet(URL *url.URL) (Sheet, error) {
	var sheet Sheet
	if err := request(URL, http.MethodPut, nil, &sheet); err != nil {
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

func responseError(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusNotFound:
		return ErrNotFound
	default:
		data, _ := ioutil.ReadAll(resp.Body)
		return errors.New(resp.Status + ": " + string(data))
	}
}
