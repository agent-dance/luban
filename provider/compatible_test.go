package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeModelIdentifierIgnoresCaseAndSeparators(t *testing.T) {
	if got, want := NormalizeModelIdentifier("GPT_4-O"), NormalizeModelIdentifier("gpt-4o"); got != want {
		t.Fatalf("normalized identifiers differ: %q != %q", got, want)
	}
}

func TestCompatibleProviderDisplayNameAndIdentifierFallback(t *testing.T) {
	registry := NewProviderRegistry()
	baseURL := "https://gateway.example.com/openai/v1"
	if got := CompatibleProviderDisplayName("", baseURL); got != "gateway.example.com" {
		t.Fatalf("display name = %q", got)
	}
	first := registry.NextUserProviderName("", baseURL)
	registry.RegisterCompatibleProvider(CompatibleProviderDefinition{
		Name: first, DisplayName: "gateway.example.com", UserDefined: true,
	})
	if second := registry.NextUserProviderName("", baseURL); second != first+"-2" {
		t.Fatalf("second provider identifier = %q, want %q", second, first+"-2")
	}
}

func TestCompatibleModelsURLRejectsUnsafeSchemesAndCredentials(t *testing.T) {
	for _, raw := range []string{"ftp://gateway.example.com/v1", "https://user:secret@gateway.example.com/v1", "gateway.example.com/v1"} {
		if _, err := compatibleModelsURL(raw, APIStyleOpenAI); err == nil {
			t.Fatalf("compatibleModelsURL(%q) succeeded", raw)
		}
	}
}

func TestDiscoverCompatibleModelsReusesNormalizedBuiltinMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		fmt.Fprint(w, `{"data":[{"id":"GPT_5.6"}]}`)
	}))
	defer server.Close()

	registry := NewProviderRegistry()
	registry.catalog = DefaultCatalog()
	models, err := registry.DiscoverCompatibleModels(context.Background(), CompatibleModelRequest{
		Provider: "custom-gateway", APIStyle: APIStyleOpenAI,
		BaseURL: server.URL + "/v1", APIKey: "test-key",
	})
	if err != nil {
		t.Fatalf("DiscoverCompatibleModels: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("models = %d, want 1", len(models))
	}
	model := models[0]
	if model.ID != "GPT_5.6" || model.Provider != "custom-gateway" {
		t.Fatalf("identity = %q/%q", model.Provider, model.ID)
	}
	if model.ContextWindow == 0 || !model.CanReason || model.CostPer1MIn == 0 {
		t.Fatalf("built-in metadata was not reused: %+v", model)
	}
	if model.APIFormat != "responses" {
		t.Fatalf("APIFormat = %q", model.APIFormat)
	}
}

func TestDiscoverCompatibleModelsAnthropicMetadataOverridesBuiltin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/gateway/v1/models" {
			t.Fatalf("path = %q, want /gateway/v1/models", request.URL.Path)
		}
		if request.Header.Get("X-Api-Key") != "secret" || request.Header.Get("anthropic-version") == "" {
			t.Fatalf("anthropic headers missing: %v", request.Header)
		}
		fmt.Fprint(w, `{"data":[{"id":"claude_sonnet_5","display_name":"Gateway Sonnet","max_input_tokens":12345,"max_tokens":678,"capabilities":{"vision":false,"thinking":true,"tool_use":true},"reasoning_efforts":["low","high"]}]}`)
	}))
	defer server.Close()

	registry := NewProviderRegistry()
	registry.catalog = DefaultCatalog()
	models, err := registry.DiscoverCompatibleModels(context.Background(), CompatibleModelRequest{
		Provider: "aggregate", APIStyle: APIStyleAnthropic,
		BaseURL: server.URL + "/gateway", APIKey: "secret",
	})
	if err != nil {
		t.Fatalf("DiscoverCompatibleModels: %v", err)
	}
	model := models[0]
	if model.Name != "Gateway Sonnet" || model.ContextWindow != 12345 || model.MaxOutput != 678 {
		t.Fatalf("remote metadata was not applied: %+v", model)
	}
	if model.CanSeeImages || !model.CanReason || !model.CanUseTools {
		t.Fatalf("remote capabilities were not applied: %+v", model)
	}
	if len(model.ReasoningEfforts) != 2 || model.APIFormat != "messages" {
		t.Fatalf("reasoning/protocol metadata = %+v", model)
	}
}

func TestDiscoverCompatibleModelsParsesAggregateMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"relay-model","architecture":{"input_modalities":["text","image"]},"top_provider":{"max_completion_tokens":4096},"supported_parameters":["tools","reasoning_effort"],"pricing":{"prompt":"0.000001","completion":"0.000002","cache_read":"0.0000001","currency":"CNY"}}]}`)
	}))
	defer server.Close()

	registry := NewProviderRegistry()
	models, err := registry.DiscoverCompatibleModels(context.Background(), CompatibleModelRequest{
		Provider: "aggregate", APIStyle: APIStyleOpenAI, BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("DiscoverCompatibleModels: %v", err)
	}
	model := models[0]
	if model.MaxOutput != 4096 || !model.CanSeeImages || !model.CanUseTools || !model.CanReason {
		t.Fatalf("aggregate capabilities were not parsed: %+v", model)
	}
	if model.CostPer1MIn != 1 || model.CostPer1MOut != 2 || model.CacheReadPer1M < 0.099 || model.CacheReadPer1M > 0.101 || model.CostCurrency != "CNY" {
		t.Fatalf("aggregate pricing was not parsed: %+v", model)
	}
}

func TestDiscoverCompatibleModelsDoesNotReuseAnotherDynamicProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"SHARED_MODEL"}]}`)
	}))
	defer server.Close()

	registry := NewProviderRegistry()
	registry.catalog.Register(ModelInfo{
		ID: "shared-model", Provider: "a-relay", Name: "Relay metadata",
		ContextWindow: 111, CostCurrency: "USD",
	})
	registry.catalog.Register(ModelInfo{
		ID: "shared-model", Provider: "z-static", Name: "Built-in metadata",
		ContextWindow: 222, CostCurrency: "USD",
	})
	registry.Register(ProviderInfo{Name: "a-relay", DynamicModels: true}, nil)

	models, err := registry.DiscoverCompatibleModels(context.Background(), CompatibleModelRequest{
		Provider: "new-relay", APIStyle: APIStyleOpenAI, BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("DiscoverCompatibleModels: %v", err)
	}
	if models[0].Name != "Built-in metadata" || models[0].ContextWindow != 222 {
		t.Fatalf("metadata came from another dynamic provider: %+v", models[0])
	}
}

func TestCompatibleBuiltinsExposeProtocolSpecificDefaults(t *testing.T) {
	registry := DefaultRegistry()
	tests := []struct {
		name          string
		openAIHost    string
		anthropicPath string
	}{
		{"volcengine", "ark.cn-beijing.volces.com", "/api/coding"},
		{"alibaba-cloud", "coding.dashscope.aliyuncs.com", "/apps/anthropic"},
		{"tencent-cloud", "api.lkeap.cloud.tencent.com", "/coding/anthropic"},
	}
	for _, test := range tests {
		info, ok := registry.Get(test.name)
		if !ok {
			t.Fatalf("provider %q not registered", test.name)
		}
		if !info.DynamicModels || info.UserDefined || len(info.APIStyles) != 2 {
			t.Fatalf("provider metadata for %q = %+v", test.name, info)
		}
		if got := info.BaseURLForStyle(APIStyleOpenAI); !strings.Contains(got, test.openAIHost) {
			t.Fatalf("OpenAI Base URL for %q = %q", test.name, got)
		}
		if got := info.BaseURLForStyle(APIStyleAnthropic); !strings.Contains(got, test.anthropicPath) {
			t.Fatalf("Anthropic Base URL for %q = %q", test.name, got)
		}
		if registry.UnregisterUserProvider(test.name) {
			t.Fatalf("built-in provider %q was deletable", test.name)
		}
	}
}

func TestCompatibleFactorySelectsConfiguredWireProtocol(t *testing.T) {
	registry := NewProviderRegistry()
	registry.RegisterCompatibleProvider(CompatibleProviderDefinition{
		Name: "aggregate", DisplayName: "Aggregate",
		BaseURLs: map[APIStyle]string{
			APIStyleOpenAI: "https://openai.example/v1", APIStyleAnthropic: "https://anthropic.example",
		},
	})

	openAIProvider, err := registry.Create("aggregate", Config{APIKey: "key", APIStyle: APIStyleOpenAI}, "model-a")
	if err != nil {
		t.Fatal(err)
	}
	openAIRetry, ok := openAIProvider.(*RetryProvider)
	if !ok {
		t.Fatalf("OpenAI provider = %T", openAIProvider)
	}
	if _, ok := openAIRetry.inner.(*OpenAIProvider); !ok || openAIProvider.Name() != "aggregate" {
		t.Fatalf("OpenAI inner/name = %T/%q", openAIRetry.inner, openAIProvider.Name())
	}

	anthropicProvider, err := registry.Create("aggregate", Config{APIKey: "key", APIStyle: APIStyleAnthropic}, "model-b")
	if err != nil {
		t.Fatal(err)
	}
	anthropicRetry, ok := anthropicProvider.(*RetryProvider)
	if !ok {
		t.Fatalf("Anthropic provider = %T", anthropicProvider)
	}
	if _, ok := anthropicRetry.inner.(*AnthropicProvider); !ok || anthropicProvider.Name() != "aggregate" {
		t.Fatalf("Anthropic inner/name = %T/%q", anthropicRetry.inner, anthropicProvider.Name())
	}
}

func TestCredentialStoreRestoresAndDeletesUserCompatibleProvider(t *testing.T) {
	store, err := NewCredentialStoreAt(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	entry := CredentialEntry{
		Provider: "custom-example.com", AuthMethod: "api_key", APIKey: "key",
		BaseURL: "https://example.com/v1", APIStyle: APIStyleAnthropic, APIFormat: "chat-completions",
		DisableStrictTools:        true,
		DisablePromptCacheOptions: true,
		DisplayName:               "Example", UserDefined: true,
		Models: []ModelInfo{{ID: "model-a", Provider: "custom-example.com", Name: "Model A", CostCurrency: "USD"}},
	}
	if err := store.Set(entry); err != nil {
		t.Fatal(err)
	}
	registry := NewProviderRegistry()
	registry.SetCredentialStore(store)
	if info, ok := registry.Get(entry.Provider); !ok || !info.UserDefined || info.DisplayName != entry.DisplayName {
		t.Fatalf("restored provider = %+v, %v", info, ok)
	}
	if models := registry.Catalog().ListByProvider(entry.Provider); len(models) != 1 || models[0].ID != "model-a" {
		t.Fatalf("restored models = %+v", models)
	}
	resolved, err := ResolveCredentialConfig(registry, entry.Provider)
	if err != nil {
		t.Fatalf("ResolveCredentialConfig: %v", err)
	}
	if resolved.APIStyle != entry.APIStyle || resolved.APIFormat != entry.APIFormat || resolved.BaseURL != entry.BaseURL || resolved.APIKey != entry.APIKey || !resolved.DisableStrictTools || !resolved.DisablePromptCacheOptions {
		t.Fatalf("restored config = %+v", resolved)
	}
	if !registry.UnregisterUserProvider(entry.Provider) {
		t.Fatal("expected user provider deletion to succeed")
	}
	if _, ok := registry.Get(entry.Provider); ok {
		t.Fatal("provider still registered after deletion")
	}
}
