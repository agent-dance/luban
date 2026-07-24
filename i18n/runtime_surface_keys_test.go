package i18n

import "testing"

func TestRuntimeSurfaceSemanticCopyCoversEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyRuntimeErrorEvidenceRetention, KeyRuntimeHookEvidenceRetention,
		KeyRuntimeToolCallPresentation, KeyRuntimeToolResultRetention,
		KeyRuntimeHiddenToolCall, KeyRuntimeHiddenToolResult,
		KeyRuntimeContextCompaction, KeyRuntimeLegacyAction, KeyRuntimeLegacyImpact,
		KeyRuntimeLegacyRule, KeyRuntimeLegacyScope, KeyRuntimeQueryCancelled,
		KeyDeleteHistoryAction, KeyDeleteHistoryImpact, KeyDeleteHistoryRisk,
		KeyDeleteHistoryRule, KeyDeleteHistoryScope, KeyDeleteHistoryBody,
		KeyDeleteHistoryMessage,
		KeyMarkdownImagePrefix,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}
