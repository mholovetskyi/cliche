package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/provider"
	sess "github.com/mholovetskyi/cliche/internal/session"
)

func TestExpandAtRefs(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "notes.md"), []byte("hello notes"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "api.txt"), []byte("sk-secret"), 0o644)

	out := expandAtRefs(root, "summarize @notes.md please")
	if !strings.Contains(out, "hello notes") || !strings.Contains(out, "notes.md") {
		t.Fatalf("@notes.md should be inlined: %q", out)
	}
	if got := expandAtRefs(root, "see @nope.md"); got != "see @nope.md" {
		t.Fatalf("unknown @ref should be left untouched: %q", got)
	}
	if strings.Contains(expandAtRefs(root, "look at @api.txt"), "sk-secret") {
		t.Fatal("a secret file must not be inlined via @ref")
	}
}

func TestMaxTaskID(t *testing.T) {
	if got := maxTaskID([]sess.Task{{ID: 3}, {ID: 7}, {ID: 1}}); got != 7 {
		t.Fatalf("maxTaskID = %d, want 7", got)
	}
	if maxTaskID(nil) != 0 {
		t.Fatal("maxTaskID(nil) should be 0")
	}
}

func TestReadProjectFileConfinedToRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<h1>hi</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A secret one level above the project root — must never be reachable.
	if err := os.WriteFile(filepath.Join(filepath.Dir(root), "secret.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A secret file inside the root must be refused even though the path is valid.
	if err := os.WriteFile(filepath.Join(root, "api.txt"), []byte("sk-secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got, ok := readProjectFile(root, "index.html"); !ok || got != "<h1>hi</h1>" {
		t.Fatalf("should read a project file: ok=%v got=%q", ok, got)
	}
	if _, ok := readProjectFile(root, "api.txt"); ok {
		t.Fatal("readProjectFile must refuse a secret file (api.txt)")
	}
	for _, bad := range []string{"../secret.txt", "..\\secret.txt", "/etc/passwd", "", "..", "subdir/../../secret.txt"} {
		if _, ok := readProjectFile(root, bad); ok {
			t.Fatalf("path %q escaped the project root", bad)
		}
	}
}

func TestToMsgsFlattensTranscript(t *testing.T) {
	tr := []provider.Message{
		{Role: "user", Text: "build a site"},
		{Role: "assistant", Text: "on it", ToolCalls: []provider.ToolCall{{Name: "write_file"}}},
		{Role: "user", ToolResults: []provider.ToolResult{{ID: "1", Content: "ok"}}}, // tool result → dropped
		{Role: "assistant", Text: "done"},
	}
	got := toMsgs(tr)
	want := []struct{ role, text string }{
		{"user", "build a site"},
		{"assistant", "on it"},
		{"tool", "write_file"},
		{"assistant", "done"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Role != w.role || got[i].Text != w.text {
			t.Fatalf("row %d = %+v, want %s/%q", i, got[i], w.role, w.text)
		}
	}
}

func TestTitleFromTruncates(t *testing.T) {
	if got := titleFrom("  hello world\nsecond line  "); got != "hello world" {
		t.Fatalf("titleFrom first-line/trim wrong: %q", got)
	}
	long := titleFrom(string(make([]byte, 0)) + "a very long prompt that certainly exceeds the sixty character display limit for sure")
	if r := []rune(long); len(r) > 61 || r[len(r)-1] != '…' {
		t.Fatalf("titleFrom should truncate with an ellipsis: %q", long)
	}
}
