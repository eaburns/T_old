// Copyright © 2016, The T Authors.

package ui

import (
	"image"
	"path"
	"testing"
	"time"

	"github.com/eaburns/T/edit"
	"github.com/eaburns/T/editor"
	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/mouse"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/event/size"
)

// TestWindowDead tests the window being closed by the window manager.
func TestWindowDead(t *testing.T) {
	s, w := makeTestUI()
	defer s.close()
	done := make(chan struct{})
	s.uiServer.SetDoneHandler(func() { close(done) })
	w.Send(lifecycle.Event{To: lifecycle.StageDead})
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	select {
	case <-done:
	case <-tick.C:
		t.Errorf("timed out waiting for done")
	}
}

func TestDraw(t *testing.T) {
	s, w := makeTestUI()
	defer s.close()
	w.Send(paint.Event{})
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	select {
	case <-w.Window.(*stubWindow).publish:
	case <-tick.C:
		t.Errorf("timed out waiting for publish")
	}
}

func TestResize(t *testing.T) {
	s, w := makeTestUI()
	defer s.close()
	done := make(chan struct{})
	s.uiServer.SetDoneHandler(func() { close(done) })
	const width, height = 100, 50
	w.Send(size.Event{WidthPx: width, HeightPx: height})
	wait(w)
	if sz := w.bounds().Size(); sz.X != width || sz.Y != height {
		t.Errorf("sz=%v, want {%d,%d}", sz, width, height)
	}
	for i, c := range w.columns {
		if !c.bounds().In(w.bounds()) {
			t.Errorf("columns[%d].bounds()=%v, not in window %v",
				i, c.bounds(), w.bounds())
		}
		for j, f := range c.frames {
			if !f.bounds().In(c.bounds()) {
				t.Errorf("columns[%d].frames[%d].bounds()=%v, not in column %v",
					i, j, f.bounds(), c.bounds())
			}
		}
	}
}

func TestFocus(t *testing.T) {
	s, w := makeTestUI()
	defer s.close()
	for c := 0; c < 3; c++ {
		for f := 0; f < 3; f++ {
			fr := w.columns[c].frames[f]
			if fr.bounds().Empty() {
				// We can't focus on a totally-hidden frame, so don't try.
				continue
			}
			mouseTo(w, center(fr))
			wait(w)
			if w.inFocus != fr.(handler) {
				t.Errorf("focus=%v != w.columns[%d].frames[%d]=%v", w.inFocus, c, f, fr)
			}
		}
	}
}

// TestDeleteFrame tests shift+2click to delete a sheet or column.
func TestDeleteFrame(t *testing.T) {
	s, w := makeTestUI()
	defer s.close()

	column0 := w.columns[0].frames[0]
	sheet0 := w.columns[0].frames[1].(*sheet)
	sheet1 := w.columns[0].frames[2].(*sheet)

	// Read the center points before we delete anything.
	// Otherwise there is a race between the deleting (which is async),
	// and reading the bounds.
	column0pt := center(column0)
	sheet0pt := center(sheet0)
	sheet1pt := center(sheet1)

	// Delete sheet1
	mouseTo(w, sheet1pt)
	shiftClick(w, sheet1pt, mouse.ButtonMiddle)
	wait(w)
	// Even after waiting, sheets are deleted from the window asynchronously.
	// But they are deleted from the server right away, so we check that.
	if _, ok := s.uiServer.sheets[sheet1.id]; ok {
		t.Errorf("sheet1 not deleted")
	}

	// Delete sheet0
	mouseTo(w, sheet0pt)
	shiftClick(w, sheet0pt, mouse.ButtonMiddle)
	wait(w)
	if _, ok := s.uiServer.sheets[sheet0.id]; ok {
		t.Errorf("sheet0 not deleted")
	}

	// Delete column0
	mouseTo(w, column0pt)
	shiftClick(w, column0pt, mouse.ButtonMiddle)
	wait(w)
	if len(w.columns) != 2 {
		t.Errorf("len(w.columns)=%d, want 2", len(w.columns))
	}
}

// TestDelete_LastColumn tests that the last column in a window cannot be deleted.
func TestDelete_LastColumn(t *testing.T) {
	s, w := makeTestUI()
	defer s.close()
	for _, c := range w.columns {
		// Cheat, because not all column tags may be visible to shich+2-click delete.
		w.Send(func() { w.deleteColumn(c) })
		wait(w)
	}
	if len(w.columns) != 1 {
		t.Errorf("len(w.columns)=%d, want 1", len(w.columns))
	}
}

// TestGrow tests shift+1click to grow a frame.
func TestGrow(t *testing.T) {
	s, w := makeTestUI()
	defer s.close()

	for c := 0; c < 3; c++ {
		for f := 0; f < 3; f++ {
			fr := w.columns[c].frames[f]
			if fr.bounds().Empty() {
				// We can't focus on a totally-hidden frame, so don't try.
				continue
			}

			mouseTo(w, center(fr))
			wait(w)
			if w.inFocus != fr.(handler) {
				t.Fatalf("focus=%v, want %v=columns[%d].frames[%d]", w.inFocus, fr, c, f)
			}

			origBounds := fr.bounds()
			shiftClick(w, center(fr), mouse.ButtonLeft)
			wait(w)
			if b := fr.bounds(); b == origBounds || !origBounds.In(b) {
				t.Fatalf("columns[%d].frames[%d].bounds()=%v, want bigger than %v\n", c, f, b, origBounds)
			}
		}
	}
}

// TestMove tests shift+1dragging to move a frame.
func TestMove(t *testing.T) {
	s, w := makeTestUI()
	defer s.close()

	sheet0 := w.columns[0].frames[1].(*sheet)
	sheet1 := w.columns[0].frames[2].(*sheet)

	// Move sheet0 down within its column.
	mouseTo(w, center(sheet0))
	shiftDrag(w, center(sheet0), center(sheet1), mouse.ButtonLeft)
	wait(w)
	if w.columns[0].frames[2] != sheet0 {
		t.Errorf("w.columns[0].frames[2] != sheet0")
	}

	sheet2 := w.columns[1].frames[1].(*sheet)

	// Move sheet0 into the next column.
	mouseTo(w, center(sheet0))
	shiftDrag(w, center(sheet0), center(sheet2), mouse.ButtonLeft)
	wait(w)
	if w.columns[1].frames[2] != sheet0 {
		t.Errorf("w.columns[1].frames[2] != sheet0")
	}

	column0 := w.columns[0]
	column0tag := w.columns[0].frames[0].(*columnTag)
	column1tag := w.columns[1].frames[0]

	// Move column0 right of column1
	mouseTo(w, center(column0tag))
	shiftDrag(w, center(column0tag), center(column1tag), mouse.ButtonLeft)
	wait(w)
	if w.columns[1] != column0 {
		t.Errorf("w.columns[1] != column0")
	}
}

// TestMove_DoesNotFit tests that a frame is returned to its original location
// if it cannot fit where it is dropped.
func TestMove_DoesNotFit(t *testing.T) {
	s, w := makeTestUI()
	defer s.close()

	// Shrink the window height such that each column can only fit two sheets.
	h := minFrameSize*2 + minFrameSize/2
	w.Send(size.Event{WidthPx: 800, HeightPx: h})

	// Wait for resize before using sheet dimensions.
	wait(w)

	sheet0 := w.columns[0].frames[1].(*sheet)
	sheet2 := w.columns[1].frames[1].(*sheet)

	// Try to move sheet0 into column 1.
	// However, each column can only hold 2 sheets,
	// and column 1 already has two sheets.
	mouseTo(w, center(sheet0))
	wait(w)
	if w.inFocus.(*sheet) != sheet0 {
		t.Errorf("inFocus=%v, want %v\n", w.inFocus, sheet0)
	}
	shiftDrag(w, center(sheet0), center(sheet2), mouse.ButtonLeft)
	wait(w)
	if w.columns[0].frames[1] != sheet0 {
		t.Errorf("w.columns[0].frames[1] != sheet0")
	}
	// 2 frames: the column tag and two sheets.
	if n := len(w.columns[1].frames); n != 3 {
		t.Errorf("len(w.columns[1].frames)=%d, want 3", n)
	}
}

// TestMoveSheetChordClick tests that pressing another mouse button
// while shift+1dragging a frame is treated as a release of button 1.
func TestMove_SheetChordClick(t *testing.T) {
	s, w := makeTestUI()
	defer s.close()

	sheet0 := w.columns[0].frames[1].(*sheet)
	sheet1 := w.columns[0].frames[2].(*sheet)
	sheet2 := w.columns[1].frames[1].(*sheet)

	// We shift+1drag sheet0 from points a → b → c.
	// However, upon arrival at point b, button 2 is pressed.
	// We expect sheet0 to end up at point b (in column 0),
	// not point c (in column 1).
	a, b, c := center(sheet0), center(sheet1), center(sheet2)
	mouseTo(w, a)
	w.Send(key.Event{
		Code:      key.CodeLeftShift,
		Direction: key.DirPress,
	})
	w.Send(mouse.Event{
		X:         float32(a.X),
		Y:         float32(a.Y),
		Button:    mouse.ButtonLeft,
		Direction: mouse.DirPress,
		Modifiers: key.ModShift,
	})
	w.Send(mouse.Event{
		X:         float32(b.X),
		Y:         float32(b.Y),
		Direction: mouse.DirNone,
		Modifiers: key.ModShift,
	})
	w.Send(mouse.Event{
		X:         float32(b.X),
		Y:         float32(b.Y),
		Direction: mouse.DirNone,
		Modifiers: key.ModShift,
	})
	// Before releasing the left button, the middle button is clicked.
	w.Send(mouse.Event{
		X:         float32(b.X),
		Y:         float32(b.X),
		Button:    mouse.ButtonMiddle,
		Direction: mouse.DirPress,
		Modifiers: key.ModShift,
	})
	w.Send(mouse.Event{
		X:         float32(b.X),
		Y:         float32(b.X),
		Button:    mouse.ButtonMiddle,
		Direction: mouse.DirRelease,
		Modifiers: key.ModShift,
	})
	// Continue to point c and release the left button.
	w.Send(mouse.Event{
		X:         float32(c.X),
		Y:         float32(c.Y),
		Direction: mouse.DirNone,
		Modifiers: key.ModShift,
	})
	w.Send(mouse.Event{
		X:         float32(c.X),
		Y:         float32(c.Y),
		Button:    mouse.ButtonLeft,
		Direction: mouse.DirRelease,
		Modifiers: key.ModShift,
	})
	w.Send(key.Event{
		Code:      key.CodeLeftShift,
		Direction: key.DirRelease,
	})
	wait(w)
	if w.columns[0].frames[2] != sheet0 {
		t.Errorf("w.columns[0].frames[2] != sheet0")
	}
}

// TestMoveColumnChordClick tests that pressing another mouse button
// while shift+1dragging a frame is treated as a release of button 1.
func TestMove_ColumnChordClick(t *testing.T) {
	s, w := makeTestUI()
	defer s.close()

	column0 := w.columns[0]
	column0tag := w.columns[0].frames[0].(*columnTag)
	column1tag := w.columns[1].frames[0]
	column2tag := w.columns[2].frames[0]

	// We shift+1drag column0 from points a → b → c.
	// However, upon arrival at point b, button 2 is pressed.
	// We expect sheet0 to end up at point b (in column 1),
	// not point c (in column 2).
	a, b, c := center(column0tag), center(column1tag), center(column2tag)
	mouseTo(w, a)
	w.Send(key.Event{
		Code:      key.CodeLeftShift,
		Direction: key.DirPress,
	})
	w.Send(mouse.Event{
		X:         float32(a.X),
		Y:         float32(a.Y),
		Button:    mouse.ButtonLeft,
		Direction: mouse.DirPress,
		Modifiers: key.ModShift,
	})
	w.Send(mouse.Event{
		X:         float32(b.X),
		Y:         float32(b.Y),
		Direction: mouse.DirNone,
		Modifiers: key.ModShift,
	})
	w.Send(mouse.Event{
		X:         float32(b.X),
		Y:         float32(b.Y),
		Direction: mouse.DirNone,
		Modifiers: key.ModShift,
	})
	// Before releasing the left button, the middle button is pressed.
	w.Send(mouse.Event{
		X:         float32(b.X),
		Y:         float32(b.X),
		Button:    mouse.ButtonMiddle,
		Direction: mouse.DirPress,
		Modifiers: key.ModShift,
	})
	w.Send(mouse.Event{
		X:         float32(b.X),
		Y:         float32(b.X),
		Button:    mouse.ButtonMiddle,
		Direction: mouse.DirRelease,
		Modifiers: key.ModShift,
	})
	// Continue to point c and release the left button.
	w.Send(mouse.Event{
		X:         float32(c.X),
		Y:         float32(c.Y),
		Direction: mouse.DirNone,
		Modifiers: key.ModShift,
	})
	w.Send(mouse.Event{
		X:         float32(c.X),
		Y:         float32(c.Y),
		Button:    mouse.ButtonLeft,
		Direction: mouse.DirRelease,
		Modifiers: key.ModShift,
	})
	w.Send(key.Event{
		Code:      key.CodeLeftShift,
		Direction: key.DirRelease,
	})
	wait(w)
	if w.columns[1] != column0 {
		t.Errorf("w.columns[1] != column0")
	}
}

// TestMoveSheetShiftRelease tests that releasing shift
// while shift+1dragging a frame,
// returns the frame to its original location.
func TestMove_SheetShiftRelease(t *testing.T) {
	s, w := makeTestUI()
	defer s.close()

	sheet0 := w.columns[0].frames[1].(*sheet)
	sheet1 := w.columns[0].frames[2].(*sheet)

	// We shift+1drag sheet0 from points a → b,
	// but shift is released before the 1 button.
	a, b := center(sheet0), center(sheet1)
	mouseTo(w, a)
	w.Send(key.Event{
		Code:      key.CodeLeftShift,
		Direction: key.DirPress,
	})
	w.Send(mouse.Event{
		X:         float32(a.X),
		Y:         float32(a.Y),
		Button:    mouse.ButtonLeft,
		Direction: mouse.DirPress,
		Modifiers: key.ModShift,
	})
	w.Send(mouse.Event{
		X:         float32(b.X),
		Y:         float32(b.Y),
		Direction: mouse.DirNone,
		Modifiers: key.ModShift,
	})
	w.Send(mouse.Event{
		X:         float32(b.X),
		Y:         float32(b.Y),
		Direction: mouse.DirNone,
		Modifiers: key.ModShift,
	})
	// Release shift before we finish dragging.
	w.Send(key.Event{
		Code:      key.CodeLeftShift,
		Direction: key.DirRelease,
	})
	w.Send(mouse.Event{
		X:         float32(b.X),
		Y:         float32(b.Y),
		Button:    mouse.ButtonLeft,
		Direction: mouse.DirRelease,
	})
	wait(w)
	if w.columns[0].frames[1] != sheet0 {
		t.Errorf("w.columns[0].frames[1] != sheet0")
	}
}

// TestMoveColumnShiftRelease tests that releasing shift
// while shift+1dragging a frame,
// returns the frame to its original location.
func TestMove_ColumnShiftRelease(t *testing.T) {
	s, w := makeTestUI()
	defer s.close()

	column1 := w.columns[1]
	column1tag := w.columns[1].frames[0].(*columnTag)
	column0tag := w.columns[0].frames[0]

	// We shift+1drag column1 from points a → b,
	// but shift is released before the 1 button.
	a, b := center(column1tag), center(column0tag)
	mouseTo(w, a)
	w.Send(key.Event{
		Code:      key.CodeLeftShift,
		Direction: key.DirPress,
	})
	w.Send(mouse.Event{
		X:         float32(a.X),
		Y:         float32(a.Y),
		Button:    mouse.ButtonLeft,
		Direction: mouse.DirPress,
		Modifiers: key.ModShift,
	})
	w.Send(mouse.Event{
		X:         float32(b.X),
		Y:         float32(b.Y),
		Direction: mouse.DirNone,
		Modifiers: key.ModShift,
	})
	w.Send(mouse.Event{
		X:         float32(b.X),
		Y:         float32(b.Y),
		Direction: mouse.DirNone,
		Modifiers: key.ModShift,
	})
	// Release shift before we finish dragging.
	w.Send(key.Event{
		Code:      key.CodeLeftShift,
		Direction: key.DirRelease,
	})
	w.Send(mouse.Event{
		X:         float32(b.X),
		Y:         float32(b.Y),
		Button:    mouse.ButtonLeft,
		Direction: mouse.DirRelease,
	})
	wait(w)
	if w.columns[1] != column1 {
		t.Errorf("w.columns[1] != column1")
	}
}

// TestMove_Closed tests that a frame can be closed,
// even when it is being moved.
func TestMove_Closed(t *testing.T) {
	s, w := makeTestUI()
	defer s.close()

	sheet0 := w.columns[0].frames[1].(*sheet)
	sheet1 := w.columns[0].frames[2].(*sheet)

	a, b := center(sheet0), center(sheet1)
	mouseTo(w, a)
	w.Send(key.Event{
		Code:      key.CodeLeftShift,
		Direction: key.DirPress,
	})
	w.Send(mouse.Event{
		X:         float32(a.X),
		Y:         float32(a.Y),
		Button:    mouse.ButtonLeft,
		Direction: mouse.DirPress,
		Modifiers: key.ModShift,
	})
	w.Send(mouse.Event{
		X:         float32(b.X),
		Y:         float32(b.Y),
		Direction: mouse.DirNone,
		Modifiers: key.ModShift,
	})
	w.Send(func() { w.deleteFrame(sheet0) })
	w.Send(mouse.Event{
		X:         float32(b.X),
		Y:         float32(b.Y),
		Button:    mouse.ButtonLeft,
		Direction: mouse.DirRelease,
		Modifiers: key.ModShift,
	})
	w.Send(key.Event{
		Code:      key.CodeLeftShift,
		Direction: key.DirRelease,
	})
	wait(w)
	if w.inFocus.(*sheet) == sheet0 {
		t.Errorf("w.inFocus=sheet0")
	}
	for i, c := range w.columns {
		for j, f := range c.frames {
			if f == sheet0 {
				t.Errorf("w.columns[%d].frames[%d]=sheet0", i, j)
			}
		}
	}
}

func TestSheetBodyText(t *testing.T) {
	s, w := makeTestUI()
	defer s.close()

	sheet0 := w.columns[0].frames[1].(*sheet)

	changesURL := *sheet0.body.bufferURL
	changesURL.Scheme = "ws"
	changesURL.Path = path.Join(sheet0.body.bufferURL.Path, "changes")
	changes, err := editor.Changes(&changesURL)
	if err != nil {
		t.Fatalf("editor.Changes(%q)=_,%v", changesURL, err)
	}
	defer changes.Close()

	mouseTo(w, center(sheet0))
	pressKey(w, -1, key.CodeTab)
	pressKey(w, 'a', key.CodeA)
	pressKey(w, -1, key.CodeDeleteBackspace)
	pressKey(w, 'b', key.CodeB)
	pressKey(w, -1, key.CodeReturnEnter)

	want := []struct {
		Span edit.Span
		Text string
	}{
		{Span: edit.Span{0, 0}, Text: "\t"},
		{Span: edit.Span{1, 1}, Text: "a"},
		{Span: edit.Span{1, 2}, Text: ""},
		{Span: edit.Span{1, 1}, Text: "b"},
		{Span: edit.Span{2, 2}, Text: "\n"},
	}

	for i, w := range want {
		cl, err := changes.Next()
		if err != nil || len(cl.Changes) != 1 || cl.Changes[0].Span != w.Span || string(cl.Changes[0].Text) != w.Text {
			t.Errorf("%d changes.Next()=%v,%v, want %v,nil", i, cl, err, w)
		}
	}
}

// MakeTestUI returns a Server that has:
// 	1 window
// 	3 column, at 0.0, 0.33, and 0.66, respectively.
//	6 sheets, 2 in each column
func makeTestUI() (*testServer, *window) {
	s := newServer(new(stubScreen))
	winListURL := urlWithPath(s.url, "/", "windows")
	win, err := NewWindow(winListURL, image.Pt(800, 600))
	if err != nil {
		panic(err)
	}
	editorURL := s.editorServer.PathURL("/")
	sheetsURL := urlWithPath(s.url, win.Path, "sheets")
	if _, err := NewSheet(sheetsURL, editorURL); err != nil {
		panic(err)
	}
	if _, err := NewSheet(sheetsURL, editorURL); err != nil {
		panic(err)
	}
	colsURL := urlWithPath(s.url, win.Path, "columns")
	if err := NewColumn(colsURL, 0.33); err != nil {
		panic(err)
	}
	if _, err := NewSheet(sheetsURL, editorURL); err != nil {
		panic(err)
	}
	if _, err := NewSheet(sheetsURL, editorURL); err != nil {
		panic(err)
	}
	if err := NewColumn(colsURL, 0.33); err != nil {
		panic(err)
	}
	if _, err := NewSheet(sheetsURL, editorURL); err != nil {
		panic(err)
	}
	if _, err := NewSheet(sheetsURL, editorURL); err != nil {
		panic(err)
	}
	w := s.uiServer.windows[win.ID]
	wait(w)
	return s, w
}

// Wait blocks until all previous events are handled by the window.
func wait(w *window) {
	done := make(chan struct{})
	w.Send(func() { close(done) })
	<-done
}

func shiftDrag(w *window, from, to image.Point, b mouse.Button) {
	w.Send(key.Event{
		Code:      key.CodeLeftShift,
		Direction: key.DirPress,
	})
	w.Send(mouse.Event{
		X:         float32(from.X),
		Y:         float32(from.Y),
		Button:    b,
		Direction: mouse.DirPress,
		Modifiers: key.ModShift,
	})
	w.Send(mouse.Event{
		X:         float32(to.X),
		Y:         float32(to.Y),
		Direction: mouse.DirNone,
		Modifiers: key.ModShift,
	})
	w.Send(mouse.Event{
		X:         float32(to.X),
		Y:         float32(to.Y),
		Direction: mouse.DirNone,
		Modifiers: key.ModShift,
	})
	w.Send(mouse.Event{
		X:         float32(to.X),
		Y:         float32(to.Y),
		Button:    b,
		Direction: mouse.DirRelease,
		Modifiers: key.ModShift,
	})
	w.Send(key.Event{
		Code:      key.CodeLeftShift,
		Direction: key.DirRelease,
	})
}

func shiftClick(w *window, p image.Point, b mouse.Button) {
	w.Send(key.Event{
		Code:      key.CodeLeftShift,
		Direction: key.DirPress,
	})
	w.Send(mouse.Event{
		X:         float32(p.X),
		Y:         float32(p.Y),
		Button:    b,
		Direction: mouse.DirPress,
		Modifiers: key.ModShift,
	})
	w.Send(mouse.Event{
		X:         float32(p.X),
		Y:         float32(p.Y),
		Button:    b,
		Direction: mouse.DirRelease,
		Modifiers: key.ModShift,
	})
	w.Send(key.Event{
		Code:      key.CodeLeftShift,
		Direction: key.DirRelease,
	})
}

func mouseTo(w *window, p image.Point) {
	w.Send(mouse.Event{X: float32(p.X), Y: float32(p.Y)})
}

func pressKey(w *window, r rune, c key.Code) {
	w.Send(key.Event{
		Rune:      r,
		Code:      c,
		Direction: key.DirPress,
	})
	w.Send(key.Event{
		Rune:      r,
		Code:      c,
		Direction: key.DirRelease,
	})
}

func center(f frame) image.Point {
	b := f.bounds()
	return image.Pt(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2)
}
