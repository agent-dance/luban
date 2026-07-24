package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestLoopQueryErrorKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range loopQueryErrorKeys {
		for _, lang := range AllLanguages() {
			copy := Text(lang, key)
			if copy == "" || copy == "["+string(key)+"]" {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, copy)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestLoopQueryErrorFormattingAndCausePolicy(t *testing.T) {
	providerErr := errors.New("raw-provider-error")
	recoveryErr := errors.New("raw-recovery-error")
	for _, lang := range AllLanguages() {
		limit := Format(lang, KeyLoopQueryMessageHistoryLimitExceeded, 501, 500)
		if !strings.Contains(limit, "501") || !strings.Contains(limit, "500") || strings.Contains(limit, "%!") {
			t.Errorf("%s message-history limit copy lost its bounds: %q", lang.Code(), limit)
		}
		got := Format(lang, KeyLoopQueryAPICallRecoveryFailed, recoveryErr, providerErr)
		for _, raw := range []string{providerErr.Error(), recoveryErr.Error()} {
			if !strings.Contains(got, raw) {
				t.Errorf("%s omitted %q: %q", lang.Code(), raw, got)
			}
		}
		if strings.Contains(got, "%!") {
			t.Errorf("%s has a formatting error: %q", lang.Code(), got)
		}
	}

	internal := errors.New("internal catalog diagnostic")
	err := WrapInternalError(KeyLoopQuerySnapshotSkillCatalogFailed, internal)
	if !errors.Is(err, internal) {
		t.Fatal("internal query error did not preserve its cause")
	}
	if strings.Contains(err.Error(), internal.Error()) {
		t.Fatalf("internal query diagnostic leaked into user copy: %q", err)
	}
}
