package cron

import "testing"

func TestSpendTracking(t *testing.T) {
	root := t.TempDir()
	if SpentLast24h(root) != 0 {
		t.Fatal("a fresh project should have $0 cron spend")
	}
	RecordSpend(root, 1.50)
	RecordSpend(root, 2.25)
	if got := SpentLast24h(root); got < 3.74 || got > 3.76 {
		t.Fatalf("rolling spend = %v, want ~3.75", got)
	}
}

func TestSetEnabled(t *testing.T) {
	root := t.TempDir()
	j, err := Add(root, "@daily", "x", "full", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := SetEnabled(root, j.ID, false); !ok {
		t.Fatal("SetEnabled should find the job")
	}
	jobs, _ := Load(root)
	if jobs[0].Enabled {
		t.Fatal("job should be disabled")
	}
	if ok, _ := SetEnabled(root, "ghost", true); ok {
		t.Fatal("a missing id should report false")
	}
}
