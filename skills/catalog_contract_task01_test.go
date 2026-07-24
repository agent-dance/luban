package skills

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

const catalogContractDigest SkillDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestSkillIDCatalogContractValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		id     SkillID
		valid  bool
		source SkillSource
	}{
		{name: "project stable ID", id: "skill:project:71da2", valid: true, source: SourceProject},
		{name: "virtual MCP ID", id: "skill:mcp:server/resource:one", valid: true, source: SourceMCP},
		{name: "locator may contain spaces", id: "skill:user:/Users/example/My Skills/review", valid: true, source: SourceUser},
		{name: "zero", id: "", valid: false},
		{name: "bare display name", id: "review", valid: false},
		{name: "missing identity", id: "skill:project:", valid: false},
		{name: "unknown source", id: "skill:somewhere:abc", valid: false},
		{name: "padded", id: " skill:project:abc", valid: false},
		{name: "control character", id: "skill:project:a\nb", valid: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.id.Validate()
			if test.valid && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if !test.valid && !errors.Is(err, ErrInvalidSkillID) {
				t.Fatalf("Validate() error = %v, want ErrInvalidSkillID", err)
			}
			if got := test.id.IsValid(); got != test.valid {
				t.Fatalf("IsValid() = %v, want %v", got, test.valid)
			}
			source, ok := test.id.Source()
			if ok != test.valid {
				t.Fatalf("Source() ok = %v, want %v", ok, test.valid)
			}
			if test.valid && source != test.source {
				t.Fatalf("Source() = %q, want %q", source, test.source)
			}
		})
	}
}

func TestVisibilityCatalogContractAndOverrideRoundTrip(t *testing.T) {
	t.Parallel()

	for _, visibility := range []Visibility{
		VisibilityAuto,
		VisibilityNameOnly,
		VisibilityManualOnly,
		VisibilityOff,
	} {
		if err := visibility.Validate(); err != nil {
			t.Errorf("%q Validate() error = %v", visibility, err)
		}
	}
	if err := Visibility("").Validate(); !errors.Is(err, ErrInvalidVisibility) {
		t.Fatalf("zero visibility error = %v, want ErrInvalidVisibility", err)
	}
	if VisibilityOff.IsNonOff() {
		t.Fatal("off visibility must not be remembered as non-off")
	}

	id := SkillID("skill:project:review")
	for _, remembered := range []Visibility{VisibilityAuto, VisibilityNameOnly, VisibilityManualOnly} {
		remembered := remembered
		override := VisibilityOverride{
			SkillID:    id,
			Scope:      SkillScopeProject,
			Visibility: VisibilityOff,
			LastNonOff: &remembered,
		}
		if err := override.Validate(); err != nil {
			t.Fatalf("Validate(%q) error = %v", remembered, err)
		}
		if got := override.RestoreVisibility(); got != remembered {
			t.Fatalf("RestoreVisibility() = %q, want %q", got, remembered)
		}

		data, err := json.Marshal(override)
		if err != nil {
			t.Fatal(err)
		}
		var decoded VisibilityOverride
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(decoded, override) {
			t.Fatalf("round trip = %#v, want %#v", decoded, override)
		}
	}

	var legacy VisibilityOverride
	if err := json.Unmarshal([]byte(`"off"`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Visibility != VisibilityOff || legacy.RestoreVisibility() != VisibilityAuto {
		t.Fatalf("legacy override = %#v, want off restoring auto", legacy)
	}
	legacy.SkillID = id
	legacy.Scope = SkillScopeProject
	if err := legacy.Validate(); err != nil {
		t.Fatalf("legacy override after map key attachment: %v", err)
	}

	invalidRemembered := VisibilityOff
	invalid := VisibilityOverride{
		SkillID: id, Scope: SkillScopeProject, Visibility: VisibilityOff, LastNonOff: &invalidRemembered,
	}
	if err := invalid.Validate(); !errors.Is(err, ErrInvalidVisibility) {
		t.Fatalf("invalid last_non_off error = %v", err)
	}
	nonOffWithHistory := VisibilityOverride{
		SkillID: id, Scope: SkillScopeProject, Visibility: VisibilityAuto, LastNonOff: visibilityPtr(VisibilityManualOnly),
	}
	if err := nonOffWithHistory.Validate(); !errors.Is(err, ErrInvalidVisibility) {
		t.Fatalf("non-off record with history error = %v", err)
	}
}

func TestCatalogContractSnapshotIsStableAndImmutableAtBoundary(t *testing.T) {
	t.Parallel()

	project := catalogContractSkill("skill:project:/repo/skills/review", "review", SourceProject, VisibilityAuto)
	user := catalogContractSkill("skill:user:/home/me/skills/review", "review", SourceUser, VisibilityNameOnly)
	input := []EffectiveSkill{user, project}

	snapshot, err := NewCatalogSnapshot(7, input)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Skills[0].ID != project.ID || snapshot.Skills[1].ID != user.ID {
		t.Fatalf("snapshot is not sorted by stable ID: %#v", snapshot.Skills)
	}
	input[0].Name = "mutated"
	if snapshot.Skills[1].Name != "review" {
		t.Fatal("constructor retained caller slice")
	}
	clone := snapshot.Clone()
	clone.Skills[0].Name = "mutated clone"
	if snapshot.Skills[0].Name != "review" {
		t.Fatal("Clone retained source slice")
	}
	if got, ok := snapshot.Find(user.ID); !ok || got.ID != user.ID {
		t.Fatalf("Find(%q) = %#v, %v", user.ID, got, ok)
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var decoded CatalogSnapshot
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, snapshot) {
		t.Fatalf("round trip = %#v, want %#v", decoded, snapshot)
	}

	wrongOwner := project
	wrongOwner.Source = SourceUser
	if err := wrongOwner.Validate(); !errors.Is(err, ErrInvalidSkillID) {
		t.Fatalf("source mismatch error = %v, want ErrInvalidSkillID", err)
	}
	if err := (CatalogSnapshot{}).Validate(); !errors.Is(err, ErrInvalidCatalogRevision) {
		t.Fatalf("zero snapshot error = %v, want ErrInvalidCatalogRevision", err)
	}
}

func TestCatalogContractDeltaRepresentsLifecycle(t *testing.T) {
	t.Parallel()

	project := catalogContractSkill("skill:project:/repo/skills/review", "review", SourceProject, VisibilityAuto)
	user := catalogContractSkill("skill:user:/home/me/skills/review", "review", SourceUser, VisibilityAuto)
	user.ShadowedBy = project.ID
	user.ModelVisible = false
	user.DescriptionVisible = false
	user.Executable = false

	delta, err := NewCatalogDelta(
		4,
		5,
		[]CatalogUpsert{
			{Skill: user, Reason: CatalogUpsertUpdated},
			{Skill: project, Reason: CatalogUpsertAdded},
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if delta.Upserts[0].Skill.ID != project.ID || delta.Upserts[1].Skill.ID != user.ID {
		t.Fatalf("upserts are not deterministically sorted: %#v", delta.Upserts)
	}

	for _, reason := range []CatalogRevokeReason{
		CatalogRevokeDisabled,
		CatalogRevokeDeleted,
		CatalogRevokeVisibility,
		CatalogRevokePermissionLost,
		CatalogRevokeShadowed,
	} {
		revokeDelta, err := NewCatalogDelta(5, 6, nil, []CatalogRevoke{{
			ID: project.ID, Name: project.Name, Source: project.Source,
			Locator: project.Locator, Revision: project.Revision, Reason: reason,
		}})
		if err != nil {
			t.Fatalf("revoke %q: %v", reason, err)
		}
		data, err := json.Marshal(revokeDelta)
		if err != nil {
			t.Fatal(err)
		}
		var decoded CatalogDelta
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatal(err)
		}
		if err := decoded.Validate(); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(decoded, revokeDelta) {
			t.Fatalf("round trip = %#v, want %#v", decoded, revokeDelta)
		}
	}

	reenabled, err := NewCatalogDelta(6, 7, []CatalogUpsert{{Skill: project, Reason: CatalogUpsertReenabled}}, nil)
	if err != nil || len(reenabled.Upserts) != 1 {
		t.Fatalf("re-enable delta = %#v, error %v", reenabled, err)
	}
	if err := (CatalogDelta{}).Validate(); err != nil {
		t.Fatalf("zero no-op delta error = %v", err)
	}
	if !((CatalogDelta{}).Empty()) {
		t.Fatal("zero delta must be empty")
	}
}

func TestCatalogContractProjectVisibilityToggleOutcomes(t *testing.T) {
	t.Parallel()

	request := ProjectVisibilityToggleRequest{
		SessionID: "session-1", SkillID: "skill:project:/repo/skills/review", ExpectedRevision: 8,
	}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (ProjectVisibilityToggleRequest{}).Validate(); err == nil {
		t.Fatal("zero toggle request unexpectedly valid")
	}

	skill := catalogContractSkill(request.SkillID, "review", SourceProject, VisibilityOff)
	snapshot, err := NewCatalogSnapshot(9, []EffectiveSkill{skill})
	if err != nil {
		t.Fatal(err)
	}
	committed := ProjectVisibilityToggleResult{
		Outcome: ProjectVisibilityToggleCommitted, RequestedSkillID: request.SkillID,
		ObservedRevision: 8, CurrentRevision: 9, Skill: &skill, Snapshot: snapshot,
	}
	if err := committed.Validate(); err != nil {
		t.Fatalf("committed result: %v", err)
	}
	if committed.RefreshRequired() {
		t.Fatal("committed result requested refresh")
	}

	rejected := committed
	rejected.Outcome = ProjectVisibilityToggleRejected
	rejected.Reason = ProjectVisibilityToggleReasonStaleRevision
	if err := rejected.Validate(); err != nil {
		t.Fatalf("rejected result: %v", err)
	}

	degraded := committed
	degraded.Outcome = ProjectVisibilityToggleDegraded
	degraded.Reason = ProjectVisibilityToggleReasonRollbackFailed
	if err := degraded.Validate(); err != nil {
		t.Fatalf("degraded result: %v", err)
	}
	if !degraded.RefreshRequired() {
		t.Fatal("degraded result must require refresh")
	}

	falseRollback := degraded
	falseRollback.Outcome = ProjectVisibilityToggleRejected
	if err := falseRollback.Validate(); err == nil {
		t.Fatal("rollback failure was incorrectly accepted as an ordinary rejection")
	}

	data, err := json.Marshal(degraded)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ProjectVisibilityToggleResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, degraded) {
		t.Fatalf("round trip = %#v, want %#v", decoded, degraded)
	}
}

func catalogContractSkill(id SkillID, name string, source SkillSource, visibility Visibility) EffectiveSkill {
	skill := EffectiveSkill{
		ID:                 id,
		Name:               name,
		Summary:            "Review changes",
		Source:             source,
		Locator:            SkillLocator("/skills/" + name),
		Digest:             catalogContractDigest,
		Revision:           1,
		Visibility:         visibility,
		VisibilitySource:   SkillScopeProject,
		ModelVisible:       true,
		DescriptionVisible: true,
		UserInvocable:      true,
		Executable:         true,
		Mutable:            true,
	}
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

func visibilityPtr(visibility Visibility) *Visibility { return &visibility }
