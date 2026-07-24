package types

import (
	"context"
	"testing"
)

type mockTool struct{}

func (m *mockTool) Name() string        { return "mock_tool" }
func (m *mockTool) Description() string  { return "A mock tool for testing" }
func (m *mockTool) Schema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"arg1": map[string]any{"type": "string"},
		},
		Required: []string{"arg1"},
	}
}
func (m *mockTool) Execute(ctx context.Context, input map[string]any) (ToolResult, error) {
	return ToolResult{Content: "mock result"}, nil
}

func TestToDefinition(t *testing.T) {
	tool := &mockTool{}
	def := ToDefinition(tool)

	if def.Name != "mock_tool" {
		t.Errorf("expected name 'mock_tool', got '%s'", def.Name)
	}
	if def.Description != "A mock tool for testing" {
		t.Errorf("expected description mismatch")
	}
	if def.InputSchema.Type != "object" {
		t.Errorf("expected schema type 'object', got '%s'", def.InputSchema.Type)
	}
	if len(def.InputSchema.Required) != 1 || def.InputSchema.Required[0] != "arg1" {
		t.Errorf("expected required ['arg1'], got %v", def.InputSchema.Required)
	}
}

func TestToDefinitions(t *testing.T) {
	tools := []Tool{&mockTool{}, &mockTool{}}
	defs := ToDefinitions(tools)
	if len(defs) != 2 {
		t.Errorf("expected 2 definitions, got %d", len(defs))
	}
}
