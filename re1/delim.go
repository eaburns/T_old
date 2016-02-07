package re1

import (
	"strings"
	"unicode/utf8"
)

// RemoveDelimiter interprets the first rune of the regexp as a delimiter,
// and returns the delimiter and the regexp with the delimiter removed.
//
// The given regexp is not checked for validity.
// If the regexp is valid, the return is guaranteed to be valid.
// If the regexp is invalid, there is no guarantee on the validity of the return.
func RemoveDelimiter(regexp string) (rune, string) {
	if len(regexp) == 0 {
		return 0, ""
	}
	var s []rune
	var d rune
	var esc, class bool
	d, w := utf8.DecodeRuneInString(regexp)
	for _, r := range regexp[w:] {
		if !esc && !class && r == d {
			break
		}
		if esc && r == d {
			// Escaped delimiter, strip the escape.
			s = s[:len(s)-1]
		}
		s = append(s, r)
		if r == '[' {
			class = true
		} else if class && r == ']' {
			class = false
		}
		esc = !esc && r == '\\'
	}
	return d, string(s)
}

// AddDelimiter returns the regexp, delimited by d.
// The given regexp is assumed to be non-delimited.
//
// The given regexp is not checked for validity.
// If the regexp is valid, the return is guaranteed to be valid.
// If the regexp is invalid, there is no guarantee on the validity of the return.
func AddDelimiter(d rune, regexp string) string {
	s := []rune{d}
	var esc, class bool
	for _, r := range regexp {
		if r == d && !class {
			if esc && strings.ContainsRune(Meta, d) {
				// Change escaped meta characters matching the delimiter
				// to character classes.
				s = append(s[:len(s)-1], '[', d, ']')
				esc = false
				continue
			}
			if !esc {
				s = append(s, '\\')
			}
		}
		s = append(s, r)
		if r == '[' {
			class = true
		} else if class && r == ']' {
			class = false
		}
		esc = !esc && r == '\\'
	}
	if esc {
		s = append(s, '\\')
	}
	return string(append(s, d))
}
