package cli

import (
	"testing"

	"github.com/mholovetskyi/cliche/internal/secrets"
)

func TestConnectorMCPAttachesToken(t *testing.T) {
	t.Setenv("CLICHE_CONFIG_HOME", t.TempDir()) // isolate the global store

	// Nothing connected → no connector MCP servers.
	if got := connectorMCP(); len(got) != 0 {
		t.Fatalf("no connectors should yield none, got %d", len(got))
	}

	// Connect github (store a token) → it appears as an MCP server with the bearer.
	if err := secrets.SaveConnector("github", secrets.ConnectorToken{Token: "gho_X", Type: "bearer"}); err != nil {
		t.Fatal(err)
	}
	got := connectorMCP()
	if len(got) != 1 {
		t.Fatalf("expected 1 connector MCP server, got %d", len(got))
	}
	s := got[0]
	if s.Name != "github" || s.URL != knownConnectors["github"].mcpURL {
		t.Fatalf("connector server wrong: %+v", s)
	}
	if s.Headers["Authorization"] != "Bearer gho_X" {
		t.Fatalf("connector must carry the bearer token, got %q", s.Headers["Authorization"])
	}

	// An unknown connected name is ignored (no registry entry).
	_ = secrets.SaveConnector("mystery", secrets.ConnectorToken{Token: "t"})
	if got := connectorMCP(); len(got) != 1 {
		t.Fatalf("unknown connector should be skipped, got %d", len(got))
	}
}
