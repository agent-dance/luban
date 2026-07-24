package i18n

import "testing"

func TestResultCompletenessSemanticKeysCoverAllLanguages(t *testing.T) {
	keys := []Key{
		KeyPresentationPaginationWarning,
		KeyPresentationSourceTruncatedWarning,
		KeyPresentationCaptureDroppedWarning,
		KeyPresentationDisplayPreviewWarning,
		KeyPresentationDisplayPreviewEvidence,
		KeyPresentationUnknownTruncationWarning,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == string(key) {
				t.Fatalf("Text(%s, %s) = %q", lang, key, got)
			}
		}
	}
}
