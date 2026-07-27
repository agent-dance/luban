package i18n

import (
	"strings"
	"testing"
)

func TestLoopFlightKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range loopFlightKeys {
		for _, lang := range AllLanguages() {
			copy := Text(lang, key)
			if copy == "" || copy == "["+string(key)+"]" || strings.Contains(copy, "%!") {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, copy)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}
