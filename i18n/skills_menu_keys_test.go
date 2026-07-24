package i18n

import (
	"strings"
	"testing"
)

func TestSkillsMenuKeysCoverEveryLanguage(t *testing.T) {
	keys := []Key{
		KeySkillsMenuTitle, KeySkillsMenuHelp, KeySkillsMenuFilter,
		KeySkillsMenuLoading, KeySkillsMenuUpdating, KeySkillsMenuRefreshing,
		KeySkillsMenuStatusRefreshed, KeySkillsMenuEmpty,
		KeySkillsMenuNoMatches, KeySkillsMenuShowing, KeySkillsMenuDetailSummary,
		KeySkillsMenuDetailSource, KeySkillsMenuDetailPath, KeySkillsMenuDetailVisibilityScope,
		KeySkillsMenuDetailIdentity, KeySkillsMenuDetailShadowed, KeySkillsMenuDetailMutable, KeySkillsMenuDetailReadOnly,
		KeySkillsMenuReadOnlyUnspecified, KeySkillsMenuBackendUnavailable,
		KeySkillsMenuSessionUnavailable, KeySkillsMenuLoadFailed, KeySkillsMenuInvalidResult,
		KeySkillsMenuStatusStale, KeySkillsMenuStatusUnknown,
		KeySkillsMenuStatusSessionOverride, KeySkillsMenuStatusReadOnly,
		KeySkillsMenuStatusPersistenceFailed, KeySkillsMenuStatusRolledBack,
		KeySkillsMenuStatusDegraded, KeySkillsMenuStatusRefreshFailed,
		KeySkillsMenuStatusUnexpected,
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

func TestSkillsMenuFormatsForEveryLanguage(t *testing.T) {
	tests := []struct {
		key  Key
		args []any
	}{
		{KeySkillsMenuFilter, []any{"review"}},
		{KeySkillsMenuUpdating, []any{"review", "skill:project:review"}},
		{KeySkillsMenuStatusRefreshed, []any{8}},
		{KeySkillsMenuNoMatches, []any{"review"}},
		{KeySkillsMenuShowing, []any{1, 7, 12}},
		{KeySkillsMenuDetailSummary, []any{"summary"}},
		{KeySkillsMenuDetailSource, []any{"project"}},
		{KeySkillsMenuDetailPath, []any{"/skills/review/SKILL.md"}},
		{KeySkillsMenuDetailVisibilityScope, []any{"manual-only", "project"}},
		{KeySkillsMenuDetailIdentity, []any{"skill:project:review", 4}},
		{KeySkillsMenuDetailShadowed, []any{"skill:project:review"}},
		{KeySkillsMenuDetailReadOnly, []any{"managed policy"}},
		{KeySkillsMenuLoadFailed, []any{"failure"}},
		{KeySkillsMenuInvalidResult, []any{"failure"}},
		{KeySkillsMenuStatusStale, []any{"skill:project:review", 5}},
		{KeySkillsMenuStatusUnknown, []any{"skill:project:review"}},
		{KeySkillsMenuStatusSessionOverride, []any{"review", "skill:project:review", "/skills reset skill:project:review --scope session"}},
		{KeySkillsMenuStatusReadOnly, []any{"review", "skill:managed:review", "managed"}},
		{KeySkillsMenuStatusPersistenceFailed, []any{"review", "skill:project:review"}},
		{KeySkillsMenuStatusRolledBack, []any{"review", "skill:project:review"}},
		{KeySkillsMenuStatusDegraded, []any{"review", "skill:project:review"}},
		{KeySkillsMenuStatusRefreshFailed, []any{"review", "skill:project:review"}},
		{KeySkillsMenuStatusUnexpected, []any{"skill:project:review", "failure"}},
	}
	for _, test := range tests {
		for _, lang := range AllLanguages() {
			if got := Format(lang, test.key, test.args...); strings.Contains(got, "%!") {
				t.Errorf("Format(%s, %q) = %q", lang.Code(), test.key, got)
			}
		}
	}
}
