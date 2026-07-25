package tui

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func TestObservationStorePersistsFormattedDecisionAtResultBoundary(t *testing.T) {
	store := NewObservationStore(NewMemoryDetailStore())
	ctx := ToolEventContext{SessionID: "session", TurnID: "turn", ActorID: "agent", WorkUnitID: "work"}
	call := types.ToolUseBlock{ID: "read-1", Name: "Read", Input: map[string]any{"file_path": "/workspace/main.go"}}
	if err := store.ApplyToolCall(ctx, call); err != nil {
		t.Fatal(err)
	}
	ctx.Outcome = OutcomeSucceeded
	if err := store.ApplyToolResult(ctx, types.ToolResultBlock{
		ToolUseID: call.ID,
		Data:      map[string]any{"line_count": 42, "byte_count": 2048},
		Outcome:   types.ToolOutcomeSucceeded,
	}); err != nil {
		t.Fatal(err)
	}

	observation := observationByToolUseID(t, store.Snapshot(), call.ID)
	if observation.Presentation.Family != FamilyFileRead || !strings.Contains(observation.Presentation.Summary, "42 lines") {
		t.Fatalf("formatted presentation not retained: %+v", observation.Presentation)
	}
	if observation.Decision.EffectiveLevel != PresentationFolded || !observation.Decision.AggregationEligible {
		t.Fatalf("policy decision not retained: %+v", observation.Decision)
	}
	if observation.Disclosure.Level != DisclosureSummary {
		t.Fatalf("disclosure = %+v, want summary", observation.Disclosure)
	}
}

func TestObservationPolicyPromotesSideEffectAndDomainFailure(t *testing.T) {
	tests := []struct {
		name   string
		tool   string
		input  map[string]any
		result types.ToolResultBlock
		reason PresentationReason
	}{
		{
			name: "write success", tool: "Write", input: map[string]any{"file_path": "/tmp/out"},
			result: types.ToolResultBlock{Data: map[string]any{"created": true, "bytes_written": 3}, Outcome: types.ToolOutcomeSucceeded},
			reason: ReasonSideEffect,
		},
		{
			name: "typed domain failure", tool: "TaskUpdate", input: map[string]any{"task_id": "7"},
			result: types.ToolResultBlock{Data: map[string]any{"success": false, "error": "invalid transition"}, Outcome: types.ToolOutcomeFailed},
			reason: ReasonWarning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewObservationStore(NewMemoryDetailStore())
			ctx := ToolEventContext{SessionID: "session", Outcome: OutcomeSucceeded}
			if err := store.ApplyToolCall(ctx, types.ToolUseBlock{ID: tt.name, Name: tt.tool, Input: tt.input}); err != nil {
				t.Fatal(err)
			}
			tt.result.ToolUseID = tt.name
			if err := store.ApplyToolResult(ctx, tt.result); err != nil {
				t.Fatal(err)
			}
			observation := observationByToolUseID(t, store.Snapshot(), tt.name)
			if observation.Disclosure.Level != DisclosureDetail || observation.Decision.EffectiveLevel < PresentationStructured {
				t.Fatalf("unsafe success was folded: %+v", observation)
			}
			if !containsPresentationReason(observation.Decision.Reasons, tt.reason) {
				t.Fatalf("reasons = %v, want %q", observation.Decision.Reasons, tt.reason)
			}
		})
	}
}

func TestStructuredDisclosureDoesNotDumpFullEvidenceAndEvidenceRedactsInput(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
	ctx := ToolEventContext{SessionID: "session", Outcome: OutcomeFailed}
	call := types.ToolUseBlock{ID: "bash-1", Name: "Bash", Input: map[string]any{
		"command": "false", "authorization": "Bearer must-not-leak",
	}}
	if err := state.ApplyToolCall(ctx, call); err != nil {
		t.Fatal(err)
	}
	raw := "root cause\nauthorization: Bearer result-must-not-leak\n" + strings.Repeat("full evidence line\n", 200)
	if err := state.ApplyToolResult(ctx, types.ToolResultBlock{
		ToolUseID: call.ID, Content: raw, IsError: true, Outcome: types.ToolOutcomeFailed,
		Metadata: map[string]string{"exit_code": "1"},
	}); err != nil {
		t.Fatal(err)
	}

	observation, _ := state.GetObservation(toolObservationID("session", call.ID))
	root := NewRootComponent(state, nil, nil)
	structured := collectElementText(root.renderToolObservation(messageFromObservation(observation, MsgToolCall)))
	if !strings.Contains(structured, "exit code 1") || !strings.Contains(structured, "Cause: root cause") {
		t.Fatalf("structured detail omitted diagnosis:\n%s", structured)
	}
	if strings.Count(structured, "full evidence line") > 60 {
		t.Fatalf("structured detail dumped full evidence (%d occurrences)", strings.Count(structured, "full evidence line"))
	}
	if strings.Contains(structured, "must-not-leak") {
		t.Fatalf("structured detail leaked secret:\n%s", structured)
	}

	if err := state.RevealObservation(observation.ID, DisclosureEvidence); err != nil {
		t.Fatal(err)
	}
	observation, _ = state.GetObservation(observation.ID)
	evidence := collectElementText(root.renderToolObservation(messageFromObservation(observation, MsgToolCall)))
	if !strings.Contains(evidence, "full evidence line") {
		t.Fatalf("evidence view did not retain raw output:\n%s", evidence)
	}
	if strings.Contains(evidence, "Bearer must-not-leak") || strings.Contains(evidence, "result-must-not-leak") || !strings.Contains(evidence, "[REDACTED]") {
		t.Fatalf("evidence display was not redacted:\n%s", evidence)
	}
}

func TestStructuredToolResultDoesNotAppendEvidenceAvailabilityFooter(t *testing.T) {
	for _, lang := range i18n.AllLanguages() {
		t.Run(lang.Code(), func(t *testing.T) {
			state := NewAppState()
			state.Language.Set(lang)
			state.SessionID.Set("session")
			state.SessionEpoch.Set(1)
			state.Activities = NewActivityStore(ActivityScope{SessionID: "session", Epoch: 1})
			ctx := ToolEventContext{SessionID: "session", Outcome: OutcomeFailed}
			call := types.ToolUseBlock{ID: "bash-1", Name: "Bash", Input: map[string]any{"command": "false"}}
			if err := state.ApplyToolCall(ctx, call); err != nil {
				t.Fatal(err)
			}
			if err := state.ApplyToolResult(ctx, types.ToolResultBlock{
				ToolUseID: call.ID,
				Content:   "root cause",
				IsError:   true,
				Outcome:   types.ToolOutcomeFailed,
			}); err != nil {
				t.Fatal(err)
			}

			observation, ok := state.GetObservation(toolObservationID("session", call.ID))
			if !ok {
				t.Fatal("tool observation was not retained")
			}
			if observation.Disclosure.Level != DisclosureDetail {
				t.Fatalf("disclosure = %v, want detail", observation.Disclosure.Level)
			}
			if got := len(observation.Presentation.DetailLines); got != 2 {
				t.Fatalf("formatted %d detail rows, want outcome and cause only", got)
			}
			element := NewRootComponent(state, nil, nil).renderToolObservation(messageFromObservation(observation, MsgToolCall))
			wantChildren := 1 + len(observation.Presentation.DetailLines)
			if got := len(element.Children()); got != wantChildren {
				t.Fatalf("rendered %d tool rows, want %d (call plus detail rows only)", got, wantChildren)
			}
		})
	}
}

func TestPersistedProjectionRebuildsPresentationDeterministically(t *testing.T) {
	persisted := []types.Message{{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			types.ToolUseBlock{ID: "grep-1", Name: "Grep", Input: map[string]any{"pattern": "TODO", "path": "/workspace"}},
			types.ToolResultBlock{ToolUseID: "grep-1", Data: map[string]any{"match_count": 3, "file_count": 2}, Outcome: types.ToolOutcomeSucceeded},
		},
	}}
	projection, err := ProjectPersistedMessages(SessionIdentity{SessionID: "session"}, persisted, NewMemoryDetailStore())
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Observations) != 1 {
		t.Fatalf("observations = %d, want 1", len(projection.Observations))
	}
	observation := projection.Observations[0]
	if !strings.Contains(observation.Presentation.Summary, "3 matches") || observation.Decision.EffectiveLevel != PresentationFolded {
		t.Fatalf("persisted presentation = %+v decision=%+v", observation.Presentation, observation.Decision)
	}
}
