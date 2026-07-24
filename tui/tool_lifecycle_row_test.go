package tui

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func TestUnifiedToolLifecycleRowReplacesRunningCallWithEmptyReceipt(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangZH)
	ctx := ToolEventContext{SessionID: "session", TurnID: "turn", ActorID: "assistant", WorkUnitID: "work"}
	call := types.ToolUseBlock{ID: "mcp-list", Name: "ListMcpResourcesTool", Input: map[string]any{}}
	if err := state.ApplyToolCall(ctx, call); err != nil {
		t.Fatal(err)
	}
	observation, _ := state.GetObservation(toolObservationID(ctx.SessionID, call.ID))
	root := NewRootComponent(state, nil, nil)
	running := collectElementText(root.renderToolObservation(messageFromObservation(observation, MsgToolCall)))
	if !strings.Contains(running, "⚡ 获取 MCP 资源") || strings.Contains(running, call.Name) {
		t.Fatalf("running row is not semantic and unified:\n%s", running)
	}

	ctx.Outcome = OutcomeSucceeded
	if err := state.ApplyToolResult(ctx, types.ToolResultBlock{
		ToolUseID: call.ID,
		Outcome:   types.ToolOutcomeSucceeded,
		Data:      []any{},
	}); err != nil {
		t.Fatal(err)
	}
	observation, _ = state.GetObservation(observation.ID)
	completed := collectElementText(root.renderToolObservation(messageFromObservation(observation, MsgToolCall)))
	if !strings.Contains(completed, "✓ 未找到 MCP 资源") {
		t.Fatalf("empty receipt is not semantic:\n%s", completed)
	}
	for _, unwanted := range []string{call.Name, "已完成", "可查看详情", "▸", "→"} {
		if strings.Contains(completed, unwanted) {
			t.Fatalf("completed row contains %q:\n%s", unwanted, completed)
		}
	}
}

func TestUnifiedToolLifecycleRowUsesOneUsefulDetailChevron(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	ctx := ToolEventContext{SessionID: "session", Outcome: OutcomeSucceeded}
	call := types.ToolUseBlock{ID: "read", Name: "Read", Input: map[string]any{"file_path": "/workspace/main.go"}}
	if err := state.ApplyToolCall(ctx, call); err != nil {
		t.Fatal(err)
	}
	if err := state.ApplyToolResult(ctx, types.ToolResultBlock{
		ToolUseID: call.ID,
		Outcome:   types.ToolOutcomeSucceeded,
		Data: map[string]any{
			"type": "text",
			"file": map[string]any{"filePath": "/workspace/main.go", "numLines": 42},
		},
	}); err != nil {
		t.Fatal(err)
	}
	observation, _ := state.GetObservation(toolObservationID(ctx.SessionID, call.ID))
	rendered := collectElementText(NewRootComponent(state, nil, nil).renderToolObservation(messageFromObservation(observation, MsgToolCall)))
	if !strings.Contains(rendered, "✓ Read · workspace/main.go · 42 lines  ▸") && !strings.Contains(rendered, "✓ Read · workspace/main.go · 42 lines ▸") {
		t.Fatalf("useful detail affordance missing:\n%s", rendered)
	}
	if strings.Count(rendered, "Read") != 1 || strings.Contains(rendered, "Details available") {
		t.Fatalf("tool receipt was rendered more than once:\n%s", rendered)
	}
}

func TestToolPresentationRelocalizesRetainedStructuredResult(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	ctx := ToolEventContext{SessionID: "session", Outcome: OutcomeSucceeded}
	call := types.ToolUseBlock{ID: "grep", Name: "Grep", Input: map[string]any{"pattern": "TODO", "path": "/workspace"}}
	if err := state.ApplyToolCall(ctx, call); err != nil {
		t.Fatal(err)
	}
	if err := state.ApplyToolResult(ctx, types.ToolResultBlock{
		ToolUseID: call.ID,
		Outcome:   types.ToolOutcomeSucceeded,
		Data:      map[string]any{"numMatches": 2, "numFiles": 1},
	}); err != nil {
		t.Fatal(err)
	}
	state.Language.Set(i18n.LangZH)
	if err := state.RelocalizeToolPresentations(i18n.LangZH); err != nil {
		t.Fatal(err)
	}
	observation, _ := state.GetObservation(toolObservationID(ctx.SessionID, call.ID))
	if !strings.Contains(observation.Presentation.Summary, "搜索文本") || !strings.Contains(observation.Presentation.Summary, "2 个匹配") {
		t.Fatalf("relocalized summary = %q", observation.Presentation.Summary)
	}
	if observation.Presentation.Language != i18n.LangZH.Code() {
		t.Fatalf("presentation language = %q", observation.Presentation.Language)
	}
}

func TestTranscriptToolSegmentsGroupRunningAndSettledCalls(t *testing.T) {
	base := Message{Kind: MsgToolCall, SessionID: "session", ActorID: "assistant", WorkUnitID: "work", Disclosure: DisclosureState{Level: DisclosureSummary}}
	runningA, runningB := base, base
	runningA.ObservationID, runningA.ToolUseID, runningA.Outcome = "a", "a", OutcomeRunning
	runningB.ObservationID, runningB.ToolUseID, runningB.Outcome = "b", "b", OutcomeRunning
	if items := BuildTranscriptToolSegments([]Message{runningA, runningB}); len(items) != 1 || items[0].Segment == nil {
		t.Fatalf("running tools did not share a live segment: %+v", items)
	}
	runningA.Outcome, runningB.Outcome = OutcomeSucceeded, OutcomeSucceeded
	if items := BuildTranscriptToolSegments([]Message{runningA, runningB}); len(items) != 1 || items[0].Segment == nil {
		t.Fatalf("settled quiet successes should group: %+v", items)
	}
}

func TestEveryRegisteredToolUsesSemanticActionWithUnknownFallback(t *testing.T) {
	for toolName, family := range staticToolFamilies {
		if family == FamilyInternal {
			continue
		}
		label := semanticToolActionInLanguage(i18n.LangZH, toolName)
		if label == "" || label == toolName {
			t.Errorf("registered tool %q exposes its implementation name", toolName)
		}
	}
	const unknown = "FutureTool"
	if got := semanticToolActionInLanguage(i18n.LangZH, unknown); got != unknown {
		t.Fatalf("unknown tool fallback = %q, want raw stable identifier", got)
	}
	if got := semanticToolActionInLanguage(i18n.LangZH, "mcp__github__search_code"); got == "mcp__github__search_code" {
		t.Fatalf("dynamic MCP tool exposed implementation identifier: %q", got)
	}
}
