package session

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestSessionMetaPersistsUsageAndPresentationAcrossTranscriptSaves(t *testing.T) {
	store := NewFileStore(t.TempDir())
	messages := []types.Message{types.UserMessage("request"), types.AssistantMessage("answer")}
	if err := store.Save("session-a", messages); err != nil {
		t.Fatalf("save transcript: %v", err)
	}

	wantUsage := &SessionUsageMeta{
		InputTokens:                1200,
		OutputTokens:               300,
		CacheReadTokens:            800,
		CacheCreateTokens:          50,
		HasCompacted:               true,
		RoundUsageKnown:            true,
		CompactionCount:            2,
		CompletedRoundInputTokens:  700,
		CompletedRoundOutputTokens: 180,
		InputTokensAtCompact:       900,
		CacheReadAtCompact:         600,
		LastInputTokens:            500,
		LastOutputTokens:           120,
		LastCacheReadTokens:        200,
		WebSearchRequests:          2,
		CumulativeCost:             0.42,
		CostKnown:                  true,
		UsedTokens:                 1700,
		MaxTokens:                  200000,
	}
	wantPresentation := &SessionPresentationMeta{
		FocusedObservationID: "focus-a",
		ScrollAnchorID:       "anchor-a",
		ScrollOffset:         6,
		InputDraft:           "unsent draft",
		PermissionMode:       "plan",
	}
	if err := store.SaveMeta("session-a", SessionMeta{
		Usage:        wantUsage,
		Presentation: wantPresentation,
	}); err != nil {
		t.Fatalf("save lifecycle metadata: %v", err)
	}

	// Normal transcript persistence derives title/count fields. It must retain
	// the independently-owned presentation and usage sidecar values.
	if err := store.Save("session-a", append(messages, types.UserMessage("next"))); err != nil {
		t.Fatalf("save updated transcript: %v", err)
	}
	got, err := store.GetMeta("session-a")
	if err != nil {
		t.Fatalf("get lifecycle metadata: %v", err)
	}
	if !reflect.DeepEqual(got.Usage, wantUsage) {
		t.Fatalf("usage metadata = %+v, want %+v", got.Usage, wantUsage)
	}
	if !reflect.DeepEqual(got.Presentation, wantPresentation) {
		t.Fatalf("presentation metadata = %+v, want %+v", got.Presentation, wantPresentation)
	}
}

func TestKnownZeroUsageRoundTripsDistinctFromMissingLegacyUsage(t *testing.T) {
	store := NewFileStore(t.TempDir())
	if err := store.Save("known-zero", nil); err != nil {
		t.Fatalf("save known-zero transcript: %v", err)
	}
	if err := store.SaveMeta("known-zero", SessionMeta{Usage: &SessionUsageMeta{}}); err != nil {
		t.Fatalf("save known-zero usage: %v", err)
	}
	known, err := store.GetMeta("known-zero")
	if err != nil {
		t.Fatalf("get known-zero metadata: %v", err)
	}
	if known.Usage == nil {
		t.Fatal("known zero usage was collapsed into missing/unknown metadata")
	}

	if err := store.Save("legacy", []types.Message{types.UserMessage("before usage metadata existed")}); err != nil {
		t.Fatalf("save legacy transcript: %v", err)
	}
	if err := os.Remove(store.metaPath("legacy")); err != nil {
		t.Fatalf("remove generated metadata to simulate legacy session: %v", err)
	}
	legacy, err := store.GetMeta("legacy")
	if err != nil {
		t.Fatalf("derive legacy metadata: %v", err)
	}
	if legacy.Usage != nil {
		t.Fatalf("missing legacy usage was fabricated as measured zero: %+v", legacy.Usage)
	}
}

func TestLegacySessionMetaJSONRemainsBackwardCompatible(t *testing.T) {
	legacyJSON := []byte(`{
		"id": "legacy",
		"title": "old sidecar",
		"cwd": "/repo/old",
		"message_count": 4
	}`)
	var meta SessionMeta
	if err := json.Unmarshal(legacyJSON, &meta); err != nil {
		t.Fatalf("unmarshal legacy metadata: %v", err)
	}
	if meta.ID != "legacy" || meta.Title != "old sidecar" || meta.CWD != "/repo/old" || meta.MessageCount != 4 {
		t.Fatalf("legacy fields changed during decode: %+v", meta)
	}
	if meta.Usage != nil || meta.Presentation != nil || meta.SeenToolUseIDs != nil {
		t.Fatalf("legacy metadata fabricated lifecycle data: usage=%+v presentation=%+v seen=%v", meta.Usage, meta.Presentation, meta.SeenToolUseIDs)
	}
}

func TestLegacyCompactedUsageDoesNotFabricateRoundEndpointTotals(t *testing.T) {
	legacyJSON := []byte(`{
		"id": "legacy-usage",
		"usage": {
			"input_tokens": 2500,
			"output_tokens": 200,
			"has_compacted": true,
			"input_tokens_at_compact": 1800,
			"last_input_tokens": 700
		}
	}`)
	var meta SessionMeta
	if err := json.Unmarshal(legacyJSON, &meta); err != nil {
		t.Fatalf("unmarshal legacy compacted usage: %v", err)
	}
	if meta.Usage == nil || !meta.Usage.HasCompacted || meta.Usage.InputTokens != 2500 || meta.Usage.LastInputTokens != 700 {
		t.Fatalf("legacy compacted usage changed during decode: %+v", meta.Usage)
	}
	if meta.Usage.RoundUsageKnown || meta.Usage.CompactionCount != 0 ||
		meta.Usage.CompletedRoundInputTokens != 0 || meta.Usage.CompletedRoundOutputTokens != 0 {
		t.Fatalf("legacy compacted usage fabricated exact round endpoints: %+v", meta.Usage)
	}
}

func TestSessionToolUseIdentityLedgerIsStableAndSurvivesTranscriptSaves(t *testing.T) {
	store := NewFileStore(t.TempDir())
	if err := store.Save("ledger", []types.Message{types.UserMessage("compacted transcript")}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMeta("ledger", SessionMeta{SeenToolUseIDs: []string{"tool-z", "tool-a", "tool-z", "  "}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save("ledger", []types.Message{types.UserMessage("later compacted transcript")}); err != nil {
		t.Fatal(err)
	}
	meta, err := store.GetMeta("ledger")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"tool-a", "tool-z"}; !reflect.DeepEqual(meta.SeenToolUseIDs, want) {
		t.Fatalf("seen tool ledger = %v, want %v", meta.SeenToolUseIDs, want)
	}
}

func TestClearConversationKeepsOldTranscriptAndLifecycleAudit(t *testing.T) {
	store := NewFileStore(t.TempDir())
	oldMessages := []types.Message{types.UserMessage("audited request"), types.AssistantMessage("audited answer")}
	oldUsage := &SessionUsageMeta{InputTokens: 700, OutputTokens: 90, UsedTokens: 640, MaxTokens: 1000}
	oldPresentation := &SessionPresentationMeta{PermissionMode: "plan", ScrollAnchorID: "old-anchor", ScrollOffset: 3}
	if err := store.Save("old", oldMessages); err != nil {
		t.Fatalf("save old transcript: %v", err)
	}
	if err := store.SaveMeta("old", SessionMeta{Usage: oldUsage, Presentation: oldPresentation, SeenToolUseIDs: []string{"tool-old"}}); err != nil {
		t.Fatalf("save old lifecycle metadata: %v", err)
	}

	// clear conversation creates another session. It must not implement clear by
	// overwriting the old session's transcript or metadata.
	if err := store.Save("new", nil); err != nil {
		t.Fatalf("create new empty session: %v", err)
	}
	if err := store.SaveMeta("new", SessionMeta{
		Usage:        &SessionUsageMeta{},
		Presentation: &SessionPresentationMeta{PermissionMode: "ask"},
	}); err != nil {
		t.Fatalf("save new session lifecycle metadata: %v", err)
	}

	gotMessages, err := store.Load("old")
	if err != nil {
		t.Fatalf("load old audit: %v", err)
	}
	if !reflect.DeepEqual(gotMessages, oldMessages) {
		t.Fatalf("clear conversation changed old transcript: got %#v want %#v", gotMessages, oldMessages)
	}
	gotMeta, err := store.GetMeta("old")
	if err != nil {
		t.Fatalf("load old lifecycle audit: %v", err)
	}
	if !reflect.DeepEqual(gotMeta.Usage, oldUsage) || !reflect.DeepEqual(gotMeta.Presentation, oldPresentation) {
		t.Fatalf("clear conversation changed old lifecycle audit: %+v", gotMeta)
	}
	if !reflect.DeepEqual(gotMeta.SeenToolUseIDs, []string{"tool-old"}) {
		t.Fatalf("clear conversation changed old identity ledger: %v", gotMeta.SeenToolUseIDs)
	}
	newMeta, err := store.GetMeta("new")
	if err != nil {
		t.Fatal(err)
	}
	if len(newMeta.SeenToolUseIDs) != 0 {
		t.Fatalf("new clear-conversation session inherited old identity ledger: %v", newMeta.SeenToolUseIDs)
	}
}

func TestSessionDecisionAuditSurvivesTranscriptSave(t *testing.T) {
	store := NewFileStore(t.TempDir())
	if err := store.Save("decision-session", []types.Message{types.UserMessage("request")}); err != nil {
		t.Fatal(err)
	}
	want := []SessionDecisionMeta{{
		DecisionID: "decision-1", ExecutionSessionID: "agent-session", ToolUseID: "tool-1", ActorID: "agent-1", Kind: "permission",
		ToolName: "Write", Input: map[string]any{"file_path": "/tmp/out", "content": "exact"}, RiskLevel: 3, Message: "approval required",
		Action: "Write file", Target: "/tmp/out", Impact: "replace contents", RiskReason: "overwrite",
		RuleSource: "project rule", ApprovalScope: "once", Choices: []string{"allow_once", "reject"},
		Outcome: "rejected", Choice: "reject",
	}}
	if err := store.SaveMeta("decision-session", SessionMeta{Decisions: want}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save("decision-session", []types.Message{types.UserMessage("request"), types.AssistantMessage("answer")}); err != nil {
		t.Fatal(err)
	}
	meta, err := store.GetMeta("decision-session")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(meta.Decisions, want) {
		t.Fatalf("decision audit = %+v, want %+v", meta.Decisions, want)
	}
}

func TestSessionStructuredEvidenceReferencesRoundTrip(t *testing.T) {
	store := NewFileStore(t.TempDir())
	if err := store.Save("evidence-session", []types.Message{types.UserMessage("retain evidence")}); err != nil {
		t.Fatal(err)
	}
	want := []SessionEvidenceMeta{{
		ObservationID: "tool:session:toolu",
		Results:       []SessionDetailRefMeta{{Source: "file", Key: "result", Size: 4, Digest: strings.Repeat("a", 64)}},
		Envelopes:     []SessionDetailRefMeta{{Source: "file", Key: "envelope", Size: 9, Digest: strings.Repeat("b", 64)}},
	}}
	if err := store.SaveMeta("evidence-session", SessionMeta{Evidence: want}); err != nil {
		t.Fatal(err)
	}
	meta, err := store.GetMeta("evidence-session")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(meta.Evidence, want) {
		t.Fatalf("evidence metadata = %+v, want %+v", meta.Evidence, want)
	}
}

func TestSessionActivityPresentationV2RoundTripsAndPreservesNestedRefs(t *testing.T) {
	store := NewFileStore(t.TempDir())
	if err := store.Save("activity-v2", []types.Message{types.UserMessage("retain activity")}); err != nil {
		t.Fatal(err)
	}
	wantActivity := []SessionActivityMeta{{
		ID: "background:agent", RunID: "run-2", Attempt: 2, BatchID: "batch", ParentRunID: "parent", AgentPath: "lead/agent",
		State: "needs_input", Lifecycle: "blocked", AttentionKind: "needs_input", AttentionUnread: true,
		Outcome: "running", ProgressMessage: "waiting for decision", FirstSequence: 4, LastSequence: 8,
		DetailRefs: []SessionDetailRefMeta{{Source: "file", Key: "agent.jsonl", Size: 42, Digest: strings.Repeat("c", 64)}},
	}}
	wantPresentation := &SessionPresentationMeta{Version: 2, ActivityFocus: "background:agent", ActivityViewOffset: 3}
	if err := store.SaveMeta("activity-v2", SessionMeta{Presentation: wantPresentation, Activities: wantActivity}); err != nil {
		t.Fatal(err)
	}
	meta, err := store.GetMeta("activity-v2")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(meta.Activities, wantActivity) || !reflect.DeepEqual(meta.Presentation, wantPresentation) {
		t.Fatalf("activity v2 round trip=%+v presentation=%+v", meta.Activities, meta.Presentation)
	}
	meta.Activities[0].DetailRefs[0].Key = "mutated"
	again, err := store.GetMeta("activity-v2")
	if err != nil {
		t.Fatal(err)
	}
	if again.Activities[0].DetailRefs[0].Key != "agent.jsonl" {
		t.Fatalf("activity metadata alias leaked across reads: %+v", again.Activities)
	}
}
