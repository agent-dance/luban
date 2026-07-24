package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestTUISessionStateErrorCatalogCoversEveryLanguage(t *testing.T) {
	for _, key := range tuiSessionStateErrorKeys {
		for _, lang := range AllLanguages() {
			copy := Text(lang, key)
			if copy == "" || strings.HasPrefix(copy, "[") {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, copy)
			}
		}
	}
}

func TestTUISessionStateErrorsPreserveEnglishContractsAndRawValues(t *testing.T) {
	cause := errors.New("raw-path-cause")
	checks := []struct {
		key  Key
		args []any
		want string
	}{
		{KeyTUISessionProjectionRetainToolResult, []any{3, 7, cause}, "retain persisted tool result at message 3 block 7: raw-path-cause"},
		{KeyTUISessionProjectionEncodeToolResult, []any{3, 7, cause}, "encode persisted tool result at message 3 block 7: raw-path-cause"},
		{KeyTUISessionProjectionRetainStructuredToolResult, []any{3, 7, cause}, "retain persisted structured tool result at message 3 block 7: raw-path-cause"},
		{KeyTUISessionProjectionEncodeBlock, []any{3, 7, cause}, "encode persisted block at message 3 block 7: raw-path-cause"},
		{KeyTUISessionProjectionRetainBlock, []any{3, 7, cause}, "retain persisted block at message 3 block 7: raw-path-cause"},
		{KeyTUISessionSnapshotEmptySessionID, nil, "session snapshot has empty session ID"},
		{KeyTUISessionSnapshotObservationEmptyID, nil, "session projection contains observation with empty ID"},
		{KeyTUISessionSnapshotDuplicateObservation, []any{"observation-17"}, `session projection contains duplicate observation "observation-17"`},
		{KeyTUISessionSnapshotRestoreActivities, []any{cause}, "restore session activities: raw-path-cause"},
		{KeyTUITranscriptSearchNotPrepared, nil, "transcript search is not prepared"},
		{KeyTUITranscriptSearchSessionChanged, nil, "session changed while searching transcript"},
		{KeyTUITranscriptSearchNotOpen, nil, "transcript search is not open"},
		{KeyTUIActivityNotFound, []any{"activity-17"}, `activity "activity-17" not found`},
		{KeyTUIStateObservationNotFound, []any{"observation-17"}, `observation "observation-17" not found`},
		{KeyTUIActivityStoreUnavailable, nil, "activity store is not initialized"},
		{KeyTUISessionViewInvalidCheckpoint, nil, "the session view checkpoint failed integrity validation"},
		{KeyTUISessionViewMissingCheckpoint, nil, "the session view checkpoint for the durable transcript is missing"},
		{KeyTUISessionViewUnsupportedVersion, []any{7, 2}, "session view checkpoint version 7 is not supported; expected version 2"},
		{KeyTUISessionViewIdentityMismatch, nil, "the session view checkpoint belongs to a different session"},
		{KeyTUISessionViewUnstableCheckpoint, nil, "the session view kept changing while its checkpoint was being created"},
		{KeyTUISessionViewStaleCapture, nil, "a stale session view capture cannot overwrite a newer checkpoint"},
		{KeyTUISessionViewMaterializeEvidence, []any{cause}, "retain session view evidence in its checkpoint: raw-path-cause"},
		{KeyTUISessionViewValidateEvidence, []any{cause}, "validate session view checkpoint evidence: raw-path-cause"},
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
			for _, raw := range []string{"raw-path-cause", "observation-17", "activity-17"} {
				if strings.Contains(check.want, raw) && !strings.Contains(got, raw) {
					t.Errorf("Format(%s, %q) omitted raw parameter %q: %q", lang.Code(), check.key, raw, got)
				}
			}
		}
	}
}

func TestTUISessionStateWrappedErrorsPreserveCause(t *testing.T) {
	cause := errors.New("raw-cause")
	err := WrapError(KeyTUISessionSnapshotRestoreActivities, cause)
	if !errors.Is(err, cause) {
		t.Fatal("wrapped TUI session error did not preserve its cause")
	}
	var target *semanticRuntimeError
	if !errors.As(err, &target) {
		t.Fatalf("wrapped TUI session error type = %T", err)
	}
}
