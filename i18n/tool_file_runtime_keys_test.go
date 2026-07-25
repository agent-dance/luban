package i18n

import (
	"strings"
	"testing"
)

func TestToolFileRuntimeKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolFileRuntimeKeys {
		for _, lang := range AllLanguages() {
			if copy := Text(lang, key); strings.TrimSpace(copy) == "" || strings.HasPrefix(copy, "[") {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, copy)
			}
		}
	}
}

func TestToolFileRuntimeKeysEnglishContract(t *testing.T) {
	got := Format(LangEN, KeyToolFilePDFPagesExtracted, 3, "/tmp/a.pdf", "42KB")
	want := "PDF pages extracted: 3 page(s) from /tmp/a.pdf (42KB)"
	if got != want {
		t.Fatalf("PDF pages copy = %q, want %q", got, want)
	}
}
