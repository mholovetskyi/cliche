// Package oauth implements the OAuth 2.0 Device Authorization Grant (RFC 8628)
// — the "go to this URL and enter this code" flow that's native to a terminal
// (no localhost redirect server, no browser round-trip required). It's how
// Cliche connects to OAuth-gated connectors (e.g. GitHub) seamlessly from the
// CLI. Pure stdlib (net/http, net/url, encoding/json).
package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DeviceConfig is a connector's device-flow endpoints + client. ClientID is
// BYO (you register an OAuth app once and supply its id), in the same spirit as
// BYO API keys — Cliche hosts no credentials.
type DeviceConfig struct {
	ClientID  string
	Scopes    []string
	DeviceURL string // device-authorization endpoint
	TokenURL  string // token endpoint
}

// DeviceCode is the device-authorization response shown to the user.
type DeviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// Token is a granted access token.
type Token struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

var (
	httpClient = &http.Client{Timeout: 30 * time.Second}
	// pollWait is the inter-poll sleep, indirected so tests run instantly.
	pollWait = func(d time.Duration) { time.Sleep(d) }
)

// RequestCode begins the flow: it returns the user_code + verification_uri to
// display and the device_code to poll with.
func RequestCode(ctx context.Context, cfg DeviceConfig) (*DeviceCode, error) {
	form := url.Values{"client_id": {cfg.ClientID}}
	if len(cfg.Scopes) > 0 {
		form.Set("scope", strings.Join(cfg.Scopes, " "))
	}
	var dc DeviceCode
	if err := postForm(ctx, cfg.DeviceURL, form, &dc); err != nil {
		return nil, err
	}
	if dc.DeviceCode == "" || dc.UserCode == "" {
		return nil, errors.New("device endpoint returned no code")
	}
	if dc.Interval <= 0 {
		dc.Interval = 5
	}
	return &dc, nil
}

// PollToken polls the token endpoint until the user authorizes, the code is
// denied, or it expires. It honors authorization_pending (keep waiting) and
// slow_down (back off), per RFC 8628.
func PollToken(ctx context.Context, cfg DeviceConfig, dc *DeviceCode) (*Token, error) {
	interval := time.Duration(dc.Interval) * time.Second
	for {
		form := url.Values{
			"client_id":   {cfg.ClientID},
			"device_code": {dc.DeviceCode},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		}
		var res struct {
			Token
			Error string `json:"error"`
		}
		if err := postForm(ctx, cfg.TokenURL, form, &res); err != nil {
			return nil, err
		}
		switch res.Error {
		case "":
			if res.AccessToken != "" {
				return &res.Token, nil
			}
			return nil, errors.New("token endpoint returned no access_token")
		case "authorization_pending":
			// not yet — wait and retry
		case "slow_down":
			interval += 5 * time.Second
		case "access_denied":
			return nil, errors.New("authorization was denied")
		case "expired_token":
			return nil, errors.New("the code expired — start the connection again")
		default:
			return nil, fmt.Errorf("oauth error: %s", res.Error)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			pollWait(interval)
		}
	}
}

func postForm(ctx context.Context, endpoint string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("oauth: unexpected response (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
