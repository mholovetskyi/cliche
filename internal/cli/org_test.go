package cli

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mholovetskyi/cliche/internal/orgpolicy"
	"github.com/mholovetskyi/cliche/internal/secrets"
)

func pinnedKey(pub ed25519.PublicKey) string {
	return "ed25519:" + base64.StdEncoding.EncodeToString(pub)
}

// The happy path: a signed policy served behind a bearer token is fetched,
// verified against the pinned key, and returned.
func TestOrgPolicyFetchVerify(t *testing.T) {
	t.Setenv("CLICHE_CONFIG_HOME", t.TempDir())
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	doc, _ := orgpolicy.Sign(orgpolicy.Policy{Deny: []string{"curl"}, MaxUSD: 2, ForbidYolo: true}, priv)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write(doc)
	}))
	defer srv.Close()

	if err := secrets.SaveOrg(secrets.OrgConfig{URL: srv.URL, Token: "tok123", Key: pinnedKey(pub)}); err != nil {
		t.Fatal(err)
	}
	pol, ok, err := orgPolicy()
	if err != nil || !ok {
		t.Fatalf("orgPolicy: ok=%v err=%v", ok, err)
	}
	if len(pol.Deny) != 1 || pol.Deny[0] != "curl" || pol.MaxUSD != 2 || !pol.ForbidYolo {
		t.Fatalf("policy not loaded faithfully: %+v", pol)
	}
}

// A policy signed by a key OTHER than the pinned one must be rejected — a rogue
// control plane can't forge a policy the kernel will accept.
func TestOrgPolicyRejectsUnpinnedSigner(t *testing.T) {
	t.Setenv("CLICHE_CONFIG_HOME", t.TempDir())
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	_, rogue, _ := ed25519.GenerateKey(rand.Reader)
	doc, _ := orgpolicy.Sign(orgpolicy.Policy{Deny: []string{"curl"}}, rogue)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(doc) }))
	defer srv.Close()

	_ = secrets.SaveOrg(secrets.OrgConfig{URL: srv.URL, Token: "t", Key: pinnedKey(pub)})
	if _, _, err := orgPolicy(); err == nil {
		t.Fatal("a policy signed by an unpinned key must fail closed")
	}
}

// An inactive subscription (402) fails closed, not open.
func TestOrgPolicyInactiveSubscription(t *testing.T) {
	t.Setenv("CLICHE_CONFIG_HOME", t.TempDir())
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
	}))
	defer srv.Close()
	_ = secrets.SaveOrg(secrets.OrgConfig{URL: srv.URL, Token: "t", Key: pinnedKey(pub)})
	if _, _, err := orgPolicy(); err == nil {
		t.Fatal("an inactive subscription (402) must fail closed")
	}
}

// No org configured → a no-op (the free CLI never phones home).
func TestOrgPolicyNotConfigured(t *testing.T) {
	t.Setenv("CLICHE_CONFIG_HOME", t.TempDir())
	pol, ok, err := orgPolicy()
	if ok || err != nil {
		t.Fatalf("unconfigured should be (zero,false,nil); got ok=%v err=%v pol=%+v", ok, err, pol)
	}
}
