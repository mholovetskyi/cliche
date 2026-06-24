package secrets

import (
	"bytes"
	"crypto/ed25519"
	"testing"
)

func TestSigningKeyPersists(t *testing.T) {
	t.Setenv("CLICHE_CONFIG_HOME", t.TempDir())
	k1, err := SigningKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(k1) != ed25519.PrivateKeySize {
		t.Fatalf("bad key size %d", len(k1))
	}
	k2, err := SigningKey()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(k1, k2) {
		t.Fatal("SigningKey should return the same persisted key across calls")
	}
}
