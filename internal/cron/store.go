package cron

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mholovetskyi/cliche/internal/config"
)

// Job is one scheduled prompt for a project. The fire reuses the normal headless
// run, so every execution passes through the Trust Kernel (budget cap, governor,
// deny rules) — a scheduled agent cannot run away.
type Job struct {
	ID         string    `json:"id"`
	Spec       string    `json:"spec"`              // cron spec or @shortcut
	Prompt     string    `json:"prompt"`            // what to run
	Mode       string    `json:"mode,omitempty"`    // permission mode for the fire ("" = full/autonomous)
	MaxUSD     float64   `json:"max_usd,omitempty"` // per-fire budget cap (0 = config default)
	Notify     string    `json:"notify,omitempty"`  // where to deliver the result: "telegram" or an https webhook URL
	Enabled    bool      `json:"enabled"`
	Created    time.Time `json:"created"`
	LastRun    time.Time `json:"last_run,omitempty"`
	LastStatus string    `json:"last_status,omitempty"`
}

func storePath(root string) string { return filepath.Join(config.Dir(root), "cron.json") }

// Load reads the project's scheduled jobs (none if the file is absent).
func Load(root string) ([]Job, error) {
	data, err := os.ReadFile(storePath(root))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var jobs []Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		return nil, fmt.Errorf("cron store %s is corrupt (edit or delete it): %w", storePath(root), err)
	}
	return jobs, nil
}

// Save writes the jobs atomically + durably (unique temp, fsync, rename) so a
// crash mid-write can't corrupt or truncate them.
func Save(root string, jobs []Job) error {
	d := config.Dir(root)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(d, "cron-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, storePath(root))
}

// Add validates the spec (rejecting a bad schedule up front) and appends a job,
// regenerating the id on the (vanishing) chance of a collision.
func Add(root, spec, prompt, mode, notify string, maxUSD float64) (Job, error) {
	if _, err := Parse(spec); err != nil {
		return Job{}, err
	}
	jobs, err := Load(root)
	if err != nil {
		return Job{}, err
	}
	id := newID()
	for tries := 0; tries < 8 && idExists(jobs, id); tries++ {
		id = newID()
	}
	j := Job{ID: id, Spec: spec, Prompt: prompt, Mode: mode, Notify: notify, MaxUSD: maxUSD, Enabled: true, Created: time.Now()}
	return j, Save(root, append(jobs, j))
}

func idExists(jobs []Job, id string) bool {
	for _, j := range jobs {
		if j.ID == id {
			return true
		}
	}
	return false
}

// SetEnabled toggles a job on/off, reporting whether it existed.
func SetEnabled(root, id string, on bool) (bool, error) {
	jobs, err := Load(root)
	if err != nil {
		return false, err
	}
	found := false
	for i := range jobs {
		if jobs[i].ID == id {
			jobs[i].Enabled = on
			found = true
		}
	}
	if !found {
		return false, nil
	}
	return found, Save(root, jobs)
}

// Remove deletes a job by id, reporting whether it existed.
func Remove(root, id string) (bool, error) {
	jobs, err := Load(root)
	if err != nil {
		return false, err
	}
	out := make([]Job, 0, len(jobs))
	found := false
	for _, j := range jobs {
		if j.ID == id {
			found = true
			continue
		}
		out = append(out, j)
	}
	if !found {
		return false, nil
	}
	return true, Save(root, out)
}

// MarkRun records a fire's result (best-effort; re-loads to avoid clobbering a
// concurrent edit to other jobs).
func MarkRun(root, id, status string, when time.Time) {
	jobs, err := Load(root)
	if err != nil {
		return
	}
	for i := range jobs {
		if jobs[i].ID == id {
			jobs[i].LastRun = when
			jobs[i].LastStatus = status
		}
	}
	_ = Save(root, jobs)
}

func newID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("job%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
