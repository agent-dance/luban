package commands_test

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/commands"
)

func TestStatusDisplaysWebSearchRequests(t *testing.T) {
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)
	var output string
	ctx := &commands.Context{
		QueryLoop:              &stubQL{model: "claude-sonnet-4-6"},
		CurrentModel:           "claude-sonnet-4-6",
		SessionID:              "session",
		TotalWebSearchRequests: 2,
		OnEvent:                func(value string) { output += value },
	}
	if err := registry.Find("status").Execute(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "2") || !strings.Contains(strings.ToLower(output), "web search") {
		t.Fatalf("status output omitted web search usage: %q", output)
	}
}
