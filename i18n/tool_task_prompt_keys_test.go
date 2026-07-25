package i18n

import "testing"

func TestToolTaskPromptKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolTaskPromptKeys {
		for _, language := range AllLanguages() {
			if got := Text(language, key); got == "" || got == "["+string(key)+"]" {
				t.Errorf("Text(%s, %q) = %q", language.Code(), key, got)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}
