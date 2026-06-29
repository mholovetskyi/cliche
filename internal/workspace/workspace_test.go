package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func mkApp(t *testing.T, dir, kind string) {
	t.Helper()
	os.MkdirAll(dir, 0o755)
	switch kind {
	case "static":
		os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>hi</h1>"), 0o644)
	case "dev":
		os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"dev":"vite"}}`), 0o644)
	}
}

func TestAppsDetectsStaticAndDev(t *testing.T) {
	root := t.TempDir()
	mkApp(t, filepath.Join(root, "site"), "static")
	mkApp(t, filepath.Join(root, "dashboard"), "dev")
	os.MkdirAll(filepath.Join(root, "notes"), 0o755) // not an app
	os.MkdirAll(filepath.Join(root, "node_modules"), 0o755)

	apps := Apps(root)
	if len(apps) != 2 {
		t.Fatalf("want 2 apps, got %d: %+v", len(apps), apps)
	}
	byName := map[string]App{}
	for _, a := range apps {
		byName[a.Name] = a
	}
	if byName["dashboard"].Kind != "dev" || byName["dashboard"].Script != "npm run dev" {
		t.Fatalf("dashboard: %+v", byName["dashboard"])
	}
	if byName["site"].Kind != "static" {
		t.Fatalf("site: %+v", byName["site"])
	}

	// The root itself counts as an app when it has index.html, listed first.
	mkApp(t, root, "static")
	if a := Apps(root); a[0].Rel != "." {
		t.Fatalf("root app should be first, got %+v", a)
	}
}

func TestProjectsCountsAppsAndChats(t *testing.T) {
	ws := t.TempDir()
	proj := filepath.Join(ws, "my-saas")
	mkApp(t, filepath.Join(proj, "frontend"), "dev")
	mkApp(t, filepath.Join(proj, "marketing"), "static")
	// two saved chats
	sess := filepath.Join(proj, ".cliche", "sessions")
	os.MkdirAll(sess, 0o755)
	os.WriteFile(filepath.Join(sess, "a.json"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(sess, "b.json"), []byte("{}"), 0o644)
	// a hidden dir is not a project
	os.MkdirAll(filepath.Join(ws, ".trash"), 0o755)

	ps := Projects(ws)
	if len(ps) != 1 || ps[0].Name != "my-saas" {
		t.Fatalf("projects: %+v", ps)
	}
	if ps[0].Apps != 2 || ps[0].Chats != 2 {
		t.Fatalf("counts: apps=%d chats=%d", ps[0].Apps, ps[0].Chats)
	}
}
