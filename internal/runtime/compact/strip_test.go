package compact

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

// ── StripImagesFromMessages ──────────────────────────────────────────────────

func TestStripImages_ReplacesImageBlock(t *testing.T) {
	msgs := []types.Message{
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				types.TextBlock{Type: types.ContentTypeText, Text: "Look at this:"},
				types.ImageBlock{
					Type:   types.ContentTypeImage,
					Source: &types.ImageSource{Type: "base64", MediaType: "image/png", Data: strings.Repeat("x", 10000)},
				},
			},
		},
	}

	result := StripImagesFromMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if len(result[0].Content) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(result[0].Content))
	}

	// First block should be unchanged text
	tb, ok := result[0].Content[0].(types.TextBlock)
	if !ok {
		t.Fatal("expected first block to be TextBlock")
	}
	if tb.Text != "Look at this:" {
		t.Errorf("expected unchanged text block, got: %s", tb.Text)
	}

	// Second block should be [image] marker
	tb2, ok := result[0].Content[1].(types.TextBlock)
	if !ok {
		t.Fatal("expected second block to be TextBlock (image marker)")
	}
	if tb2.Text != "[image]" {
		t.Errorf("expected '[image]' marker, got: %s", tb2.Text)
	}
}

func TestStripImages_PreservesAssistantMessages(t *testing.T) {
	msgs := []types.Message{
		types.AssistantMessage("I can see the image"),
	}
	result := StripImagesFromMessages(msgs)
	if result[0].GetText() != "I can see the image" {
		t.Error("assistant messages should not be modified")
	}
}

func TestStripImages_NoImagesUnchanged(t *testing.T) {
	msgs := []types.Message{
		types.UserMessage("hello"),
		types.AssistantMessage("hi"),
	}
	result := StripImagesFromMessages(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[0].GetText() != "hello" || result[1].GetText() != "hi" {
		t.Error("messages without images should be unchanged")
	}
}

func TestStripImages_EmptySlice(t *testing.T) {
	result := StripImagesFromMessages(nil)
	if len(result) != 0 {
		t.Errorf("expected empty result for nil input, got %d", len(result))
	}
}

func TestStripImages_ReplacesDocumentBlock(t *testing.T) {
	msgs := []types.Message{
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				types.TextBlock{Type: types.ContentTypeText, Text: "Check this PDF:"},
				types.DocumentBlock{
					Type:   types.ContentTypeDocument,
					Source: &types.DocumentSource{Type: "base64", MediaType: "application/pdf", Data: strings.Repeat("x", 10000)},
				},
			},
		},
	}

	result := StripImagesFromMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if len(result[0].Content) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(result[0].Content))
	}

	// First block should be unchanged text
	tb, ok := result[0].Content[0].(types.TextBlock)
	if !ok {
		t.Fatal("expected first block to be TextBlock")
	}
	if tb.Text != "Check this PDF:" {
		t.Errorf("expected unchanged text block, got: %s", tb.Text)
	}

	// Second block should be [document] marker
	tb2, ok := result[0].Content[1].(types.TextBlock)
	if !ok {
		t.Fatal("expected second block to be TextBlock (document marker)")
	}
	if tb2.Text != "[document]" {
		t.Errorf("expected '[document]' marker, got: %s", tb2.Text)
	}
}

func TestStripImages_MixedImageAndDocument(t *testing.T) {
	msgs := []types.Message{
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				types.ImageBlock{Type: types.ContentTypeImage, Source: &types.ImageSource{Data: "img1"}},
				types.TextBlock{Type: types.ContentTypeText, Text: "between"},
				types.DocumentBlock{Type: types.ContentTypeDocument, Source: &types.DocumentSource{Data: "doc1"}},
			},
		},
	}

	result := StripImagesFromMessages(msgs)
	if len(result[0].Content) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(result[0].Content))
	}

	tb1 := result[0].Content[0].(types.TextBlock)
	if tb1.Text != "[image]" {
		t.Errorf("expected '[image]' marker, got: %s", tb1.Text)
	}
	tb3 := result[0].Content[2].(types.TextBlock)
	if tb3.Text != "[document]" {
		t.Errorf("expected '[document]' marker, got: %s", tb3.Text)
	}
}

func TestStripImages_DocumentDoesNotMutateOriginal(t *testing.T) {
	origDoc := types.DocumentBlock{
		Type:   types.ContentTypeDocument,
		Source: &types.DocumentSource{Data: "original_data"},
	}
	msgs := []types.Message{
		{
			Role:    types.RoleUser,
			Content: []types.ContentBlock{origDoc},
		},
	}

	_ = StripImagesFromMessages(msgs)
	if msgs[0].Content[0].(types.DocumentBlock).Source.Data != "original_data" {
		t.Error("StripImagesFromMessages should not mutate the original messages")
	}
}

func TestStripImages_MultipleImages(t *testing.T) {
	msgs := []types.Message{
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				types.ImageBlock{Type: types.ContentTypeImage, Source: &types.ImageSource{Data: "img1"}},
				types.TextBlock{Type: types.ContentTypeText, Text: "between"},
				types.ImageBlock{Type: types.ContentTypeImage, Source: &types.ImageSource{Data: "img2"}},
			},
		},
	}

	result := StripImagesFromMessages(msgs)
	if len(result[0].Content) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(result[0].Content))
	}

	markers := 0
	for _, block := range result[0].Content {
		if tb, ok := block.(types.TextBlock); ok && tb.Text == "[image]" {
			markers++
		}
	}
	if markers != 2 {
		t.Errorf("expected 2 [image] markers, got %d", markers)
	}
}

func TestStripImages_DoesNotMutateOriginal(t *testing.T) {
	origImg := types.ImageBlock{
		Type:   types.ContentTypeImage,
		Source: &types.ImageSource{Data: "original_data"},
	}
	msgs := []types.Message{
		{
			Role:    types.RoleUser,
			Content: []types.ContentBlock{origImg},
		},
	}

	_ = StripImagesFromMessages(msgs)
	// Original should be untouched
	if msgs[0].Content[0].(types.ImageBlock).Source.Data != "original_data" {
		t.Error("StripImagesFromMessages should not mutate the original messages")
	}
}

func TestStripImages_ReplacesNestedToolResultMedia(t *testing.T) {
	msgs := []types.Message{
		types.ToolResultMessage(types.ToolResultBlock{
			ToolUseID: "tool_1",
			Content:   "summary",
			ContentBlocks: []types.ContentBlock{
				types.TextBlock{Type: types.ContentTypeText, Text: "caption"},
				types.ImageBlock{Type: types.ContentTypeImage, Source: &types.ImageSource{Data: "img"}},
				types.DocumentBlock{Type: types.ContentTypeDocument, Source: &types.DocumentSource{Data: "doc"}},
			},
		}),
	}

	result := StripImagesFromMessages(msgs)
	tr, ok := result[0].Content[0].(types.ToolResultBlock)
	if !ok {
		t.Fatalf("expected ToolResultBlock, got %#v", result[0].Content[0])
	}
	if len(tr.ContentBlocks) != 3 {
		t.Fatalf("expected 3 nested content blocks, got %d", len(tr.ContentBlocks))
	}
	if got := tr.TextContent(); got != "caption\n[image]\n[document]" {
		t.Fatalf("TextContent = %q", got)
	}
}

// ── EnforcePerMessageBudget ──────────────────────────────────────────────────

func TestEnforcePerMessageBudget_UnderBudget(t *testing.T) {
	msgs := []types.Message{
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				types.ToolResultBlock{
					Type:    types.ContentTypeToolResult,
					Content: strings.Repeat("a", 1000),
				},
			},
		},
	}

	result := EnforcePerMessageBudget(msgs)
	tr := result[0].Content[0].(types.ToolResultBlock)
	if len(tr.Content) != 1000 {
		t.Errorf("expected content unchanged when under budget, got len %d", len(tr.Content))
	}
}

func TestEnforcePerMessageBudget_OverBudget(t *testing.T) {
	// Create a message with tool results exceeding the budget
	msgs := []types.Message{
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				types.ToolResultBlock{
					Type:      types.ContentTypeToolResult,
					ToolUseID: "t1",
					Content:   strings.Repeat("a", 150_000),
				},
				types.ToolResultBlock{
					Type:      types.ContentTypeToolResult,
					ToolUseID: "t2",
					Content:   strings.Repeat("b", 100_000),
				},
			},
		},
	}

	result := EnforcePerMessageBudget(msgs)
	// The largest block (150K) should be truncated
	tr1 := result[0].Content[0].(types.ToolResultBlock)
	if len(tr1.Content) >= 150_000 {
		t.Errorf("expected largest block to be truncated, got len %d", len(tr1.Content))
	}
	if !strings.Contains(tr1.Content, "truncated by per-message budget") {
		t.Error("expected truncation notice in result")
	}
}

func TestEnforcePerMessageBudget_PreservesAssistantMessages(t *testing.T) {
	msgs := []types.Message{
		types.AssistantMessage("not a user message"),
	}
	result := EnforcePerMessageBudget(msgs)
	if result[0].GetText() != "not a user message" {
		t.Error("assistant messages should not be modified")
	}
}

func TestEnforcePerMessageBudget_PreservesTextBlocks(t *testing.T) {
	msgs := []types.Message{
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				types.TextBlock{Type: types.ContentTypeText, Text: "please run these tools"},
				types.ToolResultBlock{
					Type:      types.ContentTypeToolResult,
					ToolUseID: "t1",
					Content:   strings.Repeat("x", 250_000),
				},
			},
		},
	}

	result := EnforcePerMessageBudget(msgs)
	// Text block should be preserved
	tb, ok := result[0].Content[0].(types.TextBlock)
	if !ok {
		t.Fatal("expected first block to remain TextBlock")
	}
	if tb.Text != "please run these tools" {
		t.Errorf("text block content changed: %s", tb.Text)
	}
}

func TestEnforcePerMessageBudget_TruncatesLargestFirst(t *testing.T) {
	msgs := []types.Message{
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				types.ToolResultBlock{
					Type:      types.ContentTypeToolResult,
					ToolUseID: "t1",
					Content:   strings.Repeat("a", 50_000), // small
				},
				types.ToolResultBlock{
					Type:      types.ContentTypeToolResult,
					ToolUseID: "t2",
					Content:   strings.Repeat("b", 180_000), // large — should be truncated first
				},
			},
		},
	}

	result := EnforcePerMessageBudget(msgs)

	// Small block should NOT be truncated (50K is small enough)
	tr1 := result[0].Content[0].(types.ToolResultBlock)
	if strings.Contains(tr1.Content, "truncated") {
		t.Error("expected small block to NOT be truncated")
	}

	// Large block should be truncated
	tr2 := result[0].Content[1].(types.ToolResultBlock)
	if !strings.Contains(tr2.Content, "truncated") {
		t.Error("expected large block to be truncated")
	}
}

func TestEnforcePerMessageBudget_EmptySlice(t *testing.T) {
	result := EnforcePerMessageBudget(nil)
	if len(result) != 0 {
		t.Errorf("expected empty result for nil input, got %d", len(result))
	}
}

func TestEnforcePerMessageBudget_NoToolResults(t *testing.T) {
	msgs := []types.Message{
		types.UserMessage("just text, no tools"),
	}
	result := EnforcePerMessageBudget(msgs)
	if result[0].GetText() != "just text, no tools" {
		t.Error("messages without tool results should be unchanged")
	}
}

func TestEnforcePerMessageBudget_DoesNotMutateOriginal(t *testing.T) {
	original := strings.Repeat("x", 250_000)
	msgs := []types.Message{
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				types.ToolResultBlock{
					Type:      types.ContentTypeToolResult,
					ToolUseID: "t1",
					Content:   original,
				},
			},
		},
	}

	_ = EnforcePerMessageBudget(msgs)
	// Original should be untouched
	tr := msgs[0].Content[0].(types.ToolResultBlock)
	if len(tr.Content) != 250_000 {
		t.Error("EnforcePerMessageBudget should not mutate the original messages")
	}
}

func TestEnforcePerMessageBudget_PerMessageIsolation(t *testing.T) {
	// Two separate messages, each with 150K — individually under 200K budget
	msgs := []types.Message{
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				types.ToolResultBlock{
					Type:      types.ContentTypeToolResult,
					ToolUseID: "t1",
					Content:   strings.Repeat("a", 150_000),
				},
			},
		},
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				types.ToolResultBlock{
					Type:      types.ContentTypeToolResult,
					ToolUseID: "t2",
					Content:   strings.Repeat("b", 150_000),
				},
			},
		},
	}

	result := EnforcePerMessageBudget(msgs)
	// Neither message should be truncated since each is individually under budget
	for i, msg := range result {
		tr := msg.Content[0].(types.ToolResultBlock)
		if strings.Contains(tr.Content, "truncated") {
			t.Errorf("message %d should NOT be truncated (each individually under budget)", i)
		}
	}
}
