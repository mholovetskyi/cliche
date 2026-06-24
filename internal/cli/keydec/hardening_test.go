package keydec

import (
	"strings"
	"testing"
)

// infReader yields b forever (never EOF) — models a live TTY for the
// unterminated-paste case.
type infReader struct{ b byte }

func (r infReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.b
	}
	return len(p), nil
}

func TestAltChordDoesNotLeakRune(t *testing.T) {
	// "\x1bb" is Alt-B. It must decode to a single inert KeyEsc — NOT KeyEsc
	// followed by a literal 'b' (which the editor would insert into the buffer).
	d := NewDecoder(strings.NewReader("\x1bb"))
	k1, _ := d.ReadKey()
	if k1.Type != KeyEsc {
		t.Fatalf("first key should be KeyEsc, got %+v", k1)
	}
	k2, err := d.ReadKey()
	if err == nil && k2.Type == KeyRune {
		t.Fatalf("Alt successor byte leaked as a rune %q — it should have been consumed", k2.Rune)
	}
}

func TestUnterminatedPasteIsBounded(t *testing.T) {
	// Paste-start with no terminator on a never-EOF source must still return
	// (bounded by maxPaste) instead of hanging forever.
	d := NewDecoder(infReaderWithPrefix("\x1b[200~", 'x'))
	k, err := d.ReadKey()
	if err != nil {
		t.Fatal(err)
	}
	if k.Type != KeyPaste {
		t.Fatalf("expected KeyPaste, got %+v", k.Type)
	}
	if len(k.Text) == 0 || len(k.Text) > maxPaste {
		t.Fatalf("paste body should be bounded and non-empty, got len %d (cap %d)", len(k.Text), maxPaste)
	}
}

func infReaderWithPrefix(prefix string, fill byte) *prefixThenInf {
	return &prefixThenInf{prefix: []byte(prefix), fill: fill}
}

type prefixThenInf struct {
	prefix []byte
	off    int
	fill   byte
}

func (r *prefixThenInf) Read(p []byte) (int, error) {
	i := 0
	for i < len(p) && r.off < len(r.prefix) {
		p[i] = r.prefix[r.off]
		r.off++
		i++
	}
	for ; i < len(p); i++ {
		p[i] = r.fill
	}
	return len(p), nil
}
