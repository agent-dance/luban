package commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

func TestSkillsVisibilitySetAcrossScopesAndStates(t *testing.T) {
	visibilities := []skills.Visibility{
		skills.VisibilityAuto,
		skills.VisibilityNameOnly,
		skills.VisibilityManualOnly,
		skills.VisibilityOff,
	}
	scopes := []skills.SkillScope{
		skills.SkillScopeSession,
		skills.SkillScopeProject,
		skills.SkillScopeUser,
	}
	for _, scope := range scopes {
		for _, visibility := range visibilities {
			t.Run(string(scope)+"/"+string(visibility), func(t *testing.T) {
				backend := newTask15Backend(t, task15Skill("skill:project:alpha", "alpha", skills.SourceProject))
				output, outcome := executeTask15Skills(t, backend,
					"set alpha "+string(visibility)+" --scope "+string(scope))
				if len(backend.setCalls) != 1 {
					t.Fatalf("set calls = %#v", backend.setCalls)
				}
				call := backend.setCalls[0]
				if call.SkillID != "skill:project:alpha" || call.Scope != scope || call.Visibility != visibility {
					t.Fatalf("set call = %#v", call)
				}
				for _, want := range []string{string(visibility), string(scope), "Effective visibility", "source:"} {
					if !strings.Contains(output, want) {
						t.Fatalf("output omitted %q: %s", want, output)
					}
				}
				if outcome != CommandOutcomeSucceeded {
					t.Fatalf("outcome = %s", outcome)
				}
			})
		}
	}
}

func TestSkillsScopeResetReportsInheritedState(t *testing.T) {
	row := task15Skill("skill:project:alpha", "alpha", skills.SourceProject)
	row.Visibility = skills.VisibilityNameOnly
	row.VisibilitySource = skills.SkillScopeProject
	row.DescriptionVisible = false
	backend := newTask15Backend(t, row)

	output, outcome := executeTask15Skills(t, backend, "reset alpha --scope project")
	if len(backend.resetCalls) != 1 || backend.resetCalls[0].scope != skills.SkillScopeProject {
		t.Fatalf("reset calls = %#v", backend.resetCalls)
	}
	for _, want := range []string{"Reset skill", "project", "auto", "default"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output omitted %q: %s", want, output)
		}
	}
	if outcome != CommandOutcomeSucceeded {
		t.Fatalf("outcome = %s", outcome)
	}
}

func TestSkillsAmbiguousBareNameRequiresStableSelector(t *testing.T) {
	project := task15Skill("skill:project:alpha", "review", skills.SourceProject)
	user := task15Skill("skill:user:alpha", "review", skills.SourceUser)
	user.ShadowedBy = project.ID
	user.ModelVisible = false
	user.DescriptionVisible = false
	user.Executable = false
	backend := newTask15Backend(t, project, user)

	output, outcome := executeTask15Skills(t, backend, "set review off --scope project")
	if len(backend.setCalls) != 0 {
		t.Fatalf("ambiguous name mutated backend: %#v", backend.setCalls)
	}
	for _, want := range []string{"ambiguous", string(project.ID), string(user.ID)} {
		if !strings.Contains(output, want) {
			t.Fatalf("output omitted %q: %s", want, output)
		}
	}
	if outcome != CommandOutcomeFailed {
		t.Fatalf("outcome = %s", outcome)
	}

	output, outcome = executeTask15Skills(t, backend, "set "+string(project.ID)+" off --scope project")
	if len(backend.setCalls) != 1 || backend.setCalls[0].SkillID != project.ID {
		t.Fatalf("stable selector call = %#v", backend.setCalls)
	}
	if outcome != CommandOutcomeSucceeded || !strings.Contains(output, string(project.ID)) {
		t.Fatalf("stable selector output=%q outcome=%s", output, outcome)
	}
}

func TestSkillsShowShadowedStableIDReportsEffectiveDisabledStatus(t *testing.T) {
	winner := task15Skill("skill:project:alpha", "review", skills.SourceProject)
	shadowed := task15Skill("skill:user:alpha", "review", skills.SourceUser)
	shadowed.ShadowedBy = winner.ID
	shadowed.ModelVisible = false
	shadowed.DescriptionVisible = false
	shadowed.Executable = false
	backend := newTask15Backend(t, winner, shadowed)

	output, outcome := executeTask15Skills(t, backend, "show "+string(shadowed.ID))
	if outcome != CommandOutcomeSucceeded {
		t.Fatalf("outcome = %s, output = %q", outcome, output)
	}
	for _, want := range []string{"Status: disabled", "Shadowed by: " + string(winner.ID)} {
		if !strings.Contains(output, want) {
			t.Fatalf("show output omitted %q: %q", want, output)
		}
	}
}

func TestSkillsVisibilityListShowsEveryAuthorityField(t *testing.T) {
	row := task15Skill("skill:project:alpha", "alpha", skills.SourceProject)
	row.Visibility = skills.VisibilityManualOnly
	row.VisibilitySource = skills.SkillScopeSession
	row.ModelVisible = false
	row.DescriptionVisible = false
	backend := newTask15Backend(t, row)

	output, outcome := executeTask15Skills(t, backend, "list")
	for _, want := range []string{
		"Catalog revision: 1", "[enabled] alpha", string(row.ID), string(row.Source),
		string(row.Locator), string(row.Digest), "Skill revision: 1", "manual-only",
		"State source: session", "Mutable: yes",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("list omitted %q:\n%s", want, output)
		}
	}
	if outcome != CommandOutcomeSucceeded {
		t.Fatalf("outcome = %s", outcome)
	}
}

func TestSkillsVisibilityReadOnlyAndBackendFailuresAreExplicit(t *testing.T) {
	row := task15Skill("skill:managed:alpha", "managed", skills.SourceManaged)
	row.Mutable = false
	row.ReadOnlyReason = string(skills.CatalogPolicyReasonManagedReadOnly)
	backend := newTask15Backend(t, row)

	output, outcome := executeTask15Skills(t, backend, "set managed off --scope project")
	if len(backend.setCalls) != 0 || outcome != CommandOutcomeFailed {
		t.Fatalf("read-only mutation calls=%#v outcome=%s", backend.setCalls, outcome)
	}
	if !strings.Contains(output, "read-only") ||
		!strings.Contains(output, i18n.Text(i18n.LangEN, i18n.KeyCommandSkillsReadOnlyManaged)) ||
		strings.Contains(output, string(skills.CatalogPolicyReasonManagedReadOnly)) {
		t.Fatalf("read-only output = %q", output)
	}

	const diagnostic = "parse /secret/project/settings.json: catalog offline"
	for _, lang := range i18n.AllLanguages() {
		backend = newTask15Backend(t, task15Skill("skill:project:alpha", "alpha", skills.SourceProject))
		backend.snapshotErr = errors.New(diagnostic)
		output, outcome = executeTask15SkillsLanguage(t, backend, lang, "list")
		if outcome != CommandOutcomeFailed ||
			!strings.Contains(output, i18n.Text(lang, i18n.KeyAuxSkillFailed)) ||
			strings.Contains(output, diagnostic) || strings.Contains(output, "/secret/project/settings.json") {
			t.Fatalf("backend failure for %s output=%q outcome=%s", lang.Code(), output, outcome)
		}
	}
}

func TestSkillsVisibilityReadOnlyPolicyCodesAreLocalizedOnEverySurface(t *testing.T) {
	tests := []struct {
		reason skills.CatalogPolicyReason
		key    i18n.Key
		deny   bool
	}{
		{skills.CatalogPolicyReasonManagedReadOnly, i18n.KeyCommandSkillsReadOnlyManaged, false},
		{skills.CatalogPolicyReasonManagedDeny, i18n.KeyCommandSkillsReadOnlyDenied, true},
	}
	for _, test := range tests {
		t.Run(string(test.reason), func(t *testing.T) {
			row := task15Skill("skill:managed:alpha", "managed", skills.SourceManaged)
			row.Mutable = false
			row.ReadOnlyReason = string(test.reason)
			if test.deny {
				row.Visibility = skills.VisibilityOff
				row.VisibilitySource = skills.SkillScopeManaged
				row.ModelVisible = false
				row.DescriptionVisible = false
				row.UserInvocable = false
				row.Executable = false
			}
			backend := newTask15Backend(t, row)
			result := skills.ProjectVisibilityToggleResult{
				Outcome:          skills.ProjectVisibilityToggleRejected,
				Reason:           skills.ProjectVisibilityToggleReasonReadOnly,
				RequestedSkillID: row.ID,
				ObservedRevision: backend.snapshot.Revision,
				CurrentRevision:  backend.snapshot.Revision,
				Skill:            &row,
				Snapshot:         backend.snapshot,
			}
			if err := result.Validate(); err != nil {
				t.Fatalf("invalid fixture: %v", err)
			}

			for _, lang := range i18n.AllLanguages() {
				want := i18n.Text(lang, test.key)
				outputs := []string{
					formatSkillsList(lang, backend.snapshot, "session-a"),
				}
				commandOutput, outcome := executeTask15SkillsLanguage(t, backend, lang, "set managed off --scope project")
				if outcome != CommandOutcomeFailed {
					t.Fatalf("language %s outcome = %s", lang.Code(), outcome)
				}
				outputs = append(outputs, commandOutput)
				for _, output := range outputs {
					if !strings.Contains(output, want) {
						t.Errorf("language %s output omitted localized reason %q: %q", lang.Code(), want, output)
					}
					if strings.Contains(output, string(test.reason)) {
						t.Errorf("language %s leaked policy code: %q", lang.Code(), output)
					}
				}
			}
		})
	}
}

func TestSkillsInteractiveBackendPreservesTypedAuthoritativeResult(t *testing.T) {
	row := task15Skill("skill:project:alpha", "alpha", skills.SourceProject)
	backend := newTask15Backend(t, row)
	tests := []struct {
		outcome skills.ProjectVisibilityToggleOutcome
		reason  skills.ProjectVisibilityToggleReason
	}{
		{skills.ProjectVisibilityToggleCommitted, skills.ProjectVisibilityToggleReasonNone},
		{skills.ProjectVisibilityToggleRejected, skills.ProjectVisibilityToggleReasonSessionOverride},
		{skills.ProjectVisibilityToggleRejected, skills.ProjectVisibilityToggleReasonStaleRevision},
		{skills.ProjectVisibilityToggleRejected, skills.ProjectVisibilityToggleReasonUnknownSkill},
		{skills.ProjectVisibilityToggleRejected, skills.ProjectVisibilityToggleReasonReadOnly},
		{skills.ProjectVisibilityToggleRejected, skills.ProjectVisibilityToggleReasonPersistenceFailed},
		{skills.ProjectVisibilityToggleRejected, skills.ProjectVisibilityToggleReasonLiveApplyRolledBack},
		{skills.ProjectVisibilityToggleDegraded, skills.ProjectVisibilityToggleReasonRollbackFailed},
		{skills.ProjectVisibilityToggleDegraded, skills.ProjectVisibilityToggleReasonAuthoritativeRefresh},
	}
	for _, test := range tests {
		backend.toggleResult = skills.ProjectVisibilityToggleResult{
			Outcome:          test.outcome,
			Reason:           test.reason,
			RequestedSkillID: row.ID,
			ObservedRevision: backend.snapshot.Revision,
			CurrentRevision:  backend.snapshot.Revision,
			Skill:            &row,
			Snapshot:         backend.snapshot,
		}
		if test.reason == skills.ProjectVisibilityToggleReasonUnknownSkill {
			backend.toggleResult.Skill = nil
		}
		if err := backend.toggleResult.Validate(); err != nil {
			t.Fatalf("invalid fixture: %v", err)
		}

		var interactive InteractiveSkillsBackend = backend
		got, err := interactive.ToggleProjectVisibility("session-a", row.ID, backend.snapshot.Revision)
		if err != nil || got.Outcome != test.outcome || got.Reason != test.reason ||
			got.Snapshot.Revision != backend.snapshot.Revision {
			t.Fatalf("toggle result = %#v, %v", got, err)
		}
	}
	if len(backend.toggleCalls) != len(tests) {
		t.Fatalf("toggle calls = %#v", backend.toggleCalls)
	}
	for _, call := range backend.toggleCalls {
		if call.sessionID != "session-a" || call.id != row.ID || call.revision != backend.snapshot.Revision {
			t.Fatalf("toggle call = %#v", call)
		}
	}
}

func TestSkillsVisibilityProjectTextOffFeedsInteractiveLastNonOff(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "alpha")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: Alpha skill\n---\n# Alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := skills.NewFileOverrideStoreAt(skills.OverrideStorePaths{
		UserSettings:    filepath.Join(root, "user-settings.json"),
		ProjectSettings: filepath.Join(root, "project-settings.json"),
	}, nil, skills.NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	manager := skills.NewManager(skills.DirSource{Dir: filepath.Join(root, "skills"), Source: skills.SourceProject})
	manager.SetOverrideStore(store)

	executeTask15Skills(t, manager, "set alpha manual-only --scope project")
	executeTask15Skills(t, manager, "set alpha off --scope project")
	snapshot, err := manager.Snapshot("session-a")
	if err != nil || len(snapshot.Skills) != 1 {
		t.Fatalf("snapshot = %#v, %v", snapshot, err)
	}
	id := snapshot.Skills[0].ID
	overrides, err := store.Snapshot("session-a")
	if err != nil {
		t.Fatal(err)
	}
	record := overrides.Project[id]
	if record.Visibility != skills.VisibilityOff || record.LastNonOff == nil || *record.LastNonOff != skills.VisibilityManualOnly {
		t.Fatalf("persisted project record = %#v", record)
	}

	toggled, err := manager.ToggleProjectVisibility("session-a", id, snapshot.Revision)
	if err != nil || toggled.Outcome != skills.ProjectVisibilityToggleCommitted || toggled.Skill == nil ||
		toggled.Skill.Visibility != skills.VisibilityManualOnly || toggled.Skill.VisibilitySource != skills.SkillScopeProject {
		t.Fatalf("interactive restore = %#v, %v", toggled, err)
	}
}

func TestSkillsVisibilityProjectTextOffDefaultsInteractiveRestoreToAuto(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "alpha")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ndescription: Alpha skill\n---\n# Alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := skills.NewFileOverrideStoreAt(skills.OverrideStorePaths{
		UserSettings:    filepath.Join(root, "user-settings.json"),
		ProjectSettings: filepath.Join(root, "project-settings.json"),
	}, nil, skills.NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	manager := skills.NewManager(skills.DirSource{Dir: filepath.Join(root, "skills"), Source: skills.SourceProject})
	manager.SetOverrideStore(store)

	executeTask15Skills(t, manager, "set alpha off --scope project")
	snapshot, err := manager.Snapshot("session-a")
	if err != nil || len(snapshot.Skills) != 1 {
		t.Fatalf("snapshot = %#v, %v", snapshot, err)
	}
	id := snapshot.Skills[0].ID
	overrides, err := store.Snapshot("session-a")
	if err != nil {
		t.Fatal(err)
	}
	record := overrides.Project[id]
	if record.Visibility != skills.VisibilityOff || record.LastNonOff == nil || *record.LastNonOff != skills.VisibilityAuto {
		t.Fatalf("persisted project record = %#v", record)
	}

	toggled, err := manager.ToggleProjectVisibility("session-a", id, snapshot.Revision)
	if err != nil || toggled.Outcome != skills.ProjectVisibilityToggleCommitted || toggled.Skill == nil ||
		toggled.Skill.Visibility != skills.VisibilityAuto || toggled.Skill.VisibilitySource != skills.SkillScopeProject {
		t.Fatalf("interactive default restore = %#v, %v", toggled, err)
	}
}

func TestSkillsSkillInvokerUsesCentralOriginAndPreservesOmittedArguments(t *testing.T) {
	var captured SkillInvocationRequest
	invoker := SkillInvokerFunc(func(_ context.Context, request SkillInvocationRequest) (types.ToolResult, error) {
		captured = request
		return types.ToolResult{Content: "ok"}, nil
	})
	request := SkillInvocationRequest{
		SessionID: "session-a", Selector: "alpha", ExpectedRevision: 3, ExpectedProjectGeneration: 7,
		Origin: skills.InvocationOriginUser,
	}
	result, err := invoker.InvokeSkill(context.Background(), request)
	if err != nil || result.Content != "ok" || captured.Arguments != nil || captured.Origin != skills.InvocationOriginUser {
		t.Fatalf("invoke result=%#v request=%#v err=%v", result, captured, err)
	}

	request.Origin = "unknown"
	if _, err := invoker.InvokeSkill(context.Background(), request); err == nil {
		t.Fatal("unknown invocation origin was accepted")
	}

	request.Origin = skills.InvocationOriginUser
	request.ExpectedProjectGeneration = 0
	if _, err := invoker.InvokeSkill(context.Background(), request); err == nil {
		t.Fatal("unpinned project generation was accepted")
	}
}

type task15ResetCall struct {
	scope skills.SkillScope
	id    skills.SkillID
}

type task15ToggleCall struct {
	sessionID string
	id        skills.SkillID
	revision  skills.CatalogRevision
}

type task15Backend struct {
	t            *testing.T
	snapshot     skills.CatalogSnapshot
	snapshotErr  error
	setCalls     []skills.VisibilityOverride
	resetCalls   []task15ResetCall
	toggleResult skills.ProjectVisibilityToggleResult
	toggleErr    error
	toggleCalls  []task15ToggleCall
}

func newTask15Backend(t *testing.T, rows ...skills.EffectiveSkill) *task15Backend {
	t.Helper()
	snapshot, err := skills.NewCatalogSnapshot(1, rows)
	if err != nil {
		t.Fatal(err)
	}
	return &task15Backend{t: t, snapshot: snapshot}
}

func (backend *task15Backend) Snapshot(string) (skills.CatalogSnapshot, error) {
	return backend.snapshot.Clone(), backend.snapshotErr
}

func (backend *task15Backend) SnapshotBinding(string) (skills.CatalogBinding, error) {
	return skills.CatalogBinding{ProjectGeneration: 1, Snapshot: backend.snapshot.Clone()}, backend.snapshotErr
}

func (backend *task15Backend) ResolveLatest(request skills.SkillResolveRequest, _ func(skills.ResolvedSkill) error) (skills.SkillResolveResult, error) {
	id := skills.SkillID(request.Selector)
	row, found := backend.snapshot.Find(id)
	if !found {
		return skills.SkillResolveResult{Outcome: skills.SkillResolveNotFound, CatalogRevision: backend.snapshot.Revision}, nil
	}
	resolved := skills.ResolvedSkill{
		Effective: row,
		Skill: &skills.Skill{
			Name: row.Name, Description: row.Summary, Source: row.Source,
			FilePath: string(row.Locator), SkillDir: "/skills/" + row.Name,
		},
	}
	outcome := skills.SkillResolveResolved
	if request.ExpectedRevision != 0 && request.ExpectedRevision != row.Revision {
		outcome = skills.SkillResolveStale
	}
	return skills.SkillResolveResult{
		Outcome: outcome, CatalogRevision: backend.snapshot.Revision, Resolved: &resolved,
	}, nil
}

func (backend *task15Backend) SetVisibility(_ string, override skills.VisibilityOverride) (skills.CatalogSnapshot, error) {
	backend.setCalls = append(backend.setCalls, override)
	return backend.update(override.SkillID, override.Visibility, override.Scope), nil
}

func (backend *task15Backend) ResetVisibility(_ string, scope skills.SkillScope, id skills.SkillID) (skills.CatalogSnapshot, error) {
	backend.resetCalls = append(backend.resetCalls, task15ResetCall{scope: scope, id: id})
	return backend.update(id, skills.VisibilityAuto, skills.SkillScopeDefault), nil
}

func (backend *task15Backend) RefreshSnapshot(string) (skills.CatalogSnapshot, error) {
	return backend.snapshot.Clone(), backend.snapshotErr
}

func (backend *task15Backend) ToggleProjectVisibility(sessionID string, id skills.SkillID, revision skills.CatalogRevision) (skills.ProjectVisibilityToggleResult, error) {
	backend.toggleCalls = append(backend.toggleCalls, task15ToggleCall{sessionID: sessionID, id: id, revision: revision})
	return backend.toggleResult, backend.toggleErr
}

func (backend *task15Backend) update(id skills.SkillID, visibility skills.Visibility, scope skills.SkillScope) skills.CatalogSnapshot {
	rows := append([]skills.EffectiveSkill(nil), backend.snapshot.Skills...)
	for index := range rows {
		if rows[index].ID != id {
			continue
		}
		row := &rows[index]
		row.Visibility = visibility
		row.VisibilitySource = scope
		row.Revision++
		switch visibility {
		case skills.VisibilityAuto:
			row.ModelVisible, row.DescriptionVisible, row.UserInvocable, row.Executable = true, true, true, true
		case skills.VisibilityNameOnly:
			row.ModelVisible, row.DescriptionVisible, row.UserInvocable, row.Executable = true, false, true, true
		case skills.VisibilityManualOnly:
			row.ModelVisible, row.DescriptionVisible, row.UserInvocable, row.Executable = false, false, true, true
		case skills.VisibilityOff:
			row.ModelVisible, row.DescriptionVisible, row.UserInvocable, row.Executable = false, false, false, false
		}
	}
	snapshot, err := skills.NewCatalogSnapshot(backend.snapshot.Revision+1, rows)
	if err != nil {
		backend.t.Fatal(err)
	}
	backend.snapshot = snapshot
	return snapshot.Clone()
}

func task15Skill(id skills.SkillID, name string, source skills.SkillSource) skills.EffectiveSkill {
	return skills.EffectiveSkill{
		ID: id, Name: name, Summary: "Summary for " + name, Source: source,
		Locator:  skills.SkillLocator("/skills/" + name + "/SKILL.md"),
		Digest:   skills.SkillDigest("sha256:" + strings.Repeat("a", 64)),
		Revision: 1, Visibility: skills.VisibilityAuto, VisibilitySource: skills.SkillScopeDefault,
		ModelVisible: true, DescriptionVisible: true, UserInvocable: true, Executable: true, Mutable: true,
	}
}

func executeTask15Skills(t *testing.T, backend SkillsBackend, args string) (string, CommandOutcome) {
	return executeTask15SkillsLanguage(t, backend, i18n.LangEN, args)
}

func executeTask15SkillsLanguage(t *testing.T, backend SkillsBackend, lang i18n.Language, args string) (string, CommandOutcome) {
	t.Helper()
	var output strings.Builder
	outcome := CommandOutcomeUnknown
	ctx := &Context{
		Language: lang, SessionID: "session-a", SkillManager: backend,
		OnEvent:               func(value string) { output.WriteString(value) },
		OnCommandDomainResult: func(result CommandDomainResult) { outcome = result.Outcome },
	}
	if err := NewSkillsCommand().Execute(ctx, args); err != nil {
		t.Fatal(err)
	}
	return output.String(), outcome
}

var _ SkillsBackend = (*task15Backend)(nil)
