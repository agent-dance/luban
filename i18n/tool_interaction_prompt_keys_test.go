package i18n

import "testing"

func TestToolInteractionPromptKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolInteractionPromptKeys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == string(key) {
				t.Fatalf("translation missing: key=%q lang=%s", key, lang.Code())
			}
		}
	}
}
