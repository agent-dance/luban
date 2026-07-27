package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/runtimeevent"
	"github.com/agent-dance/luban/types"
)

func TestClassicRendererGroupsRoutineSuccessesAtTurnBoundary(t *testing.T) {
	var output bytes.Buffer
	handle := makeEventHandler(ui.NewTermRenderer(&output), false)
	emitTwoRoutineReads(handle)
	got := output.String()
	if !strings.Contains(got, "Tool: Aggregate") || !strings.Contains(got, "Read - 2 operations") {
		t.Fatalf("classic aggregate summary missing:\n%s", got)
	}
	if strings.Count(got, "State: running") != 1 || strings.Count(got, "State: succeeded") != 1 {
		t.Fatalf("classic routine members were emitted independently:\n%s", got)
	}
}

func TestJSONRendererKeepsEveryRoutineToolEventContentFree(t *testing.T) {
	var output bytes.Buffer
	handle := makeEventHandler(ui.NewJSONRenderer(&output), false)
	emitTwoRoutineReads(handle)
	got := output.String()
	if strings.Count(got, `"schema_version":"`+runtimeevent.MachineEventSchemaVersion+`"`) != 4 ||
		strings.Count(got, `"input_ref"`) != 2 || strings.Count(got, `"content_ref"`) != 2 {
		t.Fatalf("machine renderer lost routine members:\n%s", got)
	}
	for _, want := range []string{"read-a", "read-b"} {
		if !strings.Contains(got, want) {
			t.Fatalf("machine renderer lost %q:\n%s", want, got)
		}
	}
	for _, secret := range []string{"retained-read-a", "retained-read-b"} {
		if strings.Contains(got, secret) {
			t.Fatalf("machine renderer persisted raw tool result %q:\n%s", secret, got)
		}
	}
}

func TestJSONRendererForwardsAgenticMetricsAdditively(t *testing.T) {
	var output bytes.Buffer
	handle := makeEventHandler(ui.NewJSONRenderer(&output), false)
	identity := stream.Event{TurnCount: 2, TurnID: "turn-metrics", ProjectRoot: "/private/project"}
	identity.Type = stream.EventRequestEnd
	identity.RequestStatus = &stream.RequestStatusEvent{
		RequestID: "request-metrics", StartedAt: "2026-07-26T00:00:00Z", EndedAt: "2026-07-26T00:00:01Z",
		InputTokens: 10, CacheReadInputTokens: 8, OutputTokens: 2,
	}
	handle(identity)
	identity.Type = stream.EventToolRoundMetrics
	identity.RequestStatus = nil
	identity.ToolRound = &stream.ToolRoundMetricsEvent{
		RoundID: "turn-metrics", LogicalModelVisibleCalls: 2,
		PhysicalChildOperations: 2, Fanout: 2, CriticalPathMilliseconds: 15,
	}
	handle(identity)

	got := output.String()
	if strings.Count(got, `"type":"agentic_metrics"`) != 2 ||
		!strings.Contains(got, `"metric":"provider_request"`) ||
		!strings.Contains(got, `"metric":"tool_round"`) {
		t.Fatalf("agentic metrics missing from stream-json:\n%s", got)
	}
	if strings.Contains(got, "/private/project") {
		t.Fatalf("agentic metrics leaked project path:\n%s", got)
	}
}

func emitTwoRoutineReads(handle func(stream.Event)) {
	for _, id := range []string{"read-a", "read-b"} {
		call := types.ToolUseBlock{ID: id, Name: "Read", Input: map[string]any{"file_path": id + ".go"}}
		handle(stream.Event{Type: stream.EventToolUse, TurnCount: 1, TurnID: "session:query-1:turn-1", ActorID: "agent", WorkUnitID: "research", ToolUse: &call})
		result := types.ToolResultBlock{ToolUseID: id, Content: "retained-" + id, Outcome: types.ToolOutcomeSucceeded}
		handle(stream.Event{Type: stream.EventToolResult, TurnCount: 1, TurnID: "session:query-1:turn-1", ActorID: "agent", WorkUnitID: "research", ToolResult: &result})
	}
	handle(stream.Event{Type: stream.EventTurnEnd, TurnCount: 1, TurnID: "session:query-1:turn-1", ActorID: "agent", WorkUnitID: "research"})
}
