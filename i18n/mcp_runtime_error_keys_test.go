package i18n

import "testing"

func TestMCPRuntimeErrorKeysCoverEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyMCPValidationInvalidJSON, KeyMCPValidationMissingEnv, KeyMCPValidationSetEnv,
		KeyMCPValidationCommandEmpty, KeyMCPValidationSetField, KeyMCPValidationIDENameEmpty,
		KeyMCPValidationNameEmpty, KeyMCPValidationIDEmpty, KeyMCPValidationTransportInvalid,
		KeyMCPValidationUseTransport, KeyMCPValidationServerNameEmpty, KeyMCPValidationServerNameReserved,
		KeyMCPValidationServerNameInvalid, KeyMCPValidationCallbackPort, KeyMCPValidationSetCallbackPort,
		KeyMCPValidationMetadataURL, KeyMCPValidationSetMetadataURL, KeyMCPValidationMetadataHTTPS,
		KeyMCPValidationUseMetadataHTTPS, KeyMCPValidationURLEmpty, KeyMCPValidationURLInvalid,
		KeyMCPValidationSetURL, KeyMCPIntegrationUnsupported, KeyMCPIntegrationUnsupportedTransport,
		KeyMCPIntegrationUnsupportedServer, KeyMCPIntegrationUnavailable,
		KeyMCPIntegrationUnavailableInBuild, KeyMCPIntegrationFactoryNil, KeyMCPSDKBridgeMissing,
		KeyMCPNotIDETransport, KeyMCPTransportUnsupported, KeyMCPStdioCommandRequired,
		KeyMCPStdioStartFailed, KeyMCPStdioPipeFailed, KeyMCPSSEBaseURLRequired, KeyMCPHTTPBaseURLRequired,
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
