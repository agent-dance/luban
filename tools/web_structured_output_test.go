package tools

import (
	"context"
	"testing"
)

func TestBuildWebFetchStructuredResultAddsTypedOutput(t *testing.T) {
	result := buildWebFetchStructuredResult("https://example.com", "find rate limits", "Rate limits are 100 requests per minute.", webExecutionModeLocalFallback)
	output, ok := result.Data.(WebFetchOutput)
	if !ok || output.Result != result.Content || output.Code != 200 || output.Bytes == 0 {
		t.Fatalf("expected TS typed output, got %#v", result)
	}
	if len(result.ContentBlocks) != 0 {
		t.Fatalf("Go compatibility block must not alter model-visible output: %+v", result.ContentBlocks)
	}
}

func TestBuildWebSearchStructuredResultAddsProgressBlocks(t *testing.T) {
	results := []searchResult{{Title: "Go", URL: "https://go.dev", Snippet: "The Go Programming Language"}}
	result := buildWebSearchStructuredResult("golang", results, "instant_answer_empty", webExecutionModeLocalFallback)
	if len(result.ContentBlocks) < 3 {
		t.Fatalf("expected progress-related content blocks, got %+v", result.ContentBlocks)
	}
}

func TestWebSearchResultIncludesStructuredBlocks(t *testing.T) {
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
	if len(result.ContentBlocks) == 0 {
		t.Fatalf("expected content blocks in search result")
	}
}
