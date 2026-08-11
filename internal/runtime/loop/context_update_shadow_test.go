package loop

import (
	"testing"

	contextcontract "github.com/agent-dance/luban/internal/contracts/contextupdate"
	streamevent "github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/types"
)

type contextUpdateFixture struct{ decision contextcontract.Decision }

func (fixture contextUpdateFixture) ContextUpdateDecision() contextcontract.Decision {
	return fixture.decision
}

func TestContextUpdateShadowValidatesTargetWithoutApplying(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleAssistant, Content: []types.ContentBlock{types.ToolUseBlock{ID: "inspect-1", Name: "Inspect"}}},
		types.ToolResultMessage(types.ToolResultBlock{ToolUseID: "inspect-1", Content: "source", Outcome: types.ToolOutcomeSucceeded}),
	}
	uses := []types.ToolUseBlock{{ID: "update", Name: "ContextUpdate"}}
	results := []types.ToolResultBlock{{ToolUseID: "update", Outcome: types.ToolOutcomeSucceeded, Data: contextUpdateFixture{decision: contextcontract.Decision{
		Schema: contextcontract.SchemaVersion, TargetIndex: 0, TargetTool: "Inspect", Action: contextcontract.ActionKeep, Confidence: .9,
	}}}}
	var events []streamevent.Event
	(&QueryLoop{}).emitContextUpdateShadow(messages, uses, results, 3, func(event streamevent.Event) { events = append(events, event) })
	if len(events) != 1 || events[0].Progress == nil || events[0].Progress.Stage != "context_update_shadow" ||
		events[0].Progress.Metadata["target_found"] != true || events[0].Progress.Metadata["runtime_candidate"] != true || events[0].Progress.Metadata["applied"] != false {
		t.Fatalf("shadow events = %#v", events)
	}
	if events[0].Progress.Metadata["rewrite"] != nil {
		t.Fatal("shadow telemetry exposed a rewrite field")
	}
}

func TestContextUpdateShadowFailsClosedForInvalidAndUnsafeTargets(t *testing.T) {
	tests := []struct {
		name      string
		decision  contextcontract.Decision
		messages  []types.Message
		wantFound bool
	}{
		{name: "missing index", decision: contextcontract.Decision{TargetIndex: 1, TargetTool: "Inspect", Action: contextcontract.ActionDrop}, messages: []types.Message{
			{Role: types.RoleAssistant, Content: []types.ContentBlock{types.ToolUseBlock{ID: "inspect-1", Name: "Inspect"}}},
			types.ToolResultMessage(types.ToolResultBlock{ToolUseID: "inspect-1", Content: "source", Outcome: types.ToolOutcomeSucceeded}),
		}},
		{name: "tool mismatch", decision: contextcontract.Decision{TargetIndex: 0, TargetTool: "Run", Action: contextcontract.ActionKeep}, messages: []types.Message{
			{Role: types.RoleAssistant, Content: []types.ContentBlock{types.ToolUseBlock{ID: "inspect-1", Name: "Inspect"}}},
			types.ToolResultMessage(types.ToolResultBlock{ToolUseID: "inspect-1", Content: "source", Outcome: types.ToolOutcomeSucceeded}),
		}},
		{name: "failed", decision: contextcontract.Decision{TargetIndex: 0, TargetTool: "Run", Action: contextcontract.ActionDrop}, wantFound: true, messages: []types.Message{
			{Role: types.RoleAssistant, Content: []types.ContentBlock{types.ToolUseBlock{ID: "run-1", Name: "Run"}}},
			types.ToolResultMessage(types.ToolResultBlock{ToolUseID: "run-1", Content: "failure", IsError: true, Outcome: types.ToolOutcomeFailed}),
		}},
		{name: "media", decision: contextcontract.Decision{TargetIndex: 0, TargetTool: "Inspect", Action: contextcontract.ActionRewrite}, wantFound: true, messages: []types.Message{
			{Role: types.RoleAssistant, Content: []types.ContentBlock{types.ToolUseBlock{ID: "inspect-1", Name: "Inspect"}}},
			types.ToolResultMessage(types.ToolResultBlock{ToolUseID: "inspect-1", Outcome: types.ToolOutcomeSucceeded, ContentBlocks: []types.ContentBlock{types.ImageBlock{Type: types.ContentTypeImage}}}),
		}},
		{name: "self assessment", decision: contextcontract.Decision{TargetIndex: 0, TargetTool: "ContextUpdate", Action: contextcontract.ActionKeep}, messages: []types.Message{
			{Role: types.RoleAssistant, Content: []types.ContentBlock{types.ToolUseBlock{ID: "context-update-1", Name: "ContextUpdate"}}},
			types.ToolResultMessage(types.ToolResultBlock{ToolUseID: "context-update-1", Outcome: types.ToolOutcomeSucceeded, Content: "receipt"}),
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			uses := []types.ToolUseBlock{{ID: "update", Name: "ContextUpdate"}}
			results := []types.ToolResultBlock{{ToolUseID: "update", Outcome: types.ToolOutcomeSucceeded, Data: contextUpdateFixture{decision: test.decision}}}
			var events []streamevent.Event
			(&QueryLoop{}).emitContextUpdateShadow(test.messages, uses, results, 1, func(event streamevent.Event) { events = append(events, event) })
			if len(events) != 1 || events[0].Progress.Metadata["target_found"] != test.wantFound || events[0].Progress.Metadata["runtime_candidate"] != false || events[0].Progress.Metadata["applied"] != false {
				t.Fatalf("events = %#v", events)
			}
		})
	}
}

func TestContextUpdateShadowResolvesParallelPreviousBatchByCompleteIndex(t *testing.T) {
	messages := []types.Message{
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.ToolUseBlock{ID: "old-update", Name: "ContextUpdate"},
			types.ToolUseBlock{ID: "inspect-1", Name: "Inspect"},
			types.ToolUseBlock{ID: "run-1", Name: "Run"},
		}},
		types.ToolResultMessage(
			types.ToolResultBlock{ToolUseID: "old-update", Content: "receipt", Outcome: types.ToolOutcomeSucceeded},
			types.ToolResultBlock{ToolUseID: "inspect-1", Content: "source", Outcome: types.ToolOutcomeSucceeded},
			types.ToolResultBlock{ToolUseID: "run-1", Content: "verification", Outcome: types.ToolOutcomeSucceeded},
		),
	}
	uses := []types.ToolUseBlock{{ID: "update", Name: "ContextUpdate"}}
	results := []types.ToolResultBlock{{ToolUseID: "update", Outcome: types.ToolOutcomeSucceeded, Data: contextUpdateFixture{decision: contextcontract.Decision{
		Schema: contextcontract.SchemaVersion, TargetIndex: 2, TargetTool: "Run", Action: contextcontract.ActionKeep, Confidence: 1,
	}}}}
	var events []streamevent.Event
	(&QueryLoop{}).emitContextUpdateShadow(messages, uses, results, 2, func(event streamevent.Event) { events = append(events, event) })
	if len(events) != 1 || events[0].Progress.Metadata["target_found"] != true || events[0].Progress.Metadata["target_tool"] != "Run" ||
		events[0].Progress.Metadata["target_index"] != 2 || events[0].Progress.Metadata["runtime_candidate"] != true {
		t.Fatalf("parallel selector events = %#v", events)
	}
}
