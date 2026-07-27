package i18n

import (
	"strings"
	"testing"
)

func TestProviderServiceTierKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range []Key{
		KeyProviderServiceTierInvalid,
		KeyProviderServiceTierUnsupported,
		KeyProviderServiceTierMismatch,
	} {
		for _, language := range AllLanguages() {
			if text := Text(language, key); text == "" || text == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", language.Code(), key, text)
			}
		}
	}
}

func TestProviderServiceTierMismatchFormatsActualBeforeExpected(t *testing.T) {
	const actual = "actual-tier-sentinel"
	const expected = "expected-tier-sentinel"
	for _, language := range AllLanguages() {
		formatted := Format(language, KeyProviderServiceTierMismatch, actual, expected)
		actualAt := strings.Index(formatted, actual)
		expectedAt := strings.Index(formatted, expected)
		if actualAt < 0 || expectedAt < 0 || actualAt >= expectedAt {
			t.Fatalf("Format(%s) = %q, want actual tier before expected tier", language.Code(), formatted)
		}
	}
}
