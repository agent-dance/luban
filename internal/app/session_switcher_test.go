package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/hooks"
	agentruntime "github.com/agent-dance/luban/internal/agent"
	"github.com/agent-dance/luban/internal/runtime/engine"
	runtimescope "github.com/agent-dance/luban/internal/runtime/scope"
	"github.com/agent-dance/luban/internal/store/session"
	toolcollaboration "github.com/agent-dance/luban/internal/tools/collaboration"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type sessionSwitcherTestSessions struct {
	messages map[string][]types.Message
	loads    []string
	saved    []sessionSwitcherSavedContext
}

type sessionSwitcherSavedContext struct {
	sessionID string
	cwd       string
	gitBranch string
}

func (s *sessionSwitcherTestSessions) Save(sessionID string, messages []types.Message) error {
	if s.messages == nil {
		s.messages = make(map[string][]types.Message)
	}
	s.messages[sessionID] = append([]types.Message(nil), messages...)
	return nil
}

func (s *sessionSwitcherTestSessions) Load(sessionID string) ([]types.Message, error) {
	s.loads = append(s.loads, sessionID)
	messages, ok := s.messages[sessionID]
	if !ok {
		return nil, errors.New("session transcript not found")
	}
	return append([]types.Message(nil), messages...), nil
}

func (s *sessionSwitcherTestSessions) List() ([]engine.SessionInfo, error) { return nil, nil }
func (s *sessionSwitcherTestSessions) Latest() (string, error)             { return "", nil }
func (s *sessionSwitcherTestSessions) Delete(sessionID string) error {
	delete(s.messages, sessionID)
	return nil
}

func (s *sessionSwitcherTestSessions) SaveSessionContext(sessionID, cwd, gitBranch string) error {
	s.saved = append(s.saved, sessionSwitcherSavedContext{
		sessionID: sessionID,
		cwd:       cwd,
		gitBranch: gitBranch,
	})
	return nil
}

type sessionSwitcherTestEngine struct {
	engine.Engine
	sessions       *sessionSwitcherTestSessions
	resumeErr      error
	resumeIDs      []string
	transcript     []types.Message
	runtime        engine.RuntimeContext
	runtimeUpdates []engine.RuntimeContext
	resumeStarted  chan struct{}
	resumeRelease  chan struct{}
	resumeProjects []string
	stagedRuntime  engine.RuntimeContext
	afterPrepare   func()
	commits        int
	commitErr      error
	commitStarted  chan struct{}
	commitRelease  chan struct{}
}

type sessionSwitcherPreparedResume struct {
	engine    *sessionSwitcherTestEngine
	messages  []types.Message
	completed bool
	mu        sync.Mutex
}

func (p *sessionSwitcherPreparedResume) MessageCount() int { return len(p.messages) }

func (p *sessionSwitcherPreparedResume) Commit() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.completed {
		return errors.New("prepared resume already completed")
	}
	if p.engine.commitErr != nil {
		return p.engine.commitErr
	}
	p.engine.transcript = append([]types.Message(nil), p.messages...)
	p.engine.commits++
	p.completed = true
	return nil
}

func (p *sessionSwitcherPreparedResume) CommitContext(ctx context.Context) error {
	if p.engine.commitStarted != nil {
		select {
		case p.engine.commitStarted <- struct{}{}:
		default:
		}
	}
	if p.engine.commitRelease != nil {
		select {
		case <-p.engine.commitRelease:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return p.Commit()
}

func (p *sessionSwitcherPreparedResume) Abort() {
	p.mu.Lock()
	p.completed = true
	p.mu.Unlock()
}

func (e *sessionSwitcherTestEngine) Resume(_ context.Context, sessionID string) (int, error) {
	return e.resume(sessionID)
}

func (e *sessionSwitcherTestEngine) ResumeWithRuntimeContext(ctx context.Context, sessionID, projectDir string, runtime engine.RuntimeContext) (int, error) {
	prepared, err := e.PrepareRuntimeContextResume(ctx, sessionID, projectDir, runtime)
	if err != nil {
		return 0, err
	}
	if err := prepared.Commit(); err != nil {
		return 0, err
	}
	return prepared.MessageCount(), nil
}

func (e *sessionSwitcherTestEngine) PrepareRuntimeContextResume(ctx context.Context, sessionID, projectDir string, runtime engine.RuntimeContext) (engine.PreparedRuntimeContextResume, error) {
	e.resumeProjects = append(e.resumeProjects, projectDir)
	e.stagedRuntime = runtime
	if e.resumeStarted != nil {
		e.resumeStarted <- struct{}{}
	}
	if e.resumeRelease != nil {
		select {
		case <-e.resumeRelease:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	e.resumeIDs = append(e.resumeIDs, sessionID)
	if e.resumeErr != nil {
		return nil, e.resumeErr
	}
	messages, err := e.sessions.Load(sessionID)
	if err != nil {
		return nil, err
	}
	if e.afterPrepare != nil {
		e.afterPrepare()
	}
	return &sessionSwitcherPreparedResume{engine: e, messages: messages}, nil
}

func (e *sessionSwitcherTestEngine) PrepareResume(ctx context.Context, sessionID string) (engine.PreparedRuntimeContextResume, error) {
	return e.PrepareRuntimeContextResume(ctx, sessionID, "", e.runtime)
}

func (e *sessionSwitcherTestEngine) resume(sessionID string) (int, error) {
	e.resumeIDs = append(e.resumeIDs, sessionID)
	if e.resumeErr != nil {
		return 0, e.resumeErr
	}
	messages, err := e.sessions.Load(sessionID)
	if err != nil {
		return 0, err
	}
	e.transcript = messages
	return len(messages), nil
}

func (e *sessionSwitcherTestEngine) Sessions() engine.SessionManager { return e.sessions }

func (e *sessionSwitcherTestEngine) UpdateRuntimeContext(runtime engine.RuntimeContext) {
	e.runtime = runtime
	e.runtimeUpdates = append(e.runtimeUpdates, runtime)
}

type sessionSwitcherTestFixture struct {
	switcher       *sessionSwitcher
	engine         *sessionSwitcherTestEngine
	deps           *RegistryDeps
	sessionID      string
	projectDir     string
	cwd            string
	hookRunner     *hooks.Runner
	prevRuntime    engine.RuntimeContext
	prevProcessCWD string
}

func TestAllowedDirsForSessionDefaultsToUnrestricted(t *testing.T) {
	cwd := filepath.Join(string(filepath.Separator), "workspace")
	if got := allowedDirsForSession(cwd, nil); got != nil {
		t.Fatalf("default allowed dirs = %v, want nil (unrestricted)", got)
	}

	extra := filepath.Join(string(filepath.Separator), "shared")
	if got, want := allowedDirsForSession(cwd, []string{extra}), []string{cwd, extra}; !reflect.DeepEqual(got, want) {
		t.Fatalf("explicit allowed dirs = %v, want %v", got, want)
	}
}

func TestBuildWorkspacePromptPreservesCacheBoundaryAndInstructions(t *testing.T) {
	t.Setenv("LUBAN_CODE_SIMPLE", "")
	cwd := t.TempDir()
	const instruction = "workspace instruction sentinel"
	if err := os.WriteFile(filepath.Join(cwd, "AGENTS.md"), []byte(instruction), 0o600); err != nil {
		t.Fatal(err)
	}

	got := buildWorkspacePrompt("", registry.New(), cwd)
	if len(got.systemBlocks) != 2 {
		t.Fatalf("system blocks = %d, want static and dynamic blocks", len(got.systemBlocks))
	}
	if !got.systemBlocks[0].Cache || got.systemBlocks[1].Cache {
		t.Fatalf("cache boundary lost: %#v", got.systemBlocks)
	}
	if got.system != got.systemBlocks.JoinedText() {
		t.Fatalf("joined system prompt differs from typed blocks")
	}
	if strings.Contains(got.system, instruction) {
		t.Fatal("workspace instructions leaked into the system envelope")
	}
	meta, ok := got.userContext.MetaMessage()
	if !ok || !strings.Contains(meta.GetText(), instruction) {
		t.Fatalf("workspace instructions missing from user context: %#v", got.userContext)
	}
}

func newSessionSwitcherTestFixture(t *testing.T) *sessionSwitcherTestFixture {
	t.Helper()

	prevProcessCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get process cwd: %v", err)
	}
	prevCWD := t.TempDir()
	if err := os.Chdir(prevCWD); err != nil {
		t.Fatalf("enter previous cwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(prevProcessCWD); err != nil {
			t.Errorf("restore process cwd: %v", err)
		}
	})

	prevSessionID := "previous-session"
	prevProjectDir := filepath.Join(t.TempDir(), "previous-project")
	prevHooks := hooks.NewRunner([]hooks.Hook{{Type: hooks.HookPostToolUse, Command: "previous-hook"}})
	runtimeScope := runtimescope.NewRuntimeScope(prevCWD, true)
	runtimeScope.SetAllowedDirs([]string{prevCWD})
	agentTool := &agentruntime.AgentTool{System: "stable-system", HookRunner: prevHooks}
	teamManager := toolcollaboration.NewTeamManager(nil)
	deps := &RegistryDeps{
		Registry:     registry.New(),
		AgentTool:    agentTool,
		TeamManager:  teamManager,
		RuntimeScope: runtimeScope,
		SkillManager: newRegistryTestSkillManager(t, prevCWD),
	}
	prevRuntime := engine.RuntimeContext{
		SystemPrompt: "stable-system",
		HookRunner:   prevHooks,
		CWD:          prevCWD,
	}
	testEngine := &sessionSwitcherTestEngine{
		sessions: &sessionSwitcherTestSessions{messages: make(map[string][]types.Message)},
		runtime:  prevRuntime,
	}
	switcher := &sessionSwitcher{
		repo:                 session.NewRepository(t.TempDir()),
		deps:                 deps,
		eng:                  testEngine,
		sessionID:            &prevSessionID,
		sessionProjectDir:    &prevProjectDir,
		cwd:                  &prevCWD,
		hookRunnerRef:        &prevHooks,
		systemPromptOverride: "stable-system",
	}

	return &sessionSwitcherTestFixture{
		switcher:       switcher,
		engine:         testEngine,
		deps:           deps,
		sessionID:      prevSessionID,
		projectDir:     prevProjectDir,
		cwd:            prevCWD,
		hookRunner:     prevHooks,
		prevRuntime:    prevRuntime,
		prevProcessCWD: prevProcessCWD,
	}
}

func TestSessionSwitcherSwitchToLoadsTranscriptAndAppliesTargetRuntime(t *testing.T) {
	fixture := newSessionSwitcherTestFixture(t)
	targetCWD := t.TempDir()
	const targetInstruction = "target workspace instruction sentinel"
	if err := os.WriteFile(filepath.Join(targetCWD, "AGENTS.md"), []byte(targetInstruction), 0o600); err != nil {
		t.Fatal(err)
	}
	targetProjectDir := filepath.Join(t.TempDir(), "target-project")
	targetTranscript := []types.Message{{ID: "target-message", Role: types.RoleAssistant}}
	fixture.engine.sessions.messages["target-session"] = targetTranscript
	writeSessionSwitcherHookSettings(t, targetCWD)

	err := fixture.switcher.switchTo(context.Background(), commands.SessionListEntry{
		ID:         "target-session",
		ProjectDir: targetProjectDir,
		CWD:        targetCWD,
	})
	if err != nil {
		t.Fatalf("switch session: %v", err)
	}

	if !reflect.DeepEqual(fixture.engine.transcript, targetTranscript) {
		t.Fatalf("engine transcript = %#v, want %#v", fixture.engine.transcript, targetTranscript)
	}
	if !reflect.DeepEqual(fixture.engine.resumeIDs, []string{"target-session"}) {
		t.Fatalf("resume IDs = %v, want [target-session]", fixture.engine.resumeIDs)
	}
	if *fixture.switcher.sessionID != "target-session" || *fixture.switcher.sessionProjectDir != targetProjectDir || *fixture.switcher.cwd != targetCWD {
		t.Fatalf("session identity not switched coherently: id=%q project=%q cwd=%q",
			*fixture.switcher.sessionID, *fixture.switcher.sessionProjectDir, *fixture.switcher.cwd)
	}
	processCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get switched process cwd: %v", err)
	}
	assertSessionSwitcherSameDir(t, processCWD, fixture.cwd)
	targetHooks := *fixture.switcher.hookRunnerRef
	if targetHooks == nil || targetHooks == fixture.hookRunner || !targetHooks.HasHooks(hooks.HookPreToolUse) {
		t.Fatalf("target hooks not loaded: target=%p previous=%p", targetHooks, fixture.hookRunner)
	}
	agentRuntime := fixture.deps.AgentTool.SessionRuntime()
	if agentRuntime.HookRunner != targetHooks {
		t.Fatalf("registry hook consumer disagrees: ref=%p agent=%p",
			targetHooks, agentRuntime.HookRunner)
	}
	runtime := fixture.engine.runtime
	if runtime.CWD != targetCWD || runtime.HookRunner != targetHooks {
		t.Fatalf("engine runtime not switched coherently: %+v", runtime)
	}
	if len(runtime.SystemPromptBlocks) != 1 || runtime.SystemPromptBlocks[0].Source != "override" {
		t.Fatalf("typed override prompt not preserved: %#v", runtime.SystemPromptBlocks)
	}
	meta, ok := runtime.UserContext.MetaMessage()
	if !ok || !strings.Contains(meta.GetText(), targetInstruction) {
		t.Fatalf("target instructions missing from switched runtime: %#v", runtime.UserContext)
	}
	scope := fixture.deps.RuntimeScope.ToolRuntimeContext()
	if scope.ProjectRoot != targetCWD || scope.AllowedDirs != nil {
		t.Fatalf("registry runtime scope not switched coherently: %+v", scope)
	}
	if agentRuntime.System != "stable-system" || runtime.SystemPrompt != "stable-system" {
		t.Fatalf("system prompt consumers disagree: agent=%q engine=%q",
			agentRuntime.System, runtime.SystemPrompt)
	}
	if agentRuntime.ToolRuntime.SessionID != "target-session" || agentRuntime.ToolRuntime.ProjectRoot != targetCWD {
		t.Fatalf("session runtime tuple disagrees: agent=%+v", agentRuntime)
	}
	if len(fixture.engine.runtimeUpdates) != 1 || fixture.engine.runtimeUpdates[0].CWD != targetCWD {
		t.Fatalf("public runtime updates = %+v, want one target publish", fixture.engine.runtimeUpdates)
	}
	if !reflect.DeepEqual(fixture.engine.resumeProjects, []string{targetProjectDir}) {
		t.Fatalf("staged resume projects = %v, want [%s]", fixture.engine.resumeProjects, targetProjectDir)
	}
	if len(fixture.engine.sessions.saved) != 1 {
		t.Fatalf("saved session contexts = %v, want one target context", fixture.engine.sessions.saved)
	}
	saved := fixture.engine.sessions.saved[0]
	if saved.sessionID != "target-session" || saved.cwd != targetCWD {
		t.Fatalf("saved session context = %+v, want target session/cwd", saved)
	}
}

func TestSessionSwitcherSwitchToResumeFailureKeepsOldRuntimeWithoutTouchingCoordinatorProjection(t *testing.T) {
	fixture := newSessionSwitcherTestFixture(t)
	targetCWD := t.TempDir()
	targetProjectDir := filepath.Join(t.TempDir(), "target-project")
	resumeErr := errors.New("resume failed")
	fixture.engine.resumeErr = resumeErr
	writeSessionSwitcherHookSettings(t, targetCWD)

	// The TUI coordinator owns presentation projection; sessionSwitcher must not
	// mutate it while applying or rolling back engine and workspace state.
	coordinatorProjection := []string{"previous visible transcript"}
	err := fixture.switcher.switchTo(context.Background(), commands.SessionListEntry{
		ID:         "target-session",
		ProjectDir: targetProjectDir,
		CWD:        targetCWD,
	})
	if !errors.Is(err, resumeErr) {
		t.Fatalf("switch error = %v, want %v", err, resumeErr)
	}
	if !reflect.DeepEqual(coordinatorProjection, []string{"previous visible transcript"}) {
		t.Fatalf("coordinator-owned projection mutated: %v", coordinatorProjection)
	}

	assertSessionSwitcherPreviousState(t, fixture)
	if len(fixture.engine.runtimeUpdates) != 0 {
		t.Fatalf("failed resume published runtime updates: %+v", fixture.engine.runtimeUpdates)
	}
	if len(fixture.engine.sessions.saved) != 0 {
		t.Fatalf("failed switch persisted target context: %v", fixture.engine.sessions.saved)
	}
}

func TestSessionSwitcherBlockedResumeKeepsOldContextVisible(t *testing.T) {
	fixture := newSessionSwitcherTestFixture(t)
	targetCWD := t.TempDir()
	targetProjectDir := filepath.Join(t.TempDir(), "target-project")
	fixture.engine.sessions.messages["target-session"] = []types.Message{{ID: "target-message", Role: types.RoleAssistant}}
	fixture.engine.resumeStarted = make(chan struct{}, 1)
	fixture.engine.resumeRelease = make(chan struct{})
	writeSessionSwitcherHookSettings(t, targetCWD)

	done := make(chan error, 1)
	go func() {
		done <- fixture.switcher.switchTo(context.Background(), commands.SessionListEntry{
			ID: "target-session", ProjectDir: targetProjectDir, CWD: targetCWD,
		})
	}()
	<-fixture.engine.resumeStarted

	assertSessionSwitcherPreviousState(t, fixture)
	if got := fixture.deps.CurrentSessionID(); got != fixture.sessionID {
		t.Fatalf("tool session ID while resume blocked = %q, want %q", got, fixture.sessionID)
	}
	if len(fixture.engine.runtimeUpdates) != 0 {
		t.Fatalf("blocked resume published runtime: %+v", fixture.engine.runtimeUpdates)
	}

	close(fixture.engine.resumeRelease)
	if err := <-done; err != nil {
		t.Fatalf("switch after releasing resume: %v", err)
	}
	if got := fixture.deps.CurrentSessionID(); got != "target-session" {
		t.Fatalf("tool session ID after resume = %q, want target-session", got)
	}
}

func TestSessionSwitcherConcurrentContextReadersAreRaceFree(t *testing.T) {
	fixture := newSessionSwitcherTestFixture(t)
	targetCWD := t.TempDir()
	fixture.engine.sessions.messages["target-session"] = []types.Message{{ID: "target-message", Role: types.RoleAssistant}}
	fixture.engine.resumeStarted = make(chan struct{}, 1)
	fixture.engine.resumeRelease = make(chan struct{})
	writeSessionSwitcherHookSettings(t, targetCWD)

	done := make(chan error, 1)
	go func() {
		done <- fixture.switcher.switchTo(context.Background(), commands.SessionListEntry{
			ID: "target-session", ProjectDir: filepath.Join(t.TempDir(), "target-project"), CWD: targetCWD,
		})
	}()
	<-fixture.engine.resumeStarted

	stop := make(chan struct{})
	var readers sync.WaitGroup
	for i := 0; i < 8; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = fixture.deps.CurrentSessionID()
					_ = fixture.deps.AgentTool.SessionRuntime()
					_ = fixture.deps.RuntimeScope.ToolRuntimeContext()
				}
			}
		}()
	}
	close(fixture.engine.resumeRelease)
	if err := <-done; err != nil {
		t.Fatalf("switch session: %v", err)
	}
	close(stop)
	readers.Wait()
}

func TestRegistrySessionPublicationBarrierHidesPartialRuntime(t *testing.T) {
	fixture := newSessionSwitcherTestFixture(t)
	fixture.deps.BindSessionIdentity(fixture.sessionID)
	targetCWD := t.TempDir()
	targetHooks := hooks.NewRunner([]hooks.Hook{{Type: hooks.HookPreToolUse, Command: "target"}})

	fixture.deps.runtimePublishMu.Lock()
	ready := make(chan struct{}, 5)
	results := make(chan any, 5)
	go func() { ready <- struct{}{}; results <- fixture.deps.CurrentSessionID() }()
	go func() { ready <- struct{}{}; results <- fixture.deps.AgentTool.SessionRuntime() }()
	go func() { ready <- struct{}{}; results <- fixture.deps.RuntimeScope.ToolRuntimeContext() }()
	go func() { ready <- struct{}{}; results <- fixture.deps.RuntimeScope.SessionID() }()
	go func() { ready <- struct{}{}; results <- fixture.deps.RuntimeScope.TaskListID() }()
	for range 5 {
		<-ready
	}
	select {
	case result := <-results:
		fixture.deps.runtimePublishMu.Unlock()
		t.Fatalf("session reader crossed an in-progress publication: %#v", result)
	case <-time.After(20 * time.Millisecond):
	}
	fixture.deps.sessionMu.Lock()
	fixture.deps.activeSessionID = "target-session"
	fixture.deps.sessionMu.Unlock()
	prepared, err := fixture.deps.PrepareSessionContext(targetCWD)
	if err != nil {
		fixture.deps.runtimePublishMu.Unlock()
		t.Fatalf("prepare target registry context: %v", err)
	}
	fixture.deps.updateSessionContext(targetCWD, []string{targetCWD}, prepared)
	fixture.deps.AgentTool.SetSessionRuntime(agentruntime.AgentSessionRuntime{System: "target-system", HookRunner: targetHooks})
	fixture.deps.runtimePublishMu.Unlock()

	for range 5 {
		select {
		case <-results:
		case <-time.After(time.Second):
			t.Fatal("session reader did not resume after publication")
		}
	}
	if got := fixture.deps.CurrentSessionID(); got != "target-session" {
		t.Fatalf("published session ID = %q", got)
	}
	if runtime := fixture.deps.AgentTool.SessionRuntime(); runtime.System != "target-system" || runtime.HookRunner != targetHooks {
		t.Fatalf("published agent runtime = %+v", runtime)
	}
	if runtime := fixture.deps.RuntimeScope.ToolRuntimeContext(); runtime.ProjectRoot != targetCWD || !reflect.DeepEqual(runtime.AllowedDirs, []string{targetCWD}) {
		t.Fatalf("published tool runtime = %+v", runtime)
	}
}

func TestSessionSwitcherSwitchToBadTargetCWDMakesNoMutation(t *testing.T) {
	fixture := newSessionSwitcherTestFixture(t)
	badTargetCWD := filepath.Join(t.TempDir(), "missing")

	err := fixture.switcher.switchTo(context.Background(), commands.SessionListEntry{
		ID:         "target-session",
		ProjectDir: filepath.Join(t.TempDir(), "target-project"),
		CWD:        badTargetCWD,
	})
	if err == nil {
		t.Fatal("switch with missing cwd succeeded")
	}

	assertSessionSwitcherPreviousState(t, fixture)
	if len(fixture.engine.resumeIDs) != 0 {
		t.Fatalf("Resume called for invalid cwd: %v", fixture.engine.resumeIDs)
	}
	if len(fixture.engine.runtimeUpdates) != 0 {
		t.Fatalf("runtime mutated for invalid cwd: %+v", fixture.engine.runtimeUpdates)
	}
	if len(fixture.engine.sessions.saved) != 0 {
		t.Fatalf("invalid cwd persisted session context: %v", fixture.engine.sessions.saved)
	}
}

func TestSessionSwitcherTargetCWDDisappearsAfterPrepareWithoutCommittingConversation(t *testing.T) {
	fixture := newSessionSwitcherTestFixture(t)
	targetCWD := t.TempDir()
	fixture.engine.sessions.messages["target-session"] = []types.Message{{ID: "target-message", Role: types.RoleAssistant}}
	fixture.engine.afterPrepare = func() {
		if err := os.RemoveAll(targetCWD); err != nil {
			t.Fatalf("remove target cwd: %v", err)
		}
	}

	err := fixture.switcher.switchTo(context.Background(), commands.SessionListEntry{
		ID: "target-session", ProjectDir: filepath.Join(t.TempDir(), "target-project"), CWD: targetCWD,
	})
	if err == nil {
		t.Fatal("switch succeeded after target cwd disappeared")
	}
	assertSessionSwitcherPreviousState(t, fixture)
	if fixture.engine.commits != 0 || fixture.engine.transcript != nil {
		t.Fatalf("detached conversation was published: commits=%d transcript=%#v", fixture.engine.commits, fixture.engine.transcript)
	}
}

func TestSessionSwitcherCancellationAfterPrepareDoesNotCommitTarget(t *testing.T) {
	fixture := newSessionSwitcherTestFixture(t)
	targetCWD := t.TempDir()
	fixture.engine.sessions.messages["target-session"] = []types.Message{{ID: "target-message", Role: types.RoleAssistant}}
	ctx, cancel := context.WithCancel(context.Background())
	fixture.engine.afterPrepare = cancel

	err := fixture.switcher.switchTo(ctx, commands.SessionListEntry{
		ID: "target-session", ProjectDir: filepath.Join(t.TempDir(), "target-project"), CWD: targetCWD,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("switch error = %v, want context canceled", err)
	}
	assertSessionSwitcherPreviousState(t, fixture)
	if fixture.engine.commits != 0 || fixture.engine.transcript != nil {
		t.Fatalf("cancelled prepare committed target: commits=%d transcript=%#v", fixture.engine.commits, fixture.engine.transcript)
	}
}

func TestSessionSwitcherCancellationAtCommitBoundaryDoesNotPublishTarget(t *testing.T) {
	fixture := newSessionSwitcherTestFixture(t)
	targetCWD := t.TempDir()
	fixture.engine.sessions.messages["target-session"] = []types.Message{{ID: "target-message", Role: types.RoleAssistant}}
	fixture.engine.commitStarted = make(chan struct{}, 1)
	fixture.engine.commitRelease = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- fixture.switcher.switchTo(ctx, commands.SessionListEntry{
			ID: "target-session", ProjectDir: filepath.Join(t.TempDir(), "target-project"), CWD: targetCWD,
		})
	}()
	<-fixture.engine.commitStarted
	cancel()

	err := <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("switch error = %v, want context canceled", err)
	}
	assertSessionSwitcherPreviousState(t, fixture)
	if fixture.engine.commits != 0 || fixture.engine.transcript != nil {
		t.Fatalf("cancelled commit boundary published target: commits=%d transcript=%#v", fixture.engine.commits, fixture.engine.transcript)
	}
}

func TestSessionSwitcherRejectsMalformedTargetHookSettingsBeforeResume(t *testing.T) {
	fixture := newSessionSwitcherTestFixture(t)
	targetCWD := t.TempDir()
	fixture.engine.sessions.messages["target-session"] = []types.Message{{ID: "target-message", Role: types.RoleAssistant}}
	settingsDir := filepath.Join(targetCWD, ".luban-code")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatalf("create settings dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(settingsDir, "settings.json"), []byte(`{"hooks":`), 0o600); err != nil {
		t.Fatalf("write malformed hook settings: %v", err)
	}

	err := fixture.switcher.switchTo(context.Background(), commands.SessionListEntry{
		ID: "target-session", ProjectDir: filepath.Join(t.TempDir(), "target-project"), CWD: targetCWD,
	})
	if err == nil || !strings.Contains(err.Error(), "hook") {
		t.Fatalf("switch error = %v, want traceable hook settings error", err)
	}
	assertSessionSwitcherPreviousState(t, fixture)
	if len(fixture.engine.resumeIDs) != 0 || fixture.engine.commits != 0 {
		t.Fatalf("malformed hooks reached resume: resumes=%v commits=%d", fixture.engine.resumeIDs, fixture.engine.commits)
	}
}

func assertSessionSwitcherPreviousState(t *testing.T, fixture *sessionSwitcherTestFixture) {
	t.Helper()
	if *fixture.switcher.sessionID != fixture.sessionID || *fixture.switcher.sessionProjectDir != fixture.projectDir || *fixture.switcher.cwd != fixture.cwd {
		t.Fatalf("session identity changed: id=%q project=%q cwd=%q",
			*fixture.switcher.sessionID, *fixture.switcher.sessionProjectDir, *fixture.switcher.cwd)
	}
	processCWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get process cwd: %v", err)
	}
	assertSessionSwitcherSameDir(t, processCWD, fixture.cwd)
	agentRuntime := fixture.deps.AgentTool.SessionRuntime()
	if *fixture.switcher.hookRunnerRef != fixture.hookRunner || agentRuntime.HookRunner != fixture.hookRunner {
		t.Fatalf("hooks not restored: ref=%p agent=%p want=%p",
			*fixture.switcher.hookRunnerRef, agentRuntime.HookRunner, fixture.hookRunner)
	}
	if agentRuntime.System != "stable-system" {
		t.Fatalf("system prompt not restored: agent=%q", agentRuntime.System)
	}
	if !reflect.DeepEqual(fixture.engine.runtime, fixture.prevRuntime) {
		t.Fatalf("engine runtime = %+v, want previous %+v", fixture.engine.runtime, fixture.prevRuntime)
	}
	scope := fixture.deps.RuntimeScope.ToolRuntimeContext()
	if scope.ProjectRoot != fixture.cwd || !reflect.DeepEqual(scope.AllowedDirs, []string{fixture.cwd}) {
		t.Fatalf("registry runtime scope = %+v, want previous cwd %q", scope, fixture.cwd)
	}
}

func assertSessionSwitcherSameDir(t *testing.T, got, want string) {
	t.Helper()
	gotInfo, err := os.Stat(got)
	if err != nil {
		t.Fatalf("stat directory %q: %v", got, err)
	}
	wantInfo, err := os.Stat(want)
	if err != nil {
		t.Fatalf("stat expected directory %q: %v", want, err)
	}
	if !os.SameFile(gotInfo, wantInfo) {
		t.Fatalf("directory = %q, want %q", got, want)
	}
}

func writeSessionSwitcherHookSettings(t *testing.T, cwd string) {
	t.Helper()
	settingsDir := filepath.Join(cwd, ".luban-code")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatalf("create hook settings directory: %v", err)
	}
	settings := []byte(`{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"target-hook"}]}]}}`)
	if err := os.WriteFile(filepath.Join(settingsDir, "settings.json"), settings, 0o600); err != nil {
		t.Fatalf("write hook settings: %v", err)
	}
}
