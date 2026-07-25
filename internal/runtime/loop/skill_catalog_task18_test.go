package loop

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

const skillCatalogCoordinatorDigest skills.SkillDigest = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

func TestSkillCatalogCoordinatorInitialSnapshot(t *testing.T) {
	t.Parallel()

	beta := skillCatalogCoordinatorSkill("beta", 1, skills.VisibilityAuto)
	alpha := skillCatalogCoordinatorSkill("alpha", 1, skills.VisibilityNameOnly)
	current := skillCatalogCoordinatorSnapshot(t, 7, beta, alpha)
	before := current.Clone()

	plan, err := PlanSkillCatalog(SkillCatalogCoordinatorInput{
		CurrentSnapshot: current,
		ContextEpoch:    "epoch-1",
		CharBudget:      10_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Kind != SkillCatalogPlanSnapshot || plan.RebuildReason != SkillCatalogRebuildInitial || !plan.HasMessage() {
		t.Fatalf("initial plan = %#v", plan)
	}
	assertSkillCatalogDeveloperMessage(t, *plan.Message, types.DeveloperMessageKindSkillCatalogSnapshot, 7)
	if !strings.Contains(plan.Message.GetText(), `"type":"skill_catalog_snapshot"`) {
		t.Fatalf("initial payload is not a snapshot: %s", plan.Message.GetText())
	}
	if plan.Cursor.ContextEpoch != "epoch-1" || plan.Cursor.AnnouncedRevision() != 7 || plan.Cursor.LedgerSnapshot.Revision != 7 {
		t.Fatalf("initial cursor = %#v", plan.Cursor)
	}
	if err := plan.Cursor.Validate(); err != nil {
		t.Fatalf("initial cursor validation: %v", err)
	}
	if !reflect.DeepEqual(current, before) {
		t.Fatalf("coordinator mutated current snapshot\n got: %#v\nwant: %#v", current, before)
	}

	plan.Cursor.LedgerSnapshot.Skills[0].Name = "mutated output"
	if current.Skills[0].Name == "mutated output" {
		t.Fatal("cursor retained the caller snapshot slice")
	}
	plan.Cursor.AnnouncedSnapshot.Skills[0].Name = "mutated announced output"
	if current.Skills[0].Name == "mutated announced output" {
		t.Fatal("announced cursor retained the caller snapshot slice")
	}
}

func TestSkillCatalogCoordinatorRebindsExactVisibleSnapshotAfterHistoryRestore(t *testing.T) {
	t.Parallel()

	current := skillCatalogCoordinatorSnapshot(t, 8,
		skillCatalogCoordinatorSkill("alpha", 1, skills.VisibilityAuto),
	)
	initial := mustPlanSkillCatalog(t, SkillCatalogCoordinatorInput{
		CurrentSnapshot: current,
		ContextEpoch:    "epoch-before-restore",
		CharBudget:      10_000,
	})
	history := []types.Message{
		*initial.Message,
		types.UserMessage("hello"),
		types.AssistantMessage("hello response"),
	}

	rebound := mustPlanSkillCatalog(t, SkillCatalogCoordinatorInput{
		CurrentSnapshot: current,
		ContextEpoch:    "epoch-after-restore",
		VisibleHistory:  history,
		CharBudget:      10_000,
	})
	if rebound.Kind != SkillCatalogPlanNone || rebound.HasMessage() {
		t.Fatalf("exact restored snapshot emitted a duplicate: %#v", rebound)
	}
	if rebound.Cursor.ContextEpoch != "epoch-after-restore" ||
		!reflect.DeepEqual(rebound.Cursor.AnnouncedSnapshot, current) ||
		!reflect.DeepEqual(rebound.Cursor.LedgerSnapshot, current) ||
		rebound.Cursor.VisibleMessageDigest != skillCatalogMessageDigest(*initial.Message) {
		t.Fatalf("restored snapshot cursor = %#v", rebound.Cursor)
	}
	if err := rebound.Cursor.Validate(); err != nil {
		t.Fatalf("restored snapshot cursor validation: %v", err)
	}
}

func TestSkillCatalogCoordinatorRejectsUnsafeVisibleSnapshotRebind(t *testing.T) {
	t.Parallel()

	current := skillCatalogCoordinatorSnapshot(t, 8,
		skillCatalogCoordinatorSkill("alpha", 1, skills.VisibilityAuto),
	)
	initial := mustPlanSkillCatalog(t, SkillCatalogCoordinatorInput{
		CurrentSnapshot: current,
		ContextEpoch:    "epoch-before-restore",
		CharBudget:      10_000,
	})

	tests := []struct {
		name   string
		mutate func(types.Message) types.Message
	}{
		{
			name: "tampered text",
			mutate: func(message types.Message) types.Message {
				return types.DeveloperMessage(message.GetText()+" tampered", *message.DeveloperMetadata)
			},
		},
		{
			name: "not meta",
			mutate: func(message types.Message) types.Message {
				message.IsMeta = false
				return message
			},
		},
		{
			name: "delta metadata",
			mutate: func(message types.Message) types.Message {
				message.DeveloperMetadata = &types.DeveloperMessageMetadata{
					Kind:     types.DeveloperMessageKindSkillCatalogDelta,
					Revision: message.DeveloperMetadata.Revision,
				}
				return message
			},
		},
		{
			name: "wrong revision",
			mutate: func(message types.Message) types.Message {
				message.DeveloperMetadata = &types.DeveloperMessageMetadata{
					Kind:     message.DeveloperMetadata.Kind,
					Revision: message.DeveloperMetadata.Revision - 1,
				}
				return message
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			history := []types.Message{
				test.mutate(*initial.Message),
				types.UserMessage("hello"),
				types.AssistantMessage("hello response"),
			}
			plan := mustPlanSkillCatalog(t, SkillCatalogCoordinatorInput{
				CurrentSnapshot: current,
				ContextEpoch:    "epoch-after-restore",
				VisibleHistory:  history,
				CharBudget:      10_000,
			})
			if plan.Kind != SkillCatalogPlanSnapshot || !plan.HasMessage() || plan.RebuildReason != SkillCatalogRebuildInitial {
				t.Fatalf("unsafe restored snapshot plan = %#v", plan)
			}
		})
	}
}

func TestSkillCatalogCoordinatorNoChangeProducesNoMessage(t *testing.T) {
	t.Parallel()

	alpha := skillCatalogCoordinatorSkill("alpha", 1, skills.VisibilityAuto)
	initialSnapshot := skillCatalogCoordinatorSnapshot(t, 1, alpha)
	initial := mustPlanSkillCatalog(t, SkillCatalogCoordinatorInput{
		CurrentSnapshot: initialSnapshot,
		ContextEpoch:    "epoch-stable",
		CharBudget:      10_000,
	})
	history := []types.Message{*initial.Message, types.UserMessage("current history tail")}

	// The authoritative catalog revision may advance without changing the
	// model-facing snapshot. The ledger advances while the visible revision
	// remains attached to the last developer message in history.
	registryOnlyRevision := skillCatalogCoordinatorSnapshot(t, 2, alpha)
	noChange := mustPlanSkillCatalog(t, SkillCatalogCoordinatorInput{
		CurrentSnapshot: registryOnlyRevision,
		PriorCursor:     initial.Cursor,
		ContextEpoch:    "epoch-stable",
		VisibleHistory:  history,
		CharBudget:      10_000,
	})
	if noChange.Kind != SkillCatalogPlanNone || noChange.HasMessage() || !noChange.Render.Empty() {
		t.Fatalf("no-change plan emitted a message: %#v", noChange)
	}
	if noChange.Cursor.LedgerSnapshot.Revision != 2 || noChange.Cursor.AnnouncedRevision() != 1 {
		t.Fatalf("silent revision cursor = %#v", noChange.Cursor)
	}
	if !reflect.DeepEqual(noChange.Cursor.AnnouncedSnapshot, initial.Cursor.AnnouncedSnapshot) {
		t.Fatalf("silent revision changed announced snapshot: %#v", noChange.Cursor)
	}
	if err := noChange.Cursor.Validate(); err != nil {
		t.Fatalf("silent revision cursor validation: %v", err)
	}

	second := mustPlanSkillCatalog(t, SkillCatalogCoordinatorInput{
		CurrentSnapshot: registryOnlyRevision,
		PriorCursor:     noChange.Cursor,
		ContextEpoch:    "epoch-stable",
		VisibleHistory:  history,
		CharBudget:      10_000,
	})
	if second.Kind != SkillCatalogPlanNone || second.HasMessage() {
		t.Fatalf("stable silent revision forced a rebuild: %#v", second)
	}

	changedAlpha := alpha
	changedAlpha.Summary = "visible change after silent revision"
	changedAlpha.Revision = 2
	visibleChange := skillCatalogCoordinatorSnapshot(t, 3, changedAlpha)
	deltaPlan := mustPlanSkillCatalog(t, SkillCatalogCoordinatorInput{
		CurrentSnapshot: visibleChange,
		PriorCursor:     noChange.Cursor,
		ContextEpoch:    "epoch-stable",
		VisibleHistory:  history,
		CharBudget:      10_000,
	})
	var delta skillCatalogCoordinatorDeltaWire
	if err := json.Unmarshal([]byte(deltaPlan.Message.GetText()), &delta); err != nil {
		t.Fatal(err)
	}
	if delta.FromRevision != 1 || delta.ToRevision != 3 {
		t.Fatalf("delta after silent revision = %d -> %d, want 1 -> 3", delta.FromRevision, delta.ToRevision)
	}
	if deltaPlan.Cursor.AnnouncedRevision() != 3 || deltaPlan.Cursor.LedgerSnapshot.Revision != 3 {
		t.Fatalf("visible delta did not advance both ledgers: %#v", deltaPlan.Cursor)
	}
}

func TestSkillCatalogCoordinatorCoalescedCatalogDeltaLifecycle(t *testing.T) {
	t.Parallel()

	alpha := skillCatalogCoordinatorSkill("alpha", 1, skills.VisibilityAuto)
	beta := skillCatalogCoordinatorSkill("beta", 1, skills.VisibilityAuto)
	previous := skillCatalogCoordinatorSnapshot(t, 10, alpha, beta)
	initial := mustPlanSkillCatalog(t, SkillCatalogCoordinatorInput{
		CurrentSnapshot: previous,
		ContextEpoch:    "epoch-delta",
		CharBudget:      10_000,
	})
	history := []types.Message{types.UserMessage("older user"), *initial.Message, types.AssistantMessage("older assistant")}
	historyBefore := append([]types.Message(nil), history...)

	updatedAlpha := alpha
	updatedAlpha.Summary = "updated alpha"
	updatedAlpha.Revision = 2
	disabledBeta := skillCatalogCoordinatorSkill("beta", 2, skills.VisibilityOff)
	gamma := skillCatalogCoordinatorSkill("gamma", 1, skills.VisibilityAuto)
	current := skillCatalogCoordinatorSnapshot(t, 11, gamma, disabledBeta, updatedAlpha)

	plan := mustPlanSkillCatalog(t, SkillCatalogCoordinatorInput{
		CurrentSnapshot: current,
		PriorCursor:     initial.Cursor,
		ContextEpoch:    "epoch-delta",
		VisibleHistory:  history,
		CharBudget:      10_000,
	})
	if plan.Kind != SkillCatalogPlanDelta || plan.RebuildReason != "" || !plan.HasMessage() {
		t.Fatalf("delta plan = %#v", plan)
	}
	assertSkillCatalogDeveloperMessage(t, *plan.Message, types.DeveloperMessageKindSkillCatalogDelta, 11)
	if !reflect.DeepEqual(history, historyBefore) {
		t.Fatal("coordinator rewrote visible history")
	}
	var delta skillCatalogCoordinatorDeltaWire
	if err := json.Unmarshal([]byte(plan.Message.GetText()), &delta); err != nil {
		t.Fatal(err)
	}
	if delta.Type != "skill_catalog_delta" || delta.FromRevision != 10 || delta.ToRevision != 11 {
		t.Fatalf("delta revisions = %#v", delta)
	}
	if got := skillCatalogCoordinatorUpsertIDs(delta); !reflect.DeepEqual(got, []skills.SkillID{alpha.ID, gamma.ID}) {
		t.Fatalf("upsert IDs = %v", got)
	}
	if delta.Upserts[0].Reason != skills.CatalogUpsertUpdated || delta.Upserts[1].Reason != skills.CatalogUpsertAdded {
		t.Fatalf("upsert reasons = %#v", delta.Upserts)
	}
	if len(delta.Revokes) != 1 || delta.Revokes[0].ID != beta.ID || delta.Revokes[0].Reason != skills.CatalogRevokeDisabled {
		t.Fatalf("revokes = %#v", delta.Revokes)
	}
	if plan.Cursor.AnnouncedRevision() != 11 || !reflect.DeepEqual(plan.Cursor.AnnouncedSnapshot, current) || !reflect.DeepEqual(plan.Cursor.LedgerSnapshot, current) {
		t.Fatalf("delta cursor = %#v", plan.Cursor)
	}

	history = append(history, *plan.Message, types.UserMessage("next user"))
	reenabledBeta := skillCatalogCoordinatorSkill("beta", 3, skills.VisibilityAuto)
	reenabled := skillCatalogCoordinatorSnapshot(t, 12, updatedAlpha, reenabledBeta, gamma)
	next := mustPlanSkillCatalog(t, SkillCatalogCoordinatorInput{
		CurrentSnapshot: reenabled,
		PriorCursor:     plan.Cursor,
		ContextEpoch:    "epoch-delta",
		VisibleHistory:  history,
		CharBudget:      10_000,
	})
	if next.Kind != SkillCatalogPlanDelta || !next.HasMessage() {
		t.Fatalf("re-enable plan = %#v", next)
	}
	delta = skillCatalogCoordinatorDeltaWire{}
	if err := json.Unmarshal([]byte(next.Message.GetText()), &delta); err != nil {
		t.Fatal(err)
	}
	if len(delta.Upserts) != 1 || delta.Upserts[0].Skill.ID != beta.ID || delta.Upserts[0].Reason != skills.CatalogUpsertReenabled || len(delta.Revokes) != 0 {
		t.Fatalf("re-enable delta = %#v", delta)
	}
}

func TestCatalogEpochAndVisibleHistoryRequireSnapshotRebuild(t *testing.T) {
	t.Parallel()

	current := skillCatalogCoordinatorSnapshot(t, 4, skillCatalogCoordinatorSkill("alpha", 1, skills.VisibilityAuto))
	initial := mustPlanSkillCatalog(t, SkillCatalogCoordinatorInput{
		CurrentSnapshot: current,
		ContextEpoch:    "epoch-old",
		CharBudget:      10_000,
	})
	validHistory := []types.Message{*initial.Message}

	tests := []struct {
		name    string
		epoch   SkillCatalogContextEpoch
		history []types.Message
		reason  SkillCatalogRebuildReason
	}{
		{name: "epoch changed", epoch: "epoch-new", history: validHistory, reason: SkillCatalogRebuildEpochChanged},
		{name: "history missing", epoch: "epoch-old", history: nil, reason: SkillCatalogRebuildHistoryMissing},
		{
			name:  "history revision mismatch",
			epoch: "epoch-old",
			history: []types.Message{types.DeveloperMessage("stale", types.DeveloperMessageMetadata{
				Kind: types.DeveloperMessageKindSkillCatalogDelta, Revision: 99,
			})},
			reason: SkillCatalogRebuildHistoryMissing,
		},
		{
			name:  "history catalog metadata is not internal",
			epoch: "epoch-old",
			history: []types.Message{{
				Role:              types.RoleDeveloper,
				Content:           initial.Message.Content,
				DeveloperMetadata: initial.Message.DeveloperMetadata,
			}},
			reason: SkillCatalogRebuildHistoryMissing,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			historyBefore := append([]types.Message(nil), test.history...)
			plan := mustPlanSkillCatalog(t, SkillCatalogCoordinatorInput{
				CurrentSnapshot: current,
				PriorCursor:     initial.Cursor,
				ContextEpoch:    test.epoch,
				VisibleHistory:  test.history,
				CharBudget:      10_000,
			})
			if plan.Kind != SkillCatalogPlanSnapshot || plan.RebuildReason != test.reason || !plan.HasMessage() {
				t.Fatalf("rebuild plan = %#v", plan)
			}
			assertSkillCatalogDeveloperMessage(t, *plan.Message, types.DeveloperMessageKindSkillCatalogSnapshot, 4)
			if !reflect.DeepEqual(test.history, historyBefore) {
				t.Fatal("snapshot rebuild rewrote history")
			}
		})
	}
}

func TestSkillCatalogCoordinatorPreservesRenderOverflowDiagnostics(t *testing.T) {
	t.Parallel()

	longName := strings.Repeat("catalog-name-", 20)
	skill := skillCatalogCoordinatorSkill(longName, 1, skills.VisibilityAuto)
	skill.Summary = strings.Repeat("description ", 100)
	current := skillCatalogCoordinatorSnapshot(t, 20, skill)

	plan := mustPlanSkillCatalog(t, SkillCatalogCoordinatorInput{
		CurrentSnapshot: current,
		ContextEpoch:    "epoch-overflow",
		CharBudget:      1,
	})
	if !plan.Render.MandatoryOverflow || plan.Render.CharCount <= plan.Render.Budget {
		t.Fatalf("overflow diagnostics were lost: %#v", plan.Render)
	}
	if !plan.HasMessage() || plan.Message.GetText() != plan.Render.Text || !strings.Contains(plan.Render.Text, longName) {
		t.Fatalf("overflow plan discarded mandatory catalog state: %#v", plan)
	}
	if plan.Cursor.AnnouncedRevision() != current.Revision {
		t.Fatalf("overflow snapshot did not advance cursor: %#v", plan.Cursor)
	}
}

func TestSkillCatalogCoordinatorRejectsLedgerRegressionAfterSilentChange(t *testing.T) {
	t.Parallel()

	alpha := skillCatalogCoordinatorSkill("alpha", 1, skills.VisibilityAuto)
	initial := mustPlanSkillCatalog(t, SkillCatalogCoordinatorInput{
		CurrentSnapshot: skillCatalogCoordinatorSnapshot(t, 1, alpha),
		ContextEpoch:    "epoch-regression",
		CharBudget:      10_000,
	})
	history := []types.Message{*initial.Message}
	silent := mustPlanSkillCatalog(t, SkillCatalogCoordinatorInput{
		CurrentSnapshot: skillCatalogCoordinatorSnapshot(t, 3, alpha),
		PriorCursor:     initial.Cursor,
		ContextEpoch:    "epoch-regression",
		VisibleHistory:  history,
		CharBudget:      10_000,
	})
	if silent.HasMessage() {
		t.Fatalf("silent ledger advance emitted a message: %#v", silent)
	}

	_, err := PlanSkillCatalog(SkillCatalogCoordinatorInput{
		CurrentSnapshot: skillCatalogCoordinatorSnapshot(t, 2, alpha),
		PriorCursor:     silent.Cursor,
		ContextEpoch:    "epoch-regression",
		VisibleHistory:  history,
		CharBudget:      10_000,
	})
	if err == nil || !strings.Contains(err.Error(), "advance skill catalog ledger") {
		t.Fatalf("ledger regression error = %v", err)
	}
}

func TestSkillCatalogCoordinatorRejectsUnannouncedModelFacingLedgerState(t *testing.T) {
	t.Parallel()

	alpha := skillCatalogCoordinatorSkill("alpha", 1, skills.VisibilityAuto)
	initial := mustPlanSkillCatalog(t, SkillCatalogCoordinatorInput{
		CurrentSnapshot: skillCatalogCoordinatorSnapshot(t, 1, alpha),
		ContextEpoch:    "epoch-invalid-ledger",
	})
	changed := alpha
	changed.Summary = "unannounced"
	changed.Revision = 2
	invalid := initial.Cursor.Clone()
	invalid.LedgerSnapshot = skillCatalogCoordinatorSnapshot(t, 2, changed)
	if err := invalid.Validate(); err == nil || !strings.Contains(err.Error(), "unannounced model-facing changes") {
		t.Fatalf("invalid cursor error = %v", err)
	}
}

func TestCatalogDeltaPreservesRenderOverflowDiagnostics(t *testing.T) {
	t.Parallel()

	alpha := skillCatalogCoordinatorSkill(strings.Repeat("delta-name-", 20), 1, skills.VisibilityAuto)
	initial := mustPlanSkillCatalog(t, SkillCatalogCoordinatorInput{
		CurrentSnapshot: skillCatalogCoordinatorSnapshot(t, 1, alpha),
		ContextEpoch:    "epoch-delta-overflow",
		CharBudget:      10_000,
	})
	updated := alpha
	updated.Summary = strings.Repeat("updated summary ", 100)
	updated.Revision = 2
	plan := mustPlanSkillCatalog(t, SkillCatalogCoordinatorInput{
		CurrentSnapshot: skillCatalogCoordinatorSnapshot(t, 2, updated),
		PriorCursor:     initial.Cursor,
		ContextEpoch:    "epoch-delta-overflow",
		VisibleHistory:  []types.Message{*initial.Message},
		CharBudget:      1,
	})
	if plan.Kind != SkillCatalogPlanDelta || !plan.HasMessage() || !plan.Render.MandatoryOverflow {
		t.Fatalf("delta overflow diagnostics = %#v", plan)
	}
	if plan.Message.GetText() != plan.Render.Text || plan.Render.CharCount <= plan.Render.Budget {
		t.Fatalf("delta overflow payload was not preserved: %#v", plan.Render)
	}
}

func TestSkillCatalogCoordinatorCursorJSONRoundTrip(t *testing.T) {
	t.Parallel()

	current := skillCatalogCoordinatorSnapshot(t, 3, skillCatalogCoordinatorSkill("alpha", 1, skills.VisibilityAuto))
	plan := mustPlanSkillCatalog(t, SkillCatalogCoordinatorInput{
		CurrentSnapshot: current,
		ContextEpoch:    "epoch-json",
	})
	data, err := json.Marshal(plan.Cursor)
	if err != nil {
		t.Fatal(err)
	}
	var decoded SkillCatalogCursor
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, plan.Cursor) {
		t.Fatalf("cursor round trip = %#v, want %#v", decoded, plan.Cursor)
	}
}

func TestSkillCatalogCoordinatorRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	current := skillCatalogCoordinatorSnapshot(t, 1, skillCatalogCoordinatorSkill("alpha", 1, skills.VisibilityAuto))
	if _, err := PlanSkillCatalog(SkillCatalogCoordinatorInput{CurrentSnapshot: current}); err == nil {
		t.Fatal("missing context epoch unexpectedly accepted")
	}
	if _, err := PlanSkillCatalog(SkillCatalogCoordinatorInput{
		CurrentSnapshot: skills.CatalogSnapshot{}, ContextEpoch: "epoch-invalid",
	}); err == nil {
		t.Fatal("invalid current snapshot unexpectedly accepted")
	}
}

type skillCatalogCoordinatorDeltaWire struct {
	Type         string                   `json:"type"`
	FromRevision skills.CatalogRevision   `json:"from_revision"`
	ToRevision   skills.CatalogRevision   `json:"to_revision"`
	Upserts      []skillCatalogUpsertWire `json:"upserts"`
	Revokes      []struct {
		ID     skills.SkillID             `json:"id"`
		Reason skills.CatalogRevokeReason `json:"reason"`
	} `json:"revokes"`
}

type skillCatalogUpsertWire struct {
	Reason skills.CatalogUpsertReason `json:"reason"`
	Skill  struct {
		ID skills.SkillID `json:"id"`
	} `json:"skill"`
}

func skillCatalogCoordinatorUpsertIDs(delta skillCatalogCoordinatorDeltaWire) []skills.SkillID {
	ids := make([]skills.SkillID, len(delta.Upserts))
	for index, upsert := range delta.Upserts {
		ids[index] = upsert.Skill.ID
	}
	return ids
}

func skillCatalogCoordinatorSkill(name string, revision skills.SkillRevision, visibility skills.Visibility) skills.EffectiveSkill {
	skill := skills.EffectiveSkill{
		ID:                 skills.SkillID("skill:project:" + name),
		Name:               name,
		Summary:            "summary for " + name,
		Source:             skills.SourceProject,
		Locator:            skills.SkillLocator("/skills/" + name),
		Digest:             skillCatalogCoordinatorDigest,
		Revision:           revision,
		Visibility:         visibility,
		VisibilitySource:   skills.SkillScopeProject,
		ModelVisible:       true,
		DescriptionVisible: true,
		UserInvocable:      true,
		Executable:         true,
		Mutable:            true,
	}
	switch visibility {
	case skills.VisibilityNameOnly:
		skill.DescriptionVisible = false
	case skills.VisibilityManualOnly:
		skill.ModelVisible = false
		skill.DescriptionVisible = false
	case skills.VisibilityOff:
		skill.ModelVisible = false
		skill.DescriptionVisible = false
		skill.UserInvocable = false
		skill.Executable = false
	}
	return skill
}

func skillCatalogCoordinatorSnapshot(t *testing.T, revision skills.CatalogRevision, entries ...skills.EffectiveSkill) skills.CatalogSnapshot {
	t.Helper()
	snapshot, err := skills.NewCatalogSnapshot(revision, entries)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func mustPlanSkillCatalog(t *testing.T, input SkillCatalogCoordinatorInput) SkillCatalogPlan {
	t.Helper()
	plan, err := PlanSkillCatalog(input)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Message != nil {
		trusted := plan.Message.WithInternalControlProvenance(messagecontrol.Runtime())
		plan.Message = &trusted
	}
	return plan
}

func assertSkillCatalogDeveloperMessage(t *testing.T, message types.Message, kind types.DeveloperMessageKind, revision uint64) {
	t.Helper()
	if message.Role != types.RoleDeveloper || !message.IsMeta || message.DeveloperMetadata == nil {
		t.Fatalf("catalog message is not internal developer metadata: %#v", message)
	}
	if message.DeveloperMetadata.Kind != kind || message.DeveloperMetadata.Revision != revision {
		t.Fatalf("developer metadata = %#v, want kind=%q revision=%d", message.DeveloperMetadata, kind, revision)
	}
	if message.GetText() == "" {
		t.Fatal("catalog developer message has empty content")
	}
}
