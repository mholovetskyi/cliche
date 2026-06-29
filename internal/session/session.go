// Package session persists an interactive chat transcript to disk so a session
// can be resumed after the terminal closes — "leave it running" is hollow if a
// dropped connection loses all state. A session is stored as one JSON file
// under .cliche/sessions/, alongside (and in the same repo-local, no-secrets
// spirit as) the cost ledger: the saved session IS part of the audit record.
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mholovetskyi/cliche/internal/budget"
	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/provider"
)

// Task is one item on the session's lightweight plan (the /plan, /tasks, /done
// surface). Persisted with the record so resuming a session restores the plan.
type Task struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

// Record is a persisted session.
type Record struct {
	ID       string             `json:"id"`
	Title    string             `json:"title"`
	Provider string             `json:"provider"`
	Model    string             `json:"model"`
	Created  time.Time          `json:"created"`
	Updated  time.Time          `json:"updated"`
	Usage    budget.Usage       `json:"usage"`
	Messages []provider.Message `json:"messages"`
	Tasks    []Task             `json:"tasks,omitempty"`
	Limits   *Limits            `json:"limits,omitempty"` // per-session Trust-Kernel caps (nil = config defaults)
}

// Limits is a session's saved Trust-Kernel caps, so a budget/turn/token limit
// dialed for one chat is restored when that chat is reopened.
type Limits struct {
	MaxUSD    float64 `json:"max_usd"`
	MaxTokens int     `json:"max_tokens"`
	MaxTurns  int     `json:"max_turns"`
}

// Meta is a lightweight summary for listing.
type Meta struct {
	ID       string
	Title    string
	Model    string
	Updated  time.Time
	Messages int
}

func dir(root string) string { return filepath.Join(config.Dir(root), "sessions") }

// NewID returns a sortable, human-readable session id from the given time.
func NewID(t time.Time) string { return t.UTC().Format("20060102-150405") }

// Delete removes a saved session file, returning an error if the id is empty or
// the session doesn't exist.
func Delete(root, id string) error {
	if id == "" {
		return fmt.Errorf("no session id")
	}
	return os.Remove(filepath.Join(dir(root), id+".json"))
}

// Save writes the record to .cliche/sessions/<id>.json (creating the dir).
func Save(root string, r Record) error {
	d := dir(root)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	// Write atomically via a temp file + rename so a crash mid-write can't
	// corrupt an existing session.
	path := filepath.Join(d, r.ID+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load reads a session by id.
func Load(root, id string) (Record, error) {
	var r Record
	data, err := os.ReadFile(filepath.Join(dir(root), id+".json"))
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return r, fmt.Errorf("session %s is corrupt: %w", id, err)
	}
	return r, nil
}

// List returns session metadata, most-recently-updated first.
func List(root string) ([]Meta, error) {
	entries, err := os.ReadDir(dir(root))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Meta
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		r, err := Load(root, id)
		if err != nil {
			continue // skip corrupt/partial files rather than failing the list
		}
		out = append(out, Meta{ID: r.ID, Title: r.Title, Model: r.Model, Updated: r.Updated, Messages: len(r.Messages)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	return out, nil
}

// Latest returns the id of the most recently updated session, or "" if none.
func Latest(root string) string {
	metas, err := List(root)
	if err != nil || len(metas) == 0 {
		return ""
	}
	return metas[0].ID
}
