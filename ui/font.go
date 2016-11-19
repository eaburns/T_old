// Copyright © 2016, The T Authors.

package ui

import (
	"log"

	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/goregular"
)

var defaultFont = loadDefaultFont()

func newFace(dpi float64) font.Face {
	if defaultFont == nil {
		return basicfont.Face7x13
	}
	return truetype.NewFace(defaultFont, &truetype.Options{
		Size: 11, // pt
		DPI:  dpi,
	})
}

func loadDefaultFont() *truetype.Font {
	ttf, err := truetype.Parse(goregular.TTF)
	if err != nil {
		log.Printf("Failed to load default font %s", err)
		return nil
	}
	return ttf
}
