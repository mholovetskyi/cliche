package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffoldBackendSupabase(t *testing.T) {
	root := t.TempDir()
	// A TypeScript app (tsconfig present) → the client should be .ts.
	if err := os.WriteFile(filepath.Join(root, "tsconfig.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := OSExecutor{Root: root, Policy: Policy{AllowWrite: true}}

	res := e.scaffoldBackend(map[string]string{"kind": "supabase"})
	if !res.Success || !res.IsEdit {
		t.Fatalf("scaffold should succeed as an edit: %+v", res)
	}
	for _, want := range []string{"src/lib/supabaseClient.ts", ".env.example", "supabase/schema.sql", "BACKEND.md"} {
		if _, err := os.Stat(filepath.Join(root, want)); err != nil {
			t.Errorf("expected %s to be written: %v", want, err)
		}
	}
	// The client must read keys from env, never hardcode them.
	b, _ := os.ReadFile(filepath.Join(root, "src/lib/supabaseClient.ts"))
	if !strings.Contains(string(b), "import.meta.env.VITE_SUPABASE_URL") {
		t.Error("client should read the URL from env")
	}

	// Re-running must not clobber an existing file.
	custom := []byte("// my edits")
	_ = os.WriteFile(filepath.Join(root, ".env.example"), custom, 0o644)
	if res2 := e.scaffoldBackend(map[string]string{}); !res2.Success {
		t.Fatalf("second run should still succeed: %+v", res2)
	}
	if b, _ := os.ReadFile(filepath.Join(root, ".env.example")); string(b) != string(custom) {
		t.Error("scaffold clobbered an existing file")
	}
}

func TestScaffoldBackendDeniedWithoutPermission(t *testing.T) {
	e := OSExecutor{Root: t.TempDir()} // no AllowWrite, no approver → denied
	if res := e.scaffoldBackend(map[string]string{}); res.Success {
		t.Fatal("scaffold must be permission-gated")
	}
}

func TestScaffoldBackendUnknownKind(t *testing.T) {
	e := OSExecutor{Root: t.TempDir(), Policy: Policy{AllowWrite: true}}
	if res := e.scaffoldBackend(map[string]string{"kind": "firebase"}); res.Success {
		t.Fatal("unknown kind should fail")
	}
}
