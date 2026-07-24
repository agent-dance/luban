package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestProviderOpenAIOAuthCatalogCoversEveryLanguage(t *testing.T) {
	tests := []struct {
		key      Key
		args     []any
		required []string
		raw      []string
	}{
		{KeyProviderOpenAIOAuthIDTokenEmpty, nil, []string{"oauth", "id token"}, nil},
		{KeyProviderOpenAIOAuthIDTokenFormatInvalid, nil, []string{"oauth", "id token"}, nil},
		{KeyProviderOpenAIOAuthIDTokenPayloadDecodeFailed, []any{"raw-cause-17"}, []string{"oauth", "id token"}, []string{"raw-cause-17"}},
		{KeyProviderOpenAIOAuthIDTokenPayloadParseFailed, []any{"raw-cause-17"}, []string{"oauth", "id token"}, []string{"raw-cause-17"}},
		{KeyProviderOpenAIOAuthAPIKeyExchangeRequestBuildFailed, []any{"raw-cause-17"}, []string{"oauth", "api key"}, []string{"raw-cause-17"}},
		{KeyProviderOpenAIOAuthAPIKeyExchangeRequestFailed, []any{"raw-cause-17"}, []string{"oauth", "api key"}, []string{"raw-cause-17"}},
		{KeyProviderOpenAIOAuthAPIKeyExchangeRejected, []any{429, `{"error":"raw-remote-body"}`}, []string{"oauth", "api key"}, []string{"429", `{"error":"raw-remote-body"}`}},
		{KeyProviderOpenAIOAuthAPIKeyExchangeResponseDecodeFailed, []any{"raw-cause-17"}, []string{"oauth", "api key"}, []string{"raw-cause-17"}},
		{KeyProviderOpenAIOAuthAPIKeyExchangeMissingAccessToken, nil, []string{"oauth", "api key", "access_token"}, nil},
	}
	if len(providerOpenAIOAuthKeys) != len(tests) {
		t.Fatalf("provider OpenAI OAuth semantic key count = %d, want %d", len(providerOpenAIOAuthKeys), len(tests))
	}

	for _, tt := range tests {
		for _, lang := range AllLanguages() {
			got := Format(lang, tt.key, tt.args...)
			if got == "" || got == "["+string(tt.key)+"]" {
				t.Fatalf("Format(%s, %q) = %q", lang.Code(), tt.key, got)
			}
			lower := strings.ReplaceAll(strings.ToLower(got), "-", " ")
			for _, required := range tt.required {
				normalizedRequired := strings.ReplaceAll(strings.ToLower(required), "-", " ")
				if !strings.Contains(lower, normalizedRequired) {
					t.Errorf("Format(%s, %q) = %q; missing preserved value %q", lang.Code(), tt.key, got, required)
				}
			}
			for _, raw := range tt.raw {
				if !strings.Contains(got, raw) {
					t.Errorf("Format(%s, %q) = %q; raw value %q changed", lang.Code(), tt.key, got, raw)
				}
			}
			if strings.Contains(got, "%!") {
				t.Errorf("Format(%s, %q) has a formatting error: %q", lang.Code(), tt.key, got)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestProviderOpenAIOAuthCatalogPreservesEnglishCompatibility(t *testing.T) {
	tests := []struct {
		key  Key
		args []any
		want string
	}{
		{KeyProviderOpenAIOAuthIDTokenEmpty, nil, "openai oauth: id token is empty"},
		{KeyProviderOpenAIOAuthIDTokenFormatInvalid, nil, "openai oauth: invalid id token format"},
		{KeyProviderOpenAIOAuthIDTokenPayloadDecodeFailed, []any{"cause"}, "openai oauth: decode id token payload: cause"},
		{KeyProviderOpenAIOAuthIDTokenPayloadParseFailed, []any{"cause"}, "openai oauth: parse id token payload: cause"},
		{KeyProviderOpenAIOAuthAPIKeyExchangeRequestBuildFailed, []any{"cause"}, "openai oauth: build api-key exchange request: cause"},
		{KeyProviderOpenAIOAuthAPIKeyExchangeRequestFailed, []any{"cause"}, "openai oauth: api-key exchange request: cause"},
		{KeyProviderOpenAIOAuthAPIKeyExchangeRejected, []any{429, "raw-body"}, "openai oauth: api-key exchange returned 429: raw-body"},
		{KeyProviderOpenAIOAuthAPIKeyExchangeResponseDecodeFailed, []any{"cause"}, "openai oauth: decode api-key exchange response: cause"},
		{KeyProviderOpenAIOAuthAPIKeyExchangeMissingAccessToken, nil, "openai oauth: api-key exchange response missing access_token"},
	}

	for _, tt := range tests {
		if got := Format(LangEN, tt.key, tt.args...); got != tt.want {
			t.Errorf("Format(LangEN, %q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

type providerOpenAIOAuthTypedCause struct {
	marker string
}

func (e *providerOpenAIOAuthTypedCause) Error() string { return e.marker }

func TestProviderOpenAIOAuthWrappedErrorsPreserveTypedCause(t *testing.T) {
	cause := &providerOpenAIOAuthTypedCause{marker: "raw-typed-cause-23"}
	err := WrapError(KeyProviderOpenAIOAuthAPIKeyExchangeRequestFailed, cause)

	if !errors.Is(err, cause) {
		t.Fatal("OpenAI OAuth semantic error did not preserve errors.Is")
	}
	var typed *providerOpenAIOAuthTypedCause
	if !errors.As(err, &typed) || typed != cause {
		t.Fatal("OpenAI OAuth semantic error did not preserve errors.As")
	}
	localizer, ok := err.(interface{ Localized(Language) string })
	if !ok {
		t.Fatalf("semantic error %T does not support explicit localization", err)
	}
	if got := localizer.Localized(LangZH); !strings.Contains(got, cause.Error()) || !strings.Contains(got, "OAuth") {
		t.Fatalf("Localized(zh) = %q; raw cause or protocol term was lost", got)
	}
}
