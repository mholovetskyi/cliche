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
		{"ascii", []byte("ab"), []Key{{Type: KeyRune, Rune: 'a'}, {Type: KeyRune, Rune: 'b'}}},
		{"enter cr", []byte("\r"), []Key{{Type: KeyEnter}}},
		{"enter lf", []byte("\n"), []Key{{Type: KeyEnter}}},
		{"backspace del", []byte("\x7f"), []Key{{Type: KeyBackspace}}},
		{"backspace bs", []byte("\x08"), []Key{{Type: KeyBackspace}}},
		{"tab", []byte("\t"), []Key{{Type: KeyTab}}},
		{"ctrl-a", []byte("\x01"), []Key{{Type: KeyCtrlA}}},
		{"ctrl-c", []byte("\x03"), []Key{{Type: KeyCtrlC}}},
		{"ctrl-d", []byte("\x04"), []Key{{Type: KeyCtrlD}}},
		{"ctrl-e", []byte("\x05"), []Key{{Type: KeyCtrlE}}},
		{"ctrl-k", []byte("\x0b"), []Key{{Type: KeyCtrlK}}},
		{"ctrl-l", []byte("\x0c"), []Key{{Type: KeyCtrlL}}},
		{"ctrl-u", []byte("\x15"), []Key{{Type: KeyCtrlU}}},
		{"ctrl-w", []byte("\x17"), []Key{{Type: KeyCtrlW}}},
		{"utf8 2byte", []byte("é"), []Key{{Type: KeyRune, Rune: 'é'}}},
		{"utf8 3byte", []byte("世"), []Key{{Type: KeyRune, Rune: '世'}}},
		{"utf8 4byte", []byte("😀"), []Key{{Type: KeyRune, Rune: '😀'}}},
		{"csi up", []byte("\x1b[A"), []Key{{Type: KeyUp}}},
		{"csi down", []byte("\x1b[B"), []Key{{Type: KeyDown}}},
		{"csi right", []byte("\x1b[C"), []Key{{Type: KeyRight}}},
		{"csi left", []byte("\x1b[D"), []Key{{Type: KeyLeft}}},
		{"ss3 up", []byte("\x1bOA"), []Key{{Type: KeyUp}}},
		{"home letter", []byte("\x1b[H"), []Key{{Type: KeyHome}}},
		{"end letter", []byte("\x1b[F"), []Key{{Type: KeyEnd}}},
		{"home tilde", []byte("\x1b[1~"), []Key{{Type: KeyHome}}},
		{"end tilde", []byte("\x1b[4~"), []Key{{Type: KeyEnd}}},
		{"delete tilde", []byte("\x1b[3~"), []Key{{Type: KeyDelete}}},
		{"shift-tab", []byte("\x1b[Z"), []Key{{Type: KeyShiftTab}}},
		{"modified arrow", []byte("\x1b[1;2A"), []Key{{Type: KeyUp}}},
		{"lone esc", []byte("\x1b"), []Key{{Type: KeyEsc}}},
		{"esc then x", []byte("\x1bx"), []Key{{Type: KeyEsc}, {Type: KeyRune, Rune: 'x'}}},
		{"pageup unknown", []byte("\x1b[5~"), []Key{{Type: KeyUnknown}}},
		{"runaway params", []byte("\x1b[1234567890123456789~"), []Key{{Type: KeyUnknown}}},
		{"truncated utf8", []byte{0xC3}, []Key{{Type: KeyRune, Rune: utf8.RuneError}}},
		{"rune after arrow", []byte("\x1b[Cx"), []Key{{Type: KeyRight}, {Type: KeyRune, Rune: 'x'}}},
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

func TestDecoderPaste(t *testing.T) {
	in := "\x1b[200~line one\r\nline two\x1b[201~"
	keys := keysOf(t, bytes.NewReader([]byte(in)))
	if len(keys) != 1 || keys[0].Type != KeyPaste || keys[0].Text != "line one\nline two" {
		t.Fatalf("paste decode = %+v", keys)
	}
	// A paste followed by Enter: two events, the paste then the submit.
	keys = keysOf(t, bytes.NewReader([]byte("\x1b[200~hi\x1b[201~\r")))
	if len(keys) != 2 || keys[0].Type != KeyPaste || keys[0].Text != "hi" || keys[1].Type != KeyEnter {
		t.Fatalf("paste+enter = %+v", keys)
	}
}

func TestDecoderCleanEOF(t *testing.T) {
	if _, err := NewDecoder(bytes.NewReader(nil)).ReadKey(); err != io.EOF {
		t.Fatalf("empty stream should return io.EOF, got %v", err)
	}
}
