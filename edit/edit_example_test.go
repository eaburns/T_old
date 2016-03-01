package edit

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"strings"
)

func ExampleAddress() {
	// Create a new editor, editing a buffer initialized with some text.
	ed := NewEditor(NewBuffer())
	defer ed.buf.Close()
	if err := ed.Do(Append(All, "Hello, 世界!"), ioutil.Discard); err != nil {
		panic(err)
	}

	// The Addr function parses an address from a []rune.
	// It is intended to be called with runes input by a UI.
	// This wrapper makes it a bit more friendly for our example.
	parseAddr := func(s string) Address {
		a, err := Addr(strings.NewReader(s))
		if err != nil {
			panic(err)
		}
		return a
	}

	// Create various addresses.
	// Addresses can be created in two ways:
	// 1) With the T address language, parsing them with Addr.
	// 2) With functions.
	addrs := []Address{
		// 0,$ is how T specifies the address of the entire buffer.
		parseAddr("0,$"),
		// , is short-hand for 0,$
		parseAddr(","),
		// The address can also be constructed directly.
		Rune(0).To(End),
		// All is a convenient variable for the address of the entire buffer.
		All,

		// Regular expressions.
		parseAddr("/Hello/"),
		Regexp("Hello"),
		// A regular expression, searching in reverse.
		End.Minus(Regexp("Hello")),

		// Line addresses.
		parseAddr("1"),
		Line(1),

		// Range addresses.
		parseAddr("#1,#5"),
		Rune(1).To(Rune(5)),

		// Addresses relative to other addresses.
		parseAddr("#0+/l/,#5"),
		Rune(0).Plus(Regexp("l")).To(Rune(5)),
		parseAddr("$-/l/,#5"),
		End.Minus(Regexp("l")).To(Rune(5)),
	}

	// Print the contents of the editor at each address to os.Stdout.
	for _, a := range addrs {
		buf := bytes.NewBuffer(nil)
		s, err := a.Where(ed)
		if err != nil {
			panic(err)
		}
		if _, err := io.Copy(buf, ed.Reader(s)); err != nil {
			panic(err)
		}
		os.Stdout.WriteString(buf.String() + "\n")
	}

	// Output:
	// Hello, 世界!
	// Hello, 世界!
	// Hello, 世界!
	// Hello, 世界!
	// Hello
	// Hello
	// Hello
	// Hello, 世界!
	// Hello, 世界!
	// ello
	// ello
	// llo
	// llo
	// lo
	// lo
}

func ExampleEdit() {
	// Create a new editor, editing a buffer initialized with some text.
	ed := NewEditor(NewBuffer())
	defer ed.buf.Close()
	if err := ed.Do(Append(All, "Hello, World!\n"), ioutil.Discard); err != nil {
		panic(err)
	}

	// The Ed function parses an Edit from a []rune.
	// It is intended to be called with runes input by a UI.
	// This wrapper makes it a bit more friendly for our example.
	parseEd := func(s string) Edit {
		e, err := Ed(strings.NewReader(s))
		if err != nil {
			panic(err)
		}
		return e
	}

	// Create various Edits.
	// Edits can be created in two ways:
	// 1) With the T command language, parsing them with Ed.
	// 2) With functions.
	edits := []Edit{
		// p prints the contents at the address preceeding it.
		parseEd("0,$p"),
		// Here is the same Edit built with a funciton.
		Print(All),
		// This prints a different address.
		Print(Regexp(",").Plus(Rune(1)).To(End)),

		// c changes the buffer at a given address preceeding it.
		// After this change, the buffer will contain: "Hello, 世界!\n"
		parseEd("/World/c/世界"),
		parseEd(",p"),

		// Or you can do it with functions.
		// After this change, the buffer will contain: "Hello, World!\n" again.
		Change(Regexp("世界"), "World"),
		Print(All),

		// There is infinite Undo…
		parseEd("u"),
		Undo(1),

		// … and infinite Redo.
		parseEd("r"),
		Redo(1),
		Print(All),

		// You can also edit with regexp substitution.
		Change(All, "...===...\n"),
		Sub(All, "(=+)", "---${1}---"),
		Print(All),
		parseEd(`,s/[.]/_/g`),
		Print(All),

		// And various other things…
	}

	// Perform the Edits.
	// Printed output is written to os.Stdout.
	for _, e := range edits {
		if err := ed.Do(e, os.Stdout); err != nil {
			panic(err)
		}
	}

	// Output:
	// Hello, World!
	// Hello, World!
	// World!
	// Hello, 世界!
	// Hello, World!
	// Hello, World!
	// ...---===---...
	// ___---===---___
}
