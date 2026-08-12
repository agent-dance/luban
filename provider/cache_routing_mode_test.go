package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

func TestOpenAIChatCacheRoutingModeMatrix(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want CacheRoutingMode
	}{
		{name: "official OpenAI", cfg: Config{ProviderName: "openai"}, want: CacheRoutingPromptCacheKey},
		{name: "default OpenAI", cfg: Config{}, want: CacheRoutingPromptCacheKey},
		{name: "Mistral", cfg: Config{ProviderName: "mistral"}, want: CacheRoutingPromptCacheKey},
		{name: "Kimi", cfg: Config{ProviderName: "kimi"}, want: CacheRoutingPromptCacheKey},
		{name: "DeepSeek", cfg: Config{ProviderName: "deepseek"}, want: CacheRoutingDeepSeekUserID},
		{name: "compatible identity ignores DeepSeek hostname", cfg: Config{ProviderName: "custom", BaseURL: "https://api.deepseek.com/v1"}, want: CacheRoutingPromptCacheKeyBestEffort},
		{name: "Gemini", cfg: Config{ProviderName: "gemini"}, want: CacheRoutingPromptCacheKeyBestEffort},
		{name: "Groq", cfg: Config{ProviderName: "groq"}, want: CacheRoutingPromptCacheKeyBestEffort},
		{name: "custom gateway", cfg: Config{ProviderName: "custom", BaseURL: "https://gateway.example/v1"}, want: CacheRoutingPromptCacheKeyBestEffort},
		{name: "forced off", cfg: Config{ProviderName: "openai", CacheRoutingPreference: CacheRoutingOff}, want: CacheRoutingNone},
		{name: "forced on upgrades best effort", cfg: Config{ProviderName: "groq", CacheRoutingPreference: CacheRoutingOn}, want: CacheRoutingPromptCacheKey},
		{name: "forced on keeps DeepSeek native", cfg: Config{ProviderName: "deepseek", CacheRoutingPreference: CacheRoutingOn}, want: CacheRoutingDeepSeekUserID},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := NewOpenAI(test.cfg).Capabilities().CacheRouting; got != test.want {
				t.Fatalf("cache routing = %q, want %q", got, test.want)
			}
		})
	}
}

func TestProviderCacheCapabilitiesKeepRoutingSeparateFromNativeBreakpoints(t *testing.T) {
	for _, test := range []struct {
		name string
		got  ProviderCapabilities
	}{
		{name: "Anthropic", got: NewAnthropic(Config{}).Capabilities()},
		{name: "Bedrock", got: (&BedrockProvider{}).Capabilities()},
		{name: "Vertex", got: (&VertexProvider{}).Capabilities()},
	} {
		if !test.got.CacheControl || test.got.CacheRouting != CacheRoutingNone {
			t.Fatalf("%s capabilities = %#v, want native cache_control without routing key", test.name, test.got)
		}
	}
	if got := NewResponses(Config{}).Capabilities(); got.CacheControl || got.CacheRouting != CacheRoutingPromptCacheKey {
		t.Fatalf("Responses capabilities = %#v, want prompt_cache_key without native cache_control", got)
	}
}

func TestCacheRoutingPreferenceControlsOpenAICompatibleWireFields(t *testing.T) {
	for _, test := range []struct {
		name       string
		provider   string
		preference CacheRoutingPreference
		wantField  string
	}{
		{name: "auto best effort", provider: "groq", preference: CacheRoutingAuto, wantField: "prompt_cache_key"},
		{name: "on best effort", provider: "groq", preference: CacheRoutingOn, wantField: "prompt_cache_key"},
		{name: "off documented", provider: "openai", preference: CacheRoutingOff},
		{name: "off DeepSeek", provider: "deepseek", preference: CacheRoutingOff},
		{name: "on DeepSeek", provider: "deepseek", preference: CacheRoutingOn, wantField: "user_id"},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := Config{
				ProviderName:           test.provider,
				Model:                  "cache-test-model",
				CacheRoutingPreference: test.preference,
			}
			params := Params{
				Messages:       []types.Message{types.UserMessage("hello")},
				PromptCacheKey: "cache-lineage",
				UsePromptCache: true,
			}
			request := captureOpenAICompatibleCacheRequest(t, config, params)
			for _, field := range []string{"prompt_cache_key", "user_id"} {
				got, found := request[field]
				if field == test.wantField {
					want := expectedOpenAICompatibleCacheRoutingKey(config, params)
					if !found || got != want {
						t.Fatalf("%s = %#v, found=%v, want %q", field, got, found, want)
					}
				} else if found {
					t.Fatalf("unexpected %s = %#v", field, got)
				}
			}
		})
	}
}

func TestCacheRoutingOnDoesNotSilentlyDropRejectedBestEffortKey(t *testing.T) {
	attempts := 0
	config := Config{
		ProviderName:           "groq",
		APIKey:                 "test-key",
		Model:                  "cache-test-model",
		CacheRoutingPreference: CacheRoutingOn,
	}
	params := Params{
		Messages:       []types.Message{types.UserMessage("hello")},
		PromptCacheKey: "forced-lineage",
		UsePromptCache: true,
	}
	wantCacheKey := expectedOpenAICompatibleCacheRoutingKey(config, params)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		attempts++
		body, _ := io.ReadAll(request.Body)
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if got := payload["prompt_cache_key"]; got != wantCacheKey {
			t.Errorf("prompt_cache_key = %#v", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"param":"prompt_cache_key","message":"unsupported prompt_cache_key"}}`)
	}))
	defer server.Close()

	config.BaseURL = server.URL
	client := NewOpenAI(config)
	_, err := client.CreateStream(context.Background(), params)
	if err == nil {
		t.Fatal("forced cache routing unexpectedly ignored rejection")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want one strict request", attempts)
	}
}

func TestBestEffortDoesNotLearnPromptCacheKeyValueValidationAsUnsupported(t *testing.T) {
	attempts := 0
	config := Config{ProviderName: "groq", APIKey: "test-key", Model: "cache-test-model"}
	params := Params{
		Messages:       []types.Message{types.UserMessage("hello")},
		PromptCacheKey: "invalid-value",
		UsePromptCache: true,
	}
	wantCacheKey := expectedOpenAICompatibleCacheRoutingKey(config, params)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		attempts++
		body, _ := io.ReadAll(request.Body)
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if got := payload["prompt_cache_key"]; got != wantCacheKey {
			t.Errorf("prompt_cache_key = %#v", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"param":"prompt_cache_key","message":"prompt_cache_key value is too long"}}`)
	}))
	defer server.Close()

	config.BaseURL = server.URL
	client := NewOpenAI(config)
	for index := 0; index < 2; index++ {
		if _, err := client.CreateStream(context.Background(), params); err == nil {
			t.Fatalf("CreateStream %d unexpectedly succeeded", index+1)
		}
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want one strict validation failure per request", attempts)
	}
}

func TestExplicitPromptCacheKeyRejectionClassification(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want bool
	}{
		{name: "structured unsupported parameter", body: `{"error":{"param":"prompt_cache_key","message":"parameter is not supported"}}`, want: true},
		{name: "structured unknown code", body: `{"error":{"param":"prompt_cache_key","code":"unknown_parameter","message":"invalid request"}}`, want: true},
		{name: "pydantic extra field", body: `{"detail":[{"type":"extra_forbidden","loc":["body","prompt_cache_key"],"msg":"Extra inputs are not permitted"}]}`, want: true},
		{name: "unstructured named field", body: `unsupported parameter: prompt_cache_key`, want: true},
		{name: "value too long", body: `{"error":{"param":"prompt_cache_key","message":"prompt_cache_key value is too long"}}`},
		{name: "value not allowed", body: `{"error":{"param":"prompt_cache_key","message":"prompt_cache_key value is not allowed"}}`},
		{name: "different field", body: `{"error":{"param":"temperature","message":"parameter is not supported; prompt_cache_key remains valid"}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := explicitlyRejectsPromptCacheKey([]byte(test.body)); got != test.want {
				t.Fatalf("classification = %v, want %v for %s", got, test.want, test.body)
			}
		})
	}
}

func TestCacheRoutingRejectionMemoryExpires(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	memory := &cacheRoutingRejectionMemory{
		ttl: time.Minute,
		now: func() time.Time { return now },
	}
	memory.remember("model-a")
	if !memory.has("model-a") {
		t.Fatal("fresh rejection was not remembered")
	}
	now = now.Add(time.Minute)
	if memory.has("model-a") {
		t.Fatal("expired rejection remained active")
	}
}

func TestResponsesCacheRoutingOffOmitsPromptCacheKey(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: response.created\ndata: {\"id\":\"resp_cache_off\"}\n\n")
		_, _ = io.WriteString(w, "event: response.completed\ndata: {\"response\":{\"id\":\"resp_cache_off\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1},\"output\":[]}}\n\n")
	}))
	defer server.Close()

	client := NewResponses(Config{
		APIKey:                 "test-key",
		BaseURL:                server.URL,
		CacheRoutingPreference: CacheRoutingOff,
	})
	stream, err := client.CreateStream(context.Background(), Params{
		Messages:       []types.Message{types.UserMessage("hello")},
		PromptCacheKey: "cache-lineage",
		UsePromptCache: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for range stream {
	}
	if _, found := captured["prompt_cache_key"]; found {
		t.Fatalf("Responses request retained prompt_cache_key while routing was off: %#v", captured)
	}
	if got := client.Capabilities().CacheRouting; got != CacheRoutingNone {
		t.Fatalf("Responses cache routing = %q, want none", got)
	}
}

func TestBestEffortRejectionMemoryIsIsolatedByModel(t *testing.T) {
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		requests = append(requests, payload)
		if payload["model"] == "model-a" && payload["prompt_cache_key"] != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = io.WriteString(w, `{"error":{"message":"unsupported parameter: prompt_cache_key"}}`)
			return
		}
		writeOpenAICompatibleCacheResponse(w)
	}))
	defer server.Close()

	client := NewOpenAI(Config{ProviderName: "groq", APIKey: "test-key", BaseURL: server.URL, Model: "model-a"})
	call := func(model string) {
		t.Helper()
		stream, err := client.CreateStream(context.Background(), Params{
			Model:          model,
			Messages:       []types.Message{types.UserMessage("hello")},
			PromptCacheKey: "cache-lineage",
			UsePromptCache: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		for range stream {
		}
	}
	call("model-a")
	call("model-a")
	call("model-b")

	if len(requests) != 4 {
		t.Fatalf("requests = %d, want model-a probe+fallback, remembered model-a, and model-b probe", len(requests))
	}
	for index, request := range requests {
		if _, ok := request["max_tokens"]; !ok {
			t.Fatalf("request %d lost compatible max_tokens during cache fallback: %#v", index+1, request)
		}
		if _, ok := request["max_completion_tokens"]; ok {
			t.Fatalf("request %d restored max_completion_tokens during cache fallback: %#v", index+1, request)
		}
	}
	if _, found := requests[2]["prompt_cache_key"]; found {
		t.Fatalf("remembered model-a request retained prompt_cache_key: %#v", requests[2])
	}
	wantModelB := expectedOpenAICompatibleCacheRoutingKey(
		Config{ProviderName: "groq", APIKey: "test-key", Model: "model-a"},
		Params{Model: "model-b", PromptCacheKey: "cache-lineage"},
	)
	if got := requests[3]["prompt_cache_key"]; got != wantModelB {
		t.Fatalf("model-b prompt_cache_key = %#v, want isolated probe %q", got, wantModelB)
	}
}
