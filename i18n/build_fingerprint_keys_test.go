package i18n

import (
	"strings"
	"testing"
)

func TestBuildFingerprintKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range buildFingerprintKeys {
		for _, language := range AllLanguages() {
			if got := Text(language, key); got == "" || got == string(key) {
				t.Fatalf("missing build fingerprint translation for %q in %s", key, language)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatalf("semantic catalog: %v", err)
	}
	for _, language := range AllLanguages() {
		formatted := Format(language, KeyBuildFingerprintDetail,
			"VERSION_SENTINEL", "REVISION_SENTINEL", "STATE_SENTINEL", "BUILD_TIME_SENTINEL",
			"START_TIME_SENTINEL", "EXECUTABLE_SENTINEL", "HEAD_SENTINEL")
		if strings.Contains(formatted, "%!") {
			t.Fatalf("malformed build detail format for %s: %q", language, formatted)
		}
		for _, sentinel := range []string{"VERSION_SENTINEL", "REVISION_SENTINEL", "START_TIME_SENTINEL", "EXECUTABLE_SENTINEL", "HEAD_SENTINEL"} {
			if !strings.Contains(formatted, sentinel) {
				t.Fatalf("build detail for %s dropped %s: %q", language, sentinel, formatted)
			}
		}
	}
}
