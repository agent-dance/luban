package i18n

import (
	"strings"
	"testing"
)

func TestToolWorktreePromptCatalogComplete(t *testing.T) {
	keys := []Key{
		KeyToolPromptWorktreeEnterDescription, KeyToolPromptWorktreeName,
		KeyToolPromptWorktreePath, KeyToolPromptWorktreeBaseRef,
		KeyToolPromptWorktreeExitDescription, KeyToolPromptWorktreeAction,
		KeyToolPromptWorktreeDiscardChanges,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			value := Text(lang, key)
			if value == "" || value == string(key) || strings.Contains(value, "%!") {
				t.Fatalf("invalid %s translation for %s: %q", lang, key, value)
			}
		}
	}
}
