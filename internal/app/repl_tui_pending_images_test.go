package app

import (
	"reflect"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/runtime/engine"
	"github.com/agent-dance/luban/internal/ui/tui"
	"github.com/agent-dance/luban/types"
)

func TestAttachPendingImagesToQuerySupportsImageOnlyPrompt(t *testing.T) {
	req := attachPendingImagesToQuery(engine.QueryRequest{SessionID: "session"}, []tui.ImageAttachment{{
		ID: 1, Base64: "image-data", MediaType: "image/png", Placeholder: "[Image #1]",
	}})

	if req.Message != "" {
		t.Fatalf("image-only message = %q, want empty", req.Message)
	}
	if len(req.Content) != 1 {
		t.Fatalf("image-only content blocks = %d, want 1", len(req.Content))
	}
	image, ok := req.Content[0].(types.ImageBlock)
	if !ok || image.Source == nil || image.Source.Data != "image-data" || image.Source.MediaType != "image/png" {
		t.Fatalf("image-only content = %#v, want pending image block", req.Content[0])
	}
}

func TestAttachPendingImagesToQueryKeepsPromptTextBeforeImage(t *testing.T) {
	req := attachPendingImagesToQuery(engine.QueryRequest{Message: "describe"}, []tui.ImageAttachment{{
		ID: 1, Base64: "image-data", MediaType: "image/png", Placeholder: "[Image #1]",
	}})

	if req.Message != "" || len(req.Content) != 2 {
		t.Fatalf("text-and-image request = message %q content %#v", req.Message, req.Content)
	}
	text, ok := req.Content[0].(types.TextBlock)
	if !ok || text.Text != "describe" {
		t.Fatalf("first content block = %#v, want prompt text", req.Content[0])
	}
	if _, ok := req.Content[1].(types.ImageBlock); !ok {
		t.Fatalf("second content block = %#v, want image", req.Content[1])
	}
}

func TestAttachPendingImagesToQueryPreservesInlineOrderAndRuneOffsets(t *testing.T) {
	req := attachPendingImagesToQuery(engine.QueryRequest{Message: "甲前文 后文尾"}, []tui.ImageAttachment{
		{ID: 1, Base64: "first", MediaType: "image/png"},
		{ID: 2, Base64: "second", MediaType: "image/jpeg"},
	}, map[int]pendingImagePosition{1: {offset: 3}, 2: {offset: 6}})
	if req.Message != "" || len(req.Content) != 5 {
		t.Fatalf("inline request = message %q content %#v", req.Message, req.Content)
	}
	wantText := map[int]string{0: "甲前文", 2: " 后文", 4: "尾"}
	for index, want := range wantText {
		block, ok := req.Content[index].(types.TextBlock)
		if !ok || block.Text != want {
			t.Fatalf("content[%d] = %#v, want text %q", index, req.Content[index], want)
		}
	}
	for index, want := range map[int]string{1: "first", 3: "second"} {
		block, ok := req.Content[index].(types.ImageBlock)
		if !ok || block.Source == nil || block.Source.Data != want {
			t.Fatalf("content[%d] = %#v, want image %q", index, req.Content[index], want)
		}
	}
}

func TestExtractPendingImagePositionsUsesComposerOrder(t *testing.T) {
	images := []tui.ImageAttachment{
		{ID: 1, Placeholder: "[Image #1]"},
		{ID: 2, Placeholder: "[Image #2]"},
	}
	text, positions := extractPendingImagePositions("开头 [Image #2] 中间 [Image #1] 结尾", images)
	if text != "开头 中间 结尾" {
		t.Fatalf("submitted text = %q", text)
	}
	if positions[2].offset != 2 || positions[1].offset != 5 || positions[2].order != 0 || positions[1].order != 1 {
		t.Fatalf("image positions = %#v, want image 2 before image 1", positions)
	}
}

func TestAdjustPendingImagePositionsForTrim(t *testing.T) {
	positions := adjustPendingImagePositionsForTrim("  前文 后文  ", "前文 后文", map[int]pendingImagePosition{1: {offset: 5}})
	if positions[1].offset != 3 {
		t.Fatalf("adjusted position = %d, want 3", positions[1].offset)
	}
}

func TestPendingImageMarkersPreserveBoundaryAndAdjacentOrder(t *testing.T) {
	images := []tui.ImageAttachment{
		{ID: 1, Placeholder: "[Image #1]", Base64: "first", MediaType: "image/png"},
		{ID: 2, Placeholder: "[Image #2]", Base64: "second", MediaType: "image/png"},
	}
	tests := []struct {
		name      string
		composer  string
		wantText  string
		wantKinds []string
	}{
		{name: "start", composer: " [Image #1] 后文", wantText: "后文", wantKinds: []string{"first", "后文", "second"}},
		{name: "end", composer: "前文 [Image #1] ", wantText: "前文", wantKinds: []string{"前文", "first", "second"}},
		{name: "only", composer: " [Image #1] ", wantText: "", wantKinds: []string{"first", "second"}},
		{name: "adjacent composer order", composer: " [Image #2]  [Image #1] ", wantText: "", wantKinds: []string{"second", "first"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, positions := extractPendingImagePositions(tt.composer, images)
			text := strings.TrimSpace(raw)
			positions = adjustPendingImagePositionsForTrim(raw, text, positions)
			if text != tt.wantText {
				t.Fatalf("text = %q, want %q", text, tt.wantText)
			}
			req := attachPendingImagesToQuery(engine.QueryRequest{Message: text}, images, positions)
			if got := contentBlockSummary(req.Content); !reflect.DeepEqual(got, tt.wantKinds) {
				t.Fatalf("content = %#v, want %#v", got, tt.wantKinds)
			}
		})
	}
}

func contentBlockSummary(content []types.ContentBlock) []string {
	result := make([]string, 0, len(content))
	for _, block := range content {
		switch typed := block.(type) {
		case types.TextBlock:
			result = append(result, typed.Text)
		case types.ImageBlock:
			result = append(result, typed.Source.Data)
		}
	}
	return result
}
