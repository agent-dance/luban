package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestEngineHookCatalogCoversEveryLanguage(t *testing.T) {
	for _, key := range engineHookKeys {
		for _, lang := range AllLanguages() {
			copy := Text(lang, key)
			if copy == "" || strings.HasPrefix(copy, "[") {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, copy)
			}
		}
	}
}

func TestEngineHookCopyPreservesRawValues(t *testing.T) {
	for _, lang := range AllLanguages() {
		for _, test := range []struct {
			key  Key
			args []any
			raw  []string
		}{
			{KeyEngineSessionAmbiguous, []any{"session-17"}, []string{"session-17"}},
			{KeyEngineSessionCompactFailed, []any{"session-17", "raw-provider-cause"}, []string{"session-17", "raw-provider-cause"}},
			{KeyHookHTTPURLValidationFailed, []any{"raw-url-cause"}, []string{"raw-url-cause"}},
			{KeyHookNotificationDefault, []any{"PostToolUse"}, []string{"PostToolUse"}},
			{KeyHookNotificationFailed, []any{"raw-notifier-cause"}, []string{"raw-notifier-cause"}},
		} {
			got := Format(lang, test.key, test.args...)
			for _, raw := range test.raw {
				if !strings.Contains(got, raw) {
					t.Errorf("Format(%s, %q) omitted %q: %q", lang.Code(), test.key, raw, got)
				}
			}
			if strings.Contains(got, "%!") {
				t.Errorf("Format(%s, %q) has formatting error: %q", lang.Code(), test.key, got)
			}
		}
	}
}

func TestSemanticRuntimeErrorSupportsExplicitLanguage(t *testing.T) {
	cause := errors.New("internal diagnostic must stay hidden")
	err := WrapInternalError(KeyEngineSessionSaveFailed, cause)
	localizer, ok := err.(interface{ Localized(Language) string })
	if !ok {
		t.Fatalf("semantic error %T does not support explicit localization", err)
	}
	if got := localizer.Localized(LangZH); got != Text(LangZH, KeyEngineSessionSaveFailed) {
		t.Fatalf("Localized(zh) = %q", got)
	}
	if !errors.Is(err, cause) {
		t.Fatal("semantic error did not preserve its internal cause")
	}
	if got := localizer.Localized(LangEN); strings.Contains(got, cause.Error()) {
		t.Fatalf("internal cause leaked into localized copy: %q", got)
	}
}
