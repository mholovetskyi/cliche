package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestCmdModels(t *testing.T) {
	var out bytes.Buffer
	if code := cmdModels(nil, &out, &out); code != 0 {
		t.Fatalf("models exit code = %d, want 0", code)
	}
	got := out.String()
	for _, want := range []string{"model prices", "claude-opus-4-8", "fallback"} {
		if !strings.Contains(got, want) {
			t.Fatalf("models output missing %q:\n%s", want, got)
		}
	}
}
