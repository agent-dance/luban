package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestTUIStoreCheckpointCatalogCoversEveryLanguage(t *testing.T) {
	for _, key := range tuiStoreCheckpointKeys {
		for _, lang := range AllLanguages() {
			copy := Text(lang, key)
			if copy == "" || strings.HasPrefix(copy, "[") {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, copy)
			}
		}
	}
}

func TestTUIStoreCheckpointEnglishContractsAndRawValues(t *testing.T) {
	cause := errors.New("raw-os-json-cause")
	checks := []struct {
		key  Key
		args []any
		want string
	}{
		{KeyTUIObservationRetainEvidenceIndex, []any{cause}, "retain observation evidence index: raw-os-json-cause"},
		{KeyTUIActivityStateLifecycleIncompatible, []any{"running", "completed"}, `activity state/outcome mismatch: state "running" is incompatible with lifecycle completed`},
		{KeyTUIActivityStateOutcomeIncompatible, []any{"running", "failed"}, `activity state/outcome mismatch: state "running" is incompatible with outcome failed`},
		{KeyTUISessionViewMarshalTranscript, []any{cause}, "marshal session view transcript: raw-os-json-cause"},
		{KeyTUISessionViewMarshalCheckpoint, []any{cause}, "marshal session view checkpoint: raw-os-json-cause"},
		{KeyTUISessionViewPrepareCheckpointDir, []any{cause}, "prepare session view checkpoint directory: raw-os-json-cause"},
		{KeyTUISessionViewCreateCheckpoint, []any{cause}, "create session view checkpoint: raw-os-json-cause"},
		{KeyTUISessionViewSecureCheckpoint, []any{cause}, "secure session view checkpoint: raw-os-json-cause"},
		{KeyTUISessionViewWriteCheckpoint, []any{cause}, "write session view checkpoint: raw-os-json-cause"},
		{KeyTUISessionViewSyncCheckpoint, []any{cause}, "sync session view checkpoint: raw-os-json-cause"},
		{KeyTUISessionViewCloseCheckpoint, []any{cause}, "close session view checkpoint: raw-os-json-cause"},
		{KeyTUISessionViewPublishCheckpoint, []any{cause}, "publish session view checkpoint: raw-os-json-cause"},
		{KeyTUISessionViewOpenCheckpoint, []any{cause}, "open session view checkpoint: raw-os-json-cause"},
		{KeyTUISessionViewDecodeCheckpointFile, []any{cause}, "decode session view checkpoint: raw-os-json-cause"},
		{KeyTUISessionViewTrailingCheckpointData, nil, "decode session view checkpoint trailing data"},
		{KeyCommandSkillInvokerNotConfigured, nil, "skill invoker is not configured"},
	}
	for _, check := range checks {
		if got := Format(LangEN, check.key, check.args...); got != check.want {
			t.Errorf("Format(LangEN, %q) = %q, want %q", check.key, got, check.want)
		}
		for _, lang := range AllLanguages() {
			got := Format(lang, check.key, check.args...)
			if strings.Contains(got, "%!") {
				t.Errorf("Format(%s, %q) has a formatting error: %q", lang.Code(), check.key, got)
			}
			if strings.Contains(check.want, cause.Error()) && !strings.Contains(got, cause.Error()) {
				t.Errorf("Format(%s, %q) omitted raw cause: %q", lang.Code(), check.key, got)
			}
		}
	}
}

func TestTUIStoreCheckpointErrorsPreserveCausesAndHideInternalSentinels(t *testing.T) {
	previous := detectedLanguageCache.Load()
	t.Cleanup(func() { detectedLanguageCache.Store(previous) })

	externalCause := errors.New("raw-filesystem-cause-41")
	external := WrapError(KeyTUISessionViewOpenCheckpoint, externalCause)
	if !errors.Is(external, externalCause) {
		t.Fatal("checkpoint error did not preserve its external cause")
	}
	detectedLanguageCache.Store(int32(LangZH))
	if got := external.Error(); !strings.Contains(got, externalCause.Error()) || strings.HasPrefix(got, "open session view") {
		t.Fatalf("checkpoint error was not localized with its raw cause: %q", got)
	}

	internalSentinel := errors.New("INTERNAL-ACTIVITY-SENTINEL-42")
	internal := WrapInternalError(KeyTUIActivityStateOutcomeIncompatible, internalSentinel, "running", "failed")
	if !errors.Is(internal, internalSentinel) {
		t.Fatal("activity error did not preserve errors.Is for its internal sentinel")
	}
	if got := internal.Error(); strings.Contains(got, internalSentinel.Error()) || !strings.Contains(got, "running") || !strings.Contains(got, "failed") {
		t.Fatalf("activity error leaked its internal sentinel or lost raw state values: %q", got)
	}
}
