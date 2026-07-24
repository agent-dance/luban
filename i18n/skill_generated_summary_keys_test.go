package i18n

import (
	"strings"
	"testing"
)

func TestSkillGeneratedSummaryKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range skillGeneratedSummaryKeys {
		for _, lang := range AllLanguages() {
			got := Format(lang, key, "raw-skill-name")
			if got == "" || got == "["+string(key)+"]" || strings.Contains(got, "%!") {
				t.Errorf("Format(%s, %q) = %q", lang.Code(), key, got)
			}
			if !strings.Contains(got, "raw-skill-name") {
				t.Errorf("Format(%s, %q) omitted skill name: %q", lang.Code(), key, got)
			}
		}
	}
}

func TestSkillGeneratedSummaryEnglishCompatibility(t *testing.T) {
	if got := Format(LangEN, KeySkillGeneratedSummary, "review"); got != "Skill: review" {
		t.Fatalf("generated skill summary = %q", got)
	}
	if got := Format(LangEN, KeyMCPSkillGeneratedSummary, "server:review"); got != "MCP skill: server:review" {
		t.Fatalf("generated MCP skill summary = %q", got)
	}
}
