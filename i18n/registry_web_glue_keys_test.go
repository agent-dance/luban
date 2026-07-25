package i18n

import (
	"errors"
	"strings"
	"testing"
)

type registryWebGlueTestCause struct{}

func (*registryWebGlueTestCause) Error() string { return "raw-provider-cause" }

func TestRegistryWebGlueKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range registryWebGlueKeys {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if got == "" || got == "["+string(key)+"]" {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryWebGlueEnglishContract(t *testing.T) {
	cause := errors.New("raw-cause")
	tests := []struct {
		key  Key
		args []any
		want string
	}{
		{KeyRegistryWebSearchProviderNilStream, nil, "WebSearch provider returned a nil stream"},
		{KeyRegistryWebSearchProviderStreamFailed, nil, "WebSearch provider stream failed"},
		{KeyRegistryWebSearchResultMissingRawContent, nil, "WebSearch result block omitted raw content"},
		{KeyRegistryWebSearchDecodeResultBlock, []any{cause}, "decode WebSearch result block: raw-cause"},
		{KeyRegistryWebSearchDecodeHits, []any{cause}, "decode WebSearch hits: raw-cause"},
		{KeyRegistryWebSearchDecodeError, []any{cause}, "decode WebSearch error: raw-cause"},
		{KeyRegistryWebFetchSecondaryProviderUnavailable, nil, "WebFetch secondary model provider is unavailable"},
		{KeyRegistryWebFetchSecondaryModelNilStream, nil, "WebFetch secondary model returned a nil stream"},
		{KeyRegistryWebFetchSecondaryModelStreamFailed, nil, "WebFetch secondary model stream failed"},
	}

	for _, tt := range tests {
		if got := Format(LangEN, tt.key, tt.args...); got != tt.want {
			t.Errorf("Format(LangEN, %q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestRegistryWebGlueTranslationsPreserveRawCause(t *testing.T) {
	for _, lang := range AllLanguages() {
		for _, key := range []Key{
			KeyRegistryWebSearchDecodeResultBlock,
			KeyRegistryWebSearchDecodeHits,
			KeyRegistryWebSearchDecodeError,
		} {
			got := Format(lang, key, errors.New("raw-provider-status-request-cause"))
			if !strings.Contains(got, "raw-provider-status-request-cause") || strings.Contains(got, "%!") {
				t.Errorf("Format(%s, %q) did not preserve the raw cause: %q", lang.Code(), key, got)
			}
		}
	}
}

func TestRegistryWebGlueErrorsUseActiveLanguageAndPreserveCause(t *testing.T) {
	previous := detectedLanguageCache.Load()
	t.Cleanup(func() { detectedLanguageCache.Store(previous) })

	cause := &registryWebGlueTestCause{}
	err := WrapError(KeyRegistryWebSearchDecodeResultBlock, cause)
	if !errors.Is(err, cause) {
		t.Fatal("registry Web glue error did not preserve errors.Is")
	}
	var typed *registryWebGlueTestCause
	if !errors.As(err, &typed) || typed != cause {
		t.Fatal("registry Web glue error did not preserve errors.As")
	}

	detectedLanguageCache.Store(int32(LangEN))
	english := err.Error()
	if english != "decode WebSearch result block: raw-provider-cause" {
		t.Fatalf("English compatibility changed: %q", english)
	}
	detectedLanguageCache.Store(int32(LangZH))
	chinese := err.Error()
	if english == chinese || !strings.Contains(chinese, "raw-provider-cause") {
		t.Fatalf("runtime localization failed: en=%q zh=%q", english, chinese)
	}

	noCause := NewError(KeyRegistryWebFetchSecondaryProviderUnavailable)
	detectedLanguageCache.Store(int32(LangEN))
	english = noCause.Error()
	detectedLanguageCache.Store(int32(LangJA))
	japanese := noCause.Error()
	if english == japanese {
		t.Fatalf("NewError did not follow the active language: en=%q ja=%q", english, japanese)
	}
}
