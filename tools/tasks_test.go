package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func newIsolatedTaskStore(t *testing.T) *TaskStore {
	t.Helper()

	tmpHome := t.TempDir()
	t.Setenv("CLAUDE_HOME", filepath.Join(tmpHome, ".claude"))
	t.Setenv("HOME", tmpHome)
	t.Setenv("CLAUDE_CODE_TASK_LIST_ID", "test-task-list")
	t.Setenv("CLAUDE_CODE_TEAM_NAME", "")
	t.Setenv("CLAUDE_SESSION_ID", "")

	return NewTaskStore()
}

func newBackgroundManager(t *testing.T) *BackgroundTaskManager {
	t.Helper()
	manager := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(manager.Shutdown)
	return manager
}

func helperTaskCommand(t *testing.T, mode string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestBackgroundTaskHelperProcess", "--", mode)
	cmd.Env = append(os.Environ(), "GO_CLAUDE_BACKGROUND_TASK_HELPER=1")
	return cmd
}

func TestBackgroundTaskHelperProcess(t *testing.T) {
	if os.Getenv("GO_CLAUDE_BACKGROUND_TASK_HELPER") != "1" {
		return
	}
	args := os.Args
	mode := ""
	for i, arg := range args {
		if arg == "--" && i+1 < len(args) {
			mode = args[i+1]
			break
		}
	}
	switch mode {
	case "print":
		fmt.Fprint(os.Stdout, "hello from task")
	case "copy-stdin":
		_, _ = io.Copy(os.Stdout, os.Stdin)
	case "sleep":
		time.Sleep(30 * time.Second)
	default:
		fmt.Fprintf(os.Stderr, "unknown helper mode: %s", mode)
		os.Exit(2)
	}
	os.Exit(0)
}

func TestTaskCreateTool(t *testing.T) {
	store := newIsolatedTaskStore(t)
	tool := NewTaskCreateTool(store)

	result, err := tool.Execute(context.Background(), map[string]any{
		"subject":     "Fix the bug",
		"description": "There is a nil-pointer in handler.go",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Task #1 created successfully") {
		t.Fatalf("unexpected content: %s", result.Content)
	}

	// Persistence matters: a fresh store instance should see the same task list.
	store2 := NewTaskStore()
	items := store2.list()
	if len(items) != 1 {
		t.Fatalf("expected 1 persisted task, got %d", len(items))
	}
	if items[0].Subject != "Fix the bug" {
		t.Fatalf("unexpected persisted subject: %s", items[0].Subject)
	}
}

func TestTaskListAndGetTool(t *testing.T) {
	store := newIsolatedTaskStore(t)
	create := NewTaskCreateTool(store)
	list := NewTaskListTool(store)
	get := NewTaskGetTool(store)

	_, _ = create.Execute(context.Background(), map[string]any{
		"subject":     "Alpha",
		"description": "first task",
	})
	_, _ = create.Execute(context.Background(), map[string]any{
		"subject":     "Beta",
		"description": "second task",
	})

	listResult, err := list.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(listResult.Content, "#1 [pending] Alpha") {
		t.Fatalf("expected Alpha in task list, got: %s", listResult.Content)
	}
	if !strings.Contains(listResult.Content, "#2 [pending] Beta") {
		t.Fatalf("expected Beta in task list, got: %s", listResult.Content)
	}

	getResult, err := get.Execute(context.Background(), map[string]any{"taskId": "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if getResult.IsError {
		t.Fatalf("unexpected tool error: %s", getResult.Content)
	}
	if !strings.Contains(getResult.Content, "Task #1: Alpha") {
		t.Fatalf("unexpected TaskGet output: %s", getResult.Content)
	}
	if !strings.Contains(getResult.Content, "Description: first task") {
		t.Fatalf("missing description in TaskGet output: %s", getResult.Content)
	}

	notFound, err := get.Execute(context.Background(), map[string]any{"taskId": "999"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if notFound.IsError {
		t.Fatalf("expected non-error not-found response, got: %s", notFound.Content)
	}
	if notFound.Content != "Task not found" {
		t.Fatalf("unexpected not-found response: %s", notFound.Content)
	}
}

func TestTaskUpdateTool(t *testing.T) {
	store := newIsolatedTaskStore(t)
	create := NewTaskCreateTool(store)
	update := NewTaskUpdateTool(store)
	list := NewTaskListTool(store)

	_, _ = create.Execute(context.Background(), map[string]any{
		"subject":     "Build feature",
		"description": "ship it",
	})
	_, _ = create.Execute(context.Background(), map[string]any{
		"subject":     "Verify feature",
		"description": "prove it works",
	})

	result, err := update.Execute(context.Background(), map[string]any{
		"taskId":       "1",
		"status":       "in_progress",
		"owner":        "worker-a",
		"addBlockedBy": []string{"2"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Updated task #1") {
		t.Fatalf("unexpected content: %s", result.Content)
	}
	if !strings.Contains(result.Content, "blockedBy") {
		t.Fatalf("expected blockedBy in updated fields, got: %s", result.Content)
	}

	listResult, err := list.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(listResult.Content, "#1 [in_progress] Build feature (worker-a) [blocked by #2]") {
		t.Fatalf("unexpected list output: %s", listResult.Content)
	}
}

func TestTaskUpdateTask31StrictAndEmptyValueSemantics(t *testing.T) {
	store := newIsolatedTaskStore(t)
	created := store.createDetailed("Subject", "Description", "Running", map[string]any{
		"keep":   "yes",
		"delete": "gone",
	})
	created.Owner = "worker-a"
	if err := store.writeTaskLocked(created); err != nil {
		t.Fatalf("write task: %v", err)
	}
	update := NewTaskUpdateTool(store)

	for _, input := range []map[string]any{
		{"id": created.ID, "subject": "legacy alias"},
		{"taskId": created.ID, "extra": true},
	} {
		result, err := update.Execute(context.Background(), input)
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if !result.IsError || !strings.Contains(result.Content, "invalid input") {
			t.Fatalf("expected strict input error for %#v, got %#v", input, result)
		}
	}

	emptyStatus, err := update.Execute(context.Background(), map[string]any{
		"taskId": created.ID,
		"status": "",
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !emptyStatus.IsError || !strings.Contains(emptyStatus.Content, "invalid status") {
		t.Fatalf("expected empty status rejection, got %#v", emptyStatus)
	}

	result, err := update.Execute(context.Background(), map[string]any{
		"taskId":      created.ID,
		"subject":     "",
		"description": "",
		"activeForm":  "",
		"owner":       "",
		"metadata": map[string]any{
			"delete": nil,
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Content)
	}
	updated, ok := store.get(created.ID)
	if !ok {
		t.Fatal("updated task missing")
	}
	if updated.Subject != "" || updated.Description != "" || updated.ActiveForm != "" || updated.Owner != "" {
		t.Fatalf("empty values were not persisted: %#v", updated)
	}
	if _, ok := updated.Metadata["delete"]; ok {
		t.Fatalf("metadata null should delete key: %#v", updated.Metadata)
	}
	if updated.Metadata["keep"] != "yes" {
		t.Fatalf("metadata keep key lost: %#v", updated.Metadata)
	}
}

func TestTaskDeleteRemovesDependencies(t *testing.T) {
	store := newIsolatedTaskStore(t)
	create := NewTaskCreateTool(store)
	update := NewTaskUpdateTool(store)
	get := NewTaskGetTool(store)

	_, _ = create.Execute(context.Background(), map[string]any{
		"subject":     "One",
		"description": "first",
	})
	_, _ = create.Execute(context.Background(), map[string]any{
		"subject":     "Two",
		"description": "second",
	})

	_, _ = update.Execute(context.Background(), map[string]any{
		"taskId":    "1",
		"addBlocks": []string{"2"},
	})
	_, _ = update.Execute(context.Background(), map[string]any{
		"taskId": "1",
		"status": "deleted",
	})

	getResult, err := get.Execute(context.Background(), map[string]any{"taskId": "2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(getResult.Content, "Blocked by:") {
		t.Fatalf("expected deleted dependency to be removed, got: %s", getResult.Content)
	}
}

func TestTaskStopAndOutputTools(t *testing.T) {
	background := newBackgroundManager(t)
	stopTool := NewTaskStopTool(background)
	outputTool := NewTaskOutputTool(background)

	cmd := helperTaskCommand(t, "print")
	snap, err := background.StartShellTask(context.Background(), "printf 'hello from task'", "print hello", cmd)
	if err != nil {
		t.Fatalf("failed to start background shell task: %v", err)
	}

	outputResult, err := outputTool.Execute(context.Background(), map[string]any{
		"task_id": snap.ID,
		"block":   true,
		"timeout": 5000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outputResult.IsError {
		t.Fatalf("unexpected tool error: %s", outputResult.Content)
	}
	if !strings.Contains(outputResult.Content, "<retrieval_status>success</retrieval_status>") {
		t.Fatalf("unexpected retrieval status: %s", outputResult.Content)
	}
	if !strings.Contains(outputResult.Content, "hello from task") {
		t.Fatalf("expected task output content, got: %s", outputResult.Content)
	}

	longCmd := helperTaskCommand(t, "sleep")
	running, err := background.StartShellTask(context.Background(), "sleep 30", "sleep", longCmd)
	if err != nil {
		t.Fatalf("failed to start long-running task: %v", err)
	}

	notReady, err := outputTool.Execute(context.Background(), map[string]any{
		"task_id": running.ID,
		"block":   false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(notReady.Content, "<retrieval_status>not_ready</retrieval_status>") {
		t.Fatalf("expected not_ready output, got: %s", notReady.Content)
	}

	stopResult, err := stopTool.Execute(context.Background(), map[string]any{"task_id": running.ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stopResult.IsError {
		t.Fatalf("unexpected tool error: %s", stopResult.Content)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stopResult.Content), &payload); err != nil {
		t.Fatalf("TaskStop output should be JSON, got %q: %v", stopResult.Content, err)
	}
	if payload["task_id"] != running.ID {
		t.Fatalf("unexpected stopped task id: %#v", payload["task_id"])
	}
}

func TestTaskStopTask30AliasAndStrictSchema(t *testing.T) {
	background := newBackgroundManager(t)
	tool := NewTaskStopTool(background)
	aliased, ok := any(tool).(types.AliasedTool)
	if !ok {
		t.Fatal("TaskStopTool must implement types.AliasedTool")
	}
	if aliases := aliased.Aliases(); len(aliases) != 1 || aliases[0] != "KillShell" {
		t.Fatalf("unexpected aliases: %#v", aliases)
	}
	if !tool.Schema().RejectsUnknownFields() {
		t.Fatal("TaskStop schema must reject unknown fields")
	}
	reg := registry.New()
	reg.Register(tool)
	if reg.Get("TaskStop") != tool {
		t.Fatal("registry did not resolve TaskStop")
	}
	if reg.Get("KillShell") != tool {
		t.Fatal("registry did not resolve KillShell alias to TaskStop")
	}
	result, err := tool.Execute(context.Background(), map[string]any{
		"task_id": "missing",
		"extra":   true,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content, "unknown field") {
		t.Fatalf("expected strict unknown-field error, got %#v", result)
	}
}

func TestTaskOutputReadsLargeTail(t *testing.T) {
	background := newBackgroundManager(t)
	outputTool := NewTaskOutputTool(background)

	var builder strings.Builder
	for i := 0; i < 20000; i++ {
		builder.WriteString("line\n")
	}

	cmd := helperTaskCommand(t, "copy-stdin")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("failed to create stdin pipe: %v", err)
	}
	snap, err := background.StartShellTask(context.Background(), "cat", "cat input", cmd)
	if err != nil {
		t.Fatalf("failed to start task: %v", err)
	}
	go func() {
		_, _ = stdin.Write([]byte(builder.String()))
		_ = stdin.Close()
	}()

	result, err := outputTool.Execute(context.Background(), map[string]any{
		"task_id": snap.ID,
		"block":   true,
		"timeout": 5000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Content, "[Truncated. Full output:") {
		t.Fatalf("expected task output truncation header, got: %s", result.Content)
	}
}

func TestTaskUpdateAddsVerificationNudge(t *testing.T) {
	store := newIsolatedTaskStore(t)
	create := NewTaskCreateTool(store)
	update := NewTaskUpdateTool(store)

	for _, subject := range []string{"Implement feature", "Ship change", "Run tests"} {
		_, _ = create.Execute(context.Background(), map[string]any{
			"subject":     subject,
			"description": subject,
		})
	}

	for _, id := range []string{"1", "2"} {
		if _, err := update.Execute(context.Background(), map[string]any{
			"taskId": id,
			"status": "completed",
		}); err != nil {
			t.Fatalf("unexpected error updating task %s: %v", id, err)
		}
	}

	result, err := update.Execute(context.Background(), map[string]any{
		"taskId": "3",
		"status": "completed",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, `subagent_type="verification"`) {
		t.Fatalf("expected verification nudge, got: %s", result.Content)
	}
}

func TestTaskStoreUsesClaudeHome(t *testing.T) {
	store := newIsolatedTaskStore(t)
	_, _ = NewTaskCreateTool(store).Execute(context.Background(), map[string]any{
		"subject":     "Persist me",
		"description": "to disk",
	})

	taskPath := filepath.Join(os.Getenv("CLAUDE_HOME"), "tasks", "test-task-list", "1.json")
	if _, err := os.Stat(taskPath); err != nil {
		t.Fatalf("expected task file at %s: %v", taskPath, err)
	}
}

func TestTaskOutputTimeout(t *testing.T) {
	background := newBackgroundManager(t)
	outputTool := NewTaskOutputTool(background)

	cmd := helperTaskCommand(t, "sleep")
	snap, err := background.StartShellTask(context.Background(), "sleep 2", "sleep", cmd)
	if err != nil {
		t.Fatalf("failed to start task: %v", err)
	}

	result, err := outputTool.Execute(context.Background(), map[string]any{
		"task_id": snap.ID,
		"block":   true,
		"timeout": 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Content, "<retrieval_status>timeout</retrieval_status>") {
		t.Fatalf("expected timeout result, got: %s", result.Content)
	}

	if _, err := NewTaskStopTool(background).Execute(context.Background(), map[string]any{"task_id": snap.ID}); err != nil {
		t.Fatalf("failed to stop timed-out helper task: %v", err)
	}
}

func TestBackgroundTaskIDsAvoidPersistedRuntimeCollisions(t *testing.T) {
	root := t.TempDir()
	firstManager := NewBackgroundTaskManager(root)
	first, err := firstManager.StartShellTask(context.Background(), "print", "first", helperTaskCommand(t, "print"))
	if err != nil {
		t.Fatalf("start first task: %v", err)
	}
	if _, status := firstManager.Wait(first.ID, 2*time.Second); status != "success" {
		t.Fatalf("expected first task to complete, got %s", status)
	}
	firstManager.Shutdown()

	secondManager := NewBackgroundTaskManager(root)
	t.Cleanup(secondManager.Shutdown)
	second, err := secondManager.StartShellTask(context.Background(), "print", "second", helperTaskCommand(t, "print"))
	if err != nil {
		t.Fatalf("start second task: %v", err)
	}
	if second.ID == first.ID {
		t.Fatalf("expected restarted manager to avoid persisted task id %q", first.ID)
	}
}

func TestTaskOutputFallsBackToPersistedAgentResult(t *testing.T) {
	background := newBackgroundManager(t)
	outputTool := NewTaskOutputTool(background)

	snap, err := background.StartAgentTask(context.Background(), "agent prompt", "agent result only", func(context.Context, io.Writer) (string, error) {
		return "result returned without direct writes", nil
	})
	if err != nil {
		t.Fatalf("start agent task: %v", err)
	}
	result, err := outputTool.Execute(context.Background(), map[string]any{
		"task_id": snap.ID,
		"block":   true,
		"timeout": 2000,
	})
	if err != nil {
		t.Fatalf("TaskOutput: %v", err)
	}
	if !strings.Contains(result.Content, "result returned without direct writes") {
		t.Fatalf("expected persisted result in TaskOutput, got: %s", result.Content)
	}
	output, err := readBackgroundTaskOutput(snap.OutputPath, 64*1024)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if !strings.Contains(output.Content, "result returned without direct writes") {
		t.Fatalf("expected result mirrored to output file, got: %s", output.Content)
	}
}

func TestTaskStopCancelsAgentTaskAndPersistsKilledMetadata(t *testing.T) {
	background := newBackgroundManager(t)
	snap, err := background.StartAgentTask(context.Background(), "agent prompt", "blocked agent", func(ctx context.Context, _ io.Writer) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	if err != nil {
		t.Fatalf("start agent task: %v", err)
	}
	stopResult, err := NewTaskStopTool(background).Execute(context.Background(), map[string]any{"task_id": snap.ID})
	if err != nil {
		t.Fatalf("TaskStop: %v", err)
	}
	if stopResult.IsError {
		t.Fatalf("unexpected stop error: %s", stopResult.Content)
	}

	deadline := time.Now().Add(2 * time.Second)
	var record RuntimeTaskRecord
	var ok bool
	for time.Now().Before(deadline) {
		record, ok = background.store.Get(snap.ID)
		if ok && record.Status == "killed" && record.ExitCode != nil && record.FinishedAt != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ok {
		t.Fatalf("expected persisted record for killed task")
	}
	if record.Status != "killed" {
		t.Fatalf("expected killed status, got %+v", record)
	}
	if record.ExitCode == nil || *record.ExitCode != -1 {
		t.Fatalf("expected exit code -1, got %+v", record.ExitCode)
	}
	if record.FinishedAt == nil {
		t.Fatalf("expected killed task to persist finished_at")
	}
}
