package tools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestWebFetchVsSrcHarnessNormalizedGap_LocalFallback(t *testing.T) {
	result := buildWebFetchStructuredResult("https://example.com", "summarize", "Go local fallback summary", webExecutionModeLocalFallback)
	goNorm := normalizeStructuredWebFetchToolResult(result, "https://example.com", "summarize", false)

	src := runSrcHarness(t, "webfetch", "webfetch_mock_execution_input.json")
	var srcNorm webFetchNormalizedResult
	if err := json.Unmarshal(src.NormalizedResult, &srcNorm); err != nil {
		t.Fatalf("unmarshal src normalized result: %v", err)
	}
	if goNorm.Execution.Method == srcNorm.Execution.Method {
		t.Fatalf("expected implementation gap to remain visible, both methods were %q", goNorm.Execution.Method)
	}
}

func TestWebSearchVsSrcHarnessProviderNativeParity(t *testing.T) {
	tool := NewWebSearchTool(NewSearchCache())
	tool.SetWebSearchServerToolProvider(WebSearchServerToolFunc(func(_ context.Context, req WebSearchServerToolRequest) (WebSearchServerToolResponse, error) {
		return WebSearchServerToolResponse{ResultBlocks: []WebSearchServerToolResult{{
			ToolUseID: "srv_1",
			Results:   []WebSearchResult{{Title: "The Go Programming Language", URL: "https://go.dev", Snippet: "Build simple, secure, scalable systems with Go."}},
		}}}, nil
	}), true)
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
		t.Fatalf("provider-native method mismatch: go=%q src=%q", goNorm.Execution.Method, srcNorm.Execution.Method)
	}
}
