package cron

import (
	"testing"
	"time"
)

func TestStoreRoundTrip(t *testing.T) {
	root := t.TempDir()
	if jobs, _ := Load(root); len(jobs) != 0 {
		t.Fatal("a fresh store should be empty")
	}
	j, err := Add(root, "@daily", "do the thing", "full", 0.5)
	if err != nil {
		t.Fatal(err)
	}
	jobs, _ := Load(root)
	if len(jobs) != 1 || jobs[0].ID != j.ID || !jobs[0].Enabled || jobs[0].Prompt != "do the thing" {
		t.Fatalf("add/load mismatch: %+v", jobs)
	}
	// A bad spec is rejected at add time, never stored.
	if _, err := Add(root, "not a cron", "x", "full", 0); err == nil {
		t.Fatal("bad spec should be rejected")
	}
	MarkRun(root, j.ID, "completed", time.Now())
	if jobs, _ = Load(root); jobs[0].LastStatus != "completed" {
		t.Fatal("MarkRun did not persist")
	}
	if ok, _ := Remove(root, j.ID); !ok {
		t.Fatal("remove should report found")
	}
	if jobs, _ := Load(root); len(jobs) != 0 {
		t.Fatal("store should be empty after remove")
	}
	if ok, _ := Remove(root, "ghost"); ok {
		t.Fatal("removing a missing id should report false")
	}
}
