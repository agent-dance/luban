package i18n

import "testing"

func TestDiagnosticLogKeysCoverEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyLogHookUnknownEvent, KeyLogSDKPermissionMode,
		KeyLogDebugSessionStarted, KeyLogDebugUnknownPhase, KeyLogDebugPayloadNil,
		KeyLogDebugMarshalFailed, KeyLogAnthropicRequestError, KeyLogAnthropicNormalizeError,
		KeyLogAnthropicBodyOmitted, KeyLogAnthropicBodySniff, KeyLogAnthropicNormalizedGzip,
		KeyLogAnthropicNormalizedZlib,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}
