// Package sdl2 provides an SDL2-based UI implementation.
package sdl2

/*
#cgo darwin LDFLAGS: -framework SDL2
#cgo linux pkg-config: sdl2
#include <SDL2/SDL.h>

Uint32 eventType(SDL_Event* e) { return e->type; }
*/
import "C"

import (
	"errors"
	"image"
	"image/color"
	"image/draw"
	"runtime"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/eaburns/T/ui"
)

type sdl2UI struct {
	done chan chan<- struct{}
	do   chan func()

	// Wins is the set of currently opened windows.
	// It is owned by the sdl2UI.run go routine.
	wins map[C.Uint32]*window
}

// New returns a new SDL2 implementation of ui.UI.
// The hz parameter specifies the frequency at which events are polled.
func New(hz time.Duration) (ui.UI, error) {
	u := &sdl2UI{
		wins: make(map[C.Uint32]*window),
		done: make(chan chan<- struct{}),
		do:   make(chan func(), 10),
	}
	err := make(chan error)
	go u.run(hz, err)
	return u, <-err
}

func (u *sdl2UI) Close() {
	if u.done == nil {
		panic("already closed")
	}
	done := make(chan struct{})
	u.done <- done
	<-done
	*u = sdl2UI{}
}

func (u *sdl2UI) run(hz time.Duration, err chan<- error) {
	defer close(u.done)
	runtime.LockOSThread()
	if C.SDL_VideoInit(nil) < 0 {
		err <- sdlError()
	}
	err <- nil

	tick := time.Tick(hz)
	for {
		select {
		case <-tick:
			u.events()
		case f := <-u.do:
			f()
		case d := <-u.done:
			for _, w := range u.wins {
				w.close()
			}
			C.SDL_VideoQuit()
			close(d)
			return
		}
	}
}

func (u *sdl2UI) Do(f func() interface{}) interface{} {
	ret := make(chan interface{})
	u.do <- func() { ret <- f() }
	return <-ret
}

func (u *sdl2UI) events() {
	var ev C.SDL_Event
	for C.SDL_PollEvent(&ev) == 1 {
		switch t := C.eventType(&ev); t {
		case C.SDL_KEYDOWN:
			fallthrough
		case C.SDL_KEYUP:
			e := (*C.SDL_KeyboardEvent)(unsafe.Pointer(&ev))
			k, ok := keys[e.keysym.sym]
			if !ok {
				continue
			}
			u.send(e.windowID, ui.KeyEvent{
				Down: t == C.SDL_KEYDOWN,
				Key:  k,
			})

		case C.SDL_MOUSEBUTTONDOWN:
			fallthrough
		case C.SDL_MOUSEBUTTONUP:
			e := (*C.SDL_MouseButtonEvent)(unsafe.Pointer(&ev))
			if b, ok := buttons[e.button]; ok {
				u.send(e.windowID, ui.ButtonEvent{
					Down:   t == C.SDL_MOUSEBUTTONDOWN,
					Point:  image.Point{X: int(e.x), Y: int(e.y)},
					Button: b,
				})
			}

		case C.SDL_MOUSEMOTION:
			e := (*C.SDL_MouseMotionEvent)(unsafe.Pointer(&ev))
			u.send(e.windowID, ui.MotionEvent{
				Point: image.Point{X: int(e.x), Y: int(e.y)},
				Delta: image.Point{X: int(e.xrel), Y: int(e.yrel)},
			})

		case C.SDL_MOUSEWHEEL:
			e := (*C.SDL_MouseWheelEvent)(unsafe.Pointer(&ev))
			u.send(e.windowID, ui.WheelEvent{
				Delta: image.Point{X: int(e.x), Y: int(e.y)},
			})

		case C.SDL_TEXTINPUT:
			e := (*C.SDL_TextInputEvent)(unsafe.Pointer(&ev))
			r, _ := utf8.DecodeRuneInString(C.GoString(&e.text[0]))
			u.send(e.windowID, ui.TextEvent{Rune: r})

		case C.SDL_WINDOWEVENT:
			e := (*C.SDL_WindowEvent)(unsafe.Pointer(&ev))
			switch t := e.event; t {
			case C.SDL_WINDOWEVENT_CLOSE:
				u.send(e.windowID, ui.CloseEvent{})

			case C.SDL_WINDOWEVENT_FOCUS_GAINED:
				fallthrough
			case C.SDL_WINDOWEVENT_FOCUS_LOST:
				u.send(e.windowID, ui.FocusEvent{
					Gained: t == C.SDL_WINDOWEVENT_FOCUS_GAINED,
				})

			case C.SDL_WINDOWEVENT_SIZE_CHANGED:
				u.send(e.windowID, ui.ResizeEvent{
					Size: image.Point{X: int(e.data1), Y: int(e.data2)},
				})
			}
		}
	}
}

func (u *sdl2UI) send(id C.Uint32, e interface{}) {
	if w, ok := u.wins[id]; ok {
		// This send must not block waiting for a go routine
		// that may also be blocked on sdl2UI.do.
		w.in <- e
	}
}

type window struct {
	id     C.Uint32
	bounds image.Rectangle

	// In and Out are the input and output event channels.
	// The sdl2UI.run go routine sends events on in.
	// The window.run go routine receives and buffers events from in.
	// The window.run go routine sends buffered events to out.
	// Out is returned by window.Events().
	in, out chan interface{}

	// The following are owned by the sdl2UI.run go routine.
	u *sdl2UI
	w *C.SDL_Window
	r *C.SDL_Renderer
}

func (u *sdl2UI) NewWindow(title string, w, h int) ui.Window {
	return u.Do(func() interface{} {
		win := &window{
			bounds: image.Rect(0, 0, w, h),
			in:     make(chan interface{}, 10),
			out:    make(chan interface{}, 10),
			u:      u,
		}
		win.w = newWindow(title, w, h)
		win.id = C.SDL_GetWindowID(win.w)
		win.r = newRenderer(win.w)
		u.wins[win.id] = win
		go win.run()
		return win
	}).(ui.Window)
}

// NewWindow returns a new C.SDL_Window.
// It must only be called from the sdl2UI.run go routine.
func newWindow(title string, w, h int) *C.SDL_Window {
	const (
		flags C.Uint32 = C.SDL_WINDOW_ALLOW_HIGHDPI |
			C.SDL_WINDOW_OPENGL |
			C.SDL_WINDOW_RESIZABLE |
			C.SDL_WINDOW_SHOWN
		x = C.SDL_WINDOWPOS_UNDEFINED
		y = C.SDL_WINDOWPOS_UNDEFINED
	)
	t := C.CString(title)
	defer C.free(unsafe.Pointer(t))
	win := C.SDL_CreateWindow(t, x, y, C.int(w), C.int(h), flags)
	if win == nil {
		panic(sdlError())
	}
	return win
}

// NewRenderer returns a new C.SDL_Renderer.
// It must only be called from the sdl2UI.run go routine.
func newRenderer(w *C.SDL_Window) *C.SDL_Renderer {
	const flags C.Uint32 = 0
	r := C.SDL_CreateRenderer(w, -1, flags)
	if r == nil {
		panic(sdlError())
	}
	const blend = C.SDL_BLENDMODE_BLEND
	if C.SDL_SetRenderDrawBlendMode(r, blend) < 0 {
		panic(sdlError())
	}
	return r
}

// Run is the window's main event handling loop.
// It receives and buffers events from win.in,
// sends buffered events to win.out,
// and exits when win.in is closed.
// It must be called in its own go routine.
//
// This is necessary because the sdl2UI.run go routine
// must never block sending an event to a go routine
// that may also be blocked on sdl2UI.Do.
// The window.run go routine doesn't block;
// it is always ready to receive an event.
func (win *window) run() {
	var next interface{}
	var events []interface{}
	var out chan interface{}
	for {
		select {
		case ev, ok := <-win.in:
			if !ok {
				close(win.out)
				return
			}
			events = append(events, ev)
		case out <- next:
		}
		if len(events) == 0 {
			// Set out to nil, since there is no next to send yet.
			out = nil
		} else {
			out = win.out
			next = events[0]
			copy(events, events[1:])
			events = events[:len(events)-1]
		}
	}
}

func (win *window) Close() {
	win.u.Do(func() interface{} { win.close(); return nil })
}

// Close removes the window from the ui's window map,
// destroys the window's SDL resources,
// and closes win.in which stops window.run.
// It must only be called from the sdl2UI.run go routine.
func (win *window) close() {
	if _, ok := win.u.wins[win.id]; !ok {
		panic("already closed")
	}
	delete(win.u.wins, win.id)
	C.SDL_DestroyRenderer(win.r)
	C.SDL_DestroyWindow(win.w)
	close(win.in)
}

func (win *window) Events() <-chan interface{} { return win.out }

func (win *window) Draw(d ui.Drawer) {
	win.u.Do(func() interface{} {
		c := canvas{win: win}
		d.Draw(&c)
		C.SDL_RenderPresent(win.r)
		return nil
	})
}

func (win *window) Texture(b image.Rectangle) ui.Texture {
	return win.u.Do(func() interface{} {
		const (
			acc = C.SDL_TEXTUREACCESS_STREAMING
			fmt = C.SDL_PIXELFORMAT_ABGR8888
		)
		w, h := C.int(b.Dx()), C.int(b.Dy())
		t := C.SDL_CreateTexture(win.r, fmt, acc, w, h)
		if t == nil {
			panic(sdlError())
		}
		if C.SDL_SetTextureBlendMode(t, C.SDL_BLENDMODE_BLEND) < 0 {
			panic(sdlError())
		}
		img := &texture{r: win.r, t: t, b: b, locked: false}
		return img
	}).(ui.Texture)
}

type texture struct {
	r      *C.SDL_Renderer
	t      *C.SDL_Texture
	pix    *C.Uint8
	stride int
	locked bool
	b      image.Rectangle
}

func (t *texture) Close() { C.SDL_DestroyTexture(t.t) }

func (t *texture) ColorModel() color.Model { return color.NRGBAModel }

func (t *texture) Bounds() image.Rectangle { return t.b }

func (t *texture) At(x, y int) color.Color {
	t.lock()
	i := uintptr(y*t.stride + x*4)
	return color.NRGBA{
		R: uint8(*t.at(i)),
		G: uint8(*t.at(i + 1)),
		B: uint8(*t.at(i + 2)),
		A: uint8(*t.at(i + 3)),
	}
}

func (t *texture) Set(x, y int, c color.Color) {
	t.lock()
	r, g, b, a := rgba(c)
	i := uintptr(y*t.stride + x*4)
	*t.at(i) = r
	*t.at(i + 1) = g
	*t.at(i + 2) = b
	*t.at(i + 3) = a
}

func (t *texture) at(i uintptr) *C.Uint8 {
	return (*C.Uint8)(unsafe.Pointer(uintptr(unsafe.Pointer(t.pix)) + i))
}

func (t *texture) lock() {
	if t.locked {
		return
	}
	var pix unsafe.Pointer
	var stride C.int
	if C.SDL_LockTexture(t.t, rect(t.b), &pix, &stride) < 0 {
		panic(sdlError())
	}
	t.pix = (*C.Uint8)(pix)
	t.stride = int(stride)
	t.locked = true
}

func (t *texture) unlock() {
	if !t.locked {
		return
	}
	C.SDL_UnlockTexture(t.t)
	t.locked = false
}

// All of the methods on canvas
// must be called from the sdl2UI.run go routine.
type canvas struct {
	win *window
}

func (c *canvas) Bounds() image.Rectangle { return c.win.bounds }

func (c *canvas) setColor(col color.Color) {
	r, g, b, a := rgba(col)
	if C.SDL_SetRenderDrawColor(c.win.r, r, g, b, a) < 0 {
		panic(sdlError())
	}
}

func (c *canvas) Fill(col color.Color, r image.Rectangle) {
	c.setColor(col)
	if C.SDL_RenderFillRect(c.win.r, rect(r)) < 0 {
		panic(sdlError())
	}
}

func (c *canvas) Stroke(col color.Color, pts ...image.Point) {
	c.setColor(col)
	for i := 1; i < len(pts); i++ {
		x0, y0 := C.int(pts[i-1].X), C.int(pts[i-1].Y)
		x1, y1 := C.int(pts[i].X), C.int(pts[i].Y)
		if C.SDL_RenderDrawLine(c.win.r, x0, y0, x1, y1) < 0 {
			panic(sdlError())
		}
	}
}

func (c *canvas) Draw(img image.Image, pt image.Point) {
	switch img := img.(type) {
	case *texture:
		if img.r == c.win.r {
			drawTexture(c.win.r, pt, img)
		} else {
			drawNRGBA(c.win.r, pt, makeNRGBA(img))
		}
	case *image.NRGBA:
		drawNRGBA(c.win.r, pt, img)
	default:
		drawNRGBA(c.win.r, pt, makeNRGBA(img))
	}
}

func drawTexture(r *C.SDL_Renderer, pt image.Point, t *texture) {
	w, h := t.b.Dx(), t.b.Dy()
	dst := image.Rect(pt.X, pt.Y, pt.X+w, pt.Y+h)
	t.unlock()
	if C.SDL_RenderCopy(r, t.t, nil, rect(dst)) < 0 {
		panic(sdlError())
	}
}

func drawNRGBA(r *C.SDL_Renderer, pt image.Point, img *image.NRGBA) {
	t := tex(r, img)
	defer C.SDL_DestroyTexture(t)
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.Rect(pt.X, pt.Y, pt.X+w, pt.Y+h)
	if C.SDL_RenderCopy(r, t, nil, rect(dst)) < 0 {
		panic(sdlError())
	}
}

func makeNRGBA(img image.Image) *image.NRGBA {
	b := img.Bounds()
	nrgba := image.NewNRGBA(b)
	draw.Draw(nrgba, b, img, image.ZP, draw.Over)
	return nrgba
}

func rgba(col color.Color) (C.Uint8, C.Uint8, C.Uint8, C.Uint8) {
	const f float64 = 255.0 / 0xFFFF
	r, g, b, a := col.RGBA()
	return C.Uint8(float64(r) * f), C.Uint8(float64(g) * f),
		C.Uint8(float64(b) * f), C.Uint8(float64(a) * f)
}

func rect(r image.Rectangle) *C.SDL_Rect {
	return &C.SDL_Rect{
		x: C.int(r.Min.X),
		y: C.int(r.Min.Y),
		w: C.int(r.Max.X - r.Min.X),
		h: C.int(r.Max.Y - r.Min.Y),
	}
}

func tex(r *C.SDL_Renderer, img *image.NRGBA) *C.SDL_Texture {
	const (
		acc = C.SDL_TEXTUREACCESS_STATIC
		fmt = C.SDL_PIXELFORMAT_ABGR8888
	)
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	tex := C.SDL_CreateTexture(r, fmt, acc, C.int(w), C.int(h))
	if tex == nil {
		panic(sdlError())
	}
	if C.SDL_UpdateTexture(tex, nil, unsafe.Pointer(&img.Pix[0]), C.int(img.Stride)) < 0 {
		panic(sdlError())
	}
	if C.SDL_SetTextureBlendMode(tex, C.SDL_BLENDMODE_BLEND) < 0 {
		panic(sdlError())
	}
	return tex
}

// SdlError returns an error containing an SDL2 error string.
func sdlError() error { return errors.New(C.GoString(C.SDL_GetError())) }

var buttons = map[C.Uint8]ui.Button{
	C.SDL_BUTTON_LEFT:   ui.ButtonLeft,
	C.SDL_BUTTON_MIDDLE: ui.ButtonMiddle,
	C.SDL_BUTTON_RIGHT:  ui.ButtonRight,
}

var keys = map[C.SDL_Keycode]ui.Key{
	C.SDLK_0:                  ui.Key0,
	C.SDLK_1:                  ui.Key1,
	C.SDLK_2:                  ui.Key2,
	C.SDLK_3:                  ui.Key3,
	C.SDLK_4:                  ui.Key4,
	C.SDLK_5:                  ui.Key5,
	C.SDLK_6:                  ui.Key6,
	C.SDLK_7:                  ui.Key7,
	C.SDLK_8:                  ui.Key8,
	C.SDLK_9:                  ui.Key9,
	C.SDLK_a:                  ui.KeyA,
	C.SDLK_AC_BACK:            ui.KeyAcBack,
	C.SDLK_AC_BOOKMARKS:       ui.KeyAcBookmarks,
	C.SDLK_AC_FORWARD:         ui.KeyAcForward,
	C.SDLK_AC_HOME:            ui.KeyAcHome,
	C.SDLK_AC_REFRESH:         ui.KeyAcRefresh,
	C.SDLK_AC_SEARCH:          ui.KeyAcSearch,
	C.SDLK_AC_STOP:            ui.KeyAcStop,
	C.SDLK_AGAIN:              ui.KeyAgain,
	C.SDLK_ALTERASE:           ui.KeyAlterase,
	C.SDLK_QUOTE:              ui.KeyApostrophe,
	C.SDLK_APPLICATION:        ui.KeyApplication,
	C.SDLK_AUDIOMUTE:          ui.KeyAudioMute,
	C.SDLK_AUDIONEXT:          ui.KeyAudioNext,
	C.SDLK_AUDIOPLAY:          ui.KeyAudioPlay,
	C.SDLK_AUDIOPREV:          ui.KeyAudioPrev,
	C.SDLK_AUDIOSTOP:          ui.KeyAudioStop,
	C.SDLK_b:                  ui.KeyB,
	C.SDLK_BACKSLASH:          ui.KeyBackslash,
	C.SDLK_BACKSPACE:          ui.KeyBackspace,
	C.SDLK_BRIGHTNESSDOWN:     ui.KeyBrightnessDown,
	C.SDLK_BRIGHTNESSUP:       ui.KeyBrightnessUp,
	C.SDLK_c:                  ui.KeyC,
	C.SDLK_CALCULATOR:         ui.KeyCalculator,
	C.SDLK_CANCEL:             ui.KeyCancel,
	C.SDLK_CAPSLOCK:           ui.KeyCapsLock,
	C.SDLK_CLEAR:              ui.KeyClear,
	C.SDLK_CLEARAGAIN:         ui.KeyClearAgain,
	C.SDLK_COMMA:              ui.KeyComma,
	C.SDLK_COMPUTER:           ui.KeyComputer,
	C.SDLK_COPY:               ui.KeyCopy,
	C.SDLK_CRSEL:              ui.KeyCrSel,
	C.SDLK_CURRENCYSUBUNIT:    ui.KeyCurrencySubUnit,
	C.SDLK_CURRENCYUNIT:       ui.KeyCurrencyUnit,
	C.SDLK_CUT:                ui.KeyCut,
	C.SDLK_d:                  ui.KeyD,
	C.SDLK_DECIMALSEPARATOR:   ui.KeyDecimalSeparator,
	C.SDLK_DELETE:             ui.KeyDelete,
	C.SDLK_DISPLAYSWITCH:      ui.KeyDisplaySwitch,
	C.SDLK_DOWN:               ui.KeyDown,
	C.SDLK_e:                  ui.KeyE,
	C.SDLK_EJECT:              ui.KeyEject,
	C.SDLK_END:                ui.KeyEnd,
	C.SDLK_EQUALS:             ui.KeyEquals,
	C.SDLK_ESCAPE:             ui.KeyEscape,
	C.SDLK_EXECUTE:            ui.KeyExecute,
	C.SDLK_EXSEL:              ui.KeyExSel,
	C.SDLK_f:                  ui.KeyF,
	C.SDLK_F1:                 ui.KeyF1,
	C.SDLK_F10:                ui.KeyF10,
	C.SDLK_F11:                ui.KeyF11,
	C.SDLK_F12:                ui.KeyF12,
	C.SDLK_F13:                ui.KeyF13,
	C.SDLK_F14:                ui.KeyF14,
	C.SDLK_F15:                ui.KeyF15,
	C.SDLK_F16:                ui.KeyF16,
	C.SDLK_F17:                ui.KeyF17,
	C.SDLK_F18:                ui.KeyF18,
	C.SDLK_F19:                ui.KeyF19,
	C.SDLK_F2:                 ui.KeyF2,
	C.SDLK_F20:                ui.KeyF20,
	C.SDLK_F21:                ui.KeyF21,
	C.SDLK_F22:                ui.KeyF22,
	C.SDLK_F23:                ui.KeyF23,
	C.SDLK_F24:                ui.KeyF24,
	C.SDLK_F3:                 ui.KeyF3,
	C.SDLK_F4:                 ui.KeyF4,
	C.SDLK_F5:                 ui.KeyF5,
	C.SDLK_F6:                 ui.KeyF6,
	C.SDLK_F7:                 ui.KeyF7,
	C.SDLK_F8:                 ui.KeyF8,
	C.SDLK_F9:                 ui.KeyF9,
	C.SDLK_FIND:               ui.KeyFind,
	C.SDLK_g:                  ui.KeyG,
	C.SDLK_BACKQUOTE:          ui.KeyBackquote,
	C.SDLK_h:                  ui.KeyH,
	C.SDLK_HELP:               ui.KeyHelp,
	C.SDLK_HOME:               ui.KeyHome,
	C.SDLK_i:                  ui.KeyI,
	C.SDLK_INSERT:             ui.KeyInsert,
	C.SDLK_j:                  ui.KeyJ,
	C.SDLK_k:                  ui.KeyK,
	C.SDLK_KBDILLUMDOWN:       ui.KeyKBDIllumDown,
	C.SDLK_KBDILLUMTOGGLE:     ui.KeyKBDIllumToggle,
	C.SDLK_KBDILLUMUP:         ui.KeyKBDIllumUp,
	C.SDLK_KP_0:               ui.KeyKP0,
	C.SDLK_KP_00:              ui.KeyKP00,
	C.SDLK_KP_000:             ui.KeyKP000,
	C.SDLK_KP_1:               ui.KeyKP1,
	C.SDLK_KP_2:               ui.KeyKP2,
	C.SDLK_KP_3:               ui.KeyKP3,
	C.SDLK_KP_4:               ui.KeyKP4,
	C.SDLK_KP_5:               ui.KeyKP5,
	C.SDLK_KP_6:               ui.KeyKP6,
	C.SDLK_KP_7:               ui.KeyKP7,
	C.SDLK_KP_8:               ui.KeyKP8,
	C.SDLK_KP_9:               ui.KeyKP9,
	C.SDLK_KP_A:               ui.KeyKPA,
	C.SDLK_KP_AMPERSAND:       ui.KeyKPAmpersand,
	C.SDLK_KP_AT:              ui.KeyKPAt,
	C.SDLK_KP_B:               ui.KeyKPB,
	C.SDLK_KP_BACKSPACE:       ui.KeyKPBackspace,
	C.SDLK_KP_BINARY:          ui.KeyKPBinary,
	C.SDLK_KP_C:               ui.KeyKPC,
	C.SDLK_KP_CLEAR:           ui.KeyKPClear,
	C.SDLK_KP_CLEARENTRY:      ui.KeyKPClearEntry,
	C.SDLK_KP_COLON:           ui.KeyKPColon,
	C.SDLK_KP_COMMA:           ui.KeyKPComma,
	C.SDLK_KP_D:               ui.KeyKPD,
	C.SDLK_KP_DBLAMPERSAND:    ui.KeyKPDBLAmperSand,
	C.SDLK_KP_DBLVERTICALBAR:  ui.KeyKPDBLVerticalBar,
	C.SDLK_KP_DECIMAL:         ui.KeyKPDecimal,
	C.SDLK_KP_DIVIDE:          ui.KeyKPDivide,
	C.SDLK_KP_E:               ui.KeyKPE,
	C.SDLK_KP_ENTER:           ui.KeyKPEnter,
	C.SDLK_KP_EQUALS:          ui.KeyKPEquals,
	C.SDLK_KP_EQUALSAS400:     ui.KeyKPEqualsAs400,
	C.SDLK_KP_EXCLAM:          ui.KeyKPExclamation,
	C.SDLK_KP_F:               ui.KeyKPF,
	C.SDLK_KP_GREATER:         ui.KeyKPGreater,
	C.SDLK_KP_HASH:            ui.KeyKPHash,
	C.SDLK_KP_HEXADECIMAL:     ui.KeyKPHexadecimal,
	C.SDLK_KP_LEFTBRACE:       ui.KeyKPLeftBrace,
	C.SDLK_KP_LEFTPAREN:       ui.KeyKPLeftParen,
	C.SDLK_KP_LESS:            ui.KeyKPLess,
	C.SDLK_KP_MEMADD:          ui.KeyKPMemAdd,
	C.SDLK_KP_MEMCLEAR:        ui.KeyKPMemClear,
	C.SDLK_KP_MEMDIVIDE:       ui.KeyKPMemDivide,
	C.SDLK_KP_MEMMULTIPLY:     ui.KeyKPMemMultiply,
	C.SDLK_KP_MEMRECALL:       ui.KeyKPMemRecall,
	C.SDLK_KP_MEMSTORE:        ui.KeyKPMemStore,
	C.SDLK_KP_MEMSUBTRACT:     ui.KeyKPMemSubtract,
	C.SDLK_KP_MINUS:           ui.KeyKPMinus,
	C.SDLK_KP_MULTIPLY:        ui.KeyKPMultiple,
	C.SDLK_KP_OCTAL:           ui.KeyKPOctal,
	C.SDLK_KP_PERCENT:         ui.KeyKPPercent,
	C.SDLK_KP_PERIOD:          ui.KeyKPPeriod,
	C.SDLK_KP_PLUS:            ui.KeyKPPlus,
	C.SDLK_KP_PLUSMINUS:       ui.KeyKPPlusMinus,
	C.SDLK_KP_POWER:           ui.KeyKPPower,
	C.SDLK_KP_RIGHTBRACE:      ui.KeyKPRightBrace,
	C.SDLK_KP_RIGHTPAREN:      ui.KeyKPRightParen,
	C.SDLK_KP_SPACE:           ui.KeyKPSpace,
	C.SDLK_KP_TAB:             ui.KeyKPTab,
	C.SDLK_KP_VERTICALBAR:     ui.KeyKPVerticalBar,
	C.SDLK_KP_XOR:             ui.KeyKPXOR,
	C.SDLK_l:                  ui.KeyL,
	C.SDLK_LALT:               ui.KeyLeftAlt,
	C.SDLK_LCTRL:              ui.KeyLeftCtrl,
	C.SDLK_LEFT:               ui.KeyLeft,
	C.SDLK_LEFTBRACKET:        ui.KeyLeftBracket,
	C.SDLK_LGUI:               ui.KeyLGUI,
	C.SDLK_LSHIFT:             ui.KeyLShift,
	C.SDLK_m:                  ui.KeyM,
	C.SDLK_MAIL:               ui.KeyMail,
	C.SDLK_MEDIASELECT:        ui.KeyMediaSelect,
	C.SDLK_MENU:               ui.KeyMenu,
	C.SDLK_MINUS:              ui.KeyMinus,
	C.SDLK_MODE:               ui.KeyMode,
	C.SDLK_MUTE:               ui.KeyMute,
	C.SDLK_n:                  ui.KeyN,
	C.SDLK_NUMLOCKCLEAR:       ui.KeyNumLockClear,
	C.SDLK_o:                  ui.KeyO,
	C.SDLK_OPER:               ui.KeyOper,
	C.SDLK_OUT:                ui.KeyOut,
	C.SDLK_p:                  ui.KeyP,
	C.SDLK_PAGEDOWN:           ui.KeyPageDown,
	C.SDLK_PAGEUP:             ui.KeyPageUp,
	C.SDLK_PASTE:              ui.KeyPaste,
	C.SDLK_PAUSE:              ui.KeyPause,
	C.SDLK_PERIOD:             ui.KeyPeriod,
	C.SDLK_POWER:              ui.KeyPower,
	C.SDLK_PRINTSCREEN:        ui.KeyPrintScreen,
	C.SDLK_PRIOR:              ui.KeyPrior,
	C.SDLK_q:                  ui.KeyQ,
	C.SDLK_r:                  ui.KeyR,
	C.SDLK_RALT:               ui.KeyRightAlt,
	C.SDLK_RCTRL:              ui.KeyRightCtrl,
	C.SDLK_RETURN:             ui.KeyReturn,
	C.SDLK_RETURN2:            ui.KeyReturn2,
	C.SDLK_RGUI:               ui.KeyRGUI,
	C.SDLK_RIGHT:              ui.KeyRight,
	C.SDLK_RIGHTBRACKET:       ui.KeyRightBracket,
	C.SDLK_RSHIFT:             ui.KeyRightShift,
	C.SDLK_s:                  ui.KeyS,
	C.SDLK_SCROLLLOCK:         ui.KeyScrollLock,
	C.SDLK_SELECT:             ui.KeySelect,
	C.SDLK_SEMICOLON:          ui.KeySemicolon,
	C.SDLK_SEPARATOR:          ui.KeySeparator,
	C.SDLK_SLASH:              ui.KeySlash,
	C.SDLK_SLEEP:              ui.KeySleep,
	C.SDLK_SPACE:              ui.KeySpace,
	C.SDLK_STOP:               ui.KeyStop,
	C.SDLK_SYSREQ:             ui.KeySysReq,
	C.SDLK_t:                  ui.KeyT,
	C.SDLK_TAB:                ui.KeyTab,
	C.SDLK_THOUSANDSSEPARATOR: ui.KeyThousandsSeparator,
	C.SDLK_u:                  ui.KeyU,
	C.SDLK_UNDO:               ui.KeyUndo,
	C.SDLK_UNKNOWN:            ui.KeyUnknown,
	C.SDLK_UP:                 ui.KeyUp,
	C.SDLK_v:                  ui.KeyV,
	C.SDLK_VOLUMEDOWN:         ui.KeyVolumeDown,
	C.SDLK_VOLUMEUP:           ui.KeyVolumeUp,
	C.SDLK_w:                  ui.KeyW,
	C.SDLK_WWW:                ui.KeyWWW,
	C.SDLK_x:                  ui.KeyX,
	C.SDLK_y:                  ui.KeyY,
	C.SDLK_z:                  ui.KeyZ,
	C.SDLK_AMPERSAND:          ui.KeyAmpersand,
	C.SDLK_ASTERISK:           ui.KeyAsterisk,
	C.SDLK_AT:                 ui.KeyAt,
	C.SDLK_CARET:              ui.KeyCaret,
	C.SDLK_COLON:              ui.KeyColon,
	C.SDLK_DOLLAR:             ui.KeyDollar,
	C.SDLK_EXCLAIM:            ui.KeyExclaim,
	C.SDLK_GREATER:            ui.KeyGreater,
	C.SDLK_HASH:               ui.KeyHash,
	C.SDLK_LEFTPAREN:          ui.KeyLeftParen,
	C.SDLK_LESS:               ui.KeyLess,
	C.SDLK_PERCENT:            ui.KeyPercent,
	C.SDLK_PLUS:               ui.KeyPlus,
	C.SDLK_QUESTION:           ui.KeyQuestion,
	C.SDLK_QUOTEDBL:           ui.KeyQuote,
	C.SDLK_RIGHTPAREN:         ui.KeyRightParen,
	C.SDLK_UNDERSCORE:         ui.KeyUnderscore,
}
