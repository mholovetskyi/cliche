package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateKey(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("authorization")
		switch r.URL.Path {
		case "/ok/models":
			w.WriteHeader(http.StatusOK)
		case "/bad/models":
			w.WriteHeader(http.StatusUnauthorized)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	ctx := context.Background()

	if err := ValidateKey(ctx, "openrouter", "k", srv.URL+"/ok/chat/completions"); err != nil {
		t.Fatalf("2xx should validate: %v", err)
	}
	if gotAuth != "Bearer k" {
		t.Fatalf("bearer auth header not sent, got %q", gotAuth)
	}
	if err := ValidateKey(ctx, "openrouter", "k", srv.URL+"/bad/chat/completions"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("401 should be ErrUnauthorized, got %v", err)
	}
	if err := ValidateKey(ctx, "openrouter", "k", srv.URL+"/err/chat/completions"); err == nil || errors.Is(err, ErrUnauthorized) {
		t.Fatalf("500 should be a non-auth error, got %v", err)
	}
	if err := ValidateKey(ctx, "bogus", "k", ""); err == nil {
		t.Fatal("unknown provider should error")
	}
}

func TestModelsURLFrom(t *testing.T) {
	cases := map[string]string{
		"https://openrouter.ai/api/v1/chat/completions": "https://openrouter.ai/api/v1/models",
		"https://api.openai.com/v1/chat/completions":    "https://api.openai.com/v1/models",
		"https://host/api/v1":                           "https://host/api/v1/models",
	}
	for in, want := range cases {
		if got := modelsURLFrom(in); got != want {
			t.Errorf("modelsURLFrom(%q) = %q, want %q", in, got, want)
		}
	}
}
