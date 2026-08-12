package compact

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/compactproof"
	"github.com/agent-dance/luban/types"
)

type progressiveProofFixture struct {
	proof compactproof.Proof
}

type progressiveTokenCounter struct{}

func (progressiveTokenCounter) Count(text string) int { return len(text) / 4 }

func progressiveAdmission(pressure bool) ProgressiveProjectionAdmission {
	return ProgressiveProjectionAdmission{
		Enabled: true, Pressure: pressure, Counter: progressiveTokenCounter{},
		RawRequestTokens: 100_000, RawRequestEstimateKnown: true, AutoCompactThreshold: 90_000,
		PreviousCacheReadTokens: 20_000, PreviousUsageKnown: true,
		Pricing:         ProgressiveTokenPricing{InputPerMtok: 5, CacheReadPerMtok: .5, Known: true},
		MinTokenSavings: 2_000, ReuseHorizon: 3, CacheRecoveryRequests: 2,
		RemainingTools: 24, RemainingProjectedTokens: 48_000,
	}
}

func (fixture progressiveProofFixture) CompactionProof() compactproof.Proof { return fixture.proof }

func progressiveToolUse(id, name string) types.Message {
	return types.Message{Role: types.RoleAssistant, Content: []types.ContentBlock{
		types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: id, Name: name, Input: map[string]any{}},
	}}
}

func progressiveToolResult(id, content string) types.Message {
	return progressiveToolResultWithOutcome(id, content, types.ToolOutcomeSucceeded, false)
}

func progressiveToolResultWithOutcome(id, content string, outcome types.ToolOutcome, isError bool) types.Message {
	return types.ToolResultMessage(types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: id, Content: content,
		Outcome: outcome, IsError: isError,
		Data: progressiveProofFixture{proof: compactproof.Proof{
			Inspect: &compactproof.InspectProof{Items: 1},
		}},
	})
}

func progressiveInvestigationWithConsumedPatch(inspects int) []types.Message {
	messages := []types.Message{types.UserMessage("start")}
	for index := 0; index < inspects; index++ {
		id := fmt.Sprintf("inspect-%d", index)
		messages = append(messages,
			progressiveToolUse(id, "Inspect"),
			progressiveToolResult(id, progressiveInspectFixtureContent(index, 2_000)),
		)
	}
	messages = append(messages,
		progressiveToolUse("patch", "ApplyPatch"),
		progressiveToolResult("patch", "applied"),
		progressiveToolUse("verify", "Run"),
	)
	return messages
}

func progressiveInspectFixtureContent(index, repeats int) string {
	encoded, _ := json.Marshal(map[string]any{
		"requests": []any{map[string]any{"id": "read", "kind": "read", "path": fmt.Sprintf("src/file-%d.cc", index)}},
		"evidence": []any{map[string]any{
			"path": fmt.Sprintf("src/file-%d.cc", index),
			"chunks": []any{map[string]any{
				"lines": []int{10, 20}, "content": strings.Repeat(fmt.Sprintf("evidence-%d ", index), repeats),
			}},
		}},
	})
	return string(encoded)
}

func TestProgressiveProjectionAtConsumedMutationKeepsRecentSourceReads(t *testing.T) {
	messages := progressiveInvestigationWithConsumedPatch(7)
	got := ApplyProgressiveToolResultProjection(messages, NewContentReplacementState(), progressiveAdmission(false))
	if !got.Changed || got.Trigger != "consumed_mutation" || got.ProjectedTools != 5 || got.TokensSaved < 2_000 || len(got.Records) != 5 {
		t.Fatalf("projection = %#v", got)
	}
	for index := 0; index < 7; index++ {
		value := extractToolResultContent(got.Messages, fmt.Sprintf("inspect-%d", index))
		if index < 5 {
			if value == progressiveInspectFixtureContent(index, 2_000) || !strings.Contains(value, compactproof.SchemaVersion) || !strings.Contains(value, progressiveInspectRewriteSchema) {
				t.Fatalf("old inspect %d projection = %q", index, value)
			}
		} else if !strings.Contains(value, fmt.Sprintf("evidence-%d", index)) {
			t.Fatalf("protected inspect %d changed: %q", index, value)
		}
	}
}

func TestPendingProgressiveProjectionMeasuresEligibleUncommittedResults(t *testing.T) {
	messages := progressiveInvestigationWithConsumedPatch(7)
	state := NewContentReplacementState()
	admission := progressiveAdmission(false)
	pending := PendingProgressiveToolResultProjection(messages, state, admission)
	if pending.Tools != 5 || pending.TokensSaved < 2_000 {
		t.Fatalf("pending projection = %+v", pending)
	}
	if len(state.Replacements) != 0 || len(state.SeenIDs) != 0 {
		t.Fatalf("pending measurement mutated replacement state: %#v", state)
	}
	applied := ApplyProgressiveToolResultProjection(messages, state, admission)
	if !applied.Changed {
		t.Fatal("fixture projection was not applied")
	}
	if after := PendingProgressiveToolResultProjection(applied.Messages, state, admission); after != (ProgressiveProjectionPending{}) {
		t.Fatalf("applied results remained pending: %+v", after)
	}
}

func TestPendingProgressiveProjectionAnticipatesTriggerButOmitsShadowMode(t *testing.T) {
	messages := progressiveInvestigationWithConsumedPatch(7)
	beforeMutation := messages[:len(messages)-3]
	admission := progressiveAdmission(true)
	if pending := PendingProgressiveToolResultProjection(beforeMutation, NewContentReplacementState(), admission); pending.Tools <= 0 || pending.TokensSaved <= 0 {
		t.Fatalf("future trigger did not advertise waiting reads: %+v", pending)
	}
	admission.Shadow = true
	if pending := PendingProgressiveToolResultProjection(messages, NewContentReplacementState(), admission); pending != (ProgressiveProjectionPending{}) {
		t.Fatalf("shadow projection was advertised as pending: %+v", pending)
	}
}

func TestProgressiveProjectionAllowsCostPositivePressureBatchesBeforeMutation(t *testing.T) {
	messages := progressiveInvestigationWithConsumedPatch(7)
	messages = messages[:len(messages)-3]
	state := NewContentReplacementState()
	if got := ApplyProgressiveToolResultProjection(messages, state, progressiveAdmission(false)); got.Changed {
		t.Fatalf("pressure projection ignored token gate: %#v", got)
	}
	first := ApplyProgressiveToolResultProjection(messages, state, progressiveAdmission(true))
	if !first.Changed || first.Trigger != "working_set_pressure" || first.ProjectedTools <= 0 {
		t.Fatalf("pressure projection changed=%t trigger=%q tools=%d decision=%q", first.Changed, first.Trigger, first.ProjectedTools, first.Decision)
	}
	if content := extractToolResultContent(first.Messages, "inspect-6"); !strings.Contains(content, "evidence-6") {
		t.Fatalf("recent pressure working set changed: %q", content)
	}

	// More inspection may make a two-result top-up profitable after two older
	// results age out of the protected working set.
	more := append(messages,
		progressiveToolUse("inspect-later", "Inspect"),
		progressiveToolResult("inspect-later", progressiveInspectFixtureContent(8, 2_000)),
		progressiveToolUse("next", "Inspect"),
	)
	second := ApplyProgressiveToolResultProjection(more, state, progressiveAdmission(true))
	if !second.Changed || second.Trigger != "working_set_pressure_top_up" || second.ProjectedTools <= 0 {
		t.Fatalf("pressure top-up changed=%t trigger=%q tools=%d decision=%q", second.Changed, second.Trigger, second.ProjectedTools, second.Decision)
	}
	third := ApplyProgressiveToolResultProjection(more, state, progressiveAdmission(true))
	if third.Changed {
		t.Fatalf("pressure projection changed the protected working set: %#v", third)
	}
}

func TestProgressiveProjectionCanRequireConsumedMutation(t *testing.T) {
	messages := progressiveInvestigationWithConsumedPatch(7)
	beforeMutation := messages[:len(messages)-3]
	admission := progressiveAdmission(true)
	admission.RequireConsumedMutation = true
	if got := ApplyProgressiveToolResultProjection(beforeMutation, NewContentReplacementState(), admission); got.Changed || got.Trigger != "" {
		t.Fatalf("pre-mutation source reads were projected: %#v", got)
	}
	if got := ApplyProgressiveToolResultProjection(messages, NewContentReplacementState(), admission); !got.Changed || got.Trigger != "consumed_mutation" {
		t.Fatalf("consumed mutation did not release old reads: %#v", got)
	}
}

func TestProgressiveProjectionFailsClosedForMutationAndDiagnosticStates(t *testing.T) {
	t.Run("failed mutation", func(t *testing.T) {
		messages := progressiveInvestigationWithConsumedPatch(7)
		messages[len(messages)-2] = progressiveToolResultWithOutcome("patch", "failed", types.ToolOutcomeFailed, true)
		if got := ApplyProgressiveToolResultProjection(messages, NewContentReplacementState(), progressiveAdmission(true)); got.Changed {
			t.Fatalf("projection crossed failed mutation: %#v", got)
		}
	})

	t.Run("failed inspect", func(t *testing.T) {
		messages := progressiveInvestigationWithConsumedPatch(7)
		failed := progressiveInspectFixtureContent(99, 2_000)
		messages[2] = progressiveToolResultWithOutcome("inspect-0", failed, types.ToolOutcomeFailed, true)
		got := ApplyProgressiveToolResultProjection(messages, NewContentReplacementState(), progressiveAdmission(true))
		if !got.Changed || extractToolResultContent(got.Messages, "inspect-0") != failed {
			t.Fatalf("diagnostic inspect was not preserved: %#v", got)
		}
	})

	t.Run("run output", func(t *testing.T) {
		messages := progressiveInvestigationWithConsumedPatch(7)
		messages = append(messages, progressiveToolResult("verify", strings.Repeat("run output ", 2_000)), progressiveToolUse("after-run", "Inspect"))
		got := ApplyProgressiveToolResultProjection(messages, NewContentReplacementState(), progressiveAdmission(true))
		if value := extractToolResultContent(got.Messages, "verify"); !strings.Contains(value, "run output") {
			t.Fatalf("run diagnostic changed: %q", value)
		}
	})
}

func TestProgressiveProjectionRequiresProfitableBatchAndFreezesReplacement(t *testing.T) {
	messages := progressiveInvestigationWithConsumedPatch(4)
	messages[2] = progressiveToolResult("inspect-0", progressiveInspectFixtureContent(0, 100))
	if got := ApplyProgressiveToolResultProjection(messages, NewContentReplacementState(), progressiveAdmission(false)); got.Changed {
		t.Fatalf("single old result should not repay a cache break: %#v", got)
	}

	messages = progressiveInvestigationWithConsumedPatch(7)
	state := NewContentReplacementState()
	first := ApplyProgressiveToolResultProjection(messages, state, progressiveAdmission(false))
	if !first.Changed {
		t.Fatal("first projection did not change")
	}
	second := ApplyProgressiveToolResultProjection(first.Messages, state, progressiveAdmission(false))
	if second.Changed || len(second.Records) != 0 {
		t.Fatalf("frozen result changed again: %#v", second)
	}
	replayed, _, errs := ApplyToolResultBudget(messages, state, nil, nil)
	if len(errs) != 0 || extractToolResultContent(replayed, "inspect-0") != first.Records[0].Replacement {
		t.Fatal("frozen replacement was not byte-stably replayed")
	}
}

func TestProgressiveProjectionTokenCostGateAccountsForCacheBreak(t *testing.T) {
	messages := progressiveInvestigationWithConsumedPatch(7)

	t.Run("rejects ordinary cache-negative rewrite", func(t *testing.T) {
		admission := progressiveAdmission(false)
		admission.RawRequestTokens = 200_000
		admission.AutoCompactThreshold = 300_000
		admission.PreviousCacheReadTokens = 190_000
		admission.ReuseHorizon = 1
		got := ApplyProgressiveToolResultProjection(messages, NewContentReplacementState(), admission)
		if got.Changed || got.Decision != ProgressiveDecisionKeepCost || got.TokensSaved <= 0 || got.CacheBreakCostUSD <= 0 {
			t.Fatalf("cache-negative gate = %#v", got)
		}
	})

	t.Run("admits conservative compact avoidance", func(t *testing.T) {
		admission := progressiveAdmission(false)
		got := ApplyProgressiveToolResultProjection(messages, NewContentReplacementState(), admission)
		if !got.Changed || got.Decision != ProgressiveDecisionAdmittedAvoidCompact || !got.AvoidsImmediateCompaction || got.EstimatedNetSavingsUSD <= 0 {
			t.Fatalf("compact-avoidance gate = %#v", got)
		}
	})

	t.Run("rejects timing-only compact deferral", func(t *testing.T) {
		admission := progressiveAdmission(false)
		admission.PreviousCacheReadTokens = 80_000
		got := ApplyProgressiveToolResultProjection(messages, NewContentReplacementState(), admission)
		if got.Changed || got.Decision != ProgressiveDecisionKeepCompactThreshold || !got.AvoidsImmediateCompaction ||
			got.AvoidedCompactInputCostUSD != 0 || got.EstimatedNetSavingsUSD >= 0 {
			t.Fatalf("timing-only compact deferral = %#v", got)
		}
	})

	t.Run("fails closed without complete estimate or pricing", func(t *testing.T) {
		admission := progressiveAdmission(false)
		admission.RawRequestEstimateKnown = false
		if got := ApplyProgressiveToolResultProjection(messages, NewContentReplacementState(), admission); got.Changed || got.Decision != ProgressiveDecisionKeepIncomplete {
			t.Fatalf("incomplete estimate = %#v", got)
		}
		admission = progressiveAdmission(false)
		admission.Pricing.Known = false
		if got := ApplyProgressiveToolResultProjection(messages, NewContentReplacementState(), admission); got.Changed || got.Decision != ProgressiveDecisionKeepUnknownPricing {
			t.Fatalf("unknown pricing = %#v", got)
		}
		admission = progressiveAdmission(false)
		admission.PreviousUsageKnown = false
		if got := ApplyProgressiveToolResultProjection(messages, NewContentReplacementState(), admission); got.Changed || got.Decision != ProgressiveDecisionKeepIncomplete {
			t.Fatalf("missing provider baseline = %#v", got)
		}
	})
}

func TestProgressiveProjectionDeepSeekCounterfactualDoesNotChangeDefaultGate(t *testing.T) {
	base := ProgressiveProjectionAdmission{
		Enabled: true, Counter: progressiveTokenCounter{},
		RawRequestTokens: 27_073, RawRequestEstimateKnown: true,
		AutoCompactThreshold:    20_616,
		PreviousCacheReadTokens: 19_584, PreviousUsageKnown: true,
		Pricing:               ProgressiveTokenPricing{InputPerMtok: 0.14, CacheReadPerMtok: 0.0028, Known: true},
		MinTokenSavings:       2_000,
		ReuseHorizon:          3,
		CacheRecoveryRequests: 2,
	}

	withoutCounterfactual := ProgressiveProjectionResult{TokensSaved: 9_901}
	if decision := evaluateProgressiveProjectionAdmission(&withoutCounterfactual, base); decision != ProgressiveDecisionKeepCompactThreshold {
		t.Fatalf("default decision = %q, want %q", decision, ProgressiveDecisionKeepCompactThreshold)
	}
	if withoutCounterfactual.GrossCacheBreakCostUSD <= 0 || withoutCounterfactual.CacheBreakCostUSD != withoutCounterfactual.GrossCacheBreakCostUSD || withoutCounterfactual.EstimatedNetSavingsUSD >= 0 {
		t.Fatalf("default gate changed: %#v", withoutCounterfactual)
	}

	deepSeek := base
	deepSeek.ImminentCompactResetsCache = true
	withCounterfactual := ProgressiveProjectionResult{TokensSaved: 9_901}
	if decision := evaluateProgressiveProjectionAdmission(&withCounterfactual, deepSeek); decision != ProgressiveDecisionAdmittedAvoidCompact {
		t.Fatalf("counterfactual decision = %q, want %q", decision, ProgressiveDecisionAdmittedAvoidCompact)
	}
	if !withCounterfactual.AvoidsImmediateCompaction || withCounterfactual.GrossCacheBreakCostUSD <= 0 ||
		withCounterfactual.CacheBreakCostUSD != 0 || withCounterfactual.EstimatedNetSavingsUSD <= 0 {
		t.Fatalf("counterfactual gate = %#v", withCounterfactual)
	}
}

func TestProgressivePressureBatchSelectsMinimumCostPositivePrefix(t *testing.T) {
	candidates := make([]progressiveInspectCandidate, 6)
	for index := range candidates {
		candidates[index] = progressiveInspectCandidate{originalTokens: 4_000, projectedTokens: 1_000}
	}
	admission := progressiveAdmission(true)
	admission.RawRequestTokens = 100_000
	admission.AutoCompactThreshold = 88_000
	admission.PreviousCacheReadTokens = 10_000
	got := selectProgressivePressureBatch(candidates, admission)
	if len(got) != 4 {
		t.Fatalf("selected %d candidates, want smallest four-result cost-positive threshold crossing", len(got))
	}
	admission.AutoCompactThreshold = 70_000
	got = selectProgressivePressureBatch(candidates, admission)
	if len(got) != len(candidates) {
		t.Fatalf("selected %d candidates, want all %d for rejection telemetry", len(got), len(candidates))
	}
}

func TestProgressivePressureBatchFallsBackToOldestIndexes(t *testing.T) {
	candidates := make([]progressiveInspectCandidate, 4)
	for index := range candidates {
		candidates[index] = progressiveInspectCandidate{
			projected: "rewrite", indexed: "index",
			originalTokens: 4_000, projectedTokens: 2_000, indexedTokens: 500,
		}
	}
	admission := progressiveAdmission(true)
	admission.RawRequestTokens = 100_000
	admission.AutoCompactThreshold = 88_000
	admission.PreviousCacheReadTokens = 10_000
	got := selectProgressivePressureBatch(candidates, admission)
	if len(got) != 4 {
		t.Fatalf("selected %d candidates, want full rich prefix before indexing", len(got))
	}
	indexed := 0
	for _, candidate := range got {
		if candidate.projected == candidate.indexed {
			indexed++
		}
	}
	if indexed != 3 || got[3].projected != "rewrite" {
		t.Fatalf("indexed %d candidates, want oldest three while newest stays rich: %#v", indexed, got)
	}
}

func TestProgressiveProjectionRejectsCacheBreakThatCannotAvoidImmediateCompact(t *testing.T) {
	messages := progressiveInvestigationWithConsumedPatch(7)
	admission := progressiveAdmission(false)
	admission.RawRequestTokens = 100_000
	admission.AutoCompactThreshold = 70_000
	got := ApplyProgressiveToolResultProjection(messages, NewContentReplacementState(), admission)
	if got.Changed || got.Decision != ProgressiveDecisionKeepCompactThreshold {
		t.Fatalf("immediate compact projection changed=%t decision=%q request_after=%d", got.Changed, got.Decision, got.ProjectedRequestTokens)
	}
}

func TestProgressiveProjectionShadowAndResumeBudget(t *testing.T) {
	messages := progressiveInvestigationWithConsumedPatch(7)
	state := NewContentReplacementState()
	admission := progressiveAdmission(false)
	admission.Shadow = true
	shadow := ApplyProgressiveToolResultProjection(messages, state, admission)
	if shadow.Changed || !shadow.Shadow || shadow.Decision != ProgressiveDecisionShadow || len(shadow.Records) != 0 || len(state.Replacements) != 0 {
		t.Fatalf("shadow mutated state: %#v state=%#v", shadow, state)
	}

	admission.Shadow = false
	committed := ApplyProgressiveToolResultProjection(messages, state, admission)
	if !committed.Changed {
		t.Fatalf("commit = %#v", committed)
	}
	tools, projectedTokens := ProgressiveProjectionBudgetUsage(state, progressiveTokenCounter{})
	if tools != committed.ProjectedTools || projectedTokens != committed.ProjectedTokens {
		t.Fatalf("budget usage = tools:%d tokens:%d, want tools:%d tokens:%d", tools, projectedTokens, committed.ProjectedTools, committed.ProjectedTokens)
	}

	resumed := NewContentReplacementState()
	for _, record := range committed.Records {
		resumed.Replacements[record.ToolUseID] = record.Replacement
	}
	resumedTools, resumedTokens := ProgressiveProjectionBudgetUsage(resumed, progressiveTokenCounter{})
	if resumedTools != tools || resumedTokens != projectedTokens {
		t.Fatalf("resumed budget = %d/%d, want %d/%d", resumedTools, resumedTokens, tools, projectedTokens)
	}
}

func TestProgressiveProjectionSupportsReviewedRunAndPatchStrategies(t *testing.T) {
	t.Run("Run", func(t *testing.T) {
		messages := []types.Message{types.UserMessage("start")}
		for index := 0; index < 4; index++ {
			id := fmt.Sprintf("run-%d", index)
			messages = append(messages, progressiveToolUse(id, "Run"), types.ToolResultMessage(types.ToolResultBlock{
				ToolUseID: id, Content: "head\n" + strings.Repeat("successful verification output\n", 500) + "tail", Outcome: types.ToolOutcomeSucceeded,
				Data: progressiveProofFixture{proof: compactproof.Proof{Run: &compactproof.RunProof{
					LogicalExecutionCommitted: true, VerificationStatus: "passed",
					Steps: []compactproof.RunStepProof{{Ordinal: 1, Status: "succeeded", ExitCode: 0, Invoked: true}},
				}}},
			}))
		}
		messages = append(messages, progressiveToolUse("patch", "ApplyPatch"), progressiveToolResult("patch", "applied"), progressiveToolUse("next", "Inspect"))
		admission := progressiveAdmission(false)
		admission.AutoCompactThreshold = 95_000
		admission.PreviousCacheReadTokens = 0
		admission.AllowedTools = map[string]struct{}{"run": {}}
		got := ApplyProgressiveToolResultProjection(messages, NewContentReplacementState(), admission)
		if !got.Changed || got.ProjectedTools != 2 || !strings.Contains(extractToolResultContent(got.Messages, "run-0"), progressiveRunRewriteSchema) ||
			!strings.Contains(extractToolResultContent(got.Messages, "run-3"), "successful verification output") {
			t.Fatalf("Run strategy = %#v", got)
		}
	})

	t.Run("ApplyPatch", func(t *testing.T) {
		messages := []types.Message{types.UserMessage("start")}
		for index := 0; index < 4; index++ {
			id := fmt.Sprintf("patch-%d", index)
			messages = append(messages, progressiveToolUse(id, "ApplyPatch"), types.ToolResultMessage(types.ToolResultBlock{
				ToolUseID: id, Content: strings.Repeat("verbose committed patch receipt\n", 400), Outcome: types.ToolOutcomeSucceeded,
				Data: progressiveProofFixture{proof: compactproof.Proof{Patch: &compactproof.PatchProof{Status: "succeeded", CAS: "committed", Files: 1, Hunks: 2}}},
			}))
		}
		messages = append(messages, progressiveToolUse("next", "Inspect"))
		admission := progressiveAdmission(false)
		admission.AutoCompactThreshold = 95_000
		admission.PreviousCacheReadTokens = 0
		admission.AllowedTools = map[string]struct{}{"applypatch": {}}
		got := ApplyProgressiveToolResultProjection(messages, NewContentReplacementState(), admission)
		if !got.Changed || got.ProjectedTools != 2 || !strings.Contains(extractToolResultContent(got.Messages, "patch-0"), compactproof.SchemaVersion) ||
			!strings.Contains(extractToolResultContent(got.Messages, "patch-3"), "verbose committed patch receipt") {
			t.Fatalf("ApplyPatch strategy = %#v", got)
		}
	})
}

func TestProgressiveToolResultSupportsContextUpdateActions(t *testing.T) {
	inspect := progressiveToolResult("inspect", progressiveInspectFixtureContent(0, 2_000)).Content[0].(types.ToolResultBlock)
	if !ProgressiveToolResultSupportsAction("Inspect", inspect, "KEEP") ||
		!ProgressiveToolResultSupportsAction("Inspect", inspect, "REWRITE") ||
		!ProgressiveToolResultSupportsAction("Inspect", inspect, "INDEX") ||
		ProgressiveToolResultSupportsAction("Inspect", inspect, "DROP") {
		t.Fatal("Inspect ContextUpdate action support did not match deterministic runtime policy")
	}
	failed := inspect
	failed.IsError = true
	failed.Outcome = types.ToolOutcomeFailed
	if !ProgressiveToolResultSupportsAction("Inspect", failed, "KEEP") || ProgressiveToolResultSupportsAction("Inspect", failed, "REWRITE") {
		t.Fatal("failed result was not restricted to KEEP")
	}
}

func TestProgressiveProjectionSoakKeepsMediaPersistedAndParallelInvariants(t *testing.T) {
	uses := make([]types.ContentBlock, 0, 7)
	results := make([]types.ToolResultBlock, 0, 7)
	for index := 0; index < 7; index++ {
		id := fmt.Sprintf("parallel-%d", index)
		uses = append(uses, types.ToolUseBlock{ID: id, Name: "Inspect", Input: map[string]any{}})
		message := progressiveToolResultWithOutcome(id, progressiveInspectFixtureContent(index, 2_000), types.ToolOutcomeSucceeded, false)
		results = append(results, message.Content[0].(types.ToolResultBlock))
	}
	results[0].ContentBlocks = []types.ContentBlock{types.ImageBlock{Type: types.ContentTypeImage}}
	results[1].Content = "<persisted-output>\n/path/to/private-result\n</persisted-output>"
	messages := []types.Message{
		types.UserMessage("start"),
		{Role: types.RoleAssistant, Content: uses},
		types.ToolResultMessage(results...),
		progressiveToolUse("patch", "ApplyPatch"),
		progressiveToolResult("patch", "applied"),
		progressiveToolUse("next", "Run"),
	}
	got := ApplyProgressiveToolResultProjection(messages, NewContentReplacementState(), progressiveAdmission(false))
	if !got.Changed {
		t.Fatalf("parallel projection = %#v", got)
	}
	if value := extractToolResultContent(got.Messages, "parallel-0"); value != results[0].TextContent() {
		t.Fatalf("media result changed: %q", value)
	}
	if value := extractToolResultContent(got.Messages, "parallel-1"); value != results[1].TextContent() {
		t.Fatalf("persisted index changed: %q", value)
	}
	if value := extractToolResultContent(got.Messages, "parallel-2"); !strings.Contains(value, progressiveInspectRewriteSchema) {
		t.Fatalf("eligible parallel result was not rewritten: %q", value)
	}
}
