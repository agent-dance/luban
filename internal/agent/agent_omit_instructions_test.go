package agent

import (
	"strings"
	"testing"
)

func TestStripInstructionsSectionRemovesUserInstructionsBlock(t *testing.T) {
	in := "Identity stuff\n\n# User Instructions\n\nDo X. Do Y.\n\n# Available Tools\n\nRead, Edit"
	got := stripInstructionsSection(in)
	if strings.Contains(got, "Do X") {
		t.Fatalf("instruction content still present: %q", got)
	}
	if !strings.Contains(got, "Identity stuff") || !strings.Contains(got, "Available Tools") {
		t.Fatalf("surrounding sections lost: %q", got)
	}
}

func TestStripInstructionsSectionNoUserInstructionsNoChange(t *testing.T) {
	in := "Identity\n\n# Available Tools\n\nRead, Edit"
	got := stripInstructionsSection(in)
	if got != in {
		t.Fatalf("expected no change, got %q", got)
	}
}

func TestStripInstructionsSectionBlockAtEnd(t *testing.T) {
	in := "Identity\n\n# User Instructions\n\nProject rules"
	got := stripInstructionsSection(in)
	if got != "Identity" {
		t.Fatalf("expected only Identity, got %q", got)
	}
}

func TestStripInstructionsSectionOnlyInstructions(t *testing.T) {
	in := "# User Instructions\n\nProject rules"
	got := stripInstructionsSection(in)
	if strings.TrimSpace(got) != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestBuildAgentSystemPromptOmitInstructionsStripsButKeepsBase(t *testing.T) {
	base := "Identity\n\n# User Instructions\n\nProject rules\n\n# Tools\n\nLine"
	profile := agentProfile{Name: "Plan", OmitInstructions: true}
	out := buildAgentSystemPrompt(base, profile, "default", "")
	if strings.Contains(out, "Project rules") {
		t.Fatalf("instruction content should be stripped: %q", out)
	}
	if !strings.Contains(out, "Identity") {
		t.Fatalf("base identity should remain: %q", out)
	}
}

func TestBuildAgentSystemPromptOmitBaseWinsOverOmitInstructions(t *testing.T) {
	base := "Identity\n\n# User Instructions\n\nProject rules"
	profile := agentProfile{Name: "Explore", OmitBaseSystem: true, OmitInstructions: true}
	out := buildAgentSystemPrompt(base, profile, "default", "")
	if strings.Contains(out, "Identity") || strings.Contains(out, "Project rules") {
		t.Fatalf("OmitBaseSystem should drop everything, got %q", out)
	}
}
