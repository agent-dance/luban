package i18n

import (
	"strings"
	"testing"
)

func TestToolConfigKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolConfigKeys {
		for _, language := range AllLanguages() {
			translation := Text(language, key)
			if strings.TrimSpace(translation) == "" || translation == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", language.Code(), key, translation)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestToolConfigEnglishRendering(t *testing.T) {
	if got := Format(LangEN, KeyToolConfigUpdated, "theme", "dark"); got != "Config updated: theme = dark" {
		t.Fatalf("updated result = %q", got)
	}
	if got := Format(LangEN, KeyToolConfigValue, "theme", "dark"); got != "theme = dark" {
		t.Fatalf("value result = %q", got)
	}
}
