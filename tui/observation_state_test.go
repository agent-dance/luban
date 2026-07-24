package tui

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestObservationStoreAssociatesOutOfOrderSameNameResultsByToolUseID(t *testing.T) {
	details := NewMemoryDetailStore()
	store := NewObservationStore(details)
	ctx := ToolEventContext{SessionID: "session-concurrent", TurnID: "turn-9", WorkUnitID: "work-main"}

	for _, call := range []types.ToolUseBlock{
		{ID: "toolu-a", Name: "Read", Input: map[string]any{"file_path": "/tmp/a"}},
		{ID: "toolu-b", Name: "Read", Input: map[string]any{"file_path": "/tmp/b"}},
	} {
		if err := store.ApplyToolCall(ctx, call); err != nil {
			t.Fatalf("ApplyToolCall(%s) error = %v", call.ID, err)
		}
	}

	resultCtx := ctx
	resultCtx.Outcome = OutcomeSucceeded
	for _, result := range []types.ToolResultBlock{
		{ToolUseID: "toolu-b", Content: "evidence-for-b"},
		{ToolUseID: "toolu-a", Content: "evidence-for-a"},
	} {
		if err := store.ApplyToolResult(resultCtx, result); err != nil {
			t.Fatalf("ApplyToolResult(%s) error = %v", result.ToolUseID, err)
		}
	}

	snapshot := store.Snapshot()
	if len(snapshot) != 2 {
		t.Fatalf("observations = %d, want exactly one per tool call", len(snapshot))
	}
	a := observationByToolUseID(t, snapshot, "toolu-a")
	b := observationByToolUseID(t, snapshot, "toolu-b")
	if a.ID == "" || b.ID == "" || a.ID == b.ID {
		t.Fatalf("observation IDs are not stable and distinct: a=%q b=%q", a.ID, b.ID)
	}
	assertSingleEvidence(t, details, a, "evidence-for-a")
	assertSingleEvidence(t, details, b, "evidence-for-b")
}

func TestToolCallAndResultShareOneObservationAndDisclosureState(t *testing.T) {
	details := NewMemoryDetailStore()
	store := NewObservationStore(details)
	ctx := ToolEventContext{SessionID: "session-disclosure", TurnID: "turn-1"}
	if err := store.ApplyToolCall(ctx, types.ToolUseBlock{
		ID:    "toolu-joint",
		Name:  "Grep",
		Input: map[string]any{"pattern": "TODO"},
	}); err != nil {
		t.Fatalf("ApplyToolCall() error = %v", err)
	}
	ctx.Outcome = OutcomeSucceeded
	if err := store.ApplyToolResult(ctx, types.ToolResultBlock{
		ToolUseID: "toolu-joint",
		Content:   "a.go:1:TODO\nb.go:2:TODO\n",
	}); err != nil {
		t.Fatalf("ApplyToolResult() error = %v", err)
	}

	before := observationByToolUseID(t, store.Snapshot(), "toolu-joint")
	if len(store.Snapshot()) != 1 {
		t.Fatalf("call and result produced %d observations, want 1", len(store.Snapshot()))
	}
	wantDisclosure := DisclosureState{Level: DisclosureEvidence, HasMore: true, UserPinned: true}
	if err := store.SetDisclosure(before.ID, wantDisclosure); err != nil {
		t.Fatalf("SetDisclosure() error = %v", err)
	}

	after, ok := store.Get(before.ID)
	if !ok {
		t.Fatalf("Get(%q) did not find observation", before.ID)
	}
	if after.ToolName != "Grep" || len(after.ResultRefs) != 1 {
		t.Fatalf("joint observation lost call/result data: %+v", after)
	}
	if after.Disclosure != wantDisclosure {
		t.Fatalf("Disclosure = %+v, want %+v", after.Disclosure, wantDisclosure)
	}
}

func TestMissingToolUseIDsNeverUseAdjacentCallFallback(t *testing.T) {
	details := NewMemoryDetailStore()
	store := NewObservationStore(details)
	ctx := ToolEventContext{SessionID: "legacy-session", TurnID: "legacy-turn"}

	// Reducers may return a diagnostic error for malformed legacy events, but
	// they must still retain each event as independent audit evidence.
	_ = store.ApplyToolCall(ctx, types.ToolUseBlock{Name: "Read", Input: map[string]any{"file_path": "/tmp/a"}})
	_ = store.ApplyToolResult(ctx, types.ToolResultBlock{Content: "orphan evidence"})

	snapshot := store.Snapshot()
	if len(snapshot) != 2 {
		t.Fatalf("adjacent missing-ID call/result produced %d observations, want 2 independent orphans", len(snapshot))
	}
	if snapshot[0].ID == snapshot[1].ID {
		t.Fatalf("missing-ID events shared observation ID %q", snapshot[0].ID)
	}
	for _, observation := range snapshot {
		if observation.ToolName != "" && len(observation.ResultRefs) != 0 {
			t.Fatalf("adjacency fallback joined orphan result to call: %+v", observation)
		}
	}
	if snapshot[0].Outcome != OutcomeOrphan || snapshot[1].Outcome != OutcomeOrphan {
		t.Fatalf("missing-ID outcomes = [%v %v], want explicit orphan outcomes", snapshot[0].Outcome, snapshot[1].Outcome)
	}
}

func TestDuplicateToolUseIDCreatesConflictInsteadOfArbitraryBinding(t *testing.T) {
	store := NewObservationStore(NewMemoryDetailStore())
	ctx := ToolEventContext{SessionID: "session-duplicate", TurnID: "turn-1"}
	if err := store.ApplyToolCall(ctx, types.ToolUseBlock{ID: "toolu-dup", Name: "Read", Input: map[string]any{"file_path": "/tmp/a"}}); err != nil {
		t.Fatalf("first ApplyToolCall() error = %v", err)
	}
	err := store.ApplyToolCall(ctx, types.ToolUseBlock{ID: "toolu-dup", Name: "Read", Input: map[string]any{"file_path": "/tmp/b"}})
	if err == nil {
		t.Fatal("duplicate ApplyToolCall() returned nil error; conflict was not surfaced to the caller")
	}

	snapshot := store.Snapshot()
	if len(snapshot) != 2 {
		t.Fatalf("duplicate calls produced %d observations, want original plus explicit conflict", len(snapshot))
	}
	conflicts := 0
	for _, observation := range snapshot {
		if observation.Outcome == OutcomeConflict {
			conflicts++
		}
	}
	if conflicts != 1 {
		t.Fatalf("conflict observations = %d, want 1; snapshot=%+v", conflicts, snapshot)
	}
}

func TestPinnedDisclosureIsIndependentPerObservation(t *testing.T) {
	store := NewObservationStore(NewMemoryDetailStore())
	ctx := ToolEventContext{SessionID: "session-pinned", TurnID: "turn-1"}
	for _, call := range []types.ToolUseBlock{
		{ID: "toolu-pinned", Name: "Read"},
		{ID: "toolu-default", Name: "Read"},
	} {
		if err := store.ApplyToolCall(ctx, call); err != nil {
			t.Fatalf("ApplyToolCall(%s) error = %v", call.ID, err)
		}
	}
	ctx.Outcome = OutcomeSucceeded
	for _, result := range []types.ToolResultBlock{
		{ToolUseID: "toolu-pinned", Content: "pinned evidence"},
		{ToolUseID: "toolu-default", Content: "default evidence"},
	} {
		if err := store.ApplyToolResult(ctx, result); err != nil {
			t.Fatalf("ApplyToolResult(%s) error = %v", result.ToolUseID, err)
		}
	}

	pinned := observationByToolUseID(t, store.Snapshot(), "toolu-pinned")
	other := observationByToolUseID(t, store.Snapshot(), "toolu-default")
	otherBefore := other.Disclosure
	wantPinned := DisclosureState{Level: DisclosureEvidence, HasMore: true, UserPinned: true}
	if err := store.SetDisclosure(pinned.ID, wantPinned); err != nil {
		t.Fatalf("SetDisclosure() error = %v", err)
	}

	pinnedAfter, ok := store.Get(pinned.ID)
	if !ok {
		t.Fatalf("Get(%q) did not find pinned observation", pinned.ID)
	}
	otherAfter, ok := store.Get(other.ID)
	if !ok {
		t.Fatalf("Get(%q) did not find sibling observation", other.ID)
	}
	if pinnedAfter.Disclosure != wantPinned {
		t.Fatalf("pinned Disclosure = %+v, want %+v", pinnedAfter.Disclosure, wantPinned)
	}
	if otherAfter.Disclosure != otherBefore {
		t.Fatalf("pinning one observation changed sibling disclosure: before=%+v after=%+v", otherBefore, otherAfter.Disclosure)
	}
}

func TestPinnedIndexDoesNotDuplicateAfterUnpinRepinCycles(t *testing.T) {
	store := NewObservationStore(NewMemoryDetailStore())
	ctx := ToolEventContext{SessionID: "session", TurnID: "turn"}
	if err := store.ApplyToolCall(ctx, types.ToolUseBlock{ID: "tool", Name: "Read"}); err != nil {
		t.Fatal(err)
	}
	id := toolObservationID("session", "tool")
	for i := 0; i < 100; i++ {
		if err := store.SetDisclosure(id, DisclosureState{Level: DisclosureDetail, UserPinned: true}); err != nil {
			t.Fatal(err)
		}
		if err := store.SetDisclosure(id, DisclosureState{Level: DisclosureSummary}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SetDisclosure(id, DisclosureState{Level: DisclosureDetail, UserPinned: true}); err != nil {
		t.Fatal(err)
	}
	if got := store.PinnedSnapshot(); len(got) != 1 || got[0].ID != id {
		t.Fatalf("pinned snapshot after repin cycles = %+v", got)
	}
}

type countingDetailStore struct {
	inner DetailStore
	puts  int
	gets  int
}

func (s *countingDetailStore) Put(key string, data []byte) (DetailRef, error) {
	s.puts++
	return s.inner.Put(key, data)
}
func (s *countingDetailStore) Get(ref DetailRef) ([]byte, error) {
	s.gets++
	return s.inner.Get(ref)
}

func TestEvidenceDisclosureReadsRetainedDetailWithoutProducingNewEvidence(t *testing.T) {
	details := &countingDetailStore{inner: NewMemoryDetailStore()}
	state := NewAppState()
	state.Details = details
	state.Observations = NewObservationStore(details)
	ctx := ToolEventContext{SessionID: "session", Outcome: OutcomeSucceeded}
	if err := state.ApplyToolCall(ctx, types.ToolUseBlock{ID: "tool", Name: "Read"}); err != nil {
		t.Fatal(err)
	}
	if err := state.ApplyToolResult(ctx, types.ToolResultBlock{ToolUseID: "tool", Content: "retained evidence"}); err != nil {
		t.Fatal(err)
	}
	putsBefore := details.puts
	if err := state.RevealObservation(toolObservationID("session", "tool"), DisclosureEvidence); err != nil {
		t.Fatal(err)
	}
	root := NewRootComponent(state, nil, nil)
	_ = collectElementText(root.renderMessageArea(12))
	if details.puts != putsBefore {
		t.Fatalf("viewing evidence created %d new detail writes", details.puts-putsBefore)
	}
	if details.gets == 0 {
		t.Fatal("evidence view did not read the retained detail")
	}
}

func TestGlobalShowAllDoesNotMutateLocalDisclosure(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
	ctx := ToolEventContext{SessionID: "session", TurnID: "session:turn-1", ActorID: "assistant", WorkUnitID: "verify", Outcome: OutcomeSucceeded}
	call := types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "tool-show", Name: "Bash", Input: map[string]any{"command": "go test ./..."}}
	if err := state.ApplyToolCall(ctx, call); err != nil {
		t.Fatal(err)
	}
	if err := state.ApplyToolResult(ctx, types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: call.ID, Content: "exact evidence", Outcome: types.ToolOutcomeSucceeded}); err != nil {
		t.Fatal(err)
	}
	observationID := toolObservationID("session", call.ID)
	before, _ := state.GetObservation(observationID)
	if before.Disclosure.Level != DisclosureSummary || before.Disclosure.UserPinned {
		t.Fatalf("local disclosure before show-all = %+v", before.Disclosure)
	}
	message := messageFromObservation(before, MsgToolCall)
	root := NewRootComponent(state, nil, nil)
	if text := collectElementText(root.renderToolObservation(message)); strings.Contains(text, "exact evidence") {
		t.Fatalf("summary unexpectedly rendered full evidence: %s", text)
	}
	state.TranscriptShowAll.Set(true)
	if text := collectElementText(root.renderToolObservation(message)); !strings.Contains(text, "exact evidence") || !strings.Contains(text, "Evidence ID") {
		t.Fatalf("show-all did not render complete evidence: %s", text)
	}
	after, _ := state.GetObservation(observationID)
	if after.Disclosure != before.Disclosure {
		t.Fatalf("global show-all mutated local disclosure: before=%+v after=%+v", before.Disclosure, after.Disclosure)
	}
}

func TestObservationEvidenceExportsExactSingleDetailWithoutReplay(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
	ctx := ToolEventContext{SessionID: "session", TurnID: "session:turn-2", WorkUnitID: "work", ActorID: "agent", Outcome: OutcomePartial}
	call := types.ToolUseBlock{ID: "tool-detail", Name: "Bash", Input: map[string]any{"command": "go test ./..."}}
	if err := state.ApplyToolCall(ctx, call); err != nil {
		t.Fatal(err)
	}
	result := types.ToolResultBlock{ToolUseID: call.ID, Content: "complete raw output", IsError: true, Outcome: types.ToolOutcomePartial, Metadata: map[string]string{"exit": "1"}}
	if err := state.ApplyToolResult(ctx, result); err != nil {
		t.Fatal(err)
	}
	observationID := toolObservationID("session", call.ID)
	before := len(state.ObservationSnapshot())
	evidence, err := state.ObservationEvidence(observationID)
	if err != nil {
		t.Fatal(err)
	}
	text := string(evidence)
	for _, want := range []string{observationID, "session:turn-2", "work", "agent", "go test ./...", "complete raw output", "Structured evidence"} {
		if !strings.Contains(text, want) {
			t.Fatalf("single-detail export omitted %q:\n%s", want, text)
		}
	}
	if after := len(state.ObservationSnapshot()); after != before {
		t.Fatalf("opening detail replayed or mutated observations: before=%d after=%d", before, after)
	}
}

func observationByToolUseID(t *testing.T, observations []Observation, toolUseID string) Observation {
	t.Helper()
	var matches []Observation
	for _, observation := range observations {
		if observation.ToolUseID == toolUseID {
			matches = append(matches, observation)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("observations with ToolUseID %q = %d, want 1; snapshot=%+v", toolUseID, len(matches), observations)
	}
	return matches[0]
}

func assertSingleEvidence(t *testing.T, details DetailStore, observation Observation, want string) {
	t.Helper()
	if len(observation.ResultRefs) != 1 {
		t.Fatalf("observation %q ResultRefs = %d, want 1", observation.ID, len(observation.ResultRefs))
	}
	got, err := details.Get(observation.ResultRefs[0])
	if err != nil {
		t.Fatalf("Get(%q evidence) error = %v", observation.ID, err)
	}
	if string(got) != want {
		t.Fatalf("observation %q evidence = %q, want %q", observation.ID, got, want)
	}
}
