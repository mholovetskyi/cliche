package agent

import "testing"

// TestArgSummaryNamesTheTarget guards that every built-in tool yields a feed
// detail (the salient argument), so the activity feed and spinner never show a
// bare verb when an argument is available.
func TestArgSummaryNamesTheTarget(t *testing.T) {
	cases := []struct {
		args map[string]string
		want string
	}{
		{map[string]string{"file": "main.go"}, "main.go"},                         // read/edit/write
		{map[string]string{"command": "go test ./..."}, "go test ./..."},          // run
		{map[string]string{"url": "https://pkg.go.dev"}, "https://pkg.go.dev"},    // fetch
		{map[string]string{"pattern": "TODO", "path": "internal"}, "TODO"},        // search/find: pattern over path
		{map[string]string{"path": "internal/cli"}, "internal/cli"},               // list
		{map[string]string{"prompt": "summarize the repo"}, "summarize the repo"}, // subagent
		{map[string]string{}, ""},
	}
	for _, c := range cases {
		if got := argSummary(c.args); got != c.want {
			t.Errorf("argSummary(%v) = %q, want %q", c.args, got, c.want)
		}
	}
}
