package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func TestNewProviderRegistry_Empty(t *testing.T) {
	r := NewProviderRegistry()
	if len(r.All()) != 0 {
		t.Fatalf("expected empty registry, got %d providers", len(r.All()))
	}
}

func TestProviderRegistry_RegisterAndGet(t *testing.T) {
	r := NewProviderRegistry()
	r.Register(ProviderInfo{
		Name:         "test",
		DisplayName:  "Test Provider",
		EnvKey:       "TEST_API_KEY",
		DefaultModel: "test-model",
		Popularity:   50,
	}, func(cfg Config, modelOverride string) (Provider, error) {
		return NewAnthropic(Config{APIKey: "fake", Model: "test-model"}), nil
	})

	info, ok := r.Get("test")
	if !ok {
		t.Fatal("expected to find test provider")
	}
	if info.DisplayName != "Test Provider" {
		t.Errorf("expected display name %q, got %q", "Test Provider", info.DisplayName)
	}
}

func TestProviderRegistry_GetMissing(t *testing.T) {
	r := NewProviderRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Fatal("expected not found for nonexistent provider")
	}
}

func TestProviderRegistry_Create(t *testing.T) {
	r := NewProviderRegistry()
	r.Register(ProviderInfo{
		Name:         "fake",
		DisplayName:  "Fake",
		DefaultModel: "fake-model",
	}, func(cfg Config, modelOverride string) (Provider, error) {
		model := modelOverride
		if model == "" {
			model = "fake-model"
		}
		return NewAnthropic(Config{APIKey: "fake", Model: model}), nil
	})

	p, err := r.Create("fake", Config{}, "custom-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ModelID() != "custom-model" {
		t.Errorf("expected model custom-model, got %q", p.ModelID())
	}
}

func TestProviderRegistry_CreateUnknown(t *testing.T) {
	r := NewProviderRegistry()
	_, err := r.Create("nonexistent", Config{}, "")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestOpenAIRegistryAcceptsOAuthAuthTokenWithoutAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")

	r := NewProviderRegistry()
	registerOpenAI(r)

	p, err := r.Create("openai", Config{
		AuthToken: "chatgpt-access-token",
		BaseURL:   openAIChatGPTCodexBaseURL,
		Headers:   openAICodexHeaders(),
	}, "gpt-5.4")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	retry, ok := p.(*RetryProvider)
	if !ok {
		t.Fatalf("provider type = %T, want *RetryProvider", p)
	}
	responses, ok := retry.inner.(*ResponsesProvider)
	if !ok {
		t.Fatalf("inner provider type = %T, want *ResponsesProvider", retry.inner)
	}
	if responses.apiKey != "chatgpt-access-token" {
		t.Fatalf("bearer token = %q", responses.apiKey)
	}
	if responses.baseURL != openAIChatGPTCodexBaseURL {
		t.Fatalf("baseURL = %q", responses.baseURL)
	}
}

func TestProviderRegistry_All_SortedByPopularity(t *testing.T) {
	r := NewProviderRegistry()
	noop := func(cfg Config, modelOverride string) (Provider, error) {
		return NewAnthropic(Config{APIKey: "fake"}), nil
	}
	r.Register(ProviderInfo{Name: "low", Popularity: 10}, noop)
	r.Register(ProviderInfo{Name: "high", Popularity: 100}, noop)
	r.Register(ProviderInfo{Name: "mid", Popularity: 50}, noop)

	all := r.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(all))
	}
	if all[0].Name != "high" {
		t.Errorf("expected highest popularity first, got %q", all[0].Name)
	}
	if all[1].Name != "mid" {
		t.Errorf("expected mid popularity second, got %q", all[1].Name)
	}
	if all[2].Name != "low" {
		t.Errorf("expected lowest popularity last, got %q", all[2].Name)
	}
}

func TestProviderRegistry_Names(t *testing.T) {
	r := NewProviderRegistry()
	noop := func(cfg Config, modelOverride string) (Provider, error) {
		return NewAnthropic(Config{APIKey: "fake"}), nil
	}
	r.Register(ProviderInfo{Name: "charlie"}, noop)
	r.Register(ProviderInfo{Name: "alpha"}, noop)
	r.Register(ProviderInfo{Name: "bravo"}, noop)

	names := r.Names()
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d", len(names))
	}
	if names[0] != "alpha" || names[1] != "bravo" || names[2] != "charlie" {
		t.Errorf("expected alphabetical order, got %v", names)
	}
}

func TestCanonicalProviderName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"DeepSeek", "deepseek"},
		{" OpenAI ", "openai"},
	}
	for _, tt := range tests {
		if got := CanonicalProviderName(tt.name); got != tt.want {
			t.Fatalf("CanonicalProviderName(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestProviderRegistry_Visible_HidesHiddenProviders(t *testing.T) {
	r := NewProviderRegistry()
	noop := func(cfg Config, modelOverride string) (Provider, error) {
		return NewAnthropic(Config{APIKey: "fake"}), nil
	}
	r.Register(ProviderInfo{Name: "visible", Popularity: 20}, noop)
	r.Register(ProviderInfo{Name: "hidden", Hidden: true, Popularity: 100}, noop)

	visible := r.Visible()
	if len(visible) != 1 || visible[0].Name != "visible" {
		t.Fatalf("Visible() = %+v, want only visible provider", visible)
	}
	if names := r.VisibleNames(); len(names) != 1 || names[0] != "visible" {
		t.Fatalf("VisibleNames() = %v, want [visible]", names)
	}
	if len(r.All()) != 2 {
		t.Fatalf("All() should keep hidden aliases resolvable")
	}
}

func TestProviderRegistry_Available(t *testing.T) {
	r := NewProviderRegistry()
	noop := func(cfg Config, modelOverride string) (Provider, error) {
		return NewAnthropic(Config{APIKey: "fake"}), nil
	}

	// Provider with API key set
	r.Register(ProviderInfo{Name: "with-key", EnvKey: "TEST_AVAILABLE_KEY", Popularity: 50}, noop)
	// Provider without API key
	r.Register(ProviderInfo{Name: "without-key", EnvKey: "TEST_UNAVAILABLE_KEY", Popularity: 40}, noop)
	// Provider with no declared auth strategy
	r.Register(ProviderInfo{Name: "no-key", EnvKey: "", Popularity: 30}, noop)
	// Local provider with no API key requirement
	r.Register(ProviderInfo{Name: "ollama", EnvKey: "", Popularity: 20}, noop)

	t.Setenv("TEST_AVAILABLE_KEY", "test-value")
	t.Setenv("TEST_UNAVAILABLE_KEY", "")

	avail := r.Available()
	// Should include "with-key" and local "ollama", but not unconfigured providers.
	if len(avail) != 2 {
		t.Fatalf("expected 2 available providers, got %d: %v", len(avail), avail)
	}
	names := make(map[string]bool)
	for _, p := range avail {
		names[p.Name] = true
	}
	if !names["with-key"] {
		t.Error("expected with-key to be available")
	}
	if !names["ollama"] {
		t.Error("expected ollama to be available")
	}
	if names["without-key"] {
		t.Error("expected without-key to NOT be available")
	}
	if names["no-key"] {
		t.Error("expected no-key to NOT be available without an auth strategy")
	}
}

func TestProviderRegistry_Available_HidesHiddenAliases(t *testing.T) {
	r := NewProviderRegistry()
	noop := func(cfg Config, modelOverride string) (Provider, error) {
		return NewAnthropic(Config{APIKey: "fake"}), nil
	}
	r.Register(ProviderInfo{Name: "visible", EnvKey: "TEST_VISIBLE_KEY", Popularity: 20}, noop)
	r.Register(ProviderInfo{Name: "hidden", EnvKey: "TEST_HIDDEN_KEY", Hidden: true, Popularity: 100}, noop)
	t.Setenv("TEST_VISIBLE_KEY", "visible-key")
	t.Setenv("TEST_HIDDEN_KEY", "hidden-key")

	available := r.Available()
	if len(available) != 1 || available[0].Name != "visible" {
		t.Fatalf("Available() = %+v, want only visible provider", available)
	}
}

func TestProviderRegistry_ConnectionState_BedrockNoCredentials(t *testing.T) {
	clearAWSEnv(t)
	r := NewProviderRegistry()
	r.catalog = DefaultCatalog()
	registerBedrock(r)

	detail := r.ConnectionState("bedrock")
	if detail.CanSelectModels {
		t.Fatalf("expected bedrock to require credentials, got %+v", detail)
	}
	if detail.State != ConnectionStateNotConfigured {
		t.Fatalf("expected not_configured, got %q", detail.State)
	}
}

func TestProviderRegistry_ConnectionState_OpenAIOAuthUsesChatGPTAuth(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	cs, err := NewCredentialStoreAt(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.Set(CredentialEntry{
		Provider:    "openai",
		AuthMethod:  "oauth",
		AccessToken: "chatgpt-access-token",
	}); err != nil {
		t.Fatal(err)
	}

	r := NewProviderRegistry()
	r.catalog = DefaultCatalog()
	r.SetCredentialStore(cs)
	registerOpenAI(r)

	detail := r.ConnectionState("openai")
	if !detail.CanSelectModels {
		t.Fatalf("expected OpenAI OAuth to be selectable, got %+v", detail)
	}
	if got := detail.DetailText(i18n.LangEN); got != "Connected (ChatGPT OAuth)" {
		t.Fatalf("detail = %q", got)
	}
	localized := detail.DetailText(i18n.LangZH)
	if !strings.Contains(localized, "ChatGPT OAuth") {
		t.Fatalf("localized detail = %q", localized)
	}
	if detail.DetailKey != i18n.KeyProviderConnectionChatGPTOAuth {
		t.Fatalf("detail key = %q", detail.DetailKey)
	}
}

func TestProviderRegistry_ConnectionState_BedrockStaticCredentials(t *testing.T) {
	clearAWSEnv(t)
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIA_TEST")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	r := NewProviderRegistry()
	r.catalog = DefaultCatalog()
	registerBedrock(r)

	detail := r.ConnectionState("bedrock")
	if !detail.CanSelectModels {
		t.Fatalf("expected bedrock to be selectable with static credentials, got %+v", detail)
	}
	if detail.Source != "AWS_ACCESS_KEY_ID" {
		t.Fatalf("expected AWS_ACCESS_KEY_ID source, got %q", detail.Source)
	}
}

func TestProviderRegistry_ConnectionState_VertexProjectOnlyNotConnected(t *testing.T) {
	clearGCPEnv(t)
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	r := NewProviderRegistry()
	r.catalog = DefaultCatalog()
	registerVertex(r)

	detail := r.ConnectionState("vertex")
	if detail.CanSelectModels {
		t.Fatalf("expected vertex project without ADC credentials to be unselectable, got %+v", detail)
	}
	if detail.State != ConnectionStateNotConfigured {
		t.Fatalf("expected not_configured, got %q", detail.State)
	}
}

func TestProviderRegistry_ConnectionState_VertexADCAndProject(t *testing.T) {
	clearGCPEnv(t)
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", filepath.Join(t.TempDir(), "service-account.json"))
	r := NewProviderRegistry()
	r.catalog = DefaultCatalog()
	registerVertex(r)

	detail := r.ConnectionState("vertex")
	if !detail.CanSelectModels {
		t.Fatalf("expected vertex to be selectable with ADC env and project, got %+v", detail)
	}
	if detail.Source != "GOOGLE_APPLICATION_CREDENTIALS" {
		t.Fatalf("expected GOOGLE_APPLICATION_CREDENTIALS source, got %q", detail.Source)
	}
}

func TestProviderRegistry_ConnectionState_OllamaLocalUnverified(t *testing.T) {
	r := NewProviderRegistry()
	r.catalog = DefaultCatalog()
	registerOllama(r)

	detail := r.ConnectionState("ollama")
	if !detail.CanSelectModels {
		t.Fatalf("expected ollama to be model-selectable, got %+v", detail)
	}
	if detail.State != ConnectionStateLocal {
		t.Fatalf("expected local_unverified, got %q", detail.State)
	}
}

func clearAWSEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_SESSION_TOKEN",
		"AWS_BEARER_TOKEN_BEDROCK",
		"AWS_PROFILE",
	} {
		t.Setenv(key, "")
	}
}

func clearGCPEnv(t *testing.T) {
	t.Helper()
	temp := t.TempDir()
	for _, key := range []string{
		"GOOGLE_APPLICATION_CREDENTIALS",
		"GOOGLE_CLOUD_PROJECT",
		"ANTHROPIC_VERTEX_PROJECT_ID",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("APPDATA", temp)
	t.Setenv("HOME", temp)
	t.Setenv("USERPROFILE", temp)
}

func TestDefaultRegistry_AllProviders(t *testing.T) {
	r := DefaultRegistry()
	all := r.All()

	if len(all) != 16 {
		t.Errorf("expected 16 providers in default registry, got %d", len(all))
	}

	// Check all expected providers are present
	expected := []string{
		"anthropic", "openai", "bedrock", "vertex", "ollama",
		"deepseek", "gemini", "groq", "xai", "mistral", "zhipu", "minimax", "kimi",
		"volcengine", "alibaba-cloud", "tencent-cloud",
	}
	for _, name := range expected {
		if _, ok := r.Get(name); !ok {
			t.Errorf("expected provider %q to be registered", name)
		}
	}
}

func TestDefaultRegistry_AnthropicFactory(t *testing.T) {
	// Set up env for Anthropic
	os.Setenv("ANTHROPIC_API_KEY", "test-key")
	defer os.Unsetenv("ANTHROPIC_API_KEY")

	r := DefaultRegistry()
	p, err := r.Create("anthropic", Config{}, "")
	if err != nil {
		t.Fatalf("unexpected error creating anthropic provider: %v", err)
	}
	if p.Name() != "anthropic" {
		t.Errorf("expected provider name 'anthropic', got %q", p.Name())
	}
}

func TestDefaultRegistry_AnthropicFactory_WithAuthToken(t *testing.T) {
	os.Unsetenv("ANTHROPIC_API_KEY")
	os.Setenv("ANTHROPIC_AUTH_TOKEN", "test-auth-token")
	defer os.Unsetenv("ANTHROPIC_AUTH_TOKEN")

	r := DefaultRegistry()
	p, err := r.Create("anthropic", Config{}, "")
	if err != nil {
		t.Fatalf("unexpected error creating anthropic provider with auth token: %v", err)
	}
	if p.Name() != "anthropic" {
		t.Errorf("expected provider name 'anthropic', got %q", p.Name())
	}
}

func TestDefaultRegistry_AnthropicFactory_MergesCustomHeaders(t *testing.T) {
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "test-auth-token")
	t.Setenv("ANTHROPIC_CUSTOM_HEADERS", "X-Proxy-Token: abc123\nX-Tenant: acme")

	var gotProxyToken string
	var gotTenant string
	var gotSource string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotProxyToken = r.Header.Get("X-Proxy-Token")
		gotTenant = r.Header.Get("X-Tenant")
		gotSource = r.Header.Get("X-Request-Source")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_123\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4-6\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n"))
		_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer srv.Close()

	r := DefaultRegistry()
	p, err := r.Create("anthropic", Config{
		BaseURL: srv.URL,
		Headers: map[string]string{
			"X-Request-Source": "unit-test",
		},
	}, "claude-sonnet-4-6")
	if err != nil {
		t.Fatalf("unexpected error creating anthropic provider with custom headers: %v", err)
	}

	ch, err := p.CreateStream(t.Context(), Params{
		Messages: []types.Message{types.UserMessage("hello")},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range ch {
	}

	if gotProxyToken != "abc123" {
		t.Fatalf("X-Proxy-Token = %q", gotProxyToken)
	}
	if gotTenant != "acme" {
		t.Fatalf("X-Tenant = %q", gotTenant)
	}
	if gotSource != "unit-test" {
		t.Fatalf("X-Request-Source = %q", gotSource)
	}
}

func TestDefaultRegistry_OpenAIFactory(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "test-key")
	defer os.Unsetenv("OPENAI_API_KEY")

	r := DefaultRegistry()
	p, err := r.Create("openai", Config{}, "gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error creating openai provider: %v", err)
	}
	if p.Name() != "openai" {
		t.Errorf("expected provider name 'openai', got %q", p.Name())
	}
	if p.ModelID() != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %q", p.ModelID())
	}
}

func TestDefaultRegistry_XAIFactoryUsesResponsesAPI(t *testing.T) {
	t.Setenv("XAI_MODEL", "")
	var path string
	var authorization string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		authorization = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_xai\",\"status\":\"in_progress\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"type\":\"message\",\"id\":\"msg_xai\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.content_part.added\",\"output_index\":0,\"content_index\":0,\"part\":{\"type\":\"output_text\",\"text\":\"\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"ok\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_item.done\",\"output_index\":0}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_xai\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1},\"output\":[{\"type\":\"message\"}]}}\n\n"))
	}))
	defer srv.Close()

	p, err := DefaultRegistry().Create("xai", Config{
		APIKey:  "xai-test-key",
		BaseURL: srv.URL,
	}, "")
	if err != nil {
		t.Fatalf("Create xai: %v", err)
	}
	if p.Name() != "xai" || p.ModelID() != "grok-4.5" {
		t.Fatalf("provider = %s/%s, want xai/grok-4.5", p.Name(), p.ModelID())
	}
	retry, ok := p.(*RetryProvider)
	if !ok {
		t.Fatalf("provider = %T, want *RetryProvider", p)
	}
	if responses, ok := retry.inner.(*ResponsesProvider); !ok || responses.APIFormat() != "responses" {
		t.Fatalf("xAI inner provider = %T, want ResponsesProvider", retry.inner)
	}
	ch, err := p.CreateStream(t.Context(), Params{
		Messages:        []types.Message{types.UserMessage("hello")},
		ReasoningEffort: "high",
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	var responseText strings.Builder
	for event := range ch {
		if event.Type == types.EventContentBlockDelta && event.Delta != nil {
			responseText.WriteString(event.Delta.Text)
		}
	}

	if path != "/v1/responses" {
		t.Fatalf("path = %q, want /v1/responses", path)
	}
	if authorization != "Bearer xai-test-key" {
		t.Fatalf("Authorization = %q, want Bearer xai-test-key", authorization)
	}
	reasoning, _ := body["reasoning"].(map[string]any)
	if body["model"] != "grok-4.5" || reasoning["effort"] != "high" {
		t.Fatalf("request body = %#v, want grok-4.5 with high reasoning effort", body)
	}
	if responseText.String() != "ok" {
		t.Fatalf("response text = %q, want ok", responseText.String())
	}
}

func TestDefaultRegistry_XAIMissingKeyIsUnconfigured(t *testing.T) {
	t.Setenv("XAI_API_KEY", "")
	t.Setenv("XAI_MODEL", "")
	p, err := DefaultRegistry().Create("xai", Config{}, "")
	if err != nil {
		t.Fatalf("Create xai: %v", err)
	}
	if _, ok := p.(*UnconfiguredProvider); !ok {
		t.Fatalf("provider = %T, want UnconfiguredProvider", p)
	}
	if p.ModelID() != "grok-4.5" {
		t.Fatalf("model = %q, want grok-4.5", p.ModelID())
	}
}

func TestDefaultRegistry_XAIFactoryConfigPrecedence(t *testing.T) {
	envServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("environment base URL was used: %s", r.URL)
	}))
	defer envServer.Close()
	configServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %q, want /v1/responses", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer config-key" {
			t.Fatalf("Authorization = %q, want Bearer config-key", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_precedence\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_precedence\",\"status\":\"completed\",\"usage\":{},\"output\":[]}}\n\n"))
	}))
	defer configServer.Close()

	t.Setenv("XAI_API_KEY", "environment-key")
	t.Setenv("XAI_MODEL", "environment-model")
	t.Setenv("XAI_BASE_URL", envServer.URL)
	p, err := DefaultRegistry().Create("xai", Config{
		APIKey:  "config-key",
		BaseURL: configServer.URL,
		Model:   "config-model",
	}, "override-model")
	if err != nil {
		t.Fatalf("Create xai: %v", err)
	}
	if p.ModelID() != "override-model" {
		t.Fatalf("model = %q, want override-model", p.ModelID())
	}
	ch, err := p.CreateStream(t.Context(), Params{Messages: []types.Message{types.UserMessage("hello")}})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range ch {
	}
}

func TestDefaultRegistry_OllamaFactory_NoKeyRequired(t *testing.T) {
	// Ollama shouldn't require any API key
	r := DefaultRegistry()
	p, err := r.Create("ollama", Config{}, "llama3.1")
	if err != nil {
		t.Fatalf("unexpected error creating ollama provider: %v", err)
	}
	if p.ModelID() != "llama3.1" {
		t.Errorf("expected model 'llama3.1', got %q", p.ModelID())
	}
}

func TestVisibleProvidersDeclareSupportedAuthentication(t *testing.T) {
	r := DefaultRegistry()
	for _, info := range r.Visible() {
		if len(info.AuthMethods) == 0 {
			t.Errorf("provider %q does not declare an authentication method", info.Name)
		}
	}
}

func TestCustomEndpointFactoriesPreserveSpecialProviderIdentity(t *testing.T) {
	r := DefaultRegistry()
	expected := map[string]string{
		"alibaba-cloud": "compatible-chat",
		"anthropic":     "anthropic",
		"bedrock":       "bedrock",
		"deepseek":      "deepseek-chat",
		"gemini":        "gemini-chat",
		"groq":          "groq-chat",
		"kimi":          "standard-chat",
		"minimax":       "standard-chat",
		"mistral":       "mistral-chat",
		"ollama":        "ollama-chat",
		"openai":        "openai-chat",
		"tencent-cloud": "compatible-chat",
		"volcengine":    "compatible-chat",
		"xai":           "compatible-responses",
		"zhipu":         "standard-chat",
	}
	for _, info := range r.Visible() {
		if !providerSupportsMethod(info.AuthMethods, "api_key") {
			continue
		}
		t.Run(info.Name, func(t *testing.T) {
			wantContract, reviewed := expected[info.Name]
			if !reviewed {
				t.Fatalf("provider %q has no custom-endpoint contract review", info.Name)
			}
			p, err := r.Create(info.Name, Config{
				APIKey:  "test-key",
				BaseURL: "http://localhost:11434/v1",
			}, "test-model")
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if p.Name() != info.Name {
				t.Fatalf("Name = %q, want %q", p.Name(), info.Name)
			}
			retry, ok := p.(*RetryProvider)
			if !ok {
				t.Fatalf("provider = %T, want RetryProvider", p)
			}
			switch inner := retry.inner.(type) {
			case *AnthropicProvider:
				if wantContract != "anthropic" || inner.name != info.Name {
					t.Fatalf("contract = Anthropic/%q, want %s", inner.name, wantContract)
				}
			case *BedrockProvider:
				if wantContract != "bedrock" {
					t.Fatalf("contract = Bedrock, want %s", wantContract)
				}
			case *ResponsesProvider:
				if wantContract != "compatible-responses" || inner.semantics != ResponsesSemanticsCompatible {
					t.Fatalf("contract = Responses/%s, want %s", inner.semantics, wantContract)
				}
			case *OpenAIProvider:
				wantDialect := map[string]OpenAIDialect{
					"compatible-chat": DialectStandard,
					"deepseek-chat":   DialectDeepSeek,
					"gemini-chat":     DialectGemini,
					"groq-chat":       DialectGroq,
					"mistral-chat":    DialectMistral,
					"ollama-chat":     DialectOllama,
					"openai-chat":     DialectStandard,
					"standard-chat":   DialectStandard,
				}[wantContract]
				if inner.dialect != wantDialect {
					t.Fatalf("dialect = %q, want %q for %s", inner.dialect, wantDialect, wantContract)
				}
				if inner.nativeOpenAIChatContract != (wantContract == "openai-chat") {
					t.Fatalf("native OpenAI contract = %v, want %v", inner.nativeOpenAIChatContract, wantContract == "openai-chat")
				}
			default:
				t.Fatalf("provider contract = %T, want %s", retry.inner, wantContract)
			}
		})
	}
}

func providerSupportsMethod(methods []string, want string) bool {
	for _, method := range methods {
		if method == want {
			return true
		}
	}
	return false
}

func TestDefaultRegistry_MissingAPIKey(t *testing.T) {
	// Clear any existing keys
	os.Unsetenv("OPENAI_API_KEY")

	r := DefaultRegistry()
	p, err := r.Create("openai", Config{}, "")
	if err != nil {
		t.Fatalf("unexpected startup error when OPENAI_API_KEY is missing: %v", err)
	}
	if _, ok := p.(*UnconfiguredProvider); !ok {
		t.Fatalf("expected unconfigured provider when OPENAI_API_KEY is missing, got %T", p)
	}
}

func TestDefaultRegistry_Catalog(t *testing.T) {
	r := DefaultRegistry()
	c := r.Catalog()
	if c == nil {
		t.Fatal("expected non-nil catalog")
	}
	if c.Count() < 30 {
		t.Errorf("expected at least 30 models in catalog, got %d", c.Count())
	}
}

func TestDefaultRegistry_PopularityOrder(t *testing.T) {
	r := DefaultRegistry()
	all := r.All()
	if len(all) == 0 {
		t.Fatal("expected non-empty provider list")
	}
	// Anthropic should be first (highest popularity)
	if all[0].Name != "anthropic" {
		t.Errorf("expected anthropic to be first (highest popularity), got %q", all[0].Name)
	}
}

func TestDefaultRegistry_ProviderInfoFields(t *testing.T) {
	r := DefaultRegistry()

	tests := []struct {
		name        string
		displayName string
		envKey      string
	}{
		{"anthropic", "Anthropic", "ANTHROPIC_API_KEY"},
		{"openai", "OpenAI", "OPENAI_API_KEY"},
		{"gemini", "Google Gemini", "GEMINI_API_KEY"},
		{"deepseek", "DeepSeek", "DEEPSEEK_API_KEY"},
		{"groq", "Groq", "GROQ_API_KEY"},
		{"xai", "xAI", "XAI_API_KEY"},
		{"mistral", "Mistral AI", "MISTRAL_API_KEY"},
		{"zhipu", "Zhipu AI", "ZHIPU_API_KEY"},
		{"minimax", "MiniMax", "MINIMAX_API_KEY"},
		{"kimi", "Kimi (Moonshot AI)", "MOONSHOT_API_KEY"},
		{"ollama", "Ollama (Local)", ""},
	}

	for _, tt := range tests {
		info, ok := r.Get(tt.name)
		if !ok {
			t.Errorf("provider %q not found", tt.name)
			continue
		}
		if info.DisplayName != tt.displayName {
			t.Errorf("provider %q: expected display name %q, got %q", tt.name, tt.displayName, info.DisplayName)
		}
		if info.EnvKey != tt.envKey {
			t.Errorf("provider %q: expected env key %q, got %q", tt.name, tt.envKey, info.EnvKey)
		}
		if info.DefaultModel == "" {
			t.Errorf("provider %q: expected non-empty default model", tt.name)
		}
		if len(info.Models) == 0 {
			t.Errorf("provider %q: expected non-empty models list", tt.name)
		}
	}
}
