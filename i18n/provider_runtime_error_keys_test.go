package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestProviderRuntimeErrorKeysCoverEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyProviderUnconfigured, KeyProviderUnconfiguredAction, KeyProviderDisconnected,
		KeyProviderDisconnectedAction, KeyProviderThinkingUnsupported,
		KeyProviderCustomToolsUnsupported, KeyProviderCustomToolDefinitionInvalid,
		KeyProviderRetryExceededWithoutCause, KeyProviderRetryExceededWithCause,
		KeyProviderUnknown, KeyProviderBedrockInvalidBaseURL,
		KeyProviderVertexProjectRequired, KeyProviderVertexADCCredentialsFailed,
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

func TestProviderRetryExceededWithCausePreservesRuntimeDetails(t *testing.T) {
	cause := errors.New("raw upstream failure")
	err := WrapError(KeyProviderRetryExceededWithCause, cause, 2)
	if !errors.Is(err, cause) {
		t.Fatal("retry error did not preserve its external cause")
	}
	localizer, ok := err.(interface{ Localized(Language) string })
	if !ok {
		t.Fatalf("retry error = %T, want runtime-localized error", err)
	}
	for _, lang := range AllLanguages() {
		got := localizer.Localized(lang)
		if !strings.Contains(got, "2") || !strings.Contains(got, cause.Error()) {
			t.Fatalf("Localized(%s) = %q; actual retry count or raw cause was lost", lang.Code(), got)
		}
	}
}
