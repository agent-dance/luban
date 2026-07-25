package i18n

import (
	"strings"
	"testing"
)

func TestCompactVisibleKeysCoverEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyCompactSummaryHeading,
		KeyCompactContinuationPreamble,
		KeyCompactTranscriptRecovery,
		KeyCompactTranscriptUnavailable,
		KeyCompactRecentMessagesPreserved,
		KeyCompactContinueDirective,
		KeyCompactPartialLaterPreamble,
		KeyCompactPartialTranscriptRecovery,
		KeyCompactPartialTranscriptUnavailable,
		KeyCompactEarlierMessagesPreserved,
		KeyCompactAttachmentPlanTitle,
		KeyCompactAttachmentPlanFile,
		KeyCompactAttachmentPlanModeTitle,
		KeyCompactAttachmentPlanModeBody,
		KeyCompactAttachmentSkillsTitle,
		KeyCompactAttachmentBackgroundTitle,
		KeyCompactAttachmentUnknownStatus,
		KeyCompactAttachmentTypeLabel,
		KeyCompactAttachmentErrorLabel,
		KeyCompactAttachmentDeferredTitle,
		KeyCompactAttachmentLoadedTools,
		KeyCompactAttachmentDeferredPool,
		KeyCompactAttachmentAgentTitle,
		KeyCompactAttachmentMCPTitle,
		KeyCompactAttachmentMCPToolsLabel,
		KeyCompactAttachmentMCPInstructionsLabel,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Fatalf("%s is missing for %s", key, lang.Code())
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatalf("semantic catalog is incomplete: %v", err)
	}
}

func TestCompactVisibleCopyPreservesRawPathsAndEnglishContract(t *testing.T) {
	const transcript = "/tmp/session/transcript.jsonl"
	if got := Format(LangEN, KeyCompactTranscriptRecovery, transcript); got != "If you need specific details from before compaction (like exact code snippets, error messages, or content you generated), read the full transcript at: "+transcript {
		t.Fatalf("English transcript guidance changed: %q", got)
	}
	for _, lang := range AllLanguages() {
		got := Format(lang, KeyCompactTranscriptRecovery, transcript)
		if !strings.Contains(got, transcript) {
			t.Fatalf("%s transcript guidance lost raw path: %q", lang.Code(), got)
		}
	}
	if got := Text(LangZH, KeyCompactContinuationPreamble); got == Text(LangEN, KeyCompactContinuationPreamble) {
		t.Fatalf("Chinese continuation preamble was not localized: %q", got)
	}
}
