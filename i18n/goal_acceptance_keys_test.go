package i18n

import (
	"strings"
	"testing"
)

func TestGoalAcceptanceSemanticCopyIsCompleteAndFormatted(t *testing.T) {
	keys := []Key{
		KeyCommandGoalCriteriaHeader,
		KeyCommandGoalCriterionMet,
		KeyRootGoalAcceptanceCriteriaRequired,
		KeyRootGoalAcceptanceCriteriaUnmet,
		KeyToolGoalAcceptanceCriteriaHeader,
		KeyPresentationGoalCriteriaProgress,
		KeyTUIGoalPanelTitle,
		KeyTUIGoalCriterionUnmet,
		KeyLoopGoalEvaluatorCriteriaIncomplete,
	}
	for _, lang := range AllLanguages() {
		for _, key := range keys {
			if got := Text(lang, key); got == "" || strings.HasPrefix(got, "[") && strings.HasSuffix(got, "]") {
				t.Fatalf("Text(%s, %s) = %q", lang.Code(), key, got)
			}
		}
		if got := Format(lang, KeyCommandGoalCriterionMet, "AC-1", "tests pass"); !strings.Contains(got, "AC-1") || !strings.Contains(got, "tests pass") {
			t.Fatalf("criterion format for %s = %q", lang.Code(), got)
		}
	}
}
