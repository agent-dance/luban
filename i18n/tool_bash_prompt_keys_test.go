package i18n

import (
	"strings"
	"testing"
)

func TestToolBashPromptCatalogComplete(t *testing.T) {
	keys := []Key{
		KeyToolPromptBashDescription, KeyToolPromptBashCommand, KeyToolPromptBashTimeout,
		KeyToolPromptBashSummary, KeyToolPromptBashDisableSandbox, KeyToolPromptBashRunInBackground,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			value := Text(lang, key)
			if key == KeyToolPromptBashTimeout {
				value = Format(lang, key, 600000)
			}
			if value == "" || value == string(key) || strings.Contains(value, "%!") {
				t.Fatalf("invalid %s translation for %s: %q", lang, key, value)
			}
		}
	}
}
