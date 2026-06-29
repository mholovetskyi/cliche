package cron

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/mholovetskyi/cliche/internal/config"
)

// Cron's per-fire Trust Kernel bounds a single run, but a high-frequency schedule
// could still rack up unbounded TOTAL spend (e.g. "@every 1m" × the per-fire cap).
// This tracks a rolling 24h spend so the scheduler can enforce a daily ceiling —
// making "can't run away" true in aggregate, not just per fire. Persisted so a
// restart can't reset the window.

type spendEntry struct {
	At  time.Time `json:"at"`
	USD float64   `json:"usd"`
}

func spendPath(root string) string { return filepath.Join(config.Dir(root), "cron-spend.json") }

func loadSpend(root string) []spendEntry {
	data, err := os.ReadFile(spendPath(root))
	if err != nil {
		return nil
	}
	var e []spendEntry
	_ = json.Unmarshal(data, &e)
	return e
}

// RecordSpend appends a fire's cost and prunes entries older than 24h.
func RecordSpend(root string, usd float64) {
	cutoff := time.Now().Add(-24 * time.Hour)
	var kept []spendEntry
	for _, e := range loadSpend(root) {
		if e.At.After(cutoff) {
			kept = append(kept, e)
		}
	}
	kept = append(kept, spendEntry{At: time.Now(), USD: usd})
	if err := os.MkdirAll(config.Dir(root), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(kept, "", "  ")
	if err != nil {
		return
	}
	tmp := spendPath(root) + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Rename(tmp, spendPath(root))
	}
}

// SpentLast24h sums cron-fire spend over the trailing 24 hours.
func SpentLast24h(root string) float64 {
	cutoff := time.Now().Add(-24 * time.Hour)
	var sum float64
	for _, e := range loadSpend(root) {
		if e.At.After(cutoff) {
			sum += e.USD
		}
	}
	return sum
}
