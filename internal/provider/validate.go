package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrUnauthorized means the API rejected the key (HTTP 401/403) — a bad key, as
// distinct from a transient or network failure. Callers (e.g. `cliche login`)
// use it to decide whether to re-prompt or to offer saving unverified.
var ErrUnauthorized = errors.New("the API rejected this key")

// ValidateKey performs a lightweight, token-free authenticated GET against the
// provider's models endpoint to confirm a key actually works before saving it.
// baseURLOverride, when set, is the chat-completions endpoint whose host/prefix
// is reused for the models call. It returns nil on success, ErrUnauthorized on
// a rejected key, or another error on a transient/network/HTTP failure.
func ValidateKey(ctx context.Context, name, key, baseURLOverride string) error {
	req, err := validateRequest(ctx, name, key, baseURLOverride)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrUnauthorized
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return fmt.Errorf("models endpoint returned %s", resp.Status)
	}
	return nil
}

func validateRequest(ctx context.Context, name, key, baseURLOverride string) (*http.Request, error) {
	if name == "" || name == "anthropic" {
		url := "https://api.anthropic.com/v1/models"
		if baseURLOverride != "" {
			url = modelsURLFrom(baseURLOverride)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
		return req, nil
	}

	// Every other provider is OpenAI-compatible: hit its /models endpoint with a
	// Bearer token (authenticated → 401s on a bad key).
	var url string
	switch {
	case name == "openrouter":
		// OpenRouter's /models is PUBLIC (200 for any key); /auth/key is the
		// authenticated check, so use it regardless of the base URL.
		url = "https://openrouter.ai/api/v1/auth/key"
	case baseURLOverride != "":
		url = modelsURLFrom(baseURLOverride)
	case name == "openai":
		url = modelsURLFrom("https://api.openai.com/v1/chat/completions")
	default:
		return nil, fmt.Errorf("cannot validate provider %q without a base URL", name)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("authorization", "Bearer "+key)
	return req, nil
}

// modelsURLFrom derives the models endpoint from a chat-completions endpoint
// (…/chat/completions → …/models).
func modelsURLFrom(chatURL string) string {
	u := strings.TrimSuffix(chatURL, "/chat/completions")
	return strings.TrimRight(u, "/") + "/models"
}
