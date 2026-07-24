package i18n

import (
	"strings"
	"testing"
)

func TestOAuthSemanticCopyCoversEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyOAuthInvalidState, KeyOAuthAuthorizationDenied, KeyOAuthMissingCode,
		KeyOAuthAuthorizationSuccess, KeyOAuthAuthenticationError,
		KeyOAuthAuthenticationSuccess,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || strings.HasPrefix(got, "[") {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}
