package tools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestWebFetchProviderNativeMode(t *testing.T) {
	tool := NewWebFetchTool(NewSearchCache())
	tool.mode = webExecutionModeProviderNative
	tool.providerFetch = func(ctx context.Context, input WebFetchInput) (webFetchStructuredPayload, error) {
		return webFetchStructuredPayload{
			URL:     input.URL,
			Prompt:  input.Prompt,
			Method:  string(webExecutionModeProviderNative),
			Summary: "Provider-native fetched summary",
		}, nil
	}
	result, err := tool.Execute(context.Background(), map[string]any{"url": "https://example.com", "prompt": "summarize"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	norm := normalizeStructuredWebFetchToolResult(result, "https://example.com", "summarize", false)
	if norm.Execution.Method != string(webExecutionModeProviderNative) {
		t.Fatalf("expected provider_native method, got %+v", norm)
	}
}

func TestWebSearchProviderNativeMode(t *testing.T) {
	tool := NewWebSearchTool(NewSearchCache())
	tool.mode = webExecutionModeProviderNative
	tool.providerSearch = func(ctx context.Context, input WebSearchInput) (webSearchStructuredPayload, error) {
		return webSearchStructuredPayload{
			Query:  input.Query,
			Method: string(webExecutionModeProviderNative),
			Results: []searchResult{{
				Title:   "Provider result",
				URL:     "https://example.com/provider",
				Snippet: "Provider-native search result",
			}},
			Progress: []string{"query_update", "search_results_received"},
		}, nil
	}
	result, err := tool.Execute(context.Background(), map[string]any{"query": "golang"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	norm := normalizeStructuredWebSearchToolResult(result, "golang", nil, nil, false)
	if norm.Execution.Method != string(webExecutionModeProviderNative) {
		t.Fatalf("expected provider_native method, got %+v", norm)
	}
}

func TestWebFetchProviderNativeMatchesSrcHarnessMethod(t *testing.T) {
	tool := NewWebFetchTool(NewSearchCache())
	tool.mode = webExecutionModeProviderNative
	tool.providerFetch = func(ctx context.Context, input WebFetchInput) (webFetchStructuredPayload, error) {
		return webFetchStructuredPayload{URL: input.URL, Prompt: input.Prompt, Method: string(webExecutionModeProviderNative), Summary: "Provider fetched summary for summarize"}, nil
	}
	result, err := tool.Execute(context.Background(), map[string]any{"url": "https://example.com", "prompt": "summarize"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	goNorm := normalizeStructuredWebFetchToolResult(result, "https://example.com", "summarize", false)
	src := runSrcHarness(t, "webfetch", "webfetch_mock_execution_input.json")
	var srcNorm webFetchNormalizedResult
	if err := json.Unmarshal(src.NormalizedResult, &srcNorm); err != nil {
		t.Fatalf("unmarshal src normalized result: %v", err)
	}
	if goNorm.Execution.Method != srcNorm.Execution.Method {
		t.Fatalf("expected matched method, got go=%q src=%q", goNorm.Execution.Method, srcNorm.Execution.Method)
	}
}

func TestWebSearchProviderNativeMatchesSrcHarnessMethod(t *testing.T) {
	tool := NewWebSearchTool(NewSearchCache())
	tool.mode = webExecutionModeProviderNative
	tool.providerSearch = func(ctx context.Context, input WebSearchInput) (webSearchStructuredPayload, error) {
		return webSearchStructuredPayload{
			Query:  input.Query,
			Method: string(webExecutionModeProviderNative),
			Results: []searchResult{{
				Title:   "The Go Programming Language",
				URL:     "https://go.dev",
				Snippet: "Build simple, secure, scalable systems with Go.",
			}},
			Progress: []string{"query_update", "search_results_received"},
		}, nil
	}
	result, err := tool.Execute(context.Background(), map[string]any{"query": "golang", "allowed_domains": []any{"go.dev"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	goNorm := normalizeStructuredWebSearchToolResult(result, "golang", []string{"go.dev"}, nil, false)
	src := runSrcHarness(t, "websearch", "websearch_mock_execution_input.json")
	var srcNorm webSearchNormalizedResult
	if err := json.Unmarshal(src.NormalizedResult, &srcNorm); err != nil {
		t.Fatalf("unmarshal src normalized result: %v", err)
	}
	if goNorm.Execution.Method != srcNorm.Execution.Method {
		t.Fatalf("expected matched method, got go=%q src=%q", goNorm.Execution.Method, srcNorm.Execution.Method)
	}
}
