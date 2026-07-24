package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestTaskListTask28StrictMetadataAndPromptContract(t *testing.T) {
	tool := NewTaskListTool(newIsolatedTaskStore(t))

	if got := tool.Description(); got != "List all tasks in the task list" {
		t.Fatalf("description = %q", got)
	}
	if !tool.Schema().RejectsUnknownFields() {
		t.Fatalf("TaskList input schema is not strict: %+v", tool.Schema())
	}
	result, err := tool.Execute(context.Background(), map[string]any{"extra": true})
	if err != nil {
		t.Fatalf("Execute returned infrastructure error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("TaskList accepted unknown input directly: %+v", result)
	}

	contract := types.ResolveToolContract(tool)
	if !contract.Strict || !contract.ReadOnly || !contract.ConcurrencySafe || contract.MaxResultSizeChars != 100_000 || contract.OutputSchema == nil {
		t.Fatalf("contract = %+v", contract)
	}
	definition := types.ToDefinition(tool)
	if !definition.InputSchema.RejectsUnknownFields() || definition.OutputSchema == nil {
		t.Fatalf("definition schemas = %+v", definition)
	}
	if !definition.Metadata.ReadOnly || !definition.Metadata.ConcurrencySafe || definition.Metadata.MaxResultSizeChars != 100_000 {
		t.Fatalf("definition metadata = %+v", definition.Metadata)
	}
	if got := registry.DiscoveryMetadata(tool); !got.ShouldDefer || got.SearchHint != "list all tasks" {
		t.Fatalf("discovery metadata = %+v", got)
	}
	if got := types.ToolAutoClassifierInput(tool, map[string]any{"ignored": "secret"}); got != "" {
		t.Fatalf("classifier input = %q", got)
	}
	if tool.UserFacingName() != "TaskList" || tool.RenderToolUseMessage() != nil || !tool.IsReadOnly() {
		t.Fatalf("UI/read-only metadata missing on TaskList")
	}
	for _, phrase := range []string{"When to Use This Tool", "blockedBy", "Internal tasks are hidden", "Completed blockers"} {
		if !strings.Contains(tool.Prompt(), phrase) {
			t.Errorf("prompt missing %q", phrase)
		}
	}
}

func TestTaskListTask28TypedOutputFiltersInternalAndResolvedBlockers(t *testing.T) {
	store := newIsolatedTaskStore(t)
	writeTask := func(task *TaskItem) {
		t.Helper()
		if err := store.writeTaskLocked(task); err != nil {
			t.Fatal(err)
		}
	}
	writeTask(&TaskItem{ID: "1", Subject: "Implement", Description: "do it", Status: "in_progress", Owner: "worker-a", Blocks: []string{}, BlockedBy: []string{"2", "3"}, Metadata: map[string]any{}})
	writeTask(&TaskItem{ID: "2", Subject: "Done blocker", Description: "resolved", Status: "completed", Blocks: []string{}, BlockedBy: []string{}, Metadata: map[string]any{}})
	writeTask(&TaskItem{ID: "3", Subject: "Pending blocker", Description: "still blocks", Status: "pending", Blocks: []string{}, BlockedBy: []string{}, Metadata: map[string]any{}})
	writeTask(&TaskItem{ID: "4", Subject: "Internal", Description: "hidden", Status: "pending", Blocks: []string{}, BlockedBy: []string{}, Metadata: map[string]any{"_internal": true}})

	tool := NewTaskListTool(store)
	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil || result.IsError {
		t.Fatalf("Execute: result=%+v err=%v", result, err)
	}
	if strings.Contains(result.Content, "Internal") || strings.Contains(result.Content, "#2]") {
		t.Fatalf("model text leaked internal/resolved blocker: %q", result.Content)
	}
	wantText := "#1 [in_progress] Implement (worker-a) [blocked by #3]\n#2 [completed] Done blocker\n#3 [pending] Pending blocker"
	if result.Content != wantText {
		t.Fatalf("model text = %q, want %q", result.Content, wantText)
	}

	data, ok := result.Data.(TaskListResult)
	if !ok {
		t.Fatalf("typed data = %#v", result.Data)
	}
	gotJSON, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	const wantJSON = `{"tasks":[{"id":"1","subject":"Implement","status":"in_progress","owner":"worker-a","blockedBy":["3"]},{"id":"2","subject":"Done blocker","status":"completed","blockedBy":[]},{"id":"3","subject":"Pending blocker","status":"pending","blockedBy":[]}]}`
	if string(gotJSON) != wantJSON {
		t.Fatalf("typed data = %s, want %s", gotJSON, wantJSON)
	}
	block := types.MapToolResult(tool, result, "toolu_task28")
	if block.Content != wantText || block.ToolUseID != "toolu_task28" || block.Data == nil {
		t.Fatalf("mapped result = %+v", block)
	}
	if block.Metadata["maxResultSizeChars"] != "100000" {
		t.Fatalf("mapped result budget metadata = %+v", block.Metadata)
	}
}

func TestTaskListTask28ValidatesRowsAndMigratesAntStatuses(t *testing.T) {
	store := newIsolatedTaskStore(t)
	if err := os.MkdirAll(store.tasksDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	writeRow := func(id string, row any) {
		t.Helper()
		var data []byte
		var err error
		if text, ok := row.(string); ok {
			data = []byte(text)
		} else {
			data, err = json.Marshal(row)
			if err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(store.taskPath(id), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	writeRow("valid", map[string]any{"id": "valid", "subject": "kept", "description": "ok", "status": "pending", "blocks": []string{}, "blockedBy": []string{}})
	writeRow("bad-status", map[string]any{"id": "bad-status", "subject": "drop", "description": "bad", "status": "deleted", "blocks": []string{}, "blockedBy": []string{}})
	writeRow("missing-blocks", map[string]any{"id": "missing-blocks", "subject": "drop", "description": "bad", "status": "pending", "blockedBy": []string{}})
	writeRow("malformed", `{`)

	t.Setenv("USER_TYPE", "ant")
	writeRow("open", map[string]any{"id": "open", "subject": "legacy pending", "description": "old", "status": "open", "blocks": []string{}, "blockedBy": []string{}})
	writeRow("reviewing", map[string]any{"id": "reviewing", "subject": "legacy active", "description": "old", "status": "reviewing", "blocks": []string{}, "blockedBy": []string{}})

	result, err := NewTaskListTool(store).Execute(context.Background(), map[string]any{})
	if err != nil || result.IsError {
		t.Fatalf("Execute: result=%+v err=%v", result, err)
	}
	for _, forbidden := range []string{"drop", "malformed", "deleted"} {
		if strings.Contains(result.Content, forbidden) {
			t.Fatalf("invalid row leaked into model text: %q", result.Content)
		}
	}
	if !strings.Contains(result.Content, "#open [pending] legacy pending") || !strings.Contains(result.Content, "#reviewing [in_progress] legacy active") {
		t.Fatalf("legacy ant statuses were not migrated in TaskList: %q", result.Content)
	}
}

func TestTaskListTask28UsesTSSanitizedTaskListDirectory(t *testing.T) {
	store := newIsolatedTaskStore(t)
	t.Setenv("CLAUDE_CODE_TASK_LIST_ID", "")
	t.Setenv("CLAUDE_CODE_TEAM_NAME", "")
	scope := NewRuntimeScope(t.TempDir(), true)
	scope.SetSessionIDFunc(func() string { return "!!!" })
	store.SetScopeResolver(scope)

	dir := store.tasksDir()
	if got := filepath.Base(dir); got != "---" {
		t.Fatalf("TS-sanitized task-list directory = %q, want ---", got)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const row = `{"id":"1","subject":"punctuation scope","description":"TS path","status":"pending","blocks":[],"blockedBy":[]}`
	if err := os.WriteFile(filepath.Join(dir, "1.json"), []byte(row), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := NewTaskListTool(store).Execute(context.Background(), map[string]any{})
	if err != nil || result.IsError || !strings.Contains(result.Content, "punctuation scope") {
		t.Fatalf("TaskList did not read TS-sanitized task-list directory: result=%+v err=%v", result, err)
	}
}
