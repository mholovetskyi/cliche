package orgpolicy

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/tools"
)

func TestSignLoadRoundTrip(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	want := Policy{Deny: []string{"curl"}, MaxUSD: 2, ForbidYolo: true, ForceSandbox: true}

	doc, err := Sign(want, priv)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Load(doc, pub)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Deny) != 1 || got.Deny[0] != "curl" || got.MaxUSD != 2 || !got.ForbidYolo || !got.ForceSandbox {
		t.Fatalf("round-trip lost fields: %+v", got)
	}
}

func TestLoadFailsClosed(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	doc, _ := Sign(Policy{Deny: []string{"curl"}}, priv)

	// Wrong key → rejected.
	if _, err := Load(doc, otherPub); err == nil {
		t.Fatal("a policy signed by another key must be rejected")
	}
	// Tampered body → rejected (signature is over the original bytes).
	tampered := append([]byte(nil), doc...)
	for i, b := range tampered {
		if b == 'c' { // flip the 'c' in "curl"
			tampered[i] = 'd'
			break
		}
	}
	if _, err := Load(tampered, pub); err == nil {
		t.Fatal("a tampered policy body must be rejected")
	}
	// Garbage → rejected, not panicked.
	if _, err := Load([]byte("not json"), pub); err == nil {
		t.Fatal("garbage must be rejected")
	}
}

func TestApplyTightens(t *testing.T) {
	cfg := config.Default()
	cfg.Permissions.Deny = []string{"rm"}
	cfg.Egress.Allow = []string{"github.com", "api.openai.com"}
	cfg.Budget = config.Budget{MaxTokens: 2_000_000, MaxUSD: 5}
	cfg.Governor.MaxTurns = 50
	cfg.Governor.MaxWallClockSeconds = 1800

	m := Apply(cfg, Policy{
		Deny:                []string{"curl"},
		EgressAllow:         []string{"github.com"}, // drops api.openai.com
		MaxUSD:              2,
		MaxTokens:           1_000_000,
		MaxTurns:            20,
		MaxWallClockSeconds: 600,
	})

	if !has(m.Permissions.Deny, "rm") || !has(m.Permissions.Deny, "curl") {
		t.Fatalf("deny should be the union: %v", m.Permissions.Deny)
	}
	if len(m.Egress.Allow) != 1 || m.Egress.Allow[0] != "github.com" {
		t.Fatalf("egress should intersect to github.com: %v", m.Egress.Allow)
	}
	if m.Budget.MaxUSD != 2 || m.Budget.MaxTokens != 1_000_000 || m.Governor.MaxTurns != 20 || m.Governor.MaxWallClockSeconds != 600 {
		t.Fatalf("caps should drop to the org ceilings: %+v / %+v", m.Budget, m.Governor)
	}
}

func TestApplyEgressEdgeCases(t *testing.T) {
	// Local "*" (explicit allow-all) + org list → org list becomes the boundary.
	m := Apply(tighten(config.Default(), nil, []string{"*"}, 1, 0, 10, 0),
		Policy{EgressAllow: []string{"github.com"}})
	if len(m.Egress.Allow) != 1 || m.Egress.Allow[0] != "github.com" {
		t.Fatalf("local * + org list should yield the org list: %v", m.Egress.Allow)
	}

	// Org "*" (allow all) does not restrict → keep local.
	m = Apply(tighten(config.Default(), nil, []string{"only.example"}, 1, 0, 10, 0),
		Policy{EgressAllow: []string{"*"}})
	if len(m.Egress.Allow) != 1 || m.Egress.Allow[0] != "only.example" {
		t.Fatalf("org * should not restrict: %v", m.Egress.Allow)
	}

	// Disjoint lists → deny-all, and the sentinel blocks every real host.
	m = Apply(tighten(config.Default(), nil, []string{"only.example"}, 1, 0, 10, 0),
		Policy{EgressAllow: []string{"github.com"}})
	if tools.ParseEgress(m.Egress.Allow).Allowed("github.com") || tools.ParseEgress(m.Egress.Allow).Allowed("only.example") {
		t.Fatalf("disjoint lists must deny every host: %v", m.Egress.Allow)
	}
}

func TestLoadRejectsBadKeyWithoutPanic(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	doc, _ := Sign(Policy{Deny: []string{"curl"}}, priv)
	// A wrong-size key must fail closed, never panic ed25519.Verify.
	if _, err := Load(doc, ed25519.PublicKey{1, 2, 3}); err == nil {
		t.Fatal("a malformed key must be rejected")
	}
}

// The security-critical property: across an adversarial matrix of configs and
// policies — including policies that TRY to loosen (higher caps, extra egress
// hosts) — Apply never makes the effective config looser than local, and the
// result always still validates.
func TestApplyNeverLoosens(t *testing.T) {
	probes := []string{"github.com", "api.openai.com", "evil.test", "example.com"}

	configs := []config.Config{
		tighten(config.Default(), []string{"rm"}, []string{"github.com", "api.openai.com"}, 5, 2_000_000, 50, 1800),
		tighten(config.Default(), nil, nil, 0, 1_000_000, 30, 0),                            // no $ cap, no egress list (=all), no wall clock
		tighten(config.Default(), []string{"wget"}, []string{"only.example"}, 1, 0, 10, 60), // no token cap
	}
	policies := []Policy{
		{}, // no-op
		{Deny: []string{"x"}, EgressAllow: []string{"github.com"}, MaxUSD: 1, MaxTokens: 500_000, MaxTurns: 5, MaxWallClockSeconds: 30},
		// Adversarial: tries to RAISE every ceiling and ADD egress hosts.
		{EgressAllow: []string{"github.com", "evil.test", "newhost.test"}, MaxUSD: 999, MaxTokens: 9_000_000, MaxTurns: 9999, MaxWallClockSeconds: 99999},
	}

	for ci, cfg := range configs {
		if err := cfg.Validate(); err != nil {
			t.Fatalf("config[%d] invalid before Apply: %v", ci, err)
		}
		for pi, p := range policies {
			m := Apply(cfg, p)

			// 1. Deny only grows: every local deny survives.
			for _, d := range cfg.Permissions.Deny {
				if !has(m.Permissions.Deny, d) {
					t.Fatalf("cfg[%d]/pol[%d]: deny %q dropped", ci, pi, d)
				}
			}
			// 2. Egress never widens: any host reachable after must have been reachable before.
			before := tools.ParseEgress(cfg.Egress.Allow)
			after := tools.ParseEgress(m.Egress.Allow)
			for _, h := range probes {
				if after.Allowed(h) && !before.Allowed(h) {
					t.Fatalf("cfg[%d]/pol[%d]: egress widened — %q newly reachable", ci, pi, h)
				}
			}
			// 3. Caps never rise, and a set cap is never removed.
			if !capOK(cfg.Budget.MaxUSD, m.Budget.MaxUSD) {
				t.Fatalf("cfg[%d]/pol[%d]: max_usd loosened %v→%v", ci, pi, cfg.Budget.MaxUSD, m.Budget.MaxUSD)
			}
			if !capOK(float64(cfg.Budget.MaxTokens), float64(m.Budget.MaxTokens)) {
				t.Fatalf("cfg[%d]/pol[%d]: max_tokens loosened %d→%d", ci, pi, cfg.Budget.MaxTokens, m.Budget.MaxTokens)
			}
			if !capOK(float64(cfg.Governor.MaxTurns), float64(m.Governor.MaxTurns)) {
				t.Fatalf("cfg[%d]/pol[%d]: max_turns loosened %d→%d", ci, pi, cfg.Governor.MaxTurns, m.Governor.MaxTurns)
			}
			if !capOK(float64(cfg.Governor.MaxWallClockSeconds), float64(m.Governor.MaxWallClockSeconds)) {
				t.Fatalf("cfg[%d]/pol[%d]: max_wallclock loosened %d→%d", ci, pi, cfg.Governor.MaxWallClockSeconds, m.Governor.MaxWallClockSeconds)
			}
			// 4. The tightened config still passes validation (no disarmed guardrail).
			if err := m.Validate(); err != nil {
				t.Fatalf("cfg[%d]/pol[%d]: result fails Validate: %v", ci, pi, err)
			}
		}
	}
}

// capOK reports whether the after-cap is no looser than before, where 0 means
// "no cap" (looser than any positive cap).
func capOK(before, after float64) bool {
	if before > 0 { // had a cap: must keep a cap, no higher
		return after > 0 && after <= before
	}
	return true // no cap before: any value (cap added or still none) is >= as tight
}

func tighten(c config.Config, deny, egress []string, usd float64, tokens, turns, wall int) config.Config {
	c.Permissions.Deny = deny
	c.Egress.Allow = egress
	c.Budget = config.Budget{MaxUSD: usd, MaxTokens: tokens}
	c.Governor.MaxTurns = turns
	c.Governor.MaxWallClockSeconds = wall
	return c
}

func has(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
