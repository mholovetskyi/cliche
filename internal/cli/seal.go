package cli

import (
	"crypto/ed25519"
	"fmt"
	"io"

	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/ledger"
	"github.com/mholovetskyi/cliche/internal/seal"
	"github.com/mholovetskyi/cliche/internal/secrets"
	"github.com/mholovetskyi/cliche/internal/style"
)

// sealLedgerDir signs the ledger's current chain head with the user's key,
// updating the seal sidecar. Best-effort — sealing is additive and must never
// block or fail real work. Called when a run/session finishes.
func sealLedgerDir(dir string) {
	priv, err := secrets.SigningKey()
	if err != nil {
		return
	}
	led, err := ledger.Open(config.Dir(dir))
	if err != nil {
		return
	}
	if head := led.Head(); head != "" {
		_ = seal.Write(config.Dir(dir), head, priv)
	}
}

// localSigningPub returns this machine's signing public key, or nil if unavailable.
func localSigningPub() ed25519.PublicKey {
	priv, err := secrets.SigningKey()
	if err != nil {
		return nil
	}
	return priv.Public().(ed25519.PublicKey)
}

// renderSeal reports the authenticity of the ledger seal. It returns false when
// the seal is forged or by a foreign key (a tamper signal worth a nonzero exit).
func renderSeal(out io.Writer, dir, head string) bool {
	s, ok, err := seal.Read(config.Dir(dir))
	if err != nil || !ok {
		fmt.Fprintln(out, "  "+style.Gray("unsealed · no signature yet (a completed run writes one)"))
		return true
	}
	switch seal.Verify(s, head, localSigningPub()) {
	case seal.StatusValid:
		fmt.Fprintf(out, "  %s %s\n", style.Green(gl("✓", "ok")),
			style.White("sealed by your key")+style.Gray(" · "+s.Fingerprint+" · "+s.SignedAt))
		return true
	case seal.StatusStale:
		fmt.Fprintf(out, "  %s %s\n", style.Gray("~"),
			style.Gray("seal is stale — the ledger advanced since it was signed; finish a run to re-seal"))
		return true
	case seal.StatusForeign:
		fmt.Fprintf(out, "  %s %s\n", style.Red(gl("✗", "x")),
			style.Red("sealed by a DIFFERENT key ("+s.Fingerprint+") — not this machine's"))
		return false
	default: // StatusInvalid
		fmt.Fprintf(out, "  %s %s\n", style.Red(gl("✗", "x")), style.Red("seal signature is invalid (forged or corrupt)"))
		return false
	}
}
