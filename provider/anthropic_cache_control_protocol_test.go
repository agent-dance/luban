package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agent-dance/luban/prompt"
	"github.com/agent-dance/luban/types"
)

func TestAnthropicCacheControlUsesAtMostFourDocumentedBreakpoints(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_cache\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4-6\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n")
		_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	systemBlocks := make([]prompt.SystemPromptBlock, 5)
	for i := range systemBlocks {
		systemBlocks[i] = prompt.SystemPromptBlock{
			Text:       "stable system block",
			Cache:      true,
			CacheScope: prompt.CacheScopeOrg,
		}
	}
	client := NewAnthropic(Config{AuthToken: "test-token", BaseURL: server.URL, Model: "claude-sonnet-4-6"})
	stream, err := client.CreateStream(context.Background(), Params{
		SystemBlocks: systemBlocks,
		Tools: []types.ToolDefinition{{
			Name:        "read_file",
			Description: "read a file",
			InputSchema: types.StrictObjectSchema(map[string]any{"path": map[string]any{"type": "string"}}, "path"),
		}},
		Messages: []types.Message{types.UserMessage("hello")},
	})
	if err != nil {
		t.Fatal(err)
	}
	for range stream {
	}

	if got := countJSONKey(request, "cache_control"); got != 4 {
		t.Fatalf("cache_control breakpoint count = %d, want documented maximum 4: %#v", got, request)
	}
	if got := countJSONKey(request["system"], "cache_control"); got != 2 {
		t.Fatalf("system cache_control breakpoint count = %d, want 2 after reserving tool and message breakpoints", got)
	}
	assertNoJSONKey(t, request, "scope")
}

func TestVertexCustomEndpointUsesAnthropicCacheControl(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_vertex_cache\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4-6\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n")
		_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	baseURL := strings.Replace(server.URL, "127.0.0.1", "localhost", 1)
	client, err := NewVertexCustomEndpoint(Config{APIKey: "test-key", BaseURL: baseURL, Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.CreateStream(context.Background(), nativeCacheProtocolParams())
	if err != nil {
		t.Fatal(err)
	}
	for range stream {
	}
	assertNativeAnthropicCacheRequest(t, request)
}

func TestBedrockInvokeModelUsesAnthropicCacheControl(t *testing.T) {
	var (
		request     map[string]any
		requestPath string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestPath == "" {
			requestPath = r.URL.EscapedPath()
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if request == nil {
			request = payload
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"message":"test response after request capture"}`)
	}))
	defer server.Close()

	baseURL := strings.Replace(server.URL, "127.0.0.1", "localhost", 1)
	client, err := NewBedrock(context.Background(), BedrockConfig{
		Region:      "us-east-1",
		Model:       "anthropic.claude-sonnet-4-6",
		BearerToken: "test-token",
		BaseURL:     baseURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.CreateStream(context.Background(), nativeCacheProtocolParams())
	if err != nil {
		t.Fatal(err)
	}
	for range stream {
	}
	if !strings.HasSuffix(requestPath, "/invoke-with-response-stream") {
		t.Fatalf("Bedrock request path = %q, want InvokeModel streaming path", requestPath)
	}
	assertNativeAnthropicCacheRequest(t, request)
}

func nativeCacheProtocolParams() Params {
	return Params{
		SystemBlocks: []prompt.SystemPromptBlock{{
			Text:       "stable system block",
			Cache:      true,
			CacheScope: prompt.CacheScopeGlobal,
		}},
		Tools: []types.ToolDefinition{{
			Name:        "read_file",
			Description: "read a file",
			InputSchema: types.StrictObjectSchema(map[string]any{"path": map[string]any{"type": "string"}}, "path"),
		}},
		Messages: []types.Message{types.UserMessage("hello")},
	}
}

func assertNativeAnthropicCacheRequest(t *testing.T, request map[string]any) {
	t.Helper()
	if request == nil {
		t.Fatal("request was not captured")
	}
	if got := countJSONKey(request, "cache_control"); got != 3 {
		t.Fatalf("native cache_control breakpoint count = %d, want system, tool, and message breakpoints: %#v", got, request)
	}
	for _, unsupported := range []string{"scope", "cachePoint", "prompt_cache_key"} {
		assertNoJSONKey(t, request, unsupported)
	}
}

func countJSONKey(value any, key string) int {
	switch typed := value.(type) {
	case map[string]any:
		count := 0
		for nestedKey, nestedValue := range typed {
			if nestedKey == key {
				count++
			}
			count += countJSONKey(nestedValue, key)
		}
		return count
	case []any:
		count := 0
		for _, nestedValue := range typed {
			count += countJSONKey(nestedValue, key)
		}
		return count
	default:
		return 0
	}
}

func assertNoJSONKey(t *testing.T, value any, key string) {
	t.Helper()
	if got := countJSONKey(value, key); got != 0 {
		t.Fatalf("request contains undocumented %q field %d time(s): %#v", key, got, value)
	}
}
