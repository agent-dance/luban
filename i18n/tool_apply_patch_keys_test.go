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

func TestToolApplyPatchParseAndPermissionDiagnosticsRemainDistinct(t *testing.T) {
	for _, lang := range AllLanguages() {
		parseFailure := Format(lang, KeyToolApplyPatchParseFailed, "invalid_hunk_line", "target.go")
		permissionDenied := Format(lang, KeyToolApplyPatchPermissionDenied, "target.go", "denied_rule")
		permissionInvalid := Format(lang, KeyToolApplyPatchPermissionInvalid, "invalid_root")
		for name, value := range map[string]string{
			"parse":              parseFailure,
			"permission_denied":  permissionDenied,
			"permission_invalid": permissionInvalid,
		} {
			if strings.Contains(value, "%!") {
				t.Errorf("%s diagnostic for %s has invalid formatting: %q", name, lang.Code(), value)
			}
		}
		if !strings.Contains(parseFailure, "invalid_hunk_line") || !strings.Contains(parseFailure, "target.go") {
			t.Errorf("parse diagnostic for %s lost protocol details: %q", lang.Code(), parseFailure)
		}
		if parseFailure == permissionDenied || parseFailure == permissionInvalid {
			t.Errorf("parse diagnostic for %s collapsed into permission semantics: %q", lang.Code(), parseFailure)
		}
	}
}
