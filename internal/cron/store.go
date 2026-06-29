package cron

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
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
		return nil, err
	}
	return jobs, nil
}

// Save writes the jobs atomically (temp + rename) so a crash can't corrupt them.
func Save(root string, jobs []Job) error {
	d := config.Dir(root)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return err
	}
	p := storePath(root)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Add validates the spec (rejecting a bad schedule up front) and appends a job.
func Add(root, spec, prompt, mode string, maxUSD float64) (Job, error) {
	if _, err := Parse(spec); err != nil {
		return Job{}, err
	}
	jobs, err := Load(root)
	if err != nil {
		return Job{}, err
	}
	j := Job{ID: newID(), Spec: spec, Prompt: prompt, Mode: mode, MaxUSD: maxUSD, Enabled: true, Created: time.Now()}
	return j, Save(root, append(jobs, j))
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
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "job" + time.Now().UTC().Format("150405")
	}
	return hex.EncodeToString(b)
}
