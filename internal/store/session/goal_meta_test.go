package session

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/runtime/goal"
	"github.com/agent-dance/luban/types"
)

func TestSessionGoalMetaRoundTripsAndSurvivesTranscriptSaves(t *testing.T) {
	repo := NewRepository(t.TempDir())
	projectDir := repo.ProjectDirForCWD("/workspace/goal-project")
	const sessionID = "goal-session"

	messages := []types.Message{types.UserMessage("finish the persisted goal")}
	if err := repo.Save(sessionID, projectDir, messages); err != nil {
		t.Fatalf("save transcript: %v", err)
	}
	want := testSessionGoal(t, goal.StatusActive)
	if err := repo.SaveMeta(sessionID, projectDir, SessionMeta{Goal: &want}); err != nil {
		t.Fatalf("save goal metadata: %v", err)
	}

	// Transcript persistence owns derived fields and must retain the separately
	// managed goal state in the metadata sidecar.
	if err := repo.Save(sessionID, projectDir, append(messages, types.AssistantMessage("still working"))); err != nil {
		t.Fatalf("save updated transcript: %v", err)
	}
	got, ref, err := repo.GetMeta(sessionID, projectDir)
	if err != nil {
		t.Fatalf("get goal metadata: %v", err)
	}
	if ref.ProjectDir != projectDir {
		t.Fatalf("goal metadata resolved from %q, want %q", ref.ProjectDir, projectDir)
	}
	if got.Goal == nil {
		t.Fatal("persisted goal metadata is nil")
	}
	if !reflect.DeepEqual(*got.Goal, want) {
		t.Fatalf("goal metadata = %+v, want %+v", *got.Goal, want)
	}
}

func TestSessionGoalMetaSurvivesPartialMergeAndCanBeReplaced(t *testing.T) {
	store := NewFileStore(t.TempDir())
	const sessionID = "goal-merge"
	if err := store.Save(sessionID, []types.Message{types.UserMessage("retain this goal")}); err != nil {
		t.Fatal(err)
	}

	active := testSessionGoal(t, goal.StatusActive)
	if err := store.SaveMeta(sessionID, SessionMeta{Goal: &active}); err != nil {
		t.Fatalf("save active goal: %v", err)
	}
	if err := store.SaveMeta(sessionID, SessionMeta{Title: "renamed-session"}); err != nil {
		t.Fatalf("save unrelated metadata: %v", err)
	}
	merged, err := store.GetMeta(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Goal == nil || !reflect.DeepEqual(*merged.Goal, active) {
		t.Fatalf("partial SaveMeta erased goal: got %+v want %+v", merged.Goal, active)
	}

	paused := testSessionGoal(t, goal.StatusPaused)
	if err := store.SaveMeta(sessionID, SessionMeta{Goal: &paused}); err != nil {
		t.Fatalf("replace active goal: %v", err)
	}
	replaced, err := store.GetMeta(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Goal == nil || !reflect.DeepEqual(*replaced.Goal, paused) {
		t.Fatalf("replacement goal = %+v, want %+v", replaced.Goal, paused)
	}
}

func TestSessionGoalMetaPersistsEveryStatusWithoutReactivationPolicy(t *testing.T) {
	for _, status := range []goal.Status{
		goal.StatusActive,
		goal.StatusPaused,
		goal.StatusAchieved,
		goal.StatusBlocked,
		goal.StatusCleared,
	} {
		t.Run(string(status), func(t *testing.T) {
			store := NewFileStore(t.TempDir())
			const sessionID = "goal-status"
			if err := store.Save(sessionID, []types.Message{types.UserMessage("persist status")}); err != nil {
				t.Fatal(err)
			}
			want := testSessionGoal(t, status)
			if err := store.SaveMeta(sessionID, SessionMeta{Goal: &want}); err != nil {
				t.Fatalf("save %s goal: %v", status, err)
			}
			got, err := store.GetMeta(sessionID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Goal == nil || !reflect.DeepEqual(*got.Goal, want) {
				t.Fatalf("persisted %s goal = %+v, want %+v", status, got.Goal, want)
			}
		})
	}
}

func TestSessionGoalMetaJSONOmitsNilGoal(t *testing.T) {
	withoutGoal, err := json.Marshal(SessionMeta{ID: "no-goal"})
	if err != nil {
		t.Fatalf("marshal metadata without goal: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(withoutGoal, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["goal"]; ok {
		t.Fatalf("nil goal was not omitted: %s", withoutGoal)
	}

	want := testSessionGoal(t, goal.StatusActive)
	withGoal, err := json.Marshal(SessionMeta{ID: "with-goal", Goal: &want})
	if err != nil {
		t.Fatalf("marshal metadata with goal: %v", err)
	}
	fields = nil
	if err := json.Unmarshal(withGoal, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["goal"]; !ok {
		t.Fatalf("goal field missing from metadata JSON: %s", withGoal)
	}
}

func testSessionGoal(t *testing.T, status goal.Status) goal.Goal {
	t.Helper()
	createdAt := time.Date(2026, time.July, 14, 10, 0, 0, 0, time.UTC)
	state, err := goal.CreateWithCriteria("finish session goal persistence", []string{"finish session goal persistence"}, 20_000, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	state.Usage = 1_024
	state.TurnCount = 3
	state.LastEvaluatorReason = "acceptance tests remain"
	changedAt := createdAt.Add(time.Minute)

	switch status {
	case goal.StatusActive:
		return state
	case goal.StatusPaused:
		state, err = goal.Pause(state, changedAt)
	case goal.StatusAchieved:
		state, err = goal.RecordAcceptanceEvaluation(state, state.Revision, []goal.AcceptanceCriterionEvaluation{{
			CriterionID: "AC-1", Met: true, Reason: "all acceptance tests pass",
		}}, "all acceptance tests pass", changedAt)
		if err != nil {
			break
		}
		state, err = goal.Achieve(state, "all acceptance tests pass", changedAt)
	case goal.StatusBlocked:
		state, err = goal.Block(state, "waiting for credentials", changedAt)
	case goal.StatusCleared:
		state, err = goal.Clear(state, changedAt)
	default:
		t.Fatalf("unsupported goal status %q", status)
	}
	if err != nil {
		t.Fatalf("build %s goal: %v", status, err)
	}
	return state
}
