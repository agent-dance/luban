package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/types"
	"github.com/agent-dance/luban/ui"
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

func TestJSONRendererKeepsEveryRoutineToolEventLossless(t *testing.T) {
	var output bytes.Buffer
	handle := makeEventHandler(ui.NewJSONRenderer(&output), false)
	emitTwoRoutineReads(handle)
	got := output.String()
	if strings.Count(got, `"type":"tool_use"`) != 2 || strings.Count(got, `"type":"tool_result"`) != 2 {
		t.Fatalf("machine renderer lost routine members:\n%s", got)
	}
	for _, want := range []string{"retained-read-a", "retained-read-b", "read-a", "read-b"} {
		if !strings.Contains(got, want) {
			t.Fatalf("machine renderer lost %q:\n%s", want, got)
		}
	}
}

func emitTwoRoutineReads(handle func(loop.Event)) {
	for _, id := range []string{"read-a", "read-b"} {
		call := types.ToolUseBlock{ID: id, Name: "Read", Input: map[string]any{"file_path": id + ".go"}}
		handle(loop.Event{Type: loop.EventToolUse, TurnCount: 1, TurnID: "session:query-1:turn-1", ActorID: "agent", WorkUnitID: "research", ToolUse: &call})
		result := types.ToolResultBlock{ToolUseID: id, Content: "retained-" + id, Outcome: types.ToolOutcomeSucceeded}
		handle(loop.Event{Type: loop.EventToolResult, TurnCount: 1, TurnID: "session:query-1:turn-1", ActorID: "agent", WorkUnitID: "research", ToolResult: &result})
	}
	handle(loop.Event{Type: loop.EventTurnEnd, TurnCount: 1, TurnID: "session:query-1:turn-1", ActorID: "agent", WorkUnitID: "research"})
}
