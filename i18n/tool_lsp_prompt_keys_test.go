package i18n

import "testing"

func TestToolLSPPromptKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolLSPPromptKeys {
		for _, language := range AllLanguages() {
			if got := Text(language, key); got == "" || got == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", language.Code(), key, got)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}
