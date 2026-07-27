package i18n

import (
	"strings"
	"testing"
)

func TestVisibleToolCatalogKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range visibleToolCatalogKeys {
		for _, lang := range AllLanguages() {
			copy := Text(lang, key)
			if copy == "" || copy == "["+string(key)+"]" || strings.Contains(copy, "%!") {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, copy)
			}
		}
	}
	for _, lang := range AllLanguages() {
		formatted := Format(lang, KeyLoopQueryToolOutsideVisibleCatalog, "HiddenProbe")
		if !strings.Contains(formatted, "HiddenProbe") || strings.Contains(formatted, "%!") {
			t.Errorf("Format(%s, %q) = %q", lang.Code(), KeyLoopQueryToolOutsideVisibleCatalog, formatted)
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}
