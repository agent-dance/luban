package tui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/types"
)

func TestTranscriptSearchUsesRetainedEvidenceAndStableObservationIDs(t *testing.T) {
	details := NewMemoryDetailStore()
	observations := NewObservationStore(details)
	ctx := ToolEventContext{SessionID: "session-search", TurnID: "turn-1", WorkUnitID: "work-main"}

	firstEvidence := strings.Repeat("viewport-hidden-line\n", 200) + "first deep evidence needle\n"
	secondEvidence := strings.Repeat("another-hidden-line\n", 180) + "second deep evidence needle\n"
	applyTranscriptToolObservation(t, observations, ctx, "toolu-first", "Read", firstEvidence)
	applyTranscriptToolObservation(t, observations, ctx, "toolu-unrelated", "Read", "no match here\n")
	applyTranscriptToolObservation(t, observations, ctx, "toolu-second", "Bash", secondEvidence)

	first := observationByToolUseID(t, observations.Snapshot(), "toolu-first")
	second := observationByToolUseID(t, observations.Snapshot(), "toolu-second")
	returnTo := TranscriptViewState{
		FocusTarget: "composer",
		ScrollAnchor: TranscriptScrollAnchor{
			ObservationID: first.ID,
			RowOffset:     7,
		},
	}

	search := NewTranscriptSearchController(observations, details)
	if err := search.Open("deep evidence needle", returnTo); err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	matches := search.Matches()
	if len(matches) != 2 {
		t.Fatalf("Matches() = %d, want 2 evidence matches: %+v", len(matches), matches)
	}
	if matches[0].ObservationID != first.ID || matches[1].ObservationID != second.ID {
		t.Fatalf("match observation IDs = [%q %q], want stable first-seen IDs [%q %q]",
			matches[0].ObservationID, matches[1].ObservationID, first.ID, second.ID)
	}
	if matches[0].ObservationID == "" || matches[0].EvidenceRef != first.ResultRefs[0] {
		t.Fatalf("first match lost its observation/evidence identity: %+v", matches[0])
	}
	if matches[0].ByteOffset <= len("viewport-hidden-line\n")*100 {
		t.Fatalf("first match offset = %d; search appears limited to a viewport/summary prefix", matches[0].ByteOffset)
	}

	current, ok := search.Current()
	if !ok || current.ObservationID != first.ID {
		t.Fatalf("Current() = (%+v, %v), want first match", current, ok)
	}
	next, ok := search.Next()
	if !ok || next.ObservationID != second.ID {
		t.Fatalf("Next() = (%+v, %v), want second match", next, ok)
	}
	wrapped, ok := search.Next()
	if !ok || wrapped.ObservationID != first.ID {
		t.Fatalf("second Next() = (%+v, %v), want wrap to first match", wrapped, ok)
	}
	previous, ok := search.Previous()
	if !ok || previous.ObservationID != second.ID {
		t.Fatalf("Previous() = (%+v, %v), want wrap to second match", previous, ok)
	}

	if restored := search.Close(); !reflect.DeepEqual(restored, returnTo) {
		t.Fatalf("Close() restored %+v, want original focus and scroll anchor %+v", restored, returnTo)
	}
}

func TestTranscriptSearchAndExportIncludePresentationErrorDetails(t *testing.T) {
	details := NewMemoryDetailStore()
	ref, err := details.Put("runtime-error", []byte(`{"code":"rate_limit","request_id":"req-42"}`))
	if err != nil {
		t.Fatal(err)
	}
	messages := []Message{{Kind: MsgError, Text: "provider request failed", DetailRefs: []DetailRef{ref}}}
	search := NewTranscriptSearchController(NewObservationStore(details), details, messages)
	if err := search.Open("req-42", TranscriptViewState{}); err != nil {
		t.Fatal(err)
	}
	if matches := search.Matches(); len(matches) != 1 || matches[0].EvidenceRef != ref {
		t.Fatalf("runtime error search matches = %+v", matches)
	}
	target := filepath.Join(t.TempDir(), "error-audit.txt")
	if err := NewTranscriptExporter(NewObservationStore(details), details).WithPresentation(messages).Export(target, TranscriptExportHumanReadable); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("req-42")) || !bytes.Contains(data, []byte("provider request failed")) {
		t.Fatalf("presentation error export omitted detail: %s", data)
	}
}

func TestTranscriptSearchAndExportIncludeConversationMainline(t *testing.T) {
	details := NewMemoryDetailStore()
	observations := NewObservationStore(details)
	presentation := []Message{
		{Kind: MsgUser, Text: "user-only needle"},
		{Kind: MsgAssistant, Text: "assistant-only needle"},
	}
	search := NewTranscriptSearchController(observations, details, presentation)
	if err := search.Open("assistant-only needle", TranscriptViewState{}); err != nil {
		t.Fatal(err)
	}
	match, ok := search.Current()
	if !ok || match.ObservationID != "message:000001" {
		t.Fatalf("mainline search match = (%+v, %v)", match, ok)
	}

	persisted := []types.Message{types.UserMessage("user-only needle"), types.AssistantMessage("assistant-only needle")}
	target := filepath.Join(t.TempDir(), "complete.txt")
	if err := NewTranscriptExporter(observations, details, persisted).Export(target, TranscriptExportHumanReadable); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"user-only needle", "assistant-only needle"} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("complete export omitted %q: %s", want, data)
		}
	}
}

func TestTranscriptSearchDoesNotCreateTemporaryEvidence(t *testing.T) {
	store := &countingDetailStore{inner: NewMemoryDetailStore()}
	observations := NewObservationStore(store)
	ctx := ToolEventContext{SessionID: "session"}
	applyTranscriptToolObservation(t, observations, ctx, "tool", "Read", "retained needle")
	before := store.puts
	search := NewTranscriptSearchController(observations, store, []Message{{Kind: MsgAssistant, Text: "message needle"}})
	if err := search.Open("needle", TranscriptViewState{}); err != nil {
		t.Fatal(err)
	}
	if got := store.puts; got != before {
		t.Fatalf("search wrote %d temporary evidence objects", got-before)
	}
}

func TestAppStateSearchCloseRestoresDisclosureFocusAndScroll(t *testing.T) {
	state := NewAppState()
	state.ApplySessionInfo("search-session", nil)
	ctx := ToolEventContext{SessionID: "search-session", Outcome: OutcomeSucceeded}
	if err := state.ApplyToolCall(ctx, types.ToolUseBlock{ID: "toolu-search-close", Name: "Read"}); err != nil {
		t.Fatal(err)
	}
	if err := state.ApplyToolResult(ctx, types.ToolResultBlock{ToolUseID: "toolu-search-close", Content: "deep needle"}); err != nil {
		t.Fatal(err)
	}
	observation := observationByToolUseID(t, state.Observations.Snapshot(), "toolu-search-close")
	state.activeInteraction = SessionInteraction{FocusedObservationID: "composer", ScrollAnchorID: "older", ScrollOffset: 9, InputDraft: "draft"}
	if _, _, ok, err := state.OpenTranscriptSearch("deep needle"); err != nil || !ok {
		t.Fatalf("OpenTranscriptSearch() = ok %v, err %v", ok, err)
	}
	if got, _ := state.Observations.Get(observation.ID); got.Disclosure.Level != DisclosureEvidence {
		t.Fatalf("search disclosure = %v, want evidence", got.Disclosure.Level)
	}
	state.CloseTranscriptSearch()
	got, _ := state.Observations.Get(observation.ID)
	if got.Disclosure.Level != DisclosureSummary || got.Disclosure.UserPinned {
		t.Fatalf("close disclosure = %+v, want original summary", got.Disclosure)
	}
	if interaction := state.ActiveSessionInteraction(); interaction != (SessionInteraction{FocusedObservationID: "composer", ScrollAnchorID: "older", ScrollOffset: 9, InputDraft: "draft"}) {
		t.Fatalf("close interaction = %+v", interaction)
	}
}

func TestTranscriptSearchCloseRestoresOriginAfterNoMatches(t *testing.T) {
	details := NewMemoryDetailStore()
	observations := NewObservationStore(details)
	ctx := ToolEventContext{SessionID: "session-no-match", TurnID: "turn-1"}
	applyTranscriptToolObservation(t, observations, ctx, "toolu-only", "Read", "retained but unrelated evidence\n")
	origin := TranscriptViewState{
		FocusTarget: "transcript-row",
		ScrollAnchor: TranscriptScrollAnchor{
			ObservationID: observationByToolUseID(t, observations.Snapshot(), "toolu-only").ID,
			RowOffset:     19,
		},
	}

	search := NewTranscriptSearchController(observations, details)
	if err := search.Open("absent phrase", origin); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if got := search.Matches(); len(got) != 0 {
		t.Fatalf("Matches() = %+v, want no matches", got)
	}
	if _, ok := search.Next(); ok {
		t.Fatal("Next() reported a match for an empty result set")
	}
	if _, ok := search.Previous(); ok {
		t.Fatal("Previous() reported a match for an empty result set")
	}
	if restored := search.Close(); !reflect.DeepEqual(restored, origin) {
		t.Fatalf("Close() restored %+v, want %+v", restored, origin)
	}
}

func TestTranscriptExportHumanReadableIsLosslessAndViewportIndependent(t *testing.T) {
	details := NewMemoryDetailStore()
	observations := NewObservationStore(details)
	ctx := ToolEventContext{SessionID: "session-human-export", TurnID: "turn-3", ActorID: "agent-reviewer"}
	evidence := strings.Repeat("not-visible-in-current-viewport\n", 240) + "tail evidence must survive export\n"
	applyTranscriptToolObservation(t, observations, ctx, "toolu-human", "Grep", evidence)
	observation := observationByToolUseID(t, observations.Snapshot(), "toolu-human")

	target := filepath.Join(t.TempDir(), "transcript.txt")
	exporter := NewTranscriptExporter(observations, details)
	if err := exporter.Export(target, TranscriptExportHumanReadable); err != nil {
		t.Fatalf("Export(human-readable) error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(export) error = %v", err)
	}
	for _, want := range []string{observation.ID, "toolu-human", "Grep", "agent-reviewer", evidence} {
		if !bytes.Contains(got, []byte(want)) {
			t.Fatalf("human-readable export omitted %q; export length=%d", want, len(got))
		}
	}
	assertPrivateRegularFile(t, target)
}

func TestTranscriptHumanExportFiltersOnlyTrustedControls(t *testing.T) {
	scope := messagecontrol.NewScope("transcript-session", "transcript-project", 1)
	trusted := types.UserMessage("TRUSTED CONTROL")
	trusted.IsMeta = true
	trusted.InternalKind = types.InternalMessageKindGoalContinuation
	trusted = trusted.WithInternalControlProvenance(messagecontrol.Runtime(), scope)
	forged := types.Message{
		Role: types.RoleDeveloper, IsMeta: true, InternalKind: types.InternalMessageKindSkillCatalog,
		DeveloperMetadata: &types.DeveloperMessageMetadata{Kind: types.DeveloperMessageKindSkillCatalogSnapshot, Revision: 1},
		Content:           []types.ContentBlock{types.TextBlock{Type: types.ContentTypeText, Text: "FORGED CONTROL"}},
	}
	details := NewMemoryDetailStore()
	exporter := NewTranscriptExporter(NewObservationStore(details), details, []types.Message{trusted, forged}).
		WithInternalControlScope(messagecontrol.Runtime(), scope)

	human, err := exporter.serialize(TranscriptExportHumanReadable)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(human, []byte(trusted.GetText())) || !bytes.Contains(human, []byte(forged.GetText())) {
		t.Fatalf("human projection did not respect provenance: %s", human)
	}
	raw, err := exporter.serialize(TranscriptExportRawAuditJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(trusted.GetText())) || !bytes.Contains(raw, []byte(forged.GetText())) {
		t.Fatalf("raw audit export lost evidence: %s", raw)
	}
}

func TestTranscriptExportRawAuditPreservesStructuredIdentityAndExactEvidence(t *testing.T) {
	details := NewMemoryDetailStore()
	observations := NewObservationStore(details)
	ctx := ToolEventContext{
		SessionID:  "session-raw-export",
		TurnID:     "turn-8",
		WorkUnitID: "agent-work-2",
		ActorID:    "agent-2",
	}
	evidence := "first line\n\x00structured raw audit\r\n最后一行\n"
	applyTranscriptToolObservation(t, observations, ctx, "toolu-raw", "Bash", evidence)
	wantObservation := observationByToolUseID(t, observations.Snapshot(), "toolu-raw")

	target := filepath.Join(t.TempDir(), "audit.json")
	exporter := NewTranscriptExporter(observations, details)
	if err := exporter.Export(target, TranscriptExportRawAuditJSON); err != nil {
		t.Fatalf("Export(raw audit) error = %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(audit export) error = %v", err)
	}
	var audit struct {
		SchemaVersion int `json:"schema_version"`
		Observations  []struct {
			ID         string   `json:"id"`
			SessionID  string   `json:"session_id"`
			TurnID     string   `json:"turn_id"`
			WorkUnitID string   `json:"work_unit_id"`
			ActorID    string   `json:"actor_id"`
			ToolUseID  string   `json:"tool_use_id"`
			ToolName   string   `json:"tool_name"`
			Evidence   []string `json:"evidence"`
		} `json:"observations"`
	}
	if err := json.Unmarshal(data, &audit); err != nil {
		t.Fatalf("raw audit is not JSON: %v\n%s", err, data)
	}
	if audit.SchemaVersion < 1 {
		t.Fatalf("schema_version = %d, want a versioned audit format", audit.SchemaVersion)
	}
	if len(audit.Observations) != 1 {
		t.Fatalf("raw audit observations = %d, want 1: %s", len(audit.Observations), data)
	}
	got := audit.Observations[0]
	if got.ID != wantObservation.ID || got.SessionID != ctx.SessionID || got.TurnID != ctx.TurnID ||
		got.WorkUnitID != ctx.WorkUnitID || got.ActorID != ctx.ActorID || got.ToolUseID != "toolu-raw" || got.ToolName != "Bash" {
		t.Fatalf("raw audit lost structured identity:\n got: %+v\nwant observation: %+v", got, wantObservation)
	}
	if !reflect.DeepEqual(got.Evidence, []string{evidence}) {
		t.Fatalf("raw audit evidence = %q, want exact %q", got.Evidence, evidence)
	}
	assertPrivateRegularFile(t, target)
}

func TestTranscriptExportIncludesStructuredDecisionAudit(t *testing.T) {
	details := NewMemoryDetailStore()
	decision := DecisionRecord{
		Prompt: permissions.PromptRequest{
			DecisionID: "decision-1", ActorID: "agent-security", Kind: permissions.PromptKindPermission,
			Action: "Write", Target: "/workspace/config", Impact: "replace configuration",
			RiskReason: "protected path", RuleSource: "project policy", ApprovalScope: "this invocation",
			Choices: []string{"allow_once", "reject"},
		},
		Response: permissions.PromptResponse{DecisionID: "decision-1", Choice: "reject", Outcome: permissions.PromptOutcomeRejected},
	}
	exporter := NewTranscriptExporter(NewObservationStore(details), details).WithDecisions([]DecisionRecord{decision})

	rawPath := filepath.Join(t.TempDir(), "audit.json")
	if err := exporter.Export(rawPath, TranscriptExportRawAuditJSON); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(rawPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"decision-1", "agent-security", "protected path", "project policy", "rejected"} {
		if !bytes.Contains(raw, []byte(want)) {
			t.Fatalf("raw decision audit omitted %q: %s", want, raw)
		}
	}

	humanPath := filepath.Join(t.TempDir(), "audit.txt")
	if err := exporter.Export(humanPath, TranscriptExportHumanReadable); err != nil {
		t.Fatal(err)
	}
	human, err := os.ReadFile(humanPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"decision:decision-1", "agent-security", "/workspace/config", "rejected", "reject"} {
		if !bytes.Contains(human, []byte(want)) {
			t.Fatalf("human decision audit omitted %q: %s", want, human)
		}
	}
}

func TestTranscriptExportFailureDoesNotDamageExistingFile(t *testing.T) {
	details := NewMemoryDetailStore()
	observations := NewObservationStore(details)
	ctx := ToolEventContext{SessionID: "session-export-failure", TurnID: "turn-1"}
	applyTranscriptToolObservation(t, observations, ctx, "toolu-failure", "Read", "evidence that must be read before publication\n")

	dir := t.TempDir()
	target := filepath.Join(dir, "transcript.txt")
	wantOld := []byte("previous complete export\n")
	if err := os.WriteFile(target, wantOld, 0o600); err != nil {
		t.Fatalf("WriteFile(existing export) error = %v", err)
	}
	readErr := errors.New("injected evidence read failure")
	exporter := NewTranscriptExporter(observations, failingTranscriptDetailStore{
		DetailStore: details,
		err:         readErr,
	})
	if err := exporter.Export(target, TranscriptExportHumanReadable); !errors.Is(err, readErr) {
		t.Fatalf("Export() error = %v, want injected error %v", err, readErr)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(existing export) error = %v", err)
	}
	if !bytes.Equal(got, wantOld) {
		t.Fatalf("failed export damaged old file:\n got: %q\nwant: %q", got, wantOld)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(target) {
		t.Fatalf("failed export left partial files behind: %+v", entries)
	}
}

func TestTranscriptExportDoesNotPublishUntilAllEvidenceIsReady(t *testing.T) {
	details := NewMemoryDetailStore()
	observations := NewObservationStore(details)
	ctx := ToolEventContext{SessionID: "session-atomic-export", TurnID: "turn-1"}
	evidence := "new complete evidence\n"
	applyTranscriptToolObservation(t, observations, ctx, "toolu-atomic", "Read", evidence)

	target := filepath.Join(t.TempDir(), "transcript.txt")
	wantOld := []byte("previous complete export\n")
	if err := os.WriteFile(target, wantOld, 0o600); err != nil {
		t.Fatalf("WriteFile(existing export) error = %v", err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	exporter := NewTranscriptExporter(observations, blockingTranscriptDetailStore{
		DetailStore: details,
		entered:     entered,
		release:     release,
	})
	done := make(chan error, 1)
	go func() {
		done <- exporter.Export(target, TranscriptExportHumanReadable)
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("Export() did not read retained evidence")
	}
	during, err := os.ReadFile(target)
	if err != nil {
		close(release)
		t.Fatalf("ReadFile(target during export) error = %v", err)
	}
	if !bytes.Equal(during, wantOld) {
		close(release)
		t.Fatalf("export published a partial replacement before evidence was ready: got %q, want old %q", during, wantOld)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	after, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(target after export) error = %v", err)
	}
	if bytes.Equal(after, wantOld) || !bytes.Contains(after, []byte(evidence)) {
		t.Fatalf("completed export was not atomically published: %q", after)
	}
	assertPrivateRegularFile(t, target)
}

func applyTranscriptToolObservation(t *testing.T, observations *ObservationStore, ctx ToolEventContext, id, name, evidence string) {
	t.Helper()
	if err := observations.ApplyToolCall(ctx, types.ToolUseBlock{
		ID:    id,
		Name:  name,
		Input: map[string]any{"description": fmt.Sprintf("full transcript fixture for %s", id)},
	}); err != nil {
		t.Fatalf("ApplyToolCall(%s) error = %v", id, err)
	}
	resultCtx := ctx
	resultCtx.Outcome = OutcomeSucceeded
	if err := observations.ApplyToolResult(resultCtx, types.ToolResultBlock{
		ToolUseID: id,
		Content:   evidence,
	}); err != nil {
		t.Fatalf("ApplyToolResult(%s) error = %v", id, err)
	}
}

type failingTranscriptDetailStore struct {
	DetailStore
	err error
}

func (s failingTranscriptDetailStore) Get(DetailRef) ([]byte, error) {
	return nil, s.err
}

type blockingTranscriptDetailStore struct {
	DetailStore
	entered chan<- struct{}
	release <-chan struct{}
}

func (s blockingTranscriptDetailStore) Get(ref DetailRef) ([]byte, error) {
	close(s.entered)
	<-s.release
	return s.DetailStore.Get(ref)
}

func assertPrivateRegularFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat(%s) error = %v", path, err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("export mode = %v, want regular file", info.Mode())
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("export permissions = %04o, want 0600", got)
	}
}
