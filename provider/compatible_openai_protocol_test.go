package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestUserCompatibleResponsesRequestPreservesToolsAndReasoning(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/responses" {
			t.Errorf("request path = %q, want /v1/responses", request.URL.Path)
		}
		if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(writer, "event: response.completed\ndata: {\"response\":{\"id\":\"resp_compatible\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1},\"output\":[]}}\n\n")
	}))
	defer server.Close()

	baseURL := server.URL + "/v1"
	// Reproduce auth.json entries written by the old discovery path: the
	// persisted model says Chat even though the built-in model is Responses.
	registry, config := newPersistedLegacyCompatibleProtocolTestRegistry(t, baseURL)
	client, err := registry.Create("custom-gateway", config, "gpt-5.6-sol")
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	retry, ok := client.(*RetryProvider)
	if !ok {
		t.Fatalf("provider = %T, want *RetryProvider", client)
	}
	if _, ok := retry.inner.(*openAIProtocolProvider); !ok {
		t.Fatalf("inner provider = %T, want *openAIProtocolProvider", retry.inner)
	}

	stream, err := client.CreateStream(context.Background(), compatibleProtocolTestParams())
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	responseID := ""
	for event := range stream {
		if event.Type == types.EventMessageStop {
			responseID = event.ResponseID
		}
	}
	if responseID != "resp_compatible" {
		t.Fatalf("response ID = %q, want native Responses ID", responseID)
	}

	reasoning, _ := requestBody["reasoning"].(map[string]any)
	if reasoning["effort"] != "xhigh" {
		t.Fatalf("Responses reasoning = %#v, want xhigh effort", requestBody["reasoning"])
	}
	tools, _ := requestBody["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("Responses tools = %#v, want one tool", requestBody["tools"])
	}
	tool, _ := tools[0].(map[string]any)
	if tool["type"] != "function" || tool["name"] != "Read" || tool["parameters"] == nil {
		t.Fatalf("Responses tool = %#v, want native function shape", tool)
	}
	if _, nested := tool["function"]; nested {
		t.Fatalf("Responses tool used Chat Completions nesting: %#v", tool)
	}
}

func TestUserCompatibleExplicitChatSelectionRemainsAuthoritative(t *testing.T) {
	var paths []string
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		if err := json.NewDecoder(request.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(writer, "data: {\"id\":\"chatcmpl_explicit\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-5.6-sol\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	baseURL := server.URL + "/v1"
	registry := newUserCompatibleProtocolTestRegistry(baseURL, "chat-completions")
	client, err := registry.Create("custom-gateway", Config{
		APIKey: "test-key", APIStyle: APIStyleOpenAI, APIFormat: "chat-completions", BaseURL: baseURL,
	}, "gpt-5.6-sol")
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	retry, ok := client.(*RetryProvider)
	if !ok {
		t.Fatalf("provider = %T, want *RetryProvider", client)
	}
	if _, ok := retry.inner.(*OpenAIProvider); !ok {
		t.Fatalf("inner provider = %T, want *OpenAIProvider", retry.inner)
	}

	stream, err := client.CreateStream(context.Background(), compatibleProtocolTestParams())
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range stream {
	}
	if len(paths) != 1 || paths[0] != "/v1/chat/completions" {
		t.Fatalf("request paths = %v, want Chat Completions only", paths)
	}
	if requestBody["reasoning_effort"] != "xhigh" {
		t.Fatalf("Chat reasoning_effort = %#v, want xhigh", requestBody["reasoning_effort"])
	}
	tools, _ := requestBody["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("Chat tools = %#v, want one tool", requestBody["tools"])
	}
	tool, _ := tools[0].(map[string]any)
	if _, nested := tool["function"].(map[string]any); !nested {
		t.Fatalf("Chat tool = %#v, want nested function shape", tool)
	}
}

func TestUserCompatibleResponsesFallbackIsRemembered(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		paths = append(paths, request.URL.Path)
		mu.Unlock()
		switch request.URL.Path {
		case "/v1/responses":
			http.NotFound(writer, request)
		case "/v1/chat/completions":
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(writer, "data: {\"id\":\"chatcmpl_fallback\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-5.6-sol\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		default:
			http.Error(writer, "unexpected path", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	baseURL := server.URL + "/v1"
	registry := newUserCompatibleProtocolTestRegistry(baseURL, "chat-completions")
	client, err := registry.Create("custom-gateway", Config{
		APIKey: "test-key", APIStyle: APIStyleOpenAI, BaseURL: baseURL,
	}, "gpt-5.6-sol")
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		stream, streamErr := client.CreateStream(context.Background(), compatibleProtocolTestParams())
		if streamErr != nil {
			t.Fatalf("CreateStream attempt %d: %v", attempt+1, streamErr)
		}
		for range stream {
		}
	}

	mu.Lock()
	defer mu.Unlock()
	want := []string{"/v1/responses", "/v1/chat/completions", "/v1/chat/completions"}
	if len(paths) != len(want) {
		t.Fatalf("request paths = %v, want %v", paths, want)
	}
	for index := range want {
		if paths[index] != want[index] {
			t.Fatalf("request paths = %v, want %v", paths, want)
		}
	}
}

func TestProtocolFallbackConsumesSharedAttemptBudget(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		http.NotFound(writer, request)
	}))
	defer server.Close()

	baseURL := server.URL + "/v1"
	registry := newUserCompatibleProtocolTestRegistry(baseURL, "chat-completions")
	client, err := registry.Create("custom-gateway", Config{
		APIKey: "test-key", APIStyle: APIStyleOpenAI, BaseURL: baseURL,
	}, "gpt-5.6-sol")
	if err != nil {
		t.Fatal(err)
	}
	retrying := client.(*RetryProvider)
	retrying.config = normalizeRetryConfig(RetryConfig{MaxAttempts: 1})
	if _, err = client.CreateStream(context.Background(), compatibleProtocolTestParams()); !IsAttemptLimit(err) {
		t.Fatalf("protocol fallback error = %v, want shared attempt limit", err)
	}
	if len(paths) != 1 || paths[0] != "/v1/responses" {
		t.Fatalf("request paths = %v, want only budgeted Responses attempt", paths)
	}
}

func TestUserCompatibleAnthropicStyleRemainsAuthoritativeOverWireOverride(t *testing.T) {
	registry := newUserCompatibleProtocolTestRegistry("https://gateway.example", "chat-completions")
	client, err := registry.Create("custom-gateway", Config{
		APIKey: "test-key", APIStyle: APIStyleAnthropic, APIFormat: "responses", BaseURL: "https://gateway.example",
	}, "gpt-5.6-sol")
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	retry, ok := client.(*RetryProvider)
	if !ok {
		t.Fatalf("provider = %T, want *RetryProvider", client)
	}
	if _, ok := retry.inner.(*AnthropicProvider); !ok {
		t.Fatalf("inner provider = %T, want *AnthropicProvider", retry.inner)
	}
}

func TestUserCompatibleExplicitResponsesDoesNotFallback(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		http.NotFound(writer, request)
	}))
	defer server.Close()

	baseURL := server.URL + "/v1"
	registry := newUserCompatibleProtocolTestRegistry(baseURL, "chat-completions")
	client, err := registry.Create("custom-gateway", Config{
		APIKey: "test-key", APIStyle: APIStyleOpenAI, APIFormat: "responses", BaseURL: baseURL,
	}, "gpt-5.6-sol")
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	retry, ok := client.(*RetryProvider)
	if !ok {
		t.Fatalf("provider = %T, want *RetryProvider", client)
	}
	if _, ok := retry.inner.(*ResponsesProvider); !ok {
		t.Fatalf("inner provider = %T, want *ResponsesProvider", retry.inner)
	}
	if _, streamErr := client.CreateStream(context.Background(), compatibleProtocolTestParams()); streamErr == nil {
		t.Fatal("explicit Responses request unexpectedly succeeded")
	}
	if len(paths) != 1 || paths[0] != "/v1/responses" {
		t.Fatalf("request paths = %v, want explicit Responses only", paths)
	}
}

func newUserCompatibleProtocolTestRegistry(baseURL, apiFormat string) *ProviderRegistry {
	registry := NewProviderRegistry()
	registry.catalog = DefaultCatalog()
	registry.RegisterCompatibleProvider(CompatibleProviderDefinition{
		Name: "custom-gateway", DisplayName: "Custom Gateway", UserDefined: true,
		BaseURLs: map[APIStyle]string{APIStyleOpenAI: baseURL, APIStyleAnthropic: baseURL},
	})
	registry.ReplaceProviderModels("custom-gateway", []ModelInfo{{
		ID: "gpt-5.6-sol", Name: "GPT-5.6 Sol", Provider: "custom-gateway",
		CanReason: true, CanUseTools: true, APIFormat: apiFormat, CostCurrency: "USD",
	}})
	return registry
}

func newPersistedLegacyCompatibleProtocolTestRegistry(t *testing.T, baseURL string) (*ProviderRegistry, Config) {
	t.Helper()
	store, err := NewCredentialStoreAt(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatalf("create credential store: %v", err)
	}
	if err := store.Set(CredentialEntry{
		Provider: "custom-gateway", AuthMethod: "api_key", APIKey: "test-key",
		BaseURL: baseURL, APIStyle: APIStyleOpenAI, UserDefined: true,
		Models: []ModelInfo{{
			ID: "gpt-5.6-sol", Name: "GPT-5.6 Sol", Provider: "custom-gateway",
			CanReason: true, CanUseTools: true, APIFormat: "chat-completions", CostCurrency: "USD",
		}},
	}); err != nil {
		t.Fatalf("persist legacy credential: %v", err)
	}
	registry := NewProviderRegistry()
	registry.catalog = DefaultCatalog()
	registry.SetCredentialStore(store)
	config, err := ResolveCredentialConfig(registry, "custom-gateway")
	if err != nil {
		t.Fatalf("resolve persisted config: %v", err)
	}
	if config.APIFormat != "" {
		t.Fatalf("legacy catalog metadata became explicit wire override: %q", config.APIFormat)
	}
	return registry, config
}

func compatibleProtocolTestParams() Params {
	return Params{
		Messages:        []types.Message{types.UserMessage("hello")},
		ReasoningEffort: "xhigh",
		Tools: []types.ToolDefinition{{
			Name: "Read", Description: "Read a file",
			InputSchema: types.JSONSchema{
				Type: "object",
				Properties: map[string]any{
					"path": map[string]any{"type": "string"},
				},
				Required: []string{"path"},
			},
		}},
	}
}
