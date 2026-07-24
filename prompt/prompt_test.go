package prompt

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

// mockTool for testing tool descriptions
type mockTool struct {
	name string
	desc string
}

func (t *mockTool) Name() string        { return t.name }
func (t *mockTool) Description() string { return t.desc }
func (t *mockTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t *mockTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	return types.ToolResult{Content: "ok"}, nil
}

func TestBuildSystemPromptBasic(t *testing.T) {
	prompt := BuildSystemPrompt(nil, Config{})

	if !strings.Contains(prompt, "LUBAN Code") {
		t.Error("expected base identity to mention LUBAN Code")
	}
	if strings.Contains(prompt, "Anthropic's official CLI") {
		t.Error("LUBAN Code must not claim to be Anthropic's official CLI")
	}
	if strings.Contains(prompt, "Today's date is ") {
		t.Error("did not expect date in base system prompt")
	}
}

func TestBuildSystemPromptWithCWD(t *testing.T) {
	prompt := BuildSystemPrompt(nil, Config{CWD: "/test/dir"})
	if !strings.Contains(prompt, "/test/dir") {
		t.Error("expected CWD in prompt")
	}
}

func TestBuildSystemPromptBlocks(t *testing.T) {
	cfg := Config{CWD: "/test/dir"}
	blocks := BuildSystemPromptBlocks(nil, cfg)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 system prompt blocks, got %d", len(blocks))
	}
	if blocks[0].Text == "" || !blocks[0].Cache {
		t.Fatalf("expected cached static block, got %#v", blocks[0])
	}
	if blocks[1].Text == "" || blocks[1].Cache {
		t.Fatalf("expected uncached dynamic block, got %#v", blocks[1])
	}
	if got := blocks.String(); got != BuildSystemPrompt(nil, cfg) {
		t.Fatal("block string form should match legacy BuildSystemPrompt output")
	}
}

func TestBuildSystemPromptWithCustomInstructions(t *testing.T) {
	cfg := Config{
		CustomInstructions: "Always use TypeScript",
	}
	prompt := BuildSystemPrompt(nil, cfg)
	if strings.Contains(prompt, "Always use TypeScript") {
		t.Error("did not expect custom instructions in base system prompt")
	}
	if strings.Contains(prompt, "User Instructions") {
		t.Error("did not expect User Instructions header in base system prompt")
	}
}

func TestUserContextMetaMessageCarriesDateAndClaudeMd(t *testing.T) {
	cfg := Config{CustomInstructions: "Always use TypeScript"}
	ctx := (UserContextBuilder{
		Date: time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
	}).FromConfig(cfg).Build()

	msg, ok := ctx.MetaMessage()
	if !ok {
		t.Fatal("expected user context meta message")
	}
	if msg.Role != types.RoleUser || !msg.IsMeta {
		t.Fatalf("expected meta user message, got role=%s meta=%v", msg.Role, msg.IsMeta)
	}
	text := msg.GetText()
	assertInOrder(t, text, []string{
		"<system-reminder>",
		"# claudeMd",
		"Always use TypeScript",
		"# currentDate",
		"Today's date is 2026-07-10.",
		"</system-reminder>",
	})
}

func TestBuildSystemPromptWithTools(t *testing.T) {
	tools := []types.Tool{
		&mockTool{name: "Bash", desc: "Run commands"},
		&mockTool{name: "Read", desc: "Read files"},
	}
	prompt := BuildSystemPrompt(tools, Config{})
	if strings.Contains(prompt, "## Bash") {
		t.Error("did not expect Bash schema description duplicated in system prompt")
	}
	if strings.Contains(prompt, "Run commands") || strings.Contains(prompt, "Read files") {
		t.Error("did not expect tool descriptions duplicated in system prompt")
	}
	if strings.Contains(prompt, "Available Tools") {
		t.Error("did not expect Available Tools dump")
	}
	if !strings.Contains(prompt, "# Using your tools") {
		t.Error("expected global tool-use guidance")
	}
	if !strings.Contains(prompt, "To read files use Read instead of cat") {
		t.Error("expected enabled Read guidance")
	}
}

func TestBuildSystemPromptCustomToolDescriptions(t *testing.T) {
	prompt := BuildSystemPrompt(nil, Config{
		ToolDescriptions: "My custom tool docs",
	})
	if !strings.Contains(prompt, "My custom tool docs") {
		t.Error("expected custom tool descriptions")
	}
}

func TestBuildSystemPromptToolDescOverridesTools(t *testing.T) {
	tools := []types.Tool{&mockTool{name: "Bash", desc: "Run commands"}}
	prompt := BuildSystemPrompt(tools, Config{
		ToolDescriptions: "Override",
	})
	// Custom descriptions should take priority over auto-generated
	if !strings.Contains(prompt, "Override") {
		t.Error("expected custom descriptions to override")
	}
	if strings.Contains(prompt, "## Bash") {
		t.Error("auto-generated descriptions should not appear when custom provided")
	}
	if strings.Contains(prompt, "Run commands") {
		t.Error("auto-generated tool descriptions should not appear when custom provided")
	}
}

func TestLoadClaudeMDExisting(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "CLAUDE.md")
	os.WriteFile(fp, []byte("  my instructions  "), 0644)

	result := LoadClaudeMD(fp)
	if result != "my instructions" {
		t.Errorf("expected trimmed content, got '%s'", result)
	}
}

func TestLoadClaudeMDNonexistent(t *testing.T) {
	result := LoadClaudeMD("/nonexistent/CLAUDE.md")
	if result != "" {
		t.Errorf("expected empty string for missing file, got '%s'", result)
	}
}

func TestLoadClaudeMDFallback(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "fallback.md")
	os.WriteFile(fp, []byte("fallback content"), 0644)

	result := LoadClaudeMD("/nonexistent/primary.md", fp)
	if result != "fallback content" {
		t.Errorf("expected fallback content, got '%s'", result)
	}
}

func TestBaseIdentity(t *testing.T) {
	identity := baseIdentity()
	if !strings.Contains(identity, "LUBAN Code") {
		t.Error("expected LUBAN Code in identity")
	}
	if !strings.Contains(identity, "# System") {
		t.Error("expected sectioned system prompt")
	}
}

func TestBuildToolDescriptions(t *testing.T) {
	tools := []types.Tool{
		&mockTool{name: "TestTool", desc: "Does testing"},
	}
	desc := buildToolDescriptions(tools)
	if !strings.Contains(desc, "## TestTool") {
		t.Error("expected tool name as header")
	}
	if !strings.Contains(desc, "Does testing") {
		t.Error("expected tool description")
	}
}
