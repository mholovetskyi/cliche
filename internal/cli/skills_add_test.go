package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillSlug(t *testing.T) {
	cases := map[string]string{
		"My Skill":    "my-skill",
		"../../etc":   "etc",
		"PR Helper!":  "pr-helper",
		"scaffold/v2": "scaffold-v2",
		"":            "",
	}
	for in, want := range cases {
		if got := skillSlug(in); got != want {
			t.Errorf("skillSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSkillsAdd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("---\nname: deploy-helper\ndescription: how to deploy this app\n---\n\n1. run make deploy\n"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	var out, errOut bytes.Buffer
	if code := skillsAdd(srv.URL, &out, &errOut); code != 0 {
		t.Fatalf("skillsAdd code %d: %s", code, errOut.String())
	}
	data, err := os.ReadFile(filepath.Join(".cliche", "skills", "deploy-helper", "SKILL.md"))
	if err != nil || !strings.Contains(string(data), "make deploy") {
		t.Fatalf("skill not installed: %v", err)
	}
	// Re-adding the same skill is refused.
	if code := skillsAdd(srv.URL, &out, &errOut); code == 0 {
		t.Fatal("re-adding an existing skill should fail")
	}

	// A non-SKILL.md (no frontmatter) is rejected.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("just some text, not a skill"))
	}))
	defer bad.Close()
	if code := skillsAdd(bad.URL, &out, &errOut); code == 0 {
		t.Fatal("a non-SKILL.md should be rejected")
	}
}
