// Copyright © 2016, The T Authors.

package ui

import (
	"go/build"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/image/font"

	// TODO(eaburns): use github.com/golang/freetype/truetype
	// once it properly computes font heights.
	"github.com/eaburns/freetype/truetype"
)

var defaultFont = loadDefaultFont()

func newFace(dpi float64) font.Face {
	return truetype.NewFace(defaultFont, &truetype.Options{
		Size: 11, // pt
		DPI:  dpi,
	})
}

func loadDefaultFont() *truetype.Font {
	const importString = "github.com/eaburns/T"
	pkg, err := build.Import(importString, "", build.FindOnly)
	if err != nil {
		log.Printf("failed to load package info for %s: %s", importString, err)
		return nil
	}
	file := filepath.Join(pkg.Dir, "data", "Roboto-Regular.ttf")
	f, err := os.Open(file)
	if err != nil {
		log.Printf("failed to open %s: %s", file, err)
		return nil
	}
	data, err := ioutil.ReadAll(f)
	if err != nil {
		log.Printf("failed to read %s: %s", file, err)
		return nil
	}
	font, err := truetype.Parse(data)
	if err != nil {
		log.Printf("failed to parse %s: %s", file, err)
		return nil
	}
	return font
}
