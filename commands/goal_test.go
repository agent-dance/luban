package commands_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/goal"
	"github.com/agent-dance/luban/i18n"
)

type fakeGoalRuntime struct {
	current *goal.Goal
	loadErr error
	saveErr error
	saved   []goal.Goal
}

func (f *fakeGoalRuntime) LoadGoal() (*goal.Goal, error) {
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	if f.current == nil {
		return nil, nil
	}
	current := *f.current
	return &current, nil
}

func (f *fakeGoalRuntime) SaveGoal(next goal.Goal) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	saved := next
	f.current = &saved
	f.saved = append(f.saved, saved)
	return nil
}

var _ commands.GoalRuntime = (*fakeGoalRuntime)(nil)

type atomicGoalRuntime struct {
	current *goal.Goal

	loadCalls   int
	saveCalls   int
	updateCalls int
}

func (f *atomicGoalRuntime) LoadGoal() (*goal.Goal, error) {
	f.loadCalls++
	if f.current == nil {
		return nil, nil
	}
	current := *f.current
	return &current, nil
}

func (f *atomicGoalRuntime) SaveGoal(next goal.Goal) error {
	f.saveCalls++
	saved := next
	f.current = &saved
	return nil
}

func (f *atomicGoalRuntime) UpdateGoal(update goal.UpdateFunc) (goal.Goal, error) {
	f.updateCalls++
	var current *goal.Goal
	if f.current != nil {
		cloned := *f.current
		current = &cloned
	}
	next, err := update(current)
	if err != nil {
		return goal.Goal{}, err
	}
	saved := next
	f.current = &saved
	return next, nil
}

var _ commands.GoalRuntime = (*atomicGoalRuntime)(nil)
var _ goal.Updater = (*atomicGoalRuntime)(nil)

func TestRegisterBuiltinsIncludesGoalOnce(t *testing.T) {
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)

	if cmd := registry.Find("goal"); cmd == nil {
		t.Fatal("expected /goal to be registered")
	}
	count := 0
	for _, cmd := range registry.All() {
		if cmd.Name() == "goal" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("registered /goal commands = %d, want 1", count)
	}
	cmd, args := registry.Parse("/goal status")
	if cmd == nil || cmd.Name() != "goal" || args != "status" {
		t.Fatalf("Parse(/goal status) = cmd %v args %q", cmd, args)
	}
}

func TestGoalCommandShowsUnsetGoal(t *testing.T) {
	for _, args := range []string{"", "status", "view"} {
		t.Run(args, func(t *testing.T) {
			output, err := executeGoalCommand(t, &fakeGoalRuntime{}, args)
			if err != nil {
				t.Fatalf("/goal %s: %v", args, err)
			}
			assertContainsAnyFold(t, output, "no goal", "no active goal", "not set")
		})
	}
}

func TestGoalCommandShowsCurrentStatus(t *testing.T) {
	current := mustCreateGoal(t, "ship the release", 1000)
	current.Usage = 250
	current.TurnCount = 3
	current.LastEvaluatorReason = "more work remains"

	output, err := executeGoalCommand(t, &fakeGoalRuntime{current: &current}, "status")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ship the release", "active", "1000", "250", "3", "more work remains"} {
		assertContainsFold(t, output, want)
	}
}

func TestGoalCommandCreatesOrReplacesGoal(t *testing.T) {
	tests := []struct {
		name          string
		args          string
		wantCriterion string
	}{
		{name: "bare objective", args: "ship the release", wantCriterion: "ship the release"},
		{name: "explicit set", args: "set ship the release --accept focused tests pass --accept release notes are updated", wantCriterion: "focused tests pass"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			old := mustCreateGoal(t, "old objective", 99)
			old, err := goal.Clear(old, time.Now().Add(-time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			runtime := &fakeGoalRuntime{current: &old}
			output, err := executeGoalCommand(t, runtime, test.args)
			if err != nil {
				t.Fatal(err)
			}
			if len(runtime.saved) != 1 {
				t.Fatalf("saved goals = %d, want 1", len(runtime.saved))
			}
			if got := runtime.current; got == nil || got.Objective != "ship the release" || got.Status != goal.StatusActive || len(got.AcceptanceCriteria) == 0 {
				t.Fatalf("current goal = %#v", got)
			}
			assertContainsFold(t, output, "ship the release")
			assertContainsFold(t, output, "AC-1")
			assertContainsFold(t, output, test.wantCriterion)
		})
	}
}

func TestGoalCommandEditsExistingGoal(t *testing.T) {
	current := mustCreateGoal(t, "old objective", 500)
	createdAt := current.CreatedAt
	runtime := &fakeGoalRuntime{current: &current}

	output, err := executeGoalCommand(t, runtime, "edit revised objective")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.current == nil || runtime.current.Objective != "revised objective" {
		t.Fatalf("current goal = %#v", runtime.current)
	}
	if runtime.current.Status != goal.StatusActive || !runtime.current.CreatedAt.Equal(createdAt) {
		t.Fatalf("edit changed goal identity fields: %#v", runtime.current)
	}
	assertContainsFold(t, output, "revised objective")
}

func TestGoalCommandPresentsAndEditsAcceptanceCriteria(t *testing.T) {
	current, err := goal.CreateWithCriteria("ship release", []string{"focused tests pass", "release notes updated"}, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	current, err = goal.RecordAcceptanceEvaluation(current, current.Revision, []goal.AcceptanceCriterionEvaluation{
		{CriterionID: "AC-1", Met: true, Reason: "focused tests passed"},
		{CriterionID: "AC-2", Met: false, Reason: "release notes are missing"},
	}, "release notes remain", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runtime := &fakeGoalRuntime{current: &current}
	output, err := executeGoalCommand(t, runtime, "status")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"acceptance criteria", "revision 1", "[x] AC-1", "focused tests passed",
		"[!] AC-2", "release notes updated", "release notes are missing",
	} {
		assertContainsFold(t, output, want)
	}

	if _, err := executeGoalCommand(t, runtime, "criteria edit AC-2 release notes and migration notes updated"); err != nil {
		t.Fatal(err)
	}
	if _, err := executeGoalCommand(t, runtime, "criteria add clean checkout tests pass"); err != nil {
		t.Fatal(err)
	}
	if _, err := executeGoalCommand(t, runtime, "criteria remove AC-1"); err != nil {
		t.Fatal(err)
	}
	if runtime.current.Revision != 4 || len(runtime.current.AcceptanceCriteria) != 2 {
		t.Fatalf("edited goal = %+v", runtime.current)
	}
	if runtime.current.AcceptanceCriteria[0].ID != "AC-2" || runtime.current.AcceptanceCriteria[1].ID != "AC-3" {
		t.Fatalf("criterion identities after edits = %+v", runtime.current.AcceptanceCriteria)
	}
}

func TestGoalCommandPausesAndResumesGoal(t *testing.T) {
	current := mustCreateGoal(t, "ship the release", 0)
	runtime := &fakeGoalRuntime{current: &current}

	pauseOutput, err := executeGoalCommand(t, runtime, "pause")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.current == nil || runtime.current.Status != goal.StatusPaused {
		t.Fatalf("goal after pause = %#v", runtime.current)
	}
	assertContainsFold(t, pauseOutput, "paused")

	resumeOutput, err := executeGoalCommand(t, runtime, "resume")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.current == nil || runtime.current.Status != goal.StatusActive {
		t.Fatalf("goal after resume = %#v", runtime.current)
	}
	assertContainsFold(t, resumeOutput, "active")
}

func TestGoalCommandResumesBlockedGoal(t *testing.T) {
	current := mustCreateGoal(t, "recover the release", 0)
	current, err := goal.Block(current, "waiting for credentials", time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	runtime := &fakeGoalRuntime{current: &current}

	if _, err := executeGoalCommand(t, runtime, "resume"); err != nil {
		t.Fatal(err)
	}
	if runtime.current == nil || runtime.current.Status != goal.StatusActive || runtime.current.BlockedAt != nil {
		t.Fatalf("goal after blocked resume = %#v", runtime.current)
	}
}

func TestGoalCommandClearAliasesPersistClearedState(t *testing.T) {
	for _, alias := range []string{"clear", "stop", "off", "reset", "none", "cancel"} {
		t.Run(alias, func(t *testing.T) {
			current := mustCreateGoal(t, "ship the release", 0)
			runtime := &fakeGoalRuntime{current: &current}

			output, err := executeGoalCommand(t, runtime, alias)
			if err != nil {
				t.Fatal(err)
			}
			if runtime.current == nil || runtime.current.Status != goal.StatusCleared {
				t.Fatalf("goal after %q = %#v", alias, runtime.current)
			}
			if len(runtime.saved) != 1 {
				t.Fatalf("saved goals = %d, want 1", len(runtime.saved))
			}
			assertContainsFold(t, output, "clear")
		})
	}
}

func TestGoalCommandClearWithoutGoalIsIdempotent(t *testing.T) {
	runtime := &fakeGoalRuntime{}
	output, err := executeGoalCommand(t, runtime, "clear")
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.saved) != 0 {
		t.Fatalf("saved goals = %d, want 0", len(runtime.saved))
	}
	assertContainsAnyFold(t, output, "no goal", "no active goal", "not set")
}

func TestGoalCommandTransitionsUseAtomicUpdaterWhenAvailable(t *testing.T) {
	tests := []struct {
		name        string
		args        string
		initial     goal.Status
		wantStatus  goal.Status
		wantSubject string
	}{
		{name: "edit", args: "edit revised objective", initial: goal.StatusActive, wantStatus: goal.StatusActive, wantSubject: "revised objective"},
		{name: "pause", args: "pause", initial: goal.StatusActive, wantStatus: goal.StatusPaused, wantSubject: "finish atomic command"},
		{name: "resume", args: "resume", initial: goal.StatusPaused, wantStatus: goal.StatusActive, wantSubject: "finish atomic command"},
		{name: "clear", args: "clear", initial: goal.StatusActive, wantStatus: goal.StatusCleared, wantSubject: "finish atomic command"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := mustCreateGoal(t, "finish atomic command", 0)
			var err error
			if test.initial == goal.StatusPaused {
				current, err = goal.Pause(current, current.CreatedAt.Add(time.Minute))
				if err != nil {
					t.Fatal(err)
				}
			}
			runtime := &atomicGoalRuntime{current: &current}

			if _, err := executeGoalCommand(t, runtime, test.args); err != nil {
				t.Fatalf("/goal %s: %v", test.args, err)
			}
			if runtime.updateCalls != 1 {
				t.Fatalf("atomic UpdateGoal calls = %d, want 1", runtime.updateCalls)
			}
			if runtime.loadCalls != 0 || runtime.saveCalls != 0 {
				t.Fatalf("legacy goal persistence used despite goal.Updater: loads=%d saves=%d", runtime.loadCalls, runtime.saveCalls)
			}
			if runtime.current == nil || runtime.current.Status != test.wantStatus || runtime.current.Objective != test.wantSubject {
				t.Fatalf("goal after %q = %+v, want status=%q objective=%q", test.args, runtime.current, test.wantStatus, test.wantSubject)
			}
		})
	}
}

func TestGoalCommandValidatesObjectivesAndTransitions(t *testing.T) {
	active := mustCreateGoal(t, "existing objective", 0)
	tests := []struct {
		name    string
		args    string
		current *goal.Goal
		wantErr error
	}{
		{name: "set requires objective", args: "set", wantErr: goal.ErrObjectiveRequired},
		{name: "edit requires objective", args: "edit", current: &active, wantErr: goal.ErrObjectiveRequired},
		{name: "objective length", args: strings.Repeat("界", goal.MaxObjectiveCharacters+1) + " --accept verified", wantErr: goal.ErrObjectiveTooLong},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := executeGoalCommand(t, &fakeGoalRuntime{current: test.current}, test.args)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
		})
	}

	achieved := mustAchieveGoal(t, mustCreateGoal(t, "finished objective", 0))
	if _, err := executeGoalCommand(t, &fakeGoalRuntime{current: &achieved}, "edit rewrite history"); !errors.Is(err, goal.ErrInvalidTransition) {
		t.Fatalf("edit achieved goal error = %v, want invalid transition", err)
	}
}

func TestGoalCommandLocalizesStatusAndDomainErrors(t *testing.T) {
	active := mustCreateGoal(t, "整理发布说明", 0)
	active, err := goal.Block(active, "等待上游", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)
	cmd := registry.Find("goal")
	if cmd == nil {
		t.Fatal("/goal is not registered")
	}
	var output strings.Builder
	ctx := &commands.Context{Language: i18n.LangZH, GoalRuntime: &fakeGoalRuntime{current: &active}, OnEvent: func(value string) { output.WriteString(value) }}
	if err := cmd.Execute(ctx, "status"); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); !strings.Contains(got, "受阻") || strings.Contains(got, "blocked") {
		t.Fatalf("localized goal status = %q", got)
	}

	terminal := mustAchieveGoal(t, mustCreateGoal(t, "已完成目标", 0))
	ctx.GoalRuntime = &fakeGoalRuntime{current: &terminal}
	err = cmd.Execute(ctx, "edit 新目标")
	if !errors.Is(err, goal.ErrInvalidTransition) || err == nil || !strings.Contains(err.Error(), "已完成") || strings.Contains(err.Error(), "cannot edit") {
		t.Fatalf("localized transition error = %v", err)
	}
	err = cmd.Execute(&commands.Context{Language: i18n.LangZH, GoalRuntime: &fakeGoalRuntime{}}, "set")
	if !errors.Is(err, goal.ErrObjectiveRequired) || err == nil || !strings.Contains(err.Error(), "必须填写目标") {
		t.Fatalf("localized validation error = %v", err)
	}
}

func TestGoalCommandSignalsImmediateActivationOnlyForActiveTransitions(t *testing.T) {
	tests := []struct {
		name        string
		args        string
		current     *goal.Goal
		wantStarted bool
	}{
		{name: "set", args: "calculate 1+99", wantStarted: true},
		{name: "status", args: "status", current: goalPointer(mustCreateGoal(t, "existing", 0))},
		{name: "edit active", args: "edit revised", current: goalPointer(mustCreateGoal(t, "existing", 0)), wantStarted: true},
		{name: "edit paused", args: "edit revised", current: goalPointer(mustPauseGoal(t, mustCreateGoal(t, "existing", 0)))},
		{name: "pause", args: "pause", current: goalPointer(mustCreateGoal(t, "existing", 0))},
		{name: "resume", args: "resume", current: goalPointer(mustPauseGoal(t, mustCreateGoal(t, "existing", 0))), wantStarted: true},
		{name: "clear", args: "clear", current: goalPointer(mustCreateGoal(t, "existing", 0))},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := &fakeGoalRuntime{current: test.current}
			registry := commands.NewRegistry()
			commands.RegisterBuiltins(registry)
			cmd := registry.Find("goal")
			if cmd == nil {
				t.Fatal("missing /goal command")
			}
			var activated []string
			err := cmd.Execute(&commands.Context{
				GoalRuntime:     runtime,
				OnGoalActivated: func(objective string) { activated = append(activated, objective) },
			}, test.args)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if got := len(activated) > 0; got != test.wantStarted {
				t.Fatalf("activation signaled = %t, want %t (%v)", got, test.wantStarted, activated)
			}
			if test.wantStarted && activated[0] == "" {
				t.Fatal("activation objective is empty")
			}
		})
	}
}

func goalPointer(current goal.Goal) *goal.Goal { return &current }

func mustPauseGoal(t *testing.T, current goal.Goal) goal.Goal {
	t.Helper()
	paused, err := goal.Pause(current, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return paused
}

func TestGoalCommandSurfacesRuntimeErrors(t *testing.T) {
	loadErr := errors.New("load goal failed")
	if _, err := executeGoalCommand(t, &fakeGoalRuntime{loadErr: loadErr}, "status"); !errors.Is(err, loadErr) {
		t.Fatalf("load error = %v, want %v", err, loadErr)
	}

	saveErr := errors.New("save goal failed")
	runtime := &fakeGoalRuntime{saveErr: saveErr}
	if _, err := executeGoalCommand(t, runtime, "set ship release --accept focused tests pass"); !errors.Is(err, saveErr) {
		t.Fatalf("save error = %v, want %v", err, saveErr)
	}
	if runtime.current != nil || len(runtime.saved) != 0 {
		t.Fatalf("failed save mutated runtime: current=%#v saved=%#v", runtime.current, runtime.saved)
	}
}

func TestGoalCommandRequiresRuntime(t *testing.T) {
	_, err := executeGoalCommand(t, nil, "status")
	if err == nil {
		t.Fatal("expected missing goal runtime error")
	}
	assertContainsFold(t, err.Error(), "goal runtime")
}

func executeGoalCommand(t *testing.T, runtime commands.GoalRuntime, args string) (string, error) {
	t.Helper()
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)
	cmd := registry.Find("goal")
	if cmd == nil {
		t.Fatal("/goal is not registered")
	}
	var output strings.Builder
	err := cmd.Execute(&commands.Context{
		GoalRuntime: runtime,
		OnEvent: func(value string) {
			if output.Len() > 0 {
				output.WriteByte('\n')
			}
			output.WriteString(value)
		},
	}, args)
	return output.String(), err
}

func mustCreateGoal(t *testing.T, objective string, tokenBudget int) goal.Goal {
	t.Helper()
	created, err := goal.Create(objective, tokenBudget, time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func mustAchieveGoal(t *testing.T, current goal.Goal) goal.Goal {
	t.Helper()
	criteria := current.Criteria()
	results := make([]goal.AcceptanceCriterionEvaluation, 0, len(criteria))
	for _, criterion := range criteria {
		results = append(results, goal.AcceptanceCriterionEvaluation{
			CriterionID: criterion.ID, Met: true, Reason: "verified",
		})
	}
	evaluated, err := goal.RecordAcceptanceEvaluation(current, goal.Normalize(current).Revision, results, "verified", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	achieved, err := goal.Achieve(evaluated, "verified", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return achieved
}

func assertContainsFold(t *testing.T, value, want string) {
	t.Helper()
	if !strings.Contains(strings.ToLower(value), strings.ToLower(want)) {
		t.Fatalf("%q does not contain %q", value, want)
	}
}

func assertContainsAnyFold(t *testing.T, value string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if strings.Contains(strings.ToLower(value), strings.ToLower(want)) {
			return
		}
	}
	t.Fatalf("%q does not contain any of %q", value, wants)
}
