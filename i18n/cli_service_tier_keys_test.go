package i18n

import "testing"

func TestCLIServiceTierKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range []Key{
		KeyCLIFlagServiceTier,
		KeyCLIServiceTierInvalid,
	} {
		for _, language := range AllLanguages() {
			if text := Text(language, key); text == "" || text == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", language.Code(), key, text)
			}
		}
	}
}
