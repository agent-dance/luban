package i18n

import "testing"

func TestPermissionPolicyKeysCoverEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyPermissionModeAllowAllFrozen, KeyPermissionDenyDisallowedTool, KeyPermissionDenyNotAllowedTool,
		KeyPermissionDenySnapshotRule, KeyPermissionTestingApprovalRequired, KeyPermissionTestingPolicy,
		KeyPermissionDenyTestingPrompt, KeyPermissionDenySnapshotAsk, KeyPermissionAskAlwaysPolicy,
		KeyPermissionDenyAskAlways, KeyPermissionDenyRule, KeyPermissionRuleFallback,
		KeyPermissionAdvisoryPolicy, KeyPermissionMandatoryPolicy, KeyPermissionApprovalRequired,
		KeyPermissionSnapshotSource, KeyPermissionConfiguredRule, KeyPermissionConfiguredPatternRule,
		KeyPermissionSafetyProtectedPath, KeyPermissionSafetyUnavailable, KeyPermissionSafetyDangerousCommand,
		KeyPermissionSafetyShellProtectedPath, KeyPermissionSafetyPowerShell, KeyPermissionPreviewSendMessage,
		KeyPermissionPreviewSendTarget, KeyPermissionEnvironmentRoot,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}
