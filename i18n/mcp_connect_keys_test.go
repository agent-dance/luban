package i18n

import (
	"strings"
	"testing"
)

func TestMCPConnectCatalogCoversEverySupportedLanguage(t *testing.T) {
	seen := 0
	for key, translations := range semanticTranslations {
		if !strings.HasPrefix(string(key), "mcp.") && !strings.HasPrefix(string(key), "connect.") {
			continue
		}
		seen++
		for _, lang := range AllLanguages() {
			if got := translations[lang]; got == "" {
				t.Errorf("%s is missing a %s translation", key, lang.Code())
			}
		}
	}
	if seen < 123 {
		t.Fatalf("checked %d MCP/connect keys, want at least 123", seen)
	}
}

func TestMCPConnectCopyIsLocalizedAndPreservesTechnicalValues(t *testing.T) {
	for _, lang := range AllLanguages() {
		row := Format(lang, KeyMCPServerRow, "server-7", "state-raw", "http", "scope-raw", 1, 2, 3, "auth-raw")
		for _, value := range []string{"server-7", "state-raw", "http", "scope-raw", "auth-raw", "1", "2", "3"} {
			if !strings.Contains(row, value) {
				t.Errorf("%s MCP row omitted %q: %q", lang.Code(), value, row)
			}
		}
		if strings.Contains(row, "%!") {
			t.Errorf("%s MCP row has a formatting error: %q", lang.Code(), row)
		}
	}

	for _, key := range []Key{KeyConnectCancelled, KeyConnectBrowserOpening, KeyConnectWaiting, KeyMCPStateConnected} {
		english := Text(LangEN, key)
		for _, lang := range AllLanguages()[1:] {
			if got := Text(lang, key); got == english {
				t.Errorf("%s still uses English for %s: %q", lang.Code(), key, got)
			}
		}
	}
}
