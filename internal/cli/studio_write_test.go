package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteProjectFileGuards(t *testing.T) {
	root := t.TempDir()

	// A normal in-root write (with a new subdir) succeeds and persists.
	if err := writeProjectFile(root, "src/App.tsx", "export default 1"); err != nil {
		t.Fatalf("in-root write should succeed: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(root, "src", "App.tsx"))
	if err != nil || string(b) != "export default 1" {
		t.Fatalf("content not persisted: %v %q", err, b)
	}

	// Traversal, absolute paths, and sensitive files are all rejected, and none
	// of them write anything.
	for _, rel := range []string{"../escape.txt", "..\\escape.txt", ".env", "secret.pem", "config/id_rsa"} {
		if err := writeProjectFile(root, rel, "x"); err == nil {
			t.Errorf("write to %q should be rejected", rel)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape.txt")); err == nil {
		t.Fatal("traversal write escaped the root")
	}

	// Oversize content is rejected.
	if err := writeProjectFile(root, "big.txt", string(make([]byte, 513*1024))); err == nil {
		t.Error("oversize write should be rejected")
	}
	// Empty path is rejected.
	if err := writeProjectFile(root, "", "x"); err == nil {
		t.Error("empty path should be rejected")
	}
}
