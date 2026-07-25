package i18n

import (
	"strings"
	"testing"
)

func TestToolSendMessageValidationKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolSendMessageValidationKeys {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if got == "" || got == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}

func TestToolSendMessageValidationKeysPreserveEnglishContract(t *testing.T) {
	tests := []struct {
		key  Key
		args []any
		want string
	}{
		{KeyToolSendMessageStructuredObjectRequired, nil, "structured message must be an object"},
		{KeyToolSendMessageStructuredTypeUnsupported, []any{"raw_type"}, "unsupported structured message type: raw_type"},
		{KeyToolSendMessageStructuredFieldUnsupported, []any{"raw_field"}, "unsupported structured message field: raw_field"},
		{KeyToolSendMessageStructuredFieldStringRequired, []any{"raw_field"}, "raw_field must be a string"},
		{KeyToolSendMessageStructuredFieldRequired, []any{"shutdown_response", "request_id"}, "shutdown_response requires request_id"},
	}

	for _, tt := range tests {
		if got := Format(LangEN, tt.key, tt.args...); got != tt.want {
			t.Errorf("Format(LangEN, %q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestToolSendMessageValidationErrorUsesRuntimeLanguageAndPreservesRawFields(t *testing.T) {
	previous := detectedLanguageCache.Load()
	t.Cleanup(func() { detectedLanguageCache.Store(previous) })

	err := NewError(KeyToolSendMessageStructuredFieldRequired, "raw_type_17", "raw_request_id_23")
	detectedLanguageCache.Store(int32(LangEN))
	english := err.Error()
	detectedLanguageCache.Store(int32(LangZH))
	chinese := err.Error()

	if english == chinese {
		t.Fatalf("runtime localization did not change the rendered error: en=%q zh=%q", english, chinese)
	}
	for _, raw := range []string{"raw_type_17", "raw_request_id_23"} {
		if !strings.Contains(chinese, raw) {
			t.Errorf("localized error omitted raw parameter %q: %q", raw, chinese)
		}
	}
}
