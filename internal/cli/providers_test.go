package cli

import (
	"testing"

	"github.com/mholovetskyi/cliche/internal/config"
)

func TestProviderCatalogIsBroadAndConsistent(t *testing.T) {
	if len(builtinProviders) < 20 {
		t.Fatalf("expected a broad provider catalog, got %d", len(builtinProviders))
	}
	// Every auto-detect name is a real, non-local built-in with an endpoint.
	for _, n := range providerOrder {
		p, ok := builtinProviders[n]
		if !ok {
			t.Errorf("providerOrder lists %q but it's not in builtinProviders", n)
			continue
		}
		if p.local {
			t.Errorf("local provider %q must not be in the auto-detect order (no key to detect)", n)
		}
		if !p.native && p.baseURL == "" {
			t.Errorf("provider %q has no base URL", n)
		}
	}
	// A spread of the catalog is present.
	for _, n := range []string{"google", "perplexity", "cohere", "fireworks", "cerebras", "deepinfra", "nvidia", "sambanova", "ollama", "lmstudio"} {
		if _, ok := builtinProviders[n]; !ok {
			t.Errorf("expected built-in provider %q", n)
		}
	}
}

func TestResolveBackendLocalNeedsNoKey(t *testing.T) {
	t.Setenv("CLICHE_CONFIG_HOME", t.TempDir()) // no saved creds
	t.Setenv("OLLAMA_API_KEY", "")              // and no env key

	b, err := resolveBackend(config.Config{Provider: "ollama"}, &runFlags{})
	if err != nil {
		t.Fatalf("a local provider must resolve without any API key: %v", err)
	}
	if b.name != "ollama" || b.model != "llama3.2" || b.baseURL == "" {
		t.Fatalf("ollama backend = %+v", b)
	}
}
