package tools

import (
	"strings"
	"testing"
)

func TestSanitizeOutputRedactsAndBounds(t *testing.T) {
	// Credential-shaped strings are redacted.
	in := "key=sk-abcdefghijklmnopqrstuvwx token=ghp_ABCDEFGHIJKLMNOPQRST123456 ok"
	out := SanitizeOutput(in)
	if strings.Contains(out, "sk-abcdefghij") || strings.Contains(out, "ghp_ABCDEF") {
		t.Fatalf("secrets not redacted: %q", out)
	}
	if !strings.Contains(out, "[REDACTED]") || !strings.Contains(out, "ok") {
		t.Fatalf("redaction mangled output: %q", out)
	}
	// The operator's own provider key (from env) is redacted verbatim.
	t.Setenv("SOME_API_KEY", "supersecretvalue123")
	if o := SanitizeOutput("leaked supersecretvalue123 here"); strings.Contains(o, "supersecretvalue123") {
		t.Fatalf("operator secret not redacted: %q", o)
	}
	// Huge output is bounded.
	big := SanitizeOutput(strings.Repeat("x", maxToolOutputBytes+5000))
	if len(big) > maxToolOutputBytes+200 {
		t.Fatalf("output not bounded: %d bytes", len(big))
	}
	// Normal small output is untouched.
	if SanitizeOutput("wrote index.html") != "wrote index.html" {
		t.Fatal("clean output should pass through unchanged")
	}
}
