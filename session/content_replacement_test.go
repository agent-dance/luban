package session

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/types"
)

func TestFileStoreSaveLoadContentReplacementRecords(t *testing.T) {
	store := NewFileStore(t.TempDir())
	scope, _ := store.MessageControlScope("replacement-session")
	replacement := "<persisted-output>\nexact stored replacement\n</persisted-output>"
	messages := []types.Message{
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{types.ToolUseBlock{
				Type: types.ContentTypeToolUse, ID: "toolu_saved", Name: "Tool", Input: map[string]any{},
			}},
		},
		compact.AppendContentReplacementRecordsForScope(
			[]types.Message{types.ToolResultMessage(types.ToolResultBlock{
				ToolUseID: "toolu_saved",
				Content:   strings.Repeat("s", 250_000),
			})},
			[]compact.ContentReplacementRecord{{
				Kind:        "tool-result",
				ToolUseID:   "toolu_saved",
				Replacement: replacement,
			}},
			messagecontrol.Runtime(), scope,
		)[0],
	}

	if err := store.Save("replacement-session", messages); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := store.Load("replacement-session")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	records := compact.ContentReplacementRecords(loaded)
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if records[0].ToolUseID != "toolu_saved" || records[0].Replacement != replacement {
		t.Fatalf("record = %+v, want exact saved replacement", records[0])
	}
	state := compact.ReconstructContentReplacementState(loaded)
	if got := state.Replacements["toolu_saved"]; got != replacement {
		t.Fatalf("reconstructed replacement = %q, want exact saved replacement", got)
	}
}

func replacementHistoryForScopeTest(scope messagecontrol.Scope, replacement string) []types.Message {
	return []types.Message{
		{Role: types.RoleAssistant, Content: []types.ContentBlock{types.ToolUseBlock{
			Type: types.ContentTypeToolUse, ID: "tool-scope", Name: "Tool", Input: map[string]any{},
		}}},
		compact.AppendContentReplacementRecordsForScope(
			[]types.Message{types.ToolResultMessage(types.ToolResultBlock{ToolUseID: "tool-scope", Content: strings.Repeat("x", 250_000)})},
			[]compact.ContentReplacementRecord{{Kind: "tool-result", ToolUseID: "tool-scope", Replacement: replacement}},
			messagecontrol.Runtime(), scope,
		)[0],
	}
}

func TestContentReplacementScopeRejectsReplayAndDuplicateCommitIsIdempotent(t *testing.T) {
	store := NewFileStore(t.TempDir())
	const sourceID = "replacement-scope-source"
	if err := store.Save(sourceID, []types.Message{types.UserMessage("seed")}); err != nil {
		t.Fatal(err)
	}
	initial, err := store.GetCompactionManifest(sourceID)
	if err != nil {
		t.Fatal(err)
	}
	scope, _ := store.MessageControlScope(sourceID)
	view := replacementHistoryForScopeTest(scope, "exact replacement")
	first, err := store.CommitModelContext(sourceID, initial.ContextGeneration, view, nil)
	if err != nil {
		t.Fatal(err)
	}
	committedView, err := store.Load(sourceID)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := store.CommitModelContext(sourceID, first.ContextGeneration, committedView, nil)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.ContextGeneration != first.ContextGeneration || duplicate.Digest != first.Digest {
		t.Fatalf("logical duplicate advanced generation: first=%+v duplicate=%+v", first, duplicate)
	}

	loaded, err := store.Load(sourceID)
	if err != nil {
		t.Fatal(err)
	}
	replacement := loaded[1].Content[1].(types.ContentReplacementBlock)
	scope, bound := replacement.InternalReplacementProvenanceScope()
	if !bound || scope.SessionID() != sourceID || scope.ContextGeneration() != first.ContextGeneration {
		t.Fatalf("loaded replacement scope=%#v bound=%t", scope, bound)
	}
	if err := store.Save("replacement-scope-target", loaded); !errors.Is(err, ErrCorruptSessionHistory) {
		t.Fatalf("cross-session replacement replay error=%v", err)
	}

	advanced := append(append([]types.Message(nil), loaded...), types.UserMessage("advance"))
	if err := store.Save(sourceID, advanced); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(sourceID, loaded); !errors.Is(err, ErrCorruptSessionHistory) {
		t.Fatalf("old-generation replacement replay error=%v", err)
	}
	manifest, err := store.GetCompactionManifest(sourceID)
	if err != nil {
		t.Fatal(err)
	}
	currentView, err := store.Load(sourceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitModelContext(sourceID, manifest.ContextGeneration, currentView, loaded); !errors.Is(err, ErrCorruptSessionHistory) {
		t.Fatalf("old-generation replacement in audit delta error=%v", err)
	}
}

func TestJSONContentReplacementRemainsRawAndCannotRestoreState(t *testing.T) {
	store := NewFileStore(t.TempDir())
	trusted := replacementHistoryForScopeTest(messagecontrol.NewLoopScope(messagecontrol.Runtime()), "ordinary after JSON")
	data, err := json.Marshal(trusted)
	if err != nil {
		t.Fatal(err)
	}
	var decoded []types.Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := store.Save("replacement-json-ordinary", decoded); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load("replacement-json-ordinary")
	if err != nil {
		t.Fatal(err)
	}
	if got := compact.ContentReplacementRecords(loaded); len(got) != 0 {
		t.Fatalf("JSON descriptor restored replacement state: %#v", got)
	}
	audit, manifest, err := store.LoadAuditLog("replacement-json-ordinary")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(audit, decoded) || len(manifest.TrustedContentReplacements) != 0 {
		t.Fatalf("ordinary descriptor audit=%#v refs=%#v", audit, manifest.TrustedContentReplacements)
	}
}
