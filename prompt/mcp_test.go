package prompt

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestMCPInstructionsSectionEmpty(t *testing.T) {
	got := MCPInstructionsSection([]MCPServerInstruction{
		{Name: "empty"},
		{Name: "blank", Instructions: "  \n\t"},
	})
	if got != "" {
		t.Fatalf("expected no section for empty instructions, got %q", got)
	}
}

func TestMCPInstructionsSectionForLanguageLocalizesVisibleCopy(t *testing.T) {
	got := MCPInstructionsSectionForLanguage(i18n.LangZH, []MCPServerInstruction{{Name: "docs", Instructions: "raw server instructions"}})
	for _, want := range []string{"MCP Server 使用说明", "docs", "raw server instructions"} {
		if !strings.Contains(got, want) {
			t.Fatalf("localized MCP instructions = %q, want %q", got, want)
		}
	}
	if strings.Contains(got, "The following MCP servers") {
		t.Fatalf("localized MCP instructions retained English product copy: %q", got)
	}
}

func TestMCPInstructionsSectionOneServer(t *testing.T) {
	got := MCPInstructionsSection([]MCPServerInstruction{{Name: "docs", Instructions: "Prefer official docs."}})
	want := "# MCP Server Instructions\n\nThe following MCP servers have provided instructions for how to use their tools and resources:\n\n## docs\nPrefer official docs."
	if got != want {
		t.Fatalf("section mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestMCPInstructionsSectionMultipleServersSorted(t *testing.T) {
	got := MCPInstructionsSection([]MCPServerInstruction{
		{Name: "zeta", Instructions: "Use zeta."},
		{Name: "alpha", Instructions: "Use alpha."},
	})
	assertInOrder(t, got, []string{"## alpha\nUse alpha.", "## zeta\nUse zeta."})
	if strings.Contains(got, "## zeta\nUse zeta.\n\n## alpha") {
		t.Fatalf("expected stable sorted server order, got:\n%s", got)
	}
}
