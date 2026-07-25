package i18n

import "testing"

func TestAuxRuntimeKeysCoverEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyAuxCompactNotEnoughMessages, KeyAuxCompactInterrupted,
		KeyAuxCompactConversationLong, KeyAuxCompactSummaryMissing, KeyAuxCompactCancelled,
		KeyAuxCompactFailed, KeyAuxCompactEmptyToolResult, KeyAuxCompactOutputTooLarge,
		KeyAuxCompactPreview, KeyAuxCompactBytes, KeyAuxCompactTruncated,
		KeyAuxCompactBudgetTruncated,
		KeyAuxCompactInvalidDirection, KeyAuxCompactEmptyHistory, KeyAuxCompactInvalidPivot,
		KeyAuxCompactNothingBefore, KeyAuxCompactNothingAfter, KeyAuxCompactPreserveNone,
		KeyAuxClipboardUnsupported, KeyAuxClipboardCreateTemp, KeyAuxClipboardReadTemp,
		KeyAuxClipboardMissingReference, KeyAuxClipboardReferenceNotImage, KeyAuxClipboardReadImage,
		KeyAuxClipboardLinuxUnavailable, KeyAuxClipboardPowerShellFailed,
		KeyAuxHookBlockedContinuation, KeyAuxHookPreventedContinuation, KeyAuxHookExecutionFailed,
		KeyAuxToolHookBlocked, KeyAuxToolHookPrevented, KeyAuxHookNamedFeedback,
		KeyAuxHookNamedBlocked, KeyAuxPostSamplingBlocked, KeyAuxPostSamplingBlockedReason,
		KeyAuxSessionNoSessions, KeyAuxSessionDeleted, KeyAuxSessionNotFound,
		KeyAuxSessionAmbiguous, KeyAuxSessionFailed,
		KeyAuxEngineSessionNotFound, KeyAuxEngineSessionDeleted, KeyAuxEngineShutdown,
		KeyAuxEngineNoProvider,
		KeyAuxSwarmTeamNotFound, KeyAuxSwarmInvalidName, KeyAuxSwarmMailboxFailed, KeyAuxSwarmFailed,
		KeyAuxSkillNotFound, KeyAuxSkillRevisionConflict, KeyAuxSkillManagedReadOnly,
		KeyAuxSkillInvalidScope, KeyAuxSkillInvalidSession, KeyAuxSkillFailed,
		KeyAuxMCPPromptDescription,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}
