package app

import (
	"bytes"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/runtime/loop"
	"github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/types"
)

func TestTextAndPrintHandlersProjectEventErrorBeforeTerminalRendering(t *testing.T) {
	secret := "/Users/private/.config/provider.json token=sk-print-secret\x1b[2J"
	event := stream.Event{
		Type: stream.EventError, Text: secret, Error: &types.APIError{Type: "private-provider-code", Message: secret},
		ProjectRoot: "/private/project", TurnID: "private-session:query-1:turn-1", ToolUseID: "private-tool",
		ActorID: "private-actor", WorkUnitID: "private-work", Metadata: map[string]any{"authorization": "Bearer private-token"},
	}

	tests := []struct {
		name    string
		handler func(presentation.Renderer) func(stream.Event)
	}{
		{name: "print", handler: func(renderer presentation.Renderer) func(stream.Event) { return makeEventHandler(renderer, false) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			test.handler(ui.NewTermRenderer(&output))(event)
			got := output.String()
			if !strings.Contains(got, i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeErrorPublicSummary)) {
				t.Fatalf("runtime error omitted strict user projection: %q", got)
			}
			for _, private := range []string{secret, "sk-print-secret", "private-provider-code", "private-token", "private-session", "private-tool", "private-project", "private-actor", "private-work", "\x1b[2J"} {
				if strings.Contains(got, private) {
					t.Fatalf("runtime-error projection leaked %q: %q", private, got)
				}
			}
		})
	}
}

type legacyTUIRuntimeErrorCapture struct {
	ui.QuietRenderer
	message string
}

func (r *legacyTUIRuntimeErrorCapture) Error(message string) { r.message = message }

func TestTUIEventHandlerLegacyFallbackProjectsEventError(t *testing.T) {
	renderer := &legacyTUIRuntimeErrorCapture{}
	handler, cleanup := makeTUIEventHandler(renderer, nil, nil)
	t.Cleanup(cleanup)
	secret := "/private/tui/path token=sk-tui-secret"
	handler(stream.Event{
		Type: stream.EventError, Text: secret,
		Error:    &types.APIError{Type: "private-provider-code", Message: secret},
		Metadata: map[string]any{"authorization": "Bearer private-token"},
	})

	want := i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeErrorPublicSummary)
	if renderer.message != want {
		t.Fatalf("legacy TUI runtime error = %q, want strict projection %q", renderer.message, want)
	}
	for _, private := range []string{secret, "sk-tui-secret", "private-provider-code", "private-token"} {
		if strings.Contains(renderer.message, private) {
			t.Fatalf("legacy TUI runtime-error projection leaked %q: %q", private, renderer.message)
		}
	}
}

func privateSystemWarningFixture() stream.Event {
	secret := "/Users/private/.config/warning token=sk-system-warning\x1b[2J"
	event := loop.NewSystemWarningEvent(
		i18n.KeyRuntimeAutoCompactFailed,
		nil,
		&types.APIError{Type: "private-warning-code", Message: secret},
		map[string]any{"authorization": "Bearer private-token"},
		3,
	)
	// Populate every legacy bypass field to prove that presentation consumes
	// only RuntimeEvent, even when a stale producer sends both channels.
	event.Text = secret
	event.Error = &types.APIError{Type: "raw-warning-code", Message: secret}
	event.Metadata = map[string]any{"authorization": "Bearer raw-private-token"}
	event.ProjectRoot = "/private/project"
	return event
}

func assertNoPrivateSystemWarningMaterial(t *testing.T, got string) {
	t.Helper()
	if !strings.Contains(got, i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeAutoCompactFailed)) {
		t.Fatalf("semantic warning projection missing: %q", got)
	}
	for _, private := range []string{
		"/Users/private/.config/warning", "sk-system-warning", "private-warning-code",
		"raw-warning-code", "private-token", "raw-private-token", "/private/project", "\x1b[2J",
	} {
		if strings.Contains(got, private) {
			t.Fatalf("system-warning projection leaked %q: %q", private, got)
		}
	}
}

func TestTextAndPrintHandlersStrictlyProjectSystemWarnings(t *testing.T) {
	tests := []struct {
		name    string
		handler func(presentation.Renderer) func(stream.Event)
	}{
		{name: "print", handler: func(renderer presentation.Renderer) func(stream.Event) { return makeEventHandler(renderer, false) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			test.handler(ui.NewTermRenderer(&output))(privateSystemWarningFixture())
			assertNoPrivateSystemWarningMaterial(t, output.String())
		})
	}
}

type legacyTUIRuntimeWarningCapture struct {
	ui.QuietRenderer
	message string
}

func (r *legacyTUIRuntimeWarningCapture) Warning(message string) { r.message = message }

func TestTUIEventHandlerStrictlyProjectsSystemWarning(t *testing.T) {
	renderer := &legacyTUIRuntimeWarningCapture{}
	handler, cleanup := makeTUIEventHandler(renderer, nil, nil)
	t.Cleanup(cleanup)
	handler(privateSystemWarningFixture())
	assertNoPrivateSystemWarningMaterial(t, renderer.message)
}
