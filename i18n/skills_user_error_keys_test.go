package i18n

import "testing"

func TestSkillsUserErrorKeysCoverEveryLanguage(t *testing.T) {
	keys := []Key{
		KeySkillsUserErrorStoreUnavailable,
		KeySkillsUserErrorCatalogChanged,
		KeySkillsUserErrorInvalidIdentifier,
		KeySkillsUserErrorInvalidContent,
		KeySkillsUserErrorInvalidCatalogState,
		KeySkillsUserErrorInvalidVisibility,
		KeySkillsUserErrorInvalidPolicy,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if got == "" || got == "["+string(key)+"]" {
				t.Fatalf("%s is missing for %s", key, lang.Code())
			}
		}
	}
}
