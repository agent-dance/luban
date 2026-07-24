package tools

import (
	"context"
	"strings"
	"testing"
)

func newIsolatedTodoStore(t *testing.T) *TodoStore {
	t.Helper()
	t.Setenv("CLAUDE_SESSION_ID", "todo-test-session")
	return NewTodoStore(t.TempDir())
}

func TestTodoWriteTool_Name(t *testing.T) {
	tool := NewTodoWriteTool(newIsolatedTodoStore(t))
	if tool.Name() != "TodoWrite" {
		t.Fatalf("expected TodoWrite, got %s", tool.Name())
	}
}

func TestTodoWriteTool_WritesAndClearsTodos(t *testing.T) {
	store := newIsolatedTodoStore(t)
	tool := NewTodoWriteTool(store)

	result, err := tool.Execute(context.Background(), map[string]any{
		"todos": []any{
			map[string]any{"content": "Implement feature A", "activeForm": "Implementing feature A", "status": "in_progress"},
			map[string]any{"content": "Run tests", "activeForm": "Running tests", "status": "pending"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Todos have been modified successfully") {
		t.Fatalf("unexpected content: %s", result.Content)
	}

	stored := store.Load()
	if len(stored) != 2 {
		t.Fatalf("expected 2 stored todos, got %d", len(stored))
	}
	if stored[0].Content != "Implement feature A" {
		t.Fatalf("unexpected stored todo: %+v", stored[0])
	}

	result, err = tool.Execute(context.Background(), map[string]any{
		"todos": []any{
			map[string]any{"content": "Implement feature A", "activeForm": "Implementing feature A", "status": "completed"},
			map[string]any{"content": "Run tests", "activeForm": "Running tests", "status": "completed"},
			map[string]any{"content": "Build project", "activeForm": "Building project", "status": "completed"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if len(store.Load()) != 0 {
		t.Fatalf("expected completed todo list to clear persisted todos")
	}
}

func TestTodoWriteTool_Validation(t *testing.T) {
	tool := NewTodoWriteTool(newIsolatedTodoStore(t))

	cases := []struct {
		name  string
		input map[string]any
		valid bool
	}{
		{
			name:  "empty list",
			input: map[string]any{"todos": []any{}},
			valid: true,
		},
		{
			name:  "missing todos",
			input: map[string]any{},
			valid: false,
		},
		{
			name:  "unknown top-level key",
			input: map[string]any{"todos": []any{}, "extra": true},
			valid: false,
		},
		{
			name: "invalid status",
			input: map[string]any{
				"todos": []any{
					map[string]any{"content": "Do thing", "activeForm": "Doing thing", "status": "bogus"},
				},
			},
			valid: false,
		},
		{
			name: "empty content",
			input: map[string]any{
				"todos": []any{
					map[string]any{"content": "   ", "activeForm": "Doing thing", "status": "pending"},
				},
			},
			valid: false,
		},
		{
			name: "empty activeForm",
			input: map[string]any{
				"todos": []any{
					map[string]any{"content": "Do thing", "activeForm": "   ", "status": "pending"},
				},
			},
			valid: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.valid && result.IsError {
				t.Fatalf("expected success for %s, got error: %s", tc.name, result.Content)
			}
			if !tc.valid && !result.IsError {
				t.Fatalf("expected validation error for %s", tc.name)
			}
		})
	}
}

func TestTodoWriteTool_Schema(t *testing.T) {
	tool := NewTodoWriteTool(newIsolatedTodoStore(t))
	schema := tool.Schema()
	if schema.Type != "object" {
		t.Fatalf("expected object schema")
	}
	if _, ok := schema.Properties["todos"]; !ok {
		t.Fatalf("expected todos property in schema")
	}
}
