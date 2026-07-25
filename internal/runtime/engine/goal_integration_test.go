package engine

import (
	"context"
	"errors"
	"fmt"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"io/fs"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/runtime/goal"
	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

func TestRepositoryGoalSaveDoesNotCreateMissingSessionHistory(t *testing.T) {
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD(t.TempDir())
	manager := newRepositorySessionManager(repo, func() string { return projectDir })
	state, err := goal.CreateWithCriteria("release goal", []string{"verified"}, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if err := manager.saveGoalToProject("missing-session", projectDir, state); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("save goal to missing session error = %v, want fs.ErrNotExist", err)
	}
	if _, err := repo.StoreForProjectDir(projectDir).GetCompactionManifest("missing-session"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("goal sidecar created session history: %v", err)
	}
}

type goalIntegrationProvider struct {
	mu        sync.Mutex
	responses []string
	calls     []provider.Params
}

func (*goalIntegrationProvider) Name() string    { return "goal-integration" }
func (*goalIntegrationProvider) ModelID() string { return "goal-integration-model" }

func (p *goalIntegrationProvider) CreateStream(_ context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.calls = append(p.calls, params)
	index := len(p.calls) - 1
	if index >= len(p.responses) {
		return nil, fmt.Errorf("unexpected provider call %d", index+1)
	}
	if isGoalEvaluatorRequest(params) {
		return goalIntegrationTextStreamWithUsage(p.responses[index], &types.Usage{InputTokens: 7, OutputTokens: 3}), nil
	}
	return goalIntegrationTextStream(p.responses[index]), nil
}

func (p *goalIntegrationProvider) recordedCalls() []provider.Params {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]provider.Params(nil), p.calls...)
}

func TestCoreEngineAutomaticallyContinuesPersistedActiveGoal(t *testing.T) {
	repo, projectDir, sessionID, initial := prepareEngineGoalSession(t, goal.StatusActive)
	p := &goalIntegrationProvider{responses: []string{
		"implementation is drafted",
		`{"criteria":[{"id":"AC-1","met":false,"reason":"verification is still missing"}],"reason":"verification is still missing"}`,
		"verification now passes",
		`{"criteria":[{"id":"AC-1","met":true,"reason":"the transcript proves the criterion"}],"reason":"the transcript proves every acceptance criterion"}`,
	}}
	eng, err := New(Config{
		Provider:    p,
		Sessions:    NewRepositorySessionManager(repo, func() string { return projectDir }),
		ProjectRoot: "/workspace/goal-integration",
		CWD:         "/workspace/goal-integration",
		MaxTurns:    8,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	events, err := eng.Query(context.Background(), QueryRequest{
		SessionID: sessionID,
		Message:   "complete the persisted objective",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	assertGoalIntegrationQuerySucceeded(t, drainEvents(t, events, 5*time.Second))

	calls := p.recordedCalls()
	if len(calls) != 4 {
		t.Fatalf("provider calls = %d, want 2 conversation + 2 evaluator calls", len(calls))
	}
	wantKinds := []string{"conversation", "evaluator", "conversation", "evaluator"}
	gotKinds := make([]string, len(calls))
	for i, call := range calls {
		if isGoalEvaluatorRequest(call) {
			gotKinds[i] = "evaluator"
			assertToolFreeGoalEvaluatorRequest(t, call)
		} else {
			gotKinds[i] = "conversation"
		}
	}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("provider call kinds = %v, want %v", gotKinds, wantKinds)
	}

	meta, _, err := repo.GetMeta(sessionID, projectDir)
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if meta.Goal == nil {
		t.Fatal("completed goal metadata is nil")
	}
	if meta.Goal.Status != goal.StatusAchieved {
		t.Fatalf("goal status = %q, want %q", meta.Goal.Status, goal.StatusAchieved)
	}
	if meta.Goal.TurnCount != 2 {
		t.Fatalf("goal turn count = %d, want 2", meta.Goal.TurnCount)
	}
	if meta.Goal.LastEvaluatorReason != "the transcript proves every acceptance criterion" {
		t.Fatalf("goal evaluator reason = %q", meta.Goal.LastEvaluatorReason)
	}
	if meta.Goal.AchievedAt == nil {
		t.Fatal("achieved goal is missing achieved_at")
	}
	if meta.Goal.CreatedAt != initial.CreatedAt || meta.Goal.TokenBudget != initial.TokenBudget {
		t.Fatalf("goal identity fields changed: got %+v initial %+v", meta.Goal, initial)
	}
}

func TestCoreEngineModelSwitchAlsoUpdatesGoalEvaluatorModel(t *testing.T) {
	repo, projectDir, sessionID, _ := prepareEngineGoalSession(t, goal.StatusActive)
	p := &goalIntegrationProvider{responses: []string{
		"implementation and verification are complete",
		`{"criteria":[{"id":"AC-1","met":true,"reason":"the transcript proves the criterion"}],"reason":"the transcript proves the objective is complete"}`,
	}}
	eng, err := New(Config{
		Provider:    p,
		Sessions:    NewRepositorySessionManager(repo, func() string { return projectDir }),
		ProjectRoot: "/workspace/goal-integration",
		CWD:         "/workspace/goal-integration",
		Model:       "conversation-model-v1",
		MaxTurns:    4,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := eng.Resume(context.Background(), sessionID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if err := eng.SetModel(sessionID, "conversation-model-v2"); err != nil {
		t.Fatalf("SetModel: %v", err)
	}

	events, err := eng.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "finish"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	drained := drainEvents(t, events, 5*time.Second)
	assertGoalIntegrationQuerySucceeded(t, drained)

	calls := p.recordedCalls()
	if len(calls) != 2 {
		t.Fatalf("provider calls = %d, want conversation + evaluator", len(calls))
	}
	for i, call := range calls {
		if call.Model != "conversation-model-v2" {
			t.Fatalf("provider call %d model = %q, want switched conversation model", i+1, call.Model)
		}
	}
	for _, event := range drained {
		if event.Inner.Type != stream.EventGoalEvaluation {
			continue
		}
		if got := event.Inner.Metadata["model"]; got != "conversation-model-v2" {
			t.Fatalf("goal evaluation event model = %#v, want actual evaluator model", got)
		}
		return
	}
	t.Fatal("query did not emit goal evaluation usage")
}

func TestCoreEngineDoesNotEvaluatePausedOrTerminalGoals(t *testing.T) {
	for _, status := range []goal.Status{
		goal.StatusPaused,
		goal.StatusAchieved,
		goal.StatusBlocked,
		goal.StatusCleared,
	} {
		t.Run(string(status), func(t *testing.T) {
			repo, projectDir, sessionID, initial := prepareEngineGoalSession(t, status)
			p := &goalIntegrationProvider{responses: []string{"ordinary conversation response"}}
			eng, err := New(Config{
				Provider: p,
				Sessions: NewRepositorySessionManager(repo, func() string {
					return projectDir
				}),
				MaxTurns: 4,
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			events, err := eng.Query(context.Background(), QueryRequest{SessionID: sessionID, Message: "normal turn"})
			if err != nil {
				t.Fatalf("Query: %v", err)
			}
			assertGoalIntegrationQuerySucceeded(t, drainEvents(t, events, 5*time.Second))

			calls := p.recordedCalls()
			if len(calls) != 1 {
				t.Fatalf("provider calls = %d, want one conversation call", len(calls))
			}
			if isGoalEvaluatorRequest(calls[0]) {
				t.Fatal("paused or terminal goal triggered evaluator")
			}

			meta, _, err := repo.GetMeta(sessionID, projectDir)
			if err != nil {
				t.Fatalf("GetMeta: %v", err)
			}
			if meta.Goal == nil || !reflect.DeepEqual(*meta.Goal, initial) {
				t.Fatalf("inactive goal changed: got %+v want %+v", meta.Goal, initial)
			}
		})
	}
}

func prepareEngineGoalSession(t *testing.T, status goal.Status) (*session.Repository, string, string, goal.Goal) {
	t.Helper()
	repo := session.NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD("/workspace/goal-integration")
	sessionID := "goal-" + string(status)
	if err := repo.Save(sessionID, projectDir, nil); err != nil {
		t.Fatalf("create session: %v", err)
	}

	createdAt := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	state, err := goal.CreateWithCriteria("finish the engine integration", []string{"finish the engine integration"}, 50_000, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	changedAt := createdAt.Add(time.Minute)
	switch status {
	case goal.StatusActive:
	case goal.StatusPaused:
		state, err = goal.Pause(state, changedAt)
	case goal.StatusAchieved:
		state, err = goal.RecordAcceptanceEvaluation(state, state.Revision, []goal.AcceptanceCriterionEvaluation{{
			CriterionID: "AC-1", Met: true, Reason: "already complete",
		}}, "already complete", changedAt)
		if err != nil {
			break
		}
		state, err = goal.Achieve(state, "already complete", changedAt)
	case goal.StatusBlocked:
		state, err = goal.Block(state, "waiting for user input", changedAt)
	case goal.StatusCleared:
		state, err = goal.Clear(state, changedAt)
	default:
		t.Fatalf("unsupported goal status %q", status)
	}
	if err != nil {
		t.Fatalf("build %s goal: %v", status, err)
	}
	if err := repo.SaveMeta(sessionID, projectDir, session.SessionMeta{Goal: &state}); err != nil {
		t.Fatalf("save %s goal: %v", status, err)
	}
	return repo, projectDir, sessionID, state
}

func goalIntegrationTextStream(text string) <-chan types.StreamEvent {
	return goalIntegrationTextStreamWithUsage(text, nil)
}

func goalIntegrationTextStreamWithUsage(text string, usage *types.Usage) <-chan types.StreamEvent {
	events := []types.StreamEvent{
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeText}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "text_delta", Text: text}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, Usage: usage},
		{Type: types.EventMessageStop},
	}
	stream := make(chan types.StreamEvent, len(events))
	for _, event := range events {
		stream <- event
	}
	close(stream)
	return stream
}

func isGoalEvaluatorRequest(params provider.Params) bool {
	return strings.Contains(params.JoinedSystemPrompt(), "goal acceptance evaluator")
}

func assertToolFreeGoalEvaluatorRequest(t *testing.T, params provider.Params) {
	t.Helper()
	if len(params.Tools) != 0 || len(params.ExtraToolSchemas) != 0 || params.ToolChoice != nil {
		t.Fatalf("evaluator request exposed tools: tools=%d server_tools=%d choice=%+v", len(params.Tools), len(params.ExtraToolSchemas), params.ToolChoice)
	}
	if params.Thinking == nil || params.Thinking.Enabled {
		t.Fatalf("evaluator thinking config = %+v, want explicitly disabled", params.Thinking)
	}
}

func assertGoalIntegrationQuerySucceeded(t *testing.T, events []Event) {
	t.Helper()
	for _, event := range events {
		if !event.Final {
			continue
		}
		if event.Error != nil {
			t.Fatalf("query final error: %v", event.Error)
		}
		return
	}
	t.Fatal("query did not emit a final event")
}
