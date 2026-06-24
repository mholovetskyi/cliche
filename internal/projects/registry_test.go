package projects

import (
	"testing"
	"time"
)

func TestUpsertInsertsAndUpdates(t *testing.T) {
	r := &Registry{}
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r.Upsert("/a", "a", t0)
	r.Upsert("/b", "b", t0.Add(time.Hour))
	if len(r.Projects) != 2 {
		t.Fatalf("want 2 projects, got %d", len(r.Projects))
	}
	// Re-using an existing path bumps it in place, never duplicates.
	r.Upsert("/a", "a", t0.Add(2*time.Hour))
	if len(r.Projects) != 2 {
		t.Fatalf("upsert must not duplicate: %d", len(r.Projects))
	}
	if rec := r.Recent(); rec[0].Path != "/a" {
		t.Fatalf("most-recent should be /a (bumped), got %s", rec[0].Path)
	}
}

func TestRemoveByPathOrName(t *testing.T) {
	r := &Registry{}
	now := time.Now()
	r.Upsert("/x/proj", "proj", now)
	if !r.Remove("proj") || len(r.Projects) != 0 {
		t.Fatal("remove by name should drop the entry")
	}
	r.Upsert("/x/proj", "proj", now)
	if !r.Remove("/x/proj") {
		t.Fatal("remove by path should work")
	}
	if r.Remove("nope") {
		t.Fatal("removing an absent key should report false")
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	r := &Registry{Workspace: "/ws"}
	r.Upsert("/p1", "p1", time.Unix(1000, 0).UTC())
	if err := r.Save(dir); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Workspace != "/ws" || len(got.Projects) != 1 || got.Projects[0].Name != "p1" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	// A missing registry loads as empty, not an error.
	if empty, err := Load(t.TempDir()); err != nil || len(empty.Projects) != 0 {
		t.Fatalf("missing registry should be empty: %v %+v", err, empty)
	}
}
