package i18n

import (
	"strings"
	"testing"
)

func TestServicesMCPRemainingCatalogCoversEveryLanguage(t *testing.T) {
	for _, key := range servicesMCPRemainingKeys {
		for _, lang := range AllLanguages() {
			copy := Text(lang, key)
			if copy == "" || strings.HasPrefix(copy, "[") {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, copy)
			}
		}
	}
}

func TestServicesMCPMemoryTokenStoreErrorKeepsEnglishCompatibility(t *testing.T) {
	const want = "services/mcp: nil memory token store"
	if got := Text(LangEN, KeyServicesMCPOAuthMemoryTokenStoreNil); got != want {
		t.Fatalf("English copy = %q, want %q", got, want)
	}
}
