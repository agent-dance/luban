package i18n

import (
	"strings"
	"testing"
)

func TestToolPowerShellCatalogComplete(t *testing.T) {
	keys := []Key{
		KeyToolPromptPowerShellDescription,
		KeyToolPromptPowerShellCommand,
		KeyToolPromptPowerShellTimeout,
		KeyToolPromptPowerShellSummary,
		KeyToolPromptPowerShellRunInBackground,
		KeyToolPermissionPowerShellDispatch,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			value := Text(lang, key)
			if value == "" || value == string(key) || strings.Contains(value, "%!") {
				t.Fatalf("invalid %s translation for %s: %q", lang, key, value)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}
