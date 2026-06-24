// Package keydec decodes a raw keystroke byte stream into typed key events. It
// is platform-agnostic: with VT input enabled on Windows (see the rawmode
// package), arrow and edit keys arrive as the same ESC-[ sequences as on Unix,
// so one decoder serves every OS. It reads byte-by-byte and never relies on
// look-ahead, so it works against any io.Reader — a live terminal, or a reader
// that delivers a single byte per Read in tests — and a malformed/truncated
// sequence degrades to KeyEsc/KeyUnknown/RuneError rather than hanging.
package keydec

import (
	"bufio"
	"bytes"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// KeyType enumerates the editor-relevant keys the decoder emits.
type KeyType int

const (
	KeyRune      KeyType = iota // a printable rune (value in Key.Rune)
	KeyEnter                    // CR (0x0d) or LF (0x0a)
	KeyBackspace                // DEL (0x7f) or BS (0x08 / Ctrl-H)
	KeyTab                      // 0x09 / Ctrl-I
	KeyShiftTab                 // back-tab (ESC[Z)
	KeyEsc                      // a bare Escape (lone ESC, or ESC + non-introducer)
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyDelete // forward delete (ESC[3~)
	KeyCtrlA
	KeyCtrlB
	KeyCtrlC
	KeyCtrlD
	KeyCtrlE
	KeyCtrlF
	KeyCtrlK
	KeyCtrlL
	KeyCtrlN
	KeyCtrlP
	KeyCtrlU
	KeyCtrlW
	KeyPaste   // a bracketed paste; the whole pasted text is in Key.Text
	KeyMouse   // an SGR mouse event (button/coords in the Mouse* fields)
	KeyUnknown // a recognized-but-unhandled escape sequence (fully consumed)
)

// SGR mouse button codes we care about (low bits = button; the wheel uses the
// 64 bit). Coordinates in a KeyMouse are 1-based, as the terminal reports them.
const (
	MouseLeft      = 0
	MouseMiddle    = 1
	MouseRight     = 2
	MouseWheelUp   = 64
	MouseWheelDown = 65
)

// Key is a single decoded keystroke (or mouse event).
type Key struct {
	Type KeyType
	Rune rune   // set only when Type == KeyRune
	Text string // set only when Type == KeyPaste (the pasted block, newlines normalized)

	// Mouse* are set only when Type == KeyMouse.
	MouseButton int  // raw SGR button code (see Mouse* consts)
	MouseX      int  // 1-based column
	MouseY      int  // 1-based row
	MousePress  bool // true on press ('M'), false on release ('m')
}

// ctrlKeys maps a control byte to its key type. Bytes handled specially in
// ReadKey (Tab 0x09, Enter 0x0a/0x0d, Backspace 0x08, Esc 0x1b) are absent.
var ctrlKeys = map[byte]KeyType{
	0x01: KeyCtrlA, 0x02: KeyCtrlB, 0x03: KeyCtrlC, 0x04: KeyCtrlD,
	0x05: KeyCtrlE, 0x06: KeyCtrlF, 0x0b: KeyCtrlK, 0x0c: KeyCtrlL,
	0x0e: KeyCtrlN, 0x10: KeyCtrlP, 0x15: KeyCtrlU, 0x17: KeyCtrlW,
}

// Decoder reads keys from an underlying byte stream.
type Decoder struct{ r *bufio.Reader }

// NewDecoder wraps r. The bufio layer makes ReadByte/UnreadByte cheap and lets
// the decoder push back a single byte (a lone ESC's successor, a bad UTF-8
// continuation) without look-ahead.
func NewDecoder(r io.Reader) *Decoder { return &Decoder{r: bufio.NewReader(r)} }

// ReadKey reads and returns the next key, or an error (io.EOF at a clean end).
func (d *Decoder) ReadKey() (Key, error) {
	b, err := d.r.ReadByte()
	if err != nil {
		return Key{}, err
	}
	switch {
	case b == 0x0d || b == 0x0a:
		return Key{Type: KeyEnter}, nil
	case b == 0x7f || b == 0x08:
		return Key{Type: KeyBackspace}, nil
	case b == 0x09:
		return Key{Type: KeyTab}, nil
	case b == 0x1b:
		return d.readEscape()
	case b < 0x20:
		if kt, ok := ctrlKeys[b]; ok {
			return Key{Type: kt}, nil
		}
		return Key{Type: KeyUnknown}, nil
	case b < 0x80:
		return Key{Type: KeyRune, Rune: rune(b)}, nil
	default:
		return d.readUTF8(b)
	}
}

// readUTF8 accumulates the continuation bytes of a multibyte rune whose lead
// byte is first. A truncated or malformed sequence yields RuneError (never a
// hang); a stray non-continuation byte is pushed back for the next ReadKey.
func (d *Decoder) readUTF8(first byte) (Key, error) {
	var n int
	switch {
	case first&0xE0 == 0xC0:
		n = 2
	case first&0xF0 == 0xE0:
		n = 3
	case first&0xF8 == 0xF0:
		n = 4
	default:
		return Key{Type: KeyRune, Rune: utf8.RuneError}, nil
	}
	buf := make([]byte, 1, 4)
	buf[0] = first
	for i := 1; i < n; i++ {
		c, err := d.r.ReadByte()
		if err != nil {
			return Key{Type: KeyRune, Rune: utf8.RuneError}, nil // truncated at EOF
		}
		if c&0xC0 != 0x80 {
			_ = d.r.UnreadByte() // not a continuation byte
			return Key{Type: KeyRune, Rune: utf8.RuneError}, nil
		}
		buf = append(buf, c)
	}
	r, _ := utf8.DecodeRune(buf)
	return Key{Type: KeyRune, Rune: r}, nil
}

// readEscape handles the byte stream after a leading ESC: a CSI ("[") or SS3
// ("O") introducer, or a bare ESC (lone, or ESC followed by an unrelated byte
// which is pushed back).
func (d *Decoder) readEscape() (Key, error) {
	b, err := d.r.ReadByte()
	if err != nil {
		return Key{Type: KeyEsc}, nil // lone ESC at EOF
	}
	switch b {
	case '[':
		return d.readCSI()
	case 'O': // SS3 (application cursor keys)
		c, err := d.r.ReadByte()
		if err != nil {
			return Key{Type: KeyEsc}, nil
		}
		return finalKey(c), nil
	default:
		// ESC + an unrelated byte is an Alt/Meta chord (e.g. Alt-B). We don't bind
		// these, so CONSUME the successor and return an inert ESC. (Previously we
		// UnreadByte'd it, so the byte resurfaced as a plain rune and got inserted
		// into the buffer — "Alt-B" typed a stray "b".)
		return Key{Type: KeyEsc}, nil
	}
}

// readCSI reads a CSI sequence after "ESC [": optional parameter bytes
// (0x30-0x3f) then a final byte (0x40-0x7e). A truncated sequence is a bare ESC;
// a runaway parameter run is consumed as KeyUnknown.
func (d *Decoder) readCSI() (Key, error) {
	var params []byte
	overflow := false
	for {
		b, err := d.r.ReadByte()
		if err != nil {
			return Key{Type: KeyEsc}, nil
		}
		if b >= 0x30 && b <= 0x3f { // parameter byte
			if len(params) < 16 {
				params = append(params, b)
			} else {
				overflow = true // stop storing but keep consuming to the final byte
			}
			continue
		}
		if string(params) == "200" && b == '~' { // bracketed-paste start
			return d.readPaste()
		}
		if len(params) > 0 && params[0] == '<' && (b == 'M' || b == 'm') { // SGR mouse
			return mouseKey(params, b), nil
		}
		if overflow { // runaway parameters: consumed, but unclassifiable
			return Key{Type: KeyUnknown}, nil
		}
		return classifyCSI(params, b), nil
	}
}

// maxPaste bounds a bracketed-paste body so a dropped/garbled ESC[201~
// terminator can't loop forever on a live TTY (which never reaches EOF), and a
// pathological giant paste can't exhaust memory. 1 MiB is far more than any real
// prompt paste.
const maxPaste = 1 << 20

// mouseKey parses an SGR mouse sequence: params is "<btn;x;y" and final is 'M'
// (press) or 'm' (release). A malformed sequence degrades to KeyUnknown.
func mouseKey(params []byte, final byte) Key {
	parts := strings.Split(strings.TrimPrefix(string(params), "<"), ";")
	if len(parts) != 3 {
		return Key{Type: KeyUnknown}
	}
	btn, e1 := strconv.Atoi(parts[0])
	x, e2 := strconv.Atoi(parts[1])
	y, e3 := strconv.Atoi(parts[2])
	if e1 != nil || e2 != nil || e3 != nil {
		return Key{Type: KeyUnknown}
	}
	return Key{Type: KeyMouse, MouseButton: btn, MouseX: x, MouseY: y, MousePress: final == 'M'}
}

// readPaste reads a bracketed paste body up to the ESC[201~ terminator and
// returns it as one KeyPaste, normalizing CRLF/CR to LF. It always returns:
// on EOF, on the terminator, or once maxPaste bytes are read (so it never hangs
// even if the terminator never arrives). A []byte accumulator keeps the
// suffix check O(1) per byte instead of re-stringifying the whole buffer.
func (d *Decoder) readPaste() (Key, error) {
	end := []byte("\x1b[201~")
	var b []byte
	for {
		c, err := d.r.ReadByte()
		if err != nil {
			break
		}
		b = append(b, c)
		if bytes.HasSuffix(b, end) {
			return Key{Type: KeyPaste, Text: normalizeNewlines(string(b[:len(b)-len(end)]))}, nil
		}
		if len(b) >= maxPaste {
			break // unterminated/oversized: recover with what we have
		}
	}
	return Key{Type: KeyPaste, Text: normalizeNewlines(string(b))}, nil
}

func normalizeNewlines(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

// classifyCSI maps a CSI final byte (plus the leading numeric parameter for the
// "~" family) to a key. Modifiers (e.g. "1;2A") are ignored — the base key wins.
func classifyCSI(params []byte, final byte) Key {
	if k := finalKey(final); k.Type != KeyUnknown {
		return k
	}
	switch final {
	case 'Z':
		return Key{Type: KeyShiftTab}
	case '~':
		switch leadingNum(params) {
		case 1, 7:
			return Key{Type: KeyHome}
		case 4, 8:
			return Key{Type: KeyEnd}
		case 3:
			return Key{Type: KeyDelete}
		}
	}
	return Key{Type: KeyUnknown}
}

// finalKey maps a cursor-key final byte (shared by CSI and SS3 forms) to a key.
func finalKey(final byte) Key {
	switch final {
	case 'A':
		return Key{Type: KeyUp}
	case 'B':
		return Key{Type: KeyDown}
	case 'C':
		return Key{Type: KeyRight}
	case 'D':
		return Key{Type: KeyLeft}
	case 'H':
		return Key{Type: KeyHome}
	case 'F':
		return Key{Type: KeyEnd}
	}
	return Key{Type: KeyUnknown}
}

// leadingNum parses the leading decimal parameter (before any ';'), or -1.
func leadingNum(params []byte) int {
	n, got := 0, false
	for _, c := range params {
		if c < '0' || c > '9' {
			break
		}
		n, got = n*10+int(c-'0'), true
	}
	if !got {
		return -1
	}
	return n
}
