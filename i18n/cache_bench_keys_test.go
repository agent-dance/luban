package i18n

import "testing"

func TestCacheBenchKeysCoverEveryLanguage(t *testing.T) {
	keys := []Key{KeyCacheBenchProviderInitFailed, KeyCacheBenchProvider, KeyCacheBenchHeader, KeyCacheBenchRoundFailed, KeyCacheBenchNoUsage}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}
