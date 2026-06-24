// Package seal adds authenticity to the audit ledger on top of its
// tamper-evident hash chain. It signs the chain HEAD (the latest entry's hash,
// which transitively commits to the whole history) with the user's ed25519 key
// and stores the signature in a sidecar. The hash chain detects ALTERATION; the
// signature detects FORGERY — an attacker who rewrites the ledger consistently
// still can't re-seal it without the private key (which never leaves the user
// config dir). Pure stdlib.
package seal

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Seal is the signed commitment stored at <ledgerDir>/ledger.seal.json.
type Seal struct {
	Head        string `json:"head"`        // chain-head hash that was signed
	PublicKey   string `json:"public_key"`  // base64 ed25519 public key of the signer
	Fingerprint string `json:"fingerprint"` // short hash of the public key (human compare)
	Signature   string `json:"signature"`   // base64 ed25519 signature over Head
	SignedAt    string `json:"signed_at"`   // RFC3339 (informational; not signed)
}

func sidecar(ledgerDir string) string { return filepath.Join(ledgerDir, "ledger.seal.json") }

// Fingerprint is a short, human-comparable digest of a public key.
func Fingerprint(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:8])
}

// Write signs head with priv and writes the sidecar.
func Write(ledgerDir, head string, priv ed25519.PrivateKey) error {
	pub := priv.Public().(ed25519.PublicKey)
	s := Seal{
		Head:        head,
		PublicKey:   base64.StdEncoding.EncodeToString(pub),
		Fingerprint: Fingerprint(pub),
		Signature:   base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(head))),
		SignedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sidecar(ledgerDir), append(data, '\n'), 0o644)
}

// Read returns the sidecar seal; ok is false if none exists.
func Read(ledgerDir string) (Seal, bool, error) {
	data, err := os.ReadFile(sidecar(ledgerDir))
	if err != nil {
		if os.IsNotExist(err) {
			return Seal{}, false, nil
		}
		return Seal{}, false, err
	}
	var s Seal
	if err := json.Unmarshal(data, &s); err != nil {
		return Seal{}, false, err
	}
	return s, true, nil
}

// Status is the result of verifying a seal against the live chain head + key.
type Status int

const (
	StatusInvalid Status = iota // signature does not verify (forged/corrupt)
	StatusStale                 // valid signature, but signed an older head
	StatusForeign               // valid signature by a key that is NOT this machine's
	StatusValid                 // signed by this machine's key, matches the head
)

// Verify checks a seal. localPub may be nil (skips the "is it my key" check).
func Verify(s Seal, head string, localPub ed25519.PublicKey) Status {
	pub, err := base64.StdEncoding.DecodeString(s.PublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return StatusInvalid
	}
	sig, err := base64.StdEncoding.DecodeString(s.Signature)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(pub), []byte(s.Head), sig) {
		return StatusInvalid
	}
	if s.Head != head {
		return StatusStale
	}
	if localPub != nil && !bytes.Equal(pub, localPub) {
		return StatusForeign
	}
	return StatusValid
}
