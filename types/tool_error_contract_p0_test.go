package types

import (
	"context"
	"strings"
	"testing"
)

type p0ErrorTool struct{}

func (p0ErrorTool) Name() string        { return "P0Error" }
func (p0ErrorTool) Description() string { return "fixture" }
func (p0ErrorTool) Schema() JSONSchema  { return StrictObjectSchema(nil) }
func (p0ErrorTool) Execute(context.Context, map[string]any) (ToolResult, error) {
	return ToolResult{}, nil
}
func (p0ErrorTool) MapToolResultToToolResultBlock(any, string) ToolResultBlock {
	return ToolResultBlock{Content: "success mapper must not run"}
}

func TestP0ToolErrorDataProducesAllowlistedModelEnvelope(t *testing.T) {
	result := ToolResult{
		Content: "localized public message", IsError: true,
		Data: ToolErrorData{
			Schema: "tool_error/v1", Code: "file.edit.read_required", Retryable: true,
			Retry: &ToolErrorRetry{Action: "read_file", Tool: "Read", FilePath: "/tmp/a"},
		},
	}
	block := MapToolResult(p0ErrorTool{}, result, "toolu_error")
	if !strings.Contains(block.Content, "localized public message") || !strings.Contains(block.Content, "<tool_error>") || !strings.Contains(block.Content, "file.edit.read_required") {
		t.Fatalf("structured error envelope missing: %q", block.Content)
	}
}

func TestP0ArbitraryErrorDataCannotInvokeSuccessMapperOrLeak(t *testing.T) {
	result := ToolResult{
		Content: "safe public message", IsError: true,
		Data: map[string]any{"private_cause": "do-not-leak"},
	}
	block := MapToolResult(p0ErrorTool{}, result, "toolu_private")
	if block.Content != "safe public message" || strings.Contains(block.Content, "do-not-leak") || strings.Contains(block.Content, "success mapper") {
		t.Fatalf("arbitrary error Data leaked through mapper: %q", block.Content)
	}
}
