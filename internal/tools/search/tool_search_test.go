package search

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type toolSearchMockTool struct {
	name string
	desc string
}

type countingDescriptionTool struct {
	toolSearchMockTool
	mu    sync.Mutex
	calls int
}

func (t *countingDescriptionTool) Description() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.calls++
	return t.desc
}

func (t *countingDescriptionTool) descriptionCalls() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.calls
}

func TestToolSearchMCPStateProvidersAreInstanceScoped(t *testing.T) {
	first := NewToolSearch(registry.New(), func() []MCPServerVisibilityState {
		return []MCPServerVisibilityState{{Name: "alpha", State: "pending"}}
	})
	second := NewToolSearch(registry.New(), func() []MCPServerVisibilityState {
		return []MCPServerVisibilityState{{Name: "beta", State: "failed"}}
	})

	const workers = 64
	var wait sync.WaitGroup
	errors := make(chan string, workers)
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			tool := first
			want := "pending:alpha"
			if index%2 == 1 {
				tool = second
				want = "failed:beta"
			}
			result, err := tool.Execute(context.Background(), map[string]any{"query": "missing"})
			if err != nil {
				errors <- err.Error()
				return
			}
			if got := result.Metadata["mcp_server_states"]; got != want {
				errors <- got
			}
		}(index)
	}
	wait.Wait()
	close(errors)
	for failure := range errors {
		t.Fatalf("instance-scoped MCP state mismatch: %q", failure)
	}
}

func TestToolSearchReusesUnchangedIndex(t *testing.T) {
	reg := registry.New()
	indexed := &countingDescriptionTool{toolSearchMockTool: toolSearchMockTool{name: "NotebookEdit", desc: "edit notebook cells"}}
	reg.Register(indexed)
	tool := NewToolSearch(reg, nil)
	reg.Register(tool)

	for range 3 {
		if _, err := tool.Execute(context.Background(), map[string]any{"query": "notebook"}); err != nil {
			t.Fatalf("execute: %v", err)
		}
	}
	if got := indexed.descriptionCalls(); got != 1 {
		t.Fatalf("unchanged registry rebuilt tool descriptions %d times", got)
	}
	tool.Invalidate()
	if _, err := tool.Execute(context.Background(), map[string]any{"query": "notebook"}); err != nil {
		t.Fatalf("execute after invalidation: %v", err)
	}
	if got := indexed.descriptionCalls(); got != 2 {
		t.Fatalf("invalidation did not rebuild tool descriptions: %d calls", got)
	}
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
	reg.Register(&toolSearchTool{registry: reg})
	reg.Register(&toolSearchMockTool{name: "TaskCreate", desc: "create a task"})
	reg.Register(&toolSearchMockTool{name: "TaskUpdate", desc: "update tasks"})

	result, err := (&toolSearchTool{registry: reg}).Execute(context.Background(), map[string]any{
		"query": "select:TaskCreate, TaskUpdate, MissingTool",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(result.ContentBlocks) != 2 {
		t.Fatalf("expected 2 tool reference blocks, got %d", len(result.ContentBlocks))
	}
	if !strings.Contains(result.Content, "MissingTool") {
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
	if first.ToolName != "TaskCreate" || second.ToolName != "TaskUpdate" {
		t.Fatalf("unexpected tool references: %#v", result.ContentBlocks)
	}
}

func TestToolSearchKeywordSearchUsesDeferredHints(t *testing.T) {
	reg := registry.New()
	reg.Register(&toolSearchMockTool{name: "Read", desc: "read files"})
	reg.Register(&toolSearchTool{registry: reg})
	reg.Register(&toolSearchMockTool{name: "NotebookEdit", desc: "edit notebook cells"})
	reg.Register(&toolSearchMockTool{name: "WebSearch", desc: "search the internet"})

	result, err := (&toolSearchTool{registry: reg}).Execute(context.Background(), map[string]any{
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
