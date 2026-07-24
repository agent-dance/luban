package i18n

import "testing"

func TestTUISummarySemanticCopyCoversEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyTUIImageTag, KeyTUIStructuredEvidenceDecode, KeyTUIStructuredEvidenceEncode,
		KeyTUIObservationSummary, KeyTUIAdditionalLines, KeyTUIToolAdditionalLines,
		KeyTUIAgentTools, KeyTUIAgentTokens, KeyTUIAgentDuration,
		KeyTUIAgentCompleted, KeyTUIAgentBackgrounded, KeyTUITeammateSpawned,
		KeyTUIAgentOutput, KeyTUIAgentStatus,
		KeyTUIActivityAttempt, KeyTUIActivityParent, KeyTUIActivityProgress, KeyTUIActivityOccurrences,
		KeyTUISessionInputTotal, KeyTUISessionValueTotal, KeyTUISessionUsageUnknownCost, KeyTUISessionUsage,
		KeyTUICompactionRetainedRange, KeyTUICompactionDiscardedRange, KeyTUICompactionSummary, KeyTUICompactionTerminal,
		KeyTUICompactionTriggerManual, KeyTUICompactionTriggerAuto, KeyTUICompactionTriggerReactive,
		KeyTUICompactionTriggerSnip, KeyTUICompactionTriggerPartial, KeyTUICompactionTriggerUnknown,
		KeyTUIOutcomeRunning, KeyTUIOutcomeCompleted, KeyTUIOutcomeFailed, KeyTUIOutcomePartial,
		KeyTUIOutcomeDenied, KeyTUIOutcomeCancelled, KeyTUIOutcomeTimedOut,
		KeyTUIOutcomeConflict, KeyTUIOutcomeOrphan, KeyTUIOutcomeUnknown,
		KeyTUIConnectAPIKeyRequired, KeyTUIConnectSavingCredentials, KeyTUIConnectInlineUnavailable,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}

func TestTUISummaryLabelsUseRequestedLanguage(t *testing.T) {
	if got := Format(LangZH, KeyTUISessionValueTotal, "600", "3.0K"); got != "600（总计 3.0K）" {
		t.Fatalf("session value total = %q", got)
	}
	if got := TUIOutcomeLabel(LangZH, "timed_out"); got != "已超时" {
		t.Fatalf("timed_out = %q", got)
	}
	if got := TUICompactionTriggerLabel(LangJA, "partial_from"); got != "部分" {
		t.Fatalf("partial trigger = %q", got)
	}
}

func TestTUIAgentTokenMetricUsesIdiomaticLabels(t *testing.T) {
	wants := map[Language]string{
		LangEN: "42 tokens", LangZH: "42 个 Token", LangDE: "42 Token",
		LangJA: "42 トークン", LangKO: "토큰 42개", LangRU: "42 токенов",
	}
	for _, lang := range AllLanguages() {
		if got := Format(lang, KeyTUIAgentTokens, 42); got != wants[lang] {
			t.Errorf("Agent token metric in %s = %q, want %q", lang.Code(), got, wants[lang])
		}
	}
}
