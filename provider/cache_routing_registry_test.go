package provider

import "testing"

func TestBuiltinOpenAICompatibleFactoriesPreserveCacheRoutingPreference(t *testing.T) {
	t.Setenv("OPENAI_API", "chat-completions")
	registry := NewProviderRegistry()
	registerBuiltinProviders(registry)

	for _, providerName := range []string{
		"openai",
		"openai-responses",
		"ollama",
		"deepseek",
		"gemini",
		"groq",
		"mistral",
		"zhipu",
		"minimax",
		"kimi",
	} {
		t.Run(providerName, func(t *testing.T) {
			created, err := registry.Create(providerName, Config{
				APIKey:                 "test-key",
				CacheRoutingPreference: CacheRoutingOff,
			}, "cache-test-model")
			if err != nil {
				t.Fatal(err)
			}
			capabilityProvider, ok := created.(CapabilityProvider)
			if !ok {
				t.Fatalf("%s does not expose capabilities", providerName)
			}
			if got := capabilityProvider.Capabilities().CacheRouting; got != CacheRoutingNone {
				t.Fatalf("%s cache routing = %q, want off", providerName, got)
			}
		})
	}
}

func TestResolveCredentialConfigCarriesCacheRoutingEnvironment(t *testing.T) {
	t.Setenv("LUBAN_CODE_CACHE_ROUTING_MODE", "off")
	registry := NewProviderRegistry()
	cfg, err := ResolveCredentialConfig(registry, "openai")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CacheRoutingPreference != CacheRoutingOff {
		t.Fatalf("cache routing preference = %q, want off", cfg.CacheRoutingPreference)
	}
}

func TestBuiltinDeepSeekUsesCredentialScopedCacheRouting(t *testing.T) {
	registry := NewProviderRegistry()
	registerBuiltinProviders(registry)

	created, err := registry.Create("deepseek", Config{APIKey: "test-account-key"}, "deepseek-chat")
	if err != nil {
		t.Fatal(err)
	}
	retry, ok := created.(*RetryProvider)
	if !ok {
		t.Fatalf("DeepSeek factory returned %T, want *RetryProvider", created)
	}
	raw, ok := retry.inner.(*OpenAIProvider)
	if !ok {
		t.Fatalf("DeepSeek retry provider wraps %T, want *OpenAIProvider", retry.inner)
	}
	want := promptCacheUserNamespace(Config{ProviderName: "deepseek", APIKey: "test-account-key"})
	if raw.cacheUserNamespace == "" || raw.cacheUserNamespace != want {
		t.Fatalf("DeepSeek cache namespace = %q, want %q", raw.cacheUserNamespace, want)
	}
}
