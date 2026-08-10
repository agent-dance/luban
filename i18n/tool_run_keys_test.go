package i18n

import (
	"strings"
	"testing"
)

func TestToolRunKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolRunKeys {
		for _, language := range AllLanguages() {
			got := Text(language, key)
			if got == "" || got == "["+string(key)+"]" {
				t.Errorf("Text(%s, %q) = %q", language.Code(), key, got)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestToolRunFormattedCopyKeepsProtocolValues(t *testing.T) {
	for _, language := range AllLanguages() {
		permission := Format(language, KeyToolRunPermissionStep, "lint", "policy-code")
		if !strings.Contains(permission, "lint") || !strings.Contains(permission, "policy-code") || strings.Contains(permission, "%!") {
			t.Errorf("Format(%s, permission) = %q", language.Code(), permission)
		}
		step := Format(language, KeyToolRunStepResult, "test", "timed_out", -1, int64(42), true, "write")
		for _, raw := range []string{"test", "timed_out", "42", "write"} {
			if !strings.Contains(step, raw) {
				t.Errorf("Format(%s, step) omitted %q: %q", language.Code(), raw, step)
			}
		}
		if strings.Contains(step, "%!") {
			t.Errorf("Format(%s, step) has invalid formatting: %q", language.Code(), step)
		}
		seal := Format(language, KeyToolRunSealSafetyFailed, "verification_changed_source")
		if !strings.Contains(seal, "verification_changed_source") || strings.Contains(seal, "%!") {
			t.Errorf("Format(%s, seal safety) = %q", language.Code(), seal)
		}
	}
}

func TestToolRunBindingAndRecoveryCopyPreservesToolIDs(t *testing.T) {
	for _, language := range AllLanguages() {
		for _, key := range []Key{KeyToolRunSchemaRequiresPatchCommit, KeyToolRunSealReceiptMissing, KeyToolRunSealSafetyFailed} {
			copy := Text(language, key)
			for _, toolID := range []string{"ApplyPatch", "Run"} {
				if !strings.Contains(copy, toolID) {
					t.Errorf("Text(%s, %q) omitted %q: %q", language.Code(), key, toolID, copy)
				}
			}
		}
	}
	binding := Text(LangEN, KeyToolRunSchemaRequiresPatchCommit)
	for _, boundary := range []string{"one assistant response", "omission elsewhere"} {
		if !strings.Contains(binding, boundary) {
			t.Errorf("Run binding copy omitted %q: %q", boundary, binding)
		}
	}
	recovery := Text(LangEN, KeyToolRunSealReceiptMissing)
	for _, boundary := range []string{"no-op patch", "lacks adoption or sealing authority"} {
		if !strings.Contains(recovery, boundary) {
			t.Errorf("Run recovery copy omitted %q: %q", boundary, recovery)
		}
	}
}
