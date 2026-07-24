package compact

import (
	"errors"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestLocalizeUserErrorPreservesCategoryAndLanguage(t *testing.T) {
	source := compactError(ErrCompactIncomplete, MessageIncomplete, errors.New("provider stream ended"))
	for _, lang := range i18n.AllLanguages() {
		localized := LocalizeUserError(lang, source)
		if !errors.Is(localized, ErrCompactIncomplete) {
			t.Fatalf("localized %s error lost category: %v", lang.Code(), localized)
		}
		if got, want := localized.Error(), i18n.Text(lang, i18n.KeyAuxCompactInterrupted); got != want {
			t.Fatalf("localized %s error = %q, want %q", lang.Code(), got, want)
		}
	}
}

func TestHasUserErrorCategoryDistinguishesExternalFailures(t *testing.T) {
	if !HasUserErrorCategory(compactError(ErrCompactNoSummary, MessageNoSummary, nil)) {
		t.Fatal("typed compact error was not categorized")
	}
	if HasUserErrorCategory(errors.New("provider unavailable")) {
		t.Fatal("external provider error must remain a raw parameter")
	}
}
