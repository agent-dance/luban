package i18n

import (
	"strings"
	"testing"
)

func TestToolIndirectResidualKeysCoverEveryLanguageAndPreserveRuntimeValues(t *testing.T) {
	keys := []Key{
		KeyToolIndirectWorktreeKillTmux,
		KeyToolIndirectWorktreeWaitGitLocks,
		KeyToolIndirectWorktreeDeleteBranch,
		KeyToolIndirectWorktreeRemoveHookMissing,
		KeyToolIndirectWorktreeRemoveHookFailed,
		KeyToolIndirectPlanApprovalLeadOnly,
		KeyToolIndirectPlanApprovalCommit,
		KeyToolIndirectPlanApprovalModeRequired,
		KeyToolIndirectPlanApprovalPlanRequired,
		KeyToolIndirectPlanApprovalPrepareDir,
		KeyToolIndirectPlanApprovalPersist,
		KeyToolIndirectPlanApprovalTeamRequired,
		KeyToolIndirectPlanApprovalEncodeRequest,
		KeyToolIndirectPlanStateRequired,
		KeyToolIndirectPlanStateNotActive,
		KeyToolIndirectPlanStateRestoreMode,
		KeyToolIndirectPlanStateChangedDuringExit,
		KeyToolIndirectPlanStatePersistExitedState,
		KeyToolIndirectBashModeDeprecated,
		KeyToolIndirectBashModeUnknown,
		KeyToolIndirectBashModeNonReadForbidden,
		KeyToolIndirectBashModeDestructive,
		KeyToolIndirectBashModePattern,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if got == "" || got == "["+string(key)+"]" {
				t.Fatalf("%s is missing for %s", key, lang.Code())
			}
		}
	}

	for _, lang := range AllLanguages() {
		got := Format(lang, KeyToolIndirectWorktreeDeleteBranch, "feature/raw", "git-output-42")
		if !strings.Contains(got, "feature/raw") || !strings.Contains(got, "git-output-42") {
			t.Fatalf("worktree cleanup text lost raw values for %s: %q", lang.Code(), got)
		}
		got = Format(lang, KeyToolIndirectPlanStateRestoreMode, "acceptEdits", "cause-42")
		if !strings.Contains(got, "acceptEdits") || !strings.Contains(got, "cause-42") {
			t.Fatalf("plan-state text lost mode or cause for %s: %q", lang.Code(), got)
		}
		got = Format(lang, KeyToolIndirectBashModeDeprecated, "plan", "acceptEdits")
		if !strings.Contains(got, "plan") || !strings.Contains(got, "acceptEdits") {
			t.Fatalf("bash-mode text lost config identifiers for %s: %q", lang.Code(), got)
		}
	}
}

func TestToolIndirectResidualEnglishCompatibility(t *testing.T) {
	if got, want := Format(LangEN, KeyToolIndirectBashModeDeprecated, "plan", "acceptEdits"), `bash execution mode "plan" is deprecated and was renamed to "acceptEdits"; please update your config`; got != want {
		t.Fatalf("deprecated-mode English changed: got %q want %q", got, want)
	}
	if got, want := Format(LangEN, KeyToolIndirectWorktreeDeleteBranch, "feature", "raw git output"), `delete worktree branch "feature": raw git output`; got != want {
		t.Fatalf("worktree cleanup English changed: got %q want %q", got, want)
	}
}
