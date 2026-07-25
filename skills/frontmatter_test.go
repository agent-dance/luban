package skills

import (
	"strings"
	"testing"
)

func TestParseFrontmatter_Basic(t *testing.T) {
	md := `---
description: My awesome skill
allowed-tools: Read, Write, Edit
when_to_use: When the user asks to refactor code
model: sonnet
context: fork
agent: Bash
effort: high
version: "1.0"
shell: bash
---
# Refactor Skill

Do the refactoring.
`
	parsed := parseFrontmatter(md, "test.md")

	if parsed.Frontmatter.Description == nil || *parsed.Frontmatter.Description != "My awesome skill" {
		t.Errorf("description: got %v", parsed.Frontmatter.Description)
	}
	if len(parsed.Frontmatter.AllowedTools) != 3 {
		t.Errorf("allowed-tools: expected 3, got %v", parsed.Frontmatter.AllowedTools)
	}
	if parsed.Frontmatter.WhenToUse == nil || *parsed.Frontmatter.WhenToUse != "When the user asks to refactor code" {
		t.Errorf("when_to_use: got %v", parsed.Frontmatter.WhenToUse)
	}
	if parsed.Frontmatter.Model == nil || *parsed.Frontmatter.Model != "sonnet" {
		t.Errorf("model: got %v", parsed.Frontmatter.Model)
	}
	if parsed.Frontmatter.Context == nil || *parsed.Frontmatter.Context != "fork" {
		t.Errorf("context: got %v", parsed.Frontmatter.Context)
	}
	if parsed.Frontmatter.Agent == nil || *parsed.Frontmatter.Agent != "Bash" {
		t.Errorf("agent: got %v", parsed.Frontmatter.Agent)
	}
	if parsed.Frontmatter.Effort == nil || *parsed.Frontmatter.Effort != "high" {
		t.Errorf("effort: got %v", parsed.Frontmatter.Effort)
	}
	if parsed.Frontmatter.Shell == nil || *parsed.Frontmatter.Shell != "bash" {
		t.Errorf("shell: got %v", parsed.Frontmatter.Shell)
	}

	if !strings.Contains(parsed.Content, "# Refactor Skill") {
		t.Errorf("content should include body, got: %q", parsed.Content)
	}
	if strings.Contains(parsed.Content, "---") {
		t.Errorf("content should not include frontmatter delimiters, got: %q", parsed.Content)
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	md := "# Just a heading\n\nSome content."
	parsed := parseFrontmatter(md, "test.md")

	if parsed.Content != md {
		t.Errorf("expected content unchanged, got: %q", parsed.Content)
	}
	if parsed.Frontmatter.Description != nil {
		t.Errorf("expected nil description, got %v", parsed.Frontmatter.Description)
	}
}

func TestParseFrontmatter_AllowedToolsArray(t *testing.T) {
	md := `---
allowed-tools:
  - Read
  - Write
  - Bash
---
content
`
	parsed := parseFrontmatter(md, "test.md")
	if len(parsed.Frontmatter.AllowedTools) != 3 {
		t.Errorf("expected 3 tools, got %v", parsed.Frontmatter.AllowedTools)
	}
}

func TestParseFrontmatter_PathsWithGlob(t *testing.T) {
	md := `---
paths: "src/**/*.{ts,tsx}, tests/**/*.test.ts"
---
content
`
	parsed := parseFrontmatter(md, "test.md")
	if len(parsed.Frontmatter.Paths) != 2 {
		t.Errorf("expected 2 paths, got %v", parsed.Frontmatter.Paths)
	}
	if len(parsed.Frontmatter.Paths) > 0 && parsed.Frontmatter.Paths[0] != "src/**/*.{ts,tsx}" {
		t.Errorf("first path: got %q", parsed.Frontmatter.Paths[0])
	}
}

func TestParseFrontmatter_BooleanFields(t *testing.T) {
	md := `---
disable-model-invocation: "true"
user-invocable: "false"
---
content
`
	parsed := parseFrontmatter(md, "test.md")
	if parsed.Frontmatter.DisableModelInvocation == nil || *parsed.Frontmatter.DisableModelInvocation != "true" {
		t.Errorf("disable-model-invocation: got %v", parsed.Frontmatter.DisableModelInvocation)
	}
	if parsed.Frontmatter.UserInvocable == nil || *parsed.Frontmatter.UserInvocable != "false" {
		t.Errorf("user-invocable: got %v", parsed.Frontmatter.UserInvocable)
	}
}

func TestParseFrontmatter_Arguments(t *testing.T) {
	md := `---
arguments: file, mode, output
---
Process $file in $mode mode, output to $output.
`
	parsed := parseFrontmatter(md, "test.md")
	if len(parsed.Frontmatter.Arguments) != 3 {
		t.Errorf("expected 3 arguments, got %v", parsed.Frontmatter.Arguments)
	}
}

func TestApplyFrontmatter(t *testing.T) {
	md := `---
name: custom-name
description: Custom description
allowed-tools: Read, Write
model: haiku
context: fork
agent: general-purpose
effort: max
version: "2.0"
paths: "*.go"
shell: powershell
disable-model-invocation: "true"
user-invocable: "true"
arguments: src, dest
argument-hint: <source> <destination>
when_to_use: When copying files
---
Copy $src to $dest
`
	parsed := parseFrontmatter(md, "test.md")
	skill := &Skill{Name: "original", Description: "Skill: original"}
	applyFrontmatter(skill, parsed.Frontmatter)

	if skill.Name != "custom-name" {
		t.Errorf("Name: got %q", skill.Name)
	}
	if skill.Description != "Custom description" {
		t.Errorf("Description: got %q", skill.Description)
	}
	if len(skill.AllowedTools) != 2 {
		t.Errorf("AllowedTools: got %v", skill.AllowedTools)
	}
	if skill.Model != "haiku" {
		t.Errorf("Model: got %q", skill.Model)
	}
	if skill.Context != ContextFork {
		t.Errorf("Context: got %q", skill.Context)
	}
	if skill.Agent != "general-purpose" {
		t.Errorf("Agent: got %q", skill.Agent)
	}
	if skill.Effort != "max" {
		t.Errorf("Effort: got %q", skill.Effort)
	}
	if skill.Version != "2.0" {
		t.Errorf("Version: got %q", skill.Version)
	}
	if len(skill.Paths) != 1 || skill.Paths[0] != "*.go" {
		t.Errorf("Paths: got %v", skill.Paths)
	}
	if skill.Shell != "powershell" {
		t.Errorf("Shell: got %q", skill.Shell)
	}
	if !skill.DisableModelInvocation {
		t.Error("expected DisableModelInvocation=true")
	}
	if skill.UserInvocable == nil || !*skill.UserInvocable {
		t.Error("expected UserInvocable=true")
	}
	if len(skill.ArgNames) != 2 {
		t.Errorf("ArgNames: got %v", skill.ArgNames)
	}
	if skill.ArgumentHint != "<source> <destination>" {
		t.Errorf("ArgumentHint: got %q", skill.ArgumentHint)
	}
	if skill.WhenToUse != "When copying files" {
		t.Errorf("WhenToUse: got %q", skill.WhenToUse)
	}
}

func TestSplitCommaSafe(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"a, b, c", []string{"a", "b", "c"}},
		{"*.{ts,tsx}, *.go", []string{"*.{ts,tsx}", "*.go"}},
		{"{a,b}/{c,d}, e", []string{"{a,b}/{c,d}", "e"}},
		{"single", []string{"single"}},
		{"", nil},
	}

	for _, tt := range tests {
		got := splitCommaSafe(tt.input)
		if len(got) != len(tt.expected) {
			t.Errorf("splitCommaSafe(%q): got %v, want %v", tt.input, got, tt.expected)
			continue
		}
		for i := range got {
			if got[i] != tt.expected[i] {
				t.Errorf("splitCommaSafe(%q)[%d]: got %q, want %q", tt.input, i, got[i], tt.expected[i])
			}
		}
	}
}
