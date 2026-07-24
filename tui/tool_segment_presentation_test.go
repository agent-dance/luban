package tui

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func TestTranscriptToolSegmentFailedWebMembersKeepTheirTargets(t *testing.T) {
	state := NewAppState()
	state.Language.Set(i18n.LangEN)
	state.SessionID.Set("session")
	state.SessionEpoch.Set(1)
	state.AppendOrStreamTextForTurn("I will check both sources.", 1)
	state.FinalizeStream()

	type webCall struct {
		id, url, workUnit, failure string
	}
	calls := []webCall{
		{"fetch-markets", "https://example.com/markets", "session:query-market:turn-1", "markets returned HTTP 500"},
		{"fetch-news", "https://example.com/news", "session:query-market:turn-2", "news returned HTTP 503"},
	}
	for _, call := range calls {
		ctx := ToolEventContext{SessionID: "session", TurnID: call.workUnit, ActorID: "assistant", WorkUnitID: call.workUnit}
		if err := state.ApplyToolCall(ctx, types.ToolUseBlock{ID: call.id, Name: "WebFetch", Input: map[string]any{"url": call.url}}); err != nil {
			t.Fatal(err)
		}
		ctx.Outcome = OutcomeFailed
		if err := state.ApplyToolResult(ctx, types.ToolResultBlock{
			ToolUseID: call.id, Content: call.failure, IsError: true, Outcome: types.ToolOutcomeFailed,
		}); err != nil {
			t.Fatal(err)
		}
	}
	state.AppendOrStreamTextForTurn("I recovered with another source.", 2)
	state.FinalizeStream()
	root := NewRootComponent(state, nil, nil)
	collapsed := collectElementText(root.renderMessageArea(30))
	if !strings.Contains(collapsed, "Used 2 tools") || !strings.Contains(collapsed, "2 issues") {
		t.Fatalf("failed web calls did not retain a collapsed alert header: %q", collapsed)
	}
	for _, call := range calls {
		if strings.Contains(collapsed, call.url) || strings.Contains(collapsed, call.failure) {
			t.Fatalf("collapsed alert leaked member detail for %q: %q", call.id, collapsed)
		}
	}
	items := BuildTranscriptToolSegments(state.Messages.Get())
	segment := requireSegmentItem(t, items[1])
	state.SetToolSegmentExpanded(segment.ID, true)
	rendered := collectElementText(root.renderMessageArea(30))
	if !strings.Contains(rendered, "Used 2 tools") || !strings.Contains(rendered, "2 issues") {
		t.Fatalf("expanded alert lost its group header: %q", rendered)
	}
	previousEnd := -1
	for _, call := range calls {
		if count := strings.Count(rendered, call.url); count < 1 {
			t.Fatalf("member target %q is absent: %q", call.url, rendered)
		}
		position := strings.Index(rendered, call.url)
		failurePosition := strings.Index(rendered, call.failure)
		if position <= previousEnd || failurePosition <= position {
			t.Fatalf("member target and failure detail lost their call-local order: %q", rendered)
		}
		previousEnd = failurePosition
	}
}

func TestToolInputPreviewIncludesWebTarget(t *testing.T) {
	if got := toolInputPreview("WebFetch", map[string]any{"url": "https://example.com/markets"}); got != "https://example.com/markets" {
		t.Fatalf("WebFetch preview = %q", got)
	}
	if got := toolInputPreview("WebSearch", map[string]any{"query": "A-share market"}); got != "A-share market" {
		t.Fatalf("WebSearch preview = %q", got)
	}
	rendered := renderElementText(NewRootComponent(NewAppState(), nil, nil).renderToolCallLine(Message{
		ToolName: "WebFetch", Text: "https://example.com/markets",
	}), 80, 1)
	if !strings.Contains(rendered, "Fetch web page · https://example.com/markets") {
		t.Fatalf("tool member target is not visibly separated from its operation: %q", rendered)
	}
}
