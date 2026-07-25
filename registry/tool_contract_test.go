package registry

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

type strictContractTool struct {
	executed bool
	readOnly bool
}

func (t *strictContractTool) Name() string        { return "StrictContract" }
func (t *strictContractTool) Description() string { return "strict contract tool" }
func (t *strictContractTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{
		"message": map[string]any{"type": "string"},
	}, "message")
}
func (t *strictContractTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{
		ReadOnly:           t.readOnly,
		Write:              !t.readOnly,
		ConcurrencySafe:    t.readOnly,
		MaxResultSizeChars: 2048,
	}
}
func (t *strictContractTool) Execute(_ context.Context, input map[string]any) (types.ToolResult, error) {
	t.executed = true
	return types.ToolResult{Data: struct {
		Message string
	}{Message: input["message"].(string)}}, nil
}
func (t *strictContractTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	return types.ToolResultBlock{
		ToolUseID: toolUseID,
		Content:   "accepted: " + data.(struct{ Message string }).Message,
	}
}

func TestStrictToolRejectsUnknownInputBeforeExecute(t *testing.T) {
	tool := &strictContractTool{readOnly: true}
	if !tool.Schema().RejectsUnknownFields() {
		t.Fatal("strict tool schema permits unknown fields")
	}
	reg := New()
	reg.Register(tool)

	result := reg.ExecuteTool(context.Background(), tool.Name(), map[string]any{
		"message": "hello",
		"extra":   true,
	})
	if !result.IsError {
		t.Fatalf("unknown field result = %#v, want tool error", result)
	}
	if tool.executed {
		t.Fatal("strict validation ran after Execute")
	}
	if !strings.Contains(result.Content, "InputValidationError") ||
		!strings.Contains(result.Content, "`extra`") {
		t.Fatalf("unexpected strict validation text: %q", result.Content)
	}
}

func TestStrictToolMapsTypedResultData(t *testing.T) {
	tool := &strictContractTool{readOnly: true}
	reg := New()
	reg.Register(tool)

	result := reg.ExecuteTool(context.Background(), tool.Name(), map[string]any{"message": "hello"})
	if result.IsError || !tool.executed {
		t.Fatalf("valid execution failed: %#v", result)
	}
	if result.Content != "accepted: hello" {
		t.Fatalf("model content = %q", result.Content)
	}
	if result.Data == nil {
		t.Fatal("typed result data was dropped")
	}
	if result.Metadata["maxResultSizeChars"] != "2048" {
		t.Fatalf("max result metadata missing: %#v", result.Metadata)
	}
}

func TestMutatingToolMetadataRemainsSequentialAndWritable(t *testing.T) {
	tool := &strictContractTool{readOnly: false}
	reg := New()
	reg.Register(tool)
	metadata := reg.ToolMetadata(tool.Name(), nil)
	if metadata.ReadOnly || metadata.ConcurrencySafe || !metadata.Write {
		t.Fatalf("mutating tool metadata = %#v", metadata)
	}
	result := reg.ExecuteTool(context.Background(), tool.Name(), map[string]any{"message": "write"})
	if result.IsError || result.Content != "accepted: write" || !tool.executed {
		t.Fatalf("mutating contract execution failed: %#v", result)
	}
}
