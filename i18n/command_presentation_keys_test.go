package i18n

import "testing"

func TestCommandPresentationSemanticCopyCoversEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyCommandPresentationWait, KeyCommandPresentationInspectResult,
		KeyCommandPresentationInspectError, KeyCommandPresentationExitRequested,
		KeyCommandPresentationCompleted, KeyCommandPresentationResult,
		KeyCommandPresentationExtensionSuccess, KeyCommandPresentationExtensionFailure,
		KeyCommandPresentationMCPPromptDescription,
		KeyCommandPresentationMCPPromptSuccess, KeyCommandPresentationMCPPromptFailure,
		KeyCommandPresentationModelSaveWarning, KeyCommandPresentationProviderWarning,
		KeyCommandOutcomeSucceeded, KeyCommandOutcomeWarning, KeyCommandOutcomePartial,
		KeyCommandOutcomeFailed, KeyCommandOutcomeDenied, KeyCommandOutcomeCancelled,
		KeyCommandOutcomeTimedOut, KeyCommandOutcomeInterrupted,
		KeyCommandOutcomeExitRequested, KeyCommandOutcomeUnknown,
		KeyCommandDisplayReceipt, KeyCommandDisplayInspector, KeyCommandDisplayEvidence,
		KeyCommandDisplayDecision, KeyCommandRiskUnknown, KeyCommandRiskLow,
		KeyCommandRiskMedium, KeyCommandRiskHigh, KeyCommandRiskDestructive,
	}
	for _, pair := range commandPresentationNextKeys {
		keys = append(keys, pair[:]...)
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}

func TestCommandPresentationLabelsAreLocalized(t *testing.T) {
	if got := CommandOutcomeLabel(LangZH, "failed"); got != "失败" {
		t.Fatalf("failed label = %q", got)
	}
	if got := CommandRiskLabel(LangJA, "high"); got != "高" {
		t.Fatalf("high risk label = %q", got)
	}
}
