package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestLoopGoalEvaluatorKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range loopGoalEvaluatorKeys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}

func TestLoopGoalEvaluatorErrorPolicies(t *testing.T) {
	raw := errors.New("raw-provider-cause")
	visible := WrapError(KeyLoopGoalEvaluatorProviderCallFailed, raw)
	if !errors.Is(visible, raw) || !strings.Contains(visible.Error(), raw.Error()) {
		t.Fatalf("provider cause was not preserved: %v", visible)
	}
	internal := WrapInternalError(KeyLoopGoalEvaluatorMarshalFailed, raw)
	if !errors.Is(internal, raw) || strings.Contains(internal.Error(), raw.Error()) {
		t.Fatalf("internal cause policy failed: %v", internal)
	}
}
