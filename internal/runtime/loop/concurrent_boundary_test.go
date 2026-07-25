package loop

import (
	"context"
	"testing"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestExecuteToolsConcurrentlyEmpty(t *testing.T) {
	reg := registry.New()
	results, _, err := executeToolsConcurrently(context.Background(), reg, nil, nil, "", executioncontract.ToolExecutionContext{}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestExecuteToolsConcurrentlyUnknownTool(t *testing.T) {
	reg := registry.New()
	toolUses := []types.ToolUseBlock{
		{Type: types.ContentTypeToolUse, ID: "t1", Name: "NonExistent", Input: map[string]any{}},
	}
	results, _, _ := executeToolsConcurrently(context.Background(), reg, nil, nil, "", executioncontract.ToolExecutionContext{}, toolUses, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].IsError {
		t.Error("expected error for unknown tool")
	}
}
