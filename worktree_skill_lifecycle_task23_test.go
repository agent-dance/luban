package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/engine"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/session"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

type task23WorktreeLifecycleProvider struct {
	mu      sync.Mutex
	steps   [][]types.StreamEvent
	calls   []provider.Params
	callErr error
}

func (p *task23WorktreeLifecycleProvider) Name() string    { return "task23-worktree" }
func (p *task23WorktreeLifecycleProvider) ModelID() string { return "task23-worktree-model" }
func (p *task23WorktreeLifecycleProvider) CreateStream(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	index := len(p.calls)
	params.Messages = append([]types.Message(nil), params.Messages...)
	p.calls = append(p.calls, params)
	if index >= len(p.steps) {
		p.mu.Unlock()
		return nil, errors.New("unexpected provider call")
	}
	step := append([]types.StreamEvent(nil), p.steps[index]...)
	p.mu.Unlock()
	stream := make(chan types.StreamEvent, len(step))
	for _, event := range step {
		stream <- event
	}
	close(stream)
	return stream, nil
}

func (p *task23WorktreeLifecycleProvider) Calls() []provider.Params {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]provider.Params(nil), p.calls...)
}

func TestWorktreeEnterExitRetargetsOneManagerAndKeepsExactSessionNamespace(t *testing.T) {
	rootA := t.TempDir()
	rootB := filepath.Join(t.TempDir(), "worktree-b")
	writeRootTask23Skill(t, rootA, "task23-root-a")
	task23RunGit(t, rootA, "init")
	task23RunGit(t, rootA, "config", "user.email", "task23@example.test")
	task23RunGit(t, rootA, "config", "user.name", "Task 23")
	task23RunGit(t, rootA, "add", ".")
	task23RunGit(t, rootA, "commit", "-m", "initial")
	task23RunGit(t, rootA, "worktree", "add", "-b", "task23-worktree-b", rootB)
	if err := os.RemoveAll(filepath.Join(rootB, ".luban-code", "skills", "task23-root-a")); err != nil {
		t.Fatal(err)
	}
	writeRootTask23Skill(t, rootB, "task23-root-b")

	provider := &task23WorktreeLifecycleProvider{steps: [][]types.StreamEvent{
		task23GenericToolEvents("enter-worktree", "EnterWorktree", map[string]any{"path": rootB}),
		task23SkillRuntimeTextEvents("entered"),
		task23SkillRuntimeTextEvents("working in B"),
		task23GenericToolEvents("exit-worktree", "ExitWorktree", map[string]any{"action": "keep"}),
		task23SkillRuntimeTextEvents("exited"),
		task23SkillRuntimeTextEvents("working in A"),
	}}
	ref := providerpkgRef(provider)
	deps := SetupRegistry(ref, rootA, []string{rootA}, sandbox.NoopBackend{}, nil)
	t.Cleanup(func() {
		deps.CronStore.Stop()
		deps.StopWebFetchCache()
	})
	if err := prepareInitialRegistryRuntime(deps, rootA, []string{rootA}); err != nil {
		t.Fatal(err)
	}
	const sessionID = "task23-worktree-session"
	deps.BindSessionIdentity(sessionID)
	manager := deps.SkillManager
	initialGeneration := manager.ProjectGeneration()

	repo := session.NewRepository(t.TempDir())
	sessionProjectDir := repo.ProjectDirForCWD(rootA)
	eng, err := engine.New(engine.Config{
		Provider: provider, ProviderRef: ref, Registry: deps.Registry,
		Sessions:    engine.NewRepositorySessionManager(repo, func() string { return sessionProjectDir }),
		ProjectRoot: rootA, CWD: rootA, AllowedDirs: []string{rootA},
		SkillManager: deps.SkillManager, SkillSessionOverrides: deps.SkillSessionOverrides,
		MaxTurns: 6,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Shutdown(context.Background()) })
	cwd := rootA
	hookRunner := loadHooks(rootA)
	configureWorktreeSessionRuntime(deps, eng, &cwd, &hookRunner, "", nil)

	firstErr := task23RunEngineQuery(t, eng, engine.QueryRequest{
		SessionID: sessionID, SessionProjectDir: sessionProjectDir,
		Message: "enter", CWD: rootA, ProjectRoot: rootA,
	})
	if firstErr != nil {
		t.Fatalf("entering run: %v", firstErr)
	}
	if deps.SkillManager != manager || !sameRuntimePath(cwd, rootB) || !sameRuntimePath(deps.WorktreeRuntime.CurrentCWD(), rootB) {
		t.Fatalf("enter publish mismatch: manager=%p/%p cwd=%q runtime=%q", deps.SkillManager, manager, cwd, deps.WorktreeRuntime.CurrentCWD())
	}
	if manager.ProjectGeneration() == initialGeneration {
		t.Fatal("EnterWorktree did not advance project authority")
	}
	entered, err := manager.Snapshot(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, old := rootTask23Skill(entered, "task23-root-a"); old {
		t.Fatalf("entered catalog retained root A: %+v", entered.Skills)
	}
	if _, current := rootTask23Skill(entered, "task23-root-b"); !current {
		t.Fatalf("entered catalog missing root B: %+v", entered.Skills)
	}

	secondErr := task23RunEngineQuery(t, eng, engine.QueryRequest{
		SessionID: sessionID, SessionProjectDir: sessionProjectDir,
		Message: "continue in B", CWD: rootB, ProjectRoot: rootB,
	})
	if secondErr != nil {
		t.Fatalf("post-enter B query: %v", secondErr)
	}

	thirdErr := task23RunEngineQuery(t, eng, engine.QueryRequest{
		SessionID: sessionID, SessionProjectDir: sessionProjectDir,
		Message: "exit", CWD: rootB, ProjectRoot: rootB,
	})
	if thirdErr != nil {
		t.Fatalf("exiting run: %v", thirdErr)
	}
	if deps.SkillManager != manager || !sameRuntimePath(cwd, rootA) || !sameRuntimePath(deps.WorktreeRuntime.CurrentCWD(), rootA) {
		t.Fatalf("exit publish mismatch: manager=%p/%p cwd=%q runtime=%q", deps.SkillManager, manager, cwd, deps.WorktreeRuntime.CurrentCWD())
	}

	fourthErr := task23RunEngineQuery(t, eng, engine.QueryRequest{
		SessionID: sessionID, SessionProjectDir: sessionProjectDir,
		Message: "continue", CWD: rootA, ProjectRoot: rootA,
	})
	if fourthErr != nil {
		t.Fatalf("post-exit query: %v", fourthErr)
	}
	calls := provider.Calls()
	if len(calls) != 6 {
		t.Fatalf("provider calls = %d, want enter/text, B, exit/text, A", len(calls))
	}
	for index, call := range calls {
		if call.PromptCacheKey != sessionID {
			t.Fatalf("call %d PromptCacheKey = %q, want %q", index, call.PromptCacheKey, sessionID)
		}
	}
	task23AssertCatalogTransition(t, calls[0].Messages, "task23-root-a", "")
	// The old A run continues only with its frozen A catalog after Enter; B is
	// first disclosed to a fresh run/generation.
	task23AssertCatalogTransition(t, calls[1].Messages, "task23-root-a", "")
	task23AssertCatalogTransition(t, calls[2].Messages, "task23-root-b", "task23-root-a")
	task23AssertCatalogTransition(t, calls[3].Messages, "task23-root-b", "task23-root-a")
	task23AssertCatalogTransition(t, calls[4].Messages, "task23-root-b", "task23-root-a")
	task23AssertCatalogTransition(t, calls[5].Messages, "task23-root-a", "task23-root-b")
	if _, _, err := repo.LoadByID(sessionID, sessionProjectDir); err != nil {
		t.Fatalf("original durable session namespace missing: %v", err)
	}
	worktreeProjectDir := repo.ProjectDirForCWD(rootB)
	if worktreeProjectDir != sessionProjectDir {
		if _, err := repo.StoreForProjectDir(worktreeProjectDir).Load(sessionID); err == nil {
			t.Fatal("worktree transition created a second durable conversation namespace")
		}
	}
}

func TestWorktreeEnterThenExitInOneRunKeepsCatalogFrozenAndReturnsToOrigin(t *testing.T) {
	rootA := t.TempDir()
	rootB := filepath.Join(t.TempDir(), "worktree-b")
	writeRootTask23Skill(t, rootA, "same-run-a")
	task23RunGit(t, rootA, "init")
	task23RunGit(t, rootA, "config", "user.email", "task23@example.test")
	task23RunGit(t, rootA, "config", "user.name", "Task 23")
	task23RunGit(t, rootA, "add", ".")
	task23RunGit(t, rootA, "commit", "-m", "initial")
	task23RunGit(t, rootA, "worktree", "add", "-b", "task23-same-run-b", rootB)
	if err := os.RemoveAll(filepath.Join(rootB, ".luban-code", "skills", "same-run-a")); err != nil {
		t.Fatal(err)
	}
	writeRootTask23Skill(t, rootB, "same-run-b")

	provider := &task23WorktreeLifecycleProvider{steps: [][]types.StreamEvent{
		task23GenericToolEvents("same-run-enter", "EnterWorktree", map[string]any{"path": rootB}),
		task23GenericToolEvents("same-run-exit", "ExitWorktree", map[string]any{"action": "keep"}),
		task23SkillRuntimeTextEvents("returned"),
	}}
	ref := providerpkgRef(provider)
	deps := SetupRegistry(ref, rootA, []string{rootA}, sandbox.NoopBackend{}, nil)
	t.Cleanup(func() {
		deps.CronStore.Stop()
		deps.StopWebFetchCache()
	})
	if err := prepareInitialRegistryRuntime(deps, rootA, []string{rootA}); err != nil {
		t.Fatal(err)
	}
	const sessionID = "task23-same-run-session"
	deps.BindSessionIdentity(sessionID)
	manager := deps.SkillManager
	repo := session.NewRepository(t.TempDir())
	sessionProjectDir := repo.ProjectDirForCWD(rootA)
	eng, err := engine.New(engine.Config{
		Provider: provider, ProviderRef: ref, Registry: deps.Registry,
		Sessions:    engine.NewRepositorySessionManager(repo, func() string { return sessionProjectDir }),
		ProjectRoot: rootA, CWD: rootA, AllowedDirs: []string{rootA},
		SkillManager: manager, SkillSessionOverrides: deps.SkillSessionOverrides, MaxTurns: 6,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Shutdown(context.Background()) })
	cwd := rootA
	hookRunner := loadHooks(rootA)
	configureWorktreeSessionRuntime(deps, eng, &cwd, &hookRunner, "", nil)

	if err := task23RunEngineQuery(t, eng, engine.QueryRequest{
		SessionID: sessionID, SessionProjectDir: sessionProjectDir,
		Message: "enter and then exit", CWD: rootA, ProjectRoot: rootA,
	}); err != nil {
		t.Fatalf("same-run enter/exit: %v", err)
	}
	if deps.SkillManager != manager || !sameRuntimePath(cwd, rootA) || !sameRuntimePath(deps.WorktreeRuntime.CurrentCWD(), rootA) {
		t.Fatalf("same-run final runtime = manager %p/%p cwd %q runtime %q", deps.SkillManager, manager, cwd, deps.WorktreeRuntime.CurrentCWD())
	}
	finalSnapshot, err := manager.Snapshot(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := rootTask23Skill(finalSnapshot, "same-run-a"); !found {
		t.Fatalf("same-run final catalog missing A: %+v", finalSnapshot.Skills)
	}
	if _, found := rootTask23Skill(finalSnapshot, "same-run-b"); found {
		t.Fatalf("same-run final catalog leaked B: %+v", finalSnapshot.Skills)
	}
	calls := provider.Calls()
	if len(calls) != 3 {
		t.Fatalf("same-run provider calls = %d, want enter, exit, text", len(calls))
	}
	for index, call := range calls {
		if call.PromptCacheKey != sessionID {
			t.Fatalf("same-run call %d cache key = %q", index, call.PromptCacheKey)
		}
		for _, message := range call.Messages {
			if strings.Contains(message.GetText(), "same-run-b") {
				t.Fatalf("same-run call %d disclosed B catalog/body: %s", index, message.GetText())
			}
		}
	}
	if _, _, err := repo.LoadByID(sessionID, sessionProjectDir); err != nil {
		t.Fatalf("same-run durable namespace missing: %v", err)
	}
}

func TestWorktreeEnterThenAgentAndTeamInOneRunFailBeforeRetargetedSkillRead(t *testing.T) {
	rootA := t.TempDir()
	rootB := filepath.Join(t.TempDir(), "worktree-b")
	writeRootTask23Skill(t, rootA, "guard-a")
	task23RunGit(t, rootA, "init")
	task23RunGit(t, rootA, "config", "user.email", "task23@example.test")
	task23RunGit(t, rootA, "config", "user.name", "Task 23")
	task23RunGit(t, rootA, "add", ".")
	task23RunGit(t, rootA, "commit", "-m", "initial")
	task23RunGit(t, rootA, "worktree", "add", "-b", "task23-guard-b", rootB)
	if err := os.RemoveAll(filepath.Join(rootB, ".luban-code", "skills", "guard-a")); err != nil {
		t.Fatal(err)
	}
	writeRootTask23Skill(t, rootB, "guard-b-secret")

	provider := &task23WorktreeLifecycleProvider{steps: [][]types.StreamEvent{
		task23GenericToolEvents("guard-enter", "EnterWorktree", map[string]any{"path": rootB}),
		task23GenericToolEvents("guard-agent", "Agent", map[string]any{
			"description": "must stay fenced", "prompt": "inspect", "subagent_type": "task23-guard-profile",
		}),
		task23GenericToolEvents("guard-team", "TeamCreate", map[string]any{
			"team_name": "task23-guard-team", "agent_type": "executor",
		}),
		task23GenericToolEvents("guard-exit", "ExitWorktree", map[string]any{"action": "keep"}),
		task23SkillRuntimeTextEvents("done"),
	}}
	ref := providerpkgRef(provider)
	deps := SetupRegistry(ref, rootA, []string{rootA}, sandbox.NoopBackend{}, nil)
	t.Cleanup(func() {
		deps.CronStore.Stop()
		deps.StopWebFetchCache()
	})
	if err := deps.AgentTool.SetInlineProfilesFromJSON(`{"task23-guard-profile":{"description":"guard","prompt":"stay fenced","skills":["guard-b-secret"]}}`); err != nil {
		t.Fatal(err)
	}
	if err := prepareInitialRegistryRuntime(deps, rootA, []string{rootA}); err != nil {
		t.Fatal(err)
	}
	const sessionID = "task23-guard-session"
	deps.BindSessionIdentity(sessionID)
	repo := session.NewRepository(t.TempDir())
	sessionProjectDir := repo.ProjectDirForCWD(rootA)
	eng, err := engine.New(engine.Config{
		Provider: provider, ProviderRef: ref, Registry: deps.Registry,
		Sessions:    engine.NewRepositorySessionManager(repo, func() string { return sessionProjectDir }),
		ProjectRoot: rootA, CWD: rootA, AllowedDirs: []string{rootA},
		SkillManager: deps.SkillManager, SkillSessionOverrides: deps.SkillSessionOverrides, MaxTurns: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Shutdown(context.Background()) })
	cwd := rootA
	hookRunner := loadHooks(rootA)
	configureWorktreeSessionRuntime(deps, eng, &cwd, &hookRunner, "", nil)

	if err := task23RunEngineQuery(t, eng, engine.QueryRequest{
		SessionID: sessionID, SessionProjectDir: sessionProjectDir,
		Message: "enter, try children, then exit", CWD: rootA, ProjectRoot: rootA,
	}); err != nil {
		t.Fatalf("guard run: %v", err)
	}
	if got := deps.AgentTool.LoadedProfiles(); len(got) != 0 {
		t.Fatalf("stale Agent resolved profile before authority gate: %v", got)
	}
	if got := deps.TeamManager.CurrentTeamName(); got != "" {
		t.Fatalf("stale TeamCreate persisted team %q", got)
	}
	if snapshots := deps.BackgroundTasks.InMemorySnapshots(); len(snapshots) != 0 {
		t.Fatalf("stale Agent registered background tasks: %+v", snapshots)
	}
	if !sameRuntimePath(cwd, rootA) || !sameRuntimePath(deps.WorktreeRuntime.CurrentCWD(), rootA) {
		t.Fatalf("guard run did not exit to A: cwd=%q runtime=%q", cwd, deps.WorktreeRuntime.CurrentCWD())
	}
	calls := provider.Calls()
	if len(calls) != 5 {
		t.Fatalf("guard provider calls = %d, want parent-only five", len(calls))
	}
	for index, call := range calls {
		for _, message := range call.Messages {
			if strings.Contains(message.GetText(), "guard-b-secret") {
				t.Fatalf("guard call %d exposed B profile/catalog body: %s", index, message.GetText())
			}
		}
	}
}

func TestWorktreeContextSwitcherRollbackLeavesManagerAndRuntimeUnchanged(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeRootTask23Skill(t, rootA, "rollback-a")
	writeRootTask23Skill(t, rootB, "rollback-b")
	provider := &registrySetupReadProvider{name: "rollback", model: "rollback-model"}
	ref := providerpkgRef(provider)
	deps := SetupRegistry(ref, rootA, []string{rootA}, sandbox.NoopBackend{}, nil)
	t.Cleanup(func() {
		deps.CronStore.Stop()
		deps.StopWebFetchCache()
	})
	if err := prepareInitialRegistryRuntime(deps, rootA, []string{rootA}); err != nil {
		t.Fatal(err)
	}
	deps.BindSessionIdentity("rollback-session")
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(rootA)
	eng, err := engine.New(engine.Config{
		Provider: provider, ProviderRef: ref, Registry: deps.Registry,
		Sessions:    engine.NewRepositorySessionManager(repo, func() string { return projectDir }),
		ProjectRoot: rootA, CWD: rootA, SkillManager: deps.SkillManager,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Shutdown(context.Background()) })
	cwd := rootA
	hooks := loadHooks(rootA)
	configureWorktreeSessionRuntime(deps, eng, &cwd, &hooks, "", nil)
	manager := deps.SkillManager
	before, err := manager.Snapshot("rollback-session")
	if err != nil {
		t.Fatal(err)
	}
	generation := manager.ProjectGeneration()
	if err := deps.WorktreeRuntime.SwitchCWDContext(context.Background(), rootB); !errors.Is(err, engine.ErrWorkspaceRebindUnauthorized) {
		t.Fatalf("unowned worktree switch error = %v, want authorization cause", err)
	}
	after, err := manager.Snapshot("rollback-session")
	if err != nil || deps.SkillManager != manager || manager.ProjectGeneration() != generation || !sameRuntimePath(cwd, rootA) || !sameRuntimePath(deps.WorktreeRuntime.CurrentCWD(), rootA) {
		t.Fatalf("rejected switch changed runtime: after=%+v err=%v gen=%d/%d cwd=%q runtime=%q",
			after.Skills, err, manager.ProjectGeneration(), generation, cwd, deps.WorktreeRuntime.CurrentCWD())
	}
	if !equalTask23Catalog(before, after) {
		t.Fatalf("rejected switch changed catalog: before=%+v after=%+v", before.Skills, after.Skills)
	}
}

func TestPreparedWorkspaceRevisionConflictPublishesNothingAndCanRetry(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeRootTask23Skill(t, rootA, "revision-a")
	writeRootTask23Skill(t, rootB, "revision-b")
	provider := &registrySetupReadProvider{name: "revision", model: "revision-model"}
	deps := SetupRegistry(providerpkgRef(provider), rootA, []string{rootA}, sandbox.NoopBackend{}, nil)
	t.Cleanup(func() {
		deps.CronStore.Stop()
		deps.StopWebFetchCache()
	})
	if err := prepareInitialRegistryRuntime(deps, rootA, []string{rootA}); err != nil {
		t.Fatal(err)
	}
	const sessionID = "task23-revision-session"
	deps.BindSessionIdentity(sessionID)
	manager := deps.SkillManager
	generationA := manager.ProjectGeneration()
	prepared, err := deps.PrepareSessionContext(rootB)
	if err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(rootB, ".luban-code", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte("{\"unrelated\":true}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeCalls := 0
	err = deps.commitPreparedSessionRuntime(
		sessionID, rootB, []string{rootB}, "system-b", loadHooks(rootB), prepared,
		func() error { beforeCalls++; return nil },
	)
	if !errors.Is(err, skills.ErrOverrideRevisionConflict) {
		t.Fatalf("stale prepared workspace error = %v, want revision conflict", err)
	}
	if beforeCalls != 0 {
		t.Fatalf("engine callback ran before revision rejection: %d", beforeCalls)
	}
	unchanged, err := manager.Snapshot(sessionID)
	if err != nil {
		t.Fatalf("snapshot after rejected commit: %v", err)
	}
	if manager.ProjectGeneration() != generationA {
		t.Fatalf("rejected commit changed generation: %d -> %d", generationA, manager.ProjectGeneration())
	}
	if _, found := rootTask23Skill(unchanged, "revision-a"); !found {
		t.Fatalf("rejected commit lost A catalog: %+v", unchanged.Skills)
	}
	if _, found := rootTask23Skill(unchanged, "revision-b"); found {
		t.Fatalf("rejected commit exposed B catalog: %+v", unchanged.Skills)
	}
	if runtime := deps.RuntimeScope.ToolRuntimeContext(); !sameRuntimePath(runtime.ProjectRoot, rootA) {
		t.Fatalf("rejected commit changed tool runtime: %+v", runtime)
	}
	if runtime := deps.AgentTool.SessionRuntime(); !sameRuntimePath(runtime.ToolRuntime.ProjectRoot, rootA) {
		t.Fatalf("rejected commit changed Agent runtime: %+v", runtime)
	}
	if !sameRuntimePath(deps.TeamManager.CurrentCWD(), rootA) {
		t.Fatalf("rejected commit changed Team cwd: %q", deps.TeamManager.CurrentCWD())
	}

	reprepared, err := deps.PrepareSessionContext(rootB)
	if err != nil {
		t.Fatal(err)
	}
	if err := deps.commitPreparedSessionRuntime(
		sessionID, rootB, []string{rootB}, "system-b", loadHooks(rootB), reprepared,
		func() error { beforeCalls++; return nil },
	); err != nil {
		t.Fatalf("retry prepared workspace: %v", err)
	}
	if beforeCalls != 1 {
		t.Fatalf("retry engine callbacks = %d, want 1", beforeCalls)
	}
	current, err := manager.Snapshot(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := rootTask23Skill(current, "revision-b"); !found {
		t.Fatalf("retry did not publish B catalog: %+v", current.Skills)
	}
	if runtime := deps.RuntimeScope.ToolRuntimeContext(); !sameRuntimePath(runtime.ProjectRoot, rootB) {
		t.Fatalf("retry tool runtime = %+v", runtime)
	}
}

func TestTask23RegistryObserverSeesOnlyAAOrBB(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeRootTask23Skill(t, rootA, "observer-a")
	writeRootTask23Skill(t, rootB, "observer-b")
	deps := SetupRegistry(providerpkgRef(&registrySetupReadProvider{name: "observer", model: "observer-model"}), rootA, []string{rootA}, sandbox.NoopBackend{}, nil)
	t.Cleanup(func() {
		deps.CronStore.Stop()
		deps.StopWebFetchCache()
	})
	if err := prepareInitialRegistryRuntime(deps, rootA, []string{rootA}); err != nil {
		t.Fatal(err)
	}
	const sessionID = "task23-observer-session"
	deps.BindSessionIdentity(sessionID)

	type observation struct {
		root     string
		snapshot skills.CatalogSnapshot
		err      error
	}
	observe := func() observation {
		// This is the same publication barrier used by registry-owned Agent,
		// Team and RuntimeScope consumers when capturing a launch snapshot.
		deps.runtimePublishMu.RLock()
		defer deps.runtimePublishMu.RUnlock()
		runtime := deps.RuntimeScope.ToolRuntimeContextUnbarriered()
		snapshot, err := deps.SkillManager.Snapshot(sessionID)
		return observation{root: runtime.ProjectRoot, snapshot: snapshot, err: err}
	}
	assertPair := func(label string, got observation) {
		t.Helper()
		if got.err != nil {
			t.Fatalf("%s observation: %v", label, got.err)
		}
		sawA := sameRuntimePath(got.root, rootA)
		sawB := sameRuntimePath(got.root, rootB)
		hasA := false
		hasB := false
		if _, found := rootTask23Skill(got.snapshot, "observer-a"); found {
			hasA = true
		}
		if _, found := rootTask23Skill(got.snapshot, "observer-b"); found {
			hasB = true
		}
		if !(sawA && hasA && !hasB) && !(sawB && hasB && !hasA) {
			t.Fatalf("%s observed mixed runtime/catalog: root=%q skills=%+v", label, got.root, got.snapshot.Skills)
		}
	}
	assertPair("before", observe())

	prepared, err := deps.PrepareSessionContext(rootB)
	if err != nil {
		t.Fatal(err)
	}
	afterEntered := make(chan struct{})
	releaseAfter := make(chan struct{})
	commitDone := make(chan error, 1)
	go func() {
		commitDone <- deps.commitPreparedSessionRuntimeWithAfter(
			sessionID, rootB, []string{rootB}, "system-b", loadHooks(rootB), prepared, nil,
			func() {
				close(afterEntered)
				<-releaseAfter
			},
		)
	}()
	<-afterEntered

	observed := make(chan observation, 1)
	go func() { observed <- observe() }()
	select {
	case got := <-observed:
		t.Fatalf("observer crossed staged afterPublish: root=%q skills=%+v", got.root, got.snapshot.Skills)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseAfter)
	if err := <-commitDone; err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-observed:
		assertPair("after", got)
	case <-time.After(time.Second):
		t.Fatal("observer remained blocked after staged publication")
	}
	assertPair("stable after", observe())
}

func providerpkgRef(p provider.Provider) *provider.ProviderRef { return provider.NewProviderRef(p) }

func task23GenericToolEvents(id, name string, input map[string]any) []types.StreamEvent {
	encoded, _ := json.Marshal(input)
	stop := types.StopReasonToolUse
	return []types.StreamEvent{
		{Type: types.EventMessageStart, Message: &types.APIMessage{Role: types.RoleAssistant}},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeToolUse, ID: id, Name: name}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: string(encoded)}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, StopReason: &stop},
		{Type: types.EventMessageStop},
	}
}

func task23RunEngineQuery(t *testing.T, eng *engine.CoreEngine, request engine.QueryRequest) error {
	t.Helper()
	events, err := eng.Query(context.Background(), request)
	if err != nil {
		return err
	}
	var final error
	for event := range events {
		if event.Final {
			final = event.Error
		}
	}
	return final
}

func task23AssertCatalogTransition(t *testing.T, messages []types.Message, want, revoke string) {
	t.Helper()
	var latest string
	for _, message := range messages {
		if message.Role == types.RoleDeveloper && message.DeveloperMetadata != nil {
			switch message.DeveloperMetadata.Kind {
			case types.DeveloperMessageKindSkillCatalogSnapshot, types.DeveloperMessageKindSkillCatalogDelta:
				latest = message.GetText()
			}
		}
	}
	if !strings.Contains(latest, want) {
		t.Fatalf("latest catalog %q missing %q", latest, want)
	}
	if revoke != "" {
		if strings.Contains(latest, `"type":"skill_catalog_snapshot"`) {
			if strings.Contains(latest, revoke) {
				t.Fatalf("replacement snapshot %q retained old workspace %q", latest, revoke)
			}
		} else if !strings.Contains(latest, revoke) {
			t.Fatalf("catalog delta %q missing revoke %q", latest, revoke)
		}
	}
}

func task23RunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func equalTask23Catalog(left, right skills.CatalogSnapshot) bool {
	if left.Revision != right.Revision || len(left.Skills) != len(right.Skills) {
		return false
	}
	for index := range left.Skills {
		if left.Skills[index].ID != right.Skills[index].ID || left.Skills[index].Revision != right.Skills[index].Revision {
			return false
		}
	}
	return true
}
