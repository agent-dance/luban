package prompt

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestSystemPromptFromVisibleDefinitionsUsesOnlyVisibleNames(t *testing.T) {
	definitions := []types.ToolDefinition{
		{Name: "Inspect"},
		{Name: "ApplyPatch"},
		{Name: "Run"},
	}
	blocks := BuildSystemPromptBlocksForDefinitions(definitions, Config{CWD: "/repo"})
	joined := blocks.JoinedText()
	if !strings.Contains(joined, "The complete visible catalog is Inspect, ApplyPatch, and Run") {
		t.Fatal("exact V2 definitions did not activate V2 guidance")
	}
	for _, absent := range []string{"When the user explicitly asks you to use subagents", "Break down and manage your work with the TaskCreate tool"} {
		if strings.Contains(joined, absent) {
			t.Fatalf("prompt advertised a tool absent from the visible definitions: %q", absent)
		}
	}
}
