// Copyright © 2016, The T Authors.

package ui

import (
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/exp/shiny/screen"
	"golang.org/x/image/math/f64"
	"golang.org/x/mobile/event/size"
	"golang.org/x/mobile/geom"
)

type stubScreen struct{}

func (*stubScreen) NewBuffer(size image.Point) (screen.Buffer, error) {
	return newTestBuffer(size), nil
}

func (*stubScreen) NewTexture(size image.Point) (screen.Texture, error) {
	return newTestTexture(size), nil
}

func (*stubScreen) NewWindow(opts *screen.NewWindowOptions) (screen.Window, error) {
	w := newTestWindow(opts)
	const pxPerPt = defaultDPI / ptPerInch
	w.Send(size.Event{
		WidthPx:     opts.Width,
		HeightPx:    opts.Height,
		WidthPt:     geom.Pt(opts.Width) * pxPerPt,
		HeightPt:    geom.Pt(opts.Height) * pxPerPt,
		PixelsPerPt: pxPerPt,
	})
	return w, nil
}

type stubBuffer struct{ img *image.RGBA }

func newTestBuffer(size image.Point) *stubBuffer {
	return &stubBuffer{img: image.NewRGBA(image.Rect(0, 0, size.X, size.Y))}
}
func (*stubBuffer) Release()                  {}
func (t *stubBuffer) Size() image.Point       { return t.img.Bounds().Size() }
func (t *stubBuffer) Bounds() image.Rectangle { return t.img.Bounds() }
func (t *stubBuffer) RGBA() *image.RGBA       { return t.img }

type stubTexture struct{ size image.Point }

func newTestTexture(size image.Point) *stubTexture {
	return &stubTexture{size: size}
}
func (*stubTexture) Release()            {}
func (t *stubTexture) Size() image.Point { return t.size }
func (t *stubTexture) Bounds() image.Rectangle {
	return image.Rect(0, 0, t.size.X, t.size.Y)
}
func (*stubTexture) Upload(image.Point, screen.Buffer, image.Rectangle) {}
func (*stubTexture) Fill(image.Rectangle, color.Color, draw.Op)         {}

type stubWindow struct {
	w, h    int
	publish chan bool
	events  chan interface{}
}

func newTestWindow(opts *screen.NewWindowOptions) *stubWindow {
	publish := make(chan bool)
	return &stubWindow{
		w:       opts.Width,
		h:       opts.Height,
		publish: publish,
		events:  make(chan interface{}, 100),
	}
}

func (t *stubWindow) Send(event interface{})      { t.events <- event }
func (t *stubWindow) SendFirst(event interface{}) { panic("unimplemented") }
func (t *stubWindow) NextEvent() interface{}      { return <-t.events }

func (t *stubWindow) Release() {
	for range t.publish {
	}
}

func (*stubWindow) Upload(image.Point, screen.Buffer, image.Rectangle)                           {}
func (*stubWindow) Fill(image.Rectangle, color.Color, draw.Op)                                   {}
func (*stubWindow) Draw(f64.Aff3, screen.Texture, image.Rectangle, draw.Op, *screen.DrawOptions) {}
func (*stubWindow) DrawUniform(src2dst f64.Aff3, src color.Color, sr image.Rectangle, op draw.Op, opts *screen.DrawOptions) {
}
func (*stubWindow) Copy(image.Point, screen.Texture, image.Rectangle, draw.Op, *screen.DrawOptions) {}
func (*stubWindow) Scale(image.Rectangle, screen.Texture, image.Rectangle, draw.Op, *screen.DrawOptions) {
}
func (t *stubWindow) Publish() screen.PublishResult {
	go func() { t.publish <- true }()
	return screen.PublishResult{BackBufferPreserved: false}
}
