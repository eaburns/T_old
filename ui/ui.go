// Copyright © 2016, The T Authors.

// Package ui implements the T text editor UI.
package ui

// A NewWindowRequest requests a new window be created.
type NewWindowRequest struct {
	// Width is the requested width.
	Width int `json:"width"`

	// Height is the requested height.
	Height int `json:"height"`
}

// A NewColumnRequest requests a new column be created.
type NewColumnRequest struct {
	// X is the left-side of the column
	// given as a fraction of the window width.
	// 	X	where
	// 	0 	the left side of the window
	// 	0.5 	the center of the window
	// 	1 	the right side of the window
	// A new column may be restricted to a minimum width.
	X float64 `json:"x"`
}

// A NewSheetRequest requests a new sheet be created.
type NewSheetRequest struct {
	// URL is either the root URL of an editor server,
	// or the URL of an existing editor server buffer.
	//
	// If URL is an existing buffer, that buffer will be used as the sheet body.
	// Otherwise, a new buffer is created on the editor server for the body.
	URL string `json:"url"`
}

// A Window describes an opened window.
type Window struct {
	// ID is the ID of the window.
	ID string `json:"id"`

	// Path is the path of the window's resource.
	Path string `json:"path"`
}

// A Sheet describes an opened sheet.
type Sheet struct {
	// ID is the ID of the sheet.
	ID string `json:"id"`

	// Path is the path to the sheet's resource.
	Path string `json:"path"`

	// WindowPath is the path to the sheet's window's resource.
	WindowPath string `json:"windowPath"`
}
