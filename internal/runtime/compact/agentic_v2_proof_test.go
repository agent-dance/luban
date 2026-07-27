package compact

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/contracts/compactproof"
	"github.com/agent-dance/luban/types"
)

type compactProofFixture struct {
	proof compactproof.Proof
}

func (f compactProofFixture) CompactionProof() compactproof.Proof { return f.proof }

type v2ProofCase struct {
	id     string
	tool   string
	result types.ToolResultBlock
	assert func(testing.TB, agenticV2CompactionEnvelope)
}

func TestAgenticV2ProofMicrocompactSameBuildSwitchDeterminismAndSavings(t *testing.T) {
	cases := agenticV2ProofCases()
	messages := []types.Message{types.UserMessage("start")}
	for _, test := range cases {
		result := test.result
		result.Type = types.ContentTypeToolResult
		result.ToolUseID = test.id
		messages = append(messages, toolUseMsg(test.id, test.tool), types.ToolResultMessage(result))
	}
	// KeepRecent is floored at one. Add a sentinel after every asserted case so
	// all matrix entries become eligible for proof replacement.
	sentinelContent := strings.Repeat("sentinel repository evidence ", 500)
	messages = append(messages,
		toolUseMsg("proof-sentinel", "Inspect"),
		types.ToolResultMessage(types.ToolResultBlock{
			Type: types.ContentTypeToolResult, ToolUseID: "proof-sentinel", Content: sentinelContent,
			Outcome: types.ToolOutcomeSucceeded,
			Data:    compactProofFixture{proof: compactproof.Proof{Inspect: &compactproof.InspectProof{Items: 1}}},
		}),
	)

	off := v2IdleConfig(false)
	offResult := MicrocompactWithResult(messages, off)
	if offResult.Changed || !reflect.DeepEqual(offResult.Messages, messages) {
		t.Fatal("same-build disabled V2 compaction changed the conversation")
	}

	on := v2IdleConfig(true)
	first := MicrocompactWithResult(messages, on)
	second := MicrocompactWithResult(messages, on)
	if !first.Changed || first.ToolsCleared != len(cases) {
		t.Fatalf("proof compaction changed=%t tools=%d, want %d", first.Changed, first.ToolsCleared, len(cases))
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("same-build enabled V2 compaction was not deterministic")
	}
	for _, test := range cases {
		envelope := requireAgenticV2Envelope(t, first.Messages, test.id)
		if envelope.Schema != compactproof.SchemaVersion || envelope.Tool != test.tool || envelope.Outcome != test.result.Outcome || envelope.IsError != test.result.IsError {
			t.Fatalf("%s envelope identity = %#v", test.id, envelope)
		}
		if len(envelope.ContentSHA256) != 64 {
			t.Fatalf("%s content digest = %q", test.id, envelope.ContentSHA256)
		}
		test.assert(t, envelope)
	}

	beforeWire, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}
	afterWire, err := json.Marshal(first.Messages)
	if err != nil {
		t.Fatal(err)
	}
	window := NewContextWindow(1_000_000)
	beforeTokens := window.EstimateMessages(messages)
	afterTokens := window.EstimateMessages(first.Messages)
	if first.BytesSaved <= 0 || len(afterWire) >= len(beforeWire) || afterTokens >= beforeTokens {
		t.Fatalf("no measurable savings: result=%+v wire=%d/%d tokens=%d/%d", first, len(beforeWire), len(afterWire), beforeTokens, afterTokens)
	}
	if first.BytesSaved*100 < first.OriginalBytes*80 {
		t.Fatalf("tool-result byte reduction = %d/%d, want at least 80%%", first.BytesSaved, first.OriginalBytes)
	}
	if (beforeTokens-afterTokens)*100 < beforeTokens*70 {
		t.Fatalf("token reduction = %d/%d, want at least 70%%", beforeTokens-afterTokens, beforeTokens)
	}
	t.Logf("Agentic V2 proof microcompact: tool bytes %d -> %d (-%.1f%%), request bytes %d -> %d, estimated tokens %d -> %d (-%.1f%%)",
		first.OriginalBytes, first.CompactedBytes, percentReduction(first.OriginalBytes, first.CompactedBytes),
		len(beforeWire), len(afterWire), beforeTokens, afterTokens, percentReduction(beforeTokens, afterTokens))
}

func TestAgenticV2CachedMicrocompactPinsProofLedgerAndPreservesContinuation(t *testing.T) {
	messages := []types.Message{types.UserMessage("start")}
	for index, tool := range []string{"Inspect", "Run", "ApplyPatch"} {
		id := "cached-v2-" + tool
		assistant := toolUseMsg(id, tool)
		continuation := &types.ProviderContinuation{
			Protocol: "anthropic", RequestedModel: "same-build", ServedModel: "same-build",
			Items: []types.ProviderContinuationItem{types.NewProviderContinuationItem(index, json.RawMessage(`{"type":"proof"}`))},
		}
		assistant.AttachProviderContinuation(continuation)
		messages = append(messages, assistant, types.ToolResultMessage(types.ToolResultBlock{
			Type: types.ContentTypeToolResult, ToolUseID: id,
			Content: strings.Repeat(tool+" evidence ", 800), Outcome: types.ToolOutcomeSucceeded,
			Data: compactProofFixture{proof: proofForCachedTool(tool)},
		}))
	}

	cfg := DefaultMicrocompactConfig()
	cfg.QuerySource = MicrocompactSourceMain
	cfg.CachedEnabled = true
	cfg.CachedTriggerThreshold = 2
	cfg.CachedKeepRecent = 1
	state := NewCachedMicrocompactState()
	first := CachedMicrocompact(messages, cfg, state)
	if !first.Changed || len(first.DeletedToolIDs) != 2 || first.ProofBytes <= 0 || first.BytesSaved <= 0 {
		t.Fatalf("cached proof result = %+v", first)
	}
	second := CachedMicrocompact(messages, cfg, state)
	if second.Changed || !reflect.DeepEqual(first.Messages, second.Messages) {
		t.Fatal("pinned cached proof projection was not deterministic across requests")
	}
	ledger := requireCachedProofLedger(t, first.Messages)
	if ledger.Schema != "agentic-v2-cache-proof-ledger/v1" || len(ledger.Entries) != 2 {
		t.Fatalf("proof ledger = %#v", ledger)
	}
	for _, id := range first.DeletedToolIDs {
		if got := findMicrocompactTestToolResult(first.Messages, id); !strings.Contains(got, "evidence") {
			t.Fatalf("cached microcompact locally rewrote %s: %q", id, got)
		}
	}
	for _, message := range first.Messages {
		if message.Role != types.RoleAssistant {
			continue
		}
		if _, ok := message.ValidatedProviderContinuation(); !ok {
			t.Fatal("cached proof projection invalidated an untouched provider continuation")
		}
	}

	off := cfg
	off.AgenticV2ProofsEnabled = false
	offResult := CachedMicrocompact(messages, off, NewCachedMicrocompactState())
	if offResult.Changed || !reflect.DeepEqual(offResult.Messages, messages) {
		t.Fatal("same-build cached V2 switch-off changed messages")
	}
	t.Logf("cached Agentic V2 proof ledger: reclaimed=%d proof=%d saved=%d bytes", first.ReclaimedBytes, first.ProofBytes, first.BytesSaved)
}

func agenticV2ProofCases() []v2ProofCase {
	large := func(prefix string) string { return prefix + ": " + strings.Repeat("evidence payload line ", 1_200) }
	inspectProof := func(partial bool) compactproof.Proof {
		return compactproof.Proof{
			Revision: &compactproof.RevisionProof{Status: "observed", Generation: "generation-42"},
			Inspect: &compactproof.InspectProof{
				Requests: 2, Files: 4, Matches: 9, Snippets: 3, Items: 12,
				HasMoreView: partial, SourceTruncated: partial,
			},
		}
	}
	runProof := func(status string, exit int, invoked bool) compactproof.Proof {
		return compactproof.Proof{
			Revision: &compactproof.RevisionProof{Status: "revision_bound", Epoch: 7, Digest: strings.Repeat("a", 64)},
			Run: &compactproof.RunProof{
				LogicalExecutionCommitted: invoked, RevisionSealDisposition: "revision_bound", TotalDurationMS: 37,
				Steps: []compactproof.RunStepProof{{Ordinal: 0, Status: status, ExitCode: exit, DurationMS: 37, Invoked: invoked}},
			},
		}
	}
	patchProof := compactproof.Proof{
		Revision: &compactproof.RevisionProof{Status: "committed", Epoch: 8, Digest: strings.Repeat("b", 64)},
		Patch:    &compactproof.PatchProof{Status: "success", CAS: "committed", Files: 2, Hunks: 3, Additions: 8, Deletions: 2},
	}
	return []v2ProofCase{
		{id: "inspect-success", tool: "Inspect", result: types.ToolResultBlock{Content: large("inspect success"), Outcome: types.ToolOutcomeSucceeded, Data: compactProofFixture{inspectProof(false)}}, assert: func(t testing.TB, got agenticV2CompactionEnvelope) {
			if got.Proof == nil || got.Proof.Inspect == nil || got.Proof.Revision.Generation != "generation-42" || got.Proof.Inspect.Items != 12 {
				t.Fatalf("inspect success proof = %#v", got)
			}
		}},
		{id: "inspect-failed", tool: "Inspect", result: types.ToolResultBlock{Content: large("inspect failed"), IsError: true, Outcome: types.ToolOutcomeFailed}, assert: assertProofError("tool_result_failed")},
		{id: "inspect-denied", tool: "Inspect", result: types.ToolResultBlock{Content: large("inspect denied"), IsError: true, Outcome: types.ToolOutcomeDenied}, assert: assertPermissionDenied},
		{id: "inspect-partial", tool: "Inspect", result: types.ToolResultBlock{Content: large("inspect partial"), Outcome: types.ToolOutcomePartial, Data: compactProofFixture{inspectProof(true)}}, assert: func(t testing.TB, got agenticV2CompactionEnvelope) {
			if got.Proof == nil || got.Proof.Inspect == nil || !got.Proof.Inspect.HasMoreView || !got.Proof.Inspect.SourceTruncated {
				t.Fatalf("inspect partial proof = %#v", got)
			}
		}},
		{id: "run-success", tool: "Run", result: types.ToolResultBlock{Content: large("run success"), Outcome: types.ToolOutcomeSucceeded, Data: compactProofFixture{runProof("succeeded", 0, true)}, Metadata: map[string]string{"verification.status": "revision_bound", "verification.kind": "targeted_test"}}, assert: assertRunProof("succeeded", 0)},
		{id: "run-failed", tool: "Run", result: types.ToolResultBlock{Content: large("run failed"), IsError: true, Outcome: types.ToolOutcomeFailed, Data: compactProofFixture{runProof("failed", 17, true)}}, assert: assertRunProof("failed", 17)},
		{id: "run-denied", tool: "Run", result: types.ToolResultBlock{Content: large("run denied"), IsError: true, Outcome: types.ToolOutcomeDenied, Data: types.PolicyDecision{Disposition: types.PolicyBlock, Code: "run.policy.block", RuleSource: "runtime"}}, assert: assertPermissionDenied},
		{id: "run-partial", tool: "Run", result: types.ToolResultBlock{Content: large("run partial"), Outcome: types.ToolOutcomePartial, Data: compactProofFixture{runProof("skipped", -1, false)}}, assert: assertRunProof("skipped", -1)},
		{id: "patch-success", tool: "ApplyPatch", result: types.ToolResultBlock{Content: large("patch success"), Outcome: types.ToolOutcomeSucceeded, Data: compactProofFixture{patchProof}}, assert: func(t testing.TB, got agenticV2CompactionEnvelope) {
			if got.Proof == nil || got.Proof.Patch == nil || got.Proof.Patch.CAS != "committed" || got.Proof.Revision.Epoch != 8 {
				t.Fatalf("patch success proof = %#v", got)
			}
		}},
		{id: "patch-cas-failed", tool: "ApplyPatch", result: types.ToolResultBlock{Content: large("patch conflict"), IsError: true, Outcome: types.ToolOutcomeFailed, Data: types.ToolErrorData{Schema: "tool_error/v1", Code: "file.apply_patch.conflict", Retryable: true}}, assert: func(t testing.TB, got agenticV2CompactionEnvelope) {
			if got.Proof == nil || got.Proof.Patch == nil || got.Proof.Revision == nil || got.Proof.Patch.CAS != "rejected" || got.Proof.Patch.FailureReason != "file.apply_patch.conflict" || got.Proof.Revision.Status != "not_issued" || got.Error == nil || !got.Error.Retryable {
				t.Fatalf("patch CAS proof = %#v", got)
			}
		}},
		{id: "patch-commit-unknown", tool: "ApplyPatch", result: types.ToolResultBlock{Content: large("patch rollback failed"), IsError: true, Outcome: types.ToolOutcomeFailed, Data: types.ToolErrorData{Schema: "tool_error/v1", Code: "file.apply_patch.commit"}, Metadata: map[string]string{"apply_patch.failure_reason": "rollback_failed"}}, assert: func(t testing.TB, got agenticV2CompactionEnvelope) {
			if got.Proof == nil || got.Proof.Patch == nil || got.Proof.Revision == nil || got.Proof.Patch.CAS != "commit_state_unknown" || got.Proof.Patch.FailureReason != "rollback_failed" || got.Proof.Revision.Status != "not_issued" {
				t.Fatalf("patch uncertain commit proof = %#v", got)
			}
		}},
		{id: "patch-denied", tool: "ApplyPatch", result: types.ToolResultBlock{Content: large("patch denied"), IsError: true, Outcome: types.ToolOutcomeDenied, Data: types.PolicyDecision{Disposition: types.PolicyBlock, Code: "patch.permission.denied", RuleSource: "settings"}}, assert: func(t testing.TB, got agenticV2CompactionEnvelope) {
			assertPermissionDenied(t, got)
			if got.Proof == nil || got.Proof.Patch == nil || got.Proof.Patch.CAS != "not_authorized" {
				t.Fatalf("patch permission proof = %#v", got)
			}
		}},
		{id: "patch-partial", tool: "ApplyPatch", result: types.ToolResultBlock{Content: large("patch revision receipt failed"), IsError: true, Outcome: types.ToolOutcomePartial, Data: types.ToolErrorData{Schema: "tool_error/v1", Code: "file.apply_patch.commit", Retryable: true}, Metadata: map[string]string{"apply_patch.failure_reason": "revision_receipt_failed"}}, assert: func(t testing.TB, got agenticV2CompactionEnvelope) {
			if got.Proof == nil || got.Proof.Patch == nil || got.Proof.Revision == nil || got.Proof.Patch.CAS != "committed_revision_unsealed" || got.Proof.Patch.FailureReason != "revision_receipt_failed" || got.Proof.Revision.Status != "receipt_failed" || got.Error == nil {
				t.Fatalf("patch partial proof = %#v", got)
			}
		}},
	}
}

func assertProofError(code string) func(testing.TB, agenticV2CompactionEnvelope) {
	return func(t testing.TB, got agenticV2CompactionEnvelope) {
		t.Helper()
		if got.Error == nil || got.Error.Code != code || got.Error.Summary == "" || !got.Error.SummaryTruncated {
			t.Fatalf("error proof = %#v", got)
		}
	}
}

func assertPermissionDenied(t testing.TB, got agenticV2CompactionEnvelope) {
	t.Helper()
	if got.Permission == nil || got.Permission.Decision != "denied" || !got.Permission.Authoritative {
		t.Fatalf("permission proof = %#v", got)
	}
}

func assertRunProof(status string, exit int) func(testing.TB, agenticV2CompactionEnvelope) {
	return func(t testing.TB, got agenticV2CompactionEnvelope) {
		t.Helper()
		if got.Proof == nil || got.Proof.Run == nil || len(got.Proof.Run.Steps) != 1 || got.Proof.Run.Steps[0].Status != status || got.Proof.Run.Steps[0].ExitCode != exit || got.Proof.Run.TotalDurationMS != 37 {
			t.Fatalf("Run proof = %#v", got)
		}
	}
}

func requireAgenticV2Envelope(t testing.TB, messages []types.Message, id string) agenticV2CompactionEnvelope {
	t.Helper()
	content := findMicrocompactTestToolResult(messages, id)
	var envelope agenticV2CompactionEnvelope
	if err := json.Unmarshal([]byte(content), &envelope); err != nil {
		t.Fatalf("%s proof JSON: %v\n%s", id, err, content)
	}
	return envelope
}

func requireCachedProofLedger(t testing.TB, messages []types.Message) cachedAgenticV2ProofLedgerEnvelope {
	t.Helper()
	for _, message := range messages {
		for _, block := range message.Content {
			text, ok := block.(types.TextBlock)
			if !ok || !strings.Contains(text.Text, "agentic-v2-cache-proof-ledger/v1") {
				continue
			}
			var ledger cachedAgenticV2ProofLedgerEnvelope
			if err := json.Unmarshal([]byte(text.Text), &ledger); err != nil {
				t.Fatal(err)
			}
			return ledger
		}
	}
	t.Fatal("cached proof ledger not found")
	return cachedAgenticV2ProofLedgerEnvelope{}
}

func proofForCachedTool(tool string) compactproof.Proof {
	switch tool {
	case "Run":
		return compactproof.Proof{Run: &compactproof.RunProof{LogicalExecutionCommitted: true, Steps: []compactproof.RunStepProof{{Ordinal: 0, Status: "succeeded", ExitCode: 0, DurationMS: 3, Invoked: true}}}}
	case "ApplyPatch":
		return compactproof.Proof{Patch: &compactproof.PatchProof{Status: "success", CAS: "committed", Files: 1}}
	default:
		return compactproof.Proof{Inspect: &compactproof.InspectProof{Requests: 1, Files: 2, Items: 2}}
	}
}

func v2IdleConfig(enabled bool) MicrocompactConfig {
	return MicrocompactConfig{
		KeepRecent: 1, TimeBasedEnabled: true, QuerySource: MicrocompactSourceMain,
		IdleThreshold: time.Minute, LastActivity: time.Now().Add(-2 * time.Hour),
		AgenticV2ProofsEnabled: enabled,
	}
}

func percentReduction(before, after int) float64 {
	if before <= 0 {
		return 0
	}
	return float64(before-after) * 100 / float64(before)
}
