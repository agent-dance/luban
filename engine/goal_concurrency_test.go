package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/goal"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/session"
)

type blockingGoalEvaluator struct {
	started chan struct{}
	release chan struct{}
	result  loop.GoalEvaluationResult

	mu    sync.Mutex
	calls int
}

func (e *blockingGoalEvaluator) Evaluate(ctx context.Context, _ loop.GoalEvaluationRequest) (loop.GoalEvaluationResult, error) {
	e.mu.Lock()
	e.calls++
	call := e.calls
	e.mu.Unlock()

	if call == 1 {
		close(e.started)
		select {
		case <-e.release:
		case <-ctx.Done():
			return loop.GoalEvaluationResult{}, ctx.Err()
		}
		return e.result, nil
	}

	// A stale unmet result currently triggers a second turn. Terminate that
	// unexpected turn promptly so the test can report the real contract breach.
	return loop.GoalEvaluationResult{Met: true, Reason: "unexpected second evaluation"}, nil
}

func (e *blockingGoalEvaluator) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

func TestCoreEngineDoesNotOverwriteConcurrentGoalTransitionWithStaleEvaluation(t *testing.T) {
	tests := []struct {
		name       string
		result     loop.GoalEvaluationResult
		transition func(goal.Goal, time.Time) (goal.Goal, error)
		wantStatus goal.Status
	}{
		{
			name:       "met result cannot overwrite pause",
			result:     loop.GoalEvaluationResult{Met: true, Reason: "stale evaluator says complete"},
			transition: goal.Pause,
			wantStatus: goal.StatusPaused,
		},
		{
			name:       "unmet result cannot overwrite clear or continue",
			result:     loop.GoalEvaluationResult{Met: false, Reason: "stale evaluator says continue"},
			transition: goal.Clear,
			wantStatus: goal.StatusCleared,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, projectDir, sessionID, _ := prepareEngineGoalSession(t, goal.StatusActive)
			provider := &goalIntegrationProvider{responses: []string{
				"the conversation turn reached the evaluator",
				"this continuation must not run",
			}}
			evaluator := &blockingGoalEvaluator{
				started: make(chan struct{}),
				release: make(chan struct{}),
				result:  test.result,
			}
			eng, err := New(Config{
				Provider:      provider,
				Sessions:      NewRepositorySessionManager(repo, func() string { return projectDir }),
				GoalEvaluator: evaluator,
				ProjectRoot:   "/workspace/goal-integration",
				CWD:           "/workspace/goal-integration",
				MaxTurns:      4,
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			t.Cleanup(func() { _ = eng.Shutdown(context.Background()) })

			events, err := eng.Query(context.Background(), QueryRequest{
				SessionID: sessionID,
				Message:   "finish the active goal",
			})
			if err != nil {
				t.Fatalf("Query: %v", err)
			}

			select {
			case <-evaluator.started:
			case <-time.After(5 * time.Second):
				t.Fatal("query did not enter goal evaluator")
			}

			meta, _, err := repo.GetMeta(sessionID, projectDir)
			if err != nil {
				t.Fatalf("GetMeta before concurrent transition: %v", err)
			}
			if meta.Goal == nil || meta.Goal.Status != goal.StatusActive {
				t.Fatalf("goal before concurrent transition = %+v, want active", meta.Goal)
			}
			concurrent, err := test.transition(*meta.Goal, time.Now().UTC())
			if err != nil {
				t.Fatalf("concurrent transition: %v", err)
			}
			if err := repo.SaveMeta(sessionID, projectDir, session.SessionMeta{Goal: &concurrent}); err != nil {
				t.Fatalf("save concurrent transition: %v", err)
			}
			close(evaluator.release)

			assertGoalIntegrationQuerySucceeded(t, drainEvents(t, events, 5*time.Second))

			finalMeta, _, err := repo.GetMeta(sessionID, projectDir)
			if err != nil {
				t.Fatalf("GetMeta after stale evaluation: %v", err)
			}
			if finalMeta.Goal == nil {
				t.Fatal("goal metadata is nil after stale evaluation")
			}
			if finalMeta.Goal.Status != test.wantStatus {
				t.Fatalf("goal status = %q, want concurrent status %q; stale evaluation overwrote the newer transition", finalMeta.Goal.Status, test.wantStatus)
			}
			if finalMeta.Goal.AchievedAt != nil {
				t.Fatalf("stale evaluation recorded achievement at %v", finalMeta.Goal.AchievedAt)
			}
			if calls := len(provider.recordedCalls()); calls != 1 {
				t.Fatalf("provider calls = %d, want 1; stale evaluation triggered continuation", calls)
			}
			if calls := evaluator.callCount(); calls != 1 {
				t.Fatalf("evaluator calls = %d, want 1", calls)
			}
		})
	}
}
