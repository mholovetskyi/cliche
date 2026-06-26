package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/mholovetskyi/cliche/internal/style"
)

func TestFormatHUD(t *testing.T) {
	old := style.Enabled
	defer func() { style.Enabled = old }()
	style.Enabled = true

	// $0.05 of a $1.00 cap (5%), context 20% full, over 2 minutes → $0.025/min.
	h := formatHUD(0.05, 1.0, 0.20, 2*time.Minute)
	for _, want := range []string{"0.0500", "/min", "5%", "20%"} {
		if !strings.Contains(h, want) {
			t.Fatalf("HUD missing %q in %q", want, h)
		}
	}

	// No cap, no context, no elapsed: just the spend, no burn-rate, no panic.
	h2 := formatHUD(0, 0, 0, time.Second)
	if !strings.Contains(h2, "0.0000") {
		t.Fatalf("spend-only HUD wrong: %q", h2)
	}
	if strings.Contains(h2, "/min") {
		t.Fatalf("no burn-rate when nothing spent: %q", h2)
	}
}

func TestFormatHUDDegrades(t *testing.T) {
	old := style.Enabled
	defer func() { style.Enabled = old }()
	style.Enabled = false
	// Under NO_COLOR the gauges vanish but the numbers still carry the meaning.
	if h := formatHUD(0.1, 1.0, 0.5, time.Minute); !strings.Contains(h, "0.1000") {
		t.Fatalf("plain HUD should still show spend: %q", h)
	}
}
