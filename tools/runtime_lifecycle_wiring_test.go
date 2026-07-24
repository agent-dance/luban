package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/coordinator"
	"github.com/agent-dance/luban/hooks"
	svcmcp "github.com/agent-dance/luban/services/mcp"
)

func TestLifecycleBackgroundProductionPathResumesIntoCompaction(t *testing.T) {
	root := t.TempDir()
	manager := NewBackgroundTaskManager(root)
	task := &BackgroundTask{
		ID:          "agent-1",
		Type:        backgroundTaskTypeLocalAgent,
		Status:      "running",
		Description: "inspect state",
		StartedAt:   time.Now().UTC(),
	}
	manager.registerTask(task)

	active, err := NewRuntimeLifecycle(root).ActiveState()
	if err != nil {
		t.Fatal(err)
	}
	if !hasLifecycleType(active, LifecycleTaskCreated) || !hasLifecycleType(active, LifecycleToolStart) {
		t.Fatalf("production registration did not publish task/tool start: %#v", active)
	}

	resumed := NewBackgroundTaskManager(root)
	snapshots := resumed.PostCompactBackgroundTasks()
	if len(snapshots) != 1 || snapshots[0].ID != task.ID || snapshots[0].Status != "running" {
		t.Fatalf("persisted task was not consumed by compaction after resume: %#v", snapshots)
	}

	task.mu.Lock()
	task.Status = "completed"
	code := 0
	task.ExitCode = &code
	finishedAt := time.Now().UTC()
	task.FinishedAt = &finishedAt
	record := task.recordLocked()
	task.mu.Unlock()
	manager.persistRecord(record)
	manager.emitTaskCompletionNotification(context.Background(), task, "completed", 0)

	active, err = NewRuntimeLifecycle(root).ActiveState()
	if err != nil {
		t.Fatal(err)
	}
	if hasLifecycleEntity(active, task.ID) {
		t.Fatalf("terminal production path left task/tool active after resume: %#v", active)
	}
}

func TestHookLifecycleTaskCreateProductionRollback(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CLAUDE_HOME", filepath.Join(root, "home"))
	t.Setenv("CLAUDE_CODE_TASK_LIST_ID", "hook-rollback")
	store := NewTaskStore()
	store.SetScopeResolver(NewRuntimeScope(root, true))
	tool := NewTaskCreateTool(store)
	tool.HookRunner = hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookTaskCreated,
		Command: `printf '%s' '{"block":true,"system_reminder":"task rejected"}'`,
		Timeout: 1,
	}})

	result, err := tool.Execute(context.Background(), map[string]any{
		"subject":     "must roll back",
		"description": "blocking hook refuses this task",
	})
	if err != nil {
		t.Fatalf("TaskCreate Go error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content, "task rejected") {
		t.Fatalf("expected blocking TaskCreated result, got %#v", result)
	}
	if tasks := store.list(); len(tasks) != 0 {
		t.Fatalf("blocking TaskCreated hook left partial task state: %#v", tasks)
	}
}

func TestLifecycleCronProductionFireIsPersisted(t *testing.T) {
	root := t.TempDir()
	store := NewCronStore(root, NewRuntimeScope(root, true))
	schedule, err := ParseCron("* * * * *")
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.create("* * * * *", "check build", false, false, schedule)
	if err != nil {
		t.Fatal(err)
	}
	tick := time.Now().UTC().Truncate(time.Minute)
	store.mu.Lock()
	store.nextFireAt[id] = tick
	store.mu.Unlock()
	fired := store.collectDueJobs(tick)
	if len(fired) != 1 || fired[0].ID != id {
		t.Fatalf("expected cron job to fire, got %#v", fired)
	}
	events, err := NewRuntimeLifecycle(root).Events()
	if err != nil {
		t.Fatal(err)
	}
	if !hasLifecycleEvent(events, LifecycleCronFire, id) {
		t.Fatalf("cron production path did not publish fire: %#v", events)
	}
}

func TestLifecycleWorktreeProductionEnterExitIsPersisted(t *testing.T) {
	repo := setupGitRepo(t)
	originalCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalCWD) })

	enter, exit, state := newWorktreeTools()
	result, err := enter.Execute(context.Background(), map[string]any{
		"name": "lifecycle-wiring",
	})
	if err != nil || result.IsError {
		t.Fatalf("EnterWorktree: err=%v result=%s", err, result.Content)
	}
	state.mu.Lock()
	path := state.Path
	branch := state.Branch
	state.mu.Unlock()
	t.Cleanup(func() {
		_, _ = runGit(repo, "worktree", "remove", path, "--force")
		_, _ = runGit(repo, "branch", "-D", branch)
	})

	events, err := NewRuntimeLifecycle(repo).Events()
	if err != nil {
		t.Fatal(err)
	}
	if !hasLifecycleType(events, LifecycleWorktreeEnter) {
		t.Fatalf("worktree enter was not journaled: %#v", events)
	}

	result, err = exit.Execute(context.Background(), map[string]any{"action": "keep"})
	if err != nil || result.IsError {
		t.Fatalf("ExitWorktree keep: err=%v result=%s", err, result.Content)
	}
	active, err := NewRuntimeLifecycle(repo).ActiveState()
	if err != nil {
		t.Fatal(err)
	}
	if hasLifecycleType(active, LifecycleWorktreeEnter) {
		t.Fatalf("worktree exit did not close resumed active state: %#v", active)
	}
}

func TestLifecycleTeamProductionResumeAndDelete(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	manager := NewTeamManager(coordinator.NewCoordinator())
	manager.CWD = home
	manager.SessionID = func() string { return "session-team-lifecycle" }

	create := NewTeamCreateTool(manager)
	result, err := create.Execute(context.Background(), map[string]any{
		"team_name":   "lifecycle-team",
		"description": "resume this team",
	})
	if err != nil || result.IsError {
		t.Fatalf("TeamCreate: err=%v result=%s", err, result.Content)
	}

	resumed := NewTeamManager(coordinator.NewCoordinator())
	resumed.SessionID = func() string { return "session-team-lifecycle" }
	resumed.SetProjectRoot(home)
	if got := resumed.CurrentTeamName(); got != "lifecycle-team" {
		t.Fatalf("new manager did not resume active team, got %q", got)
	}

	result, err = NewTeamDeleteTool(resumed).Execute(context.Background(), map[string]any{})
	if err != nil || result.IsError {
		t.Fatalf("TeamDelete: err=%v result=%s", err, result.Content)
	}
	active, err := NewRuntimeLifecycle(home).ActiveState()
	if err != nil {
		t.Fatal(err)
	}
	if hasLifecycleType(active, LifecycleTeamCreate) {
		t.Fatalf("team delete did not close resumed team state: %#v", active)
	}
}

func TestLifecycleMCPResourceInvalidationProductionPath(t *testing.T) {
	root := t.TempDir()
	manager := NewMCPManager()
	manager.SetProjectRoot(root)
	manager.HandleMCPNotification(
		context.Background(),
		"docs",
		svcmcp.NotificationResourcesListChanged,
		json.RawMessage(`{"reason":"updated"}`),
	)

	events, err := NewRuntimeLifecycle(root).Events()
	if err != nil {
		t.Fatal(err)
	}
	if !hasLifecycleEvent(events, LifecycleMCPResourcesChanged, "docs") {
		t.Fatalf("MCP resource invalidation was not journaled: %#v", events)
	}
}

func TestResumePlanStateIsConsumedByCompactionProvider(t *testing.T) {
	root := t.TempDir()
	planPath := filepath.Join(root, "plan.md")
	if err := os.WriteFile(planPath, []byte("# Durable plan\n\nRun verification."), 0o644); err != nil {
		t.Fatal(err)
	}
	state := NewPlanState(root)
	state.Enter(planPath)
	state.SetAllowedPrompts([]PlanAllowedPrompt{{Tool: "Bash", Prompt: "go test"}})
	state.SetPrePlanState(map[string]any{"permission_mode": "ask"})

	resumed := NewPlanState(root)
	if !resumed.IsActive() || resumed.PlanFile() != planPath || len(resumed.AllowedPrompts()) != 1 {
		t.Fatalf("plan state did not resume: active=%v file=%q prompts=%#v", resumed.IsActive(), resumed.PlanFile(), resumed.AllowedPrompts())
	}
	if resumed.PrePlanState()["permission_mode"] != "ask" {
		t.Fatalf("pre-plan permission snapshot did not resume: %#v", resumed.PrePlanState())
	}

	provider := &compact.RuntimeAttachmentProvider{PlanState: resumed}
	messages := provider.PostCompactAttachments(context.Background(), compact.PostCompactAttachmentState{})
	combined := ""
	for _, message := range messages {
		combined += message.GetText() + "\n"
	}
	if !strings.Contains(combined, "Post-compaction plan state") || !strings.Contains(combined, "Durable plan") || !strings.Contains(combined, "Plan mode is still active") {
		t.Fatalf("compaction provider did not consume resumed plan state: %s", combined)
	}
}

func hasLifecycleType(events []RuntimeLifecycleEvent, eventType RuntimeLifecycleEventType) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func hasLifecycleEvent(events []RuntimeLifecycleEvent, eventType RuntimeLifecycleEventType, entityID string) bool {
	for _, event := range events {
		if event.Type == eventType && event.EntityID == entityID {
			return true
		}
	}
	return false
}

func hasLifecycleEntity(events []RuntimeLifecycleEvent, entityID string) bool {
	for _, event := range events {
		if event.EntityID == entityID {
			return true
		}
	}
	return false
}
