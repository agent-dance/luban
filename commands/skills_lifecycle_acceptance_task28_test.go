package commands

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/skills"
)

func TestSkillsLifecycleAcceptanceRealScopesSetResetAndPersistence(t *testing.T) {
	for _, scope := range []skills.SkillScope{skills.SkillScopeSession, skills.SkillScopeProject, skills.SkillScopeUser} {
		for _, visibility := range []skills.Visibility{
			skills.VisibilityAuto,
			skills.VisibilityNameOnly,
			skills.VisibilityManualOnly,
			skills.VisibilityOff,
		} {
			t.Run(string(scope)+"/"+string(visibility), func(t *testing.T) {
				fixture := newTask28CommandFixture(t, nil, task28CommandSource{source: skills.SourceProject, directory: "alpha", name: "alpha"})
				output, outcome := task28ExecuteSkills(t, fixture.manager,
					"set alpha "+string(visibility)+" --scope "+string(scope))
				if outcome != CommandOutcomeSucceeded || !strings.Contains(output, string(visibility)) || !strings.Contains(output, string(scope)) {
					t.Fatalf("set outcome=%s output=%q", outcome, output)
				}
				row := task28CommandRow(t, fixture.manager, "session-a", fixture.ids[0])
				if row.Visibility != visibility || row.VisibilitySource != scope {
					t.Fatalf("session-a row=%#v", row)
				}

				other := task28CommandRow(t, fixture.manager, "session-b", fixture.ids[0])
				if scope == skills.SkillScopeSession {
					if other.Visibility != skills.VisibilityAuto || other.VisibilitySource != skills.SkillScopeDefault {
						t.Fatalf("session override leaked=%#v", other)
					}
				} else if other.Visibility != visibility || other.VisibilitySource != scope {
					t.Fatalf("persistent override missing from second session=%#v", other)
				}

				fresh := fixture.reopen(t)
				freshRow := task28CommandRow(t, fresh, "fresh-session", fixture.ids[0])
				if scope == skills.SkillScopeSession {
					if freshRow.Visibility != skills.VisibilityAuto || freshRow.VisibilitySource != skills.SkillScopeDefault {
						t.Fatalf("session override survived fresh process=%#v", freshRow)
					}
				} else if freshRow.Visibility != visibility || freshRow.VisibilitySource != scope {
					t.Fatalf("fresh persistent row=%#v", freshRow)
				}

				resetOutput, resetOutcome := task28ExecuteSkills(t, fixture.manager,
					"reset alpha --scope "+string(scope))
				if resetOutcome != CommandOutcomeSucceeded || !strings.Contains(resetOutput, "default") {
					t.Fatalf("reset outcome=%s output=%q", resetOutcome, resetOutput)
				}
				inherited := task28CommandRow(t, fixture.manager, "session-a", fixture.ids[0])
				if inherited.Visibility != skills.VisibilityAuto || inherited.VisibilitySource != skills.SkillScopeDefault {
					t.Fatalf("reset did not inherit default=%#v", inherited)
				}
			})
		}
	}
}

func TestSkillsLifecycleAcceptanceManagedPolicyAndResetAreReadOnly(t *testing.T) {
	base := newTask28CommandFixture(t, nil, task28CommandSource{source: skills.SourceProject, directory: "locked", name: "locked"})
	id := base.ids[0]
	managed := map[skills.SkillID]skills.VisibilityOverride{id: {
		SkillID: id, Scope: skills.SkillScopeManaged, Visibility: skills.VisibilityOff,
	}}
	fixture := newTask28CommandFixtureAt(t, base.root, base.paths, managed, base.sources)
	before, err := fixture.store.Snapshot("session-a")
	if err != nil {
		t.Fatal(err)
	}
	row := task28CommandRow(t, fixture.manager, "session-a", id)
	if row.Mutable || row.Visibility != skills.VisibilityOff || row.VisibilitySource != skills.SkillScopeManaged || row.ReadOnlyReason == "" {
		t.Fatalf("managed row=%#v", row)
	}
	args := "set " + string(id) + " auto --scope project"
	output, outcome := task28ExecuteSkills(t, fixture.manager, args)
	if outcome != CommandOutcomeFailed || !strings.Contains(strings.ToLower(output), "read-only") {
		t.Fatalf("managed command %q outcome=%s output=%q", args, outcome, output)
	}
	if _, resetErr := fixture.manager.ResetVisibility("session-a", skills.SkillScopeManaged, id); !errors.Is(resetErr, skills.ErrManagedOverrideReadOnly) {
		t.Fatalf("managed reset error=%v, want ErrManagedOverrideReadOnly", resetErr)
	}
	after, err := fixture.store.Snapshot("session-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Project) != len(before.Project) || len(after.User) != len(before.User) || len(after.Session) != len(before.Session) {
		t.Fatalf("managed rejection mutated writable layers: before=%#v after=%#v", before, after)
	}
}

func TestSkillsLifecycleAcceptanceSameNameStableIDPersistenceIsolation(t *testing.T) {
	fixture := newTask28CommandFixture(t, nil,
		task28CommandSource{source: skills.SourceProject, directory: "project-review", name: "review"},
		task28CommandSource{source: skills.SourceUser, directory: "user-review", name: "review"},
	)
	initial, err := fixture.manager.Snapshot("session-a")
	if err != nil || len(initial.Skills) != 2 {
		t.Fatalf("initial=%#v err=%v", initial, err)
	}
	var winner, shadowed skills.EffectiveSkill
	for _, row := range initial.Skills {
		if row.ShadowedBy == "" {
			winner = row
		} else {
			shadowed = row
		}
	}
	if winner.ID == "" || shadowed.ID == "" || winner.ID == shadowed.ID || shadowed.ShadowedBy != winner.ID {
		t.Fatalf("same-name ownership winner=%#v shadowed=%#v", winner, shadowed)
	}
	output, outcome := task28ExecuteSkills(t, fixture.manager, "set review off --scope project")
	if outcome != CommandOutcomeFailed || !strings.Contains(output, string(winner.ID)) || !strings.Contains(output, string(shadowed.ID)) {
		t.Fatalf("ambiguous mutation outcome=%s output=%q", outcome, output)
	}

	if _, outcome = task28ExecuteSkills(t, fixture.manager, "set "+string(shadowed.ID)+" off --scope project"); outcome != CommandOutcomeSucceeded {
		t.Fatalf("stable-ID mutation outcome=%s", outcome)
	}
	persisted, err := fixture.store.Snapshot("session-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted.Project) != 1 || persisted.Project[shadowed.ID].Visibility != skills.VisibilityOff {
		t.Fatalf("project persistence targeted wrong identity=%#v", persisted.Project)
	}
	if _, exists := persisted.Project[winner.ID]; exists {
		t.Fatalf("winner received shadowed override=%#v", persisted.Project)
	}
	fresh := fixture.reopen(t)
	freshWinner := task28CommandRow(t, fresh, "fresh-session", winner.ID)
	freshShadow := task28CommandRow(t, fresh, "fresh-session", shadowed.ID)
	if freshWinner.Visibility == skills.VisibilityOff || freshShadow.Visibility != skills.VisibilityOff {
		t.Fatalf("fresh same-name rows winner=%#v shadow=%#v", freshWinner, freshShadow)
	}
	if _, outcome = task28ExecuteSkills(t, fresh, "reset "+string(shadowed.ID)+" --scope project"); outcome != CommandOutcomeSucceeded {
		t.Fatalf("stable-ID reset outcome=%s", outcome)
	}
}

func TestSkillsLifecycleAcceptanceProjectOffRestoresLastNonOffAfterFreshProcess(t *testing.T) {
	fixture := newTask28CommandFixture(t, nil, task28CommandSource{source: skills.SourceProject, directory: "manual", name: "manual"})
	id := fixture.ids[0]
	for _, args := range []string{
		"set " + string(id) + " manual-only --scope project",
		"set " + string(id) + " off --scope project",
	} {
		if _, outcome := task28ExecuteSkills(t, fixture.manager, args); outcome != CommandOutcomeSucceeded {
			t.Fatalf("%q outcome=%s", args, outcome)
		}
	}
	fresh := fixture.reopen(t)
	before, err := fresh.Snapshot("fresh-session")
	if err != nil {
		t.Fatal(err)
	}
	result, err := fresh.ToggleProjectVisibility("fresh-session", id, before.Revision)
	if err != nil || result.Outcome != skills.ProjectVisibilityToggleCommitted || result.Skill == nil ||
		result.Skill.Visibility != skills.VisibilityManualOnly || result.Skill.VisibilitySource != skills.SkillScopeProject {
		t.Fatalf("last-non-off restore=%#v err=%v", result, err)
	}
}

func TestSkillsLifecycleAcceptanceResetWalksSessionProjectUserDefaultPriority(t *testing.T) {
	fixture := newTask28CommandFixture(t, nil, task28CommandSource{
		source: skills.SourceProject, directory: "priority", name: "priority",
	})
	id := fixture.ids[0]
	steps := []struct {
		args       string
		visibility skills.Visibility
		scope      skills.SkillScope
	}{
		{"set " + string(id) + " name-only --scope user", skills.VisibilityNameOnly, skills.SkillScopeUser},
		{"set " + string(id) + " manual-only --scope project", skills.VisibilityManualOnly, skills.SkillScopeProject},
		{"set " + string(id) + " off --scope session", skills.VisibilityOff, skills.SkillScopeSession},
	}
	for _, step := range steps {
		output, outcome := task28ExecuteSkills(t, fixture.manager, step.args)
		if outcome != CommandOutcomeSucceeded {
			t.Fatalf("%q outcome=%s output=%q", step.args, outcome, output)
		}
		row := task28CommandRow(t, fixture.manager, "session-a", id)
		if row.Visibility != step.visibility || row.VisibilitySource != step.scope {
			t.Fatalf("%q row=%#v", step.args, row)
		}
	}

	resets := []struct {
		scope      skills.SkillScope
		visibility skills.Visibility
		inherited  skills.SkillScope
	}{
		{skills.SkillScopeSession, skills.VisibilityManualOnly, skills.SkillScopeProject},
		{skills.SkillScopeProject, skills.VisibilityNameOnly, skills.SkillScopeUser},
		{skills.SkillScopeUser, skills.VisibilityAuto, skills.SkillScopeDefault},
	}
	for _, reset := range resets {
		args := "reset " + string(id) + " --scope " + string(reset.scope)
		output, outcome := task28ExecuteSkills(t, fixture.manager, args)
		if outcome != CommandOutcomeSucceeded || !strings.Contains(output, string(reset.visibility)) ||
			!strings.Contains(output, string(reset.inherited)) {
			t.Fatalf("%q outcome=%s output=%q", args, outcome, output)
		}
		row := task28CommandRow(t, fixture.manager, "session-a", id)
		if row.Visibility != reset.visibility || row.VisibilitySource != reset.inherited {
			t.Fatalf("%q inherited row=%#v", args, row)
		}
	}

	persisted, err := fixture.store.Snapshot("session-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted.Session) != 0 || len(persisted.Project) != 0 || len(persisted.User) != 0 {
		t.Fatalf("reset chain left overrides behind: %#v", persisted)
	}
}

type task28CommandSource struct {
	source    skills.SkillSource
	directory string
	name      string
}

type task28CommandFixture struct {
	root    string
	paths   skills.OverrideStorePaths
	sources []skills.DirSource
	ids     []skills.SkillID
	store   *skills.FileOverrideStore
	manager *skills.Manager
}

func newTask28CommandFixture(t *testing.T, managed map[skills.SkillID]skills.VisibilityOverride, definitions ...task28CommandSource) *task28CommandFixture {
	t.Helper()
	root := t.TempDir()
	paths := skills.OverrideStorePaths{
		UserSettings:    filepath.Join(root, "settings", "user.json"),
		ProjectSettings: filepath.Join(root, "settings", "project.json"),
	}
	sources := make([]skills.DirSource, 0, len(definitions))
	for index, definition := range definitions {
		sourceRoot := filepath.Join(root, "source-"+string(rune('a'+index)))
		dir := filepath.Join(sourceRoot, definition.directory)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		body := "---\nname: " + definition.name + "\ndescription: " + definition.name + " acceptance\n---\nbody " + definition.name + "\n"
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		sources = append(sources, skills.DirSource{Dir: sourceRoot, Source: definition.source})
	}
	return newTask28CommandFixtureAt(t, root, paths, managed, sources)
}

func newTask28CommandFixtureAt(t *testing.T, root string, paths skills.OverrideStorePaths, managed map[skills.SkillID]skills.VisibilityOverride, sources []skills.DirSource) *task28CommandFixture {
	t.Helper()
	store, err := skills.NewFileOverrideStoreAt(paths, managed, skills.NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	manager := skills.NewManager(sources...)
	manager.SetOverrideStore(store)
	snapshot, err := manager.Snapshot("session-a")
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]skills.SkillID, len(snapshot.Skills))
	for index, row := range snapshot.Skills {
		ids[index] = row.ID
	}
	return &task28CommandFixture{root: root, paths: paths, sources: append([]skills.DirSource(nil), sources...), ids: ids, store: store, manager: manager}
}

func (fixture *task28CommandFixture) reopen(t *testing.T) *skills.Manager {
	t.Helper()
	store, err := skills.NewFileOverrideStoreAt(fixture.paths, nil, skills.NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	manager := skills.NewManager(fixture.sources...)
	manager.SetOverrideStore(store)
	return manager
}

func task28ExecuteSkills(t *testing.T, backend SkillsBackend, args string) (string, CommandOutcome) {
	t.Helper()
	var output strings.Builder
	outcome := CommandOutcomeUnknown
	ctx := &Context{
		Language:              i18n.LangEN,
		SessionID:             "session-a",
		SkillManager:          backend,
		OnEvent:               func(value string) { output.WriteString(value) },
		OnCommandDomainResult: func(result CommandDomainResult) { outcome = result.Outcome },
	}
	if err := NewSkillsCommand().Execute(ctx, args); err != nil {
		t.Fatal(err)
	}
	return output.String(), outcome
}

func task28CommandRow(t *testing.T, manager *skills.Manager, sessionID string, id skills.SkillID) skills.EffectiveSkill {
	t.Helper()
	snapshot, err := manager.Snapshot(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	row, found := snapshot.Find(id)
	if !found {
		t.Fatalf("skill %s missing from %#v", id, snapshot)
	}
	return row
}
