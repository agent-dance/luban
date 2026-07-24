package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/agent-dance/luban/types"
)

func task13Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func task13Repo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	task13Git(t, repo, "init", "-b", "main")
	task13Git(t, repo, "config", "user.email", "task13@example.invalid")
	task13Git(t, repo, "config", "user.name", "Task 13")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("task 13\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	task13Git(t, repo, "add", "README.md")
	task13Git(t, repo, "commit", "-m", "fixture")
	return repo
}

func task13Tools(repo, sessionID string, manager *WorktreeManager) (*EnterWorktreeTool, *ExitWorktreeTool, *WorktreeRuntime) {
	runtimeContext := NewWorktreeRuntime(repo, nil)
	state := &WorktreeState{}
	session := func() string { return sessionID }
	enter := &EnterWorktreeTool{State: state, Manager: manager, Runtime: runtimeContext, SessionID: session}
	exit := &ExitWorktreeTool{State: state, Manager: manager, Runtime: runtimeContext, SessionID: session}
	return enter, exit, runtimeContext
}

func TestTask13EnterWorktreeStrictInputContract(t *testing.T) {
	tool := &EnterWorktreeTool{State: &WorktreeState{}, Runtime: NewWorktreeRuntime(t.TempDir(), nil)}
	schema := tool.Schema()
	if !schema.RejectsUnknownFields() {
		t.Fatal("EnterWorktree input schema must be strict")
	}
	for _, key := range []string{"name", "path", "base_ref"} {
		if _, ok := schema.Properties[key]; !ok {
			t.Fatalf("input schema missing %q", key)
		}
	}

	result, err := tool.Execute(context.Background(), map[string]any{"name": "feature", "path": "/tmp/other"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content, "mutually exclusive") {
		t.Fatalf("name+path must fail before repository access: %#v", result)
	}

	result, err = tool.Execute(context.Background(), map[string]any{"name": "feature", "unknown": true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content, "InputValidationError") {
		t.Fatalf("unknown input must fail strict validation: %#v", result)
	}
}

func TestTask13WorktreeManagerScopesStateBySession(t *testing.T) {
	manager := NewWorktreeManager()
	a := manager.StateForSession("session-a", nil)
	b := manager.StateForSession("session-b", nil)
	if a == b {
		t.Fatal("different sessions share WorktreeState")
	}
	if got := manager.StateForSession("session-a", nil); got != a {
		t.Fatal("session state lookup is not stable")
	}
	a.mu.Lock()
	a.Active = true
	a.mu.Unlock()
	if manager.CountActive() != 1 {
		t.Fatalf("CountActive() = %d, want 1", manager.CountActive())
	}
}

func TestTask13EnterCreatesAndResumesWithoutProcessChdir(t *testing.T) {
	repo := task13Repo(t)
	manager := NewWorktreeManager()
	enter, exit, runtimeContext := task13Tools(repo, "create-session", manager)
	processCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	result, err := enter.Execute(context.Background(), map[string]any{"name": "topic/nested", "base_ref": "head"})
	if err != nil || result.IsError {
		t.Fatalf("create failed: err=%v result=%#v", err, result)
	}
	data, ok := result.Data.(EnterWorktreeOutput)
	if !ok {
		t.Fatalf("result data = %T, want EnterWorktreeOutput", result.Data)
	}
	if data.WorktreeBranch != "worktree-topic+nested" || data.Message != result.Content {
		t.Fatalf("unexpected TS-shaped output: %#v content=%q", data, result.Content)
	}
	if runtimeContext.CurrentCWD() != data.WorktreePath {
		t.Fatalf("runtime cwd = %q, want %q", runtimeContext.CurrentCWD(), data.WorktreePath)
	}
	if cwd, _ := os.Getwd(); cwd != processCWD {
		t.Fatalf("EnterWorktree changed process cwd: got %q want %q", cwd, processCWD)
	}

	kept, err := exit.Execute(context.Background(), map[string]any{"action": "keep"})
	if err != nil || kept.IsError {
		t.Fatalf("keep failed: err=%v result=%#v", err, kept)
	}
	if runtimeContext.CurrentCWD() != cleanWorktreePath(repo) {
		t.Fatalf("keep restored runtime cwd to %q, want %q", runtimeContext.CurrentCWD(), cleanWorktreePath(repo))
	}

	resume, resumedExit, resumedRuntime := task13Tools(repo, "resume-session", manager)
	resumed, err := resume.Execute(context.Background(), map[string]any{"name": "topic/nested", "base_ref": "head"})
	if err != nil || resumed.IsError {
		t.Fatalf("resume failed: err=%v result=%#v", err, resumed)
	}
	resumedData := resumed.Data.(EnterWorktreeOutput)
	if resumedData.WorktreePath != data.WorktreePath || resumedRuntime.CurrentCWD() != data.WorktreePath {
		t.Fatalf("resume selected wrong worktree: %#v cwd=%q", resumedData, resumedRuntime.CurrentCWD())
	}
	resumedState := manager.StateForSession("resume-session", nil)
	resumedState.mu.Lock()
	createdHere := resumedState.CreatedHere
	resumedState.mu.Unlock()
	if !createdHere {
		t.Fatal("resumed named worktree must remain managed by EnterWorktree")
	}
	removed, err := resumedExit.Execute(context.Background(), map[string]any{"action": "remove"})
	if err != nil || removed.IsError {
		t.Fatalf("resumed named worktree could not be removed: err=%v result=%#v", err, removed)
	}
	if _, err := os.Stat(data.WorktreePath); !os.IsNotExist(err) {
		t.Fatalf("resumed named worktree still exists after remove: %v", err)
	}
}

func TestTask13EnterExistingPathUsesCanonicalMainRoot(t *testing.T) {
	repo := task13Repo(t)
	existing := filepath.Join(t.TempDir(), "existing")
	task13Git(t, repo, "worktree", "add", "-b", "existing-branch", existing, "HEAD")
	t.Cleanup(func() { _, _ = runGit(repo, "worktree", "remove", "--force", existing) })

	manager := NewWorktreeManager()
	enter, exit, runtimeContext := task13Tools(filepath.Join(existing, "subdir"), "path-session", manager)
	if err := os.MkdirAll(runtimeContext.CurrentCWD(), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := enter.Execute(context.Background(), map[string]any{"path": existing})
	if err != nil || result.IsError {
		t.Fatalf("path enter failed: err=%v result=%#v", err, result)
	}
	data := result.Data.(EnterWorktreeOutput)
	if data.WorktreeBranch != "existing-branch" || runtimeContext.CurrentCWD() != cleanWorktreePath(existing) {
		t.Fatalf("unexpected existing worktree result: %#v cwd=%q", data, runtimeContext.CurrentCWD())
	}
	remove, err := exit.Execute(context.Background(), map[string]any{"action": "remove", "discard_changes": true})
	if err != nil || !remove.IsError || !strings.Contains(remove.Content, "entered by path") {
		t.Fatalf("path-entered worktree removal was not refused: err=%v result=%#v", err, remove)
	}
}

func TestTask13PersistedPathResumeResolvesRelativeToSessionCWD(t *testing.T) {
	repo := task13Repo(t)
	existing := filepath.Join(filepath.Dir(repo), "relative-existing")
	task13Git(t, repo, "worktree", "add", "-b", "relative-existing", existing, "HEAD")
	t.Cleanup(func() { _, _ = runGit(repo, "worktree", "remove", "--force", existing) })

	firstManager := NewWorktreeManager()
	first, _, _ := task13Tools(repo, "relative-path-resume", firstManager)
	result, err := first.Execute(context.Background(), map[string]any{"path": "../relative-existing"})
	if err != nil || result.IsError {
		t.Fatalf("relative path entry failed: err=%v result=%#v", err, result)
	}

	secondManager := NewWorktreeManager()
	resumed, _, runtimeContext := task13Tools(repo, "relative-path-resume", secondManager)
	result, err = resumed.Execute(context.Background(), map[string]any{"path": "../relative-existing"})
	if err != nil || result.IsError {
		t.Fatalf("relative persisted resume failed: err=%v result=%#v", err, result)
	}
	if runtimeContext.CurrentCWD() != cleanWorktreePath(existing) {
		t.Fatalf("relative persisted resume cwd = %q, want %q", runtimeContext.CurrentCWD(), existing)
	}
}

func TestTask13HookCreatePersistsAndRemoveRunsHook(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	original := t.TempDir()
	hookPath := filepath.Join(t.TempDir(), "hook-worktree")
	if err := os.MkdirAll(hookPath, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "hooks.jsonl")
	script := filepath.Join(t.TempDir(), "hook.sh")
	body := "#!/bin/sh\ninput=$(cat)\nprintf '%s\\n' \"$input\" >> \"$1\"\ncase \"$input\" in\n  *WorktreeCreate*) printf '{\"path\":\"%s\",\"branch\":\"hook-branch\"}\\n' \"$2\" ;;\n  *) printf '{}\\n' ;;\nesac\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	bridge := NewInMemoryWorktreeHookBridge()
	bridge.Register(WorktreeHook{Name: "WorktreeCreate", Command: script, Args: []string{logPath, hookPath}})
	bridge.Register(WorktreeHook{Name: "WorktreeRemove", Command: script, Args: []string{logPath, hookPath}})
	manager := NewWorktreeManager()
	enter, exit, runtimeContext := task13Tools(original, "hook-session", manager)
	enter.HookBridge = bridge
	exit.HookBridge = bridge

	result, err := enter.Execute(context.Background(), map[string]any{"name": "hook-name", "base_ref": "head"})
	if err != nil || result.IsError {
		t.Fatalf("hook create failed: err=%v result=%#v", err, result)
	}
	if runtimeContext.CurrentCWD() != cleanWorktreePath(hookPath) {
		t.Fatalf("hook did not switch scoped cwd: %q", runtimeContext.CurrentCWD())
	}
	state := manager.StateForSession("hook-session", nil)
	state.mu.Lock()
	hookBased, active, stateFile := state.HookBased, state.Active, state.StateFile
	state.mu.Unlock()
	if !hookBased || !active || stateFile == "" {
		t.Fatalf("hook state not persisted: hook=%v active=%v file=%q", hookBased, active, stateFile)
	}

	removed, err := exit.Execute(context.Background(), map[string]any{"action": "remove", "discard_changes": true})
	if err != nil || removed.IsError {
		t.Fatalf("hook remove failed: err=%v result=%#v", err, removed)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(logData)), "\n")
	if len(lines) != 2 {
		t.Fatalf("hook invocations = %d, want 2: %q", len(lines), logData)
	}
	for _, line := range lines {
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatalf("invalid hook payload %q: %v", line, err)
		}
	}
}

func TestTask13PersistenceFailureRollsBackCreatedWorktree(t *testing.T) {
	repo := task13Repo(t)
	manager := NewWorktreeManager()
	enter, _, runtimeContext := task13Tools(repo, "persist-failure", manager)
	state := manager.StateForSession("persist-failure", enter.State)
	state.writeFile = func(string, []byte, os.FileMode) error { return errors.New("state disk full") }

	result, err := enter.Execute(context.Background(), map[string]any{"name": "rollback", "base_ref": "head"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content, "state disk full") || !strings.Contains(result.Content, "rolled back") {
		t.Fatalf("unexpected persistence failure: %#v", result)
	}
	state.mu.Lock()
	active := state.Active
	state.mu.Unlock()
	if active || runtimeContext.CurrentCWD() != cleanWorktreePath(repo) {
		t.Fatalf("partial session survived rollback: active=%v cwd=%q", active, runtimeContext.CurrentCWD())
	}
	list := task13Git(t, repo, "worktree", "list", "--porcelain")
	if strings.Contains(list, filepath.Join(repo, ".claude", "worktrees", "rollback")) {
		t.Fatalf("partial worktree survived rollback:\n%s", list)
	}
}

func TestTask13SparseCheckoutFailureRollsBackCreatedWorktree(t *testing.T) {
	repo := task13Repo(t)
	t.Setenv("WORKTREE_SPARSE_CHECKOUT", "../escape")
	manager := NewWorktreeManager()
	enter, _, _ := task13Tools(repo, "sparse-failure", manager)
	result, err := enter.Execute(context.Background(), map[string]any{"name": "sparse-rollback", "base_ref": "head"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content, "invalid sparse-checkout path") || !strings.Contains(result.Content, "rolled back") {
		t.Fatalf("unexpected sparse failure: %#v", result)
	}
	state := manager.StateForSession("sparse-failure", nil)
	state.mu.Lock()
	active := state.Active
	state.mu.Unlock()
	if active {
		t.Fatal("sparse-checkout failure activated session state")
	}
	list := task13Git(t, repo, "worktree", "list", "--porcelain")
	if strings.Contains(list, "sparse-rollback") {
		t.Fatalf("partial sparse worktree survived rollback:\n%s", list)
	}
}

func TestTask13ManagerParsesLockedAndPrunableMetadata(t *testing.T) {
	refs := parseWorktreeRefs(strings.Join([]string{
		"worktree /repo/main",
		"HEAD aaaaaa",
		"branch refs/heads/main",
		"locked build in progress",
		"",
		"worktree /repo/stale",
		"HEAD bbbbbb",
		"detached",
		"prunable gitdir file points to non-existent location",
		"",
	}, "\n"))
	if len(refs) != 2 {
		t.Fatalf("parsed refs = %d, want 2: %#v", len(refs), refs)
	}
	if !refs[0].Locked || refs[0].LockedReason != "build in progress" || refs[0].Branch != "main" {
		t.Fatalf("locked metadata lost: %#v", refs[0])
	}
	if !refs[1].Prunable || refs[1].PrunableReason == "" || refs[1].Head != "bbbbbb" {
		t.Fatalf("prunable metadata lost: %#v", refs[1])
	}
}

func TestTask13ScopedCWDFailureRollsBackCreatedWorktree(t *testing.T) {
	repo := task13Repo(t)
	manager := NewWorktreeManager()
	runtimeContext := NewWorktreeRuntime(repo, func(next string) error {
		if cleanWorktreePath(next) != cleanWorktreePath(repo) {
			return errors.New("runtime update rejected")
		}
		return nil
	})
	state := &WorktreeState{}
	enter := &EnterWorktreeTool{
		State: state, Manager: manager, Runtime: runtimeContext,
		SessionID: func() string { return "cwd-failure" },
	}
	result, err := enter.Execute(context.Background(), map[string]any{"name": "cwd-rollback", "base_ref": "head"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content, "runtime update rejected") || !strings.Contains(result.Content, "rolled back") {
		t.Fatalf("unexpected cwd failure: %#v", result)
	}
	state.mu.Lock()
	active := state.Active
	state.mu.Unlock()
	if active || runtimeContext.CurrentCWD() != cleanWorktreePath(repo) {
		t.Fatalf("cwd failure left partial state: active=%v cwd=%q", active, runtimeContext.CurrentCWD())
	}
}

func TestTask13PersistedSessionRestoresScopedCWD(t *testing.T) {
	repo := task13Repo(t)
	firstManager := NewWorktreeManager()
	first, _, _ := task13Tools(repo, "persisted-resume", firstManager)
	created, err := first.Execute(context.Background(), map[string]any{"name": "persisted", "base_ref": "head"})
	if err != nil || created.IsError {
		t.Fatalf("initial create failed: err=%v result=%#v", err, created)
	}
	wantPath := created.Data.(EnterWorktreeOutput).WorktreePath

	secondManager := NewWorktreeManager()
	resumed, _, resumedRuntime := task13Tools(repo, "persisted-resume", secondManager)
	result, err := resumed.Execute(context.Background(), map[string]any{"name": "persisted", "base_ref": "head"})
	if err != nil || result.IsError {
		t.Fatalf("persisted resume failed: err=%v result=%#v", err, result)
	}
	if resumedRuntime.CurrentCWD() != wantPath || result.Data.(EnterWorktreeOutput).WorktreePath != wantPath {
		t.Fatalf("persisted resume selected wrong cwd: cwd=%q result=%#v", resumedRuntime.CurrentCWD(), result.Data)
	}
}

func TestTask13ConcurrentSessionsUseScopedState(t *testing.T) {
	repo := task13Repo(t)
	manager := NewWorktreeManager()
	enterA, exitA, _ := task13Tools(repo, "race-a", manager)
	enterB, exitB, _ := task13Tools(repo, "race-b", manager)
	type outcome struct {
		result types.ToolResult
		err    error
	}
	outcomes := make(chan outcome, 2)
	var wg sync.WaitGroup
	for _, call := range []func() (types.ToolResult, error){
		func() (types.ToolResult, error) {
			return enterA.Execute(context.Background(), map[string]any{"name": "race-a", "base_ref": "head"})
		},
		func() (types.ToolResult, error) {
			return enterB.Execute(context.Background(), map[string]any{"name": "race-b", "base_ref": "head"})
		},
	} {
		wg.Add(1)
		go func(call func() (types.ToolResult, error)) {
			defer wg.Done()
			result, err := call()
			outcomes <- outcome{result: result, err: err}
		}(call)
	}
	wg.Wait()
	close(outcomes)
	for got := range outcomes {
		if got.err != nil || got.result.IsError {
			t.Fatalf("concurrent enter failed: err=%v result=%#v", got.err, got.result)
		}
	}
	if manager.CountActive() != 2 {
		t.Fatalf("active sessions = %d, want 2", manager.CountActive())
	}
	for _, exit := range []*ExitWorktreeTool{exitA, exitB} {
		result, err := exit.Execute(context.Background(), map[string]any{"action": "keep"})
		if err != nil || result.IsError {
			t.Fatalf("concurrent session keep failed: err=%v result=%#v", err, result)
		}
	}
}

func TestTask13ConcurrentSameNameDoesNotDeleteWinner(t *testing.T) {
	repo := task13Repo(t)
	manager := NewWorktreeManager()
	enterA, _, _ := task13Tools(repo, "same-a", manager)
	enterB, _, _ := task13Tools(repo, "same-b", manager)
	results := make(chan types.ToolResult, 2)
	var wg sync.WaitGroup
	for _, enter := range []*EnterWorktreeTool{enterA, enterB} {
		wg.Add(1)
		go func(enter *EnterWorktreeTool) {
			defer wg.Done()
			result, err := enter.Execute(context.Background(), map[string]any{"name": "same-name", "base_ref": "head"})
			if err != nil {
				results <- types.ToolResult{Content: err.Error(), IsError: true}
				return
			}
			results <- result
		}(enter)
	}
	wg.Wait()
	close(results)
	successes, failures := 0, 0
	for result := range results {
		if result.IsError {
			failures++
		} else {
			successes++
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("same-name outcomes: successes=%d failures=%d", successes, failures)
	}
	path, _ := worktreePathAndBranch(repo, "same-name")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("winning worktree was removed: %v", err)
	}
}

func TestTask13FreshBaseRefUsesOriginDefaultAndScopedCache(t *testing.T) {
	repo := task13Repo(t)
	task13Git(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	task13Git(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	resetBaseRefCacheForTests()
	ref, err := ResolveBaseRefAt(repo, "base-session", "fresh")
	if err != nil {
		t.Fatal(err)
	}
	if ref != "origin/main" {
		t.Fatalf("fresh base ref = %q, want origin/main", ref)
	}
	// Removing the symbolic ref proves the second call is served by this
	// repo/session's cache rather than process cwd or another repository.
	task13Git(t, repo, "symbolic-ref", "--delete", "refs/remotes/origin/HEAD")
	cached, err := ResolveBaseRefAt(repo, "base-session", "fresh")
	if err != nil || cached != "origin/main" {
		t.Fatalf("scoped cached base ref = %q err=%v", cached, err)
	}
	otherSession, err := ResolveBaseRefAt(repo, "other-session", "fresh")
	if err != nil || otherSession != "HEAD" {
		t.Fatalf("other session reused stale cache: ref=%q err=%v", otherSession, err)
	}
}

func TestTask13LoadsConfiguredWorktreeCommandHooks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	root := t.TempDir()
	hookPath := filepath.Join(t.TempDir(), "configured-hook-worktree")
	if err := os.MkdirAll(hookPath, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "configured-hook.sh")
	scriptBody := "#!/bin/sh\ninput=$(cat)\ncase \"$input\" in\n  *WorktreeCreate*) printf '{\"worktreePath\":\"%s\",\"worktreeBranch\":\"configured\"}\\n' \"$1\" ;;\n  *) printf '{}\\n' ;;\nesac\n"
	if err := os.WriteFile(script, []byte(scriptBody), 0o755); err != nil {
		t.Fatal(err)
	}
	settingsDir := filepath.Join(root, ".claude")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Race-enabled full-suite runs can spend several seconds waiting for a
	// process slot on a saturated host. This test verifies configuration
	// loading and result mapping, not the two-second timeout boundary.
	settings := fmt.Sprintf(`{"hooks":{"WorktreeCreate":[{"hooks":[{"type":"command","command":%q,"timeout":10}]}],"WorktreeRemove":[{"hooks":[{"type":"command","command":%q,"timeout":10}]}]}}`, script+" "+hookPath, script+" "+hookPath)
	if err := os.WriteFile(filepath.Join(settingsDir, "settings.json"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}
	bridge, err := LoadWorktreeHookBridge(root)
	if err != nil {
		t.Fatal(err)
	}
	result, err := bridge.RunWithResult(context.Background(), "WorktreeCreate", map[string]any{"hook_event_name": "WorktreeCreate", "name": "configured"})
	if err != nil {
		t.Fatal(err)
	}
	if cleanWorktreePath(result.Path) != cleanWorktreePath(hookPath) || result.Branch != "configured" {
		t.Fatalf("configured hook result = %#v", result)
	}
}

func TestTask13WorktreeIncludesStayInsideRepositoryAndWorktree(t *testing.T) {
	repo := task13Repo(t)
	worktree := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, ".env"), []byte("inside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(outside, "secret")
	if err := os.WriteFile(secret, []byte("outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(repo, "outside-link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	applyWorktreeIncludes(repo, worktree, []string{".env", "../secret", "outside-link", secret})
	data, err := os.ReadFile(filepath.Join(worktree, ".env"))
	if err != nil || string(data) != "inside\n" {
		t.Fatalf("valid include not copied: data=%q err=%v", data, err)
	}
	if _, err := os.Lstat(filepath.Join(worktree, "outside-link")); !os.IsNotExist(err) {
		t.Fatalf("repository-external symlink was copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(worktree), "secret")); !os.IsNotExist(err) {
		t.Fatalf("parent traversal wrote outside worktree: %v", err)
	}
}
