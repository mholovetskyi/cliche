// Package ledger is the append-only audit trail: every turn, tool call,
// budget event, and verdict is recorded so a run is fully attributable after
// the fact. The ledger never captures secrets or raw file contents — only
// metadata (tokens, cost, event types, verdicts, short details).
package ledger

import (
	"crypto/sha256"
	"encoding/hex"
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

// Entry is one append-only record. PrevHash/Hash form a tamper-evident chain:
// each entry's Hash commits to its content AND the previous entry's Hash, so any
// alteration, deletion, reordering, or insertion of a past entry is detectable
// (see Verify). They are set by Append; callers leave them zero.
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
	PrevHash     string    `json:"prev,omitempty"`
	Hash         string    `json:"hash,omitempty"`
}

// contentHash is sha256(prev + content), where content is the entry's fields
// excluding the chain fields. Binding prev in explicitly (not via JSON) makes
// the link unambiguous and independent of field ordering.
func (e Entry) contentHash(prev string) string {
	c := e
	c.PrevHash, c.Hash = "", ""
	b, _ := json.Marshal(c)
	sum := sha256.Sum256(append([]byte(prev+"\n"), b...))
	return hex.EncodeToString(sum[:])
}

// Ledger appends entries to <dir>/ledger.jsonl.
type Ledger struct {
	mu   sync.Mutex
	path string
	now  func() time.Time
	tip  string // hash of the last entry — the chain head new entries link to
}

// Open creates dir if needed and returns a Ledger writing to ledger.jsonl. It
// loads the existing chain head so appends continue the hash chain.
func Open(dir string) (*Ledger, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	l := &Ledger{path: filepath.Join(dir, "ledger.jsonl"), now: time.Now}
	l.tip = l.lastHash()
	return l, nil
}

// lastHash returns the Hash of the final hashed entry (the chain head), or ""
// for an empty/legacy ledger.
func (l *Ledger) lastHash() string {
	f, err := os.Open(l.path)
	if err != nil {
		return ""
	}
	defer f.Close()
	tip := ""
	dec := json.NewDecoder(f)
	for dec.More() {
		var e Entry
		if dec.Decode(&e) != nil {
			break
		}
		if e.Hash != "" {
			tip = e.Hash
		}
	}
	return tip
}

// Path returns the ledger file path.
func (l *Ledger) Path() string { return l.path }

// Head returns the current chain-head hash (empty for an empty/legacy ledger).
func (l *Ledger) Head() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.tip
}

// Append writes one entry. Safe for concurrent use.
func (l *Ledger) Append(e Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if e.Time.IsZero() {
		e.Time = l.now()
	}
	// Chain this entry onto the current head before writing.
	e.PrevHash = l.tip
	e.Hash = e.contentHash(l.tip)
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
	if err := f.Close(); err != nil {
		return err
	}
	l.tip = e.Hash // advance the head only after a durable write
	return nil
}

// Report is the result of verifying the ledger's hash chain.
type Report struct {
	Entries  int    `json:"entries"`
	Verified int    `json:"verified"` // entries whose chain links and content check out
	Legacy   int    `json:"legacy"`   // pre-chain entries (no hash) — unverifiable
	OK       bool   `json:"ok"`       // true if no tampering was detected
	BrokenAt int    `json:"broken_at,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// Verify re-walks the ledger and checks the hash chain end to end: each hashed
// entry must link to the previous head and its content must hash to its recorded
// Hash. It detects alteration (content no longer hashes), deletion/reordering,
// and insertion (a broken link). Legacy entries written before the chain existed
// are counted but not verifiable. A missing file verifies as OK (empty).
func (l *Ledger) Verify() (Report, error) {
	f, err := os.Open(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return Report{OK: true}, nil
		}
		return Report{}, err
	}
	defer f.Close()

	r := Report{OK: true}
	prevTip := ""
	dec := json.NewDecoder(f)
	for i := 0; dec.More(); {
		i++
		var e Entry
		if err := dec.Decode(&e); err != nil {
			return Report{}, err
		}
		r.Entries++
		if e.Hash == "" { // pre-chain (legacy) entry — skip, not part of the chain
			r.Legacy++
			continue
		}
		if e.PrevHash != prevTip {
			r.OK, r.BrokenAt, r.Reason = false, i, "broken chain link — an entry was deleted, reordered, or inserted"
			return r, nil
		}
		if e.contentHash(prevTip) != e.Hash {
			r.OK, r.BrokenAt, r.Reason = false, i, "entry content was altered after it was written"
			return r, nil
		}
		prevTip = e.Hash
		r.Verified++
	}
	return r, nil
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
func (l *Ledger) Summarize() (Summary, error) { return l.SummarizeSince(time.Time{}) }

// SummarizeSince aggregates only entries at or after since (a zero time includes
// everything). Used to scope a report to a single session's time window rather
// than the whole project history.
func (l *Ledger) SummarizeSince(since time.Time) (Summary, error) {
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
		if !since.IsZero() && e.Time.Before(since) {
			continue
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
