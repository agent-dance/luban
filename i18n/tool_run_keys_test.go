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
	}
}
