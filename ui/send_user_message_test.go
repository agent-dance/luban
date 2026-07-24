package ui_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
	"github.com/agent-dance/luban/ui"
)

func task24RenderedOutput() types.SendUserMessageOutput {
	return types.SendUserMessageOutput{
		Message: "Deployment complete.",
		Attachments: []types.SendUserMessageAttachment{
			{Path: "/tmp/report.log", Size: 1536},
			{Path: "/tmp/screen.png", Size: 12, IsImage: true},
		},
		SentAt: "2026-07-11T04:34:56.789Z",
	}
}

func TestSendUserMessageRenderDefaultHasNoToolChrome(t *testing.T) {
	var output bytes.Buffer
	renderer := ui.NewTermRenderer(&output)
	ui.DispatchToolCall(renderer, "SendUserMessage", map[string]any{"message": "Deployment complete."})
	handled := ui.DispatchToolResult(renderer, types.ToolResultBlock{
		Content: "Message delivered to user.",
		Data:    task24RenderedOutput(),
	})
	if !handled {
		t.Fatal("typed SendUserMessage result was not specially rendered")
	}
	text := output.String()
	for _, want := range []string{"Deployment complete.", "[file] /tmp/report.log (1.5 KB)", "[image] /tmp/screen.png (12 B)"} {
		if !strings.Contains(text, want) {
			t.Fatalf("render missing %q in %q", want, text)
		}
	}
	for _, unwanted := range []string{"SendUserMessage", "Message delivered to user.", "⚡", "↳"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("default Brief render contains generic chrome %q in %q", unwanted, text)
		}
	}
}

func TestBriefRenderModesAndTranscriptSendUserMessage(t *testing.T) {
	output := task24RenderedOutput()
	now := time.Date(2026, 7, 11, 5, 0, 0, 0, time.UTC)
	chat := ui.FormatSendUserMessage(output, ui.SendUserMessageRenderOptions{
		Mode: ui.SendUserMessageRenderBriefOnly,
		Now:  now,
	})
	if !strings.Contains(chat, "Claude ") || !strings.Contains(chat, ":34") || !strings.Contains(chat, output.Message) {
		t.Fatalf("brief-only render = %q", chat)
	}
	transcript := ui.FormatSendUserMessage(output, ui.SendUserMessageRenderOptions{
		Mode: ui.SendUserMessageRenderTranscript,
	})
	if !strings.HasPrefix(transcript, "* "+output.Message) {
		t.Fatalf("transcript render = %q", transcript)
	}
	plain := ui.FormatSendUserMessage(output, ui.SendUserMessageRenderOptions{})
	if strings.Contains(plain, "Claude") || strings.HasPrefix(plain, "*") {
		t.Fatalf("default render should be plain assistant text: %q", plain)
	}
}

func TestSendUserMessageJSONRenderUsesAssistantChannel(t *testing.T) {
	var output bytes.Buffer
	renderer := ui.NewJSONRenderer(&output)
	ui.DispatchToolResult(renderer, types.ToolResultBlock{Data: task24RenderedOutput()})
	lines := decodeLines(t, &output)
	if len(lines) != 1 || lines[0]["type"] != "assistant_message" {
		t.Fatalf("JSON events = %#v", lines)
	}
	if lines[0]["message"] != "Deployment complete." {
		t.Fatalf("JSON message = %#v", lines[0]["message"])
	}
	if _, exists := lines[0]["output"]; exists {
		t.Fatalf("JSON Brief event leaked generic tool output: %#v", lines[0])
	}
}

func TestSendUserMessageJSONRenderPreservesHiddenToolIdentity(t *testing.T) {
	var output bytes.Buffer
	renderer := ui.NewJSONRenderer(&output)
	ctx := ui.ToolEventContext{
		SessionID: "session-brief", TurnID: "session-brief:query-1:turn-1",
		ActorID: "assistant", ActorType: "assistant", WorkUnitID: "brief-work",
	}
	ui.DispatchToolCallEvent(renderer, ctx, types.ToolUseBlock{
		ID: "toolu-brief", Name: "SendUserMessage", Input: map[string]any{"message": "Deployment complete."},
	})
	handled := ui.DispatchToolResultEvent(renderer, ctx, types.ToolResultBlock{
		ToolUseID: "toolu-brief", Content: "internal acknowledgement",
		Data: task24RenderedOutput(), Outcome: types.ToolOutcomeSucceeded,
	})
	if !handled {
		t.Fatal("typed SendUserMessage result was not handled")
	}
	lines := decodeLines(t, &output)
	if len(lines) != 2 || lines[0]["type"] != "tool_use" || lines[0]["hidden"] != true || lines[1]["type"] != "assistant_message" {
		t.Fatalf("JSON Brief events = %#v", lines)
	}
	for _, event := range lines {
		for key, want := range map[string]any{
			"session_id": "session-brief", "turn_id": "session-brief:query-1:turn-1",
			"actor_id": "assistant", "actor_type": "assistant", "work_unit_id": "brief-work", "tool_use_id": "toolu-brief",
		} {
			if got := event[key]; got != want {
				t.Errorf("%s = %#v, want %#v in %#v", key, got, want, event)
			}
		}
	}
	if lines[1]["outcome"] != string(types.ToolOutcomeSucceeded) || lines[1]["message"] != "Deployment complete." {
		t.Fatalf("assistant outcome/message = %#v", lines[1])
	}
	if _, displayed := lines[1]["output"]; displayed {
		t.Fatalf("assistant event displayed model acknowledgement as output: %s", output.String())
	}
	if _, displayed := lines[1]["content"]; displayed {
		t.Fatalf("assistant event displayed model acknowledgement as prose: %s", output.String())
	}
	envelope, ok := lines[1]["result_envelope"].(map[string]any)
	if !ok || envelope["content"] != "internal acknowledgement" {
		t.Fatalf("assistant event did not retain the lossless result envelope: %#v", lines[1])
	}
}
