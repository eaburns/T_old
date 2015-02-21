// Package ui provides a platform-independent API
// for manipulating and drawing to windows
// and reading keyboard and mouse events.
package ui

import (
	"image"
	"image/color"
	"image/draw"
	"strconv"
)

// A UI is a user interface.
type UI interface {
	// NewWindow creates and returns
	// a new Window on the user's display.
	NewWindow(title string, w, h int) Window
	// Close closes all open Windows and the UI.
	// The UI should not be used after Close is called.
	Close()
}

// A Window represents a graphical window on the user's display.
type Window interface {
	// Events returns a channel of the Window's events.
	// The channel is closed when the Window is Closed.
	Events() <-chan interface{}
	// Draw calls the Draw method of a Drawer
	// with a Canvas on this Window.
	Draw(Drawer)
	// Texture returns a new image that may be optimized
	// for drawing to this Window.
	// The initial pixels of the returned Texture are undefined,
	// but they may be set using the Set method.
	// The Texture must be closed when no longer needed
	// in order to free its resources.
	Texture(image.Rectangle) Texture
	// Close closes the Window and its event channel.
	Close()
}

// A Texture is a drawable image with a Close method
// to free its resources.
// It may be associated with a Window
// to provide more efficient drawing.
type Texture interface {
	draw.Image
	Close()
}

// A Canvas represents a portion of a Window.
// It provides functionality for drawing on the Window.
//
// Metods on Canvas must only be called
// from the go routine that calls Drawer.Draw.
type Canvas interface {
	// Bounds returns the bounds of the Canvas within its Window.
	Bounds() image.Rectangle
	// SetColor sets the current drawing color.
	// The default draw color is color.Black.
	SetColor(color.Color)
	// Fill fills the rectangular portion of the Canvas
	// with the current drawing color.
	Fill(image.Rectangle)
	// Stroke strokes a single-pixel line between the points
	// in the current drawing color.
	Stroke(...image.Point)
	// Draw draws an image to the canvas.
	// The first arguments specifies the point on the canvas
	// to which the upper left corner of the image will be drawn.
	//
	// Draw can draw any image to the Canvas,
	// but the most efficient way to draw an image
	// is to use a Texture created by the Canvas's
	// associated Window.
	Draw(image.Point, image.Image)
}

// A Drawer can draw itself to a Canvas.
type Drawer interface {
	Draw(Canvas)
}

// A ButtonEvent indicates that a mouse button
// was either pressed or released.
type ButtonEvent struct {
	// Down is true if the button was pressed.
	// Down is false if the button was released.
	Down bool
	// Point gives the coordinates of the pointer
	// relative to the window.
	// 0, 0 represents the upper left corner of the window.
	image.Point
	// Button is the button that caused the event.
	Button Button
}

// A CloseEvent indicates that the Window's close button was pressed.
type CloseEvent struct{}

// A FocusEvent indicates that the Window focus changed.
type FocusEvent struct {
	// Gained is true if the Window gained focus.
	// Gained is false if the Window lost focus.
	Gained bool
}

// A KeyEvent indicates that a key was either pressed or released.
type KeyEvent struct {
	// Down is true if the key was pressed.
	// Down is false if the key was released.
	Down bool
	// Key is the key that caused the event.
	Key Key
}

// A MotionEvent indicates mouse motion.
type MotionEvent struct {
	// Point gives the coordinates of the pointer
	// relative to the window.
	// 0, 0 represents the upper left corner of the window.
	image.Point
	// Delta gives the relative motion of the mouse
	// in the horizontal and vertical directions.
	Delta image.Point
}

// A ResizeEvent indicates that the Window was resized.
type ResizeEvent struct {
	// Size gives the new width and height of the window.
	Size image.Point
}

// A TextEvent inticates text typed on the keyboard.
// It differs from a KeyEvent in that it may indicate
// the result of a chord consisting of multiple key presses.
//
// For example,
// pressing 'a' will generate a KeyEvent for KeyA and a TextEvent 'a';
// pressing Shift+'a' will generate a KeyEvent for KeyShift,
// a KeyEvent for KeyA, and a TextEvent for 'A'.
type TextEvent struct {
	// Rune is the rune that was typed.
	Rune rune
}

// A WheelEvent indicates a mouse wheel movement.
type WheelEvent struct {
	// Delta gives the amount scrolled horizontally and vertically.
	Delta image.Point
}

// A Button identifies a button on a mouse.
type Button int

// Mouse button constants.
const (
	ButtonLeft Button = iota
	ButtonRight
	ButtonMiddle
)

// A Key identifies a key on a keyboard.
type Key int

// Key constants.
//
// These are based on the SDL_Keycode values from
// http://wiki.libsdl.org/SDL_Keycode
const (
	Key0 Key = iota
	Key1
	Key2
	Key3
	Key4
	Key5
	Key6
	Key7
	Key8
	Key9
	KeyA
	KeyAcBack
	KeyAcBookmarks
	KeyAcForward
	KeyAcHome
	KeyAcRefresh
	KeyAcSearch
	KeyAcStop
	KeyAgain
	KeyAlterase
	KeyApostrophe
	KeyApplication
	KeyAudioMute
	KeyAudioNext
	KeyAudioPlay
	KeyAudioPrev
	KeyAudioStop
	KeyB
	KeyBackslash
	KeyBackspace
	KeyBrightnessDown
	KeyBrightnessUp
	KeyC
	KeyCalculator
	KeyCancel
	KeyCapsLock
	KeyClear
	KeyClearAgain
	KeyComma
	KeyComputer
	KeyCopy
	KeyCrSel
	KeyCurrencySubUnit
	KeyCurrencyUnit
	KeyCut
	KeyD
	KeyDecimalSeparator
	KeyDelete
	KeyDisplaySwitch
	KeyDown
	KeyE
	KeyEject
	KeyEnd
	KeyEquals
	KeyEscape
	KeyExecute
	KeyExSel
	KeyF
	KeyF1
	KeyF10
	KeyF11
	KeyF12
	KeyF13
	KeyF14
	KeyF15
	KeyF16
	KeyF17
	KeyF18
	KeyF19
	KeyF2
	KeyF20
	KeyF21
	KeyF22
	KeyF23
	KeyF24
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyFind
	KeyG
	KeyBackquote
	KeyH
	KeyHelp
	KeyHome
	KeyI
	KeyInsert
	KeyJ
	KeyK
	KeyKBDIllumDown
	KeyKBDIllumToggle
	KeyKBDIllumUp
	KeyKP0
	KeyKP00
	KeyKP000
	KeyKP1
	KeyKP2
	KeyKP3
	KeyKP4
	KeyKP5
	KeyKP6
	KeyKP7
	KeyKP8
	KeyKP9
	KeyKPA
	KeyKPAmpersand
	KeyKPAt
	KeyKPB
	KeyKPBackspace
	KeyKPBinary
	KeyKPC
	KeyKPClear
	KeyKPClearEntry
	KeyKPColon
	KeyKPComma
	KeyKPD
	KeyKPDBLAmperSand
	KeyKPDBLVerticalBar
	KeyKPDecimal
	KeyKPDivide
	KeyKPE
	KeyKPEnter
	KeyKPEquals
	KeyKPEqualsAs400
	KeyKPExclamation
	KeyKPF
	KeyKPGreater
	KeyKPHash
	KeyKPHexadecimal
	KeyKPLeftBrace
	KeyKPLeftParen
	KeyKPLess
	KeyKPMemAdd
	KeyKPMemClear
	KeyKPMemDivide
	KeyKPMemMultiply
	KeyKPMemRecall
	KeyKPMemStore
	KeyKPMemSubtract
	KeyKPMinus
	KeyKPMultiple
	KeyKPOctal
	KeyKPPercent
	KeyKPPeriod
	KeyKPPlus
	KeyKPPlusMinus
	KeyKPPower
	KeyKPRightBrace
	KeyKPRightParen
	KeyKPSpace
	KeyKPTab
	KeyKPVerticalBar
	KeyKPXOR
	KeyL
	KeyLeftAlt
	KeyLeftCtrl
	KeyLeft
	KeyLeftBracket
	KeyLGUI
	KeyLShift
	KeyM
	KeyMail
	KeyMediaSelect
	KeyMenu
	KeyMinus
	KeyMode
	KeyMute
	KeyN
	KeyNumLockClear
	KeyO
	KeyOper
	KeyOut
	KeyP
	KeyPageDown
	KeyPageUp
	KeyPaste
	KeyPause
	KeyPeriod
	KeyPower
	KeyPrintScreen
	KeyPrior
	KeyQ
	KeyR
	KeyRightAlt
	KeyRightCtrl
	KeyReturn
	KeyReturn2
	KeyRGUI
	KeyRight
	KeyRightBracket
	KeyRightShift
	KeyS
	KeyScrollLock
	KeySelect
	KeySemicolon
	KeySeparator
	KeySlash
	KeySleep
	KeySpace
	KeyStop
	KeySysReq
	KeyT
	KeyTab
	KeyThousandsSeparator
	KeyU
	KeyUndo
	KeyUnknown
	KeyUp
	KeyV
	KeyVolumeDown
	KeyVolumeUp
	KeyW
	KeyWWW
	KeyX
	KeyY
	KeyZ
	KeyAmpersand
	KeyAsterisk
	KeyAt
	KeyCaret
	KeyColon
	KeyDollar
	KeyExclaim
	KeyGreater
	KeyHash
	KeyLeftParen
	KeyLess
	KeyPercent
	KeyPlus
	KeyQuestion
	KeyQuote
	KeyRightParen
	KeyUnderscore
)

var keyStrings = map[Key]string{
	Key0:                  "Key0",
	Key1:                  "Key1",
	Key2:                  "Key2",
	Key3:                  "Key3",
	Key4:                  "Key4",
	Key5:                  "Key5",
	Key6:                  "Key6",
	Key7:                  "Key7",
	Key8:                  "Key8",
	Key9:                  "Key9",
	KeyA:                  "KeyA",
	KeyAcBack:             "KeyAcBack",
	KeyAcBookmarks:        "KeyAcBookmarks",
	KeyAcForward:          "KeyAcForward",
	KeyAcHome:             "KeyAcHome",
	KeyAcRefresh:          "KeyAcRefresh",
	KeyAcSearch:           "KeyAcSearch",
	KeyAcStop:             "KeyAcStop",
	KeyAgain:              "KeyAgain",
	KeyAlterase:           "KeyAlterase",
	KeyApostrophe:         "KeyApostrophe",
	KeyApplication:        "KeyApplication",
	KeyAudioMute:          "KeyAudioMute",
	KeyAudioNext:          "KeyAudioNext",
	KeyAudioPlay:          "KeyAudioPlay",
	KeyAudioPrev:          "KeyAudioPrev",
	KeyAudioStop:          "KeyAudioStop",
	KeyB:                  "KeyB",
	KeyBackslash:          "KeyBackslash",
	KeyBackspace:          "KeyBackspace",
	KeyBrightnessDown:     "KeyBrightnessDown",
	KeyBrightnessUp:       "KeyBrightnessUp",
	KeyC:                  "KeyC",
	KeyCalculator:         "KeyCalculator",
	KeyCancel:             "KeyCancel",
	KeyCapsLock:           "KeyCapsLock",
	KeyClear:              "KeyClear",
	KeyClearAgain:         "KeyClearAgain",
	KeyComma:              "KeyComma",
	KeyComputer:           "KeyComputer",
	KeyCopy:               "KeyCopy",
	KeyCrSel:              "KeyCrSel",
	KeyCurrencySubUnit:    "KeyCurrencySubUnit",
	KeyCurrencyUnit:       "KeyCurrencyUnit",
	KeyCut:                "KeyCut",
	KeyD:                  "KeyD",
	KeyDecimalSeparator:   "KeyDecimalSeparator",
	KeyDelete:             "KeyDelete",
	KeyDisplaySwitch:      "KeyDisplaySwitch",
	KeyDown:               "KeyDown",
	KeyE:                  "KeyE",
	KeyEject:              "KeyEject",
	KeyEnd:                "KeyEnd",
	KeyEquals:             "KeyEquals",
	KeyEscape:             "KeyEscape",
	KeyExecute:            "KeyExecute",
	KeyExSel:              "KeyExSel",
	KeyF:                  "KeyF",
	KeyF1:                 "KeyF1",
	KeyF10:                "KeyF10",
	KeyF11:                "KeyF11",
	KeyF12:                "KeyF12",
	KeyF13:                "KeyF13",
	KeyF14:                "KeyF14",
	KeyF15:                "KeyF15",
	KeyF16:                "KeyF16",
	KeyF17:                "KeyF17",
	KeyF18:                "KeyF18",
	KeyF19:                "KeyF19",
	KeyF2:                 "KeyF2",
	KeyF20:                "KeyF20",
	KeyF21:                "KeyF21",
	KeyF22:                "KeyF22",
	KeyF23:                "KeyF23",
	KeyF24:                "KeyF24",
	KeyF3:                 "KeyF3",
	KeyF4:                 "KeyF4",
	KeyF5:                 "KeyF5",
	KeyF6:                 "KeyF6",
	KeyF7:                 "KeyF7",
	KeyF8:                 "KeyF8",
	KeyF9:                 "KeyF9",
	KeyFind:               "KeyFind",
	KeyG:                  "KeyG",
	KeyBackquote:          "KeyBackquote",
	KeyH:                  "KeyH",
	KeyHelp:               "KeyHelp",
	KeyHome:               "KeyHome",
	KeyI:                  "KeyI",
	KeyInsert:             "KeyInsert",
	KeyJ:                  "KeyJ",
	KeyK:                  "KeyK",
	KeyKBDIllumDown:       "KeyKBDIllumDown",
	KeyKBDIllumToggle:     "KeyKBDIllumToggle",
	KeyKBDIllumUp:         "KeyKBDIllumUp",
	KeyKP0:                "KeyKP0",
	KeyKP00:               "KeyKP00",
	KeyKP000:              "KeyKP000",
	KeyKP1:                "KeyKP1",
	KeyKP2:                "KeyKP2",
	KeyKP3:                "KeyKP3",
	KeyKP4:                "KeyKP4",
	KeyKP5:                "KeyKP5",
	KeyKP6:                "KeyKP6",
	KeyKP7:                "KeyKP7",
	KeyKP8:                "KeyKP8",
	KeyKP9:                "KeyKP9",
	KeyKPA:                "KeyKPA",
	KeyKPAmpersand:        "KeyKPAmpersand",
	KeyKPAt:               "KeyKPAt",
	KeyKPB:                "KeyKPB",
	KeyKPBackspace:        "KeyKPBackspace",
	KeyKPBinary:           "KeyKPBinary",
	KeyKPC:                "KeyKPC",
	KeyKPClear:            "KeyKPClear",
	KeyKPClearEntry:       "KeyKPClearEntry",
	KeyKPColon:            "KeyKPColon",
	KeyKPComma:            "KeyKPComma",
	KeyKPD:                "KeyKPD",
	KeyKPDBLAmperSand:     "KeyKPDBLAmperSand",
	KeyKPDBLVerticalBar:   "KeyKPDBLVerticalBar",
	KeyKPDecimal:          "KeyKPDecimal",
	KeyKPDivide:           "KeyKPDivide",
	KeyKPE:                "KeyKPE",
	KeyKPEnter:            "KeyKPEnter",
	KeyKPEquals:           "KeyKPEquals",
	KeyKPEqualsAs400:      "KeyKPEqualsAs400",
	KeyKPExclamation:      "KeyKPExclamation",
	KeyKPF:                "KeyKPF",
	KeyKPGreater:          "KeyKPGreater",
	KeyKPHash:             "KeyKPHash",
	KeyKPHexadecimal:      "KeyKPHexadecimal",
	KeyKPLeftBrace:        "KeyKPLeftBrace",
	KeyKPLeftParen:        "KeyKPLeftParen",
	KeyKPLess:             "KeyKPLess",
	KeyKPMemAdd:           "KeyKPMemAdd",
	KeyKPMemClear:         "KeyKPMemClear",
	KeyKPMemDivide:        "KeyKPMemDivide",
	KeyKPMemMultiply:      "KeyKPMemMultiply",
	KeyKPMemRecall:        "KeyKPMemRecall",
	KeyKPMemStore:         "KeyKPMemStore",
	KeyKPMemSubtract:      "KeyKPMemSubtract",
	KeyKPMinus:            "KeyKPMinus",
	KeyKPMultiple:         "KeyKPMultiple",
	KeyKPOctal:            "KeyKPOctal",
	KeyKPPercent:          "KeyKPPercent",
	KeyKPPeriod:           "KeyKPPeriod",
	KeyKPPlus:             "KeyKPPlus",
	KeyKPPlusMinus:        "KeyKPPlusMinus",
	KeyKPPower:            "KeyKPPower",
	KeyKPRightBrace:       "KeyKPRightBrace",
	KeyKPRightParen:       "KeyKPRightParen",
	KeyKPSpace:            "KeyKPSpace",
	KeyKPTab:              "KeyKPTab",
	KeyKPVerticalBar:      "KeyKPVerticalBar",
	KeyKPXOR:              "KeyKPXOR",
	KeyL:                  "KeyL",
	KeyLeftAlt:            "KeyLeftAlt",
	KeyLeftCtrl:           "KeyLeftCtrl",
	KeyLeft:               "KeyLeft",
	KeyLeftBracket:        "KeyLeftBracket",
	KeyLGUI:               "KeyLGUI",
	KeyLShift:             "KeyLShift",
	KeyM:                  "KeyM",
	KeyMail:               "KeyMail",
	KeyMediaSelect:        "KeyMediaSelect",
	KeyMenu:               "KeyMenu",
	KeyMinus:              "KeyMinus",
	KeyMode:               "KeyMode",
	KeyMute:               "KeyMute",
	KeyN:                  "KeyN",
	KeyNumLockClear:       "KeyNumLockClear",
	KeyO:                  "KeyO",
	KeyOper:               "KeyOper",
	KeyOut:                "KeyOut",
	KeyP:                  "KeyP",
	KeyPageDown:           "KeyPageDown",
	KeyPageUp:             "KeyPageUp",
	KeyPaste:              "KeyPaste",
	KeyPause:              "KeyPause",
	KeyPeriod:             "KeyPeriod",
	KeyPower:              "KeyPower",
	KeyPrintScreen:        "KeyPrintScreen",
	KeyPrior:              "KeyPrior",
	KeyQ:                  "KeyQ",
	KeyR:                  "KeyR",
	KeyRightAlt:           "KeyRightAlt",
	KeyRightCtrl:          "KeyRightCtrl",
	KeyReturn:             "KeyReturn",
	KeyReturn2:            "KeyReturn2",
	KeyRGUI:               "KeyRGUI",
	KeyRight:              "KeyRight",
	KeyRightBracket:       "KeyRightBracket",
	KeyRightShift:         "KeyRightShift",
	KeyS:                  "KeyS",
	KeyScrollLock:         "KeyScrollLock",
	KeySelect:             "KeySelect",
	KeySemicolon:          "KeySemicolon",
	KeySeparator:          "KeySeparator",
	KeySlash:              "KeySlash",
	KeySleep:              "KeySleep",
	KeySpace:              "KeySpace",
	KeyStop:               "KeyStop",
	KeySysReq:             "KeySysReq",
	KeyT:                  "KeyT",
	KeyTab:                "KeyTab",
	KeyThousandsSeparator: "KeyThousandsSeparator",
	KeyU:          "KeyU",
	KeyUndo:       "KeyUndo",
	KeyUnknown:    "KeyUnknown",
	KeyUp:         "KeyUp",
	KeyV:          "KeyV",
	KeyVolumeDown: "KeyVolumeDown",
	KeyVolumeUp:   "KeyVolumeUp",
	KeyW:          "KeyW",
	KeyWWW:        "KeyWWW",
	KeyX:          "KeyX",
	KeyY:          "KeyY",
	KeyZ:          "KeyZ",
	KeyAmpersand:  "KeyAmpersand",
	KeyAsterisk:   "KeyAsterisk",
	KeyAt:         "KeyAt",
	KeyCaret:      "KeyCaret",
	KeyColon:      "KeyColon",
	KeyDollar:     "KeyDollar",
	KeyExclaim:    "KeyExclaim",
	KeyGreater:    "KeyGreater",
	KeyHash:       "KeyHash",
	KeyLeftParen:  "KeyLeftParen",
	KeyLess:       "KeyLess",
	KeyPercent:    "KeyPercent",
	KeyPlus:       "KeyPlus",
	KeyQuestion:   "KeyQuestion",
	KeyQuote:      "KeyQuote",
	KeyRightParen: "KeyRightParen",
	KeyUnderscore: "KeyUnderscore",
}

func (k Key) String() string {
	s, ok := keyStrings[k]
	if !ok {
		return strconv.Itoa(int(k))
	}
	return s
}
