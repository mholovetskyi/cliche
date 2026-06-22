// Package ledger is the append-only audit trail: every turn, tool call,
// budget event, and verdict is recorded so a run is fully attributable after
// the fact. The ledger never captures secrets or raw file contents — only
// metadata (tokens, cost, event types, verdicts, short details).
package ledger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Event kinds written to the ledger.
const (
	EventTurn    = "turn"
	EventTool    = "tool"
	EventHalt    = "halt"
	EventVerdict = "verdict"
	EventInfo    = "info"
)

// Entry is one append-only record.
type Entry struct {
	Time         time.Time `json:"time"`
	Turn         int       `json:"turn,omitempty"`
	Event        string    `json:"event"`
	Model        string    `json:"model,omitempty"`
	InputTokens  int       `json:"input_tokens,omitempty"`
	OutputTokens int       `json:"output_tokens,omitempty"`
	USD          float64   `json:"usd,omitempty"`
	Verdict      string    `json:"verdict,omitempty"`
	Detail       string    `json:"detail,omitempty"`
}

// Ledger appends entries to <dir>/ledger.jsonl.
type Ledger struct {
	mu   sync.Mutex
	path string
	now  func() time.Time
}

// Open creates dir if needed and returns a Ledger writing to ledger.jsonl.
func Open(dir string) (*Ledger, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Ledger{path: filepath.Join(dir, "ledger.jsonl"), now: time.Now}, nil
}

// Path returns the ledger file path.
func (l *Ledger) Path() string { return l.path }

// Append writes one entry. Safe for concurrent use.
func (l *Ledger) Append(e Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if e.Time.IsZero() {
		e.Time = l.now()
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(f).Encode(e); err != nil {
		f.Close()
		return err
	}
	// fsync before close so an acknowledged audit record survives a crash, and
	// surface a Close error rather than dropping it.
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Summary aggregates a ledger file.
type Summary struct {
	Turns        int            `json:"turns"`
	InputTokens  int            `json:"input_tokens"`
	OutputTokens int            `json:"output_tokens"`
	USD          float64        `json:"usd"`
	Verdicts     map[string]int `json:"verdicts"`
}

// Summarize reads the whole ledger and aggregates it. A missing file is not
// an error; it yields an empty summary.
func (l *Ledger) Summarize() (Summary, error) {
	s := Summary{Verdicts: map[string]int{}}
	f, err := os.Open(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	for dec.More() {
		var e Entry
		if err := dec.Decode(&e); err != nil {
			return s, err
		}
		s.InputTokens += e.InputTokens
		s.OutputTokens += e.OutputTokens
		s.USD += e.USD
		if e.Event == EventTurn {
			s.Turns++
		}
		if e.Verdict != "" {
			s.Verdicts[e.Verdict]++
		}
	}
	return s, nil
}
