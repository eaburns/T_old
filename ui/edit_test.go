// Copyright © 2016, The T Authors.

package ui

import (
	"bytes"
	"image"
	"io/ioutil"
	"reflect"
	"testing"
	"unicode"

	"github.com/eaburns/T/edit"
	"github.com/eaburns/T/edit/edittest"
	"github.com/eaburns/T/editor"
	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/mouse"
)

func TestMouseHandler(t *testing.T) {
	tests := []struct {
		name string
		// Given is the initial state of a sheet body editor.
		// It is in the format of edittest.ParseState,
		// but only the . mark is considered.
		given string
		// Want is the desired final state of the sheet body editor
		// after handling all events.
		// It is in the format of edittest.ParseState,
		// but only the . mark is considered.
		want string

		// Events is a slice of mouse events under test.
		// Event locations correspond to locations in the text as follows:
		// The Y location is the line number
		// and the X location is the positive rune offset
		// from the beginning of the line.
		events []mouse.Event

		// Cmds are commands expected to be executed.
		cmds []string

		// If Skip is true the test is not run.
		Skip bool
	}{
		{
			name:   "click empty text",
			given:  "{..}",
			events: leftClick(image.Pt(0, 0)),
			want:   "{..}",
		},
		{
			name:   "click before text",
			given:  "abc{..}",
			events: leftClick(image.Pt(-1, -1)),
			want:   "{..}abc",
		},
		{
			name:   "click after text",
			given:  "{..}abc",
			events: leftClick(image.Pt(100, 100)),
			want:   "abc{..}",
		},
		{
			name:   "click ^",
			given:  "abc{..}",
			events: leftClick(image.Pt(0, 1)),
			want:   "{..}abc",
		},
		{
			name:   "click $",
			given:  "{..}abc",
			events: leftClick(image.Pt(3, 1)),
			want:   "abc{..}",
		},
		{
			name:   "click midline",
			given:  "{..}abc",
			events: leftClick(image.Pt(1, 1)),
			want:   "a{..}bc",
		},
		{
			name:   "click multiple lines",
			given:  "{..}abc\ndef\nghi",
			events: leftClick(image.Pt(1, 2)),
			want:   "abc\nd{..}ef\nghi",
		},

		{
			name:   "2-click",
			given:  "{..}abc\nenv\nxyz",
			events: middleClick(image.Pt(1, 2)),
			want:   "abc\ne{..}nv\nxyz",
			cmds:   []string{"env"},
		},
	}

	for _, test := range tests {
		if test.Skip {
			continue
		}

		buf := edit.NewBuffer()
		defer buf.Close()

		initText, initMarks := edittest.ParseState(test.given)
		if err := edit.Change(edit.All, initText).Do(buf, ioutil.Discard); err != nil {
			t.Fatalf("%s failed to init buffer text: %v", test.name, err)
		}
		for m, at := range initMarks {
			if err := buf.SetMark(m, edit.Span(at)); err != nil {
				t.Fatalf("%s failed to init mark %c to %v: %v", test.name, m, at, err)
			}
		}

		h := newTestHandler(buf)
		for _, e := range test.events {
			handleMouse(h, e)
		}

		// Read the buffer directly so as to not disturb the . mark.
		d, err := ioutil.ReadAll(buf.Reader(edit.Span{0: 0, 1: buf.Size()}))
		if err != nil {
			t.Fatalf("%s failed to read buffer: %v", test.name, err)
		}
		gotText := string(d)
		gotMarks := map[rune][2]int64{'.': buf.Mark('.')}

		if !edittest.StateEquals(gotText, gotMarks, test.want) {
			got := edittest.StateString(gotText, gotMarks)
			t.Errorf("%s, got %q want %q", test.name, got, test.want)
		}

		if !reflect.DeepEqual(h.cmds, test.cmds) {
			t.Errorf("%s, executed %v, want %v", test.name, h.cmds, test.cmds)
		}
	}
}

func TestKeyHandler(t *testing.T) {
	tests := []struct {
		name string
		// Given is the initial state of a sheet body editor.
		// It is in the format of edittest.ParseState,
		// but only the . mark is considered.
		given string
		// Want is the desired final state of the sheet body editor
		// after handling all events.
		// It is in the format of edittest.ParseState,
		// but only the . mark is considered.
		want   string
		events []key.Event

		// If Skip is true the test is not run.
		Skip bool
	}{
		{
			name:   "type one rune",
			given:  "{..}",
			events: typeRunes("α"),
			want:   "α{..}",
		},
		{
			name:   "typing several runes",
			given:  "{..}",
			events: typeRunes("Hello, 世界!"),
			want:   "Hello, 世界!{..}",
		},
		{
			name:   "insert runes",
			given:  "Hello{..}世界!",
			events: typeRunes(", "),
			want:   "Hello, {..}世界!",
		},
		{
			name:   "append runes",
			given:  "Hello, {..}",
			events: typeRunes("世界!"),
			want:   "Hello, 世界!{..}",
		},
		{
			name:   "change runes",
			given:  "Hello, {.}World{.}!",
			events: typeRunes("世界"),
			want:   "Hello, 世界{..}!",
		},
		{
			name:  "key repeat",
			given: "{..}",
			events: []key.Event{
				key.Event{Rune: 'a', Direction: key.DirPress},
				key.Event{Rune: 'a'},
				key.Event{Rune: 'a'},
				key.Event{Rune: 'a', Direction: key.DirRelease},
			},
			want: "aaa{..}",
		},

		{
			name:   "enter from empty file",
			given:  "{..}",
			events: keyPress(key.CodeReturnEnter),
			want:   "\n{..}",
		},
		{
			name:   "enter from mid-line",
			given:  "a{..}b",
			events: keyPress(key.CodeReturnEnter),
			want:   "a\n{..}b",
		},
		{
			name:   "enter replace runes",
			given:  "{.}abc{.}",
			events: keyPress(key.CodeReturnEnter),
			want:   "\n{..}",
		},

		{
			name:   "tab from empty file",
			given:  "{..}",
			events: keyPress(key.CodeTab),
			want:   "\t{..}",
		},
		{
			name:   "tab from mid-line",
			given:  "a{..}b",
			events: keyPress(key.CodeTab),
			want:   "a\t{..}b",
		},
		{
			name:   "tab replace runes",
			given:  "{.}abc{.}",
			events: keyPress(key.CodeTab),
			want:   "\t{..}",
		},

		{
			name:   "left from BOF",
			given:  "{..}Hello, World",
			events: keyPress(key.CodeLeftArrow),
			want:   "{..}Hello, World",
		},
		{
			name:   "left from EOF",
			given:  "Hello, World{..}",
			events: keyPress(key.CodeLeftArrow),
			want:   "Hello, Worl{..}d",
		},
		{
			name:   "left from mid-line",
			given:  "H{..}ello, World",
			events: keyPress(key.CodeLeftArrow),
			want:   "{..}Hello, World",
		},
		{
			name:   "left from selection",
			given:  "abc{.}def{.}ghi",
			events: keyPress(key.CodeLeftArrow),
			want:   "ab{..}cdefghi",
		},
		{
			name:   "left to previous line",
			given:  "Hello,\n{..}World",
			events: keyPress(key.CodeLeftArrow),
			want:   "Hello,{..}\nWorld",
		},
		{
			name:  "hold left",
			given: "aaaaaa{..}",
			events: []key.Event{
				{Rune: -1, Code: key.CodeLeftArrow, Direction: key.DirPress},
				{Rune: -1, Code: key.CodeLeftArrow},
				{Rune: -1, Code: key.CodeLeftArrow},
				{Rune: -1, Code: key.CodeLeftArrow, Direction: key.DirRelease},
			},
			want: "aaa{..}aaa",
		},

		{
			name:   "right from BOF",
			given:  "{..}Hello, World",
			events: keyPress(key.CodeRightArrow),
			want:   "H{..}ello, World",
		},
		{
			name:   "right from EOF",
			given:  "Hello, World{..}",
			events: keyPress(key.CodeRightArrow),
			want:   "Hello, World{..}",
		},
		{
			name:   "right from mid-line",
			given:  "H{..}ello, World",
			events: keyPress(key.CodeRightArrow),
			want:   "He{..}llo, World",
		},
		{
			name:   "right from selection",
			given:  "abc{.}def{.}ghi",
			events: keyPress(key.CodeRightArrow),
			want:   "abcdefg{..}hi",
		},
		{
			name:   "right to next line",
			given:  "Hello,{..}\nWorld",
			events: keyPress(key.CodeRightArrow),
			want:   "Hello,\n{..}World",
		},
		{
			name:  "hold right",
			given: "{..}aaaaaa",
			events: []key.Event{
				{Rune: -1, Code: key.CodeRightArrow, Direction: key.DirPress},
				{Rune: -1, Code: key.CodeRightArrow},
				{Rune: -1, Code: key.CodeRightArrow},
				{Rune: -1, Code: key.CodeRightArrow, Direction: key.DirRelease},
			},
			want: "aaa{..}aaa",
		},

		{
			name:   "up from BOF",
			given:  "{..}1234567890",
			events: keyPress(key.CodeUpArrow),
			want:   "{..}1234567890",
		},
		{
			name:   "up empty lines",
			given:  "\n\n\n{..}",
			events: append(keyPress(key.CodeUpArrow), keyPress(key.CodeUpArrow)...),
			want:   "\n{..}\n\n",
		},
		{
			name:   "up from ^ to empty line",
			given:  "\n{..}1234567890",
			events: keyPress(key.CodeUpArrow),
			want:   "{..}\n1234567890",
		},
		{
			name:   "up from ^ to non-empty line",
			given:  "1234567890\n{..}1234567890",
			events: keyPress(key.CodeUpArrow),
			want:   "{..}1234567890\n1234567890",
		},
		{
			name:   "up to same-length line",
			given:  "1234567890\n12345{..}67890",
			events: keyPress(key.CodeUpArrow),
			want:   "12345{..}67890\n1234567890",
		},
		{
			name:   "up to shorter line",
			given:  "1234\n12345{..}67890",
			events: keyPress(key.CodeUpArrow),
			want:   "1234{..}\n1234567890",
		},
		{
			name:   "up to longer line",
			given:  "1234567890\n12345{..}",
			events: keyPress(key.CodeUpArrow),
			want:   "12345{..}67890\n12345",
		},
		{
			name:   "up from partial-line selection",
			given:  "123\n123\n1{.}2{.}3\n123\n",
			events: keyPress(key.CodeUpArrow),
			want:   "123\n1{..}23\n123\n123\n",
		},
		{
			name:   "up from full-line selection",
			given:  "123\n123\n{.}123\n{.}123\n",
			events: keyPress(key.CodeUpArrow),
			want:   "123\n{..}123\n123\n123\n",
		},
		{
			name:  "up remember desired column",
			given: "1234567890\n\n1234567890\n1234\n\n12345{..}67890\n",
			events: []key.Event{
				{Rune: -1, Code: key.CodeUpArrow, Direction: key.DirPress},
				{Rune: -1, Code: key.CodeUpArrow, Direction: key.DirRelease},
				{Rune: -1, Code: key.CodeUpArrow, Direction: key.DirPress},
				{Rune: -1, Code: key.CodeUpArrow, Direction: key.DirRelease},
				{Rune: -1, Code: key.CodeUpArrow, Direction: key.DirPress},
				{Rune: -1, Code: key.CodeUpArrow, Direction: key.DirRelease},
				{Rune: -1, Code: key.CodeUpArrow, Direction: key.DirPress},
				{Rune: -1, Code: key.CodeUpArrow, Direction: key.DirRelease},
				{Rune: -1, Code: key.CodeUpArrow, Direction: key.DirPress},
				{Rune: -1, Code: key.CodeUpArrow, Direction: key.DirRelease},
			},
			want: "12345{..}67890\n\n1234567890\n1234\n\n1234567890\n",
		},
		{
			name:  "up repeat",
			given: "123\n123\n123\n123\n12{..}3\n",
			events: []key.Event{
				{Rune: -1, Code: key.CodeUpArrow, Direction: key.DirPress},
				{Rune: -1, Code: key.CodeUpArrow},
				{Rune: -1, Code: key.CodeUpArrow},
				{Rune: -1, Code: key.CodeUpArrow, Direction: key.DirRelease},
			},
			want: "123\n12{..}3\n123\n123\n123\n",
		},

		{
			name:   "down EOF",
			given:  "1234567890{..}",
			events: keyPress(key.CodeDownArrow),
			want:   "1234567890{..}",
		},
		{
			name:   "down empty lines",
			given:  "{..}\n\n\n",
			events: append(keyPress(key.CodeDownArrow), keyPress(key.CodeDownArrow)...),
			want:   "\n\n{..}\n",
		},
		{
			name:   "down from $ to empty line",
			given:  "1234567890{..}\n",
			events: keyPress(key.CodeDownArrow),
			want:   "1234567890\n{..}",
		},
		{
			name:   "down from $ to non-empty line",
			given:  "1234567890{..}\n1234567890",
			events: keyPress(key.CodeDownArrow),
			want:   "1234567890\n1234567890{..}",
		},
		{
			name:   "down to same-length line",
			given:  "12345{..}67890\n1234567890",
			events: keyPress(key.CodeDownArrow),
			want:   "1234567890\n12345{..}67890",
		},
		{
			name:   "down to shorter line",
			given:  "12345{..}67890\n1234",
			events: keyPress(key.CodeDownArrow),
			want:   "1234567890\n1234{..}",
		},
		{
			name:   "down to longer line",
			given:  "12345{..}\n1234567890",
			events: keyPress(key.CodeDownArrow),
			want:   "12345\n12345{..}67890",
		},
		{
			name:   "down from partial-line selection",
			given:  "123\n1{.}2{.}3\n123\n123\n",
			events: keyPress(key.CodeDownArrow),
			want:   "123\n123\n1{..}23\n123\n",
		},
		{
			name:   "down from full-line selection",
			given:  "123\n{.}123\n{.}123\n123\n",
			events: keyPress(key.CodeDownArrow),
			want:   "123\n123\n{..}123\n123\n",
		},
		{
			name:  "down remember desired column",
			given: "12345{..}67890\n\n1234567890\n1234\n\n1234567890\n",
			events: []key.Event{
				{Rune: -1, Code: key.CodeDownArrow, Direction: key.DirPress},
				{Rune: -1, Code: key.CodeDownArrow, Direction: key.DirRelease},
				{Rune: -1, Code: key.CodeDownArrow, Direction: key.DirPress},
				{Rune: -1, Code: key.CodeDownArrow, Direction: key.DirRelease},
				{Rune: -1, Code: key.CodeDownArrow, Direction: key.DirPress},
				{Rune: -1, Code: key.CodeDownArrow, Direction: key.DirRelease},
				{Rune: -1, Code: key.CodeDownArrow, Direction: key.DirPress},
				{Rune: -1, Code: key.CodeDownArrow, Direction: key.DirRelease},
				{Rune: -1, Code: key.CodeDownArrow, Direction: key.DirPress},
				{Rune: -1, Code: key.CodeDownArrow, Direction: key.DirRelease},
			},
			want: "1234567890\n\n1234567890\n1234\n\n12345{..}67890\n",
		},
		{
			name:  "down repeat",
			given: "12{..}3\n123\n123\n123\n123\n",
			events: []key.Event{
				{Rune: -1, Code: key.CodeDownArrow, Direction: key.DirPress},
				{Rune: -1, Code: key.CodeDownArrow},
				{Rune: -1, Code: key.CodeDownArrow},
				{Rune: -1, Code: key.CodeDownArrow, Direction: key.DirRelease},
			},
			want: "123\n123\n123\n12{..}3\n123\n",
		},

		{
			name:   "^a from BOF",
			given:  "{..}abc",
			events: keyCtrlPress('a'),
			want:   "{..}abc",
		},
		{
			name:   "^a from empty line",
			given:  "abc\n{..}\nabc",
			events: keyCtrlPress('a'),
			want:   "abc\n{..}\nabc",
		},
		{
			name:   "^a from ^",
			given:  "abc\n{..}abc\nabc",
			events: keyCtrlPress('a'),
			want:   "abc\n{..}abc\nabc",
		},
		{
			name:   "^a from mid-line",
			given:  "abc\nab{..}c\nabc",
			events: keyCtrlPress('a'),
			want:   "abc\n{..}abc\nabc",
		},
		{
			name:   "^a from $",
			given:  "abc\nabc{..}\nabc",
			events: keyCtrlPress('a'),
			want:   "abc\n{..}abc\nabc",
		},
		{
			name:   "^a from partial-line selection",
			given:  "123\n1{.}2{.}3\n123\n",
			events: keyCtrlPress('a'),
			want:   "123\n{..}123\n123\n",
		},
		{
			name:   "^a from full-line selection",
			given:  "123\n{.}123\n{.}123\n",
			events: keyCtrlPress('a'),
			want:   "123\n{..}123\n123\n",
		},

		{
			name:   "^e from EOF",
			given:  "abc{..}",
			events: keyCtrlPress('e'),
			want:   "abc{..}",
		},
		{
			name:   "^e from empty line",
			given:  "abc\n{..}\nabc",
			events: keyCtrlPress('e'),
			want:   "abc\n{..}\nabc",
		},
		{
			name:   "^e from ^",
			given:  "abc\n{..}abc\nabc",
			events: keyCtrlPress('e'),
			want:   "abc\nabc{..}\nabc",
		},
		{
			name:   "^e from mid-line",
			given:  "abc\na{..}bc\nabc",
			events: keyCtrlPress('e'),
			want:   "abc\nabc{..}\nabc",
		},
		{
			name:   "^e from $",
			given:  "abc\nabc{..}\nabc",
			events: keyCtrlPress('e'),
			want:   "abc\nabc{..}\nabc",
		},
		{
			name:   "^e from partial-line selection",
			given:  "123\n1{.}2{.}3\n123\n",
			events: keyCtrlPress('e'),
			want:   "123\n123{..}\n123\n",
		},
		{
			name:   "^e from full-line selection",
			given:  "123\n{.}123\n{.}123\n",
			events: keyCtrlPress('e'),
			want:   "123\n123{..}\n123\n",
		},

		{
			name:   "backspace from BOF",
			given:  "{..}",
			events: keyPress(key.CodeDeleteBackspace),
			want:   "{..}",
		},
		{
			name:   "backspace from mid-line",
			given:  "abc{..}def",
			events: keyPress(key.CodeDeleteBackspace),
			want:   "ab{..}def",
		},
		{
			name:   "backspace delete selection",
			given:  "abc{.}def{.}ghi",
			events: keyPress(key.CodeDeleteBackspace),
			want:   "ab{..}ghi",
		},

		{
			name:   "^h from BOF",
			given:  "{..}",
			events: keyCtrlPress('h'),
			want:   "{..}",
		},
		{
			name:   "^h from mid-line",
			given:  "abc{..}def",
			events: keyCtrlPress('h'),
			want:   "ab{..}def",
		},
		{
			name:   "^h delete selection",
			given:  "abc{.}def{.}ghi",
			events: keyCtrlPress('h'),
			want:   "ab{..}ghi",
		},

		{
			name:   "^u from BOF",
			given:  "{..}abc",
			events: keyCtrlPress('u'),
			want:   "{..}abc",
		},
		{
			name:   "^u from empty line",
			given:  "abc\n{..}\nabc",
			events: keyCtrlPress('u'),
			want:   "abc\n{..}\nabc",
		},
		{
			name:   "^u from ^",
			given:  "abc\n{..}abc",
			events: keyCtrlPress('u'),
			want:   "abc\n{..}abc",
		},
		{
			name:   "^u from mid-line",
			given:  "abc\nab{..}c",
			events: keyCtrlPress('u'),
			want:   "abc\n{..}c",
		},
		{
			name:   "^u from $",
			given:  "abc\nabc{..}",
			events: keyCtrlPress('u'),
			want:   "abc\n{..}",
		},
		{
			name:   "^u from partial-line selection",
			given:  "abc\na{.}b{.}c",
			events: keyCtrlPress('u'),
			want:   "abc\n{..}c",
		},
		{
			name:   "^u from full-line selection",
			given:  "abc\n{.}abc\n{.}",
			events: keyCtrlPress('u'),
			want:   "abc\n{..}",
		},

		{
			name:   "^w from BOF",
			given:  "{..}abc",
			events: keyCtrlPress('w'),
			want:   "{..}abc",
		},
		{
			name:   "^w from empty line",
			given:  "abc\n{..}\nabc",
			events: keyCtrlPress('w'),
			want:   "{..}\nabc",
		},
		{
			name:   "^w from ^",
			given:  "abc\n{..}abc",
			events: keyCtrlPress('w'),
			want:   "{..}abc",
		},
		{
			name:   "^w from first line word",
			given:  "abc\nabc{..}",
			events: keyCtrlPress('w'),
			want:   "abc\n{..}",
		},
		{
			name:   "^w from first line space",
			given:  "abc\n  \t{..}",
			events: keyCtrlPress('w'),
			want:   "{..}",
		},
		{
			name:   "^w from first line word then space",
			given:  "abc\nabc  \t{..}",
			events: keyCtrlPress('w'),
			want:   "abc\n{..}",
		},
		{
			name:   "^w from second line word",
			given:  "abc\nabc xyz  \t{..}",
			events: keyCtrlPress('w'),
			want:   "abc\nabc {..}",
		},
	}

	for _, test := range tests {
		if test.Skip {
			continue
		}

		buf := edit.NewBuffer()
		defer buf.Close()

		initText, initMarks := edittest.ParseState(test.given)
		if err := edit.Change(edit.All, initText).Do(buf, ioutil.Discard); err != nil {
			t.Fatalf("%s failed to init buffer text: %v", test.name, err)
		}
		for m, at := range initMarks {
			if err := buf.SetMark(m, edit.Span(at)); err != nil {
				t.Fatalf("%s failed to init mark %c to %v: %v", test.name, m, at, err)
			}
		}

		h := newTestHandler(buf)
		for _, e := range test.events {
			handleKey(h, e)
		}

		// Read the buffer directly so as to not disturb the . mark.
		d, err := ioutil.ReadAll(buf.Reader(edit.Span{0: 0, 1: buf.Size()}))
		if err != nil {
			t.Fatalf("%s failed to read buffer: %v", test.name, err)
		}
		gotText := string(d)
		gotMarks := map[rune][2]int64{'.': buf.Mark('.')}

		if !edittest.StateEquals(gotText, gotMarks, test.want) {
			got := edittest.StateString(gotText, gotMarks)
			t.Errorf("%s, got %q want %q", test.name, got, test.want)
		}

		if h.cmds != nil {
			t.Errorf("%s, executed %v, want []", test.name, h.cmds)
		}
	}
}

func keyCtrlPress(r rune) []key.Event {
	return []key.Event{
		{Rune: -1, Code: key.CodeLeftControl, Direction: key.DirPress},
		{Rune: r, Modifiers: key.ModControl, Direction: key.DirPress},
		{Rune: r, Modifiers: key.ModControl, Direction: key.DirRelease},
		{Rune: -1, Code: key.CodeLeftControl, Direction: key.DirRelease},
	}
}

func keyPress(code key.Code) []key.Event {
	return []key.Event{
		{Rune: -1, Code: code, Direction: key.DirPress},
		{Rune: -1, Code: code, Direction: key.DirRelease},
	}
}

func typeRunes(str string) []key.Event {
	var events []key.Event
	for _, r := range str {
		var mods key.Modifiers
		if !unicode.IsLower(r) {
			mods = key.ModShift
			events = append(events, key.Event{
				Rune:      -1,
				Code:      key.CodeLeftShift,
				Direction: key.DirPress,
			})
		}

		// We ignore Code here, but so does typed rune handling.
		events = append(events,
			key.Event{Rune: r, Direction: key.DirPress, Modifiers: mods},
			key.Event{Rune: r, Direction: key.DirRelease, Modifiers: mods},
		)

		if !unicode.IsLower(r) {
			events = append(events, key.Event{
				Rune:      -1,
				Code:      key.CodeLeftShift,
				Direction: key.DirRelease,
			})
		}
	}
	return events
}

func leftClick(p image.Point) []mouse.Event {
	x, y := float32(p.X), float32(p.Y)
	return []mouse.Event{
		{X: x, Y: y, Button: mouse.ButtonLeft, Direction: mouse.DirPress},
		{X: x, Y: y, Button: mouse.ButtonLeft, Direction: mouse.DirRelease},
	}
}

func middleClick(p image.Point) []mouse.Event {
	x, y := float32(p.X), float32(p.Y)
	return []mouse.Event{
		{X: x, Y: y, Button: mouse.ButtonMiddle, Direction: mouse.DirPress},
		{X: x, Y: y, Button: mouse.ButtonMiddle, Direction: mouse.DirRelease},
	}
}

type testHandler struct {
	buf  *edit.Buffer
	col  int
	seq  int
	cmds []string
}

func newTestHandler(buf *edit.Buffer) *testHandler {
	return &testHandler{
		buf: buf,
		col: -1,
	}
}

func (h *testHandler) exec(cmd string) { h.cmds = append(h.cmds, cmd) }

func (h *testHandler) column() int { return h.col }

func (h *testHandler) setColumn(c int) { h.col = c }

func (h *testHandler) where(p image.Point) int64 {
	line := edit.Clamp(edit.Line(p.Y)).Minus(edit.Rune(0))
	addr := line.Plus(edit.Clamp(edit.Rune(int64(p.X))))
	s, err := addr.Where(h.buf)
	if err != nil {
		panic(err)
	}
	if s[0] != s[1] {
		panic("range address")
	}
	return s[0]
}

func (h *testHandler) do(res chan<- []editor.EditResult, eds ...edit.Edit) {
	h.col = -1
	print := bytes.NewBuffer(nil)
	var results []editor.EditResult
	for _, e := range eds {
		print.Reset()
		err := e.Do(h.buf, print)
		r := editor.EditResult{
			Sequence: h.seq,
			Print:    print.String(),
		}
		if err != nil {
			r.Error = err.Error()
		}
		results = append(results, r)
		h.seq++
	}
	if res != nil {
		go func() { res <- results }()
	}
}
