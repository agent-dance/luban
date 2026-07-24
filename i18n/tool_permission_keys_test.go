package i18n

import "testing"

func TestToolPermissionKeysCoverEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyToolPermissionInvalidPath, KeyToolPermissionReadDenied, KeyToolPermissionReadRequired,
		KeyToolPermissionReadUNC, KeyToolPermissionReadBinary, KeyToolPermissionReadDevice,
		KeyToolPermissionWritePlanMode, KeyToolPermissionEditPlanMode, KeyToolPermissionNotebookPlanMode,
		KeyToolPermissionModifyRequired, KeyToolPermissionOutsideDirectories, KeyToolPermissionWriteDenied,
		KeyToolPermissionWriteProtected, KeyToolPermissionWritePending, KeyToolPermissionSearchDenied,
		KeyToolPermissionSearchRequired, KeyToolPermissionWebFetchDenied, KeyToolPermissionWebFetchPending,
		KeyToolPermissionWebDomainBlocked, KeyToolPermissionWebDomainNotAllowed, KeyToolPermissionWebInvalidURL,
		KeyToolPermissionWebSearchRequired, KeyToolPermissionExitPlanNotActive, KeyToolPermissionExitPlanConfirm,
		KeyToolPermissionAgentSpawn, KeyToolPermissionBashPlanMode, KeyToolPermissionBashRuleDenied,
		KeyToolPermissionBashRuleApproval, KeyToolPermissionBashGenericApproval,
		KeyToolPermissionBashDestructiveApproval, KeyToolPermissionBashProcessSubstitution,
		KeyToolPermissionBashMultipleDirectories, KeyToolPermissionBashCDAndGit,
		KeyToolPermissionBashCDAndRedirect, KeyToolPermissionBashBareGit, KeyToolPermissionBashGitInternal,
		KeyBashSecurityRemotePipeShell, KeyBashSecurityEncodedPipeShell, KeyBashSecurityEncodedSubstitution,
		KeyBashSecurityDynamicEval, KeyBashSecurityReverseShell, KeyBashSecurityScriptPayload,
		KeyBashSecurityHistoryTampering, KeyBashSecurityObfuscatedPayload, KeyBashSecuritySSHInlineShell,
		KeyBashSecurityRecursiveDelete, KeyBashSecurityForkBomb, KeyBashSecurityChmodSetuid,
		KeyBashSecurityChmodWorldWritable, KeyBashSecurityDownloadSubstitution, KeyBashSecurityRawDiskWrite,
		KeyBashSecurityFilesystemFormat, KeyBashSecurityPowerOperation, KeyBashSecurityCrontabRemoval,
		KeyBashSecurityFirewallFlush, KeyBashSecurityPermissionLockout, KeyBashSecurityCriticalService,
		KeyBashSecurityPrivilegedUserDelete, KeyBashSecurityDiskRepartition,
		KeyBashSecurityCompoundMultipleCD, KeyBashSecurityCompoundCrossPipeWrite,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got[0] == '[' {
				t.Errorf("Text(%s, %q) = %q, want registered translation", lang.Code(), key, got)
			}
		}
	}
}

func TestToolPermissionRepresentativeCopyIsLocalized(t *testing.T) {
	for _, key := range []Key{
		KeyToolPermissionReadDenied,
		KeyToolPermissionBashDestructiveApproval,
		KeyBashSecurityRawDiskWrite,
	} {
		if en, zh := Text(LangEN, key), Text(LangZH, key); en == zh {
			t.Errorf("%q has identical English and Chinese copy: %q", key, en)
		}
	}
}
