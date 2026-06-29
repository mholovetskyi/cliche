package session

import (
	"testing"
	"time"

	"github.com/mholovetskyi/cliche/internal/provider"
)

func TestSearch(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	if err := Save(root, Record{
		ID: "20250101-000000", Title: "auth work", Created: now, Updated: now,
		Messages: []provider.Message{
			{Role: "user", Text: "let's add JWT authentication to the API"},
			{Role: "assistant", Text: "added JWT middleware and token validation"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := Save(root, Record{
		ID: "20250102-000000", Title: "ui polish", Created: now, Updated: now.Add(time.Hour),
		Messages: []provider.Message{{Role: "user", Text: "make the primary button blue"}},
	}); err != nil {
		t.Fatal(err)
	}

	hits := Search(root, "jwt authentication", 5)
	if len(hits) == 0 || hits[0].ID != "20250101-000000" {
		t.Fatalf("expected the auth session ranked first, got %+v", hits)
	}
	if hits[0].Snippet == "" {
		t.Fatal("expected a snippet around the match")
	}
	if got := Search(root, "nonexistentterm", 5); len(got) != 0 {
		t.Fatalf("expected no hits, got %d", len(got))
	}
	if got := Search(root, "", 5); len(got) != 0 {
		t.Fatal("empty query should return nothing")
	}
}
