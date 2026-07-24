package i18n

import (
	"strings"
	"testing"
)

func TestStartupSemanticCopyCoversEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyStartupUnsupportedOutput, KeyStartupFatal, KeyStartupWarning,
		KeyStartupCredentialStoreWarning, KeyStartupOAuthStoreWarning,
		KeyStartupWorkingDirectoryFatal, KeyStartupNoActiveModel,
		KeyStartupSessionFatal, KeyStartupSandboxUnavailable,
		KeyStartupSafetyDenied, KeyStartupShutdownWarning, KeyStartupSDKError,
		KeyStartupResumeWarning, KeyStartupResumed, KeyStartupProviderMismatch,
		KeyStartupScreenReaderError, KeyStartupTUIError,
		KeyPrintQueryRequired, KeyStartupResolveSessionWarning,
		KeyStartupLatestSessionWarning, KeyStartupLoadSessionMetadata,
		KeyStartupResolveLatestSession,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || strings.HasPrefix(got, "[") {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}
