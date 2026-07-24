package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

type enterWorktreeInput struct {
	Name    string `json:"name,omitempty"`
	Path    string `json:"path,omitempty"`
	BaseRef string `json:"base_ref,omitempty"`
}

func (t *EnterWorktreeTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{Write: true}
}

type worktreeActivation struct {
	SessionID          string
	Name               string
	Path               string
	Branch             string
	OriginalDir        string
	OriginalBranch     string
	OriginalHeadCommit string
	BaseRef            string
	RepoRoot           string
	CreatedHere        bool
	NewlyCreated       bool
	HookBased          bool
	TmuxSessionName    string
	Manager            *WorktreeManager
	Bridge             WorktreeHookBridge
	Claimed            bool
}

func (t *EnterWorktreeTool) activateSession(ctx context.Context, state *WorktreeState, runtimeContext *WorktreeRuntime, activation worktreeActivation) (types.ToolResult, error) {
	manager := activation.Manager
	if manager == nil {
		manager = t.resolvedManager()
	}
	if !activation.Claimed {
		if err := manager.ClaimPath(activation.SessionID, activation.Path); err != nil {
			// Another live session owns this path. Never run cleanup here: doing so
			// could delete the winner's worktree after a same-name race.
			return worktreeInputError(err.Error()), nil
		}
	}
	keepClaim := false
	defer func() {
		if !keepClaim {
			manager.ReleasePath(activation.SessionID, activation.Path)
		}
	}()

	state.mu.Lock()
	if state.Active {
		activePath := state.Path
		state.mu.Unlock()
		return t.rollbackActivation(ctx, activation, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeWorktreeAlreadyActive, activePath)))
	}
	state.SessionID = activation.SessionID
	state.Active = true
	state.Path = activation.Path
	state.Name = activation.Name
	state.Branch = activation.Branch
	state.OriginalDir = activation.OriginalDir
	state.OriginalBranch = activation.OriginalBranch
	state.OriginalHeadCommit = activation.OriginalHeadCommit
	state.BaseRef = activation.BaseRef
	state.RepoRoot = activation.RepoRoot
	state.StateFile = worktreeStateFilePath(activation.RepoRoot, activation.SessionID)
	state.CreatedHere = activation.CreatedHere
	state.HookBased = activation.HookBased
	state.TmuxSessionName = activation.TmuxSessionName
	state.CurrentDir = activation.Path
	state.runtime = runtimeContext
	state.lifecycle = NewRuntimeLifecycle(activation.RepoRoot)
	persistErr := state.saveToDiskLocked()
	lifecycle := state.lifecycle
	if persistErr != nil {
		state.clearLocked()
		state.mu.Unlock()
		return t.rollbackActivation(ctx, activation, i18n.WrapError(i18n.KeyToolSourceSinkWorktreePersistSession, persistErr))
	}
	state.mu.Unlock()

	if err := runtimeContext.SwitchCWDContext(ctx, activation.Path); err != nil {
		state.mu.Lock()
		state.clearLocked()
		state.mu.Unlock()
		return t.rollbackActivation(ctx, activation, i18n.WrapError(i18n.KeyToolSourceSinkWorktreeSwitchCWD, err))
	}
	InvalidateWorktreeCaches()
	status := "created"
	if !activation.NewlyCreated && activation.CreatedHere {
		status = "resumed"
	} else if !activation.CreatedHere {
		status = "entered"
	}
	publishWorktreeLifecycle(lifecycle, LifecycleWorktreeEnter, activation.RepoRoot, activation.Branch, activation.Path, status, activation.CreatedHere)
	keepClaim = true
	return enterWorktreeResult(activation.Path, activation.Branch), nil
}

func (t *EnterWorktreeTool) rollbackActivation(ctx context.Context, activation worktreeActivation, cause error) (types.ToolResult, error) {
	if !activation.NewlyCreated {
		return worktreeInputError(cause.Error()), nil
	}
	if activation.HookBased {
		if activation.Bridge != nil {
			if _, ok := activation.Bridge.Lookup("WorktreeRemove"); ok {
				_, _ = runWorktreeHookWithResult(ctx, activation.Bridge, "WorktreeRemove", map[string]any{
					"hook_event_name": "WorktreeRemove",
					"worktree_path":   activation.Path,
				})
			}
		}
	} else if activation.Manager != nil {
		_ = activation.Manager.Remove(activation.RepoRoot, activation.Path, true)
		if activation.Branch != "" {
			_, _ = runGit(activation.RepoRoot, "branch", "-D", activation.Branch)
		}
	}
	return worktreeInputError(toolRuntimeFormat(i18n.KeyToolRuntimeWorktreeRolledBack, cause)), nil
}

func enterWorktreeResult(path, branch string) types.ToolResult {
	branchInfo := ""
	if branch != "" {
		branchInfo = toolRuntimeFormat(i18n.KeyToolRuntimeWorktreeOnBranch, branch)
	}
	message := toolRuntimeFormat(i18n.KeyToolRuntimeWorktreeEntered, path, branchInfo)
	return types.ToolResult{
		Content: message,
		Data: EnterWorktreeOutput{
			WorktreePath:   path,
			WorktreeBranch: branch,
			Message:        message,
		},
	}
}

func worktreeInputError(message string) types.ToolResult {
	return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeErrorPrefix, message), IsError: true}
}

func (t *EnterWorktreeTool) resolvedSessionID() string {
	if t.SessionID != nil {
		if value := strings.TrimSpace(t.SessionID()); value != "" {
			return value
		}
	}
	if t.State != nil && strings.TrimSpace(t.State.SessionID) != "" {
		return strings.TrimSpace(t.State.SessionID)
	}
	if value := strings.TrimSpace(os.Getenv("CLAUDE_SESSION_ID")); value != "" {
		return value
	}
	if t.State == nil {
		t.State = &WorktreeState{}
	}
	return stateID(t.State)
}

func (t *EnterWorktreeTool) stateForSession(sessionID string) *WorktreeState {
	if t.State == nil {
		t.State = &WorktreeState{}
	}
	if t.Manager == nil {
		t.State.SessionID = sessionID
		return t.State
	}
	return t.Manager.StateForSession(sessionID, t.State)
}

func (t *EnterWorktreeTool) runtimeForState(state *WorktreeState) *WorktreeRuntime {
	if t.Runtime != nil {
		return t.Runtime
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.runtime == nil {
		cwd := state.CurrentDir
		if cwd == "" {
			cwd, _ = os.Getwd()
		}
		state.runtime = NewWorktreeRuntime(cwd, nil)
	}
	return state.runtime
}

func (t *EnterWorktreeTool) resolvedManager() *WorktreeManager {
	if t.Manager != nil {
		return t.Manager
	}
	return DefaultWorktreeManager()
}

func (t *EnterWorktreeTool) resolvedHookBridge() WorktreeHookBridge {
	if t.HookBridge != nil {
		return t.HookBridge
	}
	return DefaultWorktreeHookBridge()
}

func generatedWorktreeSlug() string { return fmt.Sprintf("wt-%d", time.Now().UnixNano()) }

func resolveWorktreePathFrom(base, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "", fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolRuntimeWorktreePathEmpty))
	}
	if !filepath.IsAbs(requested) {
		requested = filepath.Join(base, requested)
	}
	path := cleanWorktreePath(requested)
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeWorktreeNotDirectory, path))
	}
	return path, nil
}

func runWorktreeHookWithResult(ctx context.Context, bridge WorktreeHookBridge, name string, payload map[string]any) (WorktreeHookResult, error) {
	if resultBridge, ok := bridge.(WorktreeHookResultBridge); ok {
		return resultBridge.RunWithResult(ctx, name, payload)
	}
	if err := bridge.Run(ctx, name, payload); err != nil {
		return WorktreeHookResult{}, err
	}
	return WorktreeHookResult{}, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeWorktreeHookNoStructuredOutput, name))
}

func (t *EnterWorktreeTool) enterExisting(ctx context.Context, manager *WorktreeManager, state *WorktreeState, runtimeContext *WorktreeRuntime, sessionID, repoRoot, originalDir, requestedPath string) (types.ToolResult, error) {
	absPath, err := resolveWorktreePathFrom(originalDir, requestedPath)
	if err != nil {
		return worktreeInputError(toolRuntimeFormat(i18n.KeyToolRuntimeWorktreeResolvePath, requestedPath, err)), nil
	}
	refs, err := manager.List(repoRoot)
	if err != nil {
		return worktreeInputError(err.Error()), nil
	}
	for _, ref := range refs {
		if ref.Path != absPath {
			continue
		}
		originalBranch, _ := runGit(originalDir, "rev-parse", "--abbrev-ref", "HEAD")
		originalHead, _ := runGit(originalDir, "rev-parse", "HEAD")
		return t.activateSession(ctx, state, runtimeContext, worktreeActivation{
			SessionID:          sessionID,
			Name:               filepath.Base(absPath),
			Path:               absPath,
			Branch:             ref.Branch,
			OriginalDir:        originalDir,
			OriginalBranch:     strings.TrimSpace(originalBranch),
			OriginalHeadCommit: strings.TrimSpace(originalHead),
			RepoRoot:           repoRoot,
			CreatedHere:        false,
			Manager:            manager,
		})
	}
	return worktreeInputError(toolRuntimeFormat(i18n.KeyToolRuntimeWorktreeNotRegistered, absPath)), nil
}
