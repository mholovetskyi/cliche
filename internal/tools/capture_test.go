package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScreenshotURLConfinement(t *testing.T) {
	root := t.TempDir()
	e := OSExecutor{Root: root}
	if _, err := e.screenshotURL("https://example.com/x"); err == nil {
		t.Fatal("an external URL must be refused")
	}
	if _, err := e.screenshotURL("../secret.txt"); err == nil {
		t.Fatal("a path outside the project root must be refused")
	}
	if u, err := e.screenshotURL("http://localhost:7878/preview/"); err != nil || u == "" {
		t.Fatalf("a localhost URL should be allowed: %v", err)
	}
	_ = os.WriteFile(filepath.Join(root, "index.html"), []byte("<h1>hi</h1>"), 0o644)
	u, err := e.screenshotURL("index.html")
	if err != nil || !strings.HasPrefix(u, "file:///") {
		t.Fatalf("a project file must become a file:// URL: %q %v", u, err)
	}
}

// TestScreenshotEndToEnd actually drives a headless browser when one is present
// (skipped in environments without Chrome/Edge/Chromium).
func TestScreenshotEndToEnd(t *testing.T) {
	if findBrowser() == "" {
		t.Skip("no headless browser found")
	}
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "index.html"), []byte(`<html><body style="background:#123;margin:0"><h1 style="color:#fff;font-family:sans-serif">Cliche can see</h1></body></html>`), 0o644)
	res := OSExecutor{Root: root}.screenshot(context.Background(), map[string]string{"target": "index.html", "width": "600", "height": "400"})
	if !res.Success {
		t.Fatalf("screenshot failed: %s", res.Output)
	}
	if len(res.Images) != 1 || res.Images[0].MediaType != "image/png" {
		t.Fatalf("expected exactly one PNG image, got %d", len(res.Images))
	}
	if d := res.Images[0].Data; len(d) < 8 || d[0] != 0x89 || d[1] != 'P' || d[2] != 'N' || d[3] != 'G' {
		t.Fatal("result image is not a PNG")
	}
}
