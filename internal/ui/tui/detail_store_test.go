package tui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

type failSecondPutStore struct {
	inner DetailStore
	puts  int
}

func (s *failSecondPutStore) Put(key string, data []byte) (DetailRef, error) {
	s.puts++
	if s.puts == 2 {
		return DetailRef{}, errors.New("injected envelope write failure")
	}
	return s.inner.Put(key, data)
}

func (s *failSecondPutStore) Get(ref DetailRef) ([]byte, error) { return s.inner.Get(ref) }

func TestStructuredEnvelopeWriteFailureKeepsRawResultReachable(t *testing.T) {
	details := &failSecondPutStore{inner: NewMemoryDetailStore()}
	store := NewObservationStore(details)
	ctx := ToolEventContext{SessionID: "session", Outcome: OutcomeSucceeded}
	if err := store.ApplyToolCall(ctx, types.ToolUseBlock{ID: "tool", Name: "Read"}); err != nil {
		t.Fatal(err)
	}
	err := store.ApplyToolResult(ctx, types.ToolResultBlock{ToolUseID: "tool", Content: "raw result", Metadata: map[string]string{"request_id": "r"}})
	if err == nil || !strings.Contains(err.Error(), "envelope") {
		t.Fatalf("ApplyToolResult error = %v", err)
	}
	observation, ok := store.Get(toolObservationID("session", "tool"))
	if !ok || len(observation.ResultRefs) != 1 {
		t.Fatalf("raw result became unreachable: %+v", observation)
	}
	got, err := details.Get(observation.ResultRefs[0])
	if err != nil || string(got) != "raw result" {
		t.Fatalf("reachable raw result = %q, err %v", got, err)
	}
}

func TestDetailStoresContentAddressRepeatedLogicalKeys(t *testing.T) {
	stores := map[string]DetailStore{"memory": NewMemoryDetailStore()}
	fileStore, err := NewFileDetailStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileDetailStore() error = %v", err)
	}
	stores["file"] = fileStore
	for name, store := range stores {
		t.Run(name, func(t *testing.T) {
			first, err := store.Put("same-logical-key", []byte("AAAA"))
			if err != nil {
				t.Fatal(err)
			}
			second, err := store.Put("same-logical-key", []byte("BBBB"))
			if err != nil {
				t.Fatal(err)
			}
			if first.Digest == second.Digest {
				t.Fatalf("distinct same-sized evidence shared digest %q", first.Digest)
			}
			gotFirst, err := store.Get(first)
			if err != nil || string(gotFirst) != "AAAA" {
				t.Fatalf("first evidence = %q, %v", gotFirst, err)
			}
			gotSecond, err := store.Get(second)
			if err != nil || string(gotSecond) != "BBBB" {
				t.Fatalf("second evidence = %q, %v", gotSecond, err)
			}
		})
	}
}

func TestSyncDirectoryUsesRuntimePlatformSemantics(t *testing.T) {
	if err := syncDirectory(t.TempDir()); err != nil {
		t.Fatalf("syncDirectory() error = %v", err)
	}
}

func TestFileDetailStoreRoundTripsOnCurrentPlatform(t *testing.T) {
	store, err := NewFileDetailStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ref, err := store.Put("windows-mode-regression", []byte("detail payload"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ref)
	if err != nil || string(got) != "detail payload" {
		t.Fatalf("Get() = %q, %v", got, err)
	}
}

func TestFileDetailStoreObservationJournalRoundTripsOnCurrentPlatform(t *testing.T) {
	store, err := NewFileDetailStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ref, err := store.Put("journal-mode-regression", []byte("journal evidence"))
	if err != nil {
		t.Fatal(err)
	}
	want := Observation{ID: "observation-1", ResultRefs: []DetailRef{ref}}
	if err := store.SaveObservationEvidence(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadObservationEvidence()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != want.ID || len(got[0].ResultRefs) != 1 || got[0].ResultRefs[0] != ref {
		t.Fatalf("LoadObservationEvidence() = %+v", got)
	}
}

func TestStructuredToolResultRetainsCompleteEnvelope(t *testing.T) {
	details := NewMemoryDetailStore()
	observations := NewObservationStore(details)
	ctx := ToolEventContext{SessionID: "structured", Outcome: OutcomePartial}
	if err := observations.ApplyToolCall(ctx, types.ToolUseBlock{ID: "toolu-structured", Name: "MCP"}); err != nil {
		t.Fatal(err)
	}
	result := types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: "toolu-structured", Content: "visible",
		ContentBlocks: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "visible"},
			types.ImageBlock{Type: types.ContentTypeImage, Source: &types.ImageSource{Type: "base64", MediaType: "image/png", Data: "AAEC"}},
		},
		Data: map[string]any{"cursor": "next"}, Metadata: map[string]string{"request_id": "req-7"},
		NewMessages: []types.Message{types.AssistantMessage("follow-up")},
		Usage:       &types.Usage{InputTokens: 3}, Outcome: types.ToolOutcomePartial,
	}
	if err := observations.ApplyToolResult(ctx, result); err != nil {
		t.Fatal(err)
	}
	observation := observationByToolUseID(t, observations.Snapshot(), result.ToolUseID)
	if len(observation.EnvelopeRefs) != 1 {
		t.Fatalf("EnvelopeRefs = %d, want 1", len(observation.EnvelopeRefs))
	}
	envelope, err := details.Get(observation.EnvelopeRefs[0])
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(envelope, &decoded); err != nil {
		t.Fatalf("structured envelope is invalid JSON: %v", err)
	}
	for _, want := range []string{"image/png", "AAEC", "next", "req-7", "follow-up", "partial"} {
		if !bytes.Contains(envelope, []byte(want)) {
			t.Fatalf("structured envelope omitted %q: %s", want, envelope)
		}
	}
}

func TestMemoryDetailStorePreservesExactEvidence(t *testing.T) {
	store := NewMemoryDetailStore()
	want := []byte("first line\n\x00binary-adjacent\r\n最后一行\n")

	ref, err := store.Put("tool-result/toolu-exact", want)
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	// Mutating the caller's buffer must not mutate retained evidence.
	wantCopy := append([]byte(nil), want...)
	want[0] = 'X'
	got, err := store.Get(ref)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !bytes.Equal(got, wantCopy) {
		t.Fatalf("Get() evidence differs from Put() input:\n got: %q\nwant: %q", got, wantCopy)
	}

	// Mutating a retrieved value must not corrupt subsequent reads either.
	got[0] = 'Y'
	again, err := store.Get(ref)
	if err != nil {
		t.Fatalf("second Get() error = %v", err)
	}
	if !bytes.Equal(again, wantCopy) {
		t.Fatalf("stored evidence was mutable through Get():\n got: %q\nwant: %q", again, wantCopy)
	}
}

func TestLongToolResultDefaultsToSummaryAndRetainsExactEvidence(t *testing.T) {
	details := NewMemoryDetailStore()
	observations := NewObservationStore(details)
	ctx := ToolEventContext{SessionID: "session-long", TurnID: "turn-1"}
	call := types.ToolUseBlock{ID: "toolu-long", Name: "Read", Input: map[string]any{"file_path": "/tmp/long.txt"}}
	if err := observations.ApplyToolCall(ctx, call); err != nil {
		t.Fatalf("ApplyToolCall() error = %v", err)
	}

	var lines []string
	for i := 0; i < 37; i++ {
		lines = append(lines, fmt.Sprintf("line %02d: payload that must not be truncated", i+1))
	}
	want := strings.Join(lines, "\n") + "\n"
	resultCtx := ctx
	resultCtx.Outcome = OutcomeSucceeded
	if err := observations.ApplyToolResult(resultCtx, types.ToolResultBlock{
		ToolUseID: "toolu-long",
		Content:   want,
	}); err != nil {
		t.Fatalf("ApplyToolResult() error = %v", err)
	}

	observation := observationByToolUseID(t, observations.Snapshot(), "toolu-long")
	if observation.Disclosure.Level != DisclosureSummary {
		t.Fatalf("default disclosure = %v, want Summary", observation.Disclosure.Level)
	}
	if !observation.Disclosure.HasMore {
		t.Fatal("long result did not advertise retrievable detail")
	}
	if len(observation.ResultRefs) != 1 {
		t.Fatalf("ResultRefs = %d, want 1", len(observation.ResultRefs))
	}

	got, err := details.Get(observation.ResultRefs[0])
	if err != nil {
		t.Fatalf("Get(result evidence) error = %v", err)
	}
	if string(got) != want {
		t.Fatalf("retrieved result was truncated or rewritten:\n got: %q\nwant: %q", got, want)
	}
}

func TestStructuredAndPriorResultEvidenceKeepsHasMoreAcrossEmptyUpdate(t *testing.T) {
	details := NewMemoryDetailStore()
	observations := NewObservationStore(details)
	ctx := ToolEventContext{SessionID: "session-structured", TurnID: "turn-1", Outcome: OutcomeSucceeded}
	if err := observations.ApplyToolCall(ctx, types.ToolUseBlock{ID: "tool-structured", Name: "Read"}); err != nil {
		t.Fatal(err)
	}
	if err := observations.ApplyToolResult(ctx, types.ToolResultBlock{
		ToolUseID: "tool-structured", Data: map[string]any{"exact": []string{"a", "b"}}, Metadata: map[string]string{"source": "tool"},
	}); err != nil {
		t.Fatal(err)
	}
	first := observationByToolUseID(t, observations.Snapshot(), "tool-structured")
	if !first.Disclosure.HasMore || len(first.EnvelopeRefs) != 1 {
		t.Fatalf("structured-only result did not advertise evidence: %+v", first)
	}
	if err := observations.ApplyToolResult(ctx, types.ToolResultBlock{ToolUseID: "tool-structured"}); err != nil {
		t.Fatal(err)
	}
	second := observationByToolUseID(t, observations.Snapshot(), "tool-structured")
	if !second.Disclosure.HasMore || len(second.ResultRefs) != 2 || len(second.EnvelopeRefs) != 1 {
		t.Fatalf("empty follow-up hid cumulative evidence: %+v", second)
	}
}

func TestToolResultOutcomesRemainDistinctAndLossless(t *testing.T) {
	tests := []struct {
		name      string
		outcome   ObservationOutcome
		isError   bool
		wantLevel DisclosureLevel
		evidence  string
	}{
		{name: "error", outcome: OutcomeFailed, isError: true, wantLevel: DisclosureDetail, evidence: "exit status 17\nstderr: precise failure\n"},
		{name: "partial", outcome: OutcomePartial, isError: true, wantLevel: DisclosureDetail, evidence: "2 of 5 shards completed\npartial artifact: /tmp/p\n"},
		{name: "denied", outcome: OutcomeDenied, isError: true, wantLevel: DisclosureDetail, evidence: "rule ask-always denied write to /etc/hosts\n"},
		{name: "cancelled", outcome: OutcomeCancelled, isError: true, wantLevel: DisclosureDetail, evidence: "cancelled by actor user-7 after 81ms\n"},
		{name: "timeout", outcome: OutcomeTimedOut, isError: true, wantLevel: DisclosureDetail, evidence: "deadline 250ms exceeded; retained stdout\n"},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			details := NewMemoryDetailStore()
			observations := NewObservationStore(details)
			id := fmt.Sprintf("toolu-outcome-%d", i)
			ctx := ToolEventContext{SessionID: "session-outcomes", TurnID: "turn-1"}
			if err := observations.ApplyToolCall(ctx, types.ToolUseBlock{ID: id, Name: "Bash"}); err != nil {
				t.Fatalf("ApplyToolCall() error = %v", err)
			}
			ctx.Outcome = tt.outcome
			if err := observations.ApplyToolResult(ctx, types.ToolResultBlock{
				ToolUseID: id,
				Content:   tt.evidence,
				IsError:   tt.isError,
			}); err != nil {
				t.Fatalf("ApplyToolResult() error = %v", err)
			}

			observation := observationByToolUseID(t, observations.Snapshot(), id)
			if observation.Outcome != tt.outcome {
				t.Fatalf("Outcome = %v, want %v", observation.Outcome, tt.outcome)
			}
			if observation.Disclosure.Level != tt.wantLevel {
				t.Fatalf("Disclosure.Level = %v, want %v", observation.Disclosure.Level, tt.wantLevel)
			}
			if len(observation.ResultRefs) != 1 {
				t.Fatalf("ResultRefs = %d, want 1", len(observation.ResultRefs))
			}
			got, err := details.Get(observation.ResultRefs[0])
			if err != nil {
				t.Fatalf("Get(result evidence) error = %v", err)
			}
			if string(got) != tt.evidence {
				t.Fatalf("evidence = %q, want %q", got, tt.evidence)
			}
		})
	}
}

func TestStructuredOutcomeDoesNotDependOnResultText(t *testing.T) {
	details := NewMemoryDetailStore()
	observations := NewObservationStore(details)
	ctx := ToolEventContext{
		SessionID: "session-structured-outcome",
		TurnID:    "turn-1",
		Outcome:   OutcomeSucceeded,
	}
	if err := observations.ApplyToolCall(ctx, types.ToolUseBlock{ID: "toolu-text", Name: "Read"}); err != nil {
		t.Fatalf("ApplyToolCall() error = %v", err)
	}
	if err := observations.ApplyToolResult(ctx, types.ToolResultBlock{
		ToolUseID: "toolu-text",
		Content:   "This documentation explains: permission denied, cancelled, timeout, and error.",
	}); err != nil {
		t.Fatalf("ApplyToolResult() error = %v", err)
	}

	observation := observationByToolUseID(t, observations.Snapshot(), "toolu-text")
	if observation.Outcome != OutcomeSucceeded {
		t.Fatalf("Outcome = %v, want structured success despite error-like prose", observation.Outcome)
	}
}
