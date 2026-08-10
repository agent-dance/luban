package i18n

import (
	"strings"
	"testing"
)

func TestToolFrictionKeysAreCompleteAndKeepRawValuesParameterized(t *testing.T) {
	for _, key := range toolFrictionKeys {
		translations := semanticTranslations[key]
		for _, lang := range AllLanguages() {
			if strings.TrimSpace(translations[lang]) == "" {
				t.Fatalf("key %s missing %s translation", key, lang)
			}
		}
	}
	if got := Format(LangZH, KeyPresentationInspectPartialFailures, 2); !strings.Contains(got, "2") {
		t.Fatalf("partial failure count copy = %q", got)
	}
	if got := Format(LangZH, KeyToolInspectReadDirectory, "src/assets"); !strings.Contains(got, "src/assets") {
		t.Fatalf("directory diagnostic lost raw path: %q", got)
	}
	if got := Format(LangZH, KeyPresentationInspectPartialFailure, "root", "read", ".", "read_is_directory"); !strings.Contains(got, "root") || !strings.Contains(got, "read") || !strings.Contains(got, "read_is_directory") {
		t.Fatalf("partial diagnostic lost raw protocol values: %q", got)
	}
}
