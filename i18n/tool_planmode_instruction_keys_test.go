package i18n

import (
	"strings"
	"testing"
)

func TestToolPlanModeInstructionCatalogCoversEveryLanguage(t *testing.T) {
	const rawMessage = "raw-enter-plan-message-17"
	for _, key := range toolPlanModeInstructionKeys {
		for _, lang := range AllLanguages() {
			got := Format(lang, key, rawMessage)
			if got == "" || strings.HasPrefix(got, "[") {
				t.Errorf("Format(%s, %q) = %q", lang.Code(), key, got)
			}
			if !strings.Contains(got, rawMessage) {
				t.Errorf("Format(%s, %q) omitted the localized entry message: %q", lang.Code(), key, got)
			}
			if strings.Contains(got, "%!") {
				t.Errorf("Format(%s, %q) has a formatting error: %q", lang.Code(), key, got)
			}
		}
	}
}

func TestToolPlanModeInstructionEnglishCompatibility(t *testing.T) {
	const message = "Entered plan mode. You should now focus on exploring the codebase and designing an implementation approach."
	tests := []struct {
		key  Key
		want string
	}{
		{
			KeyToolPlanModeInterviewInstructions,
			message + "\n\n" +
				"DO NOT write or edit any files except the plan file. Detailed workflow instructions will follow.",
		},
		{
			KeyToolPlanModeInstructions,
			message + "\n\n" +
				"In plan mode, you should:\n" +
				"1. Thoroughly explore the codebase to understand existing patterns\n" +
				"2. Identify similar features and architectural approaches\n" +
				"3. Consider multiple approaches and their trade-offs\n" +
				"4. Use AskUserQuestion if you need to clarify the approach\n" +
				"5. Design a concrete implementation strategy\n" +
				"6. When ready, use ExitPlanMode to present your plan for approval\n\n" +
				"Remember: DO NOT write or edit any files yet. This is a read-only exploration and planning phase.",
		},
	}
	for _, tt := range tests {
		if got := Format(LangEN, tt.key, message); got != tt.want {
			t.Errorf("Format(LangEN, %q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestToolPlanModeInstructionsKeepProtocolTerms(t *testing.T) {
	for _, lang := range AllLanguages() {
		got := Format(lang, KeyToolPlanModeInstructions, "raw-message")
		for _, term := range []string{"AskUserQuestion", "ExitPlanMode", "phase"} {
			if !strings.Contains(got, term) {
				t.Errorf("%s translation omitted protocol term %q: %q", lang.Code(), term, got)
			}
		}
	}
}
