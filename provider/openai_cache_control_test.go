package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestOpenAICompatibleCacheControlMatrix(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		wantField    string
	}{
		{name: "OpenAI Chat", providerName: "openai", wantField: "prompt_cache_key"},
		{name: "custom OpenAI-compatible", providerName: "custom", wantField: "prompt_cache_key"},
		{name: "Gemini", providerName: "gemini", wantField: "prompt_cache_key"},
		{name: "Groq", providerName: "groq", wantField: "prompt_cache_key"},
		{name: "Mistral", providerName: "mistral", wantField: "prompt_cache_key"},
		{name: "Zhipu", providerName: "zhipu", wantField: "prompt_cache_key"},
		{name: "MiniMax", providerName: "minimax", wantField: "prompt_cache_key"},
		{name: "Kimi", providerName: "kimi", wantField: "prompt_cache_key"},
		{name: "Ollama", providerName: "ollama", wantField: "prompt_cache_key"},
		{name: "DeepSeek", providerName: "deepseek", wantField: "user_id"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := Config{
				ProviderName: test.providerName,
				Model:        "cache-test-model",
			}
			params := Params{
				Messages:       []types.Message{types.UserMessage("hello")},
				PromptCacheKey: "shared-cache-lineage",
				UsePromptCache: true,
			}
			request := captureOpenAICompatibleCacheRequest(t, config, params)
			want := expectedOpenAICompatibleCacheRoutingKey(config, params)
			if got := request[test.wantField]; got != want {
				t.Fatalf("%s = %#v, want credential-scoped route %q", test.wantField, got, want)
			}
			otherField := "user_id"
			if test.wantField == otherField {
				otherField = "prompt_cache_key"
			}
			if got, found := request[otherField]; found {
				t.Fatalf("unexpected %s = %#v alongside %s", otherField, got, test.wantField)
			}
		})
	}
}

func TestOpenAICompatibleCacheControlRequiresEnabledNonEmptyLineage(t *testing.T) {
	for _, test := range []struct {
		name   string
		config Config
		params Params
	}{
		{
			name:   "disabled OpenAI-compatible",
			config: Config{ProviderName: "custom", Model: "cache-test-model"},
			params: Params{PromptCacheKey: "lineage", UsePromptCache: false},
		},
		{
			name:   "empty OpenAI-compatible lineage",
			config: Config{ProviderName: "custom", Model: "cache-test-model"},
			params: Params{UsePromptCache: true},
		},
		{
			name:   "disabled DeepSeek",
			config: Config{ProviderName: "deepseek", Model: "cache-test-model"},
			params: Params{PromptCacheKey: "lineage", UsePromptCache: false},
		},
		{
			name:   "empty DeepSeek lineage",
			config: Config{ProviderName: "deepseek", Model: "cache-test-model"},
			params: Params{UsePromptCache: true},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.params.Messages = []types.Message{types.UserMessage("hello")}
			request := captureOpenAICompatibleCacheRequest(t, test.config, test.params)
			for _, field := range []string{"prompt_cache_key", "user_id"} {
				if got, found := request[field]; found {
					t.Fatalf("%s = %#v, want omitted", field, got)
				}
			}
		})
	}
}

func TestCacheRoutingIsolationAcrossIndependentSessions(t *testing.T) {
	for _, test := range []struct {
		name         string
		providerName string
		wantField    string
		wantShared   bool
	}{
		{name: "OpenAI", providerName: "openai", wantField: "prompt_cache_key"},
		{name: "DeepSeek", providerName: "deepseek", wantField: "user_id", wantShared: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := Config{
				ProviderName: test.providerName,
				APIKey:       "same-account-secret",
				Model:        "cache-test-model",
			}
			first := captureOpenAICompatibleCacheRequest(t, config, Params{
				System:         "stable shared prefix",
				Messages:       []types.Message{types.UserMessage("first session")},
				PromptCacheKey: "session-a",
				UsePromptCache: true,
			})
			second := captureOpenAICompatibleCacheRequest(t, config, Params{
				System:         "stable shared prefix",
				Messages:       []types.Message{types.UserMessage("second session")},
				PromptCacheKey: "session-b",
				UsePromptCache: true,
			})

			firstKey, _ := first[test.wantField].(string)
			secondKey, _ := second[test.wantField].(string)
			if firstKey == "" || (firstKey == secondKey) != test.wantShared {
				t.Fatalf("independent sessions used unexpected %s sharing: %q, %q; want shared=%v", test.wantField, firstKey, secondKey, test.wantShared)
			}
			if firstKey == "session-a" || firstKey == "session-b" || firstKey == config.APIKey {
				t.Fatalf("%s leaked conversation or credential identity: %q", test.wantField, firstKey)
			}
		})
	}
}

func TestUserScopedCacheRoutingSeparatesProviderCredentials(t *testing.T) {
	params := Params{
		Messages:       []types.Message{types.UserMessage("same prompt")},
		PromptCacheKey: "same-session",
		UsePromptCache: true,
	}
	first := captureOpenAICompatibleCacheRequest(t, Config{
		ProviderName: "deepseek",
		APIKey:       "first-account-secret",
		Model:        "cache-test-model",
	}, params)
	second := captureOpenAICompatibleCacheRequest(t, Config{
		ProviderName: "deepseek",
		APIKey:       "second-account-secret",
		Model:        "cache-test-model",
	}, params)
	if first["user_id"] == second["user_id"] {
		t.Fatalf("different credentials shared DeepSeek user_id: %#v", first["user_id"])
	}
}

func TestDefaultOpenAIEndpointTreatsPromptCacheKeyAsDocumented(t *testing.T) {
	client := NewOpenAI(Config{APIKey: "test-key"})
	if got := client.Capabilities().CacheRouting; got != CacheRoutingPromptCacheKey {
		t.Fatalf("default api.openai.com cache routing = %q, want documented prompt_cache_key", got)
	}
}

func TestOpenAICompatibleCacheLineageSharesWarmRouteAcrossConcurrentStreams(t *testing.T) {
	var (
		mu       sync.Mutex
		captured = make(map[string]int)
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		key, _ := payload["prompt_cache_key"].(string)
		mu.Lock()
		captured[key]++
		mu.Unlock()
		writeOpenAICompatibleCacheResponse(w)
	}))
	defer server.Close()

	client := NewOpenAI(Config{ProviderName: "custom", APIKey: "test-key", BaseURL: server.URL, Model: "cache-test-model"})
	var wg sync.WaitGroup
	for _, key := range []string{"lineage-a", "lineage-b"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stream, err := client.CreateStream(context.Background(), Params{
				Messages:       []types.Message{types.UserMessage("hello")},
				PromptCacheKey: key,
				UsePromptCache: true,
			})
			if err != nil {
				t.Errorf("CreateStream(%s): %v", key, err)
				return
			}
			for range stream {
			}
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(captured) != 1 {
		t.Fatalf("captured cache routes = %#v, want one credential-scoped route", captured)
	}
	for key, count := range captured {
		if key == "" || key == "lineage-a" || key == "lineage-b" || count != 2 {
			t.Fatalf("captured cache route = %q count=%d", key, count)
		}
	}
}

func TestOpenAICompatibleFallbackRetriesOnceWithoutRejectedPromptCacheKey(t *testing.T) {
	var (
		mu       sync.Mutex
		requests []map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		mu.Lock()
		requests = append(requests, payload)
		attempt := len(requests)
		mu.Unlock()
		if attempt == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"type":"invalid_request_error","param":"prompt_cache_key","message":"unsupported parameter: prompt_cache_key"}}`)
			return
		}
		writeOpenAICompatibleCacheResponse(w)
	}))
	defer server.Close()

	config := Config{ProviderName: "groq", APIKey: "test-key", BaseURL: server.URL, Model: "cache-test-model"}
	params := Params{
		Messages:       []types.Message{types.UserMessage("hello")},
		PromptCacheKey: "lineage-fallback",
		UsePromptCache: true,
	}
	client := NewOpenAI(config)
	stream, err := client.CreateStream(context.Background(), params)
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range stream {
	}
	stream, err = client.CreateStream(context.Background(), Params{
		Messages:       []types.Message{types.UserMessage("hello again")},
		PromptCacheKey: "lineage-fallback",
		UsePromptCache: true,
	})
	if err != nil {
		t.Fatalf("second CreateStream: %v", err)
	}
	for range stream {
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 3 {
		t.Fatalf("request attempts = %d, want one guarded fallback followed by one remembered request", len(requests))
	}
	wantCacheKey := expectedOpenAICompatibleCacheRoutingKey(config, params)
	if got := requests[0]["prompt_cache_key"]; got != wantCacheKey {
		t.Fatalf("first request prompt_cache_key = %#v, want %q", got, wantCacheKey)
	}
	if got, found := requests[1]["prompt_cache_key"]; found {
		t.Fatalf("fallback request retained rejected prompt_cache_key = %#v", got)
	}
	if got, found := requests[2]["prompt_cache_key"]; found {
		t.Fatalf("remembered rejection request retained prompt_cache_key = %#v", got)
	}
}

func TestDocumentedPromptCacheKeyProviderDoesNotSilentlyDropRejectedKey(t *testing.T) {
	attempts := 0
	config := Config{ProviderName: "mistral", APIKey: "test-key", Model: "cache-test-model"}
	params := Params{
		Messages:       []types.Message{types.UserMessage("hello")},
		PromptCacheKey: "lineage-required",
		UsePromptCache: true,
	}
	wantCacheKey := expectedOpenAICompatibleCacheRoutingKey(config, params)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		attempts++
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if got := payload["prompt_cache_key"]; got != wantCacheKey {
			t.Errorf("OpenAI prompt_cache_key = %#v", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"type":"invalid_request_error","param":"prompt_cache_key","message":"unsupported parameter: prompt_cache_key"}}`)
	}))
	defer server.Close()

	config.BaseURL = server.URL
	client := NewOpenAI(config)
	_, err := client.CreateStream(context.Background(), params)
	if err == nil {
		t.Fatal("CreateStream unexpectedly accepted documented prompt_cache_key rejection")
	}
	if attempts != 1 {
		t.Fatalf("OpenAI request attempts = %d, want no silent field removal", attempts)
	}
}

func TestOpenAIUsageAcceptsKimiTopLevelCachedTokens(t *testing.T) {
	_, usage, err := decodeOpenAIStreamChunk([]byte(`{
		"id":"chatcmpl_kimi_cache",
		"object":"chat.completion.chunk",
		"choices":[],
		"usage":{"prompt_tokens":2006,"completion_tokens":11,"cached_tokens":1920}
	}`), DialectStandard)
	if err != nil {
		t.Fatal(err)
	}
	if usage == nil || usage.InputTokens != 2006 || usage.OutputTokens != 11 || usage.CacheReadInputTokens != 1920 {
		t.Fatalf("Kimi cache usage = %#v", usage)
	}
}

func captureOpenAICompatibleCacheRequest(t *testing.T, config Config, params Params) map[string]any {
	t.Helper()
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		writeOpenAICompatibleCacheResponse(w)
	}))
	defer server.Close()

	if config.APIKey == "" {
		config.APIKey = "test-key"
	}
	config.BaseURL = server.URL
	stream, err := NewOpenAI(config).CreateStream(context.Background(), params)
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range stream {
	}
	if captured == nil {
		t.Fatal("request was not captured")
	}
	return captured
}

func expectedOpenAICompatibleCacheRoutingKey(config Config, params Params) string {
	if config.APIKey == "" {
		config.APIKey = "test-key"
	}
	userNamespace := promptCacheUserNamespace(config)
	if resolveOpenAIChatCacheRouting(config, detectDialect(config), config.BaseURL) == CacheRoutingDeepSeekUserID {
		return deepSeekCacheUserID(userNamespace)
	}
	model := params.Model
	if model == "" {
		model = config.Model
	}
	return scopedPromptCacheKey(
		userNamespace,
		params.PromptCacheKey,
		model,
		promptCacheRoutingShardCount(config.ProviderName),
	)
}

func writeOpenAICompatibleCacheResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl_cache\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"cache-test-model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":\"stop\"}]}\n\n")
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}
