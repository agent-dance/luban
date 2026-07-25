package skills

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestCatalogDiffLifecycleAndTypedReasons(t *testing.T) {
	t.Parallel()

	base := task06Skill("skill:project:/repo/alpha", "shared", SourceProject, 1, VisibilityAuto)
	tests := []struct {
		name         string
		previous     []EffectiveSkill
		current      []EffectiveSkill
		wantUpsert   CatalogUpsertReason
		wantRevoke   CatalogRevokeReason
		wantEventID  SkillID
		wantNoEvents bool
	}{
		{name: "unchanged projection", previous: []EffectiveSkill{base}, current: []EffectiveSkill{base}, wantNoEvents: true},
		{name: "added", current: []EffectiveSkill{base}, wantUpsert: CatalogUpsertAdded, wantEventID: base.ID},
		{name: "deleted", previous: []EffectiveSkill{base}, wantRevoke: CatalogRevokeDeleted, wantEventID: base.ID},
		{name: "summary updated", previous: []EffectiveSkill{base}, current: []EffectiveSkill{task06WithSummary(base, "new summary", 2)}, wantUpsert: CatalogUpsertUpdated, wantEventID: base.ID},
		{name: "digest updated", previous: []EffectiveSkill{base}, current: []EffectiveSkill{task06WithDigest(base, "b", 2)}, wantUpsert: CatalogUpsertUpdated, wantEventID: base.ID},
		{name: "name only", previous: []EffectiveSkill{base}, current: []EffectiveSkill{task06WithVisibility(base, VisibilityNameOnly, 2)}, wantUpsert: CatalogUpsertUpdated, wantEventID: base.ID},
		{name: "disabled", previous: []EffectiveSkill{base}, current: []EffectiveSkill{task06WithVisibility(base, VisibilityOff, 2)}, wantRevoke: CatalogRevokeDisabled, wantEventID: base.ID},
		{name: "manual only", previous: []EffectiveSkill{base}, current: []EffectiveSkill{task06WithVisibility(base, VisibilityManualOnly, 2)}, wantRevoke: CatalogRevokeVisibility, wantEventID: base.ID},
		{name: "permission lost", previous: []EffectiveSkill{base}, current: []EffectiveSkill{task06WithoutPermission(base, 2)}, wantRevoke: CatalogRevokePermissionLost, wantEventID: base.ID},
		{name: "shadowed", previous: []EffectiveSkill{base}, current: []EffectiveSkill{task06Shadowed(base, "skill:user:/home/winner", 2)}, wantRevoke: CatalogRevokeShadowed, wantEventID: base.ID},
		{name: "re-enabled", previous: []EffectiveSkill{task06WithVisibility(base, VisibilityOff, 1)}, current: []EffectiveSkill{task06WithRevision(base, 2)}, wantUpsert: CatalogUpsertReenabled, wantEventID: base.ID},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			previous := task06Snapshot(t, 10, test.previous...)
			currentRevision := CatalogRevision(11)
			if test.wantNoEvents {
				currentRevision = 10
			}
			current := task06Snapshot(t, currentRevision, test.current...)
			delta, err := DiffCatalog(previous, current)
			if err != nil {
				t.Fatal(err)
			}
			if test.wantNoEvents {
				if !delta.Empty() {
					t.Fatalf("delta = %#v, want no events", delta)
				}
				return
			}
			if test.wantUpsert != "" {
				if len(delta.Upserts) != 1 || len(delta.Revokes) != 0 {
					t.Fatalf("delta = %#v, want one upsert", delta)
				}
				if got := delta.Upserts[0]; got.Skill.ID != test.wantEventID || got.Reason != test.wantUpsert {
					t.Fatalf("upsert = %#v, want ID %q reason %q", got, test.wantEventID, test.wantUpsert)
				}
				return
			}
			if len(delta.Revokes) != 1 || len(delta.Upserts) != 0 {
				t.Fatalf("delta = %#v, want one revoke", delta)
			}
			if got := delta.Revokes[0]; got.ID != test.wantEventID || got.Reason != test.wantRevoke {
				t.Fatalf("revoke = %#v, want ID %q reason %q", got, test.wantEventID, test.wantRevoke)
			}
		})
	}
}

func TestCatalogDiffUsesStableIDsAndDeterministicOrdering(t *testing.T) {
	t.Parallel()

	project := task06Skill("skill:project:/repo/shared", "shared", SourceProject, 1, VisibilityAuto)
	user := task06Skill("skill:user:/home/shared", "shared", SourceUser, 1, VisibilityAuto)
	bundled := task06Skill("skill:bundled:shared", "shared", SourceBundled, 1, VisibilityAuto)

	previous := task06Snapshot(t, 4, user, project)
	current := task06Snapshot(t, 5, bundled, task06WithVisibility(user, VisibilityOff, 2), project)
	delta, err := DiffCatalog(previous, current)
	if err != nil {
		t.Fatal(err)
	}
	if len(delta.Upserts) != 1 || delta.Upserts[0].Skill.ID != bundled.ID {
		t.Fatalf("upserts = %#v, want only bundled stable ID", delta.Upserts)
	}
	if len(delta.Revokes) != 1 || delta.Revokes[0].ID != user.ID || delta.Revokes[0].Reason != CatalogRevokeDisabled {
		t.Fatalf("revokes = %#v, want only disabled user stable ID", delta.Revokes)
	}
	if delta.Revokes[0].Name != project.Name {
		t.Fatalf("same display name fixture changed unexpectedly: %#v", delta.Revokes[0])
	}
	if delta.Revokes[0].ID == project.ID {
		t.Fatal("revoke targeted the same-name project skill")
	}

	empty := task06Snapshot(t, 6)
	all, err := DiffCatalog(empty, task06Snapshot(t, 7, user, bundled, project))
	if err != nil {
		t.Fatal(err)
	}
	gotIDs := []SkillID{all.Upserts[0].Skill.ID, all.Upserts[1].Skill.ID, all.Upserts[2].Skill.ID}
	wantIDs := []SkillID{bundled.ID, project.ID, user.ID}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("stable order = %v, want %v", gotIDs, wantIDs)
	}
}

func TestCatalogDiffHiddenStatesDoNotAnnounce(t *testing.T) {
	t.Parallel()

	auto := task06Skill("skill:project:/repo/hidden", "hidden", SourceProject, 1, VisibilityAuto)
	manual := task06WithVisibility(auto, VisibilityManualOnly, 2)
	off := task06WithVisibility(auto, VisibilityOff, 3)

	tests := []struct {
		name     string
		previous []EffectiveSkill
		current  []EffectiveSkill
	}{
		{name: "new manual-only skill", current: []EffectiveSkill{manual}},
		{name: "new off skill", current: []EffectiveSkill{off}},
		{name: "hidden state update", previous: []EffectiveSkill{manual}, current: []EffectiveSkill{off}},
		{name: "hidden skill deleted", previous: []EffectiveSkill{manual}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			delta, err := DiffCatalog(task06Snapshot(t, 30, test.previous...), task06Snapshot(t, 31, test.current...))
			if err != nil {
				t.Fatal(err)
			}
			if !delta.Empty() || delta.FromRevision != 30 || delta.ToRevision != 31 {
				t.Fatalf("delta = %#v, want revision-advancing no-op", delta)
			}
		})
	}
}

func TestCatalogRevokeOrderingUsesStableID(t *testing.T) {
	t.Parallel()

	project := task06Skill("skill:project:/repo/z", "same", SourceProject, 1, VisibilityAuto)
	user := task06Skill("skill:user:/home/a", "same", SourceUser, 1, VisibilityAuto)
	bundled := task06Skill("skill:bundled:m", "same", SourceBundled, 1, VisibilityAuto)
	delta, err := DiffCatalog(
		task06Snapshot(t, 1, user, project, bundled),
		task06Snapshot(t, 2,
			task06WithVisibility(user, VisibilityOff, 2),
			task06WithVisibility(project, VisibilityOff, 2),
			task06WithVisibility(bundled, VisibilityOff, 2),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []SkillID{bundled.ID, project.ID, user.ID}
	got := make([]SkillID, len(delta.Revokes))
	for index, revoke := range delta.Revokes {
		got[index] = revoke.ID
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("revoke order = %v, want %v", got, want)
	}
}

func TestCatalogDiffRevisionValidation(t *testing.T) {
	t.Parallel()

	base := task06Skill("skill:project:/repo/revision", "revision", SourceProject, 3, VisibilityAuto)
	tests := []struct {
		name     string
		previous CatalogSnapshot
		current  CatalogSnapshot
		want     error
	}{
		{name: "catalog regression", previous: task06Snapshot(t, 8, base), current: task06Snapshot(t, 7, base), want: ErrInvalidCatalogRevision},
		{name: "same catalog revision changed", previous: task06Snapshot(t, 8, base), current: task06Snapshot(t, 8, task06WithSummary(base, "changed", 4)), want: ErrInvalidCatalogRevision},
		{name: "skill revision regression", previous: task06Snapshot(t, 8, base), current: task06Snapshot(t, 9, task06WithRevision(base, 2)), want: ErrInvalidSkillRevision},
		{name: "state changed without skill revision", previous: task06Snapshot(t, 8, base), current: task06Snapshot(t, 9, task06WithSummary(base, "changed", 3)), want: ErrInvalidSkillRevision},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DiffCatalog(test.previous, test.current); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}

	unchangedAtNewRevision, err := DiffCatalog(task06Snapshot(t, 8, base), task06Snapshot(t, 20, base))
	if err != nil {
		t.Fatal(err)
	}
	if !unchangedAtNewRevision.Empty() || unchangedAtNewRevision.FromRevision != 8 || unchangedAtNewRevision.ToRevision != 20 {
		t.Fatalf("revision-only catalog delta = %#v", unchangedAtNewRevision)
	}
}

func TestCatalogDiffOutputIsImmutableAndDoesNotAliasSnapshots(t *testing.T) {
	t.Parallel()

	alpha := task06Skill("skill:project:/repo/alpha", "alpha", SourceProject, 1, VisibilityAuto)
	previous := task06Snapshot(t, 1)
	current := task06Snapshot(t, 2, alpha)
	delta, err := DiffCatalog(previous, current)
	if err != nil {
		t.Fatal(err)
	}
	delta.Upserts[0].Skill.Name = "mutated"
	if current.Skills[0].Name != "alpha" {
		t.Fatalf("delta retained current snapshot storage: %#v", current.Skills[0])
	}
	current.Skills[0].Summary = "mutated current"
	if delta.Upserts[0].Skill.Summary != "Review alpha" {
		t.Fatalf("current snapshot retained delta storage: %#v", delta.Upserts[0])
	}
}

func task06Snapshot(t *testing.T, revision CatalogRevision, input ...EffectiveSkill) CatalogSnapshot {
	t.Helper()
	snapshot, err := NewCatalogSnapshot(revision, input)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func task06Skill(id SkillID, name string, source SkillSource, revision SkillRevision, visibility Visibility) EffectiveSkill {
	skill := EffectiveSkill{
		ID:                 id,
		Name:               name,
		Summary:            "Review " + name,
		Source:             source,
		Locator:            SkillLocator("/skills/" + name),
		Digest:             SkillDigest("sha256:" + strings.Repeat("a", 64)),
		Revision:           revision,
		Visibility:         visibility,
		VisibilitySource:   SkillScopeProject,
		ModelVisible:       true,
		DescriptionVisible: true,
		UserInvocable:      true,
		Executable:         true,
		Mutable:            true,
	}
	return task06WithVisibility(skill, visibility, revision)
}

func task06WithRevision(skill EffectiveSkill, revision SkillRevision) EffectiveSkill {
	skill.Revision = revision
	return skill
}

func task06WithSummary(skill EffectiveSkill, summary string, revision SkillRevision) EffectiveSkill {
	skill.Summary = summary
	skill.Revision = revision
	return skill
}

func task06WithDigest(skill EffectiveSkill, value string, revision SkillRevision) EffectiveSkill {
	skill.Digest = SkillDigest("sha256:" + strings.Repeat(value, 64))
	skill.Revision = revision
	return skill
}

func task06WithVisibility(skill EffectiveSkill, visibility Visibility, revision SkillRevision) EffectiveSkill {
	skill.Revision = revision
	skill.Visibility = visibility
	skill.ModelVisible = true
	skill.DescriptionVisible = true
	skill.UserInvocable = true
	skill.Executable = true
	skill.ShadowedBy = ""
	switch visibility {
	case VisibilityNameOnly:
		skill.DescriptionVisible = false
	case VisibilityManualOnly:
		skill.ModelVisible = false
		skill.DescriptionVisible = false
	case VisibilityOff:
		skill.ModelVisible = false
		skill.DescriptionVisible = false
		skill.UserInvocable = false
		skill.Executable = false
	}
	return skill
}

func task06WithoutPermission(skill EffectiveSkill, revision SkillRevision) EffectiveSkill {
	skill.Revision = revision
	skill.ModelVisible = false
	skill.DescriptionVisible = false
	skill.Executable = false
	return skill
}

func task06Shadowed(skill EffectiveSkill, winner SkillID, revision SkillRevision) EffectiveSkill {
	skill.Revision = revision
	skill.ModelVisible = false
	skill.DescriptionVisible = false
	skill.Executable = false
	skill.ShadowedBy = winner
	return skill
}
