package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestToolPlanModeErrorKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolPlanModeErrorKeys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}

func TestToolPlanModeErrorsPreservePathAndCause(t *testing.T) {
	cause := errors.New("raw-cause")
	err := WrapError(KeyToolPlanModeCreateDirectory, cause, "/raw/plans")
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "/raw/plans") || !strings.Contains(err.Error(), cause.Error()) {
		t.Fatalf("plan mode error lost path/cause: %v", err)
	}
	if got := Format(LangEN, KeyToolPlanModePersistState, cause); got != "persist plan mode state: raw-cause" {
		t.Fatalf("English compatibility changed: %q", got)
	}
}
