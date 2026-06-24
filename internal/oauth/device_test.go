package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDeviceFlowEndToEnd(t *testing.T) {
	pollWait = func(time.Duration) {} // no real sleeping in the test
	defer func() { pollWait = func(d time.Duration) { time.Sleep(d) } }()

	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/device":
			_ = r.ParseForm()
			if r.FormValue("client_id") != "cid" {
				t.Errorf("missing client_id: %v", r.Form)
			}
			w.Write([]byte(`{"device_code":"DEV","user_code":"WXYZ-1234","verification_uri":"https://example.test/device","expires_in":900,"interval":1}`))
		case "/token":
			polls++
			if polls < 3 { // pending twice, then success
				w.Write([]byte(`{"error":"authorization_pending"}`))
				return
			}
			w.Write([]byte(`{"access_token":"gho_TOKEN","token_type":"bearer","scope":"repo"}`))
		}
	}))
	defer srv.Close()

	cfg := DeviceConfig{ClientID: "cid", Scopes: []string{"repo"}, DeviceURL: srv.URL + "/device", TokenURL: srv.URL + "/token"}
	dc, err := RequestCode(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if dc.UserCode != "WXYZ-1234" || dc.DeviceCode != "DEV" {
		t.Fatalf("device code = %+v", dc)
	}
	tok, err := PollToken(context.Background(), cfg, dc)
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "gho_TOKEN" {
		t.Fatalf("token = %+v", tok)
	}
	if polls != 3 {
		t.Fatalf("expected 3 polls (pending,pending,ok), got %d", polls)
	}
}

func TestDeviceFlowDenied(t *testing.T) {
	pollWait = func(time.Duration) {}
	defer func() { pollWait = func(d time.Duration) { time.Sleep(d) } }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"error":"access_denied"}`))
	}))
	defer srv.Close()

	cfg := DeviceConfig{ClientID: "cid", TokenURL: srv.URL}
	_, err := PollToken(context.Background(), cfg, &DeviceCode{DeviceCode: "DEV", Interval: 1})
	if err == nil {
		t.Fatal("denied authorization should error")
	}
}
