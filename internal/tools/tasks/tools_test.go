package tasktools

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/hooks"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"
	taskstore "github.com/agent-dance/luban/internal/store/tasks"
	"github.com/agent-dance/luban/types"
)

type testIdentity struct {
	team  string
	agent string
}

func (i testIdentity) TeamName() string { return i.team }
func (i testIdentity) AgentID() string  { return i.agent }

func newToolStore(t *testing.T, listID string) *taskstore.Store {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	return taskstore.New(func() string { return listID })
}

func TestTaskCRUDToolsUseTypedResultsAndStrictInputs(t *testing.T) {
	store := newToolStore(t, "session-a")
	identity := testIdentity{team: "team-a", agent: "worker-a"}
	create := NewTaskCreateTool(store, identity)
	update := NewTaskUpdateTool(store, identity, func() bool { return true })
	list := NewTaskListTool(store)
	get := NewTaskGetTool(store)

	for name, tool := range map[string]types.Tool{
		"create": create,
		"update": update,
		"list":   list,
		"get":    get,
	} {
		result, err := tool.Execute(context.Background(), map[string]any{"unexpected": true})
		if err != nil || !result.IsError {
			t.Fatalf("%s accepted unknown input: result=%#v err=%v", name, result, err)
		}
	}

	created, err := create.Execute(context.Background(), map[string]any{
		"subject": "implement", "description": "write code", "activeForm": "implementing",
		"metadata": map[string]any{"area": "task"},
	})
	if err != nil || created.IsError {
		t.Fatalf("create=%#v err=%v", created, err)
	}
	createData, ok := created.Data.(TaskCreateResult)
	if !ok || createData.Task.ID == "" || createData.Task.Subject != "implement" {
		t.Fatalf("create data=%#v", created.Data)
	}

	updated, err := update.Execute(context.Background(), map[string]any{
		"taskId": createData.Task.ID, "status": "in_progress", "owner": "worker-a",
	})
	if err != nil || updated.IsError {
		t.Fatalf("update=%#v err=%v", updated, err)
	}
	updateData, ok := updated.Data.(TaskUpdateResult)
	if !ok || !updateData.Success || updateData.StatusChange == nil || updateData.StatusChange.To != "in_progress" {
		t.Fatalf("update data=%#v", updated.Data)
	}

	got, err := get.Execute(context.Background(), map[string]any{"taskId": createData.Task.ID})
	if err != nil || got.IsError {
		t.Fatalf("get=%#v err=%v", got, err)
	}
	getData, ok := got.Data.(TaskGetResult)
	if !ok || getData.Task == nil || getData.Task.Status != "in_progress" {
		t.Fatalf("get data=%#v", got.Data)
	}

	listed, err := list.Execute(context.Background(), map[string]any{})
	if err != nil || listed.IsError {
		t.Fatalf("list=%#v err=%v", listed, err)
	}
	listData, ok := listed.Data.(TaskListResult)
	if !ok || len(listData.Tasks) != 1 || listData.Tasks[0].Owner != "worker-a" {
		t.Fatalf("list data=%#v", listed.Data)
	}
}

func TestTaskViewUsesStoreSubscriptionAsSingleChangeSource(t *testing.T) {
	store := newToolStore(t, "session-a")
	create := NewTaskCreateTool(store, testIdentity{})
	changes := 0
	unsubscribe := create.SubscribeChanges(func() { changes++ })
	defer unsubscribe()
	if _, err := create.Execute(context.Background(), map[string]any{"subject": "visible", "description": "shown"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("internal", "hidden", "", map[string]any{"_internal": true}); err != nil {
		t.Fatal(err)
	}
	if changes != 2 {
		t.Fatalf("changes=%d", changes)
	}
	items := create.TaskViewSnapshot()
	if len(items) != 1 || items[0].Subject != "visible" {
		t.Fatalf("items=%#v", items)
	}
}

func TestTaskGetBoundAgentUsesTeamTaskList(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	teamStore := taskstore.New(func() string { return "team-a" })
	created, err := teamStore.Create("team task", "description", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	base := NewTaskGetTool(taskstore.New(func() string { return "leader-session" }))
	bound, ok := base.BindAgentScope("worker@team-a", "").(*TaskGetTool)
	if !ok {
		t.Fatalf("bound type=%T", base.BindAgentScope("worker@team-a", ""))
	}
	result, err := bound.Execute(context.Background(), map[string]any{"taskId": created.ID})
	if err != nil || result.IsError {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	data := result.Data.(TaskGetResult)
	if data.Task == nil || data.Task.Subject != "team task" {
		t.Fatalf("data=%#v", data)
	}
}

func causalContext(ctx context.Context, name, id string) context.Context {
	return executioncontract.WithToolExecutionContext(ctx, executioncontract.ToolExecutionContext{
		SessionID: "session", TurnID: "turn", WorkUnitID: "work", ActorID: "actor", ActorType: "assistant",
		ToolUse: types.ToolUseBlock{ID: id, Name: name},
	})
}

func TestTaskHooksPreserveCausality(t *testing.T) {
	store := newToolStore(t, "session-a")
	identity := testIdentity{team: "team-a", agent: "worker-a"}
	create := NewTaskCreateTool(store, identity)
	create.SetHookRunner(hooks.NewRunner([]hooks.Hook{{Type: hooks.HookTaskCreated, Command: `printf 'created'`}}))
	var observed []hooks.HookExecution
	ctx := hooks.WithExecutionObserver(context.Background(), func(_ hooks.HookType, execution hooks.HookExecution) {
		observed = append(observed, execution)
	})
	result, err := create.Execute(causalContext(ctx, "TaskCreate", "use-create"), map[string]any{"subject": "ship", "description": "task"})
	if err != nil || result.IsError || len(observed) != 1 {
		t.Fatalf("result=%#v err=%v observed=%#v", result, err, observed)
	}
	input := observed[0].Input
	if input.SessionID != "session" || input.ToolUseID != "use-create" || input.TeamName != "team-a" || input.TeammateName != "worker-a" {
		t.Fatalf("hook input=%#v", input)
	}
}

func TestTaskCreatedBlockingHookRollsBackDurableTask(t *testing.T) {
	store := newToolStore(t, "session-a")
	create := NewTaskCreateTool(store, testIdentity{})
	create.SetHookRunner(hooks.NewRunner([]hooks.Hook{{
		Type: hooks.HookTaskCreated, Command: `printf '%s' '{"block":true,"system_reminder":"task rejected"}'`, Timeout: 1,
	}}))
	result, err := create.Execute(context.Background(), map[string]any{"subject": "blocked", "description": "blocked"})
	if err != nil || !result.IsError || !strings.Contains(result.Content, "task rejected") {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if tasks := store.List(); len(tasks) != 0 {
		t.Fatalf("rollback left tasks=%#v", tasks)
	}
}

func TestTaskCompletedHookPreservesCausality(t *testing.T) {
	store := newToolStore(t, "session-a")
	identity := testIdentity{team: "team-a", agent: "worker-a"}
	created, err := NewTaskCreateTool(store, identity).Execute(context.Background(), map[string]any{"subject": "verify", "description": "task"})
	if err != nil || created.IsError {
		t.Fatalf("create=%#v err=%v", created, err)
	}
	update := NewTaskUpdateTool(store, identity, func() bool { return false })
	update.SetHookRunner(hooks.NewRunner([]hooks.Hook{{Type: hooks.HookTaskCompleted, Command: `printf 'completed'`}}))
	var observed []hooks.HookExecution
	ctx := hooks.WithExecutionObserver(context.Background(), func(_ hooks.HookType, execution hooks.HookExecution) {
		observed = append(observed, execution)
	})
	id := created.Data.(TaskCreateResult).Task.ID
	result, err := update.Execute(causalContext(ctx, "TaskUpdate", "use-update"), map[string]any{"taskId": id, "status": "completed"})
	if err != nil || result.IsError || len(observed) != 1 {
		t.Fatalf("result=%#v err=%v observed=%#v", result, err, observed)
	}
	input := observed[0].Input
	if input.ToolUseID != "use-update" || input.TaskID != id || input.TeamName != "team-a" || input.TeammateName != "worker-a" {
		t.Fatalf("hook input=%#v", input)
	}
}

func TestTaskUpdateEmitsVerificationNudgeAfterCompletedPlan(t *testing.T) {
	store := newToolStore(t, "session-a")
	create := NewTaskCreateTool(store, testIdentity{})
	update := NewTaskUpdateTool(store, testIdentity{}, func() bool { return true })
	var completion TaskUpdateResult
	for _, subject := range []string{"implement", "test", "document"} {
		created, err := create.Execute(context.Background(), map[string]any{"subject": subject, "description": subject})
		if err != nil || created.IsError {
			t.Fatalf("create=%#v err=%v", created, err)
		}
		id := created.Data.(TaskCreateResult).Task.ID
		result, err := update.Execute(context.Background(), map[string]any{"taskId": id, "status": "completed"})
		if err != nil || result.IsError {
			t.Fatalf("update=%#v err=%v", result, err)
		}
		completion = result.Data.(TaskUpdateResult)
	}
	if !completion.VerificationNudgeNeeded {
		t.Fatalf("completion=%#v", completion)
	}
	items := store.List()
	last, err := update.Execute(context.Background(), map[string]any{"taskId": items[len(items)-1].ID, "owner": "done"})
	if err != nil || last.IsError {
		t.Fatalf("final update=%#v err=%v", last, err)
	}
	// The nudge is attached to the completion transition, not later edits.
	if strings.Contains(last.Content, "verification") {
		t.Fatalf("stale nudge=%q", last.Content)
	}
}

type fakeBackground struct {
	snapshot agentcontract.TaskSnapshot
	readSnap agentcontract.TaskSnapshot
	read     BackgroundOutput
	stopErr  error
}

func (f *fakeBackground) Stop(string) (agentcontract.TaskSnapshot, error) {
	return f.snapshot, f.stopErr
}
func (f *fakeBackground) Wait(string, time.Duration) (agentcontract.TaskSnapshot, string) {
	return f.snapshot, "success"
}
func (f *fakeBackground) Snapshot(string) (agentcontract.TaskSnapshot, bool) {
	return f.snapshot, f.snapshot.ID != ""
}
func (f *fakeBackground) ReadOutput(snapshot agentcontract.TaskSnapshot, _ int64) (BackgroundOutput, error) {
	f.readSnap = snapshot
	return f.read, nil
}

func TestBackgroundTaskToolsUseNarrowReceiptPortAndStrictSchemas(t *testing.T) {
	exit := 0
	background := &fakeBackground{
		snapshot: agentcontract.TaskSnapshot{ID: "task-1", Type: "shell", Status: "completed", Command: "go test", ExitCode: &exit, OutputPath: filepath.Join(t.TempDir(), "output")},
		read:     BackgroundOutput{Content: "ok"},
	}
	stop := NewTaskStopTool(background)
	output := NewTaskOutputTool(background)
	for name, tool := range map[string]types.Tool{"stop": stop, "output": output} {
		result, err := tool.Execute(context.Background(), map[string]any{"task_id": "task-1", "unknown": true})
		if err != nil || !result.IsError {
			t.Fatalf("%s accepted unknown input: result=%#v err=%v", name, result, err)
		}
	}
	result, err := output.Execute(context.Background(), map[string]any{"task_id": "task-1", "block": true})
	if err != nil || result.IsError || !strings.Contains(result.Content, "<output>\nok\n</output>") {
		t.Fatalf("output=%#v err=%v", result, err)
	}
	if background.readSnap.ID != background.snapshot.ID {
		t.Fatalf("reader received snapshot=%#v", background.readSnap)
	}
	if _, err := stop.Execute(context.Background(), map[string]any{"task_id": "task-1"}); err != nil {
		t.Fatal(err)
	}
	background.stopErr = errors.New("stop failed")
	failed, err := stop.Execute(context.Background(), map[string]any{"task_id": "task-1"})
	if err != nil || !failed.IsError {
		t.Fatalf("failed stop=%#v err=%v", failed, err)
	}
}
