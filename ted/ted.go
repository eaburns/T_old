// Ted is a text editor similar to sam in it's non-downloaded mode.
//
// The Ted editor is mostly intended as an experiment.
// It edits a single buffer, with the T edit language.
// The T language is documented here:
// https://godoc.org/github.com/eaburns/T/edit#Ed.
// Ted adds a few additional commands:
// 	e filename 	replaces the buffer with the contents of the file
// 	w filename	saves the buffer to the named file
// 	q 		quits
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/eaburns/T/edit"
)

var (
	logPath = flag.String("log", "", "a file to which all T edit commands are logged")
)

func main() {
	flag.Parse()

	var log io.Writer
	if *logPath != "" {
		var err error
		if log, err = os.Create(*logPath); err != nil {
			fmt.Println("failed to open log:", err)
			return
		}
	}

	in := bufio.NewReader(os.Stdin)

	buf := edit.NewBuffer()
	defer buf.Close()
	ed := edit.NewEditor(buf)
	defer ed.Close()

	var nl bool
	var prevAddr edit.Edit
	for {
		var e edit.Edit
		r, _, err := in.ReadRune()
		switch {
		case err != nil && err != io.EOF:
			fmt.Println("failed to read input:", err)
			return
		case err == io.EOF || r == 'q':
			return
		case r == '\n':
			if nl && prevAddr != nil {
				e = prevAddr
			}
		case r == 'w':
			line, err := readLine(in)
			if err != nil {
				fmt.Println("failed to read input:", err)
				return
			}
			e = edit.PipeTo(edit.All, "cat > "+strings.TrimSpace(line))
		case r == 'e':
			line, err := readLine(in)
			if err != nil {
				fmt.Println("failed to read input:", err)
				return
			}
			e = edit.PipeFrom(edit.All, "cat < "+strings.TrimSpace(line))
		default:
			if err := in.UnreadRune(); err != nil {
				panic(err) // Can't fail with bufio.Reader.
			}
			var err error
			e, err = edit.Ed(in)
			if err != nil {
				fmt.Println(err)
				readLine(in) // Chomp until EOL.
				continue
			}
		}
		nl = r == '\n'
		if e == nil {
			continue
		}

		if err := ed.Do(e, os.Stdout); err != nil {
			fmt.Println(err)
			continue
		}
		if log != nil {
			if _, err := io.WriteString(log, e.String()+"\n"); err != nil {
				fmt.Println("failed log edit:", err)
				return
			}
		}

		if strings.HasSuffix(e.String(), "=") || strings.HasSuffix(e.String(), "=#") {
			// The Edit just printed an address. Put a newline after it.
			fmt.Println("")
		}

		prevAddr = nil
		if strings.HasSuffix(e.String(), "k.") {
			// The Edit just set the address of dot. Print dot.
			prevAddr = e
			if err := ed.Do(edit.Print(edit.Dot), os.Stdout); err != nil {
				fmt.Println("failed to edit:", err)
				return
			}
		}
	}
}

func readLine(in io.RuneScanner) (string, error) {
	var s []rune
	for {
		switch r, _, err := in.ReadRune(); {
		case err != nil && err != io.EOF:
			return "", err
		case err == io.EOF || r == '\n':
			return string(s), nil
		default:
			s = append(s, r)
		}
	}
}
