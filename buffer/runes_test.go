package buffer

import (
	"bytes"
	"testing"
	"unicode/utf8"
)

// TestRunesBasics tests writing to a buffer and then reading it all back.
func TestRunesBasics(t *testing.T) {
	str := "Hello, World! ☺"
	l := int64(utf8.RuneCountInString(str))
	b := NewRunes(testBlockSize)
	defer b.Close()

	err := b.Put([]rune(str), Address{})
	if err != nil {
		t.Fatalf(`Put(%s), Address{})=%v, want nil`, str, err)
	}
	if s := b.Size(); s != l {
		t.Fatalf(`Size()=%d, want %d`, s, l)
	}
	rs, err := b.Get(Address{From: 0, To: b.Size()})
	if string(rs) != str || err != nil {
		t.Fatalf(`Get(Address{0, %d})="%v",%v, want %s,nil`, b.Size(), string(rs), err, str)
	}
}

func TestRunesPut(t *testing.T) {
	tests := []struct {
		init  string
		write string
		at    Address
		want  string
		err   error
	}{
		{at: Address{From: 1, To: 2}, err: AddressError(Address{From: 1, To: 2})},
		{at: Address{From: -1, To: 0}, err: AddressError(Address{From: -1, To: 0})},
		{at: Address{From: 0, To: 1}, err: AddressError(Address{From: 0, To: 1})},
		{at: Address{From: 2, To: 1}, err: AddressError(Address{From: 2, To: 1})},

		{init: "", write: "", at: Address{}, want: ""},
		{init: "", write: "Hello, World!", at: Address{}, want: "Hello, World!"},
		{init: "", write: "Hello, 世界", at: Address{}, want: "Hello, 世界"},
		{init: "Hello, World!", write: "", at: Address{0, 13}, want: ""},
		{init: "Hello, !", write: "World", at: Address{7, 7}, want: "Hello, World!"},
		{init: "Hello, World", write: "! ☺", at: Address{12, 12}, want: "Hello, World! ☺"},
		{init: ", World!", write: "Hello", at: Address{0, 0}, want: "Hello, World!"},
	}
	for _, test := range tests {
		b := NewRunes(testBlockSize)
		defer b.Close()
		if err := b.Put([]rune(test.init), Address{}); err != nil {
			t.Errorf("init Put(%v, Address{})=%v, want nil", test.init, err)
			continue
		}
		if err := b.Put([]rune(test.write), test.at); err != test.err {
			t.Errorf("Put(%v, %v)=%v, want %v", test.write, test.at, err, test.err)
			continue
		}
		if test.err != nil {
			continue
		}
		rs, err := b.Get(Address{From: 0, To: b.Size()})
		if s := string(rs); s != test.want || err != nil {
			t.Errorf(`%+v Get(Address{0, %d})="%s",%v, want %v,nil`,
				test, b.Size(), s, err, test.want)
			continue
		}
	}
}

func TestRunesGet(t *testing.T) {
	tests := []struct {
		init string
		at   Address
		want string
		err  error
	}{
		{at: Address{From: 1, To: 2}, err: AddressError(Address{From: 1, To: 2})},
		{at: Address{From: -1, To: 0}, err: AddressError(Address{From: -1, To: 0})},
		{at: Address{From: 0, To: 1}, err: AddressError(Address{From: 0, To: 1})},
		{at: Address{From: 2, To: 1}, err: AddressError(Address{From: 2, To: 1})},

		{init: "", at: Address{}, want: ""},
		{init: "Hello, World!", at: Address{13, 13}, want: ""},
		{init: "Hello, World!", at: Address{0, 13}, want: "Hello, World!"},
		{init: "Hello, 世界", at: Address{0, 9}, want: "Hello, 世界"},
		{init: "Hello, 世界", at: Address{1, 9}, want: "ello, 世界"},
		{init: "Hello, 世界", at: Address{0, 8}, want: "Hello, 世"},
		{init: "Hello, 世界", at: Address{1, 8}, want: "ello, 世"},
	}
	for _, test := range tests {
		b := NewRunes(testBlockSize)
		defer b.Close()
		if err := b.Put([]rune(test.init), Address{}); err != nil {
			t.Errorf("init Put(%v, Address{})=%v, want nil", test.init, err)
			continue
		}
		rs, err := b.Get(test.at)
		if s := string(rs); s != test.want || err != test.err {
			t.Errorf(`Get(%v)="%s",%v, want %v,%v`,
				test.at, s, err, test.want, test.err)
			continue
		}
	}
}

func TestRunesReadFrom(t *testing.T) {
	tests := []struct {
		init, get, want string
	}{
		{get: "", want: ""},
		{init: "Initial things", get: "", want: ""},
		{get: "Hello, World!", want: "Hello, World!"},
		{init: "Some stuff ☺", get: "Hello, World!", want: "Hello, World!"},
		{get: "Hello, 世界", want: "Hello, 世界"},

		// Invalid UTF8 encoding:
		// bytes.Buffer.ReadRune and bufio.Reader.ReadRune
		// both replace invalid bytes with unicode.ReplacementChar
		// '\uFFFD'.
		{get: "abc\x80def", want: "abc\uFFFDdef"},
	}
	for _, test := range tests {
		b := NewRunes(testBlockSize)
		defer b.Close()
		n, err := b.ReadFrom(bytes.NewBuffer([]byte(test.get)))
		if l := int64(len(test.get)); n != l || err != nil {
			t.Errorf("Get(%s)=%v,%v, want %v,nil", test.get, n, err, l)
			continue
		}
		rs, err := b.Get(Address{From: 0, To: b.Size()})
		if s := string(rs); s != test.want || err != nil {
			t.Errorf(`Get(all)="%s",%v, want %v,nil`, s, err, test.want)
			continue
		}
	}
}

func TestRunesWriteTo(t *testing.T) {
	big := make([]rune, testBlockSize*2)
	for i := range big {
		big[i] = rune(i)
	}

	tests := []string{
		"",
		"Hello, World!",
		"Hello, 世界",
		"abc\uFFFDdef",

		// Test the chunking logic in Put().
		string(big),
	}
	for _, test := range tests {
		b := NewRunes(testBlockSize)
		defer b.Close()
		if err := b.Put([]rune(test), Address{From: b.Size(), To: b.Size()}); err != nil {
			t.Fatalf(`b.Put("%s", end)=%v, want nil`, test, err)
		}

		f := bytes.NewBuffer(nil)
		n, err := b.WriteTo(f)
		if l := int64(len(test)); n != l || err != nil {
			t.Errorf(`b.Put("%s")=%v,%v, want %v,nil`, test, n, err, l)
			continue
		}
		if s := f.String(); s != test {
			t.Errorf(`f.String()="%s", want "%s"`, s, test)
			continue
		}
	}
}
