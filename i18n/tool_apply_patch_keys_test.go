package i18n

import (
	"strings"
	"testing"
)

func TestToolApplyPatchKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolApplyPatchKeys {
		for _, lang := range AllLanguages() {
			value := Text(lang, key)
			if value == "" || value == "["+string(key)+"]" {
				t.Errorf("%s is missing for %s", key, lang.Code())
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestToolApplyPatchPromptPreservesProtocolIdentifiers(t *testing.T) {
	for _, lang := range AllLanguages() {
		description := Text(lang, KeyToolApplyPatchDescription)
		for _, token := range []string{"ApplyPatch", "*** Begin Patch", "unified diff", "hunk"} {
			if !strings.Contains(description, token) {
				t.Errorf("ApplyPatch description for %s omitted %q", lang.Code(), token)
			}
		}
	}
}
