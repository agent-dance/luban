package prompt

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/types"
)

func TestStaticPromptSectionOrder(t *testing.T) {
	prompt := BuildSystemPrompt([]types.Tool{
		&mockTool{name: "Inspect", desc: "Inspect repository"},
		&mockTool{name: "ApplyPatch", desc: "Apply patch"},
		&mockTool{name: "Run", desc: "Run graph"},
	}, Config{CWD: "/repo"})

	wantOrder := []string{
		"You are " + brand.DisplayName,
		"# Guardrails",
		"# Outcome optimization",
		"# Coding contract",
		"# Communication",
		"Primary working directory: /repo",
	}
	assertInOrder(t, prompt, wantOrder)
	if strings.Contains(prompt, "Today's date is ") {
		t.Fatal("date should be injected through user context, not the base system prompt")
	}
}

func TestUsingToolsSectionDependsOnEnabledTools(t *testing.T) {
	complete := BuildSystemPrompt([]types.Tool{
		&mockTool{name: "Inspect"}, &mockTool{name: "ApplyPatch"}, &mockTool{name: "Run"},
	}, Config{})
	if !strings.Contains(complete, "# Coding contract") || !strings.Contains(complete, "make the smallest complete change") {
		t.Fatalf("coding guidance missing: %s", complete)
	}
	incomplete := BuildSystemPrompt([]types.Tool{
		&mockTool{name: "Read"}, &mockTool{name: "Edit"}, &mockTool{name: "Bash"},
	}, Config{})
	for _, retired := range []string{"# Coding contract", "# Using your tools", "To read files use Read", "To edit files use Edit", "Reserve Bash for system commands"} {
		if strings.Contains(incomplete, retired) {
			t.Fatalf("incomplete coding kernel retained legacy guidance %q", retired)
		}
	}
}

func TestSimpleModeMinimalPromptShape(t *testing.T) {
	t.Setenv("LUBAN_CODE_SIMPLE", "true")
	blocks := BuildSystemPromptBlocks([]types.Tool{
		&mockTool{name: "Read", desc: "Read files"},
	}, Config{CWD: "/simple"})

	if len(blocks) != 1 {
		t.Fatalf("expected simple mode to produce one block, got %d", len(blocks))
	}
	if blocks[0].Cache {
		t.Fatalf("expected simple mode block to be uncached because it contains cwd, got %#v", blocks[0])
	}
	text := blocks[0].Text
	assertInOrder(t, text, []string{
		"You are " + brand.DisplayName + ", an agentic coding CLI.",
		"CWD: /simple",
	})
	for _, notWant := range []string{"# System", "# Using your tools", "Read files", "Current working directory", "Date: "} {
		if strings.Contains(text, notWant) {
			t.Fatalf("simple mode should not contain %q in %q", notWant, text)
		}
	}
}

func assertInOrder(t *testing.T, haystack string, needles []string) {
	t.Helper()
	offset := 0
	for _, needle := range needles {
		idx := strings.Index(haystack[offset:], needle)
		if idx < 0 {
			t.Fatalf("expected %q after offset %d", needle, offset)
		}
		offset += idx + len(needle)
	}
}
