package i18n

import (
	"regexp"
	"strings"
	"testing"
)

func TestSemanticCatalogIsComplete(t *testing.T) {
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestSemanticCatalogUsesGoPositionalFormatSyntax(t *testing.T) {
	cStylePosition := regexp.MustCompile(`%[0-9]+\$`)
	for key, translations := range semanticTranslations {
		for lang, text := range translations {
			if cStylePosition.MatchString(text) {
				t.Errorf("%s %s uses C-style positional formatting; use %%[n]verb: %q", key, lang.Code(), text)
			}
		}
	}
}

func TestSemanticCatalogReorderedArgumentsRenderWithoutFormatErrors(t *testing.T) {
	tests := []struct {
		key  Key
		args []any
		want []string
	}{
		{KeyMCPServerRemoved, []any{"server-raw", "/settings/raw"}, []string{"server-raw", "/settings/raw"}},
		{KeyToolMCPReadFailed, []any{"resource://raw", "server-raw", "cause-raw"}, []string{"resource://raw", "server-raw", "cause-raw"}},
		{KeyToolLegacyAFilePDFPagesExtracted, []any{7, "/file/raw.pdf", "3 MiB"}, []string{"7", "/file/raw.pdf", "3 MiB"}},
	}
	for _, test := range tests {
		for _, lang := range AllLanguages() {
			got := Format(lang, test.key, test.args...)
			if strings.Contains(got, "%!") {
				t.Errorf("%s %s produced a format error: %q", test.key, lang.Code(), got)
			}
			for _, want := range test.want {
				if !strings.Contains(got, want) {
					t.Errorf("%s %s omitted %q: %q", test.key, lang.Code(), want, got)
				}
			}
		}
	}
}

func TestFormatUsesSemanticKey(t *testing.T) {
	if got := Format(LangZH, KeyLanguageSwitched, LangJA.String()); got != "已切换为日本語" {
		t.Fatalf("Format() = %q", got)
	}
}

func TestLanguageUnavailableIsTranslated(t *testing.T) {
	for _, lang := range AllLanguages() {
		if got := Text(lang, KeyLanguageUnavailable); got == "" || got == "["+string(KeyLanguageUnavailable)+"]" {
			t.Fatalf("Text(%s, %q) = %q", lang.Code(), KeyLanguageUnavailable, got)
		}
	}
}
