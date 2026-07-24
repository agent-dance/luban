package i18n

import (
	"strings"
	"testing"
)

func TestMCPAuthToolKeysCoverEveryLanguageAndPreserveValues(t *testing.T) {
	keys := []Key{
		KeyMCPAuthToolDescription, KeyMCPAuthToolUninitialized, KeyMCPAuthToolClaudeConnector,
		KeyMCPAuthToolUnsupportedTransport, KeyMCPAuthToolStartFailed, KeyMCPAuthToolAuthorizationURL,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if got == "" || got == "["+string(key)+"]" {
				t.Fatalf("%s is missing for %s", key, lang.Code())
			}
		}
	}
	got := Format(LangZH, KeyMCPAuthToolAuthorizationURL, "demo", "https://example.invalid/auth")
	if !strings.Contains(got, "demo") || !strings.Contains(got, "https://example.invalid/auth") {
		t.Fatalf("authorization message lost raw values: %q", got)
	}
}
