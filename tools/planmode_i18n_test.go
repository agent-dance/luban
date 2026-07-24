package tools

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestEnterPlanModeResultInstructionsUseActiveLanguage(t *testing.T) {
	previous := i18n.DetectOrLoadLanguage()
	t.Cleanup(func() {
		if err := i18n.SaveLanguage(previous); err != nil {
			t.Errorf("restore language: %v", err)
		}
	})
	if err := i18n.SaveLanguage(i18n.LangZH); err != nil {
		t.Fatalf("SaveLanguage(LangZH): %v", err)
	}

	tool := NewEnterPlanModeTool(NewPlanState(t.TempDir()))
	input := enterPlanModeResult{Message: "raw-enter-message"}

	t.Setenv("USER_TYPE", "")
	t.Setenv("CLAUDE_CODE_PLAN_MODE_INTERVIEW_PHASE", "")
	standard := tool.MapToolResultToToolResultBlock(input, "toolu_standard")
	if !strings.Contains(standard.Content, "在 plan mode 中") || strings.Contains(standard.Content, "In plan mode, you should:") {
		t.Fatalf("standard instructions did not use the active language: %q", standard.Content)
	}
	for _, raw := range []string{"raw-enter-message", "AskUserQuestion", "ExitPlanMode", "phase"} {
		if !strings.Contains(standard.Content, raw) {
			t.Errorf("standard instructions omitted raw value %q: %q", raw, standard.Content)
		}
	}

	t.Setenv("CLAUDE_CODE_PLAN_MODE_INTERVIEW_PHASE", "1")
	interview := tool.MapToolResultToToolResultBlock(input, "toolu_interview")
	if !strings.Contains(interview.Content, "除 plan 文件外") || strings.Contains(interview.Content, "Detailed workflow instructions will follow") {
		t.Fatalf("interview instructions did not use the active language: %q", interview.Content)
	}
	if interview.ToolUseID != "toolu_interview" || !strings.Contains(interview.Content, "raw-enter-message") {
		t.Fatalf("interview instructions lost protocol values: %#v", interview)
	}
}
