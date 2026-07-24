package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mustWriteSkillFile creates root/<name>/SKILL.md with the given body.
func mustWriteSkillFile(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMCPPrompt_QualifiedName(t *testing.T) {
	p := MCPPrompt{Server: "github", Name: "commit"}
	if got := p.QualifiedName(); got != "github:commit" {
		t.Errorf("expected github:commit; got %q", got)
	}
	if got := (MCPPrompt{Name: "lone"}).QualifiedName(); got != "lone" {
		t.Errorf("expected fallback to bare name; got %q", got)
	}
	if got := (MCPPrompt{Server: "x"}).QualifiedName(); got != "" {
		t.Errorf("expected empty when name missing; got %q", got)
	}
}

func TestManager_RegisterMCPPromptsExposesQualifiedName(t *testing.T) {
	m := NewManager()
	m.RegisterMCPPrompts([]MCPPrompt{
		{Server: "issues", Name: "triage", Description: "Triage issue", Body: "Triage prompt body"},
	})
	got := m.Get("issues:triage")
	if got == nil {
		t.Fatalf("expected MCP prompt to be discoverable; manager names=%v", m.Names())
	}
	if got.Source != SourceMCP {
		t.Errorf("expected SourceMCP; got %v", got.Source)
	}
	if !strings.Contains(got.Content, "Triage prompt body") {
		t.Errorf("expected body to be available; got %q", got.Content)
	}
}

func TestManager_LocalSkillBeatsMCPPrompt(t *testing.T) {
	dir := t.TempDir()
	// Note: MCP prompts use "server:name" namespace; on-disk skills use a
	// directory name that cannot include ":". To check the override path
	// we register an MCP prompt with no Server (so it qualifies as just
	// "shared") and a local "shared" skill at the same name.
	mustWriteSkillFile(t, dir, "shared", "Local body")
	m := NewManager(DirSource{Dir: dir, Source: SourceProject})
	m.RegisterMCPPrompts([]MCPPrompt{
		{Name: "shared", Description: "MCP shared", Body: "MCP body"},
	})

	got := m.Get("shared")
	if got == nil {
		t.Fatalf("expected shared skill to be loaded")
	}
	if !strings.Contains(got.Content, "Local body") {
		t.Errorf("expected local skill to win; got %q", got.Content)
	}
	if got.Source == SourceMCP {
		t.Errorf("expected local source to win, got %v", got.Source)
	}
}

func TestManager_RegisterMCPPromptsClearsOnEmpty(t *testing.T) {
	m := NewManager()
	m.RegisterMCPPrompts([]MCPPrompt{{Name: "first", Body: "x"}})
	if m.Get("first") == nil {
		t.Fatal("expected first to be present after register")
	}
	m.RegisterMCPPrompts(nil)
	if m.Get("first") != nil {
		t.Error("expected first to be removed after re-register with empty list")
	}
}

func TestManager_MCPPromptsSnapshot(t *testing.T) {
	m := NewManager()
	m.RegisterMCPPrompts([]MCPPrompt{
		{Server: "a", Name: "x", Body: "x"},
		{Server: "b", Name: "y", Body: "y"},
	})
	snaps := m.MCPPrompts()
	if len(snaps) != 2 {
		t.Errorf("expected 2 MCP prompts; got %d", len(snaps))
	}
}

// -------- frontmatter strip --------

func TestPrepareSkillContent_StripsFrontmatter(t *testing.T) {
	raw := "---\ndescription: x\nmodel: sonnet\n---\nThe real body."
	parsed := parseFrontmatter(raw, "test.md")
	if strings.Contains(parsed.Content, "model: sonnet") {
		t.Errorf("parseFrontmatter must strip the YAML block; got: %q", parsed.Content)
	}
	if strings.Contains(parsed.Content, "---") {
		t.Errorf("parseFrontmatter must remove the --- delimiters; got: %q", parsed.Content)
	}
	if !strings.Contains(parsed.Content, "The real body.") {
		t.Errorf("body should remain; got: %q", parsed.Content)
	}
}
