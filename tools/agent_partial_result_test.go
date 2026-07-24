package tools

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestExtractPartialAgentResult_CapturesTextAndToolResults(t *testing.T) {
	transcript := []types.Message{
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "Looking into the file."},
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "tu_a", Name: "Read", Input: map[string]any{}},
		}},
		{Role: types.RoleUser, Content: []types.ContentBlock{
			types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: "tu_a", Content: "line 1\nline 2"},
		}},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: "Found a hit."},
			types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: "tu_b", Name: "Grep", Input: map[string]any{}},
		}},
		// tu_b never resolved — agent was interrupted here.
	}
	out := ExtractPartialAgentResult(transcript)
	if !strings.Contains(out.Text, "Looking into") || !strings.Contains(out.Text, "Found a hit") {
		t.Fatalf("expected salvaged text, got %q", out.Text)
	}
	if len(out.ToolResults) != 1 || out.ToolResults[0].ToolUseID != "tu_a" {
		t.Fatalf("expected one resolved tool result for tu_a, got %+v", out.ToolResults)
	}
	if out.ToolResults[0].ToolName != "Read" {
		t.Fatalf("expected ToolName=Read, got %q", out.ToolResults[0].ToolName)
	}
}

func TestExtractPartialAgentResult_EmptyForEmptyTranscript(t *testing.T) {
	out := ExtractPartialAgentResult(nil)
	if out.Text != "" || out.ToolResults != nil {
		t.Fatalf("expected empty result, got %+v", out)
	}
	if FormatPartialAgentResult(out) != "" {
		t.Fatalf("expected empty format string for empty result")
	}
}

func TestFormatPartialAgentResult_RendersHeader(t *testing.T) {
	p := PartialAgentResult{
		Text: "salvaged",
		ToolResults: []PartialToolResult{
			{ToolUseID: "x", ToolName: "Bash", Content: "hello"},
		},
	}
	formatted := FormatPartialAgentResult(p)
	if !strings.Contains(formatted, "[partial result before cancel]") {
		t.Fatalf("expected header, got %q", formatted)
	}
	if !strings.Contains(formatted, "Bash") || !strings.Contains(formatted, "hello") {
		t.Fatalf("expected tool name and content, got %q", formatted)
	}
}
