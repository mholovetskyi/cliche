// Package secrets resolves and persists BYO-key API credentials, so a user
// configures a provider once instead of re-exporting an environment variable in
// every shell. Resolution order is env var first (always wins, good for CI),
// then a per-user credentials file.
//
// The credentials file is plaintext with 0600 permissions, stored under the
// user's OS config directory (NOT the project, so it is never committed). This
// is the same posture as `gh`/`aws` CLIs; an OS-keychain backend is a planned
// hardening step (see ROADMAP) — this package is the seam it will slot into.
package secrets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// EnvVar returns the environment variable holding a provider's key, by the
// universal convention <PROVIDER>_API_KEY (e.g. groq → GROQ_API_KEY). This lets
// any provider — built-in or user-defined — resolve a key from the environment
// without a hardcoded table. An empty provider is treated as Anthropic.
func EnvVar(provider string) string {
	if provider == "" {
		provider = "anthropic"
	}
	up := strings.ToUpper(strings.ReplaceAll(provider, "-", "_"))
	return up + "_API_KEY"
}

// baseDir is the per-user config directory for cliche. CLICHE_CONFIG_HOME
// overrides it (useful for tests and for relocating credentials).
func baseDir() (string, error) {
	if d := os.Getenv("CLICHE_CONFIG_HOME"); d != "" {
		return d, nil
	}
	ucd, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(ucd, "cliche"), nil
}

// CredentialsPath is the absolute path to the credentials file ("" if the user
// config directory cannot be determined).
func CredentialsPath() string {
	d, err := baseDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "credentials.json")
}

func load() (map[string]string, error) {
	creds := map[string]string{}
	path := CredentialsPath()
	if path == "" {
		return creds, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return creds, nil
		}
		return creds, err
	}
	if err := json.Unmarshal(data, &creds); err != nil {
		return map[string]string{}, err
	}
	return creds, nil
}

// Lookup returns the API key for a provider and a human-readable source
// ("env:NAME" or "file:PATH"), or ("","") if none is configured. The
// environment always wins so CI and one-off overrides behave predictably.
func Lookup(provider string) (key, source string) {
	if provider == "" {
		provider = "anthropic"
	}
	if env := EnvVar(provider); env != "" {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			return v, "env:" + env
		}
	}
	if creds, err := load(); err == nil {
		if v := strings.TrimSpace(creds[provider]); v != "" {
			return v, "file:" + CredentialsPath()
		}
	}
	return "", ""
}

// Saved returns the key stored in the credentials file for a provider, ignoring
// the environment, or "" if none. Used to detect an env var shadowing a saved
// key (which silently overrides it and is a common source of confusion).
func Saved(provider string) string {
	if provider == "" {
		provider = "anthropic"
	}
	creds, err := load()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(creds[provider])
}

// Save persists a provider's key to the credentials file (creating it 0600),
// returning the path written.
func Save(provider, key string) (string, error) {
	dir, err := baseDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	creds, err := load()
	if err != nil {
		return "", err
	}
	creds[provider] = strings.TrimSpace(key)
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "credentials.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// Remove deletes a provider's stored key (a missing entry is not an error).
// It does not affect environment variables.
func Remove(provider string) error {
	creds, err := load()
	if err != nil {
		return err
	}
	if _, ok := creds[provider]; !ok {
		return nil
	}
	delete(creds, provider)
	dir, err := baseDir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "credentials.json"), append(data, '\n'), 0o600)
}
