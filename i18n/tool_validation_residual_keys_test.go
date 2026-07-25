package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestToolValidationResidualCatalogIsComplete(t *testing.T) {
	for _, key := range toolValidationResidualKeys {
		for _, lang := range AllLanguages() {
			if text := Text(lang, key); strings.TrimSpace(text) == "" {
				t.Fatalf("missing translation for %s in %s", key, lang.Code())
			}
		}
	}
}

func TestToolValidationResidualCatalogPreservesRuntimeValues(t *testing.T) {
	for _, lang := range AllLanguages() {
		if got := Format(lang, KeyToolNotificationHookFailed, "hook-id", 17, "raw-stderr"); !strings.Contains(got, "hook-id") || !strings.Contains(got, "17") || !strings.Contains(got, "raw-stderr") {
			t.Fatalf("%s notification failure lost raw values: %q", lang.Code(), got)
		}
	}
}

func TestToolValidationResidualWrappedCausesRemainDiscoverable(t *testing.T) {
	cause := errors.New("raw-cause")
	for _, key := range []Key{KeyToolPDFCreateSwiftRendererScript, KeyToolPDFWriteSwiftRendererScript} {
		err := WrapError(key, cause)
		if !errors.Is(err, cause) {
			t.Fatalf("%s did not preserve wrapped cause", key)
		}
		for _, lang := range AllLanguages() {
			if got := err.(*semanticRuntimeError).Localized(lang); !strings.Contains(got, "raw-cause") {
				t.Fatalf("%s %s lost cause text: %q", key, lang.Code(), got)
			}
		}
	}
}
