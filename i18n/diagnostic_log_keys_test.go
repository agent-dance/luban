package i18n

import "testing"

func TestDiagnosticLogKeysCoverEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyLogMCPStartFailed, KeyLogMCPStarted, KeyLogMCPSigtermFailed, KeyLogMCPShutdownTimeout,
		KeyLogMCPStopped, KeyLogMCPRestarted, KeyLogMCPServerNotFound, KeyLogMCPHealthStopped,
		KeyLogMCPHealthLoopStarted, KeyLogMCPHealthLoopStopped, KeyLogMCPHealthyAgain,
		KeyLogMCPPingFailed, KeyLogMCPMarkedUnhealthy, KeyLogMCPReconnectNotFound,
		KeyLogMCPReconnectLoopStarted, KeyLogMCPReconnectLoopStopped, KeyLogMCPUnexpectedExit,
		KeyLogMCPStableReset, KeyLogMCPRestartAttempt, KeyLogMCPRestartSucceeded,
		KeyLogMCPRestartFailed, KeyLogMCPReconnectExhausted, KeyLogMCPStreamReconnect,
		KeyLogMCPUnparseableEvent, KeyLogHookUnknownEvent, KeyLogTmuxBorderStatusFailed,
		KeyLogTmuxBorderFormatFailed, KeyLogSDKSessionStatFailed, KeyLogSDKSessionPartialRead,
		KeyLogSDKSessionDeleted, KeyLogSDKPermissionMode,
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
