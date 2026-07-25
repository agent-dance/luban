package worktree

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/gitutil"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type task15ExitOutput struct {
	Action            string `json:"action"`
	OriginalCWD       string `json:"originalCwd"`
	WorktreePath      string `json:"worktreePath"`
	WorktreeBranch    string `json:"worktreeBranch,omitempty"`
	TmuxSessionName   string `json:"tmuxSessionName,omitempty"`
	DiscardedFiles    *int   `json:"discardedFiles,omitempty"`
	DiscardedCommits  *int   `json:"discardedCommits,omitempty"`
	CleanupIncomplete bool   `json:"cleanupIncomplete,omitempty"`
	CleanupIssueCount int    `json:"cleanupIssueCount,omitempty"`
	Message           string `json:"message"`
}

type task15GitFixture struct {
	repo         string
	worktree     string
	branch       string
	originalHead string
	state        *WorktreeState
	manager      *WorktreeManager
	runtime      *WorktreeRuntime
	exit         *ExitWorktreeTool
}

func task15Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newTask15GitFixture(t *testing.T) *task15GitFixture {
	t.Helper()
	repo := t.TempDir()
	task15Git(t, repo, "init", "-b", "main")
	task15Git(t, repo, "config", "user.email", "task15@example.invalid")
	task15Git(t, repo, "config", "user.name", "Task 15")
	if err := os.WriteFile(filepath.Join(repo, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	task15Git(t, repo, "add", "base.txt")
	task15Git(t, repo, "commit", "-m", "base")
	originalHead := task15Git(t, repo, "rev-parse", "HEAD")
	worktree := filepath.Join(t.TempDir(), "worktree")
	branch := "worktree-task15"
	task15Git(t, repo, "worktree", "add", "-b", branch, worktree, "HEAD")

	manager := NewWorktreeManager()
	runtimeContext := NewWorktreeRuntime(worktree)
	state := &WorktreeState{
		SessionID:          "task15-session",
		Active:             true,
		Path:               cleanWorktreePath(worktree),
		Name:               "task15",
		Branch:             branch,
		OriginalDir:        cleanWorktreePath(repo),
		OriginalHeadCommit: originalHead,
		RepoRoot:           cleanWorktreePath(repo),
		StateFile:          worktreeStateFilePath(repo, "task15-session"),
		CreatedHere:        true,
	}
	state.mu.Lock()
	if err := state.saveToDiskLocked(); err != nil {
		state.mu.Unlock()
		t.Fatal(err)
	}
	state.mu.Unlock()
	manager.register("task15-session", state)
	if err := manager.claimPath("task15-session", worktree); err != nil {
		t.Fatal(err)
	}
	exit := &ExitWorktreeTool{
		State: state, Manager: manager, Runtime: runtimeContext,
		SessionID: func() string { return "task15-session" },
	}
	t.Cleanup(func() {
		_, _ = gitutil.Run(repo, "worktree", "remove", "--force", worktree)
		_, _ = gitutil.Run(repo, "branch", "-D", branch)
	})
	return &task15GitFixture{
		repo: repo, worktree: worktree, branch: branch, originalHead: originalHead,
		state: state, manager: manager, runtime: runtimeContext, exit: exit,
	}
}

func task15DecodeOutput(t *testing.T, result types.ToolResult) task15ExitOutput {
	t.Helper()
	if result.IsError {
		t.Fatalf("expected successful ExitWorktree result: %q", result.Content)
	}
	var output task15ExitOutput
	if err := json.Unmarshal([]byte(result.Content), &output); err != nil {
		t.Fatalf("ExitWorktree content is not structured JSON: %v\n%s", err, result.Content)
	}
	if output.Action == "" || output.OriginalCWD == "" || output.WorktreePath == "" || output.Message == "" {
		t.Fatalf("ExitWorktree output is missing required fields: %#v", output)
	}
	return output
}

func TestExitWorktree_NoActiveSessionIsValidationError(t *testing.T) {
	state := &WorktreeState{}
	exit := &ExitWorktreeTool{
		State: state, Manager: NewWorktreeManager(), Runtime: NewWorktreeRuntime(t.TempDir()),
		SessionID: func() string { return "inactive" },
	}
	result, err := exit.Execute(context.Background(), map[string]any{"action": "keep"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content, "no active EnterWorktree session") {
		t.Fatalf("inactive ExitWorktree must be a scoped validation failure: %#v", result)
	}
}

func TestExitWorktree_InvalidActionPrecedesInactiveSession(t *testing.T) {
	exit := &ExitWorktreeTool{
		State: &WorktreeState{}, Manager: NewWorktreeManager(), Runtime: NewWorktreeRuntime(t.TempDir()),
		SessionID: func() string { return "inactive-invalid" },
	}
	result, err := exit.Execute(context.Background(), map[string]any{"action": "destroy"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content, "destroy") || strings.Contains(result.Content, "no active") {
		t.Fatalf("invalid action must fail before inactive-session validation: %#v", result)
	}
}

func TestExitWorktree_SchemaAndRegistryRejectUnknownInput(t *testing.T) {
	exit := &ExitWorktreeTool{State: &WorktreeState{}}
	if !exit.Schema().RejectsUnknownFields() {
		t.Fatal("ExitWorktree must expose z.strictObject-equivalent schema")
	}
	discard, ok := exit.Schema().Properties["discard_changes"].(map[string]any)
	if !ok || !strings.Contains(fmt.Sprint(discard["description"]), "uncommitted files") || !strings.Contains(fmt.Sprint(discard["description"]), "commits") {
		t.Fatalf("discard_changes description does not cover both destructive cases: %#v", discard)
	}
	reg := registry.New()
	reg.Register(exit)
	result := reg.ExecuteTool(context.Background(), "ExitWorktree", map[string]any{"action": "keep", "unexpected": true})
	if !result.IsError || !strings.Contains(result.Content, "InputValidationError") || !strings.Contains(result.Content, "unexpected") {
		t.Fatalf("registry accepted non-strict ExitWorktree input: %#v", result)
	}
}

func TestExitWorktree_KeepReturnsStructuredOutputAndPreservesFiles(t *testing.T) {
	fixture := newTask15GitFixture(t)
	result, err := fixture.exit.Execute(context.Background(), map[string]any{"action": "keep"})
	if err != nil {
		t.Fatal(err)
	}
	output := task15DecodeOutput(t, result)
	if output.Action != "keep" || output.OriginalCWD != cleanWorktreePath(fixture.repo) || output.WorktreePath != cleanWorktreePath(fixture.worktree) || output.WorktreeBranch != fixture.branch {
		t.Fatalf("unexpected keep output: %#v", output)
	}
	if _, err := os.Stat(fixture.worktree); err != nil {
		t.Fatalf("keep removed worktree: %v", err)
	}
	if fixture.runtime.CurrentCWD() != cleanWorktreePath(fixture.repo) {
		t.Fatalf("keep did not restore scoped cwd: %q", fixture.runtime.CurrentCWD())
	}
	fixture.state.mu.Lock()
	active := fixture.state.Active
	fixture.state.mu.Unlock()
	if active {
		t.Fatal("keep did not clear current-session state")
	}
}

func TestExitWorktree_RemoveCleanReturnsStructuredOutput(t *testing.T) {
	fixture := newTask15GitFixture(t)
	result, err := fixture.exit.Execute(context.Background(), map[string]any{"action": "remove"})
	if err != nil {
		t.Fatal(err)
	}
	output := task15DecodeOutput(t, result)
	if output.Action != "remove" || output.DiscardedFiles == nil || *output.DiscardedFiles != 0 || output.DiscardedCommits == nil || *output.DiscardedCommits != 0 {
		t.Fatalf("unexpected clean remove output: %#v", output)
	}
	if _, err := os.Stat(fixture.worktree); !os.IsNotExist(err) {
		t.Fatalf("remove left worktree on disk: %v", err)
	}
}

func TestExitWorktreeRemove_UncommittedFilesBlockWithCount(t *testing.T) {
	fixture := newTask15GitFixture(t)
	if err := os.WriteFile(filepath.Join(fixture.worktree, "untracked.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := fixture.exit.Execute(context.Background(), map[string]any{"action": "remove"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content, "1 uncommitted file") {
		t.Fatalf("dirty worktree did not fail closed with a count: %#v", result)
	}
	if _, err := os.Stat(fixture.worktree); err != nil {
		t.Fatalf("blocked remove mutated worktree: %v", err)
	}
}

func TestExitWorktreeRemove_OriginalHeadCommitRangeBlocks(t *testing.T) {
	fixture := newTask15GitFixture(t)
	if err := os.WriteFile(filepath.Join(fixture.worktree, "commit.txt"), []byte("commit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	task15Git(t, fixture.worktree, "add", "commit.txt")
	task15Git(t, fixture.worktree, "commit", "-m", "worktree commit")
	result, err := fixture.exit.Execute(context.Background(), map[string]any{"action": "remove"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content, "1 commit") {
		t.Fatalf("originalHeadCommit..HEAD was not used as the safety baseline: %#v", result)
	}
}

func TestExitWorktreeRemove_MissingOriginalHeadCommitFailsClosed(t *testing.T) {
	fixture := newTask15GitFixture(t)
	fixture.state.mu.Lock()
	fixture.state.OriginalHeadCommit = ""
	fixture.state.mu.Unlock()
	result, err := fixture.exit.Execute(context.Background(), map[string]any{"action": "remove"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content, "Could not verify worktree state") {
		t.Fatalf("missing baseline was treated as clean: %#v", result)
	}
	if _, err := os.Stat(fixture.worktree); err != nil {
		t.Fatalf("unknown-state guard mutated worktree: %v", err)
	}
}

func TestExitWorktreeRemove_DiscardReportsExecutionTimeCounts(t *testing.T) {
	fixture := newTask15GitFixture(t)
	if err := os.WriteFile(filepath.Join(fixture.worktree, "committed.txt"), []byte("commit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	task15Git(t, fixture.worktree, "add", "committed.txt")
	task15Git(t, fixture.worktree, "commit", "-m", "discarded commit")
	if err := os.WriteFile(filepath.Join(fixture.worktree, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := fixture.exit.Execute(context.Background(), map[string]any{"action": "remove", "discard_changes": true})
	if err != nil {
		t.Fatal(err)
	}
	output := task15DecodeOutput(t, result)
	if output.DiscardedFiles == nil || *output.DiscardedFiles != 1 || output.DiscardedCommits == nil || *output.DiscardedCommits != 1 {
		t.Fatalf("discard output did not use execution-time counts: %#v", output)
	}
}

func TestExitWorktreeRemove_SubmoduleUntrackedContentBlocks(t *testing.T) {
	subrepo := t.TempDir()
	task15Git(t, subrepo, "init", "-b", "main")
	task15Git(t, subrepo, "config", "user.email", "task15@example.invalid")
	task15Git(t, subrepo, "config", "user.name", "Task 15")
	if err := os.WriteFile(filepath.Join(subrepo, "tracked.txt"), []byte("submodule\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	task15Git(t, subrepo, "add", "tracked.txt")
	task15Git(t, subrepo, "commit", "-m", "submodule base")

	repo := t.TempDir()
	task15Git(t, repo, "init", "-b", "main")
	task15Git(t, repo, "config", "user.email", "task15@example.invalid")
	task15Git(t, repo, "config", "user.name", "Task 15")
	task15Git(t, repo, "-c", "protocol.file.allow=always", "submodule", "add", subrepo, "vendor/sub")
	task15Git(t, repo, "commit", "-m", "add submodule")
	head := task15Git(t, repo, "rev-parse", "HEAD")
	worktree := filepath.Join(t.TempDir(), "worktree")
	branch := "worktree-task15-submodule"
	task15Git(t, repo, "worktree", "add", "-b", branch, worktree, "HEAD")
	task15Git(t, worktree, "-c", "protocol.file.allow=always", "submodule", "update", "--init")
	t.Cleanup(func() {
		_, _ = gitutil.Run(repo, "worktree", "remove", "--force", worktree)
		_, _ = gitutil.Run(repo, "branch", "-D", branch)
	})
	if err := os.WriteFile(filepath.Join(worktree, "vendor", "sub", "untracked.txt"), []byte("hidden dirty data\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := &WorktreeState{
		SessionID: "submodule-session", Active: true, Path: cleanWorktreePath(worktree), Branch: branch,
		OriginalDir: cleanWorktreePath(repo), OriginalHeadCommit: head, RepoRoot: cleanWorktreePath(repo), CreatedHere: true,
	}
	exit := &ExitWorktreeTool{
		State: state, Manager: NewWorktreeManager(), Runtime: NewWorktreeRuntime(worktree),
		SessionID: func() string { return "submodule-session" },
	}
	result, err := exit.Execute(context.Background(), map[string]any{"action": "remove"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content, "1 uncommitted file") {
		t.Fatalf("untracked submodule content was not treated as destructive: %#v", result)
	}
}

func TestExitWorktree_DoesNotAutoAdoptPersistedSession(t *testing.T) {
	fixture := newTask15GitFixture(t)
	freshState := &WorktreeState{SessionID: "task15-session"}
	freshExit := &ExitWorktreeTool{
		State: freshState, Manager: NewWorktreeManager(), Runtime: NewWorktreeRuntime(fixture.repo),
		SessionID: func() string { return "task15-session" },
	}
	result, err := freshExit.Execute(context.Background(), map[string]any{"action": "keep"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content, "no active EnterWorktree session") {
		t.Fatalf("fresh ExitWorktree auto-adopted persisted state: %#v", result)
	}
	if freshState.Active {
		t.Fatal("fresh ExitWorktree populated current state from disk")
	}
}

func TestWorktreeManager_RejectsStateOwnedByAnotherSession(t *testing.T) {
	manager := NewWorktreeManager()
	stateA := &WorktreeState{SessionID: "session-a", Active: true, Path: "/tmp/a"}
	if got := manager.register("session-a", stateA); got != stateA {
		t.Fatal("explicit state registration failed")
	}
	if got := manager.register("session-b", stateA); got != nil {
		t.Fatalf("cross-session state registration succeeded: %#v", got)
	}
}

func TestWorktreeState_OriginalHeadCommitPersists(t *testing.T) {
	fixture := newTask15GitFixture(t)
	loaded := &WorktreeState{SessionID: "task15-session"}
	loaded.mu.Lock()
	ok := loaded.loadFromDisk(fixture.repo)
	got := loaded.OriginalHeadCommit
	loaded.mu.Unlock()
	if !ok || got != fixture.originalHead {
		t.Fatalf("OriginalHeadCommit did not round-trip: loaded=%v got=%q want=%q", ok, got, fixture.originalHead)
	}
}

func TestExitWorktree_RemoveFailureIsBestEffortAndClearsSession(t *testing.T) {
	original := t.TempDir()
	notAWorktree := t.TempDir()
	state := &WorktreeState{
		SessionID: "cleanup-failure", Active: true, Path: notAWorktree, Branch: "missing-branch",
		OriginalDir: original, RepoRoot: original, CreatedHere: true,
	}
	exit := &ExitWorktreeTool{
		State: state, Manager: NewWorktreeManager(), Runtime: NewWorktreeRuntime(notAWorktree),
		SessionID: func() string { return "cleanup-failure" },
	}
	result, err := exit.Execute(context.Background(), map[string]any{"action": "remove", "discard_changes": true})
	if err != nil {
		t.Fatal(err)
	}
	output := task15DecodeOutput(t, result)
	if output.Action != "remove" || result.Outcome != types.ToolOutcomePartial || !output.CleanupIncomplete || output.CleanupIssueCount == 0 {
		t.Fatalf("unexpected partial-cleanup output: %#v", output)
	}
	state.mu.Lock()
	active := state.Active
	state.mu.Unlock()
	if active {
		t.Fatal("cleanup failure left current session active")
	}
	if exit.Runtime.CurrentCWD() != cleanWorktreePath(original) {
		t.Fatalf("cleanup failure did not preserve restored cwd: %q", exit.Runtime.CurrentCWD())
	}
}

type task15FailingHookBridge struct{}

func (task15FailingHookBridge) Lookup(name string) (WorktreeHook, bool) {
	if name == "WorktreeRemove" {
		return WorktreeHook{Name: name, Command: "failing"}, true
	}
	return WorktreeHook{}, false
}

func (task15FailingHookBridge) RunWithResult(context.Context, string, map[string]any) (WorktreeHookResult, error) {
	return WorktreeHookResult{}, errors.New("remove hook failed")
}

func TestExitWorktreeHook_RemoveFailureIsBestEffort(t *testing.T) {
	original := t.TempDir()
	hookPath := t.TempDir()
	state := &WorktreeState{
		SessionID: "hook-failure", Active: true, Path: hookPath, OriginalDir: original,
		RepoRoot: original, CreatedHere: true, HookBased: true,
	}
	exit := &ExitWorktreeTool{
		State: state, Manager: NewWorktreeManager(), Runtime: NewWorktreeRuntime(hookPath),
		SessionID: func() string { return "hook-failure" }, HookBridge: task15FailingHookBridge{},
	}
	result, err := exit.Execute(context.Background(), map[string]any{"action": "remove", "discard_changes": true})
	if err != nil {
		t.Fatal(err)
	}
	output := task15DecodeOutput(t, result)
	if output.Action != "remove" || result.Outcome != types.ToolOutcomePartial || !output.CleanupIncomplete || output.CleanupIssueCount != 1 {
		t.Fatalf("unexpected hook cleanup output: %#v", output)
	}
	state.mu.Lock()
	active := state.Active
	state.mu.Unlock()
	if active {
		t.Fatal("hook cleanup failure left current session active")
	}
	if _, err := os.Stat(hookPath); err != nil {
		t.Fatalf("failed hook unexpectedly removed path: %v", err)
	}
}

type task15CleanupFaultBridge struct{ cause error }

func (b task15CleanupFaultBridge) Lookup(name string) (WorktreeHook, bool) {
	if name == "WorktreeRemove" {
		return WorktreeHook{Name: name, Command: "fault-injection"}, true
	}
	return WorktreeHook{}, false
}

func (b task15CleanupFaultBridge) RunWithResult(context.Context, string, map[string]any) (WorktreeHookResult, error) {
	return WorktreeHookResult{}, b.cause
}

// TestExitWorktreeCleanupFaultDoesNotLeakDiagnostics verifies that cleanup
// causes remain out of process streams and public tool copy.
func TestExitWorktreeCleanupFaultDoesNotLeakDiagnostics(t *testing.T) {
	rawCause := errors.New("private-cleanup-cause-019f")
	original := t.TempDir()
	hookPath := t.TempDir()
	state := &WorktreeState{
		SessionID: "cleanup-terminal", Active: true, Path: hookPath, OriginalDir: original,
		RepoRoot: original, CreatedHere: true, HookBased: true,
	}
	exit := &ExitWorktreeTool{
		State: state, Manager: NewWorktreeManager(), Runtime: NewWorktreeRuntime(hookPath),
		SessionID: func() string { return "cleanup-terminal" }, HookBridge: task15CleanupFaultBridge{cause: rawCause},
	}
	var result types.ToolResult
	var executeErr error
	stdout, stderr := task15CaptureProcessTerminal(t, func() {
		result, executeErr = exit.Execute(context.Background(), map[string]any{"action": "remove", "discard_changes": true})
	})
	if executeErr != nil {
		t.Fatal(executeErr)
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("cleanup fault bypassed terminal owner: stdout=%q stderr=%q", stdout, stderr)
	}
	output := task15DecodeOutput(t, result)
	if result.Outcome != types.ToolOutcomePartial || !output.CleanupIncomplete || output.CleanupIssueCount != 1 {
		t.Fatalf("cleanup fault result is not typed partial state: result=%#v output=%#v", result, output)
	}
	if strings.Contains(result.Content, rawCause.Error()) || strings.Contains(output.Message, rawCause.Error()) {
		t.Fatalf("private cleanup cause leaked into public tool result: %q", result.Content)
	}
	block := types.MapToolResult(exit, result, "tool-cleanup")
	if block.Outcome != types.ToolOutcomePartial || block.Content != output.Message {
		t.Fatalf("typed partial outcome was lost by model mapping: %#v", block)
	}
}

func task15CaptureProcessTerminal(t *testing.T, fn func()) (string, string) {
	t.Helper()
	stdoutFile, err := os.CreateTemp(t.TempDir(), "stdout-*")
	if err != nil {
		t.Fatal(err)
	}
	stderrFile, err := os.CreateTemp(t.TempDir(), "stderr-*")
	if err != nil {
		t.Fatal(err)
	}
	defer stdoutFile.Close()
	defer stderrFile.Close()

	originalStdout, originalStderr := os.Stdout, os.Stderr
	originalLogWriter, originalLogFlags, originalLogPrefix := log.Writer(), log.Flags(), log.Prefix()
	originalSlog := slog.Default()
	os.Stdout, os.Stderr = stdoutFile, stderrFile
	log.SetOutput(stderrFile)
	slog.SetDefault(slog.New(slog.NewTextHandler(stderrFile, nil)))
	defer func() {
		os.Stdout, os.Stderr = originalStdout, originalStderr
		log.SetOutput(originalLogWriter)
		log.SetFlags(originalLogFlags)
		log.SetPrefix(originalLogPrefix)
		slog.SetDefault(originalSlog)
	}()

	fn()
	if err := stdoutFile.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := stderrFile.Sync(); err != nil {
		t.Fatal(err)
	}
	read := func(file *os.File) string {
		if _, err := file.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		var buffer bytes.Buffer
		if _, err := buffer.ReadFrom(file); err != nil {
			t.Fatal(err)
		}
		return buffer.String()
	}
	return read(stdoutFile), read(stderrFile)
}

func TestExitWorktree_MetadataPermissionAndModelMapping(t *testing.T) {
	exit := &ExitWorktreeTool{}
	keep := exit.ToolMetadata(map[string]any{"action": "keep"})
	remove := exit.ToolMetadata(map[string]any{"action": "remove"})
	if !keep.Write || keep.Destructive || keep.MaxResultSizeChars != 100_000 ||
		!remove.Write || !remove.Destructive || remove.MaxResultSizeChars != 100_000 {
		t.Fatalf("input-sensitive metadata mismatch: keep=%#v remove=%#v", keep, remove)
	}
	output := ExitWorktreeOutput{
		Action: "keep", OriginalCWD: "/repo", WorktreePath: "/repo/wt", Message: "model message",
	}
	block := exit.MapToolResultToToolResultBlock(output, "toolu_task15")
	if block.Content != output.Message || block.ToolUseID != "toolu_task15" {
		t.Fatalf("model-visible mapping mismatch: %#v", block)
	}
	reg := registry.New()
	reg.Register(exit)
	if got := reg.ToolMetadata("ExitWorktree", map[string]any{"action": "remove"}); !got.Write || !got.Destructive {
		t.Fatalf("registry lost destructive metadata: %#v", got)
	}
}

func TestExitWorktreeKeep_PreservesTmux(t *testing.T) {
	fixture := newTask15GitFixture(t)
	fixture.state.mu.Lock()
	fixture.state.TmuxSessionName = "task15-tmux"
	fixture.state.mu.Unlock()

	killCalls := 0
	fixture.exit.killTmuxSessionOverride = func(context.Context, string) error {
		killCalls++
		return nil
	}
	result, err := fixture.exit.Execute(context.Background(), map[string]any{"action": "keep"})
	if err != nil {
		t.Fatal(err)
	}
	output := task15DecodeOutput(t, result)
	if output.TmuxSessionName != "task15-tmux" || !strings.Contains(output.Message, "tmux attach -t task15-tmux") {
		t.Fatalf("keep lost tmux reattach contract: %#v", output)
	}
	if killCalls != 0 {
		t.Fatalf("keep killed tmux %d times", killCalls)
	}
}

func TestExitWorktreeRemove_TmuxFailureIsBestEffort(t *testing.T) {
	fixture := newTask15GitFixture(t)
	fixture.state.mu.Lock()
	fixture.state.TmuxSessionName = "task15-remove-tmux"
	fixture.state.mu.Unlock()

	var killed string
	fixture.exit.killTmuxSessionOverride = func(_ context.Context, name string) error {
		killed = name
		return errors.New("tmux unavailable")
	}
	fixture.exit.sleepOverride = func(context.Context, time.Duration) error { return nil }
	result, err := fixture.exit.Execute(context.Background(), map[string]any{"action": "remove"})
	if err != nil {
		t.Fatal(err)
	}
	output := task15DecodeOutput(t, result)
	if killed != "task15-remove-tmux" {
		t.Fatalf("tmux best-effort cleanup mismatch: killed=%q", killed)
	}
	if output.TmuxSessionName != "" {
		t.Fatalf("remove output must not advertise a killed tmux session: %#v", output)
	}
	if result.Outcome != types.ToolOutcomePartial || !output.CleanupIncomplete || output.CleanupIssueCount != 1 {
		t.Fatalf("tmux cleanup failure was not preserved as a typed partial result: result=%#v output=%#v", result, output)
	}
}

func TestExitWorktreeRemove_BranchDeletionFailureKeepsStableOutput(t *testing.T) {
	fixture := newTask15GitFixture(t)
	fixture.state.mu.Lock()
	fixture.state.Branch = "main"
	fixture.state.mu.Unlock()
	fixture.exit.sleepOverride = func(context.Context, time.Duration) error { return nil }
	result, err := fixture.exit.Execute(context.Background(), map[string]any{"action": "remove"})
	if err != nil {
		t.Fatal(err)
	}
	output := task15DecodeOutput(t, result)
	if output.Action != "remove" || output.WorktreeBranch != "main" || output.DiscardedFiles == nil || output.DiscardedCommits == nil {
		t.Fatalf("branch failure changed structured result shape: %#v", output)
	}
	if result.Outcome != types.ToolOutcomePartial || !output.CleanupIncomplete || output.CleanupIssueCount != 1 {
		t.Fatalf("branch cleanup failure was not preserved as a typed partial result: result=%#v output=%#v", result, output)
	}
}

func TestExitWorktree_RestoreFailureRollsBackExitClaim(t *testing.T) {
	original := t.TempDir()
	worktree := t.TempDir()
	runtimeContext := NewWorktreeRuntime(worktree)
	runtimeContext.SetContextSwitcher(func(_ context.Context, next string) error {
		if cleanWorktreePath(next) == cleanWorktreePath(original) {
			return errors.New("registry restore rejected")
		}
		return nil
	})
	state := &WorktreeState{
		SessionID: "restore-failure", Active: true, Path: worktree, OriginalDir: original,
		RepoRoot: original, CreatedHere: true,
	}
	exit := &ExitWorktreeTool{
		State: state, Manager: NewWorktreeManager(), Runtime: runtimeContext,
		SessionID: func() string { return "restore-failure" },
	}
	result, err := exit.Execute(context.Background(), map[string]any{"action": "keep"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content, "registry restore rejected") {
		t.Fatalf("restore failure was not surfaced: %#v", result)
	}
	state.mu.Lock()
	active, exiting := state.Active, state.exiting
	state.mu.Unlock()
	if !active || exiting {
		t.Fatalf("restore failure corrupted session state: active=%v exiting=%v", active, exiting)
	}
	if runtimeContext.CurrentCWD() != cleanWorktreePath(worktree) {
		t.Fatalf("restore failure changed scoped cwd: %q", runtimeContext.CurrentCWD())
	}
}

func TestExitWorktree_ConcurrentCallsHaveSingleOwner(t *testing.T) {
	fixture := newTask15GitFixture(t)
	start := make(chan struct{})
	results := make(chan types.ToolResult, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := fixture.exit.Execute(context.Background(), map[string]any{"action": "keep"})
			if err != nil {
				results <- types.ToolResult{Content: err.Error(), IsError: true}
				return
			}
			results <- result
		}()
	}
	close(start)
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
		t.Fatalf("concurrent exit outcomes: successes=%d failures=%d", successes, failures)
	}
}

func TestExitWorktree_ExplicitResumeCanExitPersistedSession(t *testing.T) {
	fixture := newTask15GitFixture(t)
	manager := NewWorktreeManager()
	runtimeContext := NewWorktreeRuntime(fixture.repo)
	state := &WorktreeState{}
	sessionID := func() string { return "task15-session" }
	enter := &EnterWorktreeTool{State: state, Manager: manager, Runtime: runtimeContext, SessionID: sessionID}
	exit := &ExitWorktreeTool{State: state, Manager: manager, Runtime: runtimeContext, SessionID: sessionID}
	resumed, err := enter.Execute(context.Background(), map[string]any{"name": "task15", "base_ref": "head"})
	if err != nil || resumed.IsError {
		t.Fatalf("explicit resume failed: err=%v result=%#v", err, resumed)
	}
	if runtimeContext.CurrentCWD() != cleanWorktreePath(fixture.worktree) {
		t.Fatalf("explicit resume cwd = %q, want %q", runtimeContext.CurrentCWD(), fixture.worktree)
	}
	kept, err := exit.Execute(context.Background(), map[string]any{"action": "keep"})
	if err != nil || kept.IsError {
		t.Fatalf("explicitly resumed session could not exit: err=%v result=%#v", err, kept)
	}
	if _, err := os.Stat(worktreeStateFilePath(fixture.repo, "task15-session")); !os.IsNotExist(err) {
		t.Fatalf("exit did not clear persisted session state: %v", err)
	}
}

func TestAlignmentWorktreeSessionState_StalePersistedPathIsCleared(t *testing.T) {
	repo := t.TempDir()
	sessionID := "stale-task15"
	stateFile := worktreeStateFilePath(repo, sessionID)
	if err := os.MkdirAll(filepath.Dir(stateFile), 0o755); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"session_id":%q,"active":true,"path":%q,"original_dir":%q,"repo_root":%q}`, sessionID, filepath.Join(repo, "gone"), repo, repo)
	if err := os.WriteFile(stateFile, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	state := &WorktreeState{SessionID: sessionID}
	state.mu.Lock()
	loaded := state.loadFromDisk(repo)
	active := state.Active
	state.mu.Unlock()
	if loaded || active {
		t.Fatalf("stale persisted session was restored: loaded=%v active=%v", loaded, active)
	}
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Fatalf("stale persisted state was not cleared: %v", err)
	}
}

func TestExitWorktreeHook_KeepRestoresAndPreservesPath(t *testing.T) {
	original := t.TempDir()
	hookPath := t.TempDir()
	state := &WorktreeState{
		SessionID: "hook-keep", Active: true, Path: hookPath, OriginalDir: original,
		RepoRoot: original, CreatedHere: true, HookBased: true,
	}
	runtimeContext := NewWorktreeRuntime(hookPath)
	exit := &ExitWorktreeTool{
		State: state, Manager: NewWorktreeManager(), Runtime: runtimeContext,
		SessionID: func() string { return "hook-keep" },
	}
	result, err := exit.Execute(context.Background(), map[string]any{"action": "keep"})
	if err != nil {
		t.Fatal(err)
	}
	output := task15DecodeOutput(t, result)
	if output.Action != "keep" || runtimeContext.CurrentCWD() != cleanWorktreePath(original) {
		t.Fatalf("hook keep did not restore session: output=%#v cwd=%q", output, runtimeContext.CurrentCWD())
	}
	if _, err := os.Stat(hookPath); err != nil {
		t.Fatalf("hook keep removed path: %v", err)
	}
}

func TestExitWorktreeHook_NoRemoveHookStillClearsSession(t *testing.T) {
	original := t.TempDir()
	hookPath := t.TempDir()
	state := &WorktreeState{
		SessionID: "hook-missing", Active: true, Path: hookPath, OriginalDir: original,
		RepoRoot: original, CreatedHere: true, HookBased: true,
	}
	exit := &ExitWorktreeTool{
		State: state, Manager: NewWorktreeManager(), Runtime: NewWorktreeRuntime(hookPath),
		SessionID: func() string { return "hook-missing" }, HookBridge: NewInMemoryWorktreeHookBridge(),
	}
	result, err := exit.Execute(context.Background(), map[string]any{"action": "remove", "discard_changes": true})
	if err != nil {
		t.Fatal(err)
	}
	output := task15DecodeOutput(t, result)
	if output.Action != "remove" || result.Outcome != types.ToolOutcomePartial || !output.CleanupIncomplete || output.CleanupIssueCount != 1 {
		t.Fatalf("missing hook cleanup mismatch: output=%#v", output)
	}
	state.mu.Lock()
	active := state.Active
	state.mu.Unlock()
	if active {
		t.Fatal("missing WorktreeRemove hook left session active")
	}
	if _, err := os.Stat(hookPath); err != nil {
		t.Fatalf("missing hook unexpectedly removed path: %v", err)
	}
}

func TestExitWorktreeRemove_EmptyTmuxNameDoesNotInvokeKiller(t *testing.T) {
	fixture := newTask15GitFixture(t)
	killCalls := 0
	fixture.exit.killTmuxSessionOverride = func(context.Context, string) error {
		killCalls++
		return nil
	}
	fixture.exit.sleepOverride = func(context.Context, time.Duration) error { return nil }
	result, err := fixture.exit.Execute(context.Background(), map[string]any{"action": "remove"})
	if err != nil || result.IsError {
		t.Fatalf("clean remove failed: err=%v result=%#v", err, result)
	}
	if killCalls != 0 {
		t.Fatalf("empty tmux name invoked killer %d times", killCalls)
	}
}

func TestWorktreeState_TmuxSessionNamePersists(t *testing.T) {
	fixture := newTask15GitFixture(t)
	fixture.state.mu.Lock()
	fixture.state.TmuxSessionName = "persisted-tmux"
	if err := fixture.state.saveToDiskLocked(); err != nil {
		fixture.state.mu.Unlock()
		t.Fatal(err)
	}
	fixture.state.mu.Unlock()
	loaded := &WorktreeState{SessionID: "task15-session"}
	loaded.mu.Lock()
	ok := loaded.loadFromDisk(fixture.repo)
	got := loaded.TmuxSessionName
	loaded.mu.Unlock()
	if !ok || got != "persisted-tmux" {
		t.Fatalf("tmux session did not round-trip: loaded=%v got=%q", ok, got)
	}
}
