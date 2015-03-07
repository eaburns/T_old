package edit

import (
	"strconv"
	"testing"
)

func TestAppend(t *testing.T) {
	str := "Hello, 世界!"
	tests := []changeTest{
		{init: str, addr: "#0", add: "XYZ", want: "XYZHello, 世界!", dot: addr{0, 3}},
		{init: str, addr: "$", add: "XYZ", want: "Hello, 世界!XYZ", dot: addr{10, 13}},
		{init: str, addr: "#1", add: "XYZ", want: "HXYZello, 世界!", dot: addr{1, 4}},
		{init: str, addr: "#0,#1", add: "XYZ", want: "HXYZello, 世界!", dot: addr{1, 4}},
	}
	for _, test := range tests {
		test.run((*Editor).Append, "Append", t)
	}
}

func TestInsert(t *testing.T) {
	str := "Hello, 世界!"
	tests := []changeTest{
		{init: str, addr: "#0", add: "XYZ", want: "XYZHello, 世界!", dot: addr{0, 3}},
		{init: str, addr: "$", add: "XYZ", want: "Hello, 世界!XYZ", dot: addr{10, 13}},
		{init: str, addr: "#1", add: "XYZ", want: "HXYZello, 世界!", dot: addr{1, 4}},
		{init: str, addr: "#0,#1", add: "XYZ", want: "XYZHello, 世界!", dot: addr{0, 3}},
	}
	for _, test := range tests {
		test.run((*Editor).Insert, "Insert", t)
	}
}

func TestChange(t *testing.T) {
	str := "Hello, 世界!"
	tests := []changeTest{
		{init: str, addr: "#0", add: "XYZ", want: "XYZHello, 世界!", dot: addr{0, 3}},
		{init: str, addr: "$", add: "XYZ", want: "Hello, 世界!XYZ", dot: addr{10, 13}},
		{init: str, addr: "#1", add: "XYZ", want: "HXYZello, 世界!", dot: addr{1, 4}},
		{init: str, addr: "#0,#1", add: "XYZ", want: "XYZello, 世界!", dot: addr{0, 3}},
		{init: str, addr: "#1,$-#1", add: "XYZ", want: "HXYZ!", dot: addr{1, 4}},
	}
	for _, test := range tests {
		test.run((*Editor).Change, "Change", t)
	}
}

type changeTest struct {
	init, want, addr, add string
	dot                   addr
}

func (test changeTest) run(f func(*Editor, Address, []rune) error, name string, t *testing.T) {
	ed := NewBuffer().NewEditor()
	defer ed.Close()
	defer ed.buf.Close()
	if err := ed.Append(Rune(0), []rune(test.init)); err != nil {
		t.Fatalf("%v, init failed", test)
	}
	addr, _, err := Addr([]rune(test.addr))
	if err != nil {
		t.Errorf("%v: bad address: %s", test, test.addr)
		return
	}
	if err := f(ed, addr, []rune(test.add)); err != nil {
		t.Errorf("%v: %s(%v, %v)=%v, want nil", test, name, test.addr, test.add, err)
		return
	}
	if ed.marks['.'] != test.dot {
		t.Errorf("%v: dot=%v, want %v\n", test, ed.marks['.'], test.dot)
		return
	}
	rs, err := ed.Read(All)
	if s := string(rs); s != test.want || err != nil {
		t.Errorf("%v: string=%v, want %v\n",
			test, strconv.Quote(s), strconv.Quote(test.want))
		return
	}
}

func TestEditorDelete(t *testing.T) {
	str := "Hello, 世界!"
	tests := []struct {
		init, want, addr string
		dot              addr
	}{
		{init: str, addr: "#0", want: str, dot: addr{0, 0}},
		{init: str, addr: "#5", want: str, dot: addr{5, 5}},
		{init: str, addr: "0,$", want: "", dot: addr{0, 0}},
		{init: str, addr: "#0,#8", want: "界!", dot: addr{0, 0}},
		{init: str, addr: "#2,$", want: "He", dot: addr{2, 2}},
		{init: str, addr: "#2,$-#2", want: "He界!", dot: addr{2, 2}},
	}
	for _, test := range tests {
		ed := NewBuffer().NewEditor()
		defer ed.Close()
		defer ed.buf.Close()
		if err := ed.Append(Rune(0), []rune(test.init)); err != nil {
			t.Fatalf("%v, init failed", test)
		}
		addr, _, err := Addr([]rune(test.addr))
		if err != nil {
			t.Errorf("%v: bad address: %s", test, test.addr)
			return
		}
		if err := ed.Delete(addr); err != nil {
			t.Errorf("%v: Delete(%v)=%v, want nil", test, test.addr, err)
			continue
		}
		if ed.marks['.'] != test.dot {
			t.Errorf("%v: dot=%v, want %v\n", test, ed.marks['.'], test.dot)
			continue
		}
		rs, err := ed.Read(All)
		if s := string(rs); s != test.want || err != nil {
			t.Errorf("%v: string=%v, want %v\n",
				test, strconv.Quote(s), strconv.Quote(test.want))
			continue
		}
	}
}

func TestEditorMark(t *testing.T) {
	tests := []multiEditTest{
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: "/, World/m a", want: "Hello, World!"},
			{edit: "'a d", want: "Hello!"},
		},
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: "/, World/m", want: "Hello, World!"},
			{edit: "d", want: "Hello!"},
		},

		// Edit after the mark.
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: "/Hello/m a", want: "Hello, World!"},
			{edit: "/, World/d", want: "Hello!"},
			{edit: "'a d", want: "!"},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/m a", want: "abc123xyz"},
			{edit: "/xyz/d", want: "abc123"},
			{edit: "'a d", want: "abc"},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/m a", want: "abc123xyz"},
			{edit: "/z/d", want: "abc123xy"},
			{edit: "'a d", want: "abcxy"},
		},
		{
			{edit: "a/abc123", want: "abc123"},
			{edit: "#3m a", want: "abc123"},
			{edit: "$a/xyz", want: "abc123xyz"},
			{edit: "'a a/...", want: "abc...123xyz"},
		},

		// Edit before the mark.
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: "/World/m a", want: "Hello, World!"},
			{edit: "/Hello, /d", want: "World!"},
			{edit: "'a d", want: "!"},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/m a", want: "abc123xyz"},
			{edit: "/abc/d", want: "123xyz"},
			{edit: "'a d", want: "xyz"},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/m a", want: "abc123xyz"},
			{edit: "/a/d", want: "bc123xyz"},
			{edit: "'a d", want: "bcxyz"},
		},
		{
			{edit: "a/abc123", want: "abc123"},
			{edit: "#3m a", want: "abc123"},
			{edit: "#0a/xyz", want: "xyzabc123"},
			{edit: "'a a/...", want: "xyzabc...123"},
		},
		{
			{edit: "a/abc123", want: "abc123"},
			{edit: "#3m a", want: "abc123"},
			{edit: "#3a/xyz", want: "abcxyz123"},
			{edit: "'a a/...", want: "abcxyz...123"},
		},

		// Edit within the mark.
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: ",m a", want: "Hello, World!"},
			{edit: "/ /c/ Cruel /", want: "Hello, Cruel World!"},
			{edit: "'a d", want: ""},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/m a", want: "abc123xyz"},
			{edit: "/2/d", want: "abc13xyz"},
			{edit: "'a c/123", want: "abc123xyz"},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/m a", want: "abc123xyz"},
			{edit: "/2/a/2.5", want: "abc122.53xyz"},
			{edit: "'a c/123", want: "abc123xyz"},
		},

		// Edit over the mark.
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: "/World/m a", want: "Hello, World!"},
			{edit: ",c/abc", want: "abc"},
			{edit: "'a a/123", want: "abc123"},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/m a", want: "abc123xyz"},
			{edit: "/c123x/d", want: "abyz"},
			{edit: "'a c/123", want: "ab123yz"},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/m a", want: "abc123xyz"},
			{edit: "/c123x/c/C123X", want: "abC123Xyz"},
			{edit: "'a c/...", want: "abC123X...yz"},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/m a", want: "abc123xyz"},
			{edit: "/123xyz/d", want: "abc"},
			{edit: "'a c/...", want: "abc..."},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/m a", want: "abc123xyz"},
			{edit: "/abc123/d", want: "xyz"},
			{edit: "'a c/...", want: "...xyz"},
		},
		{
			{edit: "a/abc123", want: "abc123"},
			{edit: "#3m a", want: "abc123"},
			{edit: "/bc12/d", want: "a3"},
			{edit: "'a c/...", want: "a...3"},
		},

		// Edit over the beginning of the mark.
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: "/World/m a", want: "Hello, World!"},
			{edit: "/W/c/w", want: "Hello, world!"},
			{edit: "'a d", want: "Hello, w!"},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/m a", want: "abc123xyz"},
			{edit: "/bc1/d", want: "a23xyz"},
			{edit: "'a c/bc", want: "abcxyz"},
		},

		// Edit over the end of the mark.
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: "/World/m a", want: "Hello, World!"},
			{edit: "/d/c/D", want: "Hello, WorlD!"},
			{edit: "'a d", want: "Hello, D!"},
		},
		{
			{edit: "a/abc123xyz", want: "abc123xyz"},
			{edit: "/123/m a", want: "abc123xyz"},
			{edit: "/3xy/d", want: "abc12z"},
			{edit: "'a c/xy", want: "abcxyz"},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestEditorPrint(t *testing.T) {
	tests := []multiEditTest{
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: "p", print: "Hello, World!", want: "Hello, World!"},
			{edit: "#1p", print: "", want: "Hello, World!"},
			{edit: "#0,#1p", print: "H", want: "Hello, World!"},
			{edit: "1p", print: "Hello, World!", want: "Hello, World!"},
			{edit: "/e.*/p", print: "ello, World!", want: "Hello, World!"},
			{edit: "0p", print: "", want: "Hello, World!"},
			{edit: "$p", print: "", want: "Hello, World!"},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestEditorWhere(t *testing.T) {
	tests := []multiEditTest{
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: "=#", print: "#0,#13", want: "Hello, World!"},
			{edit: "/e.*/=#", print: "#1,#13", want: "Hello, World!"},
			{edit: "0=#", print: "#0,#0", want: "Hello, World!"},
			{edit: "$=#", print: "#13,#13", want: "Hello, World!"},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestEditorSubstitute(t *testing.T) {
	tests := []multiEditTest{
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: ",s/World/世界", want: "Hello, 世界!"},
			{edit: "d", want: ""},
		},
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: "/World/s/World/世界", want: "Hello, 世界!"},
			{edit: "d", want: "Hello, !"},
		},
		{
			{edit: "a/abcabc", want: "abcabc"},
			{edit: ",s/abc/defg/", want: "defgabc"},
		},
		{
			{edit: "a/abcabcabc", want: "abcabcabc"},
			{edit: ",s/abc/defg/g", want: "defgdefgdefg"},
		},
		{
			{edit: "a/abcabcabc", want: "abcabcabc"},
			{edit: "/abcabc/s/abc/defg/g", want: "defgdefgabc"},
			{edit: "d", want: "abc"},
		},
		{
			{edit: "a/abc abc", want: "abc abc"},
			{edit: ",s/abc/defg/", want: "defg abc"},
		},
		{
			{edit: "a/abc abc abc", want: "abc abc abc"},
			{edit: ",s/abc/defg/g", want: "defg defg defg"},
		},
		{
			{edit: "a/abc abc abc", want: "abc abc abc"},
			{edit: "/abc abc/s/abc/defg/g", want: "defg defg abc"},
			{edit: "d", want: " abc"},
		},
		{
			{edit: "a/abcabc", want: "abcabc"},
			{edit: ",s/abc/de/", want: "deabc"},
		},
		{
			{edit: "a/abcabcabc", want: "abcabcabc"},
			{edit: ",s/abc/de/g", want: "dedede"},
		},
		{
			{edit: "a/abcabcabc", want: "abcabcabc"},
			{edit: "/abcabc/s/abc/de/g", want: "dedeabc"},
			{edit: "d", want: "abc"},
		},
		{
			{edit: "a/abcabcabc", want: "abcabcabc"},
			{edit: "s/abc//g", want: ""},
			{edit: "a/xyz", want: "xyz"},
		},
		{
			{edit: "a/func f()", want: "func f()"},
			{edit: `s/func (.*)\(\)/func (T)\1()`, want: "func (T)f()"},
			{edit: "d", want: ""},
		},
		{
			{edit: "a/abcdefghi", want: "abcdefghi"},
			{edit: `s/(abc)(def)(ghi)/\0 \3 \2 \1`, want: "abcdefghi ghi def abc"},
		},
		{
			{edit: "a/abc", want: "abc"},
			{edit: `s/abc/\1`, want: ""},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestConcurrentSimpleChanges(t *testing.T) {
	tests := []multiEditTest{
		{
			{who: 0, edit: "0c/世界!", want: "世界!"},
			{who: 1, edit: "0c/Hello, ", want: "Hello, 世界!"},
			{who: 0, edit: ".d", want: "Hello, "},
			{who: 1, edit: ".d", want: ""},
		},
		{
			{who: 0, edit: "0c/世界!", want: "世界!"},
			{who: 1, edit: "0,#1c/Hello, ", want: "Hello, 界!"},
			{who: 0, edit: ".d", want: "Hello, "},
			{who: 1, edit: ".d", want: ""},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

func TestEditorEdit(t *testing.T) {
	tests := []multiEditTest{
		{{edit: "$a/Hello, World!/", want: "Hello, World!"}},
		{{edit: "$a\nHello, World!\n.", want: "Hello, World!\n"}},
		{{edit: `$a/Hello, World!\/`, want: "Hello, World!/"}},
		{{edit: `$a/Hello, World!\n`, want: "Hello, World!\n"}},
		{{edit: "$i/Hello, World!/", want: "Hello, World!"}},
		{{edit: "$i/Hello, World!", want: "Hello, World!"}},
		{{edit: "$i\nHello, World!\n.", want: "Hello, World!\n"}},
		{{edit: `$i/Hello, World!\/`, want: "Hello, World!/"}},
		{{edit: `$i/Hello, World!\n`, want: "Hello, World!\n"}},
		{
			{edit: "0a/Hello, World!", want: "Hello, World!"},
			{edit: ",c/Bye", want: "Bye"},
		},
		{
			{edit: "0a/Hello", want: "Hello"},
			{edit: "/ello/c/i", want: "Hi"},
		},
		{
			{edit: "0a/Hello, World!", want: "Hello, World!"},
			{edit: "?World?c/世界", want: "Hello, 世界!"},
		},
		{
			{edit: "0a/Hello, World!", want: "Hello, World!"},
			{edit: "/, World/d", want: "Hello!"},
		},
		{
			{edit: "0a/Hello, World!", want: "Hello, World!"},
			{edit: ",d", want: ""},
		},
		{
			{edit: "0a/Hello, World!", want: "Hello, World!"},
			{edit: "d", want: ""}, // Address defaults to dot.
		},
		{
			{edit: "a/Hello, World!", want: "Hello, World!"},
			{edit: "/World/c/Test", want: "Hello, Test!"},
			{edit: "c/World", want: "Hello, World!"},
		},
	}
	for _, test := range tests {
		test.run(t)
	}
}

// A MultiEditTest tests mutiple edits performed on a buffer.
// It checks that the buffer has the desired text after each edit.
type multiEditTest []struct {
	who               int
	edit, print, want string
}

func (test multiEditTest) nEditors() int {
	var n int
	for _, c := range test {
		if c.who > n {
			n = c.who
		}
	}
	return n + 1
}

func (test multiEditTest) run(t *testing.T) {
	b := NewBuffer()
	defer b.Close()
	eds := make([]*Editor, test.nEditors())
	for i := range eds {
		eds[i] = b.NewEditor()
		defer eds[i].Close()
	}
	for i, c := range test {
		pr, err := eds[c.who].Edit([]rune(c.edit))
		if pr := string(pr); pr != c.print || err != nil {
			t.Errorf("%v, %d, Edit(%v)=%q,%v, want %q,nil", test, i,
				strconv.Quote(c.edit), pr, err, c.print)
			continue
		}
		rs := make([]rune, b.runes.Size())
		if _, err := b.runes.Read(rs, 0); err != nil {
			t.Errorf("%v, %d, read failed=%v\n", test, i, err)
			continue
		}
		if s := string(rs); s != c.want {
			t.Errorf("%v, %d: string=%v, want %v\n",
				test, i, strconv.Quote(s), strconv.Quote(c.want))
			continue
		}
	}
}
