package tools

import (
	"context"
	"testing"
)

func TestPostToolHookFires(t *testing.T) {
	var name string
	var ok bool
	calls := 0
	e := OSExecutor{
		Root:   t.TempDir(),
		Policy: Policy{Yolo: true},
		PostToolHook: func(n string, _ map[string]string, k bool) {
			name, ok, calls = n, k, calls+1
		},
	}
	r := e.Execute(context.Background(), "list_files", map[string]string{})
	if calls != 1 || name != "list_files" || ok != r.Success {
		t.Fatalf("post hook: calls=%d name=%q ok=%v (result.Success=%v)", calls, name, ok, r.Success)
	}
}
