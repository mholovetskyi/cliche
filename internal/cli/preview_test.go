package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFindPreviewEntry(t *testing.T) {
	// Empty project → no preview (UI shows an empty state, not a dir listing).
	empty := t.TempDir()
	if p, ok := findPreviewEntry(empty); ok || p != "" {
		t.Fatalf("empty dir: got (%q, %v), want (\"\", false)", p, ok)
	}

	// Root index.html → preview the root.
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "index.html"), []byte("<h1>root</h1>"), 0o644)
	if p, ok := findPreviewEntry(root); !ok || p != "" {
		t.Fatalf("root index: got (%q, %v), want (\"\", true)", p, ok)
	}

	// No root index, but a subdir app → preview the most-recently-built one.
	sub := t.TempDir()
	for _, n := range []string{"old-app", "new-app"} {
		os.MkdirAll(filepath.Join(sub, n), 0o755)
		os.WriteFile(filepath.Join(sub, n, "index.html"), []byte("x"), 0o644)
	}
	// Make new-app the most recent.
	os.Chtimes(filepath.Join(sub, "old-app", "index.html"), time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))
	os.Chtimes(filepath.Join(sub, "new-app", "index.html"), time.Now(), time.Now())
	if p, ok := findPreviewEntry(sub); !ok || p != "new-app" {
		t.Fatalf("subdir app: got (%q, %v), want (\"new-app\", true)", p, ok)
	}

	// .cliche and node_modules are skipped.
	skip := t.TempDir()
	os.MkdirAll(filepath.Join(skip, "node_modules"), 0o755)
	os.WriteFile(filepath.Join(skip, "node_modules", "index.html"), []byte("x"), 0o644)
	if p, ok := findPreviewEntry(skip); ok || p != "" {
		t.Fatalf("node_modules should be skipped: got (%q, %v)", p, ok)
	}
}
