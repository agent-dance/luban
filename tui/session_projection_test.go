package tui

import (
	"reflect"
	"sort"
	"testing"

	"github.com/agent-dance/luban/types"
)

// Target API assumptions locked by this file:
//   - ProjectPersistedMessages is a pure, deterministic projection from a
//     session identity and persisted model messages.
//   - SessionProjection exposes stable observation identity and ToolUseID
//     values (directly or through its projected messages/observations).
//   - SessionSnapshot contains Identity and Projection fields and is applied as
//     one unit. AppState.AdmitEpoch is the projection boundary for live events.
//
// Reflection is limited to inspecting the target data shape. It keeps these
// regression tests focused on externally relevant identity semantics while the
// exact observation representation is being introduced.

func TestProjectPersistedMessagesUsesStableNamespacedLegacyFallback(t *testing.T) {
	persisted := []types.Message{
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				types.TextBlock{Type: types.ContentTypeText, Text: "legacy question"},
			},
		},
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.TextBlock{Type: types.ContentTypeText, Text: "legacy answer"},
			},
		},
	}

	identity := SessionIdentity{Namespace: "project-a", SessionID: "legacy-session", Epoch: 7}
	first, err := ProjectPersistedMessages(identity, persisted, nil)
	if err != nil {
		t.Fatalf("first projection: %v", err)
	}
	second, err := ProjectPersistedMessages(identity, persisted, nil)
	if err != nil {
		t.Fatalf("second projection: %v", err)
	}

	firstIDs := projectionObservationIDs(t, first)
	secondIDs := projectionObservationIDs(t, second)
	if !reflect.DeepEqual(firstIDs, secondIDs) {
		t.Fatalf("legacy fallback IDs are not stable: first=%v second=%v", firstIDs, secondIDs)
	}
	if len(firstIDs) < 2 {
		t.Fatalf("projected %d stable observation IDs, want at least one per persisted message", len(firstIDs))
	}

	other, err := ProjectPersistedMessages(
		SessionIdentity{Namespace: "project-b", SessionID: "legacy-session", Epoch: 1},
		persisted,
		nil,
	)
	if err != nil {
		t.Fatalf("other namespace projection: %v", err)
	}
	otherIDs := projectionObservationIDs(t, other)
	if reflect.DeepEqual(firstIDs, otherIDs) {
		t.Fatalf("legacy fallback IDs must be namespaced by session identity: project-a=%v project-b=%v", firstIDs, otherIDs)
	}
}

func TestApplySessionSnapshotRestoresToolIdentities(t *testing.T) {
	identity := SessionIdentity{Namespace: "project", SessionID: "resume-target", Epoch: 12}
	persisted := []types.Message{
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				types.ThinkingBlock{Type: types.ContentTypeThinking, Thinking: "inspect both", Signature: "sig"},
				types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "tool-a", Name: "Read", Input: map[string]any{"file_path": "a.go"}},
				types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "tool-b", Name: "Read", Input: map[string]any{"file_path": "b.go"}},
			},
		},
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				// Persisted completion order is intentionally different from call order.
				types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: "tool-b", Content: "result-b"},
				types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: "tool-a", Content: "result-a"},
			},
		},
	}

	projection, err := ProjectPersistedMessages(identity, persisted, nil)
	if err != nil {
		t.Fatalf("project persisted messages: %v", err)
	}
	projectedToolIDs := collectNamedStrings(projection, "ToolUseID")
	assertContainsStrings(t, projectedToolIDs, "tool-a", "tool-b")

	snapshot := sessionSnapshotForTest(t, identity, projection)
	state := NewAppState()
	if err := state.ApplySessionSnapshot(snapshot); err != nil {
		t.Fatalf("apply resume snapshot: %v", err)
	}
	if got := state.SessionID.Get(); got != identity.SessionID {
		t.Fatalf("active SessionID = %q, want %q", got, identity.SessionID)
	}

	visibleToolIDs := collectNamedStrings(state.Messages.Get(), "ToolUseID")
	assertContainsStrings(t, visibleToolIDs, "tool-a", "tool-b")
	visibleText := collectNamedStrings(state.Messages.Get(), "Text")
	assertContainsStrings(t, visibleText, "result-a", "result-b")
}

func TestDuplicatePersistedToolUseIDDoesNotGuessResultInput(t *testing.T) {
	identity := SessionIdentity{Namespace: "project", SessionID: "duplicate-tool-id", Epoch: 1}
	persisted := []types.Message{
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "duplicate", Name: "Read", Input: map[string]any{"file_path": "first.go"}},
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "duplicate", Name: "Bash", Input: map[string]any{"command": "second"}},
		}},
		{Role: types.RoleUser, Content: []types.ContentBlock{
			types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: "duplicate", Content: "ambiguous result"},
		}},
	}

	projection, err := ProjectPersistedMessages(identity, persisted, nil)
	if err != nil {
		t.Fatal(err)
	}
	var result *Message
	for i := range projection.Messages {
		if projection.Messages[i].Text == "ambiguous result" {
			result = &projection.Messages[i]
			break
		}
	}
	if result == nil {
		t.Fatal("ambiguous result message missing")
	}
	if result.Outcome != OutcomeConflict || result.ToolName != "" || len(result.Input) != 0 {
		t.Fatalf("ambiguous result guessed a call: %+v", *result)
	}
}

func TestAppStateRejectsStaleSessionEpoch(t *testing.T) {
	identity := SessionIdentity{Namespace: "project", SessionID: "active", Epoch: 22}
	projection, err := ProjectPersistedMessages(identity, []types.Message{types.UserMessage("active")}, nil)
	if err != nil {
		t.Fatalf("project active session: %v", err)
	}

	state := NewAppState()
	if err := state.ApplySessionSnapshot(sessionSnapshotForTest(t, identity, projection)); err != nil {
		t.Fatalf("apply active snapshot: %v", err)
	}
	if state.AdmitEpoch(identity.Epoch - 1) {
		t.Fatal("stale event epoch was admitted into the active projection")
	}
	if !state.AdmitEpoch(identity.Epoch) {
		t.Fatal("active event epoch was rejected")
	}
	if state.AdmitEpoch(identity.Epoch + 1) {
		t.Fatal("future event epoch was admitted before its session snapshot")
	}
}

func sessionSnapshotForTest(t *testing.T, identity SessionIdentity, projection SessionProjection) SessionSnapshot {
	t.Helper()
	var snapshot SessionSnapshot
	value := reflect.ValueOf(&snapshot).Elem()
	setSnapshotField(t, value, "Identity", reflect.ValueOf(identity))
	setSnapshotField(t, value, "Projection", reflect.ValueOf(projection))
	return snapshot
}

func setSnapshotField(t *testing.T, snapshot reflect.Value, name string, value reflect.Value) {
	t.Helper()
	field := snapshot.FieldByName(name)
	if !field.IsValid() {
		t.Fatalf("SessionSnapshot is missing required %s field", name)
	}
	if !field.CanSet() || !value.Type().AssignableTo(field.Type()) {
		t.Fatalf("SessionSnapshot.%s has type %v, want %v", name, field.Type(), value.Type())
	}
	field.Set(value)
}

func projectionObservationIDs(t *testing.T, projection SessionProjection) []string {
	t.Helper()
	ids := collectNamedStrings(projection, "ObservationID")
	if len(ids) == 0 {
		value := reflect.ValueOf(projection)
		for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
			if value.IsNil() {
				break
			}
			value = value.Elem()
		}
		if value.IsValid() && value.Kind() == reflect.Struct {
			observations := value.FieldByName("Observations")
			if observations.IsValid() && observations.CanInterface() {
				ids = collectNamedStrings(observations.Interface(), "ID")
			}
		}
	}
	if len(ids) == 0 {
		t.Fatal("SessionProjection exposes no non-empty observation ID values")
	}
	return uniqueSortedStrings(ids)
}

func collectNamedStrings(value any, fieldName string) []string {
	var values []string
	var visit func(reflect.Value)
	visit = func(v reflect.Value) {
		if !v.IsValid() {
			return
		}
		for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return
			}
			v = v.Elem()
		}
		switch v.Kind() {
		case reflect.Struct:
			typ := v.Type()
			for i := 0; i < v.NumField(); i++ {
				field := v.Field(i)
				if typ.Field(i).Name == fieldName && field.Kind() == reflect.String && field.String() != "" {
					values = append(values, field.String())
				}
				if field.CanInterface() {
					visit(field)
				}
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				visit(v.Index(i))
			}
		case reflect.Map:
			iter := v.MapRange()
			for iter.Next() {
				visit(iter.Key())
				visit(iter.Value())
			}
		}
	}
	visit(reflect.ValueOf(value))
	return values
}

func assertContainsStrings(t *testing.T, got []string, want ...string) {
	t.Helper()
	set := make(map[string]bool, len(got))
	for _, value := range got {
		set[value] = true
	}
	for _, value := range want {
		if !set[value] {
			t.Errorf("projected values %v do not contain %q", got, value)
		}
	}
}

func uniqueSortedStrings(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
