package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/engine"
	"github.com/agent-dance/luban/goal"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/session"
	"github.com/agent-dance/luban/tools"
	"github.com/agent-dance/luban/types"
	"github.com/agent-dance/luban/ui"
)

func TestScreenReaderGoalCommandPersistsAcrossRepositoryRestart(t *testing.T) {
	configHome := t.TempDir()
	cwd := t.TempDir()
	sessionID := "screen-reader-goal-release"
	repo := session.NewRepository(configHome)
	projectDir := repo.ProjectDirForCWD(cwd)
	if err := repo.Save(sessionID, projectDir, []types.Message{types.UserMessage("start release work")}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	var setOutput bytes.Buffer
	setRenderer := ui.NewScreenReaderRenderer(&setOutput, strings.NewReader(""))
	t.Cleanup(func() { _ = setRenderer.Close() })
	cfg := TUIREPLConfig{
		Repo: repo, SessionID: &sessionID, SessionProjectDir: &projectDir, CWD: &cwd,
	}
	handled, exit, err := handleScreenReaderCommand(
		context.Background(), cfg, setRenderer, ui.NewCostTracker("test-model"),
		"/goal verify the release candidate --accept release candidate verification passes",
	)
	if err != nil || !handled || exit {
		t.Fatalf("handle /goal = handled %t exit %t err %v", handled, exit, err)
	}
	if got := setOutput.String(); !strings.Contains(got, "Goal set") || !strings.Contains(got, "AC-1") || !strings.Contains(got, "release candidate verification passes") {
		t.Fatalf("screen-reader set receipt = %q", got)
	}

	// Recreate both the repository and command renderer to exercise recovery
	// from the persisted sidecar rather than an in-memory runtime reference.
	restartedRepo := session.NewRepository(configHome)
	var statusOutput bytes.Buffer
	statusRenderer := ui.NewScreenReaderRenderer(&statusOutput, strings.NewReader(""))
	t.Cleanup(func() { _ = statusRenderer.Close() })
	restartedCfg := cfg
	restartedCfg.Repo = restartedRepo
	handled, exit, err = handleScreenReaderCommand(
		context.Background(), restartedCfg, statusRenderer, ui.NewCostTracker("test-model"), "/goal status",
	)
	if err != nil || !handled || exit {
		t.Fatalf("handle restarted /goal status = handled %t exit %t err %v", handled, exit, err)
	}
	for _, want := range []string{"Goal status: active", "Objective: verify the release candidate"} {
		if got := statusOutput.String(); !strings.Contains(got, want) {
			t.Fatalf("restarted /goal status omitted %q: %q", want, got)
		}
	}

	meta, _, err := restartedRepo.GetMeta(sessionID, projectDir)
	if err != nil {
		t.Fatalf("load restarted metadata: %v", err)
	}
	if meta.Goal == nil || meta.Goal.Objective != "verify the release candidate" || meta.Goal.Status != goal.StatusActive {
		t.Fatalf("persisted goal after restart = %+v", meta.Goal)
	}
}

type goalPermissionProvider struct {
	mu   sync.Mutex
	turn int
}

type goalActivationProvider struct {
	mu    sync.Mutex
	calls []provider.Params
}

func (*goalActivationProvider) Name() string    { return "goal-activation-test" }
func (*goalActivationProvider) ModelID() string { return "goal-activation-test-model" }

func (p *goalActivationProvider) CreateStream(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	p.calls = append(p.calls, params)
	call := len(p.calls)
	p.mu.Unlock()

	switch call {
	case 1:
		return goalPermissionStream(
			types.StreamEvent{Type: types.EventMessageStart, Message: &types.APIMessage{Role: types.RoleAssistant}},
			types.StreamEvent{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
			types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: "1+99 = 100"}},
			types.StreamEvent{Type: types.EventContentBlockStop, Index: 0},
			types.StreamEvent{Type: types.EventMessageDelta, Usage: &types.Usage{OutputTokens: 6}, StopReason: goalPermissionStopReason(types.StopReasonEndTurn)},
			types.StreamEvent{Type: types.EventMessageStop},
		), nil
	case 2:
		return goalPermissionStream(
			types.StreamEvent{Type: types.EventMessageStart, Message: &types.APIMessage{Role: types.RoleAssistant}},
			types.StreamEvent{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
			types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: `{"criteria":[{"id":"AC-1","met":true,"reason":"the answer states that 1+99 equals 100"}],"reason":"the arithmetic acceptance criterion is verified"}`}},
			types.StreamEvent{Type: types.EventContentBlockStop, Index: 0},
			types.StreamEvent{Type: types.EventMessageDelta, Usage: &types.Usage{OutputTokens: 12}, StopReason: goalPermissionStopReason(types.StopReasonEndTurn)},
			types.StreamEvent{Type: types.EventMessageStop},
		), nil
	default:
		return nil, fmt.Errorf("unexpected provider call %d", call)
	}
}

func (p *goalActivationProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

type goalActivationFixture struct {
	cfg        TUIREPLConfig
	provider   *goalActivationProvider
	repo       *session.Repository
	sessionID  string
	projectDir string
}

func newGoalActivationFixture(t *testing.T) goalActivationFixture {
	t.Helper()
	configHome := t.TempDir()
	cwd := t.TempDir()
	repo := session.NewRepository(configHome)
	sessionID := "goal-activation-session"
	projectDir := repo.ProjectDirForCWD(cwd)
	if err := repo.Save(sessionID, projectDir, nil); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	p := &goalActivationProvider{}
	eng, err := engine.New(engine.Config{
		Provider:    p,
		Registry:    registry.New(),
		Sessions:    engine.NewRepositorySessionManager(repo, func() string { return projectDir }),
		ProjectRoot: cwd,
		CWD:         cwd,
		MaxTurns:    4,
	})
	if err != nil {
		t.Fatalf("create engine: %v", err)
	}
	t.Cleanup(func() { _ = eng.Shutdown(context.Background()) })
	return goalActivationFixture{
		cfg: TUIREPLConfig{
			Engine: eng, Repo: repo, SessionID: &sessionID, SessionProjectDir: &projectDir, CWD: &cwd,
			RuntimeScope: tools.NewRuntimeScope(cwd, true), SessionTransitionMu: &sync.Mutex{}, CommandMu: &sync.Mutex{},
		},
		provider: p, repo: repo, sessionID: sessionID, projectDir: projectDir,
	}
}

func assertGoalActivationCompleted(t *testing.T, fixture goalActivationFixture) {
	t.Helper()
	if got := fixture.provider.callCount(); got != 2 {
		t.Fatalf("provider calls = %d, want main turn plus goal evaluator", got)
	}
	meta, _, err := fixture.repo.GetMeta(fixture.sessionID, fixture.projectDir)
	if err != nil {
		t.Fatalf("load goal metadata: %v", err)
	}
	if meta.Goal == nil || meta.Goal.Status != goal.StatusAchieved || meta.Goal.Objective != "计算 1+99 等于多少" {
		t.Fatalf("activated goal = %+v, want achieved arithmetic goal", meta.Goal)
	}
}

func TestScreenReaderGoalSetStartsQueryImmediately(t *testing.T) {
	fixture := newGoalActivationFixture(t)
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, strings.NewReader(""))
	t.Cleanup(func() { _ = renderer.Close() })

	handled, exit, err := handleScreenReaderCommand(
		context.Background(), fixture.cfg, renderer, ui.NewCostTracker(fixture.provider.ModelID()),
		"/goal 计算 1+99 等于多少 --accept 答案明确等于 100",
	)
	if err != nil || !handled || exit {
		t.Fatalf("handle /goal = handled %t exit %t err %v", handled, exit, err)
	}
	assertGoalActivationCompleted(t, fixture)
	if got := output.String(); !strings.Contains(got, "1+99 = 100") {
		t.Fatalf("screen-reader output omitted activated goal answer: %q", got)
	}
}

func TestBuildGoalActivationQueryRequestUsesObjectiveAsInitialPrompt(t *testing.T) {
	req, ok := buildGoalActivationQueryRequest("session-1", "  计算 1+99 等于多少  ", "/workspace/nested", "/workspace")
	if !ok {
		t.Fatal("non-empty objective did not produce a query request")
	}
	if req.SessionID != "session-1" || req.Message != "计算 1+99 等于多少" || req.CWD != "/workspace/nested" || req.ProjectRoot != "/workspace" {
		t.Fatalf("goal activation request = %+v", req)
	}
	if _, ok := buildGoalActivationQueryRequest("session-1", "  ", "/workspace", "/workspace"); ok {
		t.Fatal("empty objective produced a query request")
	}
}

func (*goalPermissionProvider) Name() string    { return "goal-permission-test" }
func (*goalPermissionProvider) ModelID() string { return "goal-permission-test-model" }

func (p *goalPermissionProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	p.turn++
	turn := p.turn
	p.mu.Unlock()

	if turn == 1 {
		return goalPermissionStream(
			types.StreamEvent{Type: types.EventMessageStart, Message: &types.APIMessage{Role: types.RoleAssistant}},
			types.StreamEvent{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{
				Type: types.ContentTypeToolUse, ID: "toolu_goal_update", Name: "UpdateGoal",
			}},
			types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{
				Type: "input_json_delta", PartialJSON: `{"status":"complete"}`,
			}},
			types.StreamEvent{Type: types.EventContentBlockStop, Index: 0},
			types.StreamEvent{Type: types.EventMessageDelta, StopReason: goalPermissionStopReason(types.StopReasonToolUse)},
			types.StreamEvent{Type: types.EventMessageStop},
		), nil
	}
	return goalPermissionStream(
		types.StreamEvent{Type: types.EventMessageStart, Message: &types.APIMessage{Role: types.RoleAssistant}},
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: "permission respected"}},
		types.StreamEvent{Type: types.EventContentBlockStop, Index: 0},
		types.StreamEvent{Type: types.EventMessageDelta, StopReason: goalPermissionStopReason(types.StopReasonEndTurn)},
		types.StreamEvent{Type: types.EventMessageStop},
	), nil
}

func goalPermissionStream(events ...types.StreamEvent) <-chan types.StreamEvent {
	stream := make(chan types.StreamEvent, len(events))
	for _, event := range events {
		stream <- event
	}
	close(stream)
	return stream
}

func goalPermissionStopReason(reason types.StopReason) *types.StopReason { return &reason }

func TestGoalWriteToolDeniedByQueryLoopLeavesSessionMetadataUnchanged(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		permission loop.PermissionHandler
	}{
		{
			name:       "plan mode",
			mode:       "plan",
			permission: engine.AsLoopPermissionHandler(engine.AllowAllHandler{}),
		},
		{
			name: "explicit deny rule",
			permission: engine.AsLoopPermissionHandler(permissions.NewCLIPermissionHandler(permissions.NewChecker(
				permissions.ModeRuleBased,
				[]permissions.Rule{{Tool: "UpdateGoal", Decision: permissions.DecisionDeny}},
			))),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configHome := t.TempDir()
			cwd := t.TempDir()
			repo := session.NewRepository(configHome)
			projectDir := repo.ProjectDirForCWD(cwd)
			const sessionID = "goal-permission-release"
			if err := repo.Save(sessionID, projectDir, []types.Message{types.UserMessage("preserve release metadata")}); err != nil {
				t.Fatalf("seed session: %v", err)
			}

			createdAt := time.Date(2026, time.July, 14, 9, 30, 0, 0, time.UTC)
			active, err := goal.Create("do not mutate this goal", 2048, createdAt)
			if err != nil {
				t.Fatalf("create seed goal: %v", err)
			}
			seed := session.SessionMeta{
				Title: "release sentinel",
				Goal:  &active,
				Usage: &session.SessionUsageMeta{InputTokens: 17, OutputTokens: 5, UsedTokens: 22, MaxTokens: 200_000},
				Presentation: &session.SessionPresentationMeta{
					InputDraft: "keep this draft", PermissionMode: "default",
				},
			}
			if err := repo.SaveMeta(sessionID, projectDir, seed); err != nil {
				t.Fatalf("seed metadata: %v", err)
			}
			before, _, err := repo.GetMeta(sessionID, projectDir)
			if err != nil {
				t.Fatalf("load metadata before query: %v", err)
			}

			runtime := newDynamicSessionGoalRuntime(
				repo,
				func() string { return sessionID },
				func() string { return projectDir },
			)
			reg := registry.New()
			reg.Register(tools.NewUpdateGoalTool(runtime))
			runtimeScope := tools.NewRuntimeScope(cwd, true)
			if test.mode != "" {
				if err := runtimeScope.TransitionPermissionMode(test.mode); err != nil {
					t.Fatalf("set runtime permission mode: %v", err)
				}
			}
			reg.SetRuntimeContextProvider(runtimeScope)

			query := loop.New(&goalPermissionProvider{}, reg, loop.Config{
				MaxTurns: 2, SessionID: sessionID, CWD: cwd, ProjectRoot: cwd,
				PermissionHandler: test.permission,
			})
			var deniedResult *types.ToolResultBlock
			if err := query.Run(context.Background(), "finish the goal", func(event loop.Event) {
				if event.Type == loop.EventToolResult && event.ToolResult != nil {
					result := *event.ToolResult
					deniedResult = &result
				}
			}); err != nil {
				t.Fatalf("run query: %v", err)
			}
			if deniedResult == nil || !deniedResult.IsError || deniedResult.Outcome != types.ToolOutcomeDenied {
				t.Fatalf("goal tool result = %+v, want denied error", deniedResult)
			}

			after, _, err := repo.GetMeta(sessionID, projectDir)
			if err != nil {
				t.Fatalf("load metadata after query: %v", err)
			}
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("denied goal write changed metadata:\nbefore: %+v\nafter:  %+v", before, after)
			}
		})
	}
}

type goalSessionNamespaceProvider struct {
	mu   sync.Mutex
	turn int
}

func (*goalSessionNamespaceProvider) Name() string    { return "goal-session-namespace-test" }
func (*goalSessionNamespaceProvider) ModelID() string { return "goal-session-namespace-model" }

func (p *goalSessionNamespaceProvider) CreateStream(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	if strings.Contains(params.JoinedSystemPrompt(), "goal acceptance evaluator") {
		return goalPermissionStream(
			types.StreamEvent{Type: types.EventMessageStart, Message: &types.APIMessage{Role: types.RoleAssistant}},
			types.StreamEvent{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
			types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: `{"criteria":[{"id":"AC-1","met":true,"reason":"legacy goal routing is verified"}],"reason":"all acceptance criteria are verified"}`}},
			types.StreamEvent{Type: types.EventContentBlockStop, Index: 0},
			types.StreamEvent{Type: types.EventMessageDelta, StopReason: goalPermissionStopReason(types.StopReasonEndTurn)},
			types.StreamEvent{Type: types.EventMessageStop},
		), nil
	}
	p.mu.Lock()
	p.turn++
	turn := p.turn
	p.mu.Unlock()

	switch turn {
	case 1:
		return goalNamespaceToolStream("goal-create", "CreateGoal", `{"objective":"finish legacy goal routing","acceptance_criteria":["legacy goal routing is verified"]}`), nil
	case 2:
		return goalNamespaceToolStream("goal-get", "GetGoal", `{}`), nil
	default:
		return goalPermissionStream(
			types.StreamEvent{Type: types.EventMessageStart, Message: &types.APIMessage{Role: types.RoleAssistant}},
			types.StreamEvent{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
			types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: "legacy goal verified"}},
			types.StreamEvent{Type: types.EventContentBlockStop, Index: 0},
			types.StreamEvent{Type: types.EventMessageDelta, StopReason: goalPermissionStopReason(types.StopReasonEndTurn)},
			types.StreamEvent{Type: types.EventMessageStop},
		), nil
	}
}

func goalNamespaceToolStream(id, name, input string) <-chan types.StreamEvent {
	return goalPermissionStream(
		types.StreamEvent{Type: types.EventMessageStart, Message: &types.APIMessage{Role: types.RoleAssistant}},
		types.StreamEvent{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{
			Type: types.ContentTypeToolUse, ID: id, Name: name,
		}},
		types.StreamEvent{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{
			Type: "input_json_delta", PartialJSON: input,
		}},
		types.StreamEvent{Type: types.EventContentBlockStop, Index: 0},
		types.StreamEvent{Type: types.EventMessageDelta, StopReason: goalPermissionStopReason(types.StopReasonToolUse)},
		types.StreamEvent{Type: types.EventMessageStop},
	)
}

func TestModelGoalToolsKeepResolvedLegacySessionNamespace(t *testing.T) {
	configHome := t.TempDir()
	workspace := t.TempDir()
	repo := session.NewRepository(configHome)
	const sessionID = "legacy-goal-tool-session"
	legacyProjectDir := repo.LegacyRoot()
	modernProjectDir := repo.ProjectDirForCWD(workspace)
	if err := repo.Save(sessionID, legacyProjectDir, []types.Message{types.UserMessage("legacy transcript")}); err != nil {
		t.Fatalf("seed legacy session: %v", err)
	}
	ref, err := repo.Resolve(sessionID, modernProjectDir)
	if err != nil {
		t.Fatalf("resolve legacy session: %v", err)
	}
	if ref.ProjectDir != legacyProjectDir {
		t.Fatalf("resolved project dir = %q, want legacy %q", ref.ProjectDir, legacyProjectDir)
	}

	runtime := newDynamicSessionGoalRuntime(
		repo,
		func() string { return sessionID },
		func() string { return ref.ProjectDir },
	)
	reg := registry.New()
	reg.Register(tools.NewCreateGoalTool(runtime))
	reg.Register(tools.NewUpdateGoalTool(runtime))
	reg.Register(tools.NewGetGoalTool(runtime))
	provider := &goalSessionNamespaceProvider{}
	eng, err := engine.New(engine.Config{
		Provider:    provider,
		Registry:    reg,
		Sessions:    engine.NewRepositorySessionManager(repo, func() string { return ref.ProjectDir }),
		ProjectRoot: workspace,
		CWD:         workspace,
		MaxTurns:    6,
	})
	if err != nil {
		t.Fatalf("create engine: %v", err)
	}
	t.Cleanup(func() { _ = eng.Shutdown(context.Background()) })

	prepared, err := eng.PrepareRuntimeContextResume(context.Background(), sessionID, ref.ProjectDir, engine.RuntimeContext{
		ProjectRoot: workspace,
		CWD:         workspace,
	})
	if err != nil {
		t.Fatalf("prepare legacy resume: %v", err)
	}
	if err := prepared.Commit(); err != nil {
		t.Fatalf("commit legacy resume: %v", err)
	}
	events, err := eng.Query(context.Background(), engine.QueryRequest{SessionID: sessionID, Message: "manage the goal"})
	if err != nil {
		t.Fatalf("query legacy session: %v", err)
	}
	var getResult string
	for event := range events {
		if event.Final && event.Error != nil {
			t.Fatalf("query final error: %v", event.Error)
		}
		if event.Inner.Type == loop.EventToolResult && event.Inner.ToolResult != nil && event.Inner.ToolResult.ToolUseID == "goal-get" {
			getResult = event.Inner.ToolResult.TextContent()
		}
	}

	meta, gotRef, err := repo.GetMeta(sessionID, legacyProjectDir)
	if err != nil {
		t.Fatalf("load legacy metadata: %v", err)
	}
	if gotRef.ProjectDir != legacyProjectDir || meta.Goal == nil || meta.Goal.Objective != "finish legacy goal routing" || meta.Goal.Status != goal.StatusAchieved {
		t.Fatalf("legacy goal metadata = ref %+v goal %+v", gotRef, meta.Goal)
	}
	if !strings.Contains(getResult, "finish legacy goal routing") || !strings.Contains(getResult, string(goal.StatusActive)) || !strings.Contains(getResult, "AC-1") {
		t.Fatalf("GetGoal result = %q, want active legacy goal with acceptance criteria", getResult)
	}
	if _, err := repo.StoreForProjectDir(modernProjectDir).Load(sessionID); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("modern namespace gained same-ID transcript: %v", err)
	}
}
