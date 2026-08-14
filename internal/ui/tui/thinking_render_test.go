package tui

import (
	"strings"
	"testing"

	gtui "github.com/grindlemire/go-tui"
)

func TestRenderThinkingIsBoundedUntilShowAll(t *testing.T) {
	const thought = "The user just said hello.\nI should respond in a friendly and concise way without losing the end of this reasoning message."

	const width = 36
	root := NewRootComponent(NewAppState(), nil, nil)
	root.termWidth = width
	element := root.renderThinking(Message{
		Kind:      MsgAssistantThinking,
		Text:      thought,
		Collapsed: true,
	})

	height := element.HeightForWidth(width)
	rendered := strings.Join(strings.Fields(renderElementText(element, width, height)), " ")
	normalizedThought := strings.Join(strings.Fields(thought), " ")
	if strings.Contains(rendered, normalizedThought) {
		t.Fatalf("collapsed thinking rendered complete provider reasoning:\n%s", rendered)
	}
	if strings.Contains(rendered, "The user just said hello") || !strings.Contains(rendered, "end of this reasoning message") {
		t.Fatalf("collapsed thinking did not render the advancing last line: %q", rendered)
	}
	if !strings.Contains(rendered, "Alt+O") {
		t.Fatalf("collapsed thinking omitted disclosure hint: %q", rendered)
	}

	root.state.SetTranscriptShowAll(true)
	element = root.renderThinking(Message{Kind: MsgAssistantThinking, Text: thought})
	height = element.HeightForWidth(width)
	rendered = strings.Join(strings.Fields(renderElementText(element, width, height)), " ")
	if !strings.Contains(rendered, normalizedThought) {
		t.Fatalf("show-all thinking lost durable content:\n%s", rendered)
	}
}

func TestThinkingPreviewTracksStreamingTail(t *testing.T) {
	if got := thinkingPreview("first step\nsecond step\n\n", 80); got != "second step" {
		t.Fatalf("last non-empty line preview = %q", got)
	}

	const first = "0123456789"
	const advanced = first + "abcdef"
	if got := thinkingPreview(first, 6); got != "…56789" {
		t.Fatalf("initial streaming tail = %q", got)
	}
	if got := thinkingPreview(advanced, 6); got != "…bcdef" {
		t.Fatalf("advanced streaming tail = %q", got)
	}
	if got := thinkingPreview("之前的推理最后进展", 9); terminalCellWidth(got) > 9 || !strings.HasSuffix(got, "进展") {
		t.Fatalf("wide-character preview = %q (%d cells), want <= 9 cells with advancing tail", got, terminalCellWidth(got))
	}
}

func TestThinkingPreviewFillsViewportWithoutCoveringScrollbar(t *testing.T) {
	const width = 40
	root := NewRootComponent(NewAppState(), nil, nil)
	root.termWidth = width
	thinking := root.renderThinking(Message{
		Kind: MsgAssistantThinking,
		Text: "this streaming thought is deliberately much wider than the viewport and keeps advancing",
	})
	children := []*gtui.Element{thinking}
	for range 8 {
		children = append(children, gtui.New(gtui.WithText("extra row"), gtui.WithWidthPercent(100)))
	}
	scroller := gtui.New(
		gtui.WithDirection(gtui.Column),
		gtui.WithWidth(width),
		gtui.WithHeight(4),
		gtui.WithScrollable(gtui.ScrollVertical),
		gtui.WithChildren(children...),
	)
	buffer := gtui.NewBuffer(width, 4)
	scroller.Render(buffer, width, 4)

	if got := buffer.Cell(width-2, 1).Rune; got == 0 || got == ' ' {
		t.Fatalf("thinking preview did not fill the last usable cell: %q", renderElementText(scroller, width, 4))
	}
	if got := buffer.Cell(width-1, 1).Rune; got != '│' && got != '█' {
		t.Fatalf("thinking preview covered scrollbar column with %q", got)
	}
}
