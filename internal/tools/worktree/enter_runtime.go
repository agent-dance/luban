package worktree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/gitutil"
	"github.com/agent-dance/luban/types"
)

type enterWorktreeInput struct {
	Name    string `json:"name,omitempty"`
	Path    string `json:"path,omitempty"`
	BaseRef string `json:"base_ref,omitempty"`
}

func (t *EnterWorktreeTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{Write: true, MaxResultSizeChars: 100_000}
}

type worktreeActivation struct {
	SessionID          string
	Name               string
	Path               string
	Branch             string
	OriginalDir        string
	OriginalHeadCommit string
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
	if !activation.Claimed {
		if err := manager.claimPath(activation.SessionID, activation.Path); err != nil {
			// Another live session owns this path. Never run cleanup here: doing so
			// could delete the winner's worktree after a same-name race.
			return worktreeInputError(err.Error()), nil
		}
	}
	keepClaim := false
	defer func() {
		if !keepClaim {
			manager.releasePath(activation.SessionID, activation.Path)
		}
	}()

	state.mu.Lock()
	if state.Active {
		activePath := state.Path
		state.mu.Unlock()
		return t.rollbackActivation(ctx, activation, fmt.Errorf("%s", runtimeFormat(i18n.KeyToolRuntimeWorktreeAlreadyActive, activePath)))
	}
	state.SessionID = activation.SessionID
	state.Active = true
	state.Path = activation.Path
	state.Name = activation.Name
	state.Branch = activation.Branch
	state.OriginalDir = activation.OriginalDir
	state.OriginalHeadCommit = activation.OriginalHeadCommit
	state.RepoRoot = activation.RepoRoot
	state.StateFile = worktreeStateFilePath(activation.RepoRoot, activation.SessionID)
	state.CreatedHere = activation.CreatedHere
	state.HookBased = activation.HookBased
	state.TmuxSessionName = activation.TmuxSessionName
	state.lifecycle = state.newLifecycle(activation.RepoRoot)
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
	status := "created"
	if !activation.NewlyCreated && activation.CreatedHere {
		status = "resumed"
	} else if !activation.CreatedHere {
		status = "entered"
	}
	publishWorktreeLifecycle(lifecycle, LifecycleEnter, activation.RepoRoot, activation.Branch, activation.Path, status, activation.CreatedHere)
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
		_ = activation.Manager.remove(activation.RepoRoot, activation.Path, true)
		if activation.Branch != "" {
			_, _ = gitutil.Run(activation.RepoRoot, "branch", "-D", activation.Branch)
		}
	}
	return worktreeInputError(runtimeFormat(i18n.KeyToolRuntimeWorktreeRolledBack, cause)), nil
}

func enterWorktreeResult(path, branch string) types.ToolResult {
	branchInfo := ""
	if branch != "" {
		branchInfo = runtimeFormat(i18n.KeyToolRuntimeWorktreeOnBranch, branch)
	}
	message := runtimeFormat(i18n.KeyToolRuntimeWorktreeEntered, path, branchInfo)
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
	return types.ToolResult{Content: runtimeFormat(i18n.KeyToolRuntimeErrorPrefix, message), IsError: true}
}

func (t *EnterWorktreeTool) dependencies() (string, *WorktreeState, *WorktreeManager, *WorktreeRuntime) {
	if t == nil || t.Manager == nil || t.State == nil || t.Runtime == nil || t.SessionID == nil {
		return "", nil, nil, nil
	}
	sessionID := strings.TrimSpace(t.SessionID())
	if sessionID == "" {
		return "", nil, nil, nil
	}
	state := t.Manager.register(sessionID, t.State)
	if state == nil {
		return "", nil, nil, nil
	}
	return sessionID, state, t.Manager, t.Runtime
}

func generatedWorktreeSlug() string { return fmt.Sprintf("wt-%d", time.Now().UnixNano()) }

func resolveWorktreePathFrom(base, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "", fmt.Errorf("%s", runtimeText(i18n.KeyToolRuntimeWorktreePathEmpty))
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
		return "", fmt.Errorf("%s", runtimeFormat(i18n.KeyToolRuntimeWorktreeNotDirectory, path))
	}
	return path, nil
}

func runWorktreeHookWithResult(ctx context.Context, bridge WorktreeHookBridge, name string, payload map[string]any) (WorktreeHookResult, error) {
	return bridge.RunWithResult(ctx, name, payload)
}

func (t *EnterWorktreeTool) enterExisting(ctx context.Context, manager *WorktreeManager, state *WorktreeState, runtimeContext *WorktreeRuntime, sessionID, repoRoot, originalDir, requestedPath string) (types.ToolResult, error) {
	absPath, err := resolveWorktreePathFrom(originalDir, requestedPath)
	if err != nil {
		return worktreeInputError(runtimeFormat(i18n.KeyToolRuntimeWorktreeResolvePath, requestedPath, err)), nil
	}
	refs, err := manager.list(repoRoot)
	if err != nil {
		return worktreeInputError(err.Error()), nil
	}
	for _, ref := range refs {
		if ref.Path != absPath {
			continue
		}
		originalHead, _ := gitutil.Run(originalDir, "rev-parse", "HEAD")
		return t.activateSession(ctx, state, runtimeContext, worktreeActivation{
			SessionID:          sessionID,
			Name:               filepath.Base(absPath),
			Path:               absPath,
			Branch:             ref.Branch,
			OriginalDir:        originalDir,
			OriginalHeadCommit: strings.TrimSpace(originalHead),
			RepoRoot:           repoRoot,
			CreatedHere:        false,
			Manager:            manager,
		})
	}
	return worktreeInputError(runtimeFormat(i18n.KeyToolRuntimeWorktreeNotRegistered, absPath)), nil
}
