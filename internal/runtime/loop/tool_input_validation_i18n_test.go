package loop

import (
	"context"
	"testing"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type localizedStrictInputTool struct {
	executed bool
}

func (t *localizedStrictInputTool) Name() string        { return "LocalizedStrict" }
func (t *localizedStrictInputTool) Description() string { return "strict input fixture" }
func (t *localizedStrictInputTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{
		"value": map[string]any{"type": "string"},
	}, "value")
}
func (t *localizedStrictInputTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	t.executed = true
	return types.ToolResult{Content: "unexpected execution"}, nil
}

func TestConcurrentToolValidationUsesSingleLocalizedEnvelope(t *testing.T) {
	tool := &localizedStrictInputTool{}
	reg := registry.New()
	reg.Register(tool)

	got, err := executeOneTool(
		context.Background(),
		reg,
		nil,
		nil,
		"session-1",
		executioncontract.ToolExecutionContext{},
		types.ToolUseBlock{
			Type:  types.ContentTypeToolUse,
			ID:    "tool-use-1",
			Name:  tool.Name(),
			Input: map[string]any{"value": "ok", "extra": true},
		},
	)
	if err != nil {
		t.Fatalf("executeOneTool returned error: %v", err)
	}
	want := "<tool_use_error>InputValidationError: LocalizedStrict failed due to the following issue:\nAn unexpected parameter `extra` was provided</tool_use_error>"
	if got.Result.Content != want {
		t.Fatalf("validation result = %q, want %q", got.Result.Content, want)
	}
	if !got.Result.IsError || got.Result.Outcome != types.ToolOutcomeFailed || tool.executed {
		t.Fatalf("validation result = %#v, executed=%v", got.Result, tool.executed)
	}
}
