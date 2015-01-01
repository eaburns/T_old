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
	if ed.dot != test.dot {
		t.Errorf("%v: dot=%v, want %v\n", test, ed.dot, test.dot)
		return
	}
	rs, err := ed.Read(All())
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
		if ed.dot != test.dot {
			t.Errorf("%v: dot=%v, want %v\n", test, ed.dot, test.dot)
			continue
		}
		rs, err := ed.Read(All())
		if s := string(rs); s != test.want || err != nil {
			t.Errorf("%v: string=%v, want %v\n",
				test, strconv.Quote(s), strconv.Quote(test.want))
			continue
		}
	}
}

func TestConcurrentSimpleChanges(t *testing.T) {
	tests := [][]struct {
		who             int
		addr, str, want string
	}{
		{
			{who: 0, addr: "0", str: "世界!", want: "世界!"},
			{who: 1, addr: "0", str: "Hello, ", want: "Hello, 世界!"},
			{who: 0, addr: ".", str: "", want: "Hello, "},
			{who: 1, addr: ".", str: "", want: ""},
		},
		{
			{who: 0, addr: "0", str: "世界!", want: "世界!"},
			{who: 1, addr: "0,#1", str: "Hello, ", want: "Hello, 界!"},
			{who: 0, addr: ".", str: "", want: "Hello, "},
			{who: 1, addr: ".", str: "", want: ""},
		},
	}
	for _, test := range tests {
		b := NewBuffer()
		defer b.Close()
		eds := [2]*Editor{b.NewEditor(), b.NewEditor()}
		for _, ed := range eds {
			defer ed.Close()
		}
		for i, c := range test {
			addr, _, err := Addr([]rune(c.addr))
			if err != nil {
				t.Errorf("%v, %d: bad address: %s", test, i, c.addr)
				return
			}
			if err = eds[c.who].Change(addr, []rune(c.str)); err != nil {
				t.Errorf("%v, %d, Change(%v, %v)=%v, want nil", test, i, c.addr, c.str, err)
				break
			}
			rs := make([]rune, b.runes.Size())
			if _, err := b.runes.Read(rs, 0); err != nil {
				t.Errorf("%v, %d, read failed=%v\n", test, i, err)
				break
			}
			if s := string(rs); s != c.want || err != nil {
				t.Errorf("%v, %d: string=%v, want %v\n",
					test, i, strconv.Quote(s), strconv.Quote(c.want))
				break
			}
		}
	}
}
