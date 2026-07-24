package goal

import (
	"errors"
	"testing"
	"time"
)

func TestCreateWithCriteriaRequiresClearUniqueConditions(t *testing.T) {
	now := time.Date(2026, time.July, 19, 10, 0, 0, 0, time.UTC)
	created, err := CreateWithCriteria("ship the release", []string{
		" focused tests pass ",
		"documentation describes the new behavior",
	}, 4096, now)
	if err != nil {
		t.Fatal(err)
	}
	if created.Revision != 1 || created.NextCriterionID != 3 || len(created.AcceptanceCriteria) != 2 {
		t.Fatalf("created goal acceptance metadata = %+v", created)
	}
	if got := created.AcceptanceCriteria[0]; got.ID != "AC-1" || got.Text != "focused tests pass" {
		t.Fatalf("first criterion = %+v", got)
	}
	if _, err := CreateWithCriteria("ship", nil, 0, now); !errors.Is(err, ErrAcceptanceCriteriaRequired) {
		t.Fatalf("missing criteria error = %v", err)
	}
	if _, err := CreateWithCriteria("ship", []string{"same", " SAME "}, 0, now); !errors.Is(err, ErrAcceptanceCriterionDuplicate) {
		t.Fatalf("duplicate criteria error = %v", err)
	}
}

func TestAcceptanceCriterionEditsCreateRevisionAndInvalidateEvaluation(t *testing.T) {
	now := time.Date(2026, time.July, 19, 10, 0, 0, 0, time.UTC)
	current, err := CreateWithCriteria("ship", []string{"tests pass", "docs updated"}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	current, err = RecordAcceptanceEvaluation(current, 1, []AcceptanceCriterionEvaluation{
		{CriterionID: "AC-1", Met: true, Reason: "tests passed"},
		{CriterionID: "AC-2", Met: false, Reason: "docs missing"},
	}, "documentation remains", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	current, err = Block(current, "waiting for documentation", now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	current, err = EditAcceptanceCriterion(current, "ac-2", "docs and migration notes updated", now.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != StatusBlocked || current.Revision != 2 || current.LastAcceptanceEvaluation != nil || current.LastEvaluatorReason != "" {
		t.Fatalf("edited blocked goal = %+v", current)
	}
	if current.AcceptanceCriteria[1].ID != "AC-2" || current.AcceptanceCriteria[1].Text != "docs and migration notes updated" {
		t.Fatalf("edited criterion = %+v", current.AcceptanceCriteria[1])
	}
}

func TestAgentReplacementPreservesCriterionIDsAndInvalidatesOldResults(t *testing.T) {
	now := time.Date(2026, time.July, 21, 10, 0, 0, 0, time.UTC)
	current, err := CreateWithCriteria("ship", []string{"tests pass", "docs updated"}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	current, err = RecordAcceptanceEvaluation(current, 1, []AcceptanceCriterionEvaluation{
		{CriterionID: "AC-1", Met: true, Reason: "passed"},
		{CriterionID: "AC-2", Met: false, Reason: "missing"},
	}, "docs remain", now)
	if err != nil {
		t.Fatal(err)
	}
	revised, err := ReplaceAcceptanceCriteria(current, []string{"focused tests pass", "TUI shows statuses", "copy is localized"}, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if revised.Revision != 2 || revised.LastAcceptanceEvaluation != nil || len(revised.AcceptanceCriteria) != 3 {
		t.Fatalf("replacement revision = %+v", revised)
	}
	for index, id := range []string{"AC-1", "AC-2", "AC-3"} {
		if revised.AcceptanceCriteria[index].ID != id {
			t.Fatalf("criterion IDs = %+v", revised.AcceptanceCriteria)
		}
	}
}

func TestAchieveRequiresEveryCriterionInCurrentRevision(t *testing.T) {
	now := time.Date(2026, time.July, 19, 10, 0, 0, 0, time.UTC)
	current, err := CreateWithCriteria("ship", []string{"tests pass", "docs updated"}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Achieve(current, "self reported", now); !errors.Is(err, ErrAcceptanceCriteriaUnmet) {
		t.Fatalf("unevaluated Achieve error = %v", err)
	}
	current, err = RecordAcceptanceEvaluation(current, 1, []AcceptanceCriterionEvaluation{
		{CriterionID: "AC-1", Met: true, Reason: "tests passed"},
		{CriterionID: "AC-2", Met: false, Reason: "docs missing"},
	}, "one condition remains", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if AcceptanceCriteriaMet(current) {
		t.Fatal("partially met criteria reported complete")
	}
	current, err = RecordAcceptanceEvaluation(current, 1, []AcceptanceCriterionEvaluation{
		{CriterionID: "AC-2", Met: true, Reason: "docs updated"},
		{CriterionID: "AC-1", Met: true, Reason: "tests passed"},
	}, "all conditions verified", now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	completed, err := Achieve(current, current.LastAcceptanceEvaluation.Summary, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != StatusAchieved || completed.AchievedAt == nil {
		t.Fatalf("completed goal = %+v", completed)
	}
}

func TestLegacyGoalNormalizesObjectiveIntoCriterion(t *testing.T) {
	legacy := Goal{Objective: "finish migration", Status: StatusActive}
	normalized := Normalize(legacy)
	if normalized.Revision != 1 || normalized.NextCriterionID != 2 || len(normalized.AcceptanceCriteria) != 1 {
		t.Fatalf("normalized legacy goal = %+v", normalized)
	}
	if normalized.AcceptanceCriteria[0].ID != "AC-1" || normalized.AcceptanceCriteria[0].Text != legacy.Objective {
		t.Fatalf("legacy criterion = %+v", normalized.AcceptanceCriteria[0])
	}
}
