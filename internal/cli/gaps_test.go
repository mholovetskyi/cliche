package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/config"
	sess "github.com/mholovetskyi/cliche/internal/session"
	"github.com/mholovetskyi/cliche/internal/style"
)

func TestForkSession(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	s := newMgmtSession(t, dir, &out)
	s.id, s.title = "20200101-000000", "original"

	s.forkSession()
	if s.id == "20200101-000000" {
		t.Fatal("/fork should assign a new session id")
	}
	if !strings.Contains(out.String(), "forked") {
		t.Fatalf("/fork should confirm:\n%s", out.String())
	}
	// The original is frozen on disk and the fork is saved → two sessions.
	if metas, _ := sess.List(dir); len(metas) != 2 {
		t.Fatalf("expected original + fork = 2 sessions, got %d", len(metas))
	}
}

func TestRenderMCP(t *testing.T) {
	oldE, oldNC := style.Enabled, noColor
	style.Enabled, noColor = false, true
	defer func() { style.Enabled, noColor = oldE, oldNC }()

	var out bytes.Buffer
	renderMCP(&out, []config.MCPServer{{Name: "gh", Command: "gh-mcp"}, {Name: "db", URL: "http://x"}})
	for _, want := range []string{"mcp servers", "gh", "gh-mcp", "stdio", "db", "http://x", "http"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("renderMCP missing %q:\n%s", want, out.String())
		}
	}

	out.Reset()
	renderMCP(&out, nil)
	if !strings.Contains(out.String(), "no MCP servers") {
		t.Fatalf("empty MCP should say so:\n%s", out.String())
	}
}
