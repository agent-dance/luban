package i18n

import "testing"

func TestEvidenceSemanticCopyCoversEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyEvidenceObservationHeader, KeyEvidenceInput, KeyEvidenceResultBoundary,
		KeyEvidenceStructured, KeyEvidenceObservationNotFound, KeyEvidenceEncodeInputError,
		KeyEvidenceReadResultError, KeyEvidenceReadStructuredError,
		KeyTranscriptMissingStore, KeyTranscriptUnsupportedFormat,
		KeyTranscriptObservationHeader, KeyTranscriptStructuredEvidence,
		KeyTranscriptPresentationHeader, KeyTranscriptDecisionHeader,
		KeyTranscriptRoleUser, KeyTranscriptRoleAssistant, KeyTranscriptRoleOther,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}
