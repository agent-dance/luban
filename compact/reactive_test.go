package compact

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestReactiveCompactWithoutCompactorFailsClosedAndPreservesHistory(t *testing.T) {
	messages := []types.Message{
		types.UserMessage("first ordinary prompt"),
		types.AssistantMessage("middle response"),
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				types.TextBlock{Type: types.ContentTypeText, Text: "tail attachment"},
				types.ImageBlock{Type: types.ContentTypeImage, Source: &types.ImageSource{
					Type: "base64", MediaType: "image/png", Data: "raw-media",
				}},
			},
		},
	}
	want, err := json.Marshal(messages)
	if err != nil {
		t.Fatalf("marshal original history: %v", err)
	}

	result, attempted, err := TryReactiveCompact(context.Background(), messages, ReactiveCompactOptions{
		MediaStrip: true,
		Trigger:    "reactive",
	})
	if !attempted {
		t.Fatal("attempted = false, want typed fail-closed attempt")
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
	var unavailable *ReactiveCompactorUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("error type = %T, want *ReactiveCompactorUnavailableError: %v", err, err)
	}
	got, marshalErr := json.Marshal(messages)
	if marshalErr != nil {
		t.Fatalf("marshal recovered history: %v", marshalErr)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("nil-compactor recovery mutated history:\n got: %s\nwant: %s", got, want)
	}
}

func TestReactiveCompactUsesCompactorOnce(t *testing.T) {
	compactor := &HistorySnip{KeepFirst: 1, KeepLast: 1}
	messages := []types.Message{
		types.UserMessage("first"),
		types.AssistantMessage("second"),
		types.UserMessage("third"),
		types.AssistantMessage("fourth"),
	}

	result, attempted, err := TryReactiveCompact(context.Background(), messages, ReactiveCompactOptions{
		Compactor: compactor,
		Trigger:   "reactive",
	})
	result = authorizeCompactionResultForTest(result)
	if err != nil {
		t.Fatalf("TryReactiveCompact: %v", err)
	}
	if !attempted {
		t.Fatal("attempted = false, want true")
	}
	post := BuildPostCompactMessages(result)
	if !IsCompactBoundaryMessage(post[0]) {
		t.Fatalf("first post compact message should be compact boundary: %#v", post[0])
	}
	got := joinedCompactTestText(post)
	if strings.Contains(got, "second") || strings.Contains(got, "third") {
		t.Fatalf("post compact text still contains snipped middle messages: %q", got)
	}

	result, attempted, err = TryReactiveCompact(context.Background(), messages, ReactiveCompactOptions{
		Compactor:    compactor,
		HasAttempted: true,
		Trigger:      "reactive",
	})
	if err != nil {
		t.Fatalf("guarded TryReactiveCompact: %v", err)
	}
	if attempted || result != nil {
		t.Fatalf("guarded attempt = %v result=%#v, want no-op", attempted, result)
	}
}

func joinedCompactTestText(messages []types.Message) string {
	var b strings.Builder
	for _, msg := range messages {
		b.WriteString(msg.GetText())
		b.WriteByte('\n')
	}
	return b.String()
}

func TestReactiveCompactMediaStripRemovesMediaFromCompactorInputAndPreservesTail(t *testing.T) {
	compactor := &reactiveRecordingCompactor{}
	messages := []types.Message{
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				types.TextBlock{Type: types.ContentTypeText, Text: "old attached"},
				types.ImageBlock{Type: types.ContentTypeImage, Source: &types.ImageSource{Type: "base64", MediaType: "image/png", Data: strings.Repeat("x", 100)}},
				types.ToolResultBlock{
					Type:      types.ContentTypeToolResult,
					ToolUseID: "toolu_1",
					ContentBlocks: []types.ContentBlock{
						types.DocumentBlock{Type: types.ContentTypeDocument, Source: &types.DocumentSource{Type: "base64", MediaType: "application/pdf", Data: strings.Repeat("y", 100)}},
					},
				},
			},
		},
		types.AssistantMessage("middle"),
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				types.TextBlock{Type: types.ContentTypeText, Text: "tail attached"},
				types.ImageBlock{Type: types.ContentTypeImage, Source: &types.ImageSource{Type: "base64", MediaType: "image/png", Data: strings.Repeat("z", 100)}},
			},
		},
	}

	result, attempted, err := TryReactiveCompact(context.Background(), messages, ReactiveCompactOptions{
		Compactor:  compactor,
		MediaStrip: true,
	})
	if err != nil {
		t.Fatalf("TryReactiveCompact: %v", err)
	}
	if !attempted {
		t.Fatal("attempted = false, want true")
	}
	if containsCompactTestMedia(compactor.received) {
		t.Fatalf("compactor input still contained media: %#v", compactor.received)
	}
	post := BuildPostCompactMessages(result)
	if !containsCompactTestMedia(post) {
		t.Fatalf("post-compact preserved tail should keep original media: %#v", post)
	}
}

type reactiveRecordingCompactor struct {
	received []types.Message
}

func (c *reactiveRecordingCompactor) Compact(ctx context.Context, messages []types.Message, keepRecent int) (*CompactionResult, error) {
	return c.CompactWithTrigger(ctx, messages, keepRecent, "reactive")
}

func (c *reactiveRecordingCompactor) CompactWithTrigger(_ context.Context, messages []types.Message, _ int, trigger string) (*CompactionResult, error) {
	c.received = append([]types.Message(nil), messages...)
	boundary := trustedCompactBoundaryForTest(CompactBoundaryMetadata{Trigger: trigger})
	return &CompactionResult{
		BoundaryMarker:  &boundary,
		SummaryMessages: []types.Message{types.UserMessage("summary")},
		MessagesToKeep:  messages[len(messages)-1:],
	}, nil
}

func containsCompactTestMedia(messages []types.Message) bool {
	for _, msg := range messages {
		for _, block := range msg.Content {
			switch typed := block.(type) {
			case types.ImageBlock, types.DocumentBlock:
				return true
			case types.ToolResultBlock:
				for _, nested := range typed.ContentBlocks {
					if nested.GetType() == types.ContentTypeImage || nested.GetType() == types.ContentTypeDocument {
						return true
					}
				}
			}
		}
	}
	return false
}
