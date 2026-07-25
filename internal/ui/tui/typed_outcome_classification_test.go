package tui

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func TestUntypedErrorCopyDoesNotDriveOutcomeClassification(t *testing.T) {
	for _, lang := range i18n.AllLanguages() {
		lines, ok := renderStructuredToolResultLinesInLanguage(lang, Message{
			Kind: MsgToolResult, ToolName: "Bash", Text: "permission denied / 权限被拒绝 / Zugriff verweigert", IsError: true,
		})
		if !ok || len(lines) == 0 {
			t.Fatalf("%s: missing generic untyped failure projection", lang.Code())
		}
		if !strings.Contains(lines[0], i18n.TUIOutcomeLabel(lang, "unknown")) {
			t.Fatalf("%s: untyped prose changed typed classification: %#v", lang.Code(), lines)
		}
	}
}

func TestTUIRendererRejectsMissingToolOutcomeBeforeProjection(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session-outcome")
	state.SessionEpoch.Set(1)
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session-outcome", Epoch: 1})
	ctx := ToolEventContext{SessionID: "session-outcome", TurnID: "turn-1", WorkUnitID: "work-1", ActorID: "actor-1"}
	if err := state.ApplyToolCall(ctx, types.ToolUseBlock{ID: "tool-unknown", Name: "Read"}); err != nil {
		t.Fatal(err)
	}
	result := types.ToolResultBlock{ToolUseID: "tool-unknown", IsError: true, Content: `{"error":true,"status":"failed"}`}
	renderer := &TuiRenderer{state: state, enqueue: func(fn func()) bool { fn(); return true }}
	renderer.RenderToolResult(presentation.ToolEventContext{
		SessionID: ctx.SessionID, SessionEpoch: 1, TurnID: ctx.TurnID, WorkUnitID: ctx.WorkUnitID, ActorID: ctx.ActorID,
	}, result)
	activity, ok := state.GetActivity("tool:tool-unknown")
	if !ok || activity.State != ActivityRunning || activity.Outcome != OutcomeRunning {
		t.Fatalf("missing outcome terminalized activity: %#v", activity)
	}
	observation, ok := state.GetObservation(toolObservationID("session-outcome", "tool-unknown"))
	if !ok || observation.Outcome != OutcomeRunning || len(observation.ResultRefs) != 0 {
		t.Fatalf("observation outcome = %#v", observation)
	}
	messages := state.Messages.Get()
	if len(messages) < 2 || messages[len(messages)-1].Kind != MsgError {
		t.Fatalf("missing outcome did not publish a semantic failure: %#v", messages)
	}
}

func TestTypedDeniedOutcomeIsIndependentOfDisplayLanguage(t *testing.T) {
	for _, lang := range i18n.AllLanguages() {
		lines, ok := renderStructuredToolResultLinesInLanguage(lang, Message{
			Kind: MsgToolResult, ToolName: "Bash", Text: "arbitrary external copy", IsError: true, Outcome: OutcomeDenied,
		})
		if !ok || len(lines) == 0 || !strings.Contains(lines[0], observationOutcomeLabelInLanguage(lang, OutcomeDenied)) {
			t.Fatalf("%s: typed denied outcome projection = %#v", lang.Code(), lines)
		}
	}
}
