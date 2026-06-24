package keydec

import (
	"bytes"
	"io"
	"testing"
	"unicode/utf8"
)

// oneByteReader returns at most one byte per Read — proving the decoder never
// relies on buffered look-ahead across Read boundaries.
type oneByteReader struct {
	data []byte
	i    int
}

func (o *oneByteReader) Read(p []byte) (int, error) {
	if o.i >= len(o.data) {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}
	p[0] = o.data[o.i]
	o.i++
	return 1, nil
}

func keysOf(t *testing.T, r io.Reader) []Key {
	t.Helper()
	d := NewDecoder(r)
	var out []Key
	for {
		k, err := d.ReadKey()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatalf("ReadKey error: %v", err)
		}
		out = append(out, k)
	}
}

func TestDecoderTable(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want []Key
	}{
		{"ascii", []byte("ab"), []Key{{KeyRune, 'a'}, {KeyRune, 'b'}}},
		{"enter cr", []byte("\r"), []Key{{KeyEnter, 0}}},
		{"enter lf", []byte("\n"), []Key{{KeyEnter, 0}}},
		{"backspace del", []byte("\x7f"), []Key{{KeyBackspace, 0}}},
		{"backspace bs", []byte("\x08"), []Key{{KeyBackspace, 0}}},
		{"tab", []byte("\t"), []Key{{KeyTab, 0}}},
		{"ctrl-a", []byte("\x01"), []Key{{KeyCtrlA, 0}}},
		{"ctrl-c", []byte("\x03"), []Key{{KeyCtrlC, 0}}},
		{"ctrl-d", []byte("\x04"), []Key{{KeyCtrlD, 0}}},
		{"ctrl-e", []byte("\x05"), []Key{{KeyCtrlE, 0}}},
		{"ctrl-k", []byte("\x0b"), []Key{{KeyCtrlK, 0}}},
		{"ctrl-l", []byte("\x0c"), []Key{{KeyCtrlL, 0}}},
		{"ctrl-u", []byte("\x15"), []Key{{KeyCtrlU, 0}}},
		{"ctrl-w", []byte("\x17"), []Key{{KeyCtrlW, 0}}},
		{"utf8 2byte", []byte("é"), []Key{{KeyRune, 'é'}}},
		{"utf8 3byte", []byte("世"), []Key{{KeyRune, '世'}}},
		{"utf8 4byte", []byte("😀"), []Key{{KeyRune, '😀'}}},
		{"csi up", []byte("\x1b[A"), []Key{{KeyUp, 0}}},
		{"csi down", []byte("\x1b[B"), []Key{{KeyDown, 0}}},
		{"csi right", []byte("\x1b[C"), []Key{{KeyRight, 0}}},
		{"csi left", []byte("\x1b[D"), []Key{{KeyLeft, 0}}},
		{"ss3 up", []byte("\x1bOA"), []Key{{KeyUp, 0}}},
		{"home letter", []byte("\x1b[H"), []Key{{KeyHome, 0}}},
		{"end letter", []byte("\x1b[F"), []Key{{KeyEnd, 0}}},
		{"home tilde", []byte("\x1b[1~"), []Key{{KeyHome, 0}}},
		{"end tilde", []byte("\x1b[4~"), []Key{{KeyEnd, 0}}},
		{"delete tilde", []byte("\x1b[3~"), []Key{{KeyDelete, 0}}},
		{"shift-tab", []byte("\x1b[Z"), []Key{{KeyShiftTab, 0}}},
		{"modified arrow", []byte("\x1b[1;2A"), []Key{{KeyUp, 0}}},
		{"lone esc", []byte("\x1b"), []Key{{KeyEsc, 0}}},
		{"esc then x", []byte("\x1bx"), []Key{{KeyEsc, 0}, {KeyRune, 'x'}}},
		{"pageup unknown", []byte("\x1b[5~"), []Key{{KeyUnknown, 0}}},
		{"runaway params", []byte("\x1b[1234567890123456789~"), []Key{{KeyUnknown, 0}}},
		{"truncated utf8", []byte{0xC3}, []Key{{KeyRune, utf8.RuneError}}},
		{"rune after arrow", []byte("\x1b[Cx"), []Key{{KeyRight, 0}, {KeyRune, 'x'}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, mk := range []struct {
				tag string
				r   io.Reader
			}{
				{"bytes", bytes.NewReader(c.in)},
				{"onebyte", &oneByteReader{data: c.in}},
			} {
				if got := keysOf(t, mk.r); !equalKeys(got, c.want) {
					t.Errorf("[%s] decode(%q) = %v, want %v", mk.tag, c.in, got, c.want)
				}
			}
		})
	}
}

func equalKeys(a, b []Key) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestDecoderCleanEOF(t *testing.T) {
	if _, err := NewDecoder(bytes.NewReader(nil)).ReadKey(); err != io.EOF {
		t.Fatalf("empty stream should return io.EOF, got %v", err)
	}
}
