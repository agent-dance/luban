package tui

import (
	"strings"
	"testing"
)

func TestRenderThinkingIsBoundedUntilShowAll(t *testing.T) {
	const thought = "The user just said hello. This is a simple greeting. I should respond in a friendly and concise way without losing the end of this reasoning message."

	root := NewRootComponent(NewAppState(), nil, nil)
	element := root.renderThinking(Message{
		Kind:      MsgAssistantThinking,
		Text:      thought,
		Collapsed: true,
	})

	const width = 36
	height := element.HeightForWidth(width)
	rendered := strings.Join(strings.Fields(renderElementText(element, width, height)), " ")
	if strings.Contains(rendered, thought) {
		t.Fatalf("collapsed thinking rendered complete provider reasoning:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Alt+O") {
		t.Fatalf("collapsed thinking omitted disclosure hint: %q", rendered)
	}

	root.state.SetTranscriptShowAll(true)
	element = root.renderThinking(Message{Kind: MsgAssistantThinking, Text: thought})
	height = element.HeightForWidth(width)
	rendered = strings.Join(strings.Fields(renderElementText(element, width, height)), " ")
	if !strings.Contains(rendered, thought) {
		t.Fatalf("show-all thinking lost durable content:\n%s", rendered)
	}
}
