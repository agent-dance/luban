package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestTaskCreateTask26StrictInputMatchesTS(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
	}{
		{name: "unknown key", input: map[string]any{"subject": "s", "description": "d", "extra": true}},
		{name: "missing subject", input: map[string]any{"description": "d"}},
		{name: "missing description", input: map[string]any{"subject": "s"}},
		{name: "null metadata", input: map[string]any{"subject": "s", "description": "d", "metadata": nil}},
		{name: "non-object metadata", input: map[string]any{"subject": "s", "description": "d", "metadata": []any{"no"}}},
		{name: "null activeForm", input: map[string]any{"subject": "s", "description": "d", "activeForm": nil}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newIsolatedTaskStore(t)
			result, err := NewTaskCreateTool(store).Execute(context.Background(), tc.input)
			if err != nil {
				t.Fatalf("Execute returned Go error: %v", err)
			}
			if !result.IsError {
				t.Fatalf("TS-invalid input succeeded: %+v", result)
			}
			if got := len(store.list()); got != 0 {
				t.Fatalf("invalid input persisted %d task(s)", got)
			}
		})
	}

	for _, subject := range []string{"", "   \t"} {
		store := newIsolatedTaskStore(t)
		result, err := NewTaskCreateTool(store).Execute(context.Background(), map[string]any{
			"subject": subject, "description": "present", "metadata": map[string]any{"nested": map[string]any{"ok": true}},
		})
		if err != nil || result.IsError {
			t.Fatalf("z.string-compatible subject %q was rejected: result=%+v err=%v", subject, result, err)
		}
	}

	schema := NewTaskCreateTool(newIsolatedTaskStore(t)).Schema()
	if !schema.RejectsUnknownFields() {
		t.Fatalf("TaskCreate schema must be a strict object: %+v", schema)
	}
}

func TestTaskCreateTask26PersistsExactTSTaskShape(t *testing.T) {
	store := newIsolatedTaskStore(t)
	result, err := NewTaskCreateTool(store).Execute(context.Background(), map[string]any{
		"subject": "shape", "description": "verify persisted JSON",
	})
	if err != nil || result.IsError {
		t.Fatalf("Execute: result=%+v err=%v", result, err)
	}

	body, err := os.ReadFile(store.taskPath("1"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	wantKeys := map[string]bool{
		"id": true, "subject": true, "description": true, "status": true, "blocks": true, "blockedBy": true,
	}
	if len(raw) != len(wantKeys) {
		t.Fatalf("persisted fields = %v, want exactly TS task fields %v; body=%s", mapKeys(raw), mapKeys(wantKeys), body)
	}
	for key := range wantKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("persisted task missing %q: %s", key, body)
		}
	}
	for _, key := range []string{"blocks", "blockedBy"} {
		var values []string
		if err := json.Unmarshal(raw[key], &values); err != nil || values == nil || len(values) != 0 {
			t.Errorf("%s must persist as [], got %s (err=%v)", key, raw[key], err)
		}
	}
}

func TestTaskCreateTask26StorageErrorPreservesCause(t *testing.T) {
	root := t.TempDir()
	blockingFile := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewTaskStore()
	store.baseDir = blockingFile
	result, err := NewTaskCreateTool(store).Execute(context.Background(), map[string]any{
		"subject": "cannot persist", "description": "surface filesystem cause",
	})
	if err != nil {
		t.Fatalf("Execute returned Go error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content, blockingFile) {
		t.Fatalf("storage error lost filesystem detail: %+v", result)
	}
}

func TestTaskCreateTask26OutputMetadataPromptConcurrentContract(t *testing.T) {
	store := newIsolatedTaskStore(t)
	tool := NewTaskCreateTool(store)
	result, err := tool.Execute(context.Background(), map[string]any{
		"subject": "structured", "description": "retain typed output",
	})
	if err != nil || result.IsError {
		t.Fatalf("Execute: result=%+v err=%v", result, err)
	}
	const wantText = "Task #1 created successfully: structured"
	if result.Content != wantText {
		t.Fatalf("model-facing content = %q, want %q", result.Content, wantText)
	}
	data, ok := result.Data.(TaskCreateResult)
	if !ok || data.Task.ID != "1" || data.Task.Subject != "structured" {
		t.Fatalf("typed result = %#v", result.Data)
	}

	block := types.MapToolResult(tool, result, "toolu_task26")
	if block.Content != wantText || block.ToolUseID != "toolu_task26" || block.Data == nil {
		t.Fatalf("mapped result = %+v", block)
	}
	wire, err := json.Marshal(block)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wire), wantText) {
		t.Fatalf("serialized model result lost final text: %s", wire)
	}

	reg := registry.New()
	reg.Register(tool)
	registryResult, err := reg.ExecuteToolWithError(context.Background(), "TaskCreate", map[string]any{
		"subject": "registry", "description": "registry mapping",
	})
	if err != nil || registryResult.IsError || registryResult.Data == nil || registryResult.Content != "Task #2 created successfully: registry" {
		t.Fatalf("registry result=%+v err=%v", registryResult, err)
	}

	definition := types.ToDefinition(tool)
	if definition.OutputSchema == nil || !definition.Strict || !definition.Metadata.ConcurrencySafe || definition.Metadata.MaxResultSizeChars != 100_000 {
		t.Fatalf("TaskCreate definition contract = %+v", definition)
	}
	if !tool.IsConcurrentSafe() || tool.UserFacingName() != "TaskCreate" || tool.RenderToolUseMessage() != nil {
		t.Fatalf("TaskCreate UI/scheduling metadata is incomplete")
	}
	if got := types.ToolAutoClassifierInput(tool, map[string]any{"subject": "  keep exact subject  "}); got != "  keep exact subject  " {
		t.Fatalf("auto-classifier input = %q", got)
	}
	if got := registry.DiscoveryMetadata(tool).SearchHint; got != "create a task in the task list" {
		t.Fatalf("search hint = %q", got)
	}
	for _, phrase := range []string{"When to Use", "When NOT to Use", "activeForm", "TaskList", "dependencies", "teammates"} {
		if !strings.Contains(tool.Description(), phrase) {
			t.Errorf("TaskCreate prompt missing %q", phrase)
		}
	}
}

func TestTaskCreateTask26NotifyTaskStoreEventsAreIsolated(t *testing.T) {
	store := newIsolatedTaskStore(t)
	var calls atomic.Int32
	unsubscribe := store.Subscribe(func(event TaskStoreEvent) error {
		calls.Add(1)
		if event.Type == TaskStoreEventCreate && (event.Task == nil || event.Task.ID == "") {
			t.Errorf("create event missing task: %+v", event)
		}
		return errors.New("listener failure must be isolated")
	})
	store.Subscribe(func(TaskStoreEvent) error { panic("listener panic must be isolated") })

	created, createErr := store.createDetailedWithError("events", "create", "", nil)
	if createErr != nil || created == nil {
		t.Fatalf("create failed because listener failed: task=%+v err=%v", created, createErr)
	}
	if _, _, ok := store.updateDetailed(created.ID, map[string]any{"status": "in_progress"}); !ok {
		t.Fatal("update failed")
	}
	if _, ok := store.delete(created.ID); !ok {
		t.Fatal("delete failed")
	}
	if err := store.reset(); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if got := calls.Load(); got != 4 {
		t.Fatalf("listener calls = %d, want create/update/delete/reset", got)
	}
	unsubscribe()
	if _, err := store.createDetailedWithError("after unsubscribe", "", "", nil); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 4 {
		t.Fatalf("unsubscribed listener called again: %d", got)
	}
}

func TestTaskCreateTask26NativeHookPayloadFeedbackRollbackAndView(t *testing.T) {
	store := newIsolatedTaskStore(t)
	scope := NewRuntimeScope(t.TempDir(), true)
	scope.SetTeamNameFunc(func() string { return "alignment-team" })
	scope.SetAgentIDFunc(func() string { return "worker-id" })
	store.SetScopeResolver(scope)
	t.Setenv("CLAUDE_CODE_AGENT_NAME", "worker-name")

	payloadPath := filepath.Join(t.TempDir(), "hook-input.json")
	tool := NewTaskCreateTool(store)
	tool.SetHookRunner(hooks.NewRunner([]hooks.Hook{{
		Type: hooks.HookTaskCreated, Command: fmt.Sprintf("cat > %q", payloadPath), Timeout: 2,
	}}))
	var views atomic.Int32
	tool.SetTaskViewNotifier(func(items []TaskViewItem) {
		views.Add(1)
		if len(items) != 1 || items[0].Subject != "hook payload" {
			t.Errorf("task view snapshot = %+v", items)
		}
	})
	result, err := tool.Execute(context.Background(), map[string]any{
		"subject": "hook payload", "description": "native TaskCreated fields",
	})
	if err != nil || result.IsError {
		t.Fatalf("Execute: result=%+v err=%v", result, err)
	}
	if views.Load() != 1 {
		t.Fatalf("successful create did not expand task view")
	}
	payloadBytes, err := os.ReadFile(payloadPath)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"type": "TaskCreated", "hook_event_name": "TaskCreated", "task_id": "1", "task_subject": "hook payload",
		"task_description": "native TaskCreated fields", "team_name": "alignment-team", "teammate_name": "worker-name",
	}
	for key, value := range want {
		if payload[key] != value {
			t.Errorf("hook payload[%q] = %#v, want %q; payload=%v", key, payload[key], value, payload)
		}
	}
	for _, notificationField := range []string{"message", "title", "tool_name", "tool_input"} {
		if _, ok := payload[notificationField]; ok {
			t.Errorf("TaskCreated payload leaked Notification field %q: %v", notificationField, payload)
		}
	}

	tool.SetHookRunner(hooks.NewRunner([]hooks.Hook{{
		Type: hooks.HookTaskCreated, Command: `printf '%s' '{"block":true,"system_reminder":"policy says no"}'`, Timeout: 2,
	}}))
	blocked, err := tool.Execute(context.Background(), map[string]any{
		"subject": "blocked", "description": "must roll back",
	})
	if err != nil {
		t.Fatalf("blocked Execute returned Go error: %v", err)
	}
	if !blocked.IsError || blocked.Content != "TaskCreated hook feedback:\npolicy says no" {
		t.Fatalf("blocking feedback = %+v", blocked)
	}
	if got := len(store.list()); got != 1 {
		t.Fatalf("blocking hook left task behind: %+v", store.list())
	}
	if views.Load() != 1 {
		t.Fatalf("blocked create expanded task view: %d", views.Load())
	}
}

func mapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}
