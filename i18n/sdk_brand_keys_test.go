package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestSDKBrandKeysCoverEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyBrandTagline,
		KeySDKReady,
		KeySDKSystemPromptReceived,
		KeySDKStdinReadError,
		KeySDKInvalidJSONEnvelope,
		KeySDKUnknownMessageType,
		KeySDKParseUserMessage,
		KeySDKUserUUIDRequired,
		KeySDKExtractMessageText,
		KeySDKUnsupportedMessageContent,
		KeySDKStreamEndedWithoutFinalEvent,
		KeySDKQueryCancelled,
		KeySDKQueryAlreadyActive,
		KeySDKParseControlRequest,
		KeySDKControlRequestIDRequired,
		KeySDKControlRequestIDConflict,
		KeySDKParseRequestSubtype,
		KeySDKUnsupportedControlSubtype,
		KeySDKControlUnavailableDuringQuery,
		KeySDKParseControlPayload,
		KeySDKMarshalInitializeResponse,
		KeySDKMarshalResumeResponse,
		KeySDKMarshalCompactResponse,
		KeySDKMarshalContextUsage,
		KeySDKParseControlResponse,
		KeySDKUnrecognizedControlResponsePayload,
		KeySDKMarshalOutput,
		KeySDKWriteOutput,
		KeySDKServeAlreadyStarted,
		KeySDKPermissionDuplicateRequestID,
	}

	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if got := semanticTranslations[key][lang]; got == "" {
				t.Errorf("translation for %q in %s is missing", key, lang.Code())
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatalf("semantic catalog is incomplete: %v", err)
	}
}

func TestSDKBrandEnglishCopyRemainsCompatible(t *testing.T) {
	want := map[Key]string{
		KeyBrandTagline:                          "Agentic coding in your terminal",
		KeySDKReady:                              "ready",
		KeySDKSystemPromptReceived:               "system_prompt_received",
		KeySDKStdinReadError:                     "sdk: stdin read error: raw-error",
		KeySDKInvalidJSONEnvelope:                "sdk: invalid JSON envelope: raw-error",
		KeySDKUnknownMessageType:                 `sdk: unknown message type "raw-value"`,
		KeySDKParseUserMessage:                   "sdk: parse user message: raw-error",
		KeySDKUserUUIDRequired:                   "user message uuid is required",
		KeySDKExtractMessageText:                 "sdk: extract message text: raw-error",
		KeySDKUnsupportedMessageContent:          "unsupported user message content",
		KeySDKStreamEndedWithoutFinalEvent:       "stream ended without final event",
		KeySDKQueryCancelled:                     "query cancelled",
		KeySDKQueryAlreadyActive:                 "another query is already running",
		KeySDKParseControlRequest:                "sdk: parse control_request: raw-error",
		KeySDKControlRequestIDRequired:           "control request_id is required",
		KeySDKControlRequestIDConflict:           `control request ID "raw-value" was reused with a different payload`,
		KeySDKParseRequestSubtype:                "parse request subtype: raw-error",
		KeySDKUnsupportedControlSubtype:          `unsupported control subtype "raw-value"`,
		KeySDKControlUnavailableDuringQuery:      `control subtype "raw-value" is unavailable while a query is running`,
		KeySDKParseControlPayload:                "parse raw-value: raw-error",
		KeySDKMarshalInitializeResponse:          "marshal initialize response: raw-error",
		KeySDKMarshalResumeResponse:              "marshal resume response: raw-error",
		KeySDKMarshalCompactResponse:             "marshal compact response: raw-error",
		KeySDKMarshalContextUsage:                "marshal context usage: raw-error",
		KeySDKParseControlResponse:               "sdk: parse control_response: raw-error",
		KeySDKUnrecognizedControlResponsePayload: "sdk: unrecognized control_response payload: raw-value",
		KeySDKMarshalOutput:                      "sdk: marshal output: raw-error",
		KeySDKWriteOutput:                        "sdk: write output: raw-error",
		KeySDKServeAlreadyStarted:                "sdk server can only be served once",
		KeySDKPermissionDuplicateRequestID:       `sdk: permission request ID "raw-value" is already pending`,
	}

	for key, expected := range want {
		var got string
		switch key {
		case KeySDKUnknownMessageType, KeySDKUnsupportedControlSubtype,
			KeySDKControlUnavailableDuringQuery, KeySDKPermissionDuplicateRequestID,
			KeySDKControlRequestIDConflict:
			got = Format(LangEN, key, "raw-value")
		case KeySDKParseControlPayload:
			got = Format(LangEN, key, "raw-value", errors.New("raw-error"))
		case KeySDKUnrecognizedControlResponsePayload:
			got = Format(LangEN, key, "raw-value")
		case KeySDKStdinReadError, KeySDKInvalidJSONEnvelope, KeySDKParseUserMessage,
			KeySDKExtractMessageText, KeySDKParseControlRequest, KeySDKParseRequestSubtype,
			KeySDKMarshalInitializeResponse, KeySDKMarshalResumeResponse, KeySDKMarshalCompactResponse, KeySDKMarshalContextUsage,
			KeySDKParseControlResponse, KeySDKMarshalOutput, KeySDKWriteOutput:
			got = Format(LangEN, key, errors.New("raw-error"))
		default:
			got = Text(LangEN, key)
		}
		if got != expected {
			t.Errorf("English copy for %q = %q, want %q", key, got, expected)
		}
	}
}

func TestSDKLocalizedCopyPreservesRawProtocolValues(t *testing.T) {
	rawBody := `{"request_id":"req-7","opaque":true}`
	tests := []struct {
		lang    Language
		key     Key
		args    []any
		retains []string
	}{
		{LangZH, KeySDKUnsupportedControlSubtype, []any{"vendor_extension"}, []string{"vendor_extension"}},
		{LangDE, KeySDKParseControlPayload, []any{"set_model", errors.New("raw-cause")}, []string{"set_model", "raw-cause"}},
		{LangJA, KeySDKUnrecognizedControlResponsePayload, []any{rawBody}, []string{rawBody}},
	}
	for _, tt := range tests {
		got := Format(tt.lang, tt.key, tt.args...)
		for _, raw := range tt.retains {
			if !strings.Contains(got, raw) {
				t.Errorf("Format(%s, %q) lost raw value %q: %q", tt.lang.Code(), tt.key, raw, got)
			}
		}
	}
	if got := Text(LangZH, KeyBrandTagline); got == Text(LangEN, KeyBrandTagline) {
		t.Fatalf("Chinese tagline was not localized: %q", got)
	}

	cause := errors.New("raw-sdk-cause")
	wrapped := WrapError(KeySDKInvalidJSONEnvelope, cause)
	if !errors.Is(wrapped, cause) {
		t.Fatal("localized SDK error no longer preserves its cause")
	}
}
