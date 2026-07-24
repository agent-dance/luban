package tools

import (
	"context"
	"strings"
	"testing"
)

func TestWebFetchCacheKeyUsesOriginalURLOnly(t *testing.T) {
	a := makeWebFetchCacheKey("https://example.com/docs", "Summarize installation steps")
	b := makeWebFetchCacheKey("https://example.com/docs", "Find rate limits")
	if a != b {
		t.Fatalf("prompt must not affect fetched-content key: %q != %q", a, b)
	}
}

func TestExtractRelevantContentUsesPromptKeywords(t *testing.T) {
	body := "Installation steps: download the binary and run setup.\n\nRate limits: 100 requests per minute.\n\nOverview: general information."
	got := extractRelevantContent(body, "Find rate limits")
	if !strings.Contains(strings.ToLower(got), "rate limits") {
		t.Fatalf("expected relevant content to include rate limits, got %q", got)
	}
}

func TestWebSearchRejectsAllowedAndBlockedDomainsTogether(t *testing.T) {
	tool := NewWebSearchTool(NewSearchCache())
	result, err := tool.Execute(context.Background(), map[string]any{
		"query":           "golang",
		"allowed_domains": []any{"go.dev"},
		"blocked_domains": []any{"example.com"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError || result.Content != "Error: Cannot specify both allowed_domains and blocked_domains in the same request" || result.Metadata["errorCode"] != "2" {
		t.Fatalf("expected validation error, got %+v", result)
	}
}
