package session

import (
	"sort"
	"strings"
	"time"
)

// Hit is one past-session match for cross-session recall.
type Hit struct {
	ID      string
	Title   string
	Updated time.Time
	Snippet string // a short window around the first matching term
	Score   int    // total term occurrences across the transcript
}

// Search scans this project's saved sessions for the query terms and returns the
// best matches, most-relevant first. A deliberately simple pure-Go scan (no index,
// no SQLite) — fine for the dozens-to-hundreds of sessions a project accrues, and
// it keeps the zero-dependency guarantee.
func Search(root, query string, limit int) []Hit {
	terms := queryTerms(query)
	if len(terms) == 0 {
		return nil
	}
	metas, _ := List(root)
	var hits []Hit
	for _, m := range metas {
		rec, err := Load(root, m.ID)
		if err != nil {
			continue
		}
		var sb strings.Builder
		for _, msg := range rec.Messages {
			if msg.Text != "" {
				sb.WriteString(msg.Text)
				sb.WriteByte('\n')
			}
		}
		lower := strings.ToLower(sb.String())
		score, first := 0, -1
		for _, t := range terms {
			if c := strings.Count(lower, t); c > 0 {
				score += c
				if i := strings.Index(lower, t); first < 0 || i < first {
					first = i
				}
			}
		}
		if score == 0 {
			continue
		}
		hits = append(hits, Hit{ID: m.ID, Title: m.Title, Updated: m.Updated, Score: score, Snippet: snippet(lower, first)})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Updated.After(hits[j].Updated)
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

func queryTerms(q string) []string {
	var out []string
	for _, f := range strings.Fields(strings.ToLower(q)) {
		if len(f) >= 2 {
			out = append(out, f)
		}
	}
	return out
}

// snippet returns a whitespace-collapsed window around idx (operating on the
// already-lowercased text, so byte indices are valid).
func snippet(text string, idx int) string {
	if idx < 0 {
		return ""
	}
	start, end := idx-60, idx+140
	if start < 0 {
		start = 0
	}
	if end > len(text) {
		end = len(text)
	}
	s := strings.Join(strings.Fields(text[start:end]), " ")
	if start > 0 {
		s = "…" + s
	}
	if end < len(text) {
		s += "…"
	}
	return s
}
