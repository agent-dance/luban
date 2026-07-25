package i18n

import (
	"strings"
	"testing"
)

func TestToolPlanExitKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolPlanExitKeys {
		for _, lang := range AllLanguages() {
			if copy := Text(lang, key); strings.TrimSpace(copy) == "" || strings.HasPrefix(copy, "[") {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, copy)
			}
		}
	}
}

func TestToolPlanExitKeysEnglishContract(t *testing.T) {
	got := Format(LangEN, KeyToolPlanExitCommitRollback, "rollback failed", "commit failed")
	want := "commit plan exit: commit failed; restore original plan: rollback failed"
	if got != want {
		t.Fatalf("commit rollback copy = %q, want %q", got, want)
	}
}
