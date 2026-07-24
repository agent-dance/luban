package i18n

import (
	"strings"
	"testing"
)

func TestInternalTranscriptCatalogIsComplete(t *testing.T) {
	for _, key := range internalTranscriptKeys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); strings.TrimSpace(got) == "" {
				t.Fatalf("missing %s translation for %s", key, lang.Code())
			}
		}
	}
}

func TestInternalTranscriptCatalogPreservesRuntimeValues(t *testing.T) {
	for _, lang := range AllLanguages() {
		got := Format(lang, KeyToolReadOffsetBeyondEndWarning, 17, 4)
		if !strings.Contains(got, "17") || !strings.Contains(got, "4") || strings.Contains(got, "%!") {
			t.Fatalf("%s offset warning lost values: %q", lang.Code(), got)
		}
	}
}
