package ledger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChainVerifiesCleanLedger(t *testing.T) {
	dir := t.TempDir()
	l, _ := Open(dir)
	for i := 0; i < 5; i++ {
		if err := l.Append(Entry{Event: EventTurn, Turn: i, Detail: "step"}); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := l.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK || rep.Verified != 5 || rep.Entries != 5 || rep.Legacy != 0 {
		t.Fatalf("clean chain should fully verify: %+v", rep)
	}
}

func TestChainDetectsAlteredEntry(t *testing.T) {
	dir := t.TempDir()
	l, _ := Open(dir)
	for i := 0; i < 4; i++ {
		l.Append(Entry{Event: EventTool, Detail: fmt.Sprintf("call-%d", i)})
	}
	// Tamper with entry 2's content in place (Hash left unchanged).
	rewriteLine(t, filepath.Join(dir, "ledger.jsonl"), 1, func(s string) string {
		return strings.Replace(s, "call-1", "call-HACKED", 1)
	})
	rep, _ := l.Verify()
	if rep.OK || rep.BrokenAt != 2 {
		t.Fatalf("altered content should be caught at entry 2: %+v", rep)
	}
}

func TestChainDetectsDeletion(t *testing.T) {
	dir := t.TempDir()
	l, _ := Open(dir)
	for i := 0; i < 4; i++ {
		l.Append(Entry{Event: EventTool, Detail: fmt.Sprintf("c%d", i)})
	}
	// Delete entry 2 — the next entry's PrevHash no longer links.
	path := filepath.Join(dir, "ledger.jsonl")
	lines := readLines(t, path)
	lines = append(lines[:1], lines[2:]...)
	writeLines(t, path, lines)

	rep, _ := l.Verify()
	if rep.OK {
		t.Fatalf("deletion should break the chain: %+v", rep)
	}
}

// helpers

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

func writeLines(t *testing.T, path string, lines []string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func rewriteLine(t *testing.T, path string, idx int, f func(string) string) {
	t.Helper()
	lines := readLines(t, path)
	lines[idx] = f(lines[idx])
	writeLines(t, path, lines)
}
