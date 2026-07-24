package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
	gtui "github.com/grindlemire/go-tui"
)

// These tests specify transcript-level tool segments. A segment is a render
// projection bounded by assistant prose; it is deliberately independent from
// the same-family semantic aggregation represented by ObservationAggregate.

func segmentTestAssistant(text string) Message {
	return Message{Kind: MsgAssistant, Text: text}
}

func segmentTestTool(id, name, actor, workUnit string, outcome ObservationOutcome) Message {
	return Message{
		Kind:          MsgToolCall,
		Text:          name + " " + id,
		ToolName:      name,
		ToolUseID:     id,
		ObservationID: "observation-" + id,
		ActorID:       actor,
		WorkUnitID:    workUnit,
		Outcome:       outcome,
		IsError:       outcome != OutcomeRunning && outcome != OutcomeSucceeded,
		Disclosure:    DisclosureState{Level: DisclosureSummary, HasMore: true},
	}
}

func requireSegmentItem(t *testing.T, item TranscriptRenderItem) *TranscriptToolSegment {
	t.Helper()
	if item.Message != nil || item.Segment == nil {
		t.Fatalf("render item = %+v, want a tool segment", item)
	}
	return item.Segment
}

func requireToolIDs(t *testing.T, segment *TranscriptToolSegment, want ...string) {
	t.Helper()
	if segment == nil {
		t.Fatal("nil tool segment")
	}
	if len(segment.Messages) != len(want) {
		t.Fatalf("segment contains %d messages, want %d: %+v", len(segment.Messages), len(want), segment)
	}
	for index, id := range want {
		if segment.Messages[index].ToolUseID != id {
			t.Fatalf("segment tool order[%d] = %q, want %q; segment=%+v", index, segment.Messages[index].ToolUseID, id, segment)
		}
	}
}

func TestTranscriptToolSegmentGroupsHeterogeneousCallsBetweenAssistantText(t *testing.T) {
	messages := []Message{
		segmentTestAssistant("I will inspect the implementation."),
		segmentTestTool("read", "Read", "assistant", "foreground", OutcomeSucceeded),
		segmentTestTool("grep", "Grep", "assistant", "foreground", OutcomeSucceeded),
		segmentTestTool("bash", "Bash", "assistant", "foreground", OutcomeSucceeded),
		segmentTestAssistant("The implementation uses transcript boundaries."),
	}

	items := BuildTranscriptToolSegments(messages)
	if len(items) != 3 {
		t.Fatalf("render items = %d, want assistant + segment + assistant: %+v", len(items), items)
	}
	segment := requireSegmentItem(t, items[1])
	requireToolIDs(t, segment, "read", "grep", "bash")
	if segment.Alert || segment.DefaultExpanded {
		t.Fatalf("successful multi-tool segment should default collapsed: %+v", segment)
	}
	if summary := segment.Summary(i18n.LangEN); summary == "" {
		t.Fatal("collapsed segment has no group summary")
	}
}

func TestTranscriptToolSegmentLeavesSingletonAsIndividualTool(t *testing.T) {
	messages := []Message{
		segmentTestAssistant("I will read one file."),
		segmentTestTool("only", "Read", "assistant", "foreground", OutcomeSucceeded),
		segmentTestAssistant("That file contains the relevant state."),
	}

	items := BuildTranscriptToolSegments(messages)
	if len(items) != 3 {
		t.Fatalf("render items = %d, want 3: %+v", len(items), items)
	}
	if items[1].Segment != nil || items[1].Message == nil || items[1].Message.ToolUseID != "only" {
		t.Fatalf("single tool was wrapped in a segment: %+v", items[1])
	}
}

func TestTranscriptToolSegmentTreatsAgentObservationAsBoundary(t *testing.T) {
	for _, agentTool := range []string{"Agent", "Task"} {
		t.Run(agentTool, func(t *testing.T) {
			messages := []Message{
				segmentTestTool("before", "Read", "assistant", "foreground", OutcomeSucceeded),
				segmentTestTool("agent", agentTool, "assistant", "foreground", OutcomeSucceeded),
				segmentTestTool("after", "Read", "assistant", "foreground", OutcomeSucceeded),
			}

			items := BuildTranscriptToolSegments(messages)
			if len(items) != 3 {
				t.Fatalf("Read + %s + Read produced %d render items, want three independent messages: %+v", agentTool, len(items), items)
			}
			for index, want := range []string{"before", "agent", "after"} {
				if items[index].Segment != nil || items[index].Message == nil || items[index].Message.ToolUseID != want {
					t.Fatalf("render item %d = %+v, want independent tool message %q", index, items[index], want)
				}
			}
			if items[1].Message.ToolName != agentTool {
				t.Fatalf("middle boundary lost %s identity: %+v", agentTool, items[1].Message)
			}
		})
	}
}

func TestTranscriptToolSegmentAssistantTextClosesCurrentSegment(t *testing.T) {
	messages := []Message{
		segmentTestAssistant("First investigation."),
		segmentTestTool("first-read", "Read", "assistant", "foreground", OutcomeSucceeded),
		segmentTestTool("first-grep", "Grep", "assistant", "foreground", OutcomeSucceeded),
		segmentTestAssistant("First findings."),
		segmentTestTool("second-read", "Read", "assistant", "foreground", OutcomeSucceeded),
		segmentTestTool("second-bash", "Bash", "assistant", "foreground", OutcomeSucceeded),
		segmentTestAssistant("Second findings."),
	}

	items := BuildTranscriptToolSegments(messages)
	if len(items) != 5 {
		t.Fatalf("render items = %d, want assistant/segment/assistant/segment/assistant: %+v", len(items), items)
	}
	first := requireSegmentItem(t, items[1])
	second := requireSegmentItem(t, items[3])
	requireToolIDs(t, first, "first-read", "first-grep")
	requireToolIDs(t, second, "second-read", "second-bash")
	if first.ID == "" || second.ID == "" || first.ID == second.ID {
		t.Fatalf("assistant boundary did not produce distinct stable segments: first=%+v second=%+v", first, second)
	}
}

func TestTranscriptToolSegmentVisibleUserAndBriefRowsAreBoundaries(t *testing.T) {
	for _, boundary := range []Message{
		{Kind: MsgUser, Text: "continue"},
		{Kind: MsgSendUserMessage, Text: "visible brief"},
	} {
		messages := []Message{
			segmentTestTool("before-read", "Read", "assistant", "foreground", OutcomeSucceeded),
			segmentTestTool("before-grep", "Grep", "assistant", "foreground", OutcomeSucceeded),
			boundary,
			segmentTestTool("after-read", "Read", "assistant", "foreground", OutcomeSucceeded),
			segmentTestTool("after-grep", "Grep", "assistant", "foreground", OutcomeSucceeded),
		}
		items := BuildTranscriptToolSegments(messages)
		if len(items) != 3 {
			t.Fatalf("visible boundary kind %v produced %d items: %+v", boundary.Kind, len(items), items)
		}
		requireToolIDs(t, requireSegmentItem(t, items[0]), "before-read", "before-grep")
		requireToolIDs(t, requireSegmentItem(t, items[2]), "after-read", "after-grep")
	}
}

func TestTranscriptToolSegmentPreservesCallOrderWhenResultsFinishOutOfOrder(t *testing.T) {
	state := NewAppState()
	state.SessionID.Set("segment-session")
	ctx := ToolEventContext{SessionID: "segment-session", TurnID: "turn-1", ActorID: "assistant", WorkUnitID: "foreground"}
	state.AppendOrStreamTextForTurn("Run independent inspections.", 1)
	state.FinalizeStream()
	calls := []types.ToolUseBlock{
		{ID: "one", Name: "Read", Input: map[string]any{"file_path": "/workspace/one.go"}},
		{ID: "two", Name: "Grep", Input: map[string]any{"pattern": "two", "path": "/workspace"}},
		{ID: "three", Name: "Bash", Input: map[string]any{"command": "go test ./..."}},
	}
	for _, call := range calls {
		if err := state.ApplyToolCall(ctx, call); err != nil {
			t.Fatalf("ApplyToolCall(%s) error = %v", call.ID, err)
		}
	}
	beforeResults := BuildTranscriptToolSegments(state.Messages.Get())
	if len(beforeResults) != 2 {
		t.Fatalf("live render items = %d, want assistant + live segment: %+v", len(beforeResults), beforeResults)
	}
	liveSegment := requireSegmentItem(t, beforeResults[1])
	requireToolIDs(t, liveSegment, "one", "two", "three")
	if liveSegment.ID == "" {
		t.Fatal("live segment has no stable identity")
	}
	resultCtx := ctx
	resultCtx.Outcome = OutcomeSucceeded
	for index := len(calls) - 1; index >= 0; index-- {
		if err := state.ApplyToolResult(resultCtx, types.ToolResultBlock{
			ToolUseID: calls[index].ID,
			Content:   "evidence-" + calls[index].ID,
			Outcome:   types.ToolOutcomeSucceeded,
		}); err != nil {
			t.Fatalf("ApplyToolResult(%s) error = %v", calls[index].ID, err)
		}
	}
	state.AppendOrStreamTextForTurn("All inspections completed.", 1)
	state.FinalizeStream()

	items := BuildTranscriptToolSegments(state.Messages.Get())
	if len(items) != 3 {
		t.Fatalf("render items = %d, want assistant + segment + assistant: %+v", len(items), items)
	}
	segment := requireSegmentItem(t, items[1])
	requireToolIDs(t, segment, "one", "two", "three")
	if segment.ID != liveSegment.ID {
		t.Fatalf("result completion changed segment identity: before=%q after=%q", liveSegment.ID, segment.ID)
	}
}

func TestTranscriptToolSegmentFailureIsAlertAndCollapsedAfterSettlement(t *testing.T) {
	failed := segmentTestTool("grep-failed", "Grep", "assistant", "foreground", OutcomeFailed)
	failed.Disclosure.Level = DisclosureDetail
	messages := []Message{
		segmentTestAssistant("Inspect and verify."),
		segmentTestTool("read-ok", "Read", "assistant", "foreground", OutcomeSucceeded),
		failed,
		segmentTestTool("bash-ok", "Bash", "assistant", "foreground", OutcomeSucceeded),
		segmentTestAssistant("One inspection failed."),
	}

	items := BuildTranscriptToolSegments(messages)
	if len(items) != 3 {
		t.Fatalf("render items = %d, want assistant + alerted segment + assistant: %+v", len(items), items)
	}
	segment := requireSegmentItem(t, items[1])
	requireToolIDs(t, segment, "read-ok", "grep-failed", "bash-ok")
	if !segment.Alert || segment.IssueCount != 1 || segment.DefaultExpanded {
		t.Fatalf("settled failure did not remain a collapsed alert: %+v", segment)
	}
	if !segment.Messages[1].IsError || segment.Messages[1].Outcome != OutcomeFailed {
		t.Fatalf("failed member lost its error state: %+v", segment.Messages[1])
	}
}

func TestTranscriptToolSegmentStructuredSuccessStaysInGroupButDefaultsCollapsed(t *testing.T) {
	read := segmentTestTool("read", "Read", "assistant", "foreground", OutcomeSucceeded)
	write := segmentTestTool("write", "Write", "assistant", "foreground", OutcomeSucceeded)
	write.Disclosure.Level = DisclosureDetail
	items := BuildTranscriptToolSegments([]Message{read, write})
	if len(items) != 1 {
		t.Fatalf("structured side-effect receipt split the contiguous group: %+v", items)
	}
	segment := requireSegmentItem(t, items[0])
	requireToolIDs(t, segment, "read", "write")
	if segment.DefaultExpanded {
		t.Fatalf("settled structured segment should default collapsed: %+v", segment)
	}
}

func TestTranscriptToolSegmentExpandedStructuredMembersRenderEditDiff(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	state.SessionID.Set("segment-diff")
	ctx := ToolEventContext{SessionID: "segment-diff", TurnID: "turn-1", ActorID: "assistant", WorkUnitID: "foreground"}

	for index, change := range [][2]string{{"before-one", "after-one"}, {"before-two", "after-two"}} {
		id := fmt.Sprintf("edit-%d", index+1)
		if err := state.ApplyToolCall(ctx, types.ToolUseBlock{
			ID: id, Name: "Edit", Input: map[string]any{"file_path": "/workspace/edit.go"},
		}); err != nil {
			t.Fatal(err)
		}
		resultCtx := ctx
		resultCtx.Outcome = OutcomeSucceeded
		if err := state.ApplyToolResult(resultCtx, types.ToolResultBlock{
			ToolUseID: id,
			Content:   "file updated",
			Outcome:   types.ToolOutcomeSucceeded,
			Data: map[string]any{
				"filePath": "/workspace/edit.go",
				"structuredPatch": []any{map[string]any{
					"oldStart": 1, "oldLines": 1, "newStart": 1, "newLines": 1,
					"lines": []any{"-" + change[0], "+" + change[1]},
				}},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	items := BuildTranscriptToolSegments(state.Messages.Get())
	if len(items) != 1 {
		t.Fatalf("consecutive edits produced %d render items: %+v", len(items), items)
	}
	segment := requireSegmentItem(t, items[0])
	root := NewRootComponent(state, nil, nil)
	collapsed := collectElementText(root.renderMessageArea(30))
	if !strings.Contains(collapsed, "Used 2 tools") || strings.Contains(collapsed, "before-one") {
		t.Fatalf("settled edit segment did not start collapsed: %q", collapsed)
	}

	state.SetToolSegmentExpanded(segment.ID, true)
	expanded := collectElementText(root.renderMessageArea(30))
	for _, want := range []string{"@@ -1,1 +1,1 @@", "-before-one", "+after-one", "-before-two", "+after-two"} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expanded edit segment omitted diff line %q: %q", want, expanded)
		}
	}
}

func TestTranscriptToolSegmentDoesNotCrossActorOrWorkUnit(t *testing.T) {
	messages := []Message{
		segmentTestAssistant("Run isolated work streams."),
		segmentTestTool("a-read", "Read", "agent-a", "research", OutcomeSucceeded),
		segmentTestTool("a-grep", "Grep", "agent-a", "research", OutcomeSucceeded),
		segmentTestTool("b-read", "Read", "agent-b", "research", OutcomeSucceeded),
		segmentTestTool("b-grep", "Grep", "agent-b", "research", OutcomeSucceeded),
		segmentTestTool("verify-read", "Read", "agent-a", "verify", OutcomeSucceeded),
		segmentTestTool("verify-grep", "Grep", "agent-a", "verify", OutcomeSucceeded),
		segmentTestAssistant("Isolated work completed."),
	}

	items := BuildTranscriptToolSegments(messages)
	if len(items) != 5 {
		t.Fatalf("render items = %d, want assistant + three isolated segments + assistant: %+v", len(items), items)
	}
	wants := [][]string{{"a-read", "a-grep"}, {"b-read", "b-grep"}, {"verify-read", "verify-grep"}}
	seen := make(map[string]struct{}, len(wants))
	for index, want := range wants {
		segment := requireSegmentItem(t, items[index+1])
		requireToolIDs(t, segment, want...)
		if segment.ActorID != segment.Messages[0].ActorID || segment.WorkUnitID != segment.Messages[0].WorkUnitID {
			t.Fatalf("segment identity does not match its members: %+v", segment)
		}
		if _, duplicate := seen[segment.ID]; duplicate || segment.ID == "" {
			t.Fatalf("actor/work-unit scopes shared or lacked segment identity: %+v", segment)
		}
		seen[segment.ID] = struct{}{}
	}
}

func TestTranscriptToolSegmentDoesNotCrossSession(t *testing.T) {
	messages := make([]Message, 0, 4)
	for _, sessionID := range []string{"session-a", "session-b"} {
		for _, suffix := range []string{"read", "grep"} {
			message := segmentTestTool(sessionID+"-"+suffix, "Read", "assistant", "foreground", OutcomeSucceeded)
			message.SessionID = sessionID
			messages = append(messages, message)
		}
	}
	items := BuildTranscriptToolSegments(messages)
	if len(items) != 2 {
		t.Fatalf("session boundary produced %d items, want two groups: %+v", len(items), items)
	}
	for index, sessionID := range []string{"session-a", "session-b"} {
		segment := requireSegmentItem(t, items[index])
		for _, message := range segment.Messages {
			if message.SessionID != sessionID {
				t.Fatalf("segment %d crossed session boundary: %+v", index, segment)
			}
		}
	}
}

func TestTranscriptToolSegmentRetainsD0HiddenMembersForExpansionAndAudit(t *testing.T) {
	messages := []Message{segmentTestAssistant("Inspect files.")}
	for index := 1; index <= 5; index++ {
		id := fmt.Sprintf("read-%d", index)
		message := segmentTestTool(id, "Read", "assistant", "foreground", OutcomeSucceeded)
		message.AggregateID = "d0-read-group"
		message.PresentationHidden = index < 5
		message.AggregateSummary = "5 read operations"
		message.DetailRefs = []DetailRef{{Source: "memory", Key: id + "-evidence", Size: 12}}
		messages = append(messages, message)
	}
	messages = append(messages, segmentTestAssistant("Inspection complete."))

	items := BuildTranscriptToolSegments(messages)
	if len(items) != 3 {
		t.Fatalf("render items = %d, want assistant + segment + assistant: %+v", len(items), items)
	}
	segment := requireSegmentItem(t, items[1])
	requireToolIDs(t, segment, "read-1", "read-2", "read-3", "read-4", "read-5")
	for index, message := range segment.Messages {
		if len(message.DetailRefs) != 1 {
			t.Fatalf("expanded/audited member %d lost evidence refs: %+v", index, message)
		}
	}
	for index := 0; index < 4; index++ {
		if !segment.Messages[index].PresentationHidden {
			t.Fatalf("builder rewrote D0 hidden member %d instead of retaining it for group expansion", index)
		}
	}
}

func TestTranscriptToolSegmentExpansionOverrideIsGroupScopedAndReversible(t *testing.T) {
	state := NewAppState()
	items := BuildTranscriptToolSegments([]Message{
		segmentTestAssistant("Inspect files."),
		segmentTestTool("read", "Read", "assistant", "foreground", OutcomeSucceeded),
		segmentTestTool("grep", "Grep", "assistant", "foreground", OutcomeSucceeded),
		segmentTestAssistant("Inspection complete."),
	})
	segment := requireSegmentItem(t, items[1])
	if segment.DefaultExpanded {
		t.Fatalf("successful closed segment should default collapsed: %+v", segment)
	}
	if state.ToolSegmentExpanded(segment.ID) {
		t.Fatal("new segment unexpectedly has an explicit expansion override")
	}

	state.SetToolSegmentExpanded(segment.ID, true)
	if !state.ToolSegmentExpanded(segment.ID) {
		t.Fatal("group-level expand interaction was not retained")
	}
	state.SetToolSegmentExpanded(segment.ID, false)
	if state.ToolSegmentExpanded(segment.ID) {
		t.Fatal("group-level collapse interaction was not retained")
	}

	for _, message := range segment.Messages {
		if message.ObservationID == "" {
			t.Fatalf("group toggle discarded member audit identity: %+v", message)
		}
	}
}

func TestTranscriptToolSegmentRunningStaysExpandedUntilStructuredSettlement(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	first := segmentTestTool("ongoing-read-1", "Read", "assistant", "foreground", OutcomeRunning)
	first.Text = "ONGOING_MEMBER_1"
	second := segmentTestTool("ongoing-read-2", "Read", "assistant", "foreground", OutcomeRunning)
	second.Text = "ONGOING_MEMBER_2"
	messages := []Message{
		segmentTestAssistant("Inspect files."),
		first,
		second,
		{Kind: MsgInfo, Text: "internal bookkeeping", PresentationHidden: true},
	}
	state.Messages.Set(messages)
	root := NewRootComponent(state, nil, nil)

	ongoing := collectElementText(root.renderMessageArea(20))
	for _, want := range []string{"Read files", "ONGOING_MEMBER_1", "ONGOING_MEMBER_2"} {
		if !strings.Contains(ongoing, want) {
			t.Fatalf("ongoing segment did not stay expanded with %q: %q", want, ongoing)
		}
	}

	settledMessages := append([]Message(nil), messages...)
	settledMessages[1].Outcome = OutcomeSucceeded
	settledMessages[2].Outcome = OutcomeFailed
	settledMessages[2].IsError = true
	state.Messages.Set(settledMessages)
	closed := collectElementText(root.renderMessageArea(20))
	if !strings.Contains(closed, "Read files") || !strings.Contains(closed, "1 issue") {
		t.Fatalf("settled alert did not retain its collapsed group header: %q", closed)
	}
	if strings.Contains(closed, "ONGOING_MEMBER_") {
		t.Fatalf("settled segment leaked member details before expansion: %q", closed)
	}
}

func TestTranscriptToolSegmentCrossesInvisibleToolRoundTurnIDs(t *testing.T) {
	first := segmentTestTool("read", "Read", "assistant", "foreground", OutcomeSucceeded)
	first.SessionID, first.TurnID = "session", "turn-1"
	second := segmentTestTool("grep", "Grep", "assistant", "foreground", OutcomeSucceeded)
	second.SessionID, second.TurnID = "session", "turn-2"
	items := BuildTranscriptToolSegments([]Message{
		segmentTestAssistant("Inspect the implementation."), first, second,
		segmentTestAssistant("Here are the findings."),
	})
	if len(items) != 3 {
		t.Fatalf("invisible tool rounds split the visible segment: %+v", items)
	}
	requireToolIDs(t, requireSegmentItem(t, items[1]), "read", "grep")
}

func TestTranscriptToolSegmentCrossesSyntheticForegroundTurnWorkUnits(t *testing.T) {
	const (
		sessionID = "session-real"
		queryID   = "019f-query"
	)
	tool := func(id, name string, turn int) Message {
		message := segmentTestTool(id, name, "assistant", fmt.Sprintf("%s:query-%s:turn-%d", sessionID, queryID, turn), OutcomeSucceeded)
		message.SessionID = sessionID
		// makeTUIEventHandler currently reconstructs this shorter TurnID because
		// ToolUse/ToolResult events carry WorkUnitID but omit TurnID.
		message.TurnID = fmt.Sprintf("%s:turn-%d", sessionID, turn)
		return message
	}
	messages := []Message{
		segmentTestAssistant("I will research the market."),
		tool("search-a", "WebSearch", 1),
		tool("search-b", "WebSearch", 1),
		tool("fetch-a", "WebFetch", 2),
		tool("fetch-b", "WebFetch", 2),
		segmentTestAssistant("Here are the findings."),
	}

	items := BuildTranscriptToolSegments(messages)
	if len(items) != 3 {
		t.Fatalf("synthetic foreground work units split one visible tool segment into %d items: %+v", len(items), items)
	}
	requireToolIDs(t, requireSegmentItem(t, items[1]), "search-a", "search-b", "fetch-a", "fetch-b")
}

func TestTranscriptToolSegmentSyntheticForegroundScopeDoesNotMergeQueriesOrExplicitWork(t *testing.T) {
	tool := func(id, workUnit string) Message {
		message := segmentTestTool(id, "Read", "assistant", workUnit, OutcomeSucceeded)
		message.SessionID = "session"
		return message
	}
	messages := []Message{
		tool("query-a-1", "session:query-a:turn-1"),
		tool("query-a-2", "session:query-a:turn-2"),
		tool("query-b-1", "session:query-b:turn-1"),
		tool("query-b-2", "session:query-b:turn-2"),
		tool("review-1", "review"),
		tool("review-2", "review"),
		tool("verify-1", "verify"),
		tool("verify-2", "verify"),
	}

	items := BuildTranscriptToolSegments(messages)
	if len(items) != 4 {
		t.Fatalf("query or explicit work-unit boundaries were merged: %+v", items)
	}
	wants := [][]string{
		{"query-a-1", "query-a-2"},
		{"query-b-1", "query-b-2"},
		{"review-1", "review-2"},
		{"verify-1", "verify-2"},
	}
	for index, want := range wants {
		requireToolIDs(t, requireSegmentItem(t, items[index]), want...)
	}
}

func TestTranscriptToolSegmentIgnoresOnlyEmptyOrExplicitlyHiddenInternalRows(t *testing.T) {
	tool := func(id string, turn int) Message {
		message := segmentTestTool(id, "Read", "assistant", fmt.Sprintf("session:query-one:turn-%d", turn), OutcomeSucceeded)
		message.SessionID = "session"
		return message
	}
	messages := []Message{
		segmentTestAssistant("Inspect."),
		tool("first", 1),
		{Kind: MsgAssistantThinking, Text: " \n\t"},
		{Kind: MsgInfo, Text: "internal bookkeeping", PresentationHidden: true},
		tool("second", 2),
		segmentTestAssistant("Done."),
	}

	items := BuildTranscriptToolSegments(messages)
	if len(items) != 3 {
		t.Fatalf("transparent internal rows split or leaked into the render projection: %+v", items)
	}
	requireToolIDs(t, requireSegmentItem(t, items[1]), "first", "second")

	visible := append([]Message(nil), messages...)
	visible[2] = Message{Kind: MsgInfo, Text: "Retrying provider request"}
	items = BuildTranscriptToolSegments(visible)
	if len(items) != 5 || items[2].Message == nil || items[2].Message.Kind != MsgInfo {
		t.Fatalf("visible internal status row must remain a segment boundary: %+v", items)
	}
}

func TestTranscriptToolSegmentRenderCollapsesAndExpandsAllMembers(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	messages := []Message{segmentTestAssistant("Inspect files.")}
	for index := 1; index <= 3; index++ {
		message := segmentTestTool(fmt.Sprintf("read-%d", index), "Read", "assistant", "foreground", OutcomeSucceeded)
		message.Text = fmt.Sprintf("MEMBER_%d", index)
		message.PresentationHidden = index < 3 // emulate the older D0 aggregate
		messages = append(messages, message)
	}
	messages = append(messages, segmentTestAssistant("Done."))
	state.Messages.Set(messages)
	root := NewRootComponent(state, nil, nil)

	collapsed := collectElementText(root.renderMessageArea(20))
	if !strings.Contains(collapsed, "Read files") {
		t.Fatalf("collapsed transcript lacks one segment header: %q", collapsed)
	}
	if strings.Contains(collapsed, "MEMBER_") {
		t.Fatalf("collapsed segment leaked individual calls: %q", collapsed)
	}

	segment := requireSegmentItem(t, BuildTranscriptToolSegments(messages)[1])
	state.SetToolSegmentExpanded(segment.ID, true)
	expanded := collectElementText(root.renderMessageArea(20))
	for index := 1; index <= 3; index++ {
		if !strings.Contains(expanded, fmt.Sprintf("MEMBER_%d", index)) {
			t.Fatalf("expanded segment lost D0-hidden member %d: %q", index, expanded)
		}
	}
}

func TestTranscriptToolSegmentHeaderUsesActiveLanguageAtRenderTime(t *testing.T) {
	state := NewAppState()
	state.Messages.Set([]Message{
		segmentTestTool("read", "Read", "assistant", "foreground", OutcomeSucceeded),
		segmentTestTool("read-2", "Read", "assistant", "foreground", OutcomeSucceeded),
	})
	root := NewRootComponent(state, nil, nil)
	state.Language.Set(i18n.LangEN)
	if text := collectElementText(root.renderMessageArea(10)); !strings.Contains(text, "Read files") {
		t.Fatalf("English segment header = %q", text)
	}
	state.Language.Set(i18n.LangZH)
	if text := collectElementText(root.renderMessageArea(10)); !strings.Contains(text, "已读取文件") {
		t.Fatalf("Chinese segment header did not update at render time: %q", text)
	}
}

func TestTranscriptToolSegmentHeaderClickExpandsGroup(t *testing.T) {
	state := NewAppState()
	messages := []Message{
		segmentTestTool("read", "Read", "assistant", "foreground", OutcomeSucceeded),
		segmentTestTool("grep", "Grep", "assistant", "foreground", OutcomeSucceeded),
		segmentTestAssistant("Inspection complete."),
	}
	state.Messages.Set(messages)
	root := NewRootComponent(state, nil, nil)
	area := root.renderMessageArea(10)
	area.Render(gtui.NewBuffer(80, 10), 80, 10)
	segment := requireSegmentItem(t, BuildTranscriptToolSegments(messages)[0])
	header := root.segmentRefs.Get(segment.ID)
	if header == nil || header.Rect().IsEmpty() {
		t.Fatalf("segment header ref was not laid out: %#v", header)
	}
	rect := header.Rect()
	if !root.HandleMouse(gtui.MouseEvent{Button: gtui.MouseLeft, Action: gtui.MousePress, X: rect.X, Y: rect.Y}) {
		t.Fatal("segment header click was not handled")
	}
	if !state.ToolSegmentExpanded(segment.ID) {
		t.Fatal("segment header click did not expand the group")
	}
}

func TestTranscriptToolSegmentKeyboardTogglesLatestGroup(t *testing.T) {
	state := NewAppState()
	messages := []Message{
		segmentTestTool("read", "Read", "assistant", "foreground", OutcomeSucceeded),
		segmentTestTool("grep", "Grep", "assistant", "foreground", OutcomeSucceeded),
		segmentTestAssistant("Inspection complete."),
	}
	state.Messages.Set(messages)
	root := NewRootComponent(state, nil, nil)
	segment := requireSegmentItem(t, BuildTranscriptToolSegments(messages)[0])
	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: 'g', Mod: gtui.ModAlt})
	if !state.ToolSegmentExpanded(segment.ID) {
		t.Fatal("keyboard path did not expand the latest tool segment")
	}
	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: 'g', Mod: gtui.ModAlt})
	if state.ToolSegmentExpanded(segment.ID) {
		t.Fatal("keyboard path did not collapse the latest tool segment")
	}
}

func TestBoundedTranscriptRenderItemsKeepsWholeLongSegment(t *testing.T) {
	state := NewAppState()
	messages := []Message{segmentTestAssistant("before")}
	for index := 0; index < 90; index++ {
		messages = append(messages, segmentTestTool(fmt.Sprintf("tool-%d", index), "Read", "assistant", "foreground", OutcomeSucceeded))
	}
	messages = append(messages, segmentTestAssistant("after"))
	state.Messages.Set(messages)
	root := NewRootComponent(state, nil, nil)
	items := root.boundedTranscriptRenderItems(3)
	if len(items) != 2 {
		t.Fatalf("bounded items = %d, want full segment + trailing text: %+v", len(items), items)
	}
	segment := requireSegmentItem(t, items[0])
	if len(segment.Messages) != 90 {
		t.Fatalf("viewport truncated segment to %d members, want 90", len(segment.Messages))
	}
}

func TestExpandTranscriptMessageRangeCrossesTransparentInternalRows(t *testing.T) {
	first := segmentTestTool("first", "Read", "assistant", "session:query-one:turn-1", OutcomeSucceeded)
	first.SessionID = "session"
	second := segmentTestTool("second", "Read", "assistant", "session:query-one:turn-2", OutcomeSucceeded)
	second.SessionID = "session"
	messages := []Message{
		segmentTestAssistant("before"),
		first,
		{Kind: MsgInfo, Text: "internal", PresentationHidden: true},
		second,
		segmentTestAssistant("after"),
	}

	messageRange := expandTranscriptMessageRange(messages, 3, 4)
	if messageRange.Start != 1 || messageRange.End != 4 {
		t.Fatalf("range around second tool = %+v, want the complete visible segment [1,4)", messageRange)
	}
}

func TestBoundedTranscriptRenderItemsDoesNotProjectWholeOrdinaryHistory(t *testing.T) {
	state := NewAppState()
	messages := make([]Message, 0, 1200)
	for group := 0; group < 200; group++ {
		messages = append(messages, segmentTestAssistant(fmt.Sprintf("phase-%d", group)))
		for member := 0; member < 5; member++ {
			messages = append(messages, segmentTestTool(fmt.Sprintf("%d-%d", group, member), "Read", "assistant", "foreground", OutcomeSucceeded))
		}
	}
	state.Messages.Set(messages)
	items := NewRootComponent(state, nil, nil).boundedTranscriptRenderItems(3)
	if len(items) >= 50 {
		t.Fatalf("bounded projection rebuilt too much history: %d render items", len(items))
	}
	for _, item := range items {
		if item.Segment != nil && len(item.Segment.Messages) != 5 {
			t.Fatalf("bounded projection truncated a group: %+v", item.Segment)
		}
	}
}

func TestTranscriptToolSegmentRealEventSequenceRendersOneMixedContainer(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.AppendOrStreamTextForTurn("I will inspect and verify.", 1)
	state.FinalizeStream()
	calls := []struct {
		ctx  ToolEventContext
		call types.ToolUseBlock
	}{
		{ToolEventContext{SessionID: "session", TurnID: "turn-1", ActorID: "assistant", WorkUnitID: "foreground"}, types.ToolUseBlock{ID: "read-a", Name: "Read", Input: map[string]any{"file_path": "/workspace/a.go"}}},
		{ToolEventContext{SessionID: "session", TurnID: "turn-1", ActorID: "assistant", WorkUnitID: "foreground"}, types.ToolUseBlock{ID: "read-b", Name: "Read", Input: map[string]any{"file_path": "/workspace/b.go"}}},
		{ToolEventContext{SessionID: "session", TurnID: "turn-2", ActorID: "assistant", WorkUnitID: "foreground"}, types.ToolUseBlock{ID: "verify", Name: "Bash", Input: map[string]any{"command": "go test ./tui"}}},
	}
	for _, item := range calls {
		if err := state.ApplyToolCall(item.ctx, item.call); err != nil {
			t.Fatal(err)
		}
	}
	for index := len(calls) - 1; index >= 0; index-- {
		resultCtx := calls[index].ctx
		resultCtx.Outcome = OutcomeSucceeded
		if err := state.ApplyToolResult(resultCtx, types.ToolResultBlock{
			ToolUseID: calls[index].call.ID, Content: "ok", Outcome: types.ToolOutcomeSucceeded,
		}); err != nil {
			t.Fatal(err)
		}
	}
	state.AppendOrStreamTextForTurn("The verification passed.", 2)
	state.FinalizeStream()

	root := NewRootComponent(state, nil, nil)
	collapsed := collectElementText(root.renderMessageArea(30))
	if strings.Count(collapsed, "Used 3 tools") != 1 {
		t.Fatalf("mixed tool sequence did not render as one header: %q", collapsed)
	}
	for _, hidden := range []string{"/workspace/a.go", "/workspace/b.go", "go test ./tui"} {
		if strings.Contains(collapsed, hidden) {
			t.Fatalf("collapsed mixed group leaked %q: %q", hidden, collapsed)
		}
	}
	for _, width := range []int{40, 80, 120} {
		root.termWidth = width
		rendered := renderElementText(root.renderMessageArea(20), width, 20)
		if !strings.Contains(rendered, "Used 3 tools") {
			t.Fatalf("%d-column viewport lost the grouped header:\n%s", width, rendered)
		}
		if strings.Contains(rendered, "/workspace/a.go") || strings.Contains(rendered, "go test ./tui") {
			t.Fatalf("%d-column viewport expanded a settled group by accident:\n%s", width, rendered)
		}
	}

	items := BuildTranscriptToolSegments(state.Messages.Get())
	segment := requireSegmentItem(t, items[1])
	state.SetToolSegmentExpanded(segment.ID, true)
	expanded := collectElementText(root.renderMessageArea(30))
	for _, visible := range []string{"workspace/a.go", "workspace/b.go", "go test ./tui"} {
		if !strings.Contains(expanded, visible) {
			t.Fatalf("expanded mixed group lost %q: %q", visible, expanded)
		}
	}
	if strings.Contains(expanded, "details available") {
		t.Fatalf("expanded group rendered quiet success details instead of one compact row per call: %q", expanded)
	}
}
