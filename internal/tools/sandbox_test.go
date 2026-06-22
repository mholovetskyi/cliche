package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSandboxForcesConfinement(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	// AllowOutsideRoot would normally permit this read; Sandbox must override it.
	e := OSExecutor{Root: root, Policy: Policy{Yolo: true, AllowOutsideRoot: true, Sandbox: true}}
	if r := e.Execute(context.Background(), "read_file", map[string]string{"file": outside}); r.Success {
		t.Fatal("sandbox must confine to root even with --allow-outside-root")
	}
	// Without sandbox, allow-outside-root still works (no regression).
	e2 := OSExecutor{Root: root, Policy: Policy{Yolo: true, AllowOutsideRoot: true}}
	if r := e2.Execute(context.Background(), "read_file", map[string]string{"file": outside}); !r.Success {
		t.Fatalf("allow-outside-root should still permit the read when not sandboxed: %s", r.Output)
	}
}

func TestSandboxDeniesNetworkWithoutAllowlist(t *testing.T) {
	e := OSExecutor{Policy: Policy{Yolo: true, AllowWeb: true, Sandbox: true}}
	r := e.Execute(context.Background(), "web_fetch", map[string]string{"url": "https://example.com"})
	if r.Success || !strings.Contains(r.Output, "sandbox") {
		t.Fatalf("sandbox without an allowlist must block the fetch, got: %q", r.Output)
	}
}

func TestScrubbedEnvRemovesSecrets(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "sk-secret")
	t.Setenv("GITHUB_TOKEN", "ghp_secret")
	t.Setenv("CLICHE_CONFIG_HOME", "/tmp/x")
	t.Setenv("MY_SECRET_VALUE", "hunter2")
	t.Setenv("PATH_LIKE_NORMAL", "keepme")

	got := strings.Join(scrubbedEnv(), "\n")
	for _, leaked := range []string{"OPENROUTER_API_KEY=", "GITHUB_TOKEN=", "CLICHE_CONFIG_HOME=", "MY_SECRET_VALUE="} {
		if strings.Contains(got, leaked) {
			t.Errorf("scrubbed env must not contain %q", leaked)
		}
	}
	if !strings.Contains(got, "PATH_LIKE_NORMAL=keepme") {
		t.Error("scrubbed env should keep non-secret variables")
	}
}

func TestIsSecretEnvKey(t *testing.T) {
	for _, k := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "CLICHE_FOO", "AWS_SECRET_ACCESS_KEY", "GH_TOKEN"} {
		if !isSecretEnvKey(k) {
			t.Errorf("%q should be treated as secret", k)
		}
	}
	for _, k := range []string{"PATH", "HOME", "GOPATH", "LANG"} {
		if isSecretEnvKey(k) {
			t.Errorf("%q should NOT be treated as secret", k)
		}
	}
}
