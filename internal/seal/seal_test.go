package seal

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

func TestSealRoundTripAndVerify(t *testing.T) {
	dir := t.TempDir()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	pub := priv.Public().(ed25519.PublicKey)
	head := "abc123def"

	if err := Write(dir, head, priv); err != nil {
		t.Fatal(err)
	}
	s, ok, err := Read(dir)
	if err != nil || !ok {
		t.Fatalf("read: ok=%v err=%v", ok, err)
	}
	if st := Verify(s, head, pub); st != StatusValid {
		t.Fatalf("matching key + head should be Valid, got %v", st)
	}
	if st := Verify(s, "moved-on", pub); st != StatusStale {
		t.Fatalf("a moved head should be Stale, got %v", st)
	}
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	if st := Verify(s, head, other.Public().(ed25519.PublicKey)); st != StatusForeign {
		t.Fatalf("a different local key should be Foreign, got %v", st)
	}
	if st := Verify(s, head, nil); st != StatusValid {
		t.Fatalf("nil localPub should skip the key check (Valid), got %v", st)
	}
}

func TestSealDetectsForgery(t *testing.T) {
	dir := t.TempDir()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	if err := Write(dir, "head", priv); err != nil {
		t.Fatal(err)
	}
	// Corrupt signature.
	s, _, _ := Read(dir)
	s.Signature = "AAAA"
	if st := Verify(s, "head", nil); st != StatusInvalid {
		t.Fatalf("corrupt signature should be Invalid, got %v", st)
	}
	// Editing the sealed Head to match a tampered chain breaks the signature
	// (it was made over the original head).
	s2, _, _ := Read(dir)
	s2.Head = "tampered"
	if st := Verify(s2, "tampered", nil); st != StatusInvalid {
		t.Fatalf("a re-pointed head should be Invalid, got %v", st)
	}
}

func TestReadMissing(t *testing.T) {
	if _, ok, err := Read(t.TempDir()); ok || err != nil {
		t.Fatalf("missing seal should be ok=false, err=nil; got ok=%v err=%v", ok, err)
	}
}
