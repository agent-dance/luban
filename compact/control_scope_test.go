package compact

import (
	"testing"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/types"
)

func TestCompactProjectionsRequireExactControlScope(t *testing.T) {
	current := messagecontrol.NewScope("session", "/project", 8)
	stale := messagecontrol.NewScope("session", "/project", 7)
	boundary := NewCompactBoundaryMessage(
		CompactBoundaryMetadata{Trigger: "manual"},
		messagecontrol.Runtime(),
	).WithInternalControlProvenance(messagecontrol.Runtime(), stale)
	messages := []types.Message{
		types.UserMessage("before"),
		boundary,
		types.AssistantMessage("after"),
	}
	if got := GetMessagesAfterCompactBoundaryForScope(messages, current, true); len(got) != len(messages) {
		t.Fatalf("stale boundary truncated history: got %d messages, want %d", len(got), len(messages))
	}
	currentBoundary := boundary.WithInternalControlProvenance(messagecontrol.Runtime(), current)
	messages[1] = currentBoundary
	if got := GetMessagesAfterCompactBoundaryForScope(messages, current, true); len(got) != 1 || got[0].GetText() != "after" {
		t.Fatalf("current boundary projection = %#v", got)
	}

	replacement := types.ContentReplacementBlock{
		Type:        types.ContentTypeReplacement,
		Kind:        "tool-result",
		ToolUseID:   "tool-scope",
		Replacement: "stored",
	}.WithInternalReplacementProvenance(messagecontrol.Runtime(), stale)
	replacementMessages := []types.Message{{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: "tool-scope", Content: "raw"},
			replacement,
		},
	}}
	if got := ContentReplacementRecordsForScope(replacementMessages, current, true); len(got) != 0 {
		t.Fatalf("stale replacement records = %#v, want none", got)
	}
	replacementMessages[0].Content[1] = replacement.WithInternalReplacementProvenance(messagecontrol.Runtime(), current)
	if got := ContentReplacementRecordsForScope(replacementMessages, current, true); len(got) != 1 || got[0].Replacement != "stored" {
		t.Fatalf("current replacement records = %#v", got)
	}
}
