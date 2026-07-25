package worktree

import (
	"context"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/gitutil"
	"github.com/agent-dance/luban/types"
)

func (t *EnterWorktreeTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if err := types.ValidateToolInput(t, input); err != nil {
		return types.ToolResult{Content: runtimeFormat(i18n.KeyToolRuntimeWorktreeInputValidation, err), IsError: true}, nil
	}
	in, err := types.DecodeStrictToolInput[enterWorktreeInput](input)
	if err != nil {
		return worktreeInputError(runtimeFormat(i18n.KeyToolRuntimeInvalidInput, err)), nil
	}
	_, hasName := input["name"]
	_, hasPath := input["path"]
	if hasName && hasPath {
		return worktreeInputError(runtimeText(i18n.KeyToolRuntimeWorktreeNamePathExclusive)), nil
	}
	if hasPath && strings.TrimSpace(in.BaseRef) != "" {
		return worktreeInputError(runtimeText(i18n.KeyToolRuntimeWorktreeBaseRefWithPath)), nil
	}
	name := in.Name
	if hasName {
		if _, err := normalizeSlug(name); err != nil {
			return errorResponse(err), nil
		}
	}
	if hasPath && strings.TrimSpace(in.Path) == "" {
		return worktreeInputError(runtimeText(i18n.KeyToolRuntimeWorktreePathEmpty)), nil
	}

	sessionID, state, manager, runtimeContext := t.dependencies()
	if state == nil {
		return worktreeInputError(runtimeText(i18n.KeyToolSourceSinkWorktreeRuntimeMissing)), nil
	}
	originalDir := runtimeContext.CurrentCWD()
	if originalDir == "" {
		return worktreeInputError(runtimeText(i18n.KeyToolRuntimeWorktreeCWDUnavailable)), nil
	}
	state.mu.Lock()
	if state.Active {
		activePath := state.Path
		state.mu.Unlock()
		return worktreeInputError(runtimeFormat(i18n.KeyToolRuntimeWorktreeAlreadyActive, activePath)), nil
	}
	state.mu.Unlock()

	bridge := t.HookBridge
	if bridge != nil {
		if _, ok := bridge.Lookup("WorktreeCreate"); ok {
			if hasPath {
				return worktreeInputError(runtimeText(i18n.KeyToolRuntimeWorktreePathWithHook)), nil
			}
			if !hasName {
				name = generatedWorktreeSlug()
			}
			hookResult, hookErr := runWorktreeHookWithResult(ctx, bridge, "WorktreeCreate", map[string]any{
				"hook_event_name": "WorktreeCreate",
				"name":            name,
				"baseRef":         strings.TrimSpace(in.BaseRef),
			})
			if hookErr != nil {
				return worktreeInputError(hookErr.Error()), nil
			}
			hookPath, pathErr := resolveWorktreePathFrom(originalDir, hookResult.Path)
			if pathErr != nil {
				if _, ok := bridge.Lookup("WorktreeRemove"); ok {
					_, _ = runWorktreeHookWithResult(ctx, bridge, "WorktreeRemove", map[string]any{
						"hook_event_name": "WorktreeRemove",
						"worktree_path":   hookResult.Path,
					})
				}
				return worktreeInputError(runtimeFormat(i18n.KeyToolRuntimeWorktreeInvalidHookOutput, pathErr)), nil
			}
			return t.activateSession(ctx, state, runtimeContext, worktreeActivation{
				SessionID:    sessionID,
				Name:         name,
				Path:         hookPath,
				Branch:       hookResult.Branch,
				OriginalDir:  originalDir,
				RepoRoot:     originalDir,
				CreatedHere:  true,
				NewlyCreated: true,
				HookBased:    true,
				Manager:      manager,
				Bridge:       bridge,
			})
		}
	}

	repoRoot, rootErr := gitutil.CanonicalRoot(originalDir)
	if rootErr != nil {
		return worktreeInputError(runtimeText(i18n.KeyToolRuntimeWorktreeNoRepositoryOrHook)), nil
	}
	state.mu.Lock()
	loaded := state.loadFromDisk(repoRoot)
	if loaded {
		persistedName, persistedPath, persistedBranch := state.Name, state.Path, state.Branch
		state.mu.Unlock()
		requestedPath := ""
		if hasPath {
			requestedPath, err = resolveWorktreePathFrom(originalDir, in.Path)
			if err != nil {
				return worktreeInputError(runtimeFormat(i18n.KeyToolRuntimeWorktreeResolvePath, in.Path, err)), nil
			}
		}
		matches := (!hasName || name == persistedName) && (!hasPath || requestedPath == cleanWorktreePath(persistedPath))
		if !matches {
			return worktreeInputError(runtimeFormat(i18n.KeyToolRuntimeWorktreeAlreadyActive, persistedPath)), nil
		}
		if err := manager.claimPath(sessionID, persistedPath); err != nil {
			return worktreeInputError(err.Error()), nil
		}
		if err := runtimeContext.SwitchCWDContext(ctx, persistedPath); err != nil {
			manager.releasePath(sessionID, persistedPath)
			return worktreeInputError(runtimeFormat(i18n.KeyToolRuntimeWorktreeRestoreFailed, err)), nil
		}
		state.mu.Lock()
		lifecycle := state.lifecycle
		createdHere := state.CreatedHere
		state.mu.Unlock()
		publishWorktreeLifecycle(lifecycle, LifecycleEnter, repoRoot, persistedBranch, persistedPath, "resumed", createdHere)
		return enterWorktreeResult(persistedPath, persistedBranch), nil
	}
	state.mu.Unlock()

	if hasPath {
		return t.enterExisting(ctx, manager, state, runtimeContext, sessionID, repoRoot, originalDir, in.Path)
	}
	baseSetting := strings.TrimSpace(in.BaseRef)
	if !hasName && isPRRef(baseSetting) {
		parsed, parseErr := parsePRRef(baseSetting)
		if parseErr != nil {
			return errorResponse(parseErr), nil
		}
		name = suggestedPRWorktreeName(parsed)
	}
	if name == "" {
		name = generatedWorktreeSlug()
	}
	if _, err := normalizeSlug(name); err != nil {
		return errorResponse(err), nil
	}

	var baseRef string
	if isPRRef(baseSetting) {
		parsed, parseErr := parsePRRef(baseSetting)
		if parseErr != nil {
			return errorResponse(parseErr), nil
		}
		baseRef, err = preparePRRef(repoRoot, parsed)
	} else {
		baseRef, err = resolveBaseRefAt(repoRoot, sessionID, baseSetting)
	}
	if err != nil {
		return errorResponse(err), nil
	}
	originalHead, _ := gitutil.Run(originalDir, "rev-parse", "HEAD")
	expectedPath, _ := worktreePathAndBranch(repoRoot, name)
	if claimErr := manager.claimPath(sessionID, expectedPath); claimErr != nil {
		return worktreeInputError(claimErr.Error()), nil
	}
	created, createErr := manager.create(repoRoot, name, baseRef, worktreeSparseCheckoutPatterns())
	if createErr != nil {
		manager.releasePath(sessionID, expectedPath)
		return worktreeInputError(createErr.Error()), nil
	}
	if created.Created {
		if includes := worktreeIncludeFile(repoRoot); len(includes) > 0 {
			applyWorktreeIncludes(repoRoot, created.Path, includes)
		}
		applyWorktreeSettingsAndHusky(repoRoot, created.Path)
	}
	return t.activateSession(ctx, state, runtimeContext, worktreeActivation{
		SessionID:          sessionID,
		Name:               name,
		Path:               created.Path,
		Branch:             created.Branch,
		OriginalDir:        originalDir,
		OriginalHeadCommit: strings.TrimSpace(originalHead),
		RepoRoot:           repoRoot,
		CreatedHere:        true,
		NewlyCreated:       created.Created,
		Manager:            manager,
		Claimed:            true,
	})
}
