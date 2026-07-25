package app

import (
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
