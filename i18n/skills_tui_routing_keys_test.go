package i18n

import (
	"strings"
	"testing"
)

func TestSkillsTUIRoutingKeysCoverEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyTUISkillsMenuUnavailable,
		KeyTUISkillsMenuOpenFailed,
		KeyTUISkillsInvalidSelector,
		KeyTUISkillsBackendUnavailable,
		KeyTUISkillsSnapshotFailed,
		KeyTUISkillsNotFound,
		KeyTUISkillsAmbiguous,
		KeyTUISkillsUnavailable,
		KeyTUISkillsInvokerUnavailable,
		KeyTUISkillsInvocationFailed,
		KeyTUISkillsInvocationRejected,
		KeyTUISkillsEmptyEnvelope,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || strings.HasPrefix(got, "[") {
				t.Errorf("Text(%s, %q) = %q, want registered translation", lang.Code(), key, got)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatalf("semantic catalog is incomplete: %v", err)
	}
}

func TestSkillsTUIRoutingFormatsForEveryLanguage(t *testing.T) {
	tests := []struct {
		key  Key
		args []any
	}{
		{KeyTUISkillsMenuOpenFailed, []any{"failure"}},
		{KeyTUISkillsInvalidSelector, []any{"skill:bad"}},
		{KeyTUISkillsSnapshotFailed, []any{"failure"}},
		{KeyTUISkillsNotFound, []any{"review"}},
		{KeyTUISkillsAmbiguous, []any{"review", "skill:project:a, skill:user:b"}},
		{KeyTUISkillsUnavailable, []any{"review"}},
		{KeyTUISkillsInvocationFailed, []any{"review", "failure"}},
		{KeyTUISkillsInvocationRejected, []any{"review"}},
		{KeyTUISkillsEmptyEnvelope, []any{"review"}},
	}
	for _, test := range tests {
		for _, lang := range AllLanguages() {
			if got := Format(lang, test.key, test.args...); strings.Contains(got, "%!") {
				t.Errorf("Format(%s, %q) = %q", lang.Code(), test.key, got)
			}
		}
	}
}
