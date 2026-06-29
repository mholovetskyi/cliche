package cli

import (
	"strings"
	"testing"
)

func TestDoctorChecks(t *testing.T) {
	t.Setenv("CLICHE_CONFIG_HOME", t.TempDir())
	checks := doctorChecks(t.TempDir())
	if len(checks) == 0 {
		t.Fatal("expected a non-empty set of checks")
	}
	// Every check has a label and a valid status.
	var sawConfig, sawCron bool
	for _, c := range checks {
		if c.label == "" {
			t.Error("a check has an empty label")
		}
		switch c.status {
		case "ok", "warn", "fail":
		default:
			t.Errorf("invalid status %q", c.status)
		}
		if strings.Contains(c.label, "config valid") || strings.Contains(c.label, "config invalid") {
			sawConfig = true
		}
		if strings.Contains(c.label, "cron job") {
			sawCron = true
		}
	}
	if !sawConfig {
		t.Error("expected a Trust-Kernel/config check")
	}
	if !sawCron {
		t.Error("expected a cron-jobs check")
	}
}
