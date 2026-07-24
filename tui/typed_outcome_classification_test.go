package tui

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func TestLegacyErrorCopyDoesNotDriveOutcomeClassification(t *testing.T) {
	for _, lang := range i18n.AllLanguages() {
		lines, ok := renderStructuredToolResultLinesInLanguage(lang, Message{
			Kind: MsgToolResult, ToolName: "Bash", Text: "permission denied / 权限被拒绝 / Zugriff verweigert", IsError: true,
		})
		if !ok || len(lines) == 0 {
			t.Fatalf("%s: missing generic legacy failure projection", lang.Code())
		}
		if !strings.Contains(lines[0], i18n.TUIOutcomeLabel(lang, "unknown")) {
			t.Fatalf("%s: legacy prose changed typed classification: %#v", lang.Code(), lines)
		}
	}
}

func TestMissingToolOutcomeDoesNotTerminalizeRunningActivity(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("session-outcome")
	state.SessionEpoch.Set(1)
	state.Activities = NewActivityStore(ActivityScope{SessionID: "session-outcome", Epoch: 1})
	ctx := ToolEventContext{SessionID: "session-outcome", TurnID: "turn-1", WorkUnitID: "work-1", ActorID: "actor-1"}
	if err := state.ApplyToolCall(ctx, types.ToolUseBlock{ID: "tool-unknown", Name: "Read"}); err != nil {
		t.Fatal(err)
	}
	result := types.ToolResultBlock{ToolUseID: "tool-unknown", IsError: true, Content: `{"error":true,"status":"failed"}`}
	outcome := observationOutcomeForResult(result)
	if outcome != OutcomeUnknown {
		t.Fatalf("missing typed outcome was inferred as %s", outcome)
	}
	ctx.Outcome = outcome
	if err := state.ApplyToolResult(ctx, result); err != nil {
		t.Fatal(err)
	}
	activity, ok := state.GetActivity("tool:tool-unknown")
	if !ok || activity.State != ActivityRunning || activity.Outcome != OutcomeRunning {
		t.Fatalf("missing outcome terminalized activity: %#v", activity)
	}
	observation, ok := state.GetObservation(toolObservationID("session-outcome", "tool-unknown"))
	if !ok || observation.Outcome != OutcomeUnknown {
		t.Fatalf("observation outcome = %#v", observation)
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
