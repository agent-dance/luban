package registry

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

type parityDispatcherOutput struct {
	Message string `json:"message"`
}

type parityDispatcherTool struct {
	executed bool
}

func (t *parityDispatcherTool) Name() string        { return "ParityDispatcher" }
func (t *parityDispatcherTool) Description() string { return "parity dispatcher test tool" }
func (t *parityDispatcherTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(map[string]any{
		"message": map[string]any{"type": "string"},
	}, "message")
}
func (t *parityDispatcherTool) Execute(_ context.Context, input map[string]any) (types.ToolResult, error) {
	t.executed = true
	return types.ToolResult{Data: parityDispatcherOutput{Message: input["message"].(string)}}, nil
}
func (t *parityDispatcherTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	return types.ToolResultBlock{ToolUseID: toolUseID, Content: "accepted: " + data.(parityDispatcherOutput).Message}
}

// This test is the registry-side anchor for the fixture harness. Tool fixtures
// exercise real built-ins; this keeps the dispatcher invariants explicit so a
// fixture cannot accidentally bypass strict validation or result mapping.
func TestParityRegistryDispatcherValidatesAndMapsTypedResults(t *testing.T) {
	tool := &parityDispatcherTool{}
	reg := New()
	reg.Register(tool)

	decision, err := reg.CheckToolPermissions(context.Background(), tool.Name(), map[string]any{"message": "hello"}, types.ToolPermissionRequest{})
	if err != nil {
		t.Fatalf("CheckToolPermissions: %v", err)
	}
	if decision.Behavior != types.PermissionBehaviorPassthrough {
		t.Fatalf("permission behavior = %q, want passthrough", decision.Behavior)
	}

	invalid, err := reg.ExecuteToolWithError(context.Background(), tool.Name(), map[string]any{
		"message": "must not execute",
		"extra":   true,
	})
	if err != nil {
		t.Fatalf("invalid dispatch: %v", err)
	}
	if !invalid.IsError || !strings.Contains(invalid.Content, "InputValidationError") {
		t.Fatalf("invalid dispatch result = %#v", invalid)
	}
	if tool.executed {
		t.Fatal("strict validation ran after Execute")
	}

	valid, err := reg.ExecuteToolWithError(context.Background(), tool.Name(), map[string]any{"message": "hello"})
	if err != nil {
		t.Fatalf("valid dispatch: %v", err)
	}
	if valid.IsError || valid.Content != "accepted: hello" {
		t.Fatalf("model-visible result = %#v", valid)
	}
	data, ok := valid.Data.(parityDispatcherOutput)
	if !ok || data.Message != "hello" {
		t.Fatalf("typed result data = %#v", valid.Data)
	}
}
