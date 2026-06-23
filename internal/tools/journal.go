package tools

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// EditJournal records every successful file mutation an OSExecutor makes during
// a session, so the user can see exactly what the agent changed (/diff) and
// revert it (/undo). Each entry captures the file's content immediately before
// the change, which is what makes an exact restore — including un-creating a
// file the agent created — possible. It is safe for concurrent use, so parallel
// subagents sharing one executor all record into the same journal.
//
// The journal is in-memory and session-scoped; it is deliberately not persisted
// (it can hold file contents, which the cost ledger never does).
type EditJournal struct {
	mu    sync.Mutex
	root  string
	stack []change
}

type change struct {
	path    string // absolute path on disk
	before  string // content prior to this mutation
	existed bool   // whether the file existed prior to this mutation
}

// NewEditJournal returns a journal that reports paths relative to root.
func NewEditJournal(root string) *EditJournal { return &EditJournal{root: root} }

// record appends one successful mutation. before/existed describe the file's
// state immediately before the write. A nil journal is a no-op, so callers need
// not branch on whether journaling is enabled.
func (j *EditJournal) record(path, before string, existed bool) {
	if j == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.stack = append(j.stack, change{path: path, before: before, existed: existed})
}

// FileChange is the net session change for one file: its content at the first
// time the session touched it (Before) versus now (After).
type FileChange struct {
	Path    string // path relative to the journal root, forward-slashed
	Before  string
	After   string
	WasNew  bool // the file did not exist when the session first touched it
	Deleted bool // the file no longer exists on disk
}

// Changes returns the net change per file (collapsing multiple edits to the
// same file into one before→after), in the order files were first touched.
// No-op nets (edited back to the original) are omitted.
func (j *EditJournal) Changes() []FileChange {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()

	firstAt := map[string]int{}
	var order []string
	for i, c := range j.stack {
		if _, seen := firstAt[c.path]; !seen {
			firstAt[c.path] = i
			order = append(order, c.path)
		}
	}
	var out []FileChange
	for _, p := range order {
		origin := j.stack[firstAt[p]]
		data, err := os.ReadFile(p)
		after, deleted := string(data), err != nil
		before := origin.before
		if !origin.existed {
			before = ""
		}
		if before == after && !deleted {
			continue // net no-op
		}
		out = append(out, FileChange{
			Path:    j.relPath(p),
			Before:  before,
			After:   after,
			WasNew:  !origin.existed,
			Deleted: deleted,
		})
	}
	return out
}

// Undo reverts the most recent recorded mutation, restoring the file to its
// state immediately before that op (deleting it if the op had created it). It
// returns the reverted file's display path, whether anything was undone, and
// any IO error.
func (j *EditJournal) Undo() (path string, did bool, err error) {
	if j == nil {
		return "", false, nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if len(j.stack) == 0 {
		return "", false, nil
	}
	c := j.stack[len(j.stack)-1]
	if c.existed {
		err = os.WriteFile(c.path, []byte(c.before), 0o644)
	} else if rmErr := os.Remove(c.path); rmErr != nil && !os.IsNotExist(rmErr) {
		err = rmErr
	}
	if err != nil {
		return "", true, err
	}
	j.stack = j.stack[:len(j.stack)-1]
	return j.relPath(c.path), true, nil
}

// PendingUndo previews the mutation Undo would revert next — WITHOUT reverting —
// returning the file's display path, the content it would be restored to, its
// current on-disk content, and ok=false when the journal is empty. Callers pass
// (current, restored) to PreviewChange to show the rollback diff.
func (j *EditJournal) PendingUndo() (path, restored, current string, ok bool) {
	if j == nil {
		return "", "", "", false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if len(j.stack) == 0 {
		return "", "", "", false
	}
	c := j.stack[len(j.stack)-1]
	restored = c.before
	if !c.existed {
		restored = "" // undo will delete a file this op created
	}
	data, _ := os.ReadFile(c.path)
	return j.relPath(c.path), restored, string(data), true
}

// relPath reports p relative to the journal root for display. The executor
// records absolute, symlink-resolved paths (see OSExecutor.resolve), so the
// root is normalized the same way before relativizing — otherwise a journal
// built with the default root "." would fail to relativize and leak the user's
// absolute filesystem layout into /diff, /undo, and the run summary.
// RewindAll reverts every file the session touched back to its state when the
// session first touched it (deleting files the agent created), then clears the
// journal — a one-shot "undo everything the agent did". Returns the reverted
// display paths. Stops at the first IO error, returning what was reverted so far.
func (j *EditJournal) RewindAll() ([]string, error) {
	if j == nil {
		return nil, nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	firstAt := map[string]int{}
	var order []string
	for i, c := range j.stack {
		if _, seen := firstAt[c.path]; !seen {
			firstAt[c.path] = i
			order = append(order, c.path)
		}
	}
	var reverted []string
	for _, p := range order {
		origin := j.stack[firstAt[p]]
		var err error
		if origin.existed {
			err = os.WriteFile(p, []byte(origin.before), 0o644)
		} else if rmErr := os.Remove(p); rmErr != nil && !os.IsNotExist(rmErr) {
			err = rmErr
		}
		if err != nil {
			return reverted, err
		}
		reverted = append(reverted, j.relPath(p))
	}
	j.stack = nil
	return reverted, nil
}

func (j *EditJournal) relPath(p string) string {
	root := j.root
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	if r, err := filepath.Rel(root, p); err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(r)
	}
	return filepath.ToSlash(p)
}

// PreviewChange renders a compact, bounded before→after diff in the same format
// used by approval prompts. Exported so callers (e.g. the /diff command) can
// render a journal entry without re-implementing the diff.
func PreviewChange(before, after string) string { return changePreview(before, after) }
