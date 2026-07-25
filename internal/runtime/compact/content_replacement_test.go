package compact

import (
	"errors"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

type replacementStoreFunc func(toolUseID, content string) (string, error)

func (f replacementStoreFunc) PersistReplacement(toolUseID, content string) (string, error) {
	return f(toolUseID, content)
}

func assistantWithTools(ids ...string) types.Message {
	content := make([]types.ContentBlock, len(ids))
	for i, id := range ids {
		content[i] = types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: id, Name: "Tool", Input: map[string]any{}}
	}
	return types.Message{Role: types.RoleAssistant, Content: content}
}

func toolResultMessage(id, content string) types.Message {
	return types.ToolResultMessage(types.ToolResultBlock{ToolUseID: id, Content: content})
}

func TestContentReplacementBudgetOverBudgetFreshResults(t *testing.T) {
	state := NewContentReplacementState()
	calls := 0
	store := replacementStoreFunc(func(toolUseID, content string) (string, error) {
		calls++
		return "<persisted-output>\n" + toolUseID + "\n</persisted-output>", nil
	})
	messages := []types.Message{
		assistantWithTools("a", "b"),
		types.ToolResultMessage(
			types.ToolResultBlock{ToolUseID: "a", Content: strings.Repeat("a", 120_000)},
			types.ToolResultBlock{ToolUseID: "b", Content: strings.Repeat("b", 120_000)},
		),
	}

	got, records, errs := ApplyToolResultBudget(messages, state, store, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if calls != 1 || len(records) != 1 {
		t.Fatalf("calls=%d records=%d, want one replacement", calls, len(records))
	}
	replacedID := records[0].ToolUseID
	if replacedID != "a" && replacedID != "b" {
		t.Fatalf("unexpected replacement id %q", replacedID)
	}
	if !strings.Contains(extractToolResultContent(got, replacedID), "<persisted-output>") {
		t.Fatalf("selected result was not replaced")
	}
	if _, ok := state.SeenIDs["a"]; !ok {
		t.Fatalf("a not marked seen")
	}
	if _, ok := state.SeenIDs["b"]; !ok {
		t.Fatalf("b not marked seen")
	}
}

func TestContentReplacementBudgetFrozenUnreplacedDoesNotChangeLater(t *testing.T) {
	state := NewContentReplacementState()
	messages := []types.Message{
		assistantWithTools("old"),
		toolResultMessage("old", strings.Repeat("o", 120_000)),
	}
	store := replacementStoreFunc(func(toolUseID, content string) (string, error) {
		return "replacement", nil
	})
	if _, records, _ := ApplyToolResultBudget(messages, state, store, nil); len(records) != 0 {
		t.Fatalf("under-budget first pass should not replace")
	}

	messages = append(messages,
		assistantWithTools("new"),
		toolResultMessage("new", strings.Repeat("n", 120_000)),
	)
	got, records, errs := ApplyToolResultBudget(messages, state, store, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(records) != 0 {
		t.Fatalf("separate API user-message groups should not combine across assistant boundary")
	}
	if content := extractToolResultContent(got, "old"); strings.Contains(content, "replacement") {
		t.Fatalf("frozen unreplaced result changed: %q", content)
	}
}

func TestContentReplacementBudgetReappliesRecordedReplacement(t *testing.T) {
	state := NewContentReplacementState()
	state.SeenIDs["a"] = struct{}{}
	state.Replacements["a"] = "exact replacement"
	messages := []types.Message{assistantWithTools("a"), toolResultMessage("a", "original")}

	got, records, errs := ApplyToolResultBudget(messages, state, nil, nil)
	if len(errs) != 0 || len(records) != 0 {
		t.Fatalf("errs=%v records=%v, want pure reapply", errs, records)
	}
	if content := extractToolResultContent(got, "a"); content != "exact replacement" {
		t.Fatalf("content = %q, want exact replacement", content)
	}
}

func TestContentReplacementBudgetPersistenceFailureFreezesOriginal(t *testing.T) {
	state := NewContentReplacementState()
	store := replacementStoreFunc(func(toolUseID, content string) (string, error) {
		return "", errors.New("disk full")
	})
	messages := []types.Message{
		assistantWithTools("a", "b"),
		types.ToolResultMessage(
			types.ToolResultBlock{ToolUseID: "a", Content: strings.Repeat("a", 120_000)},
			types.ToolResultBlock{ToolUseID: "b", Content: strings.Repeat("b", 120_000)},
		),
	}

	got, records, errs := ApplyToolResultBudget(messages, state, store, nil)
	if len(errs) != 1 || len(records) != 0 {
		t.Fatalf("errs=%d records=%d, want one failure and no record", len(errs), len(records))
	}
	if extractToolResultContent(got, "a") == "" || extractToolResultContent(got, "b") == "" {
		t.Fatalf("original content should remain visible")
	}

	_, records, errs = ApplyToolResultBudget(messages, state, store, nil)
	if len(errs) != 0 || len(records) != 0 {
		t.Fatalf("second pass should be frozen with no retry, errs=%v records=%v", errs, records)
	}
}

func TestContentReplacementBudgetSkipsImageAndOptOut(t *testing.T) {
	state := NewContentReplacementState()
	calls := 0
	store := replacementStoreFunc(func(toolUseID, content string) (string, error) {
		calls++
		return "replacement", nil
	})
	messages := []types.Message{
		assistantWithTools("image", "optout"),
		types.ToolResultMessage(
			types.ToolResultBlock{ToolUseID: "image", ContentBlocks: []types.ContentBlock{
				types.ImageBlock{Type: types.ContentTypeImage},
			}},
			types.ToolResultBlock{ToolUseID: "optout", Content: strings.Repeat("x", 250_000), Metadata: map[string]string{"maxResultSizeChars": "inf"}},
		),
	}

	_, records, errs := ApplyToolResultBudget(messages, state, store, nil)
	if len(errs) != 0 || len(records) != 0 || calls != 0 {
		t.Fatalf("errs=%v records=%v calls=%d, want all skipped", errs, records, calls)
	}
	if _, ok := state.SeenIDs["optout"]; !ok {
		t.Fatalf("opt-out text result should be frozen as seen")
	}
	if _, ok := state.SeenIDs["image"]; ok {
		t.Fatalf("image result should not be tracked as text candidate")
	}
}

func TestContentReplacementBudgetAdjacentUserMessagesGroupTogether(t *testing.T) {
	state := NewContentReplacementState()
	store := replacementStoreFunc(func(toolUseID, content string) (string, error) {
		return "replacement-" + toolUseID, nil
	})
	messages := []types.Message{
		assistantWithTools("a", "b"),
		toolResultMessage("a", strings.Repeat("a", 120_000)),
		toolResultMessage("b", strings.Repeat("b", 120_000)),
	}

	_, records, errs := ApplyToolResultBudget(messages, state, store, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(records) != 1 {
		t.Fatalf("adjacent user tool results should be one wire group; records=%d", len(records))
	}
}

func extractToolResultContent(messages []types.Message, id string) string {
	for _, msg := range messages {
		for _, block := range msg.Content {
			if tr, ok := block.(types.ToolResultBlock); ok && tr.ToolUseID == id {
				return tr.TextContent()
			}
		}
	}
	return ""
}
