package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestProviderResidualKeysCoverEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyProviderOpenAIStreamChunkParseFailed,
		KeyProviderResponsesCompletedParseFailed,
		KeyProviderResponsesFailedParseFailed,
	}
	diagnostic := errors.New("invalid character 'x'")

	for _, key := range keys {
		for _, lang := range AllLanguages() {
			got := Format(lang, key, diagnostic)
			if got == "" || got == "["+string(key)+"]" {
				t.Fatalf("Format(%s, %q) = %q", lang.Code(), key, got)
			}
			if !strings.Contains(got, diagnostic.Error()) {
				t.Fatalf("Format(%s, %q) = %q; raw diagnostic was not preserved", lang.Code(), key, got)
			}
		}
	}
}

func TestProviderResidualKeysPreserveEnglishContract(t *testing.T) {
	diagnostic := errors.New("unexpected EOF")
	tests := []struct {
		key  Key
		want string
	}{
		{KeyProviderOpenAIStreamChunkParseFailed, "failed to parse stream chunk: unexpected EOF"},
		{KeyProviderResponsesCompletedParseFailed, "failed to parse response.completed: unexpected EOF"},
		{KeyProviderResponsesFailedParseFailed, "failed to parse response.failed: unexpected EOF"},
	}

	for _, tt := range tests {
		if got := Format(LangEN, tt.key, diagnostic); got != tt.want {
			t.Errorf("Format(LangEN, %q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestProviderResponsesContinuationInvalidCoversEveryLanguage(t *testing.T) {
	for _, key := range []Key{KeyProviderResponsesContinuationInvalid, KeyProviderResponsesCustomToolCallInvalid} {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if got == "" || got == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}
