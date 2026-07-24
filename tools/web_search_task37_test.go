package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestWebSearchTask37SchemaAndValidationPrecedeProgress(t *testing.T) {
	tool := NewWebSearchTool(nil)
	schema := tool.Schema()
	if !schema.RejectsUnknownFields() {
		t.Fatal("WebSearch schema must be strict")
	}
	query, ok := schema.Properties["query"].(map[string]any)
	if !ok || query["minLength"] != 2 {
		t.Fatalf("query schema = %#v, want minLength=2", schema.Properties["query"])
	}

	var progress []WebSearchProgressEvent
	tool.OnProgress = func(event WebSearchProgressEvent) { progress = append(progress, event) }
	missing, err := tool.Execute(context.Background(), map[string]any{"query": ""})
	if err != nil || !missing.IsError || missing.Content != "Error: Missing query" || missing.Metadata["errorCode"] != "1" {
		t.Fatalf("missing query = %+v err=%v", missing, err)
	}
	conflict, err := tool.Execute(context.Background(), map[string]any{
		"query": "go", "allowed_domains": []any{"go.dev"}, "blocked_domains": []any{"example.com"},
	})
	if err != nil || !conflict.IsError || conflict.Content != "Error: Cannot specify both allowed_domains and blocked_domains in the same request" || conflict.Metadata["errorCode"] != "2" {
		t.Fatalf("conflict = %+v err=%v", conflict, err)
	}
	if len(progress) != 0 {
		t.Fatalf("invalid inputs emitted progress: %+v", progress)
	}
}

func TestWebSearchTask37ProviderNativeOutputProgressAndFreshExecution(t *testing.T) {
	tool := NewWebSearchTool(nil)
	calls := 0
	tool.SetWebSearchServerToolProvider(WebSearchServerToolFunc(func(_ context.Context, req WebSearchServerToolRequest) (WebSearchServerToolResponse, error) {
		calls++
		if req.MaxUses != 8 || len(req.AllowedDomains) != 1 || req.AllowedDomains[0] != "go.dev" {
			t.Fatalf("server request = %+v", req)
		}
		req.OnProgress(WebSearchProgressEvent{Type: "query_update", ToolUseID: "srv_1", Query: "golang docs"})
		req.OnProgress(WebSearchProgressEvent{Type: "search_results_received", ToolUseID: "srv_1", Query: "golang docs", ResultCount: 1})
		return WebSearchServerToolResponse{
			ResultBlocks: []WebSearchServerToolResult{{
				ToolUseID: "srv_1",
				Results:   []WebSearchResult{{Title: "Go", URL: "https://go.dev", Snippet: "must not enter Links JSON"}},
			}},
			DurationMs: 1250,
			Usage:      types.Usage{ServerToolUse: types.ServerToolUsage{WebSearchRequests: 1}},
		}, nil
	}), true)

	var progress []WebSearchProgressEvent
	tool.OnProgress = func(event WebSearchProgressEvent) { progress = append(progress, event) }
	for i := 0; i < 2; i++ {
		result, err := tool.Execute(context.Background(), map[string]any{
			"query": "golang", "allowed_domains": []any{"go.dev"},
		})
		if err != nil || result.IsError {
			t.Fatalf("call %d result=%+v err=%v", i, result, err)
		}
		output, ok := result.Data.(WebSearchOutput)
		if !ok || output.Query != "golang" || output.DurationSeconds != 1.25 || len(output.Results) != 1 {
			t.Fatalf("typed output = %#v", result.Data)
		}
		search, ok := output.Results[0].(WebSearchOutputSearchResult)
		if !ok || search.ToolUseID != "srv_1" || len(search.Content) != 1 || search.Content[0].URL != "https://go.dev" {
			t.Fatalf("typed search result = %#v", output.Results[0])
		}
		if result.Usage == nil || result.Usage.ServerToolUse.WebSearchRequests != 1 {
			t.Fatalf("result usage = %+v", result.Usage)
		}
		mapped := tool.MapToolResultToToolResultBlock(output, "outer_1")
		if mapped.ToolUseID != "outer_1" || !strings.Contains(mapped.Content, `Links: [{"title":"Go","url":"https://go.dev"}]`) || strings.Contains(mapped.Content, "must not enter Links JSON") {
			t.Fatalf("mapped content = %q", mapped.Content)
		}
	}
	if calls != 2 {
		t.Fatalf("provider-native calls = %d, want fresh execution on every call", calls)
	}
	if len(progress) != 4 || progress[0].Type != "query_update" || progress[1].ResultCount != 1 || progress[1].Query != "golang docs" {
		t.Fatalf("progress = %+v", progress)
	}
}

func TestWebSearchTask37ServerErrorIsOutputStringAndNeverFallsBack(t *testing.T) {
	tool := NewWebSearchTool(nil)
	tool.SetWebSearchServerToolProvider(WebSearchServerToolFunc(func(_ context.Context, _ WebSearchServerToolRequest) (WebSearchServerToolResponse, error) {
		return WebSearchServerToolResponse{ResultBlocks: []WebSearchServerToolResult{{ToolUseID: "srv_err", ErrorCode: "max_uses_exceeded"}}}, nil
	}), true)
	tool.doInstantSearch = func(context.Context, string) ([]searchResult, error) {
		t.Fatal("server-tool error must not invoke local fallback")
		return nil, nil
	}

	result, err := tool.Execute(context.Background(), map[string]any{"query": "golang"})
	if err != nil || result.IsError {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	output := result.Data.(WebSearchOutput)
	if len(output.Results) != 1 || output.Results[0] != "Web search error: max_uses_exceeded" {
		t.Fatalf("output = %#v", output)
	}
	if !strings.Contains(result.Content, "Web search error: max_uses_exceeded") {
		t.Fatalf("model content = %q", result.Content)
	}
}

func TestWebSearchTask37UnavailableFallbackPreservesURLsWithoutFakeProgress(t *testing.T) {
	tool := NewWebSearchTool(NewSearchCache())
	providerCalls := 0
	tool.SetWebSearchServerToolProvider(WebSearchServerToolFunc(func(_ context.Context, _ WebSearchServerToolRequest) (WebSearchServerToolResponse, error) {
		providerCalls++
		return WebSearchServerToolResponse{}, ErrWebSearchServerToolUnavailable
	}), true)
	localCalls := 0
	tool.doInstantSearch = func(context.Context, string) ([]searchResult, error) {
		localCalls++
		return []searchResult{{Title: "Go", URL: "https://go.dev", Snippet: "docs"}}, nil
	}
	var progress []WebSearchProgressEvent
	tool.OnProgress = func(event WebSearchProgressEvent) { progress = append(progress, event) }

	for i := 0; i < 2; i++ {
		result, err := tool.Execute(context.Background(), map[string]any{"query": "golang"})
		if err != nil || result.IsError || strings.Contains(result.Content, "cache://websearch") || !strings.Contains(result.Content, "https://go.dev") {
			t.Fatalf("call %d result=%+v err=%v", i, result, err)
		}
	}
	if providerCalls != 2 || localCalls != 1 {
		t.Fatalf("provider/local calls = %d/%d, want 2/1", providerCalls, localCalls)
	}
	if len(progress) != 0 {
		t.Fatalf("unavailable/local cache path emitted fake server progress: %+v", progress)
	}
}

func TestWebSearchTask37ProviderFailureDoesNotFallback(t *testing.T) {
	tool := NewWebSearchTool(nil)
	tool.SetWebSearchServerToolProvider(WebSearchServerToolFunc(func(_ context.Context, _ WebSearchServerToolRequest) (WebSearchServerToolResponse, error) {
		return WebSearchServerToolResponse{}, errors.New("provider down")
	}), true)
	tool.doInstantSearch = func(context.Context, string) ([]searchResult, error) {
		t.Fatal("provider failure must not be masked by fallback")
		return nil, nil
	}
	result, err := tool.Execute(context.Background(), map[string]any{"query": "golang"})
	if err != nil || !result.IsError || !strings.Contains(result.Content, "provider down") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
