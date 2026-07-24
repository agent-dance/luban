package tools

import (
	"context"
	"strings"
	"testing"
)

func TestNormalizeStructuredWebFetchToolResult(t *testing.T) {
	result := buildWebFetchStructuredResult("https://example.com", "find rate limits", "Rate limits are 100 requests per minute.", webExecutionModeLocalFallback)
	norm := normalizeStructuredWebFetchToolResult(result, "https://example.com", "find rate limits", false)
	if norm.Execution.Method != "local_fallback" {
		t.Fatalf("unexpected method: %+v", norm)
	}
	if !strings.Contains(strings.ToLower(norm.Content.Body), "rate limits") {
		t.Fatalf("expected normalized body to contain summary, got %+v", norm)
	}
}

func TestNormalizeStructuredWebSearchToolResult(t *testing.T) {
	results := []searchResult{{Title: "Go", URL: "https://go.dev", Snippet: "The Go Programming Language"}}
	result := buildWebSearchStructuredResult("golang", results, "instant_answer_empty", webExecutionModeLocalFallback)
	norm := normalizeStructuredWebSearchToolResult(result, "golang", nil, nil, false)
	if norm.Execution.FallbackReason != "instant_answer_empty" {
		t.Fatalf("unexpected fallback reason: %+v", norm)
	}
	if len(norm.Results) != 1 || norm.Results[0].URL != "https://go.dev" {
		t.Fatalf("unexpected normalized results: %+v", norm)
	}
}

func TestWebSearchStructuredNormalizationFromExecute(t *testing.T) {
	tool := NewWebSearchTool(NewSearchCache())
	tool.doInstantSearch = func(ctx context.Context, query string) ([]searchResult, error) {
		return nil, nil
	}
	tool.doLiteSearch = func(ctx context.Context, query string) ([]searchResult, error) {
		return []searchResult{{Title: "Go", URL: "https://go.dev", Snippet: "The Go Programming Language"}}, nil
	}
	result, err := tool.Execute(context.Background(), map[string]any{"query": "golang"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	norm := normalizeStructuredWebSearchToolResult(result, "golang", nil, nil, false)
	if len(norm.Results) != 1 {
		t.Fatalf("expected one normalized result, got %+v", norm)
	}
}
