package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func writeDiff(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "change.diff")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestVerifyExitCodes(t *testing.T) {
	dir := t.TempDir()
	clean := writeDiff(t, "--- a/x.go\n+++ b/x.go\n-x := 1\n+x := 2\n")
	hack := writeDiff(t, "--- a/x_test.go\n+++ b/x_test.go\n-func TestPay(t *testing.T) {\n-}\n")

	var out, errOut bytes.Buffer

	// Clean diff, static-only -> unverified -> exit 0.
	if code := cmdVerify([]string{"--dir", dir, "--diff", clean, "--no-tests"}, &out, &errOut); code != 0 {
		t.Fatalf("clean+no-tests should exit 0, got %d", code)
	}
	// Same, but --strict -> unverified -> exit 2.
	if code := cmdVerify([]string{"--dir", dir, "--diff", clean, "--no-tests", "--strict"}, &out, &errOut); code != 2 {
		t.Fatalf("clean+strict should exit 2, got %d", code)
	}
	// Deleted-test diff -> flagged -> exit 5.
	if code := cmdVerify([]string{"--dir", dir, "--diff", hack, "--no-tests"}, &out, &errOut); code != 5 {
		t.Fatalf("reward-hack diff should exit 5, got %d", code)
	}
}
