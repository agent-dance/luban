package i18n

import "testing"

func TestHookRuntimeErrorKeysCoverEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyHookURLInvalid, KeyHookSchemeNotAllowed, KeyHookHostnameMissing, KeyHookDNSLookupFailed,
		KeyHookBlockedIP, KeyHookSSRFBlocked, KeyHookRedirectLimit, KeyHookRedirectBlocked,
		KeyHookRequestBuildFailed, KeyHookResponseReadFailed, KeyHookResponseTruncated, KeyHookAttemptsFailed,
		KeyHookConfigLegacyParse, KeyHookConfigMapParse, KeyHookConfigUnexpected,
		KeyHookConfigSettingsParse, KeyHookConfigEventInvalid, KeyHookConfigKindUnknown,
		KeyHookLifecycleApplyMissing, KeyHookLifecycleRollback, KeyHookLifecycleBlocked, KeyHookBlockedDefault,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}
