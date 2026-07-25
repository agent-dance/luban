package prompt

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/types"
)

func TestStaticPromptSectionOrder(t *testing.T) {
	prompt := BuildSystemPrompt([]types.Tool{
		&mockTool{name: "Bash", desc: "Run commands"},
		&mockTool{name: "Read", desc: "Read files"},
		&mockTool{name: "Edit", desc: "Edit files"},
		&mockTool{name: "Write", desc: "Write files"},
		&mockTool{name: "Glob", desc: "Find files"},
		&mockTool{name: "Grep", desc: "Search files"},
		&mockTool{name: "TaskCreate", desc: "Create tasks"},
	}, Config{CWD: "/repo"})

	wantOrder := []string{
		"You are " + brand.DisplayName,
		"# System",
		"# Doing tasks",
		"# Executing actions with care",
		"# Using your tools",
		"# Tone and style",
		"# Output efficiency",
		"Primary working directory: /repo",
	}
	assertInOrder(t, prompt, wantOrder)
	if strings.Contains(prompt, "Today's date is ") {
		t.Fatal("date should be injected through user context, not the base system prompt")
	}
}

func TestUsingToolsSectionDependsOnEnabledTools(t *testing.T) {
	tests := []struct {
		name    string
		tools   []types.Tool
		want    []string
		notWant []string
	}{
		{
			name: "read edit write search and task guidance",
			tools: []types.Tool{
				&mockTool{name: "Bash", desc: "Run commands"},
				&mockTool{name: "Read", desc: "Read files"},
				&mockTool{name: "Edit", desc: "Edit files"},
				&mockTool{name: "Write", desc: "Write files"},
				&mockTool{name: "Glob", desc: "Find files"},
				&mockTool{name: "Grep", desc: "Search files"},
				&mockTool{name: "TaskCreate", desc: "Create tasks"},
			},
			want: []string{
				"To read files use Read instead of cat",
				"To edit files use Edit instead of sed",
				"To create files use Write instead of cat with heredoc",
				"To search for files use Glob instead of find",
				"To search the content of files use Grep instead of grep",
				"Break down and manage your work with the TaskCreate tool",
			},
			notWant: []string{
				"## Read",
				"Read files",
			},
		},
		{
			name: "task create guidance",
			tools: []types.Tool{
				&mockTool{name: "TaskCreate", desc: "Create tasks"},
			},
			want: []string{
				"Break down and manage your work with the TaskCreate tool",
			},
			notWant: []string{
				"Create tasks",
			},
		},
		{
			name: "explicit subagent requests use agent tool",
			tools: []types.Tool{
				&mockTool{name: "Agent", desc: "Launch an agent"},
				&mockTool{name: "Bash", desc: "Run commands"},
			},
			want: []string{
				"explicitly asks you to use subagents or agents",
				"honor the requested count",
				"same response so they run in parallel",
			},
		},
		{
			name: "missing tools omit dedicated guidance",
			tools: []types.Tool{
				&mockTool{name: "Bash", desc: "Run commands"},
			},
			want: []string{
				"# Using your tools",
				"Reserve Bash for system commands",
			},
			notWant: []string{
				"To read files use Read",
				"To edit files use Edit",
				"To search for files use Glob",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := BuildSystemPrompt(tt.tools, Config{})
			for _, want := range tt.want {
				if !strings.Contains(prompt, want) {
					t.Fatalf("expected prompt to contain %q", want)
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(prompt, notWant) {
					t.Fatalf("did not expect prompt to contain %q", notWant)
				}
			}
		})
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
