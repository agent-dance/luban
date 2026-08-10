package i18n

import "testing"

func TestRuntimeErrorPublicSummaryCoversEveryLanguage(t *testing.T) {
	for _, lang := range AllLanguages() {
		for _, key := range []Key{
			KeyRuntimeErrorPublicSummary,
			KeyRuntimeErrorProviderAPISuggestion,
			KeyRuntimeErrorProviderAPIsExhausted,
		} {
			if got := Text(lang, key); got == "" || got == string(key) {
				t.Fatalf("missing %s for %s", key, lang)
			}
		}
	}
}
