package ui

import (
	"bytes"
	"testing"

	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/types"
)

// TestQuietRenderer_Text_ProducesOutput verifies the only method that should
// write to the underlying writer is Text().
func TestQuietRenderer_Text_ProducesOutput(t *testing.T) {
	var buf bytes.Buffer
	r := NewQuietRenderer(&buf)
	r.Text("assistant reply")
	if buf.String() != "assistant reply" {
		t.Errorf("Text() wrote %q; want %q", buf.String(), "assistant reply")
	}
}

// TestQuietRenderer_Text_MultipleWrites checks that successive Text() calls
// accumulate output in order with no extra separators.
func TestQuietRenderer_Text_MultipleWrites(t *testing.T) {
	var buf bytes.Buffer
	r := NewQuietRenderer(&buf)
	r.Text("foo")
	r.Text(" ")
	r.Text("bar")
	if buf.String() != "foo bar" {
		t.Errorf("got %q; want %q", buf.String(), "foo bar")
	}
}

// TestQuietRenderer_NoOps verifies every non-Text method is a no-op.
func TestQuietRenderer_NoOps(t *testing.T) {
	var buf bytes.Buffer
	r := NewQuietRenderer(&buf)

	// Call every no-op method.
	r.Thinking("deep thoughts")
	r.Error("oh no")
	r.Info("fyi")
	r.Success("great")
	r.Warning("careful")
	r.Bold("important")
	r.RenderToolCall(presentation.ToolEventContext{}, types.ToolUseBlock{Name: "Bash", Input: map[string]any{"command": "ls"}})
	r.RenderToolResult(presentation.ToolEventContext{}, types.ToolResultBlock{Content: "output", Outcome: types.ToolOutcomeSucceeded})
	r.RenderToolResult(presentation.ToolEventContext{}, types.ToolResultBlock{Content: "err", IsError: true, Outcome: types.ToolOutcomeFailed})
	r.Usage(&types.Usage{InputTokens: 100, OutputTokens: 50})
	r.Usage(nil)
	r.Banner("anthropic", "claude-3-5-sonnet")
	r.SessionInfo("sess-123", []string{"Bash", "Read"})
	r.Newline()
	r.Goodbye()
	r.CostSummary(0.001, 0.01, 100, 50)
	r.ContextBar(50000, 200000)
	stop := r.SpinnerStart("Read")
	stop()

	if buf.Len() != 0 {
		t.Errorf("no-op methods wrote %q; want empty output", buf.String())
	}
}

// TestQuietRenderer_Prompt_Empty verifies that Prompt() returns an empty
// string (quiet mode shouldn't show prompts).
func TestQuietRenderer_Prompt_Empty(t *testing.T) {
	r := NewQuietRenderer(bytes.NewBuffer(nil))
	if p := r.Prompt(); p != "" {
		t.Errorf("Prompt() = %q; want empty string", p)
	}
}

// TestQuietRenderer_ImplementsRenderer is a compile-time check that
// *QuietRenderer satisfies the presentation.Renderer interface.
func TestQuietRenderer_ImplementsRenderer(t *testing.T) {
	var _ presentation.Renderer = (*QuietRenderer)(nil)
}

// TestJSONRenderer_ImplementsRenderer is a compile-time check that
// *JSONRenderer satisfies the presentation.Renderer interface.
func TestJSONRenderer_ImplementsRenderer(t *testing.T) {
	var _ presentation.Renderer = (*JSONRenderer)(nil)
}
