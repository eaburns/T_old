// Copyright © 2016, The T Authors.

// Package edittest contains utility functions for testing edits.
package edittest

import (
	"fmt"
	"sort"
)

// StateEquals returns whether the state (text plus marks)
// equals the described state.
func StateEquals(text string, marks map[rune][2]int64, descr string) bool {
	wantText, wantMarks := ParseState(descr)
	if text != wantText || len(marks) != len(wantMarks) {
		return false
	}
	for m, a := range marks {
		if wantMarks[m] != a {
			return false
		}
	}
	return true
}

// ParseState parses an editor state description.
//
// An editor state description describes
// the contents of the buffer and the editor's marks.
// Runes that are not between { and } represent the buffer contents.
// Each rune between { and } represent
// the beginning (first occurrence)
// or end (second occurrence)
// of a mark region with the name of the rune.
//
// As a special case, the empty string, "", is equal to "{..}".
//
// For example:
// 	"{mm}abc{.}xyz{.n}123{n}αβξ"
// Is a buffer with the contents:
// 	"abcxyz123αβξ"
// The mark m is the empty string at the beginning of the buffer.
// The mark . contains the text "xyz".
// The mark n contains the text "123".
func ParseState(str string) (string, map[rune][2]int64) {
	if str == "" {
		str = "{..}"
	}

	var mark bool
	var contents []rune
	marks := make(map[rune][2]int64)
	count := make(map[rune]int)
	for _, r := range str {
		switch {
		case !mark && r == '{':
			mark = true
		case mark && r == '}':
			mark = false
		case mark:
			count[r]++
			at := int64(len(contents))
			if s, ok := marks[r]; !ok {
				marks[r] = [2]int64{at}
			} else {
				marks[r] = [2]int64{s[0], at}
			}
		default:
			contents = append(contents, r)
		}
	}
	for m, c := range count {
		if c != 2 {
			panic(fmt.Sprintf("%q, mark %c appears %d times", str, m, c))
		}
	}
	return string(contents), marks
}

// StateString returns the state string description of the text and marks.
// The returned string is in the format of ParseState.
// The returned string is normalized so that multiple marks within { and }
// are lexicographically ordered.
func StateString(text string, marks map[rune][2]int64) string {
	addrMarks := make(map[int64]RuneSlice)
	for m, a := range marks {
		addrMarks[a[0]] = append(addrMarks[a[0]], m)
		addrMarks[a[1]] = append(addrMarks[a[1]], m)
	}
	var c []rune
	str := []rune(text)
	for i := 0; i < len(str)+1; i++ {
		if ms := addrMarks[int64(i)]; len(ms) > 0 {
			sort.Sort(ms)
			c = append(c, '{')
			c = append(c, ms...)
			c = append(c, '}')
		}
		if i < len(str) {
			c = append(c, str[i])
		}
	}
	return string(c)

}

type RuneSlice []rune

func (rs RuneSlice) Len() int           { return len(rs) }
func (rs RuneSlice) Less(i, j int) bool { return rs[i] < rs[j] }
func (rs RuneSlice) Swap(i, j int)      { rs[i], rs[j] = rs[j], rs[i] }
