package tools

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func (t *EnterWorktreeTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if err := types.ValidateToolInput(t, input); err != nil {
		return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeWorktreeInputValidation, err), IsError: true}, nil
	}
	in, err := types.DecodeStrictToolInput[enterWorktreeInput](input)
	if err != nil {
		return worktreeInputError(toolRuntimeFormat(i18n.KeyToolRuntimeInvalidInput, err)), nil
	}
	_, hasName := input["name"]
	_, hasPath := input["path"]
	if hasName && hasPath {
		return worktreeInputError(toolRuntimeText(i18n.KeyToolRuntimeWorktreeNamePathExclusive)), nil
	}
	if hasPath && strings.TrimSpace(in.BaseRef) != "" {
		return worktreeInputError(toolRuntimeText(i18n.KeyToolRuntimeWorktreeBaseRefWithPath)), nil
	}
	name := in.Name
	if hasName {
		if err := validateWorktreeSlug(name); err != nil {
			return ErrorResponse(err), nil
		}
	}
	if hasPath && strings.TrimSpace(in.Path) == "" {
		return worktreeInputError(toolRuntimeText(i18n.KeyToolRuntimeWorktreePathEmpty)), nil
	}

	sessionID := t.resolvedSessionID()
	state := t.stateForSession(sessionID)
	runtimeContext := t.runtimeForState(state)
	originalDir := runtimeContext.CurrentCWD()
	if originalDir == "" {
		return worktreeInputError(toolRuntimeText(i18n.KeyToolRuntimeWorktreeCWDUnavailable)), nil
	}
	state.mu.Lock()
	if state.Active {
		activePath := state.Path
		state.mu.Unlock()
		return worktreeInputError(toolRuntimeFormat(i18n.KeyToolRuntimeWorktreeAlreadyActive, activePath)), nil
	}
	state.mu.Unlock()

	bridge := t.resolvedHookBridge()
	if bridge != nil {
		if _, ok := bridge.Lookup("WorktreeCreate"); ok {
			if hasPath {
				return worktreeInputError(toolRuntimeText(i18n.KeyToolRuntimeWorktreePathWithHook)), nil
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
				return worktreeInputError(toolRuntimeFormat(i18n.KeyToolRuntimeWorktreeInvalidHookOutput, pathErr)), nil
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
				Bridge:       bridge,
			})
		}
	}

	repoRoot, rootErr := canonicalGitRootFrom(originalDir)
	if rootErr != nil {
		return worktreeInputError(toolRuntimeText(i18n.KeyToolRuntimeWorktreeNoRepositoryOrHook)), nil
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
				return worktreeInputError(toolRuntimeFormat(i18n.KeyToolRuntimeWorktreeResolvePath, in.Path, err)), nil
			}
		}
		matches := (!hasName || name == persistedName) && (!hasPath || requestedPath == cleanWorktreePath(persistedPath))
		if !matches {
			return worktreeInputError(toolRuntimeFormat(i18n.KeyToolRuntimeWorktreeAlreadyActive, persistedPath)), nil
		}
		manager := t.resolvedManager()
		if err := manager.ClaimPath(sessionID, persistedPath); err != nil {
			return worktreeInputError(err.Error()), nil
		}
		if err := runtimeContext.SwitchCWDContext(ctx, persistedPath); err != nil {
			manager.ReleasePath(sessionID, persistedPath)
			return worktreeInputError(toolRuntimeFormat(i18n.KeyToolRuntimeWorktreeRestoreFailed, err)), nil
		}
		state.mu.Lock()
		state.runtime = runtimeContext
		state.CurrentDir = persistedPath
		lifecycle := state.lifecycle
		createdHere := state.CreatedHere
		state.mu.Unlock()
		InvalidateWorktreeCaches()
		publishWorktreeLifecycle(lifecycle, LifecycleWorktreeEnter, repoRoot, persistedBranch, persistedPath, "resumed", createdHere)
		return enterWorktreeResult(persistedPath, persistedBranch), nil
	}
	state.mu.Unlock()

	manager := t.resolvedManager()
	if hasPath {
		return t.enterExisting(ctx, manager, state, runtimeContext, sessionID, repoRoot, originalDir, in.Path)
	}
	baseSetting := strings.TrimSpace(in.BaseRef)
	if !hasName && IsPRRef(baseSetting) {
		parsed, parseErr := ParsePRRef(baseSetting)
		if parseErr != nil {
			return ErrorResponse(parseErr), nil
		}
		name = SuggestedPRWorktreeName(parsed)
	}
	if name == "" {
		name = generatedWorktreeSlug()
	}
	if err := validateWorktreeSlug(name); err != nil {
		return ErrorResponse(err), nil
	}

	var baseRef string
	if IsPRRef(baseSetting) {
		parsed, parseErr := ParsePRRef(baseSetting)
		if parseErr != nil {
			return ErrorResponse(parseErr), nil
		}
		baseRef, err = PreparePRRef(repoRoot, parsed)
	} else {
		baseRef, err = ResolveBaseRefAt(repoRoot, sessionID, baseSetting)
	}
	if err != nil {
		return ErrorResponse(err), nil
	}
	originalBranch, _ := runGit(originalDir, "rev-parse", "--abbrev-ref", "HEAD")
	originalHead, _ := runGit(originalDir, "rev-parse", "HEAD")
	expectedPath, _ := worktreePathAndBranch(repoRoot, name)
	if claimErr := manager.ClaimPath(sessionID, expectedPath); claimErr != nil {
		return worktreeInputError(claimErr.Error()), nil
	}
	created, createErr := manager.Create(repoRoot, name, baseRef, worktreeSparseCheckoutPatterns())
	if createErr != nil {
		manager.ReleasePath(sessionID, expectedPath)
		return worktreeInputError(createErr.Error()), nil
	}
	if created.Created {
		if includes := WorktreeIncludeFile(repoRoot); len(includes) > 0 {
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
		OriginalBranch:     strings.TrimSpace(originalBranch),
		OriginalHeadCommit: strings.TrimSpace(originalHead),
		BaseRef:            baseRef,
		RepoRoot:           repoRoot,
		CreatedHere:        true,
		NewlyCreated:       created.Created,
		Manager:            manager,
		Claimed:            true,
	})
}

func (t *EnterWorktreeTool) enterCreatedSession(repoRoot, absPath, branch string, createdHere, resumed bool, baseRef string) (types.ToolResult, error) {
	sessionID := t.resolvedSessionID()
	state := t.stateForSession(sessionID)
	runtimeContext := t.runtimeForState(state)
	return t.activateSession(context.Background(), state, runtimeContext, worktreeActivation{
		SessionID:    sessionID,
		Name:         filepath.Base(absPath),
		Path:         cleanWorktreePath(absPath),
		Branch:       branch,
		OriginalDir:  runtimeContext.CurrentCWD(),
		BaseRef:      baseRef,
		RepoRoot:     cleanWorktreePath(repoRoot),
		CreatedHere:  createdHere,
		NewlyCreated: createdHere,
		Manager:      t.resolvedManager(),
	})
}
