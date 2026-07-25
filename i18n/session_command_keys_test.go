package i18n

import (
	"strings"
	"testing"
)

func TestSessionCommandKeysAreTranslatedForEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyCommandResumeTransitionUnavailable,
		KeyCommandRenameSucceeded,
		KeyCommandForkPickerUnavailable,
		KeyCommandReviewAllChanges,
		KeyCommandSkillsListHeader,
		KeyCommandSkillsDescription,
		KeyCommandSkillsFullUsage,
		KeyCommandSkillsSetUsage,
		KeyCommandSkillsResetUsage,
		KeyCommandSkillsOperationFailed,
		KeyCommandSkillsInvalidSelector,
		KeyCommandSkillsAmbiguous,
		KeyCommandSkillsAmbiguousCandidate,
		KeyCommandSkillsReadOnly,
		KeyCommandSkillsSetResult,
		KeyCommandSkillsResetResult,
		KeyCommandSkillsReadOnlyManaged,
		KeyCommandSkillsReadOnlyDenied,
		KeyCommandSkillsCatalogRevision,
		KeyCommandSkillsListIdentity,
		KeyCommandSkillsListRevision,
		KeyCommandSkillsListPolicy,
		KeyCommandSkillsListShadowed,
		KeyCommandSkillsDetailIdentity,
		KeyCommandSkillsDetailRevision,
		KeyCommandSkillsDetailPolicy,
		KeyCommandSkillsMutableYes,
		KeyCommandSkillsMutableNo,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got[0] == '[' {
				t.Errorf("Text(%s, %q) = %q, want a registered translation", lang.Code(), key, got)
			}
		}
	}
}

func TestSkillsCommandKeysFormatForEveryLanguage(t *testing.T) {
	tests := []struct {
		key  Key
		args []any
	}{
		{KeyCommandSkillsOperationFailed, []any{"failure"}},
		{KeyCommandSkillsNotFound, []any{"review"}},
		{KeyCommandSkillsListHeader, []any{2, 1, 1, "session test"}},
		{KeyCommandSkillsListEntry, []any{"enabled", "review"}},
		{KeyCommandSkillsListSummary, []any{"summary"}},
		{KeyCommandSkillsDetailHeader, []any{"review"}},
		{KeyCommandSkillsDetailStatus, []any{"enabled", "session test"}},
		{KeyCommandSkillsDetailSummary, []any{"summary"}},
		{KeyCommandSkillsDetailPath, []any{"/skills/review/SKILL.md"}},
		{KeyCommandSkillsDetailDirectory, []any{"/skills/review"}},
		{KeyCommandSkillsDetailModelInvoke, []any{"enabled"}},
		{KeyCommandSkillsDetailUserInvoke, []any{"enabled"}},
		{KeyCommandSkillsDetailContext, []any{"fork"}},
		{KeyCommandSkillsDetailModel, []any{"model-id"}},
		{KeyCommandSkillsDetailVersion, []any{"1.0"}},
		{KeyCommandSkillsDetailTools, []any{"Read, Grep"}},
		{KeyCommandSkillsSession, []any{"test"}},
		{KeyCommandSkillsInvalidSelector, []any{"skill:bad"}},
		{KeyCommandSkillsAmbiguous, []any{"review"}},
		{KeyCommandSkillsAmbiguousCandidate, []any{"skill:project:id", "project", "/skill"}},
		{KeyCommandSkillsReadOnly, []any{"review", "skill:managed:id", "managed"}},
		{KeyCommandSkillsSetResult, []any{"review", "skill:project:id", "off", "project", "off", "project"}},
		{KeyCommandSkillsResetResult, []any{"review", "skill:project:id", "project", "auto", "default"}},
		{KeyCommandSkillsCatalogRevision, []any{7}},
		{KeyCommandSkillsListIdentity, []any{"skill:project:id", "project", "/skill"}},
		{KeyCommandSkillsListRevision, []any{"sha256:digest", 3}},
		{KeyCommandSkillsListPolicy, []any{"manual-only", "project", "yes", "none"}},
		{KeyCommandSkillsListShadowed, []any{"skill:project:winner"}},
		{KeyCommandSkillsDetailIdentity, []any{"skill:project:id", "project", "/skill"}},
		{KeyCommandSkillsDetailRevision, []any{"sha256:digest", 3, 7}},
		{KeyCommandSkillsDetailPolicy, []any{"manual-only", "project", "yes", "none"}},
	}
	for _, test := range tests {
		for _, lang := range AllLanguages() {
			if got := Format(lang, test.key, test.args...); strings.Contains(got, "%!") {
				t.Errorf("Format(%s, %q) = %q", lang.Code(), test.key, got)
			}
		}
	}
}
