package i18n

import (
	"strings"
	"testing"
)

func TestSessionCommandKeysAreTranslatedForEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyCommandResumeLoadedMessages,
		KeyCommandRenameSucceeded,
		KeyCommandForkPickerUnavailable,
		KeyCommandDiffNotRepository,
		KeyCommandReviewAllChanges,
		KeyCommandPasteConfirm,
		KeyCommandMemoryOpened,
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
		KeyCommandSkillsEffectiveState,
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
		KeyCommandSkillsToggleCommitted,
		KeyCommandSkillsToggleStale,
		KeyCommandSkillsToggleUnknown,
		KeyCommandSkillsToggleReadOnly,
		KeyCommandSkillsToggleSession,
		KeyCommandSkillsTogglePersistFailed,
		KeyCommandSkillsToggleRolledBack,
		KeyCommandSkillsToggleDegraded,
		KeyCommandSkillsToggleRefreshFailed,
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
		{KeyCommandSkillsToggleUsage, []any{"enable"}},
		{KeyCommandSkillsAllToggled, []any{"Enabled", 2, "session test"}},
		{KeyCommandSkillsAlreadyToggled, []any{"review", "enabled", "session test"}},
		{KeyCommandSkillsToggled, []any{"Enabled", "review", "session test"}},
		{KeyCommandSkillsListHeader, []any{2, 1, 1, "session test"}},
		{KeyCommandSkillsListEntry, []any{"enabled", "review"}},
		{KeyCommandSkillsListSummary, []any{"summary"}},
		{KeyCommandSkillsListSource, []any{"project", "model + user"}},
		{KeyCommandSkillsListPath, []any{"/skills/review/SKILL.md"}},
		{KeyCommandSkillsDetailHeader, []any{"review"}},
		{KeyCommandSkillsDetailStatus, []any{"enabled", "session test"}},
		{KeyCommandSkillsDetailSummary, []any{"summary"}},
		{KeyCommandSkillsDetailSource, []any{"project"}},
		{KeyCommandSkillsDetailPath, []any{"/skills/review/SKILL.md"}},
		{KeyCommandSkillsDetailDirectory, []any{"/skills/review"}},
		{KeyCommandSkillsDetailModelInvoke, []any{"enabled"}},
		{KeyCommandSkillsDetailUserInvoke, []any{"enabled"}},
		{KeyCommandSkillsDetailContext, []any{"fork"}},
		{KeyCommandSkillsDetailModel, []any{"model-id"}},
		{KeyCommandSkillsDetailVersion, []any{"1.0"}},
		{KeyCommandSkillsDetailTools, []any{"Read, Grep"}},
		{KeyCommandSkillsDetailAliases, []any{"audit"}},
		{KeyCommandSkillsSession, []any{"test"}},
		{KeyCommandSkillsInvalidSelector, []any{"skill:bad"}},
		{KeyCommandSkillsAmbiguous, []any{"review"}},
		{KeyCommandSkillsAmbiguousCandidate, []any{"skill:project:id", "project", "/skill"}},
		{KeyCommandSkillsReadOnly, []any{"review", "skill:managed:id", "managed"}},
		{KeyCommandSkillsSetResult, []any{"review", "skill:project:id", "off", "project", "off", "project"}},
		{KeyCommandSkillsResetResult, []any{"review", "skill:project:id", "project", "auto", "default"}},
		{KeyCommandSkillsEffectiveState, []any{"manual-only", "project"}},
		{KeyCommandSkillsCatalogRevision, []any{7}},
		{KeyCommandSkillsListIdentity, []any{"skill:project:id", "project", "/skill"}},
		{KeyCommandSkillsListRevision, []any{"sha256:digest", 3}},
		{KeyCommandSkillsListPolicy, []any{"manual-only", "project", "yes", "none"}},
		{KeyCommandSkillsListShadowed, []any{"skill:project:winner"}},
		{KeyCommandSkillsDetailIdentity, []any{"skill:project:id", "project", "/skill"}},
		{KeyCommandSkillsDetailRevision, []any{"sha256:digest", 3, 7}},
		{KeyCommandSkillsDetailPolicy, []any{"manual-only", "project", "yes", "none"}},
		{KeyCommandSkillsToggleCommitted, []any{"review", "skill:project:id", "off"}},
		{KeyCommandSkillsToggleStale, []any{"skill:project:id"}},
		{KeyCommandSkillsToggleUnknown, []any{"skill:project:id"}},
		{KeyCommandSkillsToggleReadOnly, []any{"review", "skill:managed:id", "managed"}},
		{KeyCommandSkillsToggleSession, []any{"review", "skill:project:id", "skill:project:id"}},
		{KeyCommandSkillsTogglePersistFailed, []any{"review", "skill:project:id"}},
		{KeyCommandSkillsToggleRolledBack, []any{"review", "skill:project:id"}},
		{KeyCommandSkillsToggleDegraded, []any{"review", "skill:project:id"}},
		{KeyCommandSkillsToggleRefreshFailed, []any{"review", "skill:project:id"}},
	}
	for _, test := range tests {
		for _, lang := range AllLanguages() {
			if got := Format(lang, test.key, test.args...); strings.Contains(got, "%!") {
				t.Errorf("Format(%s, %q) = %q", lang.Code(), test.key, got)
			}
		}
	}
}
