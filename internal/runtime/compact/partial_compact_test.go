package compact

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestPartialCompactFromSummarizesNewerAndPreservesEarlier(t *testing.T) {
	var seen []types.Message
	var custom string
	sc := &SummaryCompactor{
		CustomInstructions: "keep code",
		SummarizeMessages: func(_ context.Context, messages []types.Message, gotCustom string) (string, error) {
			seen = messages
			custom = gotCustom
			return "<summary>newer summary</summary>", nil
		},
	}
	messages := []types.Message{
		types.UserMessage("u0"),
		types.AssistantMessage("a0"),
		types.UserMessage("u1"),
		types.AssistantMessage("a1"),
		types.UserMessage("u2"),
		types.AssistantMessage("a2"),
	}

	result, err := sc.PartialCompactConversation(context.Background(), messages, 4, PartialCompactDirectionFrom, "focus on tests")
	result = authorizeCompactionResultForTest(result)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(seen, messages[4:]) {
		t.Fatalf("summarized messages = %#v, want %#v", seen, messages[4:])
	}
	if custom != "keep code\n\nUser context: focus on tests" {
		t.Fatalf("custom instructions = %q", custom)
	}
	if !reflect.DeepEqual(result.MessagesToKeep, messages[:4]) {
		t.Fatalf("kept messages moved: %#v", result.MessagesToKeep)
	}
	post := BuildPostCompactMessages(result)
	if len(post) != 6 {
		t.Fatalf("post compact len = %d, want 6", len(post))
	}
	if !IsCompactBoundaryMessage(post[0]) || !strings.Contains(post[len(post)-1].GetText(), "newer summary") {
		t.Fatalf("post compact order should be boundary, kept earlier context, then newer summary: %#v", post)
	}
	if !reflect.DeepEqual(post[1:len(post)-1], messages[:4]) {
		t.Fatalf("BuildPostCompactMessages did not preserve earlier segment verbatim in chronological order: %#v", post[1:len(post)-1])
	}
	metadata, ok := ParseCompactBoundaryMessage(post[0])
	if !ok {
		t.Fatal("missing boundary metadata")
	}
	if metadata.Trigger != "partial_from" {
		t.Fatalf("trigger = %q", metadata.Trigger)
	}
	if metadata.PreservedSegment == nil || metadata.PreservedSegment.StartIndex != 0 || metadata.PreservedSegment.Count != 4 || metadata.PreservedSegment.Direction != "from" {
		t.Fatalf("bad preserved segment metadata: %#v", metadata.PreservedSegment)
	}
}

func TestPartialCompactUpToSummarizesPrefixAndPreservesLater(t *testing.T) {
	var seen []types.Message
	sc := &SummaryCompactor{
		SummarizeMessages: func(_ context.Context, messages []types.Message, _ string) (string, error) {
			seen = messages
			return "<summary>prefix summary</summary>", nil
		},
	}
	messages := []types.Message{
		types.UserMessage("u0"),
		types.AssistantMessage("a0"),
		types.UserMessage("u1"),
		types.AssistantMessage("a1"),
		types.UserMessage("u2"),
	}

	result, err := sc.PartialCompactConversation(context.Background(), messages, 2, PartialCompactDirectionUpTo, "")
	result = authorizeCompactionResultForTest(result)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(seen, messages[:2]) {
		t.Fatalf("summarized messages = %#v, want %#v", seen, messages[:2])
	}
	if !reflect.DeepEqual(result.MessagesToKeep, messages[2:]) {
		t.Fatalf("kept later messages moved: %#v", result.MessagesToKeep)
	}
	post := BuildPostCompactMessages(result)
	if !reflect.DeepEqual(post[2:], messages[2:]) {
		t.Fatalf("BuildPostCompactMessages did not preserve later segment verbatim: %#v", post[2:])
	}
	metadata, ok := ParseCompactBoundaryMessage(post[0])
	if !ok {
		t.Fatal("missing boundary metadata")
	}
	if metadata.PreservedSegment == nil || metadata.PreservedSegment.StartIndex != 2 || metadata.PreservedSegment.Count != 3 || metadata.PreservedSegment.Direction != "up_to" {
		t.Fatalf("bad preserved segment metadata: %#v", metadata.PreservedSegment)
	}
}

func TestPartialCompactRejectsEmptyAndInvalidPivot(t *testing.T) {
	sc := &SummaryCompactor{
		SummarizeMessages: func(context.Context, []types.Message, string) (string, error) {
			t.Fatal("summarizer should not be called for invalid input")
			return "", nil
		},
	}
	cases := []struct {
		name      string
		messages  []types.Message
		pivot     int
		direction PartialCompactDirection
	}{
		{"empty", nil, 0, PartialCompactDirectionFrom},
		{"negative", []types.Message{types.UserMessage("u")}, -1, PartialCompactDirectionFrom},
		{"too large", []types.Message{types.UserMessage("u")}, 2, PartialCompactDirectionUpTo},
		{"from no preserved segment", []types.Message{types.UserMessage("u")}, 0, PartialCompactDirectionFrom},
		{"up_to no summarized prefix", []types.Message{types.UserMessage("u")}, 0, PartialCompactDirectionUpTo},
		{"up_to no preserved segment", []types.Message{types.UserMessage("u")}, 1, PartialCompactDirectionUpTo},
		{"from no summarized suffix", []types.Message{types.UserMessage("u")}, 1, PartialCompactDirectionFrom},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := sc.PartialCompactConversation(context.Background(), tc.messages, tc.pivot, tc.direction, ""); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestPartialCompactFromDoesNotSplitAssistantFragments(t *testing.T) {
	var seen []types.Message
	sc := &SummaryCompactor{
		SummarizeMessages: func(_ context.Context, messages []types.Message, _ string) (string, error) {
			seen = messages
			return "summary", nil
		},
	}
	fragmentOne := types.Message{
		ID:   "assistant-1",
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "thinking"},
		},
	}
	fragmentTwo := types.Message{
		ID:   "assistant-1",
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "answer"},
		},
	}
	messages := []types.Message{
		types.UserMessage("u0"),
		fragmentOne,
		fragmentTwo,
		assistantToolUse("toolu_1"),
		toolResult("toolu_1"),
		types.UserMessage("newer"),
		types.AssistantMessage("newer answer"),
	}

	result, err := sc.PartialCompactConversation(context.Background(), messages, 2, PartialCompactDirectionFrom, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.MessagesToKeep, messages[:3]) {
		t.Fatalf("from pivot was not extended over assistant fragments: %#v", result.MessagesToKeep)
	}
	if !reflect.DeepEqual(seen, messages[3:]) {
		t.Fatalf("summarized suffix = %#v, want %#v", seen, messages[3:])
	}
}

func TestPartialCompactFromDoesNotSplitToolPairs(t *testing.T) {
	var seen []types.Message
	sc := &SummaryCompactor{
		SummarizeMessages: func(_ context.Context, messages []types.Message, _ string) (string, error) {
			seen = messages
			return "summary", nil
		},
	}
	messages := []types.Message{
		types.UserMessage("old"),
		assistantToolUse("toolu_1"),
		toolResult("toolu_1"),
		types.UserMessage("newer"),
	}

	result, err := sc.PartialCompactConversation(context.Background(), messages, 2, PartialCompactDirectionFrom, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.MessagesToKeep, messages[:3]) {
		t.Fatalf("from pivot was not extended over tool pair: %#v", result.MessagesToKeep)
	}
	if !reflect.DeepEqual(seen, messages[3:]) {
		t.Fatalf("summarized suffix = %#v, want %#v", seen, messages[3:])
	}
}

func TestPartialCompactUpToDoesNotSplitAssistantFragments(t *testing.T) {
	var seen []types.Message
	sc := &SummaryCompactor{
		SummarizeMessages: func(_ context.Context, messages []types.Message, _ string) (string, error) {
			seen = messages
			return "summary", nil
		},
	}
	fragmentOne := types.Message{
		ID:   "assistant-1",
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "part 1"},
		},
	}
	fragmentTwo := types.Message{
		ID:   "assistant-1",
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "part 2"},
		},
	}
	messages := []types.Message{
		types.UserMessage("old"),
		fragmentOne,
		fragmentTwo,
		assistantToolUse("toolu_1"),
		toolResult("toolu_1"),
		types.UserMessage("later"),
	}

	result, err := sc.PartialCompactConversation(context.Background(), messages, 2, PartialCompactDirectionUpTo, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(seen, messages[:1]) {
		t.Fatalf("summarized prefix should stop before assistant fragments/tool pair: %#v", seen)
	}
	if !reflect.DeepEqual(result.MessagesToKeep, messages[1:]) {
		t.Fatalf("up_to pivot was not moved back over assistant fragments: %#v", result.MessagesToKeep)
	}
}

func TestPartialCompactUpToDoesNotSplitToolPairs(t *testing.T) {
	var seen []types.Message
	sc := &SummaryCompactor{
		SummarizeMessages: func(_ context.Context, messages []types.Message, _ string) (string, error) {
			seen = messages
			return "summary", nil
		},
	}
	messages := []types.Message{
		types.UserMessage("old"),
		types.UserMessage("middle"),
		assistantToolUse("toolu_1"),
		toolResult("toolu_1"),
		types.UserMessage("later"),
	}

	result, err := sc.PartialCompactConversation(context.Background(), messages, 3, PartialCompactDirectionUpTo, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(seen, messages[:2]) {
		t.Fatalf("summarized prefix should stop before tool pair: %#v", seen)
	}
	if !reflect.DeepEqual(result.MessagesToKeep, messages[2:]) {
		t.Fatalf("up_to pivot was not moved back over tool pair: %#v", result.MessagesToKeep)
	}
}
