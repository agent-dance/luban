package i18n

import (
	"errors"
	"strings"
	"testing"
)

type sdkResidualCause struct {
	detail string
}

func (e *sdkResidualCause) Error() string { return e.detail }

func TestSDKResidualCatalogCoversEveryLanguage(t *testing.T) {
	for _, key := range sdkResidualKeys {
		for _, lang := range AllLanguages() {
			if got := semanticTranslations[key][lang]; got == "" {
				t.Errorf("translation for %q in %s is missing", key, lang.Code())
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatalf("semantic catalog is incomplete: %v", err)
	}
}

func TestSDKResidualEnglishCopyRemainsCompatible(t *testing.T) {
	cause := errors.New("raw-cause")
	tests := []struct {
		key  Key
		args []any
		want string
	}{
		{KeySDKSessionsIDRequired, nil, "sdk/sessions: session ID must not be empty"},
		{KeySDKSessionsIDTooLong, []any{128}, "sdk/sessions: session ID too long (max 128 chars)"},
		{KeySDKSessionsIDInvalid, []any{"bad/id"}, `sdk/sessions: session ID "bad/id" contains invalid characters (only alphanumeric, hyphen, underscore allowed)`},
		{KeySDKSessionsListFailed, []any{cause}, "sdk/sessions: list sessions: raw-cause"},
		{KeySDKPermissionMarshalRequest, []any{cause}, "sdk: marshal permission request: raw-cause"},
		{KeySDKPermissionSendRequest, []any{cause}, "sdk: send permission request: raw-cause"},
	}

	for _, tt := range tests {
		if got := Format(LangEN, tt.key, tt.args...); got != tt.want {
			t.Errorf("English copy for %q = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestSDKResidualErrorsPreserveTypedCauseAndExplicitLanguage(t *testing.T) {
	cause := &sdkResidualCause{detail: "raw-os-detail"}
	err := WrapError(KeySDKSessionsListFailed, cause)
	if !errors.Is(err, cause) {
		t.Fatal("localized SDK error no longer preserves errors.Is")
	}
	var typed *sdkResidualCause
	if !errors.As(err, &typed) || typed != cause {
		t.Fatal("localized SDK error no longer preserves errors.As")
	}

	var localized interface {
		Localized(Language) string
	}
	if !errors.As(err, &localized) {
		t.Fatal("SDK error does not support explicit-language rendering")
	}
	got := localized.Localized(LangZH)
	if !strings.Contains(got, cause.detail) || strings.Contains(got, "list sessions") {
		t.Fatalf("Chinese SDK error did not localize its prefix while preserving the raw cause: %q", got)
	}
}
