package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestTaskGetTask27StrictTypedContractAndTSRendering(t *testing.T) {
	store := newIsolatedTaskStore(t)
	created, err := store.createDetailedWithError("Alpha", "first task", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	created.Blocks = []string{"2", "3"}
	created.BlockedBy = []string{"4"}
	if err := store.writeTaskLocked(created); err != nil {
		t.Fatal(err)
	}

	tool := NewTaskGetTool(store)
	if got := tool.Description(); got != "Get a task by ID from the task list" {
		t.Fatalf("description = %q", got)
	}
	if !tool.Schema().RejectsUnknownFields() {
		t.Fatalf("TaskGet input schema is not strict: %+v", tool.Schema())
	}
	for name, input := range map[string]map[string]any{
		"missing":    {},
		"wrong type": {"taskId": 1},
		"unknown":    {"taskId": "1", "extra": true},
	} {
		t.Run(name, func(t *testing.T) {
			result, executeErr := tool.Execute(context.Background(), input)
			if executeErr != nil {
				t.Fatalf("Go error: %v", executeErr)
			}
			if !result.IsError {
				t.Fatalf("input unexpectedly accepted: %+v", result)
			}
		})
	}

	result, err := tool.Execute(context.Background(), map[string]any{"taskId": "1"})
	if err != nil || result.IsError {
		t.Fatalf("Execute: result=%+v err=%v", result, err)
	}
	wantText := "Task #1: Alpha\nStatus: pending\nDescription: first task\nBlocked by: #4\nBlocks: #2, #3"
	if result.Content != wantText {
		t.Fatalf("model text = %q, want %q", result.Content, wantText)
	}
	dataJSON, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	const wantData = `{"task":{"id":"1","subject":"Alpha","description":"first task","status":"pending","blocks":["2","3"],"blockedBy":["4"]}}`
	if string(dataJSON) != wantData {
		t.Fatalf("typed data = %s, want %s", dataJSON, wantData)
	}
	block := types.MapToolResult(tool, result, "toolu_task27")
	if block.Content != wantText || block.ToolUseID != "toolu_task27" || block.Data == nil {
		t.Fatalf("mapped result = %+v", block)
	}
	if block.Metadata["maxResultSizeChars"] != "100000" {
		t.Fatalf("mapped result budget metadata = %+v", block.Metadata)
	}
	reg := registry.New()
	reg.Register(tool)
	registryResult, err := reg.ExecuteToolWithError(context.Background(), "TaskGet", map[string]any{"taskId": "1"})
	if err != nil || registryResult.IsError || registryResult.Data == nil || registryResult.Content != wantText {
		t.Fatalf("registry result = %+v, err=%v", registryResult, err)
	}

	notFound, err := tool.Execute(context.Background(), map[string]any{"taskId": "999"})
	if err != nil || notFound.IsError || notFound.Content != "Task not found" {
		t.Fatalf("not found = %+v, err=%v", notFound, err)
	}
	notFoundJSON, err := json.Marshal(notFound.Data)
	if err != nil {
		t.Fatal(err)
	}
	if string(notFoundJSON) != `{"task":null}` {
		t.Fatalf("not-found data = %s", notFoundJSON)
	}
}

func TestTaskGetTask27TSRenderingDependencyCombinations(t *testing.T) {
	tests := []struct {
		name      string
		blocks    []string
		blockedBy []string
		want      string
	}{
		{name: "none", blocks: []string{}, blockedBy: []string{}, want: "Task #1: row\nStatus: pending\nDescription: details"},
		{name: "blockedBy only", blocks: []string{}, blockedBy: []string{"2", "3"}, want: "Task #1: row\nStatus: pending\nDescription: details\nBlocked by: #2, #3"},
		{name: "blocks only", blocks: []string{"4"}, blockedBy: []string{}, want: "Task #1: row\nStatus: pending\nDescription: details\nBlocks: #4"},
		{name: "both", blocks: []string{"4"}, blockedBy: []string{"2"}, want: "Task #1: row\nStatus: pending\nDescription: details\nBlocked by: #2\nBlocks: #4"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := taskGetModelText(TaskGetResult{Task: &TaskGetResultTask{
				ID: "1", Subject: "row", Description: "details", Status: "pending",
				Blocks: tc.blocks, BlockedBy: tc.blockedBy,
			}})
			if got != tc.want {
				t.Fatalf("render = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTaskGetTask27MetadataPromptAndDiscoveryParity(t *testing.T) {
	tool := NewTaskGetTool(newIsolatedTaskStore(t))
	contract := types.ResolveToolContract(tool)
	if !contract.Strict || !contract.ReadOnly || !contract.ConcurrencySafe || contract.MaxResultSizeChars != 100_000 || contract.OutputSchema == nil {
		t.Fatalf("contract = %+v", contract)
	}
	definition := types.ToDefinition(tool)
	if !definition.Metadata.ReadOnly || !definition.Metadata.ConcurrencySafe || definition.Metadata.MaxResultSizeChars != 100_000 || definition.OutputSchema == nil {
		t.Fatalf("definition = %+v", definition)
	}
	if got := types.ToolAutoClassifierInput(tool, map[string]any{"taskId": "  42  ", "ignored": "secret"}); got != "  42  " {
		t.Fatalf("classifier input = %q", got)
	}
	if got := registry.DiscoveryMetadata(tool); !got.ShouldDefer || got.SearchHint != "retrieve a task by ID" {
		t.Fatalf("discovery metadata = %+v", got)
	}
	readOnly, ok := any(tool).(interface{ IsReadOnly() bool })
	if !ok || !readOnly.IsReadOnly() {
		t.Fatal("TaskGet does not expose read-only metadata")
	}
	ui, ok := any(tool).(interface {
		UserFacingName() string
		RenderToolUseMessage() *string
	})
	if !ok || ui.UserFacingName() != "TaskGet" || ui.RenderToolUseMessage() != nil {
		t.Fatalf("TaskGet UI metadata missing: %#v", tool)
	}
	prompt, ok := any(tool).(interface{ Prompt() string })
	if !ok {
		t.Fatal("TaskGet prompt surface missing")
	}
	for _, phrase := range []string{"When to Use This Tool", "blockedBy", "TaskList", "verify its blockedBy list is empty"} {
		if !strings.Contains(prompt.Prompt(), phrase) {
			t.Errorf("prompt missing %q", phrase)
		}
	}
}

func TestTaskGetTask27SanitizerMatchesTSUTF16Replacement(t *testing.T) {
	tests := map[string]string{
		"abc-DEF_123": "abc-DEF_123",
		"-leading-":   "-leading-",
		"!!!":         "---",
		"":            "",
		"a/b c":       "a-b-c",
		"café":        "caf-",
	}
	for input, want := range tests {
		if got := sanitizeTaskPathComponent(input); got != want {
			t.Errorf("sanitizeTaskPathComponent(%q) = %q, want %q", input, got, want)
		}
	}
	// JS regex without /u replaces both UTF-16 surrogate code units.
	if got := sanitizeTaskPathComponent(string(rune(0x1f600))); got != "--" {
		t.Errorf("sanitizeTaskPathComponent(non-BMP rune) = %q, want %q", got, "--")
	}

	store := newIsolatedTaskStore(t)
	if got := filepath.Base(store.taskPath("!!!")); got != "---.json" {
		t.Fatalf("punctuation-only task path = %q", got)
	}
	if err := os.MkdirAll(store.tasksDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	const punctuationRow = `{"id":"!!!","subject":"punctuation","description":"TS filename","status":"pending","blocks":[],"blockedBy":[]}`
	if err := os.WriteFile(filepath.Join(store.tasksDir(), "---.json"), []byte(punctuationRow), 0o600); err != nil {
		t.Fatal(err)
	}
	lookup, err := NewTaskGetTool(store).Execute(context.Background(), map[string]any{"taskId": "!!!"})
	if err != nil || lookup.IsError || !strings.Contains(lookup.Content, "punctuation") {
		t.Fatalf("TS-sanitized lookup = %+v, err=%v", lookup, err)
	}
	if got := filepath.Base(store.tasksDir()); got == "" || got == "." {
		t.Fatalf("default/runtime task list directory is unusable: %q", store.tasksDir())
	}
}

func TestTaskGetTask27ValidatesRowsAndMigratesAntStatuses(t *testing.T) {
	store := newIsolatedTaskStore(t)
	tool := NewTaskGetTool(store)
	if err := os.MkdirAll(store.tasksDir(), 0o755); err != nil {
		t.Fatal(err)
	}

	valid := map[string]any{
		"id": "1", "subject": "row", "description": "desc", "status": "pending",
		"blocks": []string{}, "blockedBy": []string{},
	}
	writeRow := func(t *testing.T, id string, row any) {
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
	assertNotFound := func(t *testing.T, id string) {
		t.Helper()
		result, err := tool.Execute(context.Background(), map[string]any{"taskId": id})
		if err != nil || result.IsError || result.Content != "Task not found" {
			t.Fatalf("invalid row result=%+v err=%v", result, err)
		}
	}

	invalidRows := map[string]any{
		"malformed":       `{`,
		"missing blocks":  map[string]any{"id": "missing blocks", "subject": "s", "description": "d", "status": "pending", "blockedBy": []string{}},
		"null blockedBy":  map[string]any{"id": "null blockedBy", "subject": "s", "description": "d", "status": "pending", "blocks": []string{}, "blockedBy": nil},
		"bad status":      map[string]any{"id": "bad status", "subject": "s", "description": "d", "status": "deleted", "blocks": []string{}, "blockedBy": []string{}},
		"numeric subject": map[string]any{"id": "numeric subject", "subject": 4, "description": "d", "status": "pending", "blocks": []string{}, "blockedBy": []string{}},
		"null metadata":   map[string]any{"id": "null metadata", "subject": "s", "description": "d", "status": "pending", "blocks": []string{}, "blockedBy": []string{}, "metadata": nil},
	}
	for id, row := range invalidRows {
		t.Run(id, func(t *testing.T) {
			writeRow(t, id, row)
			assertNotFound(t, id)
		})
	}

	t.Setenv("USER_TYPE", "ant")
	migrations := map[string]string{
		"open": "pending", "resolved": "completed", "planning": "in_progress",
		"implementing": "in_progress", "reviewing": "in_progress", "verifying": "in_progress",
	}
	for legacy, want := range migrations {
		t.Run("migrate "+legacy, func(t *testing.T) {
			row := make(map[string]any, len(valid))
			for key, value := range valid {
				row[key] = value
			}
			row["id"] = legacy
			row["status"] = legacy
			writeRow(t, legacy, row)
			result, err := tool.Execute(context.Background(), map[string]any{"taskId": legacy})
			if err != nil || result.IsError || !strings.Contains(result.Content, "Status: "+want) {
				t.Fatalf("migration %q => result=%+v err=%v", legacy, result, err)
			}
		})
	}

	t.Setenv("USER_TYPE", "customer")
	legacy := make(map[string]any, len(valid))
	for key, value := range valid {
		legacy[key] = value
	}
	legacy["id"] = "legacy-customer"
	legacy["status"] = "open"
	writeRow(t, "legacy-customer", legacy)
	assertNotFound(t, "legacy-customer")
}

func TestTaskGetTask27PersistsRequiredDependencyArrays(t *testing.T) {
	store := newIsolatedTaskStore(t)
	created, err := store.createDetailedWithError("shape", "TS readable", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(store.taskPath(created.ID))
	if err != nil {
		t.Fatal(err)
	}
	var row map[string]any
	if err := json.Unmarshal(raw, &row); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(row["blocks"], []any{}) || !reflect.DeepEqual(row["blockedBy"], []any{}) {
		t.Fatalf("persisted dependency arrays = blocks:%#v blockedBy:%#v; row=%s", row["blocks"], row["blockedBy"], raw)
	}
	result, err := NewTaskGetTool(store).Execute(context.Background(), map[string]any{"taskId": created.ID})
	if err != nil || strings.Contains(result.Content, "Blocked by:") || strings.Contains(result.Content, "Blocks:") {
		t.Fatalf("empty dependencies leaked into text: result=%+v err=%v", result, err)
	}
}

func TestTaskGetTask27InProcessTeammateUsesTeamTaskList(t *testing.T) {
	t.Setenv("CLAUDE_HOME", t.TempDir())
	t.Setenv("CLAUDE_CODE_TASK_LIST_ID", "")
	t.Setenv("CLAUDE_CODE_TEAM_NAME", "wrong-process-team")
	t.Setenv("CLAUDE_SESSION_ID", "wrong-session")
	store := NewTaskStore()

	teamStore := NewTaskStore()
	teamScope := NewRuntimeScope(t.TempDir(), true)
	teamScope.SetTeamNameFunc(func() string { return "alignment-team" })
	teamStore.SetScopeResolver(teamScope)
	t.Setenv("CLAUDE_CODE_TASK_LIST_ID", "alignment-team")
	if _, err := teamStore.createDetailedWithError("team row", "from teammate context", "", nil); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CODE_TASK_LIST_ID", "")

	base := NewTaskGetTool(store)
	scoper, ok := any(base).(inProcessAgentScopedTool)
	if !ok {
		t.Fatal("TaskGet does not bind in-process teammate context")
	}
	scoped := scoper.withInProcessAgentID("worker@alignment-team")
	result, err := scoped.Execute(context.Background(), map[string]any{"taskId": "1"})
	if err != nil || result.IsError || !strings.Contains(result.Content, "team row") {
		t.Fatalf("teammate TaskGet resolved wrong list: result=%+v err=%v", result, err)
	}
}

func TestTaskGetTask27TaskListScopePrecedence(t *testing.T) {
	scope := NewRuntimeScope(t.TempDir(), true)
	scope.SetSessionIDFunc(func() string { return "session-list" })
	scope.SetTeamNameFunc(func() string { return "leader-team" })

	t.Setenv("CLAUDE_CODE_TASK_LIST_ID", "explicit-list")
	t.Setenv("CLAUDE_CODE_TEAM_NAME", "process-team")
	if got := scope.TaskListID(); got != "explicit-list" {
		t.Fatalf("explicit precedence = %q", got)
	}
	t.Setenv("CLAUDE_CODE_TASK_LIST_ID", "")
	scope.SetTeammateTeamNameFunc(func() string { return "teammate-team" })
	if got := scope.TaskListID(); got != "teammate-team" {
		t.Fatalf("teammate precedence = %q", got)
	}
	scope.SetTeammateTeamNameFunc(func() string { return "" })
	if got := scope.TaskListID(); got != "process-team" {
		t.Fatalf("process-team precedence = %q", got)
	}
	t.Setenv("CLAUDE_CODE_TEAM_NAME", "")
	if got := scope.TaskListID(); got != "leader-team" {
		t.Fatalf("leader-team precedence = %q", got)
	}
	scope.SetTeamNameFunc(func() string { return "" })
	if got := scope.TaskListID(); got != "session-list" {
		t.Fatalf("session precedence = %q", got)
	}
	scope.SetSessionIDFunc(func() string { return "" })
	t.Setenv("CLAUDE_SESSION_ID", "")
	if got := scope.TaskListID(); got != "default" {
		t.Fatalf("standalone default = %q", got)
	}
}

func TestTaskGetTask27LeaderTeamChangesNotifyTaskWatchers(t *testing.T) {
	store := newIsolatedTaskStore(t)
	mgr := NewTeamManager(nil)
	mgr.SetTaskListChangeNotifier(store.NotifyScopeChanged)
	scope := NewRuntimeScope(t.TempDir(), true)
	scope.SetTeamNameFunc(mgr.CurrentTeamName)
	store.SetScopeResolver(scope)

	var resets int
	store.Subscribe(func(event TaskStoreEvent) error {
		if event.Type == TaskStoreEventReset {
			resets++
		}
		return nil
	})

	mgr.mu.Lock()
	mgr.activeTeamID = "team-1"
	mgr.teams["team-1"] = &TeamInfo{ID: "team-1", Name: "leader-team"}
	mgr.mu.Unlock()
	mgr.notifyTaskListChanged()
	if got := store.taskListID(); got != "test-task-list" {
		t.Fatalf("explicit task-list must still override leader: %q", got)
	}
	t.Setenv("CLAUDE_CODE_TASK_LIST_ID", "")
	if got := store.taskListID(); got != "leader-team" {
		t.Fatalf("leader task-list = %q", got)
	}

	mgr.mu.Lock()
	mgr.activeTeamID = ""
	mgr.mu.Unlock()
	mgr.notifyTaskListChanged()
	if resets != 2 {
		t.Fatalf("scope reset notifications = %d, want 2", resets)
	}
}

func TestTaskGetTask27ProductionTeamLifecycleRetargetsAndNotifies(t *testing.T) {
	mgr := newTestManager(t)
	scope := NewRuntimeScope(mgr.CWD, true)
	scope.SetSessionIDFunc(func() string { return "standalone-session" })
	scope.SetTeamNameFunc(mgr.CurrentTeamName)
	store := NewTaskStore()
	store.SetScopeResolver(scope)
	mgr.SetTaskListChangeNotifier(store.NotifyScopeChanged)

	var resets int
	store.Subscribe(func(event TaskStoreEvent) error {
		if event.Type == TaskStoreEventReset {
			resets++
		}
		return nil
	})
	created, err := NewTeamCreateTool(mgr).Execute(context.Background(), map[string]any{"team_name": "scope-team"})
	if err != nil || created.IsError {
		t.Fatalf("TeamCreate: result=%+v err=%v", created, err)
	}
	if got := store.taskListID(); got != "scope-team" {
		t.Fatalf("leader-created team did not retarget task store: %q", got)
	}
	if _, err := store.createDetailedWithError("team task", "stored under team", "", nil); err != nil {
		t.Fatal(err)
	}
	if got := filepath.Base(store.tasksDir()); got != "scope-team" {
		t.Fatalf("team task directory = %q", got)
	}

	deleted, err := NewTeamDeleteTool(mgr).Execute(context.Background(), map[string]any{})
	if err != nil || deleted.IsError {
		t.Fatalf("TeamDelete: result=%+v err=%v", deleted, err)
	}
	if got := store.taskListID(); got != "standalone-session" {
		t.Fatalf("team deletion did not restore session scope: %q", got)
	}
	if resets != 2 {
		t.Fatalf("production lifecycle reset notifications = %d, want 2", resets)
	}
}
