package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/types"
	"github.com/agent-dance/luban/ui"
)

func TestScreenReaderRuntimeErrorUsesSafePublicProjection(t *testing.T) {
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, strings.NewReader(""))
	defer renderer.Close()
	base := ui.ToolEventContext{SessionID: "session-screen", ProjectRoot: "/workspace/project", TurnID: "turn-base", ActorID: "actor-base", ActorType: "assistant", WorkUnitID: "work-base"}

	handler := makeScreenReaderEventHandler(renderer, nil, nil, base)
	handler(loop.Event{
		Type: loop.EventError, Text: "tool failed", ToolUseID: "toolu-screen", ProjectRoot: "/workspace/project",
		TurnID: "turn-error", ActorID: "actor-error", ActorType: "reviewer", WorkUnitID: "work-error",
		Error:    &types.APIError{Type: "tool_error", Message: "complete provider failure", Status: 503},
		Metadata: map[string]any{"outcome": "partial", "retryable": true},
	})

	text := output.String()
	for _, want := range []string{"Runtime error", "runtime operation failed"} {
		if !strings.Contains(text, want) {
			t.Fatalf("screen-reader runtime error omitted %q:\n%s", want, text)
		}
	}
	for _, secret := range []string{
		"tool failed", "toolu-screen", "session-screen", "/workspace/project", "turn-error",
		"actor-error", "reviewer", "work-error", "tool_error", "complete provider failure", "503", "partial", "retryable",
	} {
		if strings.Contains(text, secret) {
			t.Fatalf("screen-reader runtime error leaked %q:\n%s", secret, text)
		}
	}
}

func TestScreenReaderSystemWarningUsesStrictProjection(t *testing.T) {
	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, strings.NewReader(""))
	defer renderer.Close()
	handler := makeScreenReaderEventHandler(renderer, nil, nil, ui.ToolEventContext{
		SessionID: "private-session", ProjectRoot: "/private/base",
	})
	handler(privateSystemWarningFixture())
	got := output.String()
	if !strings.Contains(got, i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeAutoCompactFailed)) {
		t.Fatalf("screen-reader warning omitted semantic projection: %q", got)
	}
	for _, private := range []string{
		"/Users/private/.config/warning", "sk-system-warning", "private-warning-code",
		"raw-warning-code", "private-token", "raw-private-token", "/private/project", "\x1b[2J",
	} {
		if strings.Contains(got, private) {
			t.Fatalf("screen-reader warning leaked %q: %q", private, got)
		}
	}
}
