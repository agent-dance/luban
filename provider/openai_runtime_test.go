package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestShouldUseOpenAIResponsesAPI(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{model: "gpt-4o", want: false},
		{model: "gpt-5", want: true},
		{model: "gpt-5.5", want: true},
		{model: "gpt-5.4", want: true},
		{model: "gpt-5.3-codex", want: true},
		{model: "gpt-5.1-codex", want: true},
		{model: "codex-mini-latest", want: true},
	}
	for _, tt := range tests {
		if got := shouldUseOpenAIResponsesAPI(tt.model); got != tt.want {
			t.Fatalf("shouldUseOpenAIResponsesAPI(%q) = %v, want %v", tt.model, got, tt.want)
		}
	}
}

func TestResolveOpenAIAPIFormat(t *testing.T) {
	tests := []struct {
		name          string
		authToken     string
		requested     string
		baseURL       string
		model         string
		wantResponses bool
	}{
		{name: "official GPT 5 defaults to responses", baseURL: "https://api.openai.com/v1", model: "gpt-5.5", wantResponses: true},
		{name: "official endpoint honors catalog responses format", baseURL: "https://api.openai.com/v1", model: "gpt-5.4-mini", wantResponses: true},
		{name: "custom endpoint keeps chat default for cataloged model", baseURL: "https://gateway.example.com", model: "gpt-5.4-mini"},
		{name: "custom endpoint keeps chat default for unknown model", baseURL: "https://gateway.example.com", model: "vendor-unknown"},
		{name: "custom endpoint can force responses", requested: "responses", baseURL: "https://gateway.example.com", model: "gpt-5.5", wantResponses: true},
		{name: "explicit chat overrides catalog format", requested: "chat-completions", baseURL: "https://gateway.example.com", model: "gpt-5.4-mini"},
		{name: "oauth always uses responses", authToken: "oauth-token", requested: "chat-completions", baseURL: openAIChatGPTCodexBaseURL, model: "gpt-5.5", wantResponses: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveOpenAIResponsesMode(tt.authToken, tt.requested, tt.baseURL, tt.model)
			if got != tt.wantResponses {
				t.Fatalf("resolveOpenAIResponsesMode() = %v, want %v", got, tt.wantResponses)
			}
		})
	}
}

func TestOpenAIModelRoutingNegotiatesCompatibleGatewayProtocol(t *testing.T) {
	r := NewProviderRegistry()
	registerOpenAI(r)
	registerDeepSeek(r)

	openAIProvider, err := r.Create("openai", Config{
		APIKey:  "test-openai-key",
		BaseURL: "https://gateway.example.com/v1",
		Model:   "gpt-5.4-mini",
	}, "")
	if err != nil {
		t.Fatalf("create OpenAI provider: %v", err)
	}
	openAIRetry, ok := openAIProvider.(*RetryProvider)
	if !ok {
		t.Fatalf("OpenAI provider = %T, want *RetryProvider", openAIProvider)
	}
	if _, ok := openAIRetry.inner.(*openAIProtocolProvider); !ok {
		t.Fatalf("OpenAI inner provider = %T, want *openAIProtocolProvider", openAIRetry.inner)
	}

	compatibleProvider, err := r.Create("deepseek", Config{
		APIKey:  "test-deepseek-key",
		BaseURL: "https://gateway.example.com/v1",
		Model:   "deepseek-v4-flash",
	}, "")
	if err != nil {
		t.Fatalf("create compatible provider: %v", err)
	}
	compatibleRetry, ok := compatibleProvider.(*RetryProvider)
	if !ok {
		t.Fatalf("compatible provider = %T, want *RetryProvider", compatibleProvider)
	}
	if _, ok := compatibleRetry.inner.(*OpenAIProvider); !ok {
		t.Fatalf("compatible inner provider = %T, want *OpenAIProvider", compatibleRetry.inner)
	}
}

func TestOpenAIProtocolProviderPrefersResponsesForCatalogedModel(t *testing.T) {
	var paths []string
	var requestBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path != "/responses" {
			t.Fatalf("request path = %q, want /responses", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: response.completed\ndata: {\"response\":{\"id\":\"resp_cache\",\"status\":\"completed\",\"usage\":{\"input_tokens\":2006,\"output_tokens\":12,\"input_tokens_details\":{\"cached_tokens\":1920}},\"output\":[]}}\n\n")
	}))
	defer srv.Close()

	r := NewProviderRegistry()
	registerOpenAI(r)
	p, err := r.Create("openai", Config{APIKey: "test-key", BaseURL: srv.URL, Model: "gpt-5.4-mini"}, "")
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	ch, err := p.CreateStream(context.Background(), Params{
		Messages:       []types.Message{types.UserMessage("hello")},
		PromptCacheKey: "cache-lineage",
		UsePromptCache: true,
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	var usage *types.Usage
	for event := range ch {
		if event.Usage != nil {
			usage = event.Usage
		}
	}
	if len(paths) != 1 || paths[0] != "/responses" {
		t.Fatalf("request paths = %v, want [/responses]", paths)
	}
	if cacheKey, _ := requestBody["prompt_cache_key"].(string); cacheKey == "" {
		t.Fatalf("prompt_cache_key = %#v, want stable cache routing key", requestBody["prompt_cache_key"])
	}
	if usage == nil || usage.InputTokens != 2006 || usage.CacheReadInputTokens != 1920 {
		t.Fatalf("usage = %+v, want Responses cache usage 2006/1920", usage)
	}
}

func TestOpenAIProtocolProviderFallsBackOnceWhenResponsesUnavailable(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/responses":
			http.NotFound(w, r)
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl_fallback\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-5.4-mini\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		default:
			http.Error(w, "unexpected path", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	r := NewProviderRegistry()
	registerOpenAI(r)
	p, err := r.Create("openai", Config{APIKey: "test-key", BaseURL: srv.URL, Model: "gpt-5.4-mini"}, "")
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		ch, streamErr := p.CreateStream(context.Background(), Params{Messages: []types.Message{types.UserMessage("hello")}})
		if streamErr != nil {
			t.Fatalf("CreateStream attempt %d: %v", attempt+1, streamErr)
		}
		for range ch {
		}
	}
	want := []string{"/responses", "/v1/chat/completions", "/v1/chat/completions"}
	if len(paths) != len(want) {
		t.Fatalf("request paths = %v, want %v", paths, want)
	}
	for index := range want {
		if paths[index] != want[index] {
			t.Fatalf("request paths = %v, want %v", paths, want)
		}
	}
}

func TestOpenAICompatibleChatUsesMaxTokensAndOverride(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Errorf("unmarshal request: %v", err)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl_compat\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-5.4-mini\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
	}))
	defer srv.Close()

	p := NewOpenAI(Config{
		ProviderName: "openai",
		APIKey:       "test-key",
		BaseURL:      srv.URL,
		Model:        "gpt-5.4-mini",
	})
	ch, err := p.CreateStream(context.Background(), Params{
		Messages:                []types.Message{types.UserMessage("hello")},
		MaxTokens:               1024,
		MaxOutputTokensOverride: 64000,
		ReasoningEffort:         "ultra",
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range ch {
	}
	if got := captured["max_tokens"]; got != float64(64000) {
		t.Fatalf("max_tokens = %#v, want 64000", got)
	}
	if _, ok := captured["max_completion_tokens"]; ok {
		t.Fatalf("custom endpoint retained max_completion_tokens: %#v", captured["max_completion_tokens"])
	}
	if got := captured["reasoning_effort"]; got != "max" {
		t.Fatalf("reasoning_effort = %#v, want max", got)
	}
}

func TestOpenAIChatAssignsCanonicalContentBlockIndexes(t *testing.T) {
	tests := []struct {
		name        string
		delta       string
		wantIndexes []int
	}{
		{
			name:        "tool only starts at zero",
			delta:       `{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"Read","arguments":"{}"}}]}`,
			wantIndexes: []int{0},
		},
		{
			name:        "text then tool is sequential",
			delta:       `{"role":"assistant","content":"hello","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"Read","arguments":"{}"}}]}`,
			wantIndexes: []int{0, 1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl_blocks\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":"+test.delta+",\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer srv.Close()

			stream, err := NewOpenAI(Config{ProviderName: "custom", APIKey: "test-key", BaseURL: srv.URL, Model: "test-model"}).CreateStream(context.Background(), Params{
				Messages: []types.Message{types.UserMessage("hello")},
			})
			if err != nil {
				t.Fatalf("CreateStream: %v", err)
			}
			var got []int
			for event := range stream {
				if event.Type == types.EventContentBlockStart {
					got = append(got, event.Index)
				}
			}
			if len(got) != len(test.wantIndexes) {
				t.Fatalf("content block indexes = %v, want %v", got, test.wantIndexes)
			}
			for index := range got {
				if got[index] != test.wantIndexes[index] {
					t.Fatalf("content block indexes = %v, want %v", got, test.wantIndexes)
				}
			}
		})
	}
}

func TestNormalizeOpenAIChatBaseURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "", want: ""},
		{input: "https://gateway.example.com", want: "https://gateway.example.com/v1"},
		{input: "https://gateway.example.com/", want: "https://gateway.example.com/v1"},
		{input: "https://gateway.example.com/v1", want: "https://gateway.example.com/v1"},
		{input: "https://gateway.example.com/openai/v1/", want: "https://gateway.example.com/openai/v1"},
	}

	for _, tt := range tests {
		if got := normalizeOpenAIChatBaseURL(tt.input); got != tt.want {
			t.Errorf("normalizeOpenAIChatBaseURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsOpenAIChatGPTCodexBaseURL(t *testing.T) {
	tests := []struct {
		baseURL string
		want    bool
	}{
		{baseURL: "https://chatgpt.com/backend-api/codex", want: true},
		{baseURL: "https://chatgpt.com/backend-api/codex/", want: true},
		{baseURL: " https://chatgpt.com/backend-api/codex/ ", want: true},
		{baseURL: "https://api.openai.com/v1", want: false},
		{baseURL: "https://chatgpt.com/backend-api", want: false},
	}
	for _, tt := range tests {
		if got := isOpenAIChatGPTCodexBaseURL(tt.baseURL); got != tt.want {
			t.Fatalf("isOpenAIChatGPTCodexBaseURL(%q) = %v, want %v", tt.baseURL, got, tt.want)
		}
	}
}

func TestResolveCredentialConfigAddsCodexHeadersForOpenAIOAuth(t *testing.T) {
	cs, err := NewCredentialStoreAt(t.TempDir() + "/auth.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.Set(CredentialEntry{
		Provider:                "openai",
		AuthMethod:              "oauth",
		AccessToken:             "access-token",
		AccountID:               "acct_123",
		ChatGPTAccountIsFedRAMP: true,
	}); err != nil {
		t.Fatal(err)
	}

	reg := NewProviderRegistry()
	reg.SetCredentialStore(cs)

	cfg, err := ResolveCredentialConfig(reg, "openai")
	if err != nil {
		t.Fatalf("ResolveCredentialConfig: %v", err)
	}
	if cfg.AuthToken != "access-token" {
		t.Fatalf("cfg.AuthToken = %q", cfg.AuthToken)
	}
	if cfg.APIKey != "" {
		t.Fatalf("cfg.APIKey = %q, want empty", cfg.APIKey)
	}
	if cfg.BaseURL != openAIChatGPTCodexBaseURL {
		t.Fatalf("cfg.BaseURL = %q", cfg.BaseURL)
	}
	if got := cfg.Headers["originator"]; got != openAICodexOriginator {
		t.Fatalf("originator header = %q", got)
	}
	if got := cfg.Headers["User-Agent"]; got != openAICodexOriginator {
		t.Fatalf("User-Agent = %q", got)
	}
	if got := cfg.Headers["ChatGPT-Account-ID"]; got != "acct_123" {
		t.Fatalf("ChatGPT-Account-ID = %q", got)
	}
	if got := cfg.Headers["X-OpenAI-Fedramp"]; got != "true" {
		t.Fatalf("X-OpenAI-Fedramp = %q", got)
	}
}

func TestResponsesProviderUsesAuthTokenBearer(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}]}}\n\n"))
	}))
	defer srv.Close()

	p := NewResponses(Config{
		AuthToken: "chatgpt-access-token",
		BaseURL:   srv.URL,
	})
	ch, err := p.CreateStream(context.Background(), Params{
		Messages: []types.Message{types.UserMessage("hello")},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range ch {
	}

	if gotAuth != "Bearer chatgpt-access-token" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
}
