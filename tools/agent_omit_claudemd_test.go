package tools

import (
	"strings"
	"testing"
)

func TestStripClaudeMdSection_RemovesUserInstructionsBlock(t *testing.T) {
	in := "Identity stuff\n\n# User Instructions\n\nDo X. Do Y.\n\n# Available Tools\n\nRead, Edit"
	got := stripClaudeMdSection(in)
	if strings.Contains(got, "Do X") {
		t.Fatalf("CLAUDE.md content still present: %q", got)
	}
	if !strings.Contains(got, "Identity stuff") || !strings.Contains(got, "Available Tools") {
		t.Fatalf("surrounding sections lost: %q", got)
	}
}

func TestStripClaudeMdSection_NoUserInstructions_NoChange(t *testing.T) {
	in := "Identity\n\n# Available Tools\n\nRead, Edit"
	got := stripClaudeMdSection(in)
	if got != in {
		t.Fatalf("expected no change, got %q", got)
	}
}

func TestStripClaudeMdSection_BlockAtEnd(t *testing.T) {
	in := "Identity\n\n# User Instructions\n\nProject rules"
	got := stripClaudeMdSection(in)
	if got != "Identity" {
		t.Fatalf("expected only Identity, got %q", got)
	}
}

func TestStripClaudeMdSection_OnlyClaudeMd(t *testing.T) {
	in := "# User Instructions\n\nProject rules"
	got := stripClaudeMdSection(in)
	if strings.TrimSpace(got) != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestBuildAgentSystemPrompt_OmitClaudeMdStripsButKeepsBase(t *testing.T) {
	base := "Identity\n\n# User Instructions\n\nProject rules\n\n# Tools\n\nLine"
	profile := agentProfile{Name: "Plan", OmitClaudeMd: true}
	out := buildAgentSystemPrompt(base, profile, "default", "")
	if strings.Contains(out, "Project rules") {
		t.Fatalf("CLAUDE.md content should be stripped: %q", out)
	}
	if !strings.Contains(out, "Identity") {
		t.Fatalf("base identity should remain: %q", out)
	}
}

func TestBuildAgentSystemPrompt_OmitBaseWinsOverOmitClaudeMd(t *testing.T) {
	base := "Identity\n\n# User Instructions\n\nProject rules"
	profile := agentProfile{Name: "Explore", OmitBaseSystem: true, OmitClaudeMd: true}
	out := buildAgentSystemPrompt(base, profile, "default", "")
	if strings.Contains(out, "Identity") || strings.Contains(out, "Project rules") {
		t.Fatalf("OmitBaseSystem should drop everything, got %q", out)
	}
}
