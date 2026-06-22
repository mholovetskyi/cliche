package cli

import (
	"regexp"

	"github.com/mholovetskyi/cliche/internal/config"
)

var providerNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// validProviderName reports whether s is a syntactically valid provider name
// (lowercase letters/digits/'-'/'_'). Any such name can hold a saved key; what
// makes it usable is a base URL (built-in, config, or --base-url).
func validProviderName(s string) bool { return providerNameRe.MatchString(s) }

// providerInfo is the metadata Cliche needs to talk to a model backend.
type providerInfo struct {
	label        string
	baseURL      string // OpenAI-compatible chat-completions endpoint (empty for the native Anthropic API)
	defaultModel string
	keysURL      string // where to get a key (shown in the login wizard)
	native       bool   // true = Anthropic Messages API; false = OpenAI-compatible
}

// builtinProviders are the presets shown in `cliche login` and selectable by
// name with no extra configuration. Everything except Anthropic speaks the
// OpenAI-compatible Chat Completions API, so adding a new hosted service is
// usually just another row here — or a `providers` entry in .cliche/config.json
// for anything not listed (including local servers like Ollama / LM Studio).
var builtinProviders = map[string]providerInfo{
	"anthropic":  {label: "Anthropic", native: true, defaultModel: "claude-sonnet-4-6", keysURL: "console.anthropic.com → API keys"},
	"openrouter": {label: "OpenRouter", baseURL: "https://openrouter.ai/api/v1/chat/completions", defaultModel: "openai/gpt-4o-mini", keysURL: "openrouter.ai/keys · one key, many models"},
	"openai":     {label: "OpenAI", baseURL: "https://api.openai.com/v1/chat/completions", defaultModel: "gpt-4o-mini", keysURL: "platform.openai.com/api-keys"},
	"groq":       {label: "Groq", baseURL: "https://api.groq.com/openai/v1/chat/completions", defaultModel: "llama-3.3-70b-versatile", keysURL: "console.groq.com/keys · very fast"},
	"deepseek":   {label: "DeepSeek", baseURL: "https://api.deepseek.com/chat/completions", defaultModel: "deepseek-chat", keysURL: "platform.deepseek.com/api_keys"},
	"mistral":    {label: "Mistral", baseURL: "https://api.mistral.ai/v1/chat/completions", defaultModel: "mistral-large-latest", keysURL: "console.mistral.ai/api-keys"},
	"together":   {label: "Together", baseURL: "https://api.together.xyz/v1/chat/completions", defaultModel: "meta-llama/Llama-3.3-70B-Instruct-Turbo", keysURL: "api.together.xyz/settings/api-keys"},
	"xai":        {label: "xAI (Grok)", baseURL: "https://api.x.ai/v1/chat/completions", defaultModel: "grok-2-latest", keysURL: "console.x.ai"},
}

// providerOrder is the auto-detection precedence (built-ins first, then any
// config-defined providers appended by lookups that need the full list).
var providerOrder = []string{"anthropic", "openrouter", "openai", "groq", "deepseek", "mistral", "together", "xai"}

// lookupProvider returns metadata for a provider, with config-defined providers
// taking precedence over (and able to extend or override) the built-ins.
func lookupProvider(cfg config.Config, name string) (providerInfo, bool) {
	if name == "" {
		name = "anthropic"
	}
	info, ok := builtinProviders[name]
	for _, p := range cfg.Providers {
		if p.Name != name {
			continue
		}
		if !ok {
			info = providerInfo{label: name}
		}
		if p.BaseURL != "" {
			info.baseURL = p.BaseURL
		}
		if p.DefaultModel != "" {
			info.defaultModel = p.DefaultModel
		}
		ok = true
	}
	return info, ok
}

// allProviderNames is the auto-detection order plus any config-only providers.
func allProviderNames(cfg config.Config) []string {
	names := append([]string(nil), providerOrder...)
	seen := map[string]bool{}
	for _, n := range names {
		seen[n] = true
	}
	for _, p := range cfg.Providers {
		if !seen[p.Name] {
			names = append(names, p.Name)
			seen[p.Name] = true
		}
	}
	return names
}
