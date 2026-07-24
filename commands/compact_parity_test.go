package commands_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/types"
)

func TestCompactCommandParityFailureSurfacesError(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	ql := &stubQL{messages: []types.Message{
		types.UserMessage("old user"),
		types.AssistantMessage("old assistant"),
		types.UserMessage("middle user"),
		types.AssistantMessage("middle assistant"),
		types.UserMessage("latest user"),
		types.AssistantMessage("latest assistant"),
	}}
	var output string
	ctx := &commands.Context{
		QueryLoop: ql,
		OnEvent:   func(s string) { output += s },
		CompactFunc: func(string) error {
			return errors.New("summary provider unavailable")
		},
	}

	err := r.Find("compact").Execute(ctx, "")
	if err == nil || !strings.Contains(err.Error(), "summary provider unavailable") {
		t.Fatalf("error = %v, want provider failure", err)
	}
	if strings.Contains(output, "truncation fallback") {
		t.Fatalf("output should not mention truncation fallback: %q", output)
	}
	if len(ql.messages) != 6 {
		t.Fatalf("messages changed after failed compact: got %d", len(ql.messages))
	}
}

func TestCompactCommandSurfacesFriendlyCompactErrors(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	ql := &stubQL{messages: []types.Message{
		types.UserMessage("old user"),
		types.AssistantMessage("old assistant"),
		types.UserMessage("latest user"),
		types.AssistantMessage("latest assistant"),
	}}
	ctx := &commands.Context{
		QueryLoop: ql,
		OnEvent:   func(string) {},
		CompactFunc: func(string) error {
			return &compact.CompactError{
				Kind:    compact.ErrCompactIncomplete,
				Message: compact.MessageIncomplete,
			}
		},
	}

	err := r.Find("compact").Execute(ctx, "")
	if err == nil {
		t.Fatal("expected compact error")
	}
	if got := err.Error(); got != compact.MessageIncomplete {
		t.Fatalf("error = %q, want friendly incomplete compact message", got)
	}
}

func TestCompactCommandParityPassesCustomInstructions(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	ql := &stubQL{messages: []types.Message{
		types.UserMessage("old user"),
		types.AssistantMessage("old assistant"),
		types.UserMessage("latest user"),
		types.AssistantMessage("latest assistant"),
	}}
	var seen string
	ctx := &commands.Context{
		QueryLoop: ql,
		OnEvent:   func(string) {},
		CompactFunc: func(customInstructions string) error {
			seen = customInstructions
			ql.SetMessages([]types.Message{types.UserMessage("compacted")})
			return nil
		},
	}

	if err := r.Find("compact").Execute(ctx, "focus on test output"); err != nil {
		t.Fatal(err)
	}
	if seen != "focus on test output" {
		t.Fatalf("custom instructions = %q", seen)
	}
}
