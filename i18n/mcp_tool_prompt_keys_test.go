package i18n

import (
	"strings"
	"testing"
)

func TestMCPToolPromptKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range mcpToolPromptKeys {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if got == "" || got == "["+string(key)+"]" {
				t.Errorf("%s is missing for %s: %q", key, lang.Code(), got)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestMCPToolPromptKeysPreserveProtocolAndExternalValues(t *testing.T) {
	for _, lang := range AllLanguages() {
		fallback := Format(lang, KeyMCPDynamicToolFallbackDescription, "lookup", "docs")
		for _, token := range []string{"MCP", "lookup", "docs"} {
			if !strings.Contains(fallback, token) {
				t.Errorf("fallback description for %s omitted %q: %q", lang.Code(), token, fallback)
			}
		}
		location := Format(lang, KeyMCPAuthToolTransportLocation, "http", "https://example.invalid/mcp")
		for _, token := range []string{"http", "https://example.invalid/mcp"} {
			if !strings.Contains(location, token) {
				t.Errorf("transport location for %s omitted %q: %q", lang.Code(), token, location)
			}
		}
		truncated := Format(lang, KeyMCPDynamicToolTruncatedDescription, "external description")
		if !strings.Contains(truncated, "external description") {
			t.Errorf("truncated description for %s lost external copy: %q", lang.Code(), truncated)
		}
	}
}
