package i18n

import "testing"

func TestProviderRuntimeErrorKeysCoverEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyProviderUnconfigured, KeyProviderUnconfiguredAction, KeyProviderDisconnected,
		KeyProviderDisconnectedAction, KeyProviderThinkingUnsupported,
		KeyProviderCustomToolsUnsupported, KeyProviderCustomToolDefinitionInvalid,
		KeyProviderRetryExceededWithoutCause, KeyProviderUnknown, KeyProviderBedrockInvalidBaseURL,
		KeyProviderVertexProjectRequired, KeyProviderVertexAPIKeyRequired, KeyProviderVertexBaseURLRequired,
		KeyProviderVertexEndpointInvalid, KeyCredentialHomeFailed, KeyCredentialReadFailed,
		KeyCredentialDecodeFailed, KeyCredentialDirectoryFailed, KeyCredentialEncodeFailed,
		KeyCredentialTempCreateFailed, KeyCredentialTempWriteFailed, KeyCredentialPermissionsFailed,
		KeyCredentialTempCloseFailed, KeyCredentialReplaceFailed, KeyProviderDebugOpenFailed,
		KeyProviderDebugWriting, KeyProviderBedrockConfigInvalid, KeyProviderBedrockAWSConfigFailed,
		KeyProviderRequestEncodeFailed, KeyProviderRequestBuildFailed, KeyProviderRequestFailed,
		KeyProviderStreamCreateFailed, KeyProviderAnthropicUnavailable, KeyProviderTokenCountInvalid,
		KeyProviderToolsConvertFailed, KeyProviderServerToolsConvertFailed,
		KeyProviderToolSchemaEncodeFailed, KeyProviderToolSchemaDecodeFailed,
		KeyProviderServerToolNameInvalid, KeyProviderServerToolDomainsConflict,
		KeyProviderServerToolMaxUsesInvalid, KeyProviderServerToolTypeUnsupported,
		KeyProviderDebugPathEmpty, KeyProviderDebugFileOpenFailed, KeyProviderDebugPermissionsFailed,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}
