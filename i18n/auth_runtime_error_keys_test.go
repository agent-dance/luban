package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestAuthRuntimeErrorCatalogCoversEveryLanguage(t *testing.T) {
	for _, key := range authRuntimeErrorKeys {
		for _, lang := range AllLanguages() {
			copy := Text(lang, key)
			if copy == "" || strings.HasPrefix(copy, "[") {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, copy)
			}
		}
	}
}

func TestSemanticRuntimeErrorUsesCurrentLanguageAndPreservesCause(t *testing.T) {
	previous := detectedLanguageCache.Load()
	t.Cleanup(func() { detectedLanguageCache.Store(previous) })

	cause := errors.New("raw-cause-17")
	err := WrapError(KeyAuthOAuthTokenRequest, cause)
	if !errors.Is(err, cause) {
		t.Fatal("WrapError did not preserve the underlying cause")
	}

	detectedLanguageCache.Store(int32(LangEN))
	english := err.Error()
	detectedLanguageCache.Store(int32(LangZH))
	chinese := err.Error()
	if english == chinese || !strings.Contains(chinese, "raw-cause-17") {
		t.Fatalf("runtime localization failed: en=%q zh=%q", english, chinese)
	}
}

func TestWrapInternalErrorPreservesCauseWithoutRenderingIt(t *testing.T) {
	cause := errors.New("internal diagnostic must stay hidden")
	err := WrapInternalError(KeyAuthOAuthNoCredentials, cause)

	if !errors.Is(err, cause) {
		t.Fatal("internal semantic error did not preserve its cause")
	}
	if got := err.Error(); strings.Contains(got, cause.Error()) {
		t.Fatalf("internal cause leaked into user-visible copy: %q", got)
	}
}

func TestWrapInternalErrorInLanguageUsesCapturedLanguageAndHidesCause(t *testing.T) {
	previous := detectedLanguageCache.Load()
	t.Cleanup(func() { detectedLanguageCache.Store(previous) })
	detectedLanguageCache.Store(int32(LangEN))

	cause := errors.New("internal editor diagnostic must stay hidden")
	err := WrapInternalErrorInLanguage(LangZH, KeyAuthOAuthNoCredentials, cause)
	if !errors.Is(err, cause) {
		t.Fatal("explicit-language semantic error did not preserve its cause")
	}
	if got, want := err.Error(), Text(LangZH, KeyAuthOAuthNoCredentials); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	if got := err.Error(); strings.Contains(got, cause.Error()) {
		t.Fatalf("internal cause leaked into explicit-language copy: %q", got)
	}
}

func TestDescribeSemanticErrorReturnsUnrenderedMetadata(t *testing.T) {
	cause := errors.New("raw-descriptor-cause")
	err := WrapError(KeyAuthOAuthTokenEndpointRejected, cause, 418)
	info, ok := DescribeSemanticError(err)
	if !ok || info.Key != KeyAuthOAuthTokenEndpointRejected || !info.IncludeCause || !errors.Is(info.Cause, cause) {
		t.Fatalf("DescribeSemanticError() = %#v, %v", info, ok)
	}
	if len(info.Args) != 1 || info.Args[0] != 418 {
		t.Fatalf("semantic args = %#v", info.Args)
	}
	info.Args[0] = 500
	if got := err.Error(); !strings.Contains(got, "418") || strings.Contains(got, "500") {
		t.Fatalf("descriptor mutated error state: %q", got)
	}
	if _, ok := DescribeSemanticError(errors.New("plain")); ok {
		t.Fatal("plain error was reported as semantic")
	}
}

func TestAuthRuntimeErrorsPreserveRawParameters(t *testing.T) {
	for _, lang := range AllLanguages() {
		got := Format(lang, KeyAuthOAuthTokenEndpointRejected, 418, "raw-remote-body")
		for _, raw := range []string{"418", "raw-remote-body"} {
			if !strings.Contains(got, raw) {
				t.Errorf("%s omitted raw parameter %q: %q", lang.Code(), raw, got)
			}
		}
		if strings.Contains(got, "%!") {
			t.Errorf("%s has a formatting error: %q", lang.Code(), got)
		}
	}
}
