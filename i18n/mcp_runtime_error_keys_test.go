package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestMCPRuntimeErrorKeysCoverEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyMCPValidationInvalidJSON, KeyMCPValidationInvalidJSONCause,
		KeyMCPValidationMissingEnv, KeyMCPValidationSetEnv,
		KeyMCPValidationCommandEmpty, KeyMCPValidationSetField, KeyMCPValidationIDENameEmpty,
		KeyMCPValidationTransportInvalid,
		KeyMCPValidationUseTransport, KeyMCPValidationServerNameEmpty,
		KeyMCPValidationServerNameInvalid, KeyMCPValidationCallbackPort, KeyMCPValidationSetCallbackPort,
		KeyMCPValidationMetadataURL, KeyMCPValidationSetMetadataURL, KeyMCPValidationMetadataHTTPS,
		KeyMCPValidationUseMetadataHTTPS, KeyMCPValidationURLEmpty, KeyMCPValidationURLInvalid,
		KeyMCPValidationSetURL, KeyMCPTransportUnsupported, KeyMCPStdioCommandRequired,
		KeyMCPStdioStartFailed, KeyMCPSSEBaseURLRequired, KeyMCPHTTPBaseURLRequired,
		KeyMCPHTTPURLInvalid, KeyMCPHTTPSchemeInvalid, KeyMCPWebSocketURLRequired,
		KeyMCPWebSocketURLInvalid, KeyMCPWebSocketSchemeInvalid, KeyMCPOAuthMetadataURLInvalid,
		KeyMCPOAuthMetadataHTTPS, KeyMCPOAuthUnsupportedTransport, KeyMCPOAuthServerURLRequired,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}

func TestMCPValidationInvalidJSONCausePreservesAndLocalizesCause(t *testing.T) {
	cause := errors.New("raw-json-cause")
	err := WrapError(KeyMCPValidationInvalidJSONCause, cause)
	if !errors.Is(err, cause) {
		t.Fatal("wrapped invalid JSON error does not preserve its cause")
	}
	localizer, ok := err.(interface{ Localized(Language) string })
	if !ok {
		t.Fatalf("wrapped error type %T does not support explicit localization", err)
	}
	for _, lang := range AllLanguages() {
		got := localizer.Localized(lang)
		if !strings.Contains(got, "raw-json-cause") || got == "["+string(KeyMCPValidationInvalidJSONCause)+"]" {
			t.Fatalf("Localized(%s) = %q", lang.Code(), got)
		}
	}
}
