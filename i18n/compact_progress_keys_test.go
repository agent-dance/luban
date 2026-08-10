package i18n

import (
	"strings"
	"testing"
)

func TestCompactProgressSemanticCopyCoversEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyTUICompactProgressTitle,
		KeyTUICompactProgressPreparing,
		KeyTUICompactProgressSummarizing,
		KeyTUICompactProgressInstalling,
		KeyTUICompactProgressPersisting,
		KeyTUICompactProgressElapsedCancel,
		KeyTUICompactProgressInputQueues,
		KeyTUICompactProgressInputQueued,
		KeyTUICompactProgressCompleted,
		KeyTUICompactProgressCompletedNoCounts,
		KeyTUICompactProgressFailed,
		KeyTUICompactProgressCancelled,
		KeyTUICompactProgressCause,
		KeyTUICompactProgressProviderCalibration,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}

func TestCompactProgressChineseCopyExplainsQueueCancellationAndCalibration(t *testing.T) {
	if got := Format(LangZH, KeyTUICompactProgressElapsedCancel, "01:24"); !strings.Contains(got, "Esc") || !strings.Contains(got, "01:24") {
		t.Fatalf("elapsed/cancel copy = %q", got)
	}
	if got := Format(LangZH, KeyTUICompactProgressInputQueued, 2); !strings.Contains(got, "2") || !strings.Contains(got, "排队") {
		t.Fatalf("queued copy = %q", got)
	}
	if got := Text(LangZH, KeyTUICompactProgressProviderCalibration); !strings.Contains(got, "provider") || !strings.Contains(got, "校准") {
		t.Fatalf("provider calibration copy = %q", got)
	}
}
