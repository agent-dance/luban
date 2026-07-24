package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestTodoWriteUsesIndependentFilesForInProcessAgents(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CLAUDE_CODE_AGENT_ID", "")
	t.Setenv("CLAUDE_SESSION_ID", "parent-session")
	store := NewTodoStore(root)
	if err := store.Save([]TodoItem{{Content: "parent", Status: "pending", ActiveForm: "parent"}}); err != nil {
		t.Fatalf("save parent todos: %v", err)
	}

	base := NewTodoWriteTool(store)
	for _, agentID := range []string{"agent-a", "agent-b"} {
		tool := base.withInProcessAgentID(agentID)
		result, err := tool.Execute(context.Background(), map[string]any{
			"todos": []any{map[string]any{
				"content":    agentID,
				"status":     "pending",
				"activeForm": "working " + agentID,
			}},
		})
		if err != nil || result.IsError {
			t.Fatalf("TodoWrite(%s) result=%+v err=%v", agentID, result, err)
		}
		path := filepath.Join(root, ".claude", "todos", agentID+".json")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("agent todo file %q: %v", path, err)
		}
	}

	parent := store.Load()
	if len(parent) != 1 || parent[0].Content != "parent" {
		t.Fatalf("subagent TodoWrite changed parent todos: %#v", parent)
	}
}

func TestTodoWriteUsesAgentCWDForScopedStore(t *testing.T) {
	parentRoot := t.TempDir()
	childRoot := t.TempDir()
	tool := NewTodoWriteTool(NewTodoStore(parentRoot)).withInProcessAgentScope("agent-cwd", childRoot)
	result, err := tool.Execute(context.Background(), map[string]any{
		"todos": []any{map[string]any{
			"content": "child work", "status": "pending", "activeForm": "working in child cwd",
		}},
	})
	if err != nil || result.IsError {
		t.Fatalf("TodoWrite result=%+v err=%v", result, err)
	}
	childPath := filepath.Join(childRoot, ".claude", "todos", "agent-cwd.json")
	if _, err := os.Stat(childPath); err != nil {
		t.Fatalf("child cwd todo file %q: %v", childPath, err)
	}
	parentPath := filepath.Join(parentRoot, ".claude", "todos", "agent-cwd.json")
	if _, err := os.Stat(parentPath); !os.IsNotExist(err) {
		t.Fatalf("agent todo leaked into parent root %q: %v", parentPath, err)
	}
}
