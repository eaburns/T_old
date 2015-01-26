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
	who        int
	edit, want string
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
		if err := eds[c.who].Edit([]rune(c.edit)); err != nil {
			t.Errorf("%v, %d, Edit(%v)=%v, want nil", test, i,
				strconv.Quote(c.edit), err)
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
