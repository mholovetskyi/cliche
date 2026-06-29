package devserver

import (
	"os"
	"path/filepath"
	"testing"
)

func writePkg(t *testing.T, dir string, scripts string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"name":"x","scripts":` + scripts + `}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetectIn(t *testing.T) {
	// dev script at the root.
	root := t.TempDir()
	writePkg(t, root, `{"dev":"vite"}`)
	if dir, cmd, ok := DetectIn(root); !ok || cmd != "npm run dev" || dir != root {
		t.Fatalf("root dev: got (%q,%q,%v)", dir, cmd, ok)
	}

	// dev script one level down (app scaffolded into a subfolder); .dirs skipped.
	sub := t.TempDir()
	os.MkdirAll(filepath.Join(sub, ".cache"), 0o755)
	writePkg(t, filepath.Join(sub, "app"), `{"start":"next dev"}`)
	if dir, cmd, ok := DetectIn(sub); !ok || cmd != "npm run start" || filepath.Base(dir) != "app" {
		t.Fatalf("subdir dev: got (%q,%q,%v)", dir, cmd, ok)
	}

	// no scripts → not detected.
	empty := t.TempDir()
	writePkg(t, empty, `{"build":"tsc"}`)
	if _, _, ok := DetectIn(empty); ok {
		t.Fatal("a project with no dev/start/serve script must not be detected")
	}
	if _, _, ok := DetectIn(t.TempDir()); ok {
		t.Fatal("an empty dir must not be detected")
	}
}

func TestURLParseAndStrip(t *testing.T) {
	cases := map[string]string{
		"  ➜  Local:   http://localhost:5173/":           "http://localhost:5173",
		"VITE ready at http://127.0.0.1:4321/":           "http://127.0.0.1:4321",
		"Listening on http://0.0.0.0:3000":               "http://localhost:3000",
		"\x1b[32m  Local: http://localhost:8080/\x1b[0m": "http://localhost:8080",
	}
	for line, want := range cases {
		got := normalizeURL(devURLRe.FindString(stripANSI(line)))
		if got != want {
			t.Fatalf("parse %q: got %q want %q", line, got, want)
		}
	}
	// A plain log line yields no URL.
	if u := devURLRe.FindString("compiling modules..."); u != "" {
		t.Fatalf("non-URL line matched: %q", u)
	}
}

func TestStatusDetectsWhileStopped(t *testing.T) {
	root := t.TempDir()
	writePkg(t, root, `{"dev":"vite"}`)
	m := New()
	st := m.Status(root)
	if st.State != StateStopped || !st.Detected || st.Script != "npm run dev" {
		t.Fatalf("status: %+v", st)
	}
}
