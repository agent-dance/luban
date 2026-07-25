package goal

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func createGoal(objective string, tokenBudget int, now time.Time) (Goal, error) {
	return CreateWithCriteria(objective, []string{objective}, tokenBudget, now)
}

func TestGoalCreateDefinesPersistedFields(t *testing.T) {
	now := time.Date(2026, time.July, 14, 9, 30, 0, 0, time.UTC)

	got, err := createGoal("fix every failing test", 12_000, now)
	if err != nil {
		t.Fatalf("createGoal() error = %v", err)
	}
	if got.Objective != "fix every failing test" {
		t.Fatalf("Objective = %q", got.Objective)
	}
	if got.Status != StatusActive {
		t.Fatalf("Status = %q, want %q", got.Status, StatusActive)
	}
	if got.TokenBudget != 12_000 || got.Usage != 0 || got.TurnCount != 0 {
		t.Fatalf("budget fields = budget:%d usage:%d turns:%d", got.TokenBudget, got.Usage, got.TurnCount)
	}
	if got.LastEvaluatorReason != "" || got.AchievedAt != nil || got.BlockedAt != nil {
		t.Fatalf("new goal has terminal metadata: %+v", got)
	}
	if !got.CreatedAt.Equal(now) || !got.UpdatedAt.Equal(now) {
		t.Fatalf("timestamps = created:%v updated:%v, want %v", got.CreatedAt, got.UpdatedAt, now)
	}

	persisted := got
	persisted.Usage = 128
	persisted.TurnCount = 1
	persisted.LastEvaluatorReason = "work remains"
	data, err := json.Marshal(persisted)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	for _, key := range []string{"objective", "status", "token_budget", "usage", "turn_count", "last_evaluator_reason", "created_at", "updated_at"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("persisted Goal JSON missing %q: %s", key, data)
		}
	}
	for _, key := range []string{"achieved_at", "blocked_at"} {
		if _, ok := fields[key]; ok {
			t.Errorf("active Goal JSON unexpectedly contains %q: %s", key, data)
		}
	}
}

func TestGoalStatusesAreStable(t *testing.T) {
	tests := map[Status]string{
		StatusActive:   "active",
		StatusPaused:   "paused",
		StatusAchieved: "achieved",
		StatusBlocked:  "blocked",
		StatusCleared:  "cleared",
	}
	for status, want := range tests {
		if string(status) != want {
			t.Errorf("status = %q, want %q", status, want)
		}
	}
}

func TestSetEvaluatorReasonPersistsStableSemanticMetadata(t *testing.T) {
	now := time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC)
	current, err := createGoal("finish i18n", 0, now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	got := SetEvaluatorReason(current, " rendered compatibility ", EvaluatorReasonFailed,
		"loop.goal_evaluator.provider_call_failed", " raw provider detail ", now)
	if got.LastEvaluatorReason != "rendered compatibility" || got.LastEvaluatorReasonKind != EvaluatorReasonFailed ||
		got.LastEvaluatorReasonKey != "loop.goal_evaluator.provider_call_failed" || got.LastEvaluatorReasonDetail != "raw provider detail" {
		t.Fatalf("SetEvaluatorReason() = %+v", got)
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"last_evaluator_reason_kind", "last_evaluator_reason_key", "last_evaluator_reason_detail"} {
		if !strings.Contains(string(data), `"`+field+`"`) {
			t.Fatalf("persisted goal omitted %s: %s", field, data)
		}
	}
}

func TestGoalObjectiveValidationUsesUnicodeCharacters(t *testing.T) {
	now := time.Date(2026, time.July, 14, 9, 30, 0, 0, time.UTC)

	for _, objective := range []string{"", " \t\n "} {
		if _, err := createGoal(objective, 0, now); !errors.Is(err, ErrObjectiveRequired) {
			t.Errorf("createGoal(%q) error = %v, want ErrObjectiveRequired", objective, err)
		}
	}
	if _, err := createGoal(strings.Repeat("界", 4000), 0, now); err != nil {
		t.Fatalf("createGoal(4000 characters) error = %v", err)
	}
	if _, err := createGoal(strings.Repeat("界", 4001), 0, now); !errors.Is(err, ErrObjectiveTooLong) {
		t.Fatalf("createGoal(4001 characters) error = %v, want ErrObjectiveTooLong", err)
	}
	current, err := createGoal("original objective", 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Edit(current, "\t", now); !errors.Is(err, ErrObjectiveRequired) {
		t.Errorf("Edit(empty) error = %v, want ErrObjectiveRequired", err)
	}
	if _, err := Edit(current, strings.Repeat("界", 4001), now); !errors.Is(err, ErrObjectiveTooLong) {
		t.Errorf("Edit(4001 characters) error = %v, want ErrObjectiveTooLong", err)
	}
	if err := ErrObjectiveTooLong; err.Error() != "goal objective must not exceed 4000 characters" {
		t.Fatalf("ErrObjectiveTooLong = %q", err)
	}
}

func TestGoalTransitions(t *testing.T) {
	createdAt := time.Date(2026, time.July, 14, 9, 30, 0, 0, time.UTC)
	changedAt := createdAt.Add(time.Minute)
	base, err := createGoal("finish the goal package", 8_000, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	base.Usage = 512
	base.TurnCount = 2
	base.LastEvaluatorReason = "tests are still failing"
	base.LastEvaluatorReasonKind = EvaluatorReasonFailed
	base.LastEvaluatorReasonKey = "loop.goal_evaluator.provider_unavailable"
	base.LastEvaluatorReasonDetail = "old detail"

	edited, err := Edit(base, "finish the complete goal package", changedAt)
	assertGoalTransition(t, "edit", base, edited, err, StatusActive, changedAt)
	if edited.Objective != "finish the complete goal package" {
		t.Fatalf("Edit() objective = %q", edited.Objective)
	}

	paused, err := Pause(base, changedAt)
	assertGoalTransition(t, "pause", base, paused, err, StatusPaused, changedAt)

	resumed, err := Resume(paused, changedAt.Add(time.Minute))
	assertGoalTransition(t, "resume paused", paused, resumed, err, StatusActive, changedAt.Add(time.Minute))

	blocked, err := Block(base, "waiting for credentials", changedAt)
	assertGoalTransition(t, "block", base, blocked, err, StatusBlocked, changedAt)
	if blocked.BlockedAt == nil || !blocked.BlockedAt.Equal(changedAt) || blocked.LastEvaluatorReason != "waiting for credentials" {
		t.Fatalf("Block() terminal metadata = blocked_at:%v reason:%q", blocked.BlockedAt, blocked.LastEvaluatorReason)
	}
	if blocked.LastEvaluatorReasonKind != "" || blocked.LastEvaluatorReasonKey != "" || blocked.LastEvaluatorReasonDetail != "" {
		t.Fatalf("Block() retained stale semantic reason metadata: %+v", blocked)
	}

	resumed, err = Resume(blocked, changedAt.Add(time.Minute))
	assertGoalTransition(t, "resume blocked", blocked, resumed, err, StatusActive, changedAt.Add(time.Minute))

	evaluated, err := RecordAcceptanceEvaluation(base, base.Revision, []AcceptanceCriterionEvaluation{{
		CriterionID: "AC-1", Met: true, Reason: "all acceptance criteria pass",
	}}, "all acceptance criteria pass", changedAt)
	if err != nil {
		t.Fatal(err)
	}
	achieved, err := Achieve(evaluated, "all acceptance criteria pass", changedAt)
	assertGoalTransition(t, "achieve", base, achieved, err, StatusAchieved, changedAt)
	if achieved.AchievedAt == nil || !achieved.AchievedAt.Equal(changedAt) || achieved.LastEvaluatorReason != "all acceptance criteria pass" {
		t.Fatalf("Achieve() terminal metadata = achieved_at:%v reason:%q", achieved.AchievedAt, achieved.LastEvaluatorReason)
	}
	if achieved.LastEvaluatorReasonKind != "" || achieved.LastEvaluatorReasonKey != "" || achieved.LastEvaluatorReasonDetail != "" {
		t.Fatalf("Achieve() retained stale semantic reason metadata: %+v", achieved)
	}

	for _, current := range []Goal{base, paused, blocked, achieved} {
		cleared, clearErr := Clear(current, changedAt)
		assertGoalTransition(t, "clear "+string(current.Status), current, cleared, clearErr, StatusCleared, changedAt)
	}
}

func TestGoalTransitionErrorsAreStable(t *testing.T) {
	now := time.Date(2026, time.July, 14, 9, 30, 0, 0, time.UTC)
	active, err := createGoal("finish goal transitions", 0, now)
	if err != nil {
		t.Fatal(err)
	}
	paused, _ := Pause(active, now)
	blocked, _ := Block(active, "blocked", now)
	evaluated, recordErr := RecordAcceptanceEvaluation(active, active.Revision, []AcceptanceCriterionEvaluation{{
		CriterionID: "AC-1", Met: true, Reason: "done",
	}}, "done", now)
	if recordErr != nil {
		t.Fatal(recordErr)
	}
	achieved, achieveErr := Achieve(evaluated, "done", now)
	if achieveErr != nil {
		t.Fatal(achieveErr)
	}
	cleared, _ := Clear(active, now)

	tests := []struct {
		name   string
		action string
		from   Status
		run    func() error
	}{
		{name: "edit terminal", action: "edit", from: StatusAchieved, run: func() error { _, err := Edit(achieved, "new", now); return err }},
		{name: "pause paused", action: "pause", from: StatusPaused, run: func() error { _, err := Pause(paused, now); return err }},
		{name: "resume active", action: "resume", from: StatusActive, run: func() error { _, err := Resume(active, now); return err }},
		{name: "achieve paused", action: "achieve", from: StatusPaused, run: func() error { _, err := Achieve(paused, "done", now); return err }},
		{name: "block blocked", action: "block", from: StatusBlocked, run: func() error { _, err := Block(blocked, "again", now); return err }},
		{name: "clear cleared", action: "clear", from: StatusCleared, run: func() error { _, err := Clear(cleared, now); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("error = %v, want ErrInvalidTransition", err)
			}
			want := fmt.Sprintf("goal: cannot %s from %s status: invalid transition", tt.action, tt.from)
			if err.Error() != want {
				t.Fatalf("error = %q, want %q", err, want)
			}
		})
	}
}

func assertGoalTransition(t *testing.T, name string, before, after Goal, err error, wantStatus Status, wantUpdatedAt time.Time) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s error = %v", name, err)
	}
	if after.Status != wantStatus {
		t.Fatalf("%s status = %q, want %q", name, after.Status, wantStatus)
	}
	if !after.UpdatedAt.Equal(wantUpdatedAt) {
		t.Fatalf("%s updated_at = %v, want %v", name, after.UpdatedAt, wantUpdatedAt)
	}
	if before.Status == StatusActive && before.UpdatedAt.Equal(after.UpdatedAt) && reflect.DeepEqual(before, after) {
		t.Fatalf("%s did not return a changed Goal", name)
	}
	if after.CreatedAt != before.CreatedAt || after.TokenBudget != before.TokenBudget || after.Usage != before.Usage || after.TurnCount != before.TurnCount {
		t.Fatalf("%s did not preserve shared fields: before=%+v after=%+v", name, before, after)
	}
}
