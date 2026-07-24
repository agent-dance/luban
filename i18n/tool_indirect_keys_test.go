package i18n

import (
	"strings"
	"testing"
)

func TestToolIndirectKeysCoverEveryLanguageAndPreserveRawValues(t *testing.T) {
	keys := []Key{
		KeyToolWebSummariserUnavailable, KeyToolWebSummariserFailed, KeyToolWebNoModelResponse,
		KeyToolWebSearchError, KeyToolWebServerToolFailed, KeyToolMCPFormatJSONSchema, KeyToolMCPFormatJSON,
		KeyToolMCPFormatJSONArraySchema, KeyToolMCPFormatJSONArray, KeyToolMCPFormatPlainText,
		KeyToolMCPLargeOutputStored, KeyToolVerificationNudge, KeyToolWebRedirectMarker,
		KeyToolWebSearchResultsHeader, KeyToolWebSearchLinks, KeyToolWebSearchSourcesReminder,
		KeyToolWebSearchSourcesHeading, KeyToolMCPPaginationHint, KeyToolMCPTruncationHint,
		KeyToolMCPReadServerURIRequired, KeyToolMCPReadServerRequired, KeyToolMCPReadURIRequired,
		KeyToolMCPReadInvalidInput, KeyToolMCPReadNotConnected, KeyToolMCPReadNotConnectedCause,
		KeyToolMCPReadUnsupported, KeyToolMCPReadFailed, KeyToolMCPReadInvalidResult,
		KeyToolMCPReadMarshalResult, KeyToolMCPReadEncodeRequest, KeyToolMCPReadGenericError,
		KeyToolMCPReadHTTPResponse, KeyToolMCPReadOAuthRequired, KeyToolMCPReadInvalidJSONRPC,
		KeyToolMCPReadRPCFailed, KeyToolMCPReadMissingResult, KeyToolMCPDynamicUninitialized,
		KeyToolMCPReadContentURIRequired, KeyToolMCPReadInvalidBase64, KeyToolMCPReadBinarySaveFailed,
		KeyToolMCPUnsafeOutputPath,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Fatalf("%s is missing for %s", key, lang.Code())
			}
		}
	}
	got := Format(LangZH, KeyToolMCPLargeOutputStored, "1200", "/tmp/raw", "JSON", "/tmp/raw")
	if !strings.Contains(got, "/tmp/raw") || !strings.Contains(got, "JSON") {
		t.Fatalf("large-output text lost raw values: %q", got)
	}
}
