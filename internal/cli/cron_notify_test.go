package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendNotifyWebhook(t *testing.T) {
	got := make(chan map[string]any, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &b)
		got <- b
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dest := sendNotify(srv.URL, "build the thing", "completed", "all done", 0.0123)
	if dest != "webhook" {
		t.Fatalf("expected webhook destination, got %q", dest)
	}
	select {
	case b := <-got:
		if b["prompt"] != "build the thing" || b["stop"] != "completed" || b["text"] != "all done" {
			t.Fatalf("webhook payload missing fields: %#v", b)
		}
	default:
		t.Fatal("webhook never received the POST")
	}
}

func TestSendNotifyUnconfigured(t *testing.T) {
	// telegram without env config, and an empty/unknown destination, both no-op.
	t.Setenv("CLICHE_TELEGRAM_TOKEN", "")
	t.Setenv("CLICHE_TELEGRAM_CHAT", "")
	if d := sendNotify("telegram", "p", "completed", "m", 0); d != "" {
		t.Fatalf("telegram without env config should be a no-op, got %q", d)
	}
	if d := sendNotify("", "p", "completed", "m", 0); d != "" {
		t.Fatalf("empty destination should be a no-op, got %q", d)
	}
}
