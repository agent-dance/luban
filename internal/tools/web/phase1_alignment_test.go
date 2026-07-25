package web

import (
	"context"
	"testing"
)

func TestWebSearchRejectsAllowedAndBlockedDomainsTogether(t *testing.T) {
	tool := NewWebSearchTool()
	result, err := tool.Execute(context.Background(), map[string]any{
		"query":           "golang",
		"allowed_domains": []any{"go.dev"},
		"blocked_domains": []any{"example.com"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError || result.Metadata["errorCode"] != "2" {
		t.Fatalf("expected validation error, got %+v", result)
	}
}
