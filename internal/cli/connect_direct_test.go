package cli

import (
	"bytes"
	"testing"

	"github.com/mholovetskyi/cliche/internal/secrets"
)

// The seamless path: when a GitHub token already exists (here via env, the same
// resolver that also reads `gh auth token`), `connect github` succeeds with no
// OAuth app, no client id, and no browser.
func TestConnectUsesExistingGitHubToken(t *testing.T) {
	t.Setenv("CLICHE_CONFIG_HOME", t.TempDir())
	t.Setenv("GITHUB_PERSONAL_ACCESS_TOKEN", "ghp_seamless")
	// Ensure no BYO OAuth app is configured, so success can only come from the
	// direct-token path.
	t.Setenv("CLICHE_GITHUB_CLIENT_ID", "")

	var out, errOut bytes.Buffer
	if code := cmdConnect([]string{"github"}, &out, &errOut); code != 0 {
		t.Fatalf("connect github should succeed via existing token; code=%d err=%q", code, errOut.String())
	}
	tok, ok := secrets.Connector("github")
	if !ok || tok.Token != "ghp_seamless" {
		t.Fatalf("token not saved from direct path: %+v ok=%v", tok, ok)
	}
}

// With no token anywhere and no OAuth app, the failure message points at the
// easy `gh auth login` path first.
func TestConnectNoTokenSuggestsGh(t *testing.T) {
	t.Setenv("CLICHE_CONFIG_HOME", t.TempDir())
	t.Setenv("GITHUB_PERSONAL_ACCESS_TOKEN", "")
	t.Setenv("CLICHE_GITHUB_CLIENT_ID", "")
	// PATH stripped so `gh` can't be found → direct path yields nothing.
	t.Setenv("PATH", "")

	var out, errOut bytes.Buffer
	if code := cmdConnect([]string{"github"}, &out, &errOut); code == 0 {
		t.Fatal("connect github should fail with no token and no OAuth app")
	}
	if msg := errOut.String(); !contains(msg, "gh auth login") {
		t.Fatalf("failure should suggest `gh auth login`, got: %q", msg)
	}
}
