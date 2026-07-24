package i18n

import "testing"

func TestRuntimeErrorPublicSummaryCoversEveryLanguage(t *testing.T) {
	for _, lang := range AllLanguages() {
		if got := Text(lang, KeyRuntimeErrorPublicSummary); got == "" || got == string(KeyRuntimeErrorPublicSummary) {
			t.Fatalf("missing public runtime-error summary for %s", lang)
		}
	}
}
