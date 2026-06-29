package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientRoundTrip(t *testing.T) {
	var sent map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "getUpdates"):
			w.Write([]byte(`{"ok":true,"result":[{"update_id":5,"message":{"message_id":1,"chat":{"id":42},"text":"hi"}}]}`))
		case strings.Contains(r.URL.Path, "sendMessage"):
			_ = json.NewDecoder(r.Body).Decode(&sent)
			w.Write([]byte(`{"ok":true,"result":{}}`))
		default:
			http.Error(w, "no", 404)
		}
	}))
	defer srv.Close()

	c := New("tok")
	c.api = srv.URL

	ups, err := c.GetUpdates(context.Background(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ups) != 1 || ups[0].Message == nil || ups[0].Message.Chat.ID != 42 || ups[0].Message.Text != "hi" {
		t.Fatalf("getUpdates parsed wrong: %+v", ups)
	}
	if err := c.SendMessage(context.Background(), 42, "yo"); err != nil {
		t.Fatal(err)
	}
	if sent["text"] != "yo" || sent["chat_id"].(float64) != 42 {
		t.Fatalf("sendMessage payload wrong: %v", sent)
	}
}

func TestGetUpdatesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":false,"error_code":401,"description":"Unauthorized"}`))
	}))
	defer srv.Close()
	c := New("bad")
	c.api = srv.URL
	if _, err := c.GetUpdates(context.Background(), 0, 0); err == nil {
		t.Fatal("expected an error on ok:false")
	}
}
