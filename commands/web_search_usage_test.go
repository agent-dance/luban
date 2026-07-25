package commands_test

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/commands"
)

func TestStatusDisplaysWebSearchRequests(t *testing.T) {
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)
	var output strings.Builder
	ctx := &commands.Context{
		QueryLoop:              &stubQL{model: "claude-sonnet-4-6"},
		CurrentModel:           "claude-sonnet-4-6",
		SessionID:              "session",
		TotalWebSearchRequests: 2,
		OnCommandPresentation:  captureCompletedCommand(&output),
	}
	if err := registry.Find("status").Execute(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "2") || !strings.Contains(strings.ToLower(output.String()), "web search") {
		t.Fatalf("status output omitted web search usage: %q", output.String())
	}
}
