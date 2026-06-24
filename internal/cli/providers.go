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
	keysURL      string            // where to get a key (shown in the login wizard)
	native       bool              // true = Anthropic Messages API; false = OpenAI-compatible
	local        bool              // true = a local server (no API key required, e.g. Ollama)
	headers      map[string]string // extra request headers (config-defined gateway providers)
}

// builtinProviders are the presets shown in `cliche login` and selectable by
// name with no extra configuration. Everything except Anthropic speaks the
// OpenAI-compatible Chat Completions API, so adding a new hosted service is
// usually just another row here — or a `providers` entry in .cliche/config.json
// for anything not listed (including local servers like Ollama / LM Studio).
var builtinProviders = map[string]providerInfo{
	// Native Anthropic Messages API.
	"anthropic": {label: "Anthropic", native: true, defaultModel: "claude-sonnet-4-6", keysURL: "console.anthropic.com → API keys"},

	// Aggregators (one key, hundreds of models).
	"openrouter": {label: "OpenRouter", baseURL: "https://openrouter.ai/api/v1/chat/completions", defaultModel: "openai/gpt-4o-mini", keysURL: "openrouter.ai/keys · one key, many models"},

	// First-party hosted, OpenAI-compatible.
	"openai":     {label: "OpenAI", baseURL: "https://api.openai.com/v1/chat/completions", defaultModel: "gpt-4o-mini", keysURL: "platform.openai.com/api-keys"},
	"google":     {label: "Google (Gemini)", baseURL: "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions", defaultModel: "gemini-2.0-flash", keysURL: "aistudio.google.com/apikey"},
	"xai":        {label: "xAI (Grok)", baseURL: "https://api.x.ai/v1/chat/completions", defaultModel: "grok-2-latest", keysURL: "console.x.ai"},
	"deepseek":   {label: "DeepSeek", baseURL: "https://api.deepseek.com/chat/completions", defaultModel: "deepseek-chat", keysURL: "platform.deepseek.com/api_keys"},
	"mistral":    {label: "Mistral", baseURL: "https://api.mistral.ai/v1/chat/completions", defaultModel: "mistral-large-latest", keysURL: "console.mistral.ai/api-keys"},
	"cohere":     {label: "Cohere", baseURL: "https://api.cohere.ai/compatibility/v1/chat/completions", defaultModel: "command-r-plus", keysURL: "dashboard.cohere.com/api-keys"},
	"perplexity": {label: "Perplexity", baseURL: "https://api.perplexity.ai/chat/completions", defaultModel: "sonar", keysURL: "perplexity.ai/settings/api"},
	"moonshot":   {label: "Moonshot (Kimi)", baseURL: "https://api.moonshot.cn/v1/chat/completions", defaultModel: "moonshot-v1-8k", keysURL: "platform.moonshot.cn/console/api-keys"},
	"zhipu":      {label: "Zhipu (GLM)", baseURL: "https://open.bigmodel.cn/api/paas/v4/chat/completions", defaultModel: "glm-4-plus", keysURL: "open.bigmodel.cn"},
	"github":     {label: "GitHub Models", baseURL: "https://models.inference.ai.azure.com/chat/completions", defaultModel: "gpt-4o-mini", keysURL: "github.com/settings/tokens (a PAT)"},

	// Fast inference clouds (open-weight models).
	"groq":       {label: "Groq", baseURL: "https://api.groq.com/openai/v1/chat/completions", defaultModel: "llama-3.3-70b-versatile", keysURL: "console.groq.com/keys · very fast"},
	"cerebras":   {label: "Cerebras", baseURL: "https://api.cerebras.ai/v1/chat/completions", defaultModel: "llama-3.3-70b", keysURL: "cloud.cerebras.ai"},
	"together":   {label: "Together", baseURL: "https://api.together.xyz/v1/chat/completions", defaultModel: "meta-llama/Llama-3.3-70B-Instruct-Turbo", keysURL: "api.together.xyz/settings/api-keys"},
	"fireworks":  {label: "Fireworks", baseURL: "https://api.fireworks.ai/inference/v1/chat/completions", defaultModel: "accounts/fireworks/models/llama-v3p3-70b-instruct", keysURL: "fireworks.ai/account/api-keys"},
	"deepinfra":  {label: "DeepInfra", baseURL: "https://api.deepinfra.com/v1/openai/chat/completions", defaultModel: "meta-llama/Llama-3.3-70B-Instruct", keysURL: "deepinfra.com/dash/api_keys"},
	"nvidia":     {label: "NVIDIA NIM", baseURL: "https://integrate.api.nvidia.com/v1/chat/completions", defaultModel: "meta/llama-3.3-70b-instruct", keysURL: "build.nvidia.com"},
	"sambanova":  {label: "SambaNova", baseURL: "https://api.sambanova.ai/v1/chat/completions", defaultModel: "Meta-Llama-3.3-70B-Instruct", keysURL: "cloud.sambanova.ai"},
	"hyperbolic": {label: "Hyperbolic", baseURL: "https://api.hyperbolic.xyz/v1/chat/completions", defaultModel: "meta-llama/Llama-3.3-70B-Instruct", keysURL: "app.hyperbolic.xyz/settings"},
	"novita":     {label: "Novita", baseURL: "https://api.novita.ai/v3/openai/chat/completions", defaultModel: "meta-llama/llama-3.3-70b-instruct", keysURL: "novita.ai/settings/key-management"},

	// Local servers — no API key required (OpenAI-compatible).
	"ollama":   {label: "Ollama (local)", baseURL: "http://localhost:11434/v1/chat/completions", defaultModel: "llama3.2", local: true},
	"lmstudio": {label: "LM Studio (local)", baseURL: "http://localhost:1234/v1/chat/completions", defaultModel: "local-model", local: true},
	"vllm":     {label: "vLLM (local)", baseURL: "http://localhost:8000/v1/chat/completions", defaultModel: "local-model", local: true},
}

// providerOrder is the auto-detection precedence: a configured key for an earlier
// provider wins. Cheap/aggregator first, then first-party, then inference clouds.
// Local servers are excluded from auto-detect (they need no key, so they'd never
// be "detected" — you opt in with --provider ollama).
var providerOrder = []string{
	"anthropic", "openrouter", "openai", "google", "xai", "deepseek", "mistral",
	"cohere", "perplexity", "moonshot", "zhipu", "github",
	"groq", "cerebras", "together", "fireworks", "deepinfra", "nvidia", "sambanova", "hyperbolic", "novita",
}

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
		if len(p.Headers) > 0 {
			info.headers = p.Headers
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
