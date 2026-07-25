package i18n

import (
	"strings"
	"testing"
)

func TestToolSearchPromptCatalogComplete(t *testing.T) {
	for _, key := range toolSearchPromptKeys {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if strings.TrimSpace(got) == "" || got == "["+string(key)+"]" {
				t.Fatalf("%s translation for %s = %q", lang.Code(), key, got)
			}
		}
	}
}
