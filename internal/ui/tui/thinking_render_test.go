package tui

import (
	"strings"
	"testing"
)

func TestRenderThinkingShowsCompleteWrappedText(t *testing.T) {
	const thought = "The user just said hello. This is a simple greeting. I should respond in a friendly and concise way without losing the end of this reasoning message."

	root := NewRootComponent(NewAppState(), nil, nil)
	element := root.renderThinking(Message{
		Kind:      MsgAssistantThinking,
		Text:      thought,
		Collapsed: true,
	})

	const width = 36
	height := element.HeightForWidth(width)
	if height < 3 {
		t.Fatalf("thinking height = %d, want wrapped full content", height)
	}
	rendered := strings.Join(strings.Fields(renderElementText(element, width, height)), " ")
	if !strings.Contains(rendered, thought) {
		t.Fatalf("wrapped thinking lost content:\n%s", rendered)
	}
	if strings.Contains(rendered, "…") {
		t.Fatalf("thinking still rendered an ellipsis: %q", rendered)
	}
}
