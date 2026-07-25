package worktree

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/gitutil"
	"github.com/agent-dance/luban/types"
)

func noActiveWorktreeSessionMessage() string {
	return runtimeText(i18n.KeyToolRuntimeWorktreeNoActiveSession)
}

type worktreeChangeSummary struct {
	ChangedFiles int
	Commits      int
}

type exitWorktreeInput struct {
	Action         string `json:"action"`
	DiscardChanges bool   `json:"discard_changes"`
}

type exitWorktreeSnapshot struct {
	Path               string
	Branch             string
	RepoRoot           string
	OriginalDir        string
	OriginalHeadCommit string
	TmuxSessionName    string
	CreatedHere        bool
	HookBased          bool
	Lifecycle          LifecyclePublisher
}

func (t *ExitWorktreeTool) ToolMetadata(input map[string]any) types.ToolMetadata {
	action, _ := input["action"].(string)
	return types.ToolMetadata{
		Write:              true,
		Destructive:        strings.TrimSpace(action) == "remove",
		MaxResultSizeChars: 100_000,
	}
}

func (t *ExitWorktreeTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	var output ExitWorktreeOutput
	switch value := data.(type) {
	case ExitWorktreeOutput:
		output = value
	case *ExitWorktreeOutput:
		if value != nil {
			output = *value
		}
	default:
		return types.ToolResultBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: toolUseID,
			Content:   fmt.Sprint(data),
		}
	}
	return types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: toolUseID,
		Content:   output.Message,
	}
}

func (t *ExitWorktreeTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if err := types.ValidateToolInput(t, input); err != nil {
		return types.ToolResult{Content: runtimeFormat(i18n.KeyToolRuntimeWorktreeInputValidation, err), IsError: true}, nil
	}
	in, err := types.DecodeStrictToolInput[exitWorktreeInput](input)
	if err != nil {
		return worktreeInputError(runtimeFormat(i18n.KeyToolRuntimeInvalidInput, err)), nil
	}
	if in.Action != "keep" && in.Action != "remove" {
		return worktreeInputError(runtimeFormat(i18n.KeyToolRuntimeWorktreeInvalidAction, in.Action)), nil
	}

	sessionID, state, runtimeContext := t.dependencies()
	if state == nil {
		return worktreeInputError(runtimeText(i18n.KeyToolSourceSinkWorktreeRuntimeMissing)), nil
	}
	snapshot, beginErr := beginExitWorktree(state)
	if beginErr != nil {
		return worktreeInputError(beginErr.Error()), nil
	}
	completed := false
	defer func() {
		if completed {
			return
		}
		state.mu.Lock()
		state.exiting = false
		state.mu.Unlock()
	}()

	if in.Action == "remove" && !snapshot.CreatedHere {
		return worktreeInputError(runtimeText(i18n.KeyToolRuntimeWorktreeRemoveEnteredPath)), nil
	}

	if in.Action == "remove" && !in.DiscardChanges {
		summary, countErr := countWorktreeChanges(snapshot.Path, snapshot.OriginalHeadCommit)
		if countErr != nil {
			return worktreeInputError(runtimeFormat(i18n.KeyToolRuntimeWorktreeVerifyBeforeRemove, snapshot.Path)), nil
		}
		if summary.ChangedFiles > 0 || summary.Commits > 0 {
			return worktreeInputError(worktreeChangesConfirmationMessage(snapshot, summary)), nil
		}
	}

	// Recount at execution time. A failure here is only an analytics/output
	// uncertainty: the destructive safety gate above already failed closed.
	summary, countErr := countWorktreeChanges(snapshot.Path, snapshot.OriginalHeadCommit)
	if countErr != nil {
		summary = worktreeChangeSummary{}
	}

	if err := runtimeContext.SwitchCWDContext(ctx, snapshot.OriginalDir); err != nil {
		return worktreeInputError(runtimeFormat(i18n.KeyToolRuntimeWorktreeRestoreCWD, err)), nil
	}

	if in.Action == "keep" {
		t.completeExit(state, sessionID, snapshot, "kept")
		completed = true
		message := runtimeFormat(
			i18n.KeyToolRuntimeWorktreeKept,
			snapshot.Path,
			optionalWorktreeBranch(snapshot.Branch),
			snapshot.OriginalDir,
			optionalTmuxReattachNote(snapshot.TmuxSessionName),
		)
		return exitWorktreeSuccess(ExitWorktreeOutput{
			Action:          "keep",
			OriginalCWD:     snapshot.OriginalDir,
			WorktreePath:    snapshot.Path,
			WorktreeBranch:  snapshot.Branch,
			TmuxSessionName: snapshot.TmuxSessionName,
			Message:         message,
		}, types.ToolOutcomeSucceeded)
	}

	cleanupErrors := make([]error, 0, 2)
	observeCleanupError := func(err error) {
		if err == nil {
			return
		}
		cleanupErrors = append(cleanupErrors, err)
	}

	if snapshot.TmuxSessionName != "" {
		if err := t.killTmuxSession(ctx, snapshot.TmuxSessionName); err != nil {
			observeCleanupError(i18n.WrapError(i18n.KeyToolIndirectWorktreeKillTmux, err, snapshot.TmuxSessionName))
		}
	}

	if snapshot.HookBased {
		t.removeHookWorktree(ctx, snapshot, observeCleanupError)
	} else {
		if err := t.Manager.remove(snapshot.RepoRoot, snapshot.Path, true); err != nil {
			observeCleanupError(err)
		}
	}

	if !snapshot.HookBased && snapshot.Branch != "" {
		if err := t.sleep(ctx, 100*time.Millisecond); err != nil {
			observeCleanupError(i18n.WrapError(i18n.KeyToolIndirectWorktreeWaitGitLocks, err))
		} else {
			root := snapshot.RepoRoot
			if strings.TrimSpace(root) == "" {
				root = snapshot.OriginalDir
			}
			if output, err := gitutil.Run(root, "branch", "-D", snapshot.Branch); err != nil {
				observeCleanupError(i18n.WrapInternalError(i18n.KeyToolIndirectWorktreeDeleteBranch, err, snapshot.Branch, strings.TrimSpace(output)))
			}
		}
	}

	// Once every cleanup step has been attempted, clear the current session
	// deterministically even when one failed. The lifecycle status must reflect
	// that partial outcome instead of claiming the filesystem was fully removed.
	lifecycleStatus := "removed"
	if len(cleanupErrors) > 0 {
		lifecycleStatus = "cleanup_incomplete"
	}
	t.completeExit(state, sessionID, snapshot, lifecycleStatus)
	completed = true

	message := runtimeFormat(i18n.KeyToolRuntimeWorktreeRemoved, snapshot.Path, discardedWorktreeNote(summary), snapshot.OriginalDir)
	outcome := types.ToolOutcomeSucceeded
	if len(cleanupErrors) > 0 {
		outcome = types.ToolOutcomePartial
		message = runtimeFormat(i18n.KeyToolRuntimeWorktreeCleanupIncomplete, snapshot.Path, snapshot.OriginalDir)
	}
	return exitWorktreeSuccess(ExitWorktreeOutput{
		Action:            "remove",
		OriginalCWD:       snapshot.OriginalDir,
		WorktreePath:      snapshot.Path,
		WorktreeBranch:    snapshot.Branch,
		DiscardedFiles:    intPointer(summary.ChangedFiles),
		DiscardedCommits:  intPointer(summary.Commits),
		CleanupIncomplete: len(cleanupErrors) > 0,
		CleanupIssueCount: len(cleanupErrors),
		Message:           message,
	}, outcome)
}

func beginExitWorktree(state *WorktreeState) (exitWorktreeSnapshot, error) {
	if state == nil {
		return exitWorktreeSnapshot{}, fmt.Errorf("%s", noActiveWorktreeSessionMessage())
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.Active {
		return exitWorktreeSnapshot{}, fmt.Errorf("%s", noActiveWorktreeSessionMessage())
	}
	if state.exiting {
		return exitWorktreeSnapshot{}, fmt.Errorf("%s", runtimeText(i18n.KeyToolRuntimeWorktreeExitAlreadyRunning))
	}
	state.exiting = true
	return exitWorktreeSnapshot{
		Path:               state.Path,
		Branch:             state.Branch,
		RepoRoot:           state.RepoRoot,
		OriginalDir:        state.OriginalDir,
		OriginalHeadCommit: state.OriginalHeadCommit,
		TmuxSessionName:    state.TmuxSessionName,
		CreatedHere:        state.CreatedHere,
		HookBased:          state.HookBased,
		Lifecycle:          state.lifecycle,
	}, nil
}

// countWorktreeChanges returns an error whenever cleanliness cannot be proven.
// Callers guarding destructive removal must treat that error as unsafe.
func countWorktreeChanges(worktreePath, originalHeadCommit string) (worktreeChangeSummary, error) {
	status, err := gitutil.Run(worktreePath, "status", "--porcelain=v1", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return worktreeChangeSummary{}, fmt.Errorf("git status failed: %s", strings.TrimSpace(status))
	}
	changedFiles := 0
	for _, line := range strings.Split(status, "\n") {
		if strings.TrimSpace(line) != "" {
			changedFiles++
		}
	}
	originalHeadCommit = strings.TrimSpace(originalHeadCommit)
	if originalHeadCommit == "" {
		return worktreeChangeSummary{}, fmt.Errorf("original HEAD commit is unavailable")
	}
	countOutput, err := gitutil.Run(worktreePath, "rev-list", "--count", originalHeadCommit+"..HEAD")
	if err != nil {
		return worktreeChangeSummary{}, fmt.Errorf("git rev-list failed: %s", strings.TrimSpace(countOutput))
	}
	commits, err := strconv.Atoi(strings.TrimSpace(countOutput))
	if err != nil || commits < 0 {
		return worktreeChangeSummary{}, fmt.Errorf("invalid git rev-list count %q", strings.TrimSpace(countOutput))
	}
	return worktreeChangeSummary{ChangedFiles: changedFiles, Commits: commits}, nil
}

func worktreeChangesConfirmationMessage(snapshot exitWorktreeSnapshot, summary worktreeChangeSummary) string {
	parts := make([]string, 0, 2)
	if summary.ChangedFiles > 0 {
		parts = append(parts, runtimeFormat(i18n.KeyToolRuntimeWorktreeUncommittedFiles, summary.ChangedFiles))
	}
	if summary.Commits > 0 {
		branch := snapshot.Branch
		if branch == "" {
			branch = runtimeText(i18n.KeyToolRuntimeWorktreeBranchFallback)
		}
		parts = append(parts, runtimeFormat(i18n.KeyToolRuntimeWorktreeCommitsOnBranch, summary.Commits, branch))
	}
	return runtimeFormat(i18n.KeyToolRuntimeWorktreeDiscardConfirmation, strings.Join(parts, runtimeText(i18n.KeyToolRuntimeWorktreeAnd)))
}

func (t *ExitWorktreeTool) completeExit(state *WorktreeState, sessionID string, snapshot exitWorktreeSnapshot, status string) {
	state.mu.Lock()
	state.clearLocked()
	state.mu.Unlock()
	t.Manager.releasePath(sessionID, snapshot.Path)
	t.Manager.forget(sessionID)
	clearBaseRefCacheForSession(sessionID)
	lifecycle := snapshot.Lifecycle
	if lifecycle == nil && snapshot.RepoRoot != "" {
		lifecycle = state.newLifecycle(snapshot.RepoRoot)
	}
	publishWorktreeLifecycle(lifecycle, LifecycleExit, snapshot.RepoRoot, snapshot.Branch, snapshot.Path, status, snapshot.CreatedHere)
}

func (t *ExitWorktreeTool) removeHookWorktree(ctx context.Context, snapshot exitWorktreeSnapshot, observe func(error)) {
	bridge := t.HookBridge
	if bridge == nil {
		observe(i18n.NewError(i18n.KeyToolIndirectWorktreeRemoveHookMissing, snapshot.Path))
		return
	}
	if _, ok := bridge.Lookup("WorktreeRemove"); !ok {
		observe(i18n.NewError(i18n.KeyToolIndirectWorktreeRemoveHookMissing, snapshot.Path))
		return
	}
	if _, err := runWorktreeHookWithResult(ctx, bridge, "WorktreeRemove", map[string]any{
		"hook_event_name": "WorktreeRemove",
		"worktree_path":   snapshot.Path,
	}); err != nil {
		observe(i18n.WrapError(i18n.KeyToolIndirectWorktreeRemoveHookFailed, err, snapshot.Path))
	}
}

func (t *ExitWorktreeTool) killTmuxSession(ctx context.Context, sessionName string) error {
	if t.killTmuxSessionOverride != nil {
		return t.killTmuxSessionOverride(ctx, sessionName)
	}
	path, err := exec.LookPath("tmux")
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, path, "kill-session", "-t", sessionName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (t *ExitWorktreeTool) sleep(ctx context.Context, duration time.Duration) error {
	if t.sleepOverride != nil {
		return t.sleepOverride(ctx, duration)
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func exitWorktreeSuccess(output ExitWorktreeOutput, outcome types.ToolOutcome) (types.ToolResult, error) {
	content, err := json.Marshal(output)
	if err != nil {
		return types.ToolResult{Content: runtimeFormat(i18n.KeyToolRuntimeResponseMarshalFailed, err), IsError: true, Outcome: types.ToolOutcomeFailed}, nil
	}
	return types.ToolResult{Content: string(content), Data: output, Outcome: outcome}, nil
}

func optionalWorktreeBranch(branch string) string {
	if branch == "" {
		return ""
	}
	return runtimeFormat(i18n.KeyToolRuntimeWorktreeOnBranch, branch)
}

func optionalTmuxReattachNote(sessionName string) string {
	if sessionName == "" {
		return ""
	}
	return runtimeFormat(i18n.KeyToolRuntimeWorktreeTmuxReattach, sessionName, sessionName)
}

func discardedWorktreeNote(summary worktreeChangeSummary) string {
	parts := make([]string, 0, 2)
	if summary.Commits > 0 {
		parts = append(parts, runtimeFormat(i18n.KeyToolRuntimeWorktreeCommits, summary.Commits))
	}
	if summary.ChangedFiles > 0 {
		parts = append(parts, runtimeFormat(i18n.KeyToolRuntimeWorktreeUncommittedFiles, summary.ChangedFiles))
	}
	if len(parts) == 0 {
		return ""
	}
	return runtimeFormat(i18n.KeyToolRuntimeWorktreeDiscarded, strings.Join(parts, runtimeText(i18n.KeyToolRuntimeWorktreeAnd)))
}

func intPointer(value int) *int { return &value }

func (t *ExitWorktreeTool) dependencies() (string, *WorktreeState, *WorktreeRuntime) {
	if t == nil || t.Manager == nil || t.State == nil || t.Runtime == nil || t.SessionID == nil {
		return "", nil, nil
	}
	sessionID := strings.TrimSpace(t.SessionID())
	if sessionID == "" {
		return "", nil, nil
	}
	state := t.Manager.register(sessionID, t.State)
	if state == nil {
		return "", nil, nil
	}
	return sessionID, state, t.Runtime
}
