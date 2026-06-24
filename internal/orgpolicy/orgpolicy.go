// Package orgpolicy is the CLI side of Cliche's commercial control plane: a
// signed, org-defined policy that the Trust Kernel enforces on every run. Its
// defining property is that it can only TIGHTEN — applied to a looser local
// config it restricts; applied to an already-tighter one it is a no-op. It can
// never loosen a guardrail, and a policy carries no way to grant authority
// (only `deny`, which already wins over `allow` and over `--yolo`). A rogue or
// compromised control plane can therefore at worst over-restrict (a visible
// denial), never silently disarm a guardrail.
//
// The package is pure and network-free: Load verifies a signed document against
// a pinned ed25519 key (fail-closed), and Apply folds the policy into a
// config.Config. Fetching the document and wiring Apply into the agent build
// live in the CLI layer. See docs/control-plane.md.
package orgpolicy

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mholovetskyi/cliche/internal/config"
)

// Policy is the org-defined document. Every field can only restrict. Zero
// values mean "the org does not constrain this axis" (the local config stands).
type Policy struct {
	Deny                []string `json:"deny,omitempty"`                  // tools to forbid (union with local)
	EgressAllow         []string `json:"egress_allow,omitempty"`          // network host allowlist (intersect with local)
	MaxUSD              float64  `json:"max_usd,omitempty"`               // dollar cap ceiling (min with local)
	MaxTokens           int      `json:"max_tokens,omitempty"`            // token cap ceiling (min with local)
	MaxTurns            int      `json:"max_turns,omitempty"`             // governor turn ceiling (min with local)
	MaxWallClockSeconds int      `json:"max_wallclock_seconds,omitempty"` // governor wall-clock ceiling (min with local)
	// ForbidYolo and ForceSandbox are flag-level, not config fields, so they are
	// surfaced here for the CLI to enforce at the flag layer (step 2), not by Apply.
	ForbidYolo   bool `json:"forbid_yolo,omitempty"`   // reject --yolo (approvals always required)
	ForceSandbox bool `json:"force_sandbox,omitempty"` // force the OS sandbox on
}

// Signed wraps a policy document with a detached ed25519 signature over the
// exact policy bytes (so verification never depends on re-marshaling).
type Signed struct {
	Policy json.RawMessage `json:"policy"`
	Sig    string          `json:"sig"` // base64 ed25519 signature over the Policy bytes
}

// ErrUnsigned is returned when a configured policy fails signature verification
// or can't be parsed. Callers MUST fail closed on it — a governance tool must
// not run unpoliced when a policy was expected.
var ErrUnsigned = errors.New("org policy signature does not verify")

// ParseKey reads a pinned public key in "ed25519:<base64>" (or bare base64) form.
func ParseKey(s string) (ed25519.PublicKey, error) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "ed25519:"))
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("org key: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("org key: want %d bytes, got %d", ed25519.PublicKeySize, len(raw))
	}
	return ed25519.PublicKey(raw), nil
}

// Load parses a signed policy document and verifies it against the pinned key.
// It fails closed: any parse or signature error returns an error and a zero
// Policy, so a caller that refuses to run on error can never proceed unpoliced.
func Load(data []byte, pub ed25519.PublicKey) (Policy, error) {
	if len(pub) != ed25519.PublicKeySize {
		return Policy{}, ErrUnsigned // never call ed25519.Verify with a bad key (it panics)
	}
	var s Signed
	if err := json.Unmarshal(data, &s); err != nil {
		return Policy{}, fmt.Errorf("org policy: %w", err)
	}
	sig, err := base64.StdEncoding.DecodeString(s.Sig)
	if err != nil || len(s.Policy) == 0 || !ed25519.Verify(pub, s.Policy, sig) {
		return Policy{}, ErrUnsigned
	}
	var p Policy
	if err := json.Unmarshal(s.Policy, &p); err != nil {
		return Policy{}, fmt.Errorf("org policy: %w", err)
	}
	return p, nil
}

// Sign produces a signed document for a policy (for admins/tests that mint a
// policy with the org private key).
func Sign(p Policy, priv ed25519.PrivateKey) ([]byte, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	sig := ed25519.Sign(priv, raw)
	return json.Marshal(Signed{Policy: raw, Sig: base64.StdEncoding.EncodeToString(sig)})
}

// Apply folds a policy into cfg, tightening only. It touches the four
// config-level guardrails; ForbidYolo/ForceSandbox are enforced by the caller at
// the flag layer. The result still satisfies config.Validate() for any valid
// input, because Apply only lowers a positive cap or adds one — it never zeroes
// a cap and never raises MaxTurns to a disarming value.
func Apply(cfg config.Config, p Policy) config.Config {
	// Deny: union. More denies is tighter, and deny wins over allow and --yolo.
	cfg.Permissions.Deny = unionDedup(cfg.Permissions.Deny, p.Deny)

	// Egress: a non-empty allowlist restricts which hosts web_fetch may reach.
	// An empty LOCAL list means "all hosts", so the org's list wins outright;
	// two lists intersect (only hosts BOTH permit remain reachable).
	// Egress restricts only if the org names specific hosts. An org list of "*"
	// permits all → no restriction (keep local).
	if len(p.EgressAllow) > 0 && !contains(p.EgressAllow, "*") {
		switch {
		case egressIsAll(cfg.Egress.Allow):
			// Local permits everything → org's list becomes the boundary.
			cfg.Egress.Allow = append([]string(nil), p.EgressAllow...)
		default:
			merged := intersect(cfg.Egress.Allow, p.EgressAllow)
			if len(merged) == 0 {
				// Disjoint allowlists ⇒ the agent may reach nothing. An EMPTY list
				// would mean "allow all" (the worst possible loosen), so encode
				// deny-all explicitly with an unmatchable sentinel.
				merged = []string{egressDenyAll}
			}
			cfg.Egress.Allow = merged
		}
	}

	// Caps: lower is tighter; 0 means "no cap on this axis" (treated as +inf).
	cfg.Budget.MaxUSD = minPositiveF(cfg.Budget.MaxUSD, p.MaxUSD)
	cfg.Budget.MaxTokens = minPositiveI(cfg.Budget.MaxTokens, p.MaxTokens)
	cfg.Governor.MaxTurns = minPositiveI(cfg.Governor.MaxTurns, p.MaxTurns)
	cfg.Governor.MaxWallClockSeconds = minPositiveI(cfg.Governor.MaxWallClockSeconds, p.MaxWallClockSeconds)
	return cfg
}

// minPositiveI returns the tighter of two ceilings where <= 0 means "no cap".
// It never raises a positive cap and never turns a positive cap into "no cap".
func minPositiveI(local, org int) int {
	switch {
	case local <= 0:
		return org // local uncapped → adopt org's (possibly still 0 = uncapped)
	case org <= 0:
		return local // org doesn't constrain → keep local
	case org < local:
		return org
	default:
		return local
	}
}

func minPositiveF(local, org float64) float64 {
	switch {
	case local <= 0:
		return org
	case org <= 0:
		return local
	case org < local:
		return org
	default:
		return local
	}
}

func unionDedup(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range append(append([]string(nil), a...), b...) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// egressDenyAll is a sentinel allowlist entry that matches no real host (a host
// can't contain a null byte), used to represent "deny all egress" when two
// allowlists are disjoint — a result an empty list (= allow all) can't express.
// Note: pattern intersection is by exact equality, so overlapping wildcards
// (e.g. local "*.openai.com" vs org "api.openai.com") collapse to deny-all
// rather than the narrower host. That over-restricts, never loosens — the safe
// direction for a guardrail.
const egressDenyAll = "\x00(deny-all)"

// egressIsAll reports whether a local allowlist permits every host — either
// unconfigured (empty) or an explicit "*".
func egressIsAll(allow []string) bool { return len(allow) == 0 || contains(allow, "*") }

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// intersect returns the elements present in both lists (order follows a).
func intersect(a, b []string) []string {
	inB := make(map[string]bool, len(b))
	for _, s := range b {
		inB[s] = true
	}
	out := make([]string, 0, len(a))
	for _, s := range a {
		if inB[s] {
			out = append(out, s)
		}
	}
	return out
}
