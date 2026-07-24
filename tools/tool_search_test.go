package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type toolSearchMockTool struct {
	name string
	desc string
}

func (t *toolSearchMockTool) Name() string        { return t.name }
func (t *toolSearchMockTool) Description() string { return t.desc }
func (t *toolSearchMockTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t *toolSearchMockTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	return types.ToolResult{Content: "ok"}, nil
}

func TestToolSearchSelectReturnsStructuredToolReferences(t *testing.T) {
	reg := registry.New()
	reg.Register(&toolSearchMockTool{name: "Read", desc: "read files"})
	reg.Register(&ToolSearchTool{Registry: reg})
	reg.Register(&toolSearchMockTool{name: "TaskCreate", desc: "create a task"})
	reg.Register(&toolSearchMockTool{name: "TodoWrite", desc: "write todos"})

	result, err := (&ToolSearchTool{Registry: reg}).Execute(context.Background(), map[string]any{
		"query": "select:TaskCreate, TodoWrite, MissingTool",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(result.ContentBlocks) != 2 {
		t.Fatalf("expected 2 tool reference blocks, got %d", len(result.ContentBlocks))
	}
	if !strings.Contains(result.Content, "Missing: MissingTool") {
		t.Fatalf("expected summary to mention missing tool, got %q", result.Content)
	}

	first, ok := result.ContentBlocks[0].(types.ToolReferenceBlock)
	if !ok {
		t.Fatalf("expected first block to be ToolReferenceBlock, got %#v", result.ContentBlocks[0])
	}
	second, ok := result.ContentBlocks[1].(types.ToolReferenceBlock)
	if !ok {
		t.Fatalf("expected second block to be ToolReferenceBlock, got %#v", result.ContentBlocks[1])
	}
	if first.ToolName != "TaskCreate" || second.ToolName != "TodoWrite" {
		t.Fatalf("unexpected tool references: %#v", result.ContentBlocks)
	}
}

func TestToolSearchKeywordSearchUsesDeferredHints(t *testing.T) {
	reg := registry.New()
	reg.Register(&toolSearchMockTool{name: "Read", desc: "read files"})
	reg.Register(&ToolSearchTool{Registry: reg})
	reg.Register(&toolSearchMockTool{name: "NotebookEdit", desc: "edit notebook cells"})
	reg.Register(&toolSearchMockTool{name: "WebSearch", desc: "search the internet"})

	result, err := (&ToolSearchTool{Registry: reg}).Execute(context.Background(), map[string]any{
		"query":       "jupyter notebook",
		"max_results": float64(1),
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(result.ContentBlocks) != 1 {
		t.Fatalf("expected 1 tool reference block, got %d", len(result.ContentBlocks))
	}
	ref, ok := result.ContentBlocks[0].(types.ToolReferenceBlock)
	if !ok {
		t.Fatalf("expected tool reference block, got %#v", result.ContentBlocks[0])
	}
	if ref.ToolName != "NotebookEdit" {
		t.Fatalf("expected NotebookEdit match, got %q", ref.ToolName)
	}
	if !strings.Contains(result.Content, "NotebookEdit") {
		t.Fatalf("expected summary to mention NotebookEdit, got %q", result.Content)
	}
}
