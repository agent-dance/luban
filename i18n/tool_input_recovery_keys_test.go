package i18n

import (
	"strings"
	"testing"
)

func TestToolInputRecoveryKeysAreCompleteAndParameterized(t *testing.T) {
	keys := []Key{
		KeyRuntimeToolInputRecoveryRetry,
		KeyRuntimeToolInputRecoveryFailed,
		KeyRuntimeToolInputRecoveryRepeated,
		KeyRuntimeToolInputRecoveryAbandoned,
		KeyLoopVisibleToolInputRecovery,
		KeyLoopVisibleToolInputRecoveryMissingValue,
		KeyLoopVisibleToolInputRecoveryAtOffset,
		KeyLoopToolInputRecoveryFailed,
		KeyLoopToolInputRecoveryRepeated,
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
	if got := Format(LangZH, KeyLoopVisibleToolInputRecoveryMissingValue, "Inspect", "cursor", 12); !strings.Contains(got, "Inspect") || !strings.Contains(got, "cursor") || !strings.Contains(got, "12") {
		t.Fatalf("missing-value recovery format = %q", got)
	}
	if got := Format(LangZH, KeyLoopVisibleToolInputRecoveryAtOffset, "Inspect", 12); !strings.Contains(got, "Inspect") || !strings.Contains(got, "12") {
		t.Fatalf("offset recovery format = %q", got)
	}
	if got := Format(LangZH, KeyRuntimeToolInputRecoveryRepeated, "Inspect", 1); !strings.Contains(got, "Inspect") || !strings.Contains(got, "1") {
		t.Fatalf("repeated recovery format = %q", got)
	}
}
