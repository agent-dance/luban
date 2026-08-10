package i18n

import (
	"strings"
	"testing"
)

func TestToolInputRecoveryKeysAreCompleteAndParameterized(t *testing.T) {
	keys := []Key{
		KeyRuntimeToolInputRecoveryRetry,
		KeyRuntimeToolInputRecoveryFailed,
		KeyRuntimeToolInputRecoveryAbandoned,
		KeyLoopVisibleToolInputRecovery,
		KeyLoopToolInputRecoveryFailed,
		KeyLoopToolInputRecoveryAbandoned,
		KeyTUIInvalidToolUse,
		KeyThinkingCollapsedHint,
	}
	for _, key := range keys {
		translations := semanticTranslations[key]
		for _, lang := range AllLanguages() {
			if strings.TrimSpace(translations[lang]) == "" {
				t.Fatalf("key %s missing %s translation", key, lang)
			}
		}
	}
	if got := Format(LangZH, KeyRuntimeToolInputRecoveryRetry, "Inspect", 1, 1); !strings.Contains(got, "Inspect") || !strings.Contains(got, "1/1") {
		t.Fatalf("recovery retry format = %q", got)
	}
}
