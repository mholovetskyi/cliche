package cli

import (
	"strings"
	"testing"
)

func TestProviderHint(t *testing.T) {
	cases := []struct {
		msg  string
		want string // substring expected in the hint ("" = no hint)
	}{
		{"api error: This request requires more credits, or fewer max_tokens", "credits"},
		{"api error: User not found.", "login"},
		{"openrouter: 429 Too Many Requests rate limit exceeded", "rate limited"},
		{"401 Unauthorized: invalid api key", "login"},
		{"model anthropic/nope-9 not found", "model id"},
		{"some unrelated network blip", ""},
	}
	for _, c := range cases {
		got := providerHint(c.msg)
		if c.want == "" {
			if got != "" {
				t.Errorf("providerHint(%q) = %q, want no hint", c.msg, got)
			}
			continue
		}
		if !strings.Contains(got, c.want) {
			t.Errorf("providerHint(%q) = %q, want substring %q", c.msg, got, c.want)
		}
	}
}
