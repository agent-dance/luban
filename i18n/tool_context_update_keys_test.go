package i18n

import "testing"

func TestToolContextUpdateKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolContextUpdateKeys {
		for _, lang := range AllLanguages() {
			if value := Text(lang, key); value == "" || value == "["+string(key)+"]" {
				t.Errorf("%s is missing for %s", key, lang.Code())
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}
