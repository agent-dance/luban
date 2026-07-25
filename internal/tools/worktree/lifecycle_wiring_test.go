package worktree

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/internal/gitutil"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"
)

func TestLifecycleProductionEnterExitIsPersisted(t *testing.T) {
	repo := setupLifecycleGitRepo(t)
	state := &WorktreeState{LifecycleFactory: func(root string) LifecyclePublisher {
		lifecycle := runtimestore.NewRuntimeLifecycle(root)
		return LifecyclePublisherFunc(func(ctx context.Context, event LifecycleEvent) error {
			return lifecycle.Publish(ctx, runtimestore.RuntimeLifecycleEvent{
				Type:     runtimestore.RuntimeLifecycleEventType(event.Type),
				EntityID: event.EntityID,
				ToolName: event.ToolName,
				Status:   event.Status,
				Payload: map[string]any{
					"repo_root":    event.RepoRoot,
					"branch":       event.Branch,
					"path":         event.Path,
					"created_here": event.CreatedHere,
				},
			})
		})
	}}
	manager := NewWorktreeManager()
	runtimeContext := NewWorktreeRuntime(repo)
	sessionID := func() string { return "lifecycle-worktree" }
	enter := &EnterWorktreeTool{State: state, Manager: manager, Runtime: runtimeContext, SessionID: sessionID}
	exit := &ExitWorktreeTool{State: state, Manager: manager, Runtime: runtimeContext, SessionID: sessionID}
	result, err := enter.Execute(context.Background(), map[string]any{
		"name": "lifecycle-wiring",
	})
	if err != nil || result.IsError {
		t.Fatalf("EnterWorktree: err=%v result=%s", err, result.Content)
	}
	entered, ok := result.Data.(EnterWorktreeOutput)
	if !ok {
		t.Fatalf("EnterWorktree data = %T", result.Data)
	}
	path := entered.WorktreePath
	branch := entered.WorktreeBranch
	t.Cleanup(func() {
		_, _ = gitutil.Run(repo, "worktree", "remove", path, "--force")
		_, _ = gitutil.Run(repo, "branch", "-D", branch)
	})

	events, err := runtimestore.NewRuntimeLifecycle(repo).Events()
	if err != nil {
		t.Fatal(err)
	}
	if !hasLifecycleType(events, runtimestore.LifecycleWorktreeEnter) {
		t.Fatalf("worktree enter was not journaled: %#v", events)
	}

	result, err = exit.Execute(context.Background(), map[string]any{"action": "keep"})
	if err != nil || result.IsError {
		t.Fatalf("ExitWorktree keep: err=%v result=%s", err, result.Content)
	}
	active, err := runtimestore.NewRuntimeLifecycle(repo).ActiveState()
	if err != nil {
		t.Fatal(err)
	}
	if hasLifecycleType(active, runtimestore.LifecycleWorktreeEnter) {
		t.Fatalf("worktree exit did not close resumed active state: %#v", active)
	}
}

func setupLifecycleGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if out, err := gitutil.Run(repo, "init", "-b", "main"); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	_, _ = gitutil.Run(repo, "config", "user.email", "lifecycle@example.invalid")
	_, _ = gitutil.Run(repo, "config", "user.name", "Lifecycle Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := gitutil.Run(repo, "add", "README.md"); err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}
	if out, err := gitutil.Run(repo, "commit", "-m", "fixture"); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}
	return repo
}

func hasLifecycleType(events []runtimestore.RuntimeLifecycleEvent, eventType runtimestore.RuntimeLifecycleEventType) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}
