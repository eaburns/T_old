// Copyright © 2016, The T Authors.

// Package view provides a View type,
// which is an editor client
// that maintains a local, consistent copy
// of a segment of its buffer,
// and a set of marks.
//
// A View tracks text within line boundaries
// defined by a starting line
// and a number of following lines.
// The View text and marks can be read atomically,
// giving a consistent view of the tracked segment of the buffer.
// It can also be scrolled or warped to new starting line,
// and it can be resized to track a different number of lines.
//
// A typical user will:
// 	In one go routine:
// 	1. Receive from the Notify channel to wait for a change.
// 	2. Call View to read the updated text and marks.
// 	3. Goto 1.
//
// 	In another go routine:
// 	• Call Scroll, Resize, and Do as desired.
package view

// TODO(eaburns): more efficient support for change-style edits.
// Currently, the Do method refreshes the entire text and all marks.
// This is needed in case the user changes one of the marks.
// Add View.Change(a Address, to string), which cannot set marks,
// and doesn't need a complete refresh.

import (
	"fmt"
	"net/url"
	"path"
	"strings"
	"sync"

	"github.com/eaburns/T/edit"
	"github.com/eaburns/T/editor"
)

const (
	// ViewMark is the mark rune indicating the start
	// of the text tracked by a View.
	ViewMark = '0'

	// TmpMark is a mark used temporarily to save and restore dot.
	TmpMark = '1'
)

// A View is an editor client
// that maintains a local, consistent copy
// of a segment of its buffer,
// and a set of marks.
//
// All methods of a View are safe to call concurrently.
type View struct {
	// Notify is used to notify of changes to the View.
	// An empty struct is sent on Notify when the view changes.
	// Notify is closed when the View is closed.
	// Notify is single-buffered; if a send cannot proceed, it is dropped.
	Notify <-chan struct{}

	editorURL *url.URL
	textURL   *url.URL
	changes   *editor.ChangeStream
	do        chan<- viewDo

	seq int

	mu    sync.RWMutex
	n     int
	text  []byte
	marks []Mark
}

// A Mark is a mark tracked by a View.
type Mark struct {
	// Name is the mark rune.
	Name rune
	// Where is the address of the mark as rune offsets.
	Where [2]int64
}

type viewDo struct {
	edits  []edit.Edit
	result chan<- []editor.EditResult
}

// New returns a new View for a buffer.
// The new view tracks the empty string at line 0 and the given marks.
func New(bufferURL *url.URL, markRunes ...rune) (*View, error) {
	ed, err := editor.NewEditor(bufferURL)
	if err != nil {
		return nil, err
	}
	editorURL := *bufferURL
	editorURL.Path = ed.Path
	textURL := *bufferURL
	textURL.Path = path.Join(ed.Path, "text")

	changesURL := editorURL
	changesURL.Path = path.Join(bufferURL.Path, "changes")
	changesURL.Scheme = "ws"
	changes, err := editor.Changes(&changesURL)
	if err != nil {
		editor.Close(&editorURL)
		return nil, err
	}

	dedupedMarks := make(map[rune]bool)
	dedupedMarks[ViewMark] = true
	for _, r := range markRunes {
		dedupedMarks[r] = true
	}
	marks := make([]Mark, 0, len(markRunes))
	for r := range dedupedMarks {
		marks = append(marks, Mark{
			Name:  r,
			Where: [2]int64{-1, -1},
		})
	}

	// Notify has a single-element buffer.
	// The sender always does a non-blocking send.
	// If there is no receiver and the channel is empty,
	// the notification is sent.
	// If there is no receiver and the channel is full,
	// the notification is dropped;
	// the next receiver will get the one sitting in the channel.
	Notify := make(chan struct{}, 1)
	do := make(chan viewDo)

	v := &View{
		Notify:    Notify,
		editorURL: &editorURL,
		textURL:   &textURL,
		changes:   changes,
		do:        do,
		marks:     marks,
	}

	go v.run(do, Notify)

	v.do <- viewDo{}
	<-Notify

	return v, nil
}

// Close closes the view, and deletes its editor.
func (v *View) Close() error {
	close(v.do)
	err := v.changes.Close()
	editorErr := editor.Close(v.editorURL)
	if err == nil {
		err = editorErr
	}
	return err
}

// View calls the function with the current text and marks.
// The text and marks will not change until f returns.
func (v *View) View(f func(text []byte, marks []Mark)) {
	v.mu.RLock()
	f(v.text, v.marks)
	v.mu.RUnlock()
}

// Resize resizes the View to track the given number of lines,
// and returns whether the size actually changed.
func (v *View) Resize(nLines int) bool {
	if nLines < 0 {
		nLines = 0
	}
	v.mu.Lock()
	if v.n == nLines {
		v.mu.Unlock()
		return false
	}
	v.n = nLines
	v.mu.Unlock()
	v.do <- viewDo{}
	return true
}

// Scroll scrolls the View by the given delta.
func (v *View) Scroll(deltaLines int) {
	if deltaLines == 0 {
		return
	}
	var a edit.Address
	mark := edit.Mark(ViewMark)
	zero := edit.Clamp(edit.Rune(0))
	if deltaLines < 0 {
		lines := edit.Clamp(edit.Line(-deltaLines))
		a = mark.Minus(lines).Minus(zero)
	} else {
		lines := edit.Clamp(edit.Line(deltaLines))
		a = mark.Plus(lines).Plus(zero)
	}
	v.Warp(a)
}

// Warp moves the first line of  the view
// to the line containin the beginning of an Address.
func (v *View) Warp(addr edit.Address) { v.Do(nil, edit.Set(addr, ViewMark)) }

// Do performs edits using the View's editor
// and sends the result on the given channel.
// If the result channel is nil, the result is discarded.
func (v *View) Do(result chan<- []editor.EditResult, edits ...edit.Edit) {
	vd := viewDo{edits: edits, result: result}
	v.do <- vd
}

func (v *View) run(do <-chan viewDo, Notify chan<- struct{}) {
	changes := make(chan editor.ChangeList)
	go func(changes chan<- editor.ChangeList) {
		defer close(changes)
		for {
			cl, err := v.changes.Next()
			if err != nil {
				// TODO(eaburns): return error on Close.
				return
			}
			changes <- cl
		}
	}(changes)

	defer func() {
		close(Notify)
		// Flush any remaining channels on return.
		for range do {
		}
		for range changes {
		}
	}()

	for {
		select {
		case vd, ok := <-do:
			if !ok {
				return
			}
			if err := v.edit(vd, Notify); err != nil {
				// TODO(eaburns): return error on Close.
				return
			}
		case cl, ok := <-changes:
			if !ok {
				return
			}
			if v.seq >= cl.Sequence {
				break
			}
			// TODO(eaburns): this does a complete, blocking refresh.
			// Don't require a complete refresh with every change.
			if err := v.edit(viewDo{}, Notify); err != nil {
				return
			}
		}
	}
}

var (
	saveDot    = edit.Set(edit.Dot, TmpMark)
	restoreDot = edit.Set(edit.Mark('1'), '.')
)

func (v *View) edit(vd viewDo, Notify chan<- struct{}) error {
	v.mu.RLock()
	var prints []edit.Edit
	for _, m := range v.marks {
		n := m.Name
		if m.Name == '.' {
			n = TmpMark
		}
		prints = append(prints, edit.Where(edit.Mark(n)))
	}
	// Use the start of the mark's line, regardless of where it ends up in the line.
	start := edit.Mark(ViewMark).Minus(edit.Line(0)).Minus(edit.Rune(0))
	end := start.Plus(edit.Clamp(edit.Line(v.n)))
	win := start.To(end)
	prints = append(prints, edit.Print(win))
	v.mu.RUnlock()

	edits := append(vd.edits, saveDot, edit.Block(edit.All, prints...), restoreDot)
	res, err := editor.Do(v.textURL, edits...)
	// TODO(eaburns): If there is an error parsing the edit,
	// we have no way to signal it back to the original doer.
	if err != nil {
		if vd.result != nil {
			go func() { vd.result <- nil }()
		}
		return err
	}
	if vd.result != nil {
		go func() { vd.result <- res[:len(res)-3] }()
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	update := res[len(res)-2]
	printed := strings.SplitN(update.Print, "\n", len(prints))
	if len(printed) != len(prints) || update.Error != "" {
		panic(fmt.Sprintf("bad update: len(%v)=%d want %d, Error=%v",
			printed, len(printed), len(v.marks)+1, update.Error))
	}
	for i := range v.marks {
		m := &v.marks[i]
		n, err := fmt.Sscanf(printed[i], "#%d,#%d", &m.Where[0], &m.Where[1])
		if n == 1 {
			m.Where[1] = m.Where[0]
		} else if n != 2 || err != nil {
			panic("failed to scan address: " + printed[i])
		}
	}
	v.text = []byte(printed[len(printed)-1])
	v.seq = update.Sequence

	select {
	case Notify <- struct{}{}:
	default:
	}

	return nil
}
