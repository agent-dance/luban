package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
	"github.com/agent-dance/luban/ui"
)

func TestDropTextInBriefTurnSendUserMessageRender(t *testing.T) {
	state := NewAppState()
	state.AppendMessage(Message{Kind: MsgUser, Text: "status?"})
	state.AppendOrStreamTextForTurn("Earlier tool-turn detail", 1)
	state.AppendMessage(Message{Kind: MsgToolCall, ToolName: "Read"})
	state.AppendMessage(Message{Kind: MsgToolResult, ToolName: "Read", Text: "ok"})
	state.AppendOrStreamTextForTurn("Duplicate final text", 2)
	state.AppendSendUserMessage(types.SendUserMessageOutput{
		Message: "The build is green.", SentAt: "2026-07-11T04:34:56.789Z",
	}, ui.SendUserMessageRenderOptions{DropAssistantText: true, TurnCount: 2})

	messages := state.Messages.Get()
	assistantCount := 0
	briefCount := 0
	for _, message := range messages {
		if message.Kind == MsgAssistant {
			assistantCount++
		}
		if message.Kind == MsgSendUserMessage {
			briefCount++
		}
	}
	if assistantCount != 1 || briefCount != 1 {
		t.Fatalf("assistant/Brief counts = %d/%d; messages=%#v", assistantCount, briefCount, messages)
	}
	if len(messages) != 5 || messages[1].Kind != MsgAssistant || messages[2].Kind != MsgToolCall || messages[3].Kind != MsgToolResult {
		t.Fatalf("non-text detail events were not preserved: %#v", messages)
	}
}

func TestTranscriptSendUserMessageRenderPreservesAssistantDetail(t *testing.T) {
	state := NewAppState()
	state.AppendMessage(Message{Kind: MsgUser, Text: "status?"})
	state.AppendOrStreamTextForTurn("detail text", 7)
	state.AppendSendUserMessage(types.SendUserMessageOutput{Message: "visible message"}, ui.SendUserMessageRenderOptions{
		Mode: ui.SendUserMessageRenderTranscript, TurnCount: 7, DropAssistantText: true,
	})
	messages := state.Messages.Get()
	if len(messages) != 3 || messages[1].Kind != MsgAssistant || messages[2].Kind != MsgSendUserMessage {
		t.Fatalf("transcript messages = %#v", messages)
	}
}

func TestBriefOnlySendUserMessageRenderShowsLabelTimestampAndAttachments(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)
	output := types.SendUserMessageOutput{
		Message: "Artifacts ready.",
		Attachments: []types.SendUserMessageAttachment{
			{Path: "/tmp/output.log", Size: 10},
			{Path: "/tmp/image.png", Size: 20, IsImage: true},
		},
		SentAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	text := collectElementText(root.renderSendUserMessage(Message{
		Kind: MsgSendUserMessage, Brief: &output, BriefMode: ui.SendUserMessageRenderBriefOnly,
	}))
	for _, want := range []string{"Claude", "Artifacts ready.", "[file] /tmp/output.log (10 B)", "[image] /tmp/image.png (20 B)"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Brief render missing %q in %q", want, text)
		}
	}
	if strings.Contains(text, "SendUserMessage") || strings.Contains(text, "Message delivered") {
		t.Fatalf("Brief render contains tool chrome/ack: %q", text)
	}
}
