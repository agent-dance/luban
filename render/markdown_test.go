package render

import (
	"regexp"
	"strings"
	"testing"
)

// stripANSI removes all ANSI escape sequences from a string,
// making it easier to test rendered content.
var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiEscapeRe.ReplaceAllString(s, "")
}

func TestMarkdown_Headings(t *testing.T) {
	input := "# Title\n## Subtitle\n### Section"
	out := stripANSI(Markdown(input))
	if !strings.Contains(out, "Title") {
		t.Error("expected heading text")
	}
	if !strings.Contains(out, "Subtitle") {
		t.Error("expected subtitle text")
	}
}

func TestMarkdown_CodeBlock(t *testing.T) {
	input := "```go\nfmt.Println(\"hello\")\n```"
	out := stripANSI(Markdown(input))
	if !strings.Contains(out, "fmt") {
		t.Error("expected code content 'fmt'")
	}
	if !strings.Contains(out, "Println") {
		t.Error("expected code content 'Println'")
	}
}

func TestMarkdown_BulletList(t *testing.T) {
	input := "- item one\n- item two"
	out := stripANSI(Markdown(input))
	if !strings.Contains(out, "item one") {
		t.Error("expected list item text")
	}
	if !strings.Contains(out, "item two") {
		t.Error("expected second list item text")
	}
}

func TestMarkdown_NumberedList(t *testing.T) {
	input := "1. first\n2. second"
	out := stripANSI(Markdown(input))
	if !strings.Contains(out, "first") {
		t.Error("expected numbered list item")
	}
}

func TestMarkdown_InlineFormatting(t *testing.T) {
	input := "This is **bold** and *italic* and `code`"
	out := stripANSI(Markdown(input))
	if !strings.Contains(out, "bold") {
		t.Error("expected bold text")
	}
	if !strings.Contains(out, "italic") {
		t.Error("expected italic text")
	}
	if !strings.Contains(out, "code") {
		t.Error("expected code text")
	}
}

func TestMarkdown_Link(t *testing.T) {
	input := "See [docs](https://example.com)"
	out := stripANSI(Markdown(input))
	if !strings.Contains(out, "docs") {
		t.Error("expected link text")
	}
}

func TestMarkdown_HorizontalRule(t *testing.T) {
	input := "---"
	out := Markdown(input)
	if strings.TrimSpace(out) == "" {
		t.Error("expected non-empty output for horizontal rule")
	}
}

func TestMarkdown_Blockquote(t *testing.T) {
	input := "> important note"
	out := stripANSI(Markdown(input))
	if !strings.Contains(out, "important") {
		t.Error("expected blockquote text")
	}
}

func TestMarkdown_PlainText(t *testing.T) {
	input := "Just plain text"
	out := stripANSI(Markdown(input))
	if !strings.Contains(out, "Just") || !strings.Contains(out, "plain") || !strings.Contains(out, "text") {
		t.Errorf("expected plain text passthrough, got: %q", out)
	}
}

func TestMarkdown_Table(t *testing.T) {
	input := "| Name | Age |\n|------|-----|\n| Alice | 30 |"
	out := stripANSI(Markdown(input))
	if !strings.Contains(out, "Alice") {
		t.Error("expected table content")
	}
	if !strings.Contains(out, "30") {
		t.Error("expected table value")
	}
}

func TestMarkdown_NonEmpty(t *testing.T) {
	// Verify that various inputs produce non-empty output
	inputs := []string{
		"Hello world",
		"# Heading",
		"- list",
		"> quote",
		"```\ncode\n```",
		"**bold**",
	}
	for _, input := range inputs {
		out := Markdown(input)
		if strings.TrimSpace(out) == "" {
			t.Errorf("Markdown(%q) returned empty output", input)
		}
	}
}
