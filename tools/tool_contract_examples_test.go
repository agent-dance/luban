package tools

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestToolContractExamplesDeclareStrictInputAndTypedOutput(t *testing.T) {
	taskList := NewTaskListTool(newIsolatedTaskStore(t))
	todoWrite := NewTodoWriteTool(newIsolatedTodoStore(t))

	for _, tool := range []types.Tool{taskList, todoWrite} {
		def := types.ToDefinition(tool)
		if !def.InputSchema.RejectsUnknownFields() {
			t.Errorf("%s input schema is not strict: %#v", tool.Name(), def.InputSchema)
		}
		if def.OutputSchema == nil || def.OutputSchema.Type != "object" {
			t.Errorf("%s output schema missing: %#v", tool.Name(), def.OutputSchema)
		}
		if def.Metadata.MaxResultSizeChars != 100_000 {
			t.Errorf("%s max result size = %d, want 100000", tool.Name(), def.Metadata.MaxResultSizeChars)
		}
	}

	taskDef := types.ToDefinition(taskList)
	if !taskDef.Metadata.ReadOnly || !taskDef.Metadata.ConcurrencySafe {
		t.Fatalf("TaskList metadata = %#v, want read-only and concurrency-safe", taskDef.Metadata)
	}
	todoDef := types.ToDefinition(todoWrite)
	if todoDef.Metadata.ReadOnly || todoDef.Metadata.ConcurrencySafe {
		t.Fatalf("TodoWrite metadata = %#v, want mutating and sequential", todoDef.Metadata)
	}
}

func TestToolContractExamplesRejectUnknownInputBeforeMutation(t *testing.T) {
	taskList := NewTaskListTool(newIsolatedTaskStore(t))
	todoStore := newIsolatedTodoStore(t)
	todoWrite := NewTodoWriteTool(todoStore)
	reg := registry.New()
	reg.Register(taskList)
	reg.Register(todoWrite)

	listResult := reg.ExecuteTool(context.Background(), taskList.Name(), map[string]any{"extra": true})
	if !listResult.IsError {
		t.Fatalf("TaskList accepted unknown input: %#v", listResult)
	}

	writeResult := reg.ExecuteTool(context.Background(), todoWrite.Name(), map[string]any{
		"todos": []any{
			map[string]any{
				"content":    "must not persist",
				"activeForm": "Not persisting",
				"status":     "pending",
			},
		},
		"extra": true,
	})
	if !writeResult.IsError {
		t.Fatalf("TodoWrite accepted unknown input: %#v", writeResult)
	}
	if got := todoStore.Load(); len(got) != 0 {
		t.Fatalf("TodoWrite mutated before strict validation: %#v", got)
	}
}

func TestToolContractExamplesSeparateDataFromModelText(t *testing.T) {
	reg := registry.New()
	taskList := NewTaskListTool(newIsolatedTaskStore(t))
	todoWrite := NewTodoWriteTool(newIsolatedTodoStore(t))
	reg.Register(taskList)
	reg.Register(todoWrite)

	listResult := reg.ExecuteTool(context.Background(), taskList.Name(), map[string]any{})
	if listResult.Content != "No tasks found" {
		t.Fatalf("TaskList model text = %q", listResult.Content)
	}
	if data, ok := listResult.Data.(TaskListResult); !ok || len(data.Tasks) != 0 {
		t.Fatalf("TaskList typed data = %#v", listResult.Data)
	}

	writeResult := reg.ExecuteTool(context.Background(), todoWrite.Name(), map[string]any{"todos": []any{}})
	if writeResult.IsError {
		t.Fatalf("TodoWrite failed: %#v", writeResult)
	}
	if _, ok := writeResult.Data.(TodoWriteResult); !ok {
		t.Fatalf("TodoWrite typed data = %#v", writeResult.Data)
	}
	if writeResult.Content == "" || writeResult.Content[0] == '{' {
		t.Fatalf("TodoWrite model text leaked JSON data: %q", writeResult.Content)
	}
}
