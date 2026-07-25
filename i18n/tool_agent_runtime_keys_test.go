package i18n

import (
	"strings"
	"testing"
)

func TestToolAgentRuntimeKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolAgentRuntimeKeys {
		for _, lang := range AllLanguages() {
			if copy := Text(lang, key); strings.TrimSpace(copy) == "" || strings.HasPrefix(copy, "[") {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, copy)
			}
		}
	}
}

func TestToolAgentRuntimeKeysEnglishContract(t *testing.T) {
	got := Format(LangEN, KeyToolAgentTeamMissing, "core")
	want := "Team \"core\" does not exist. Create it with TeamCreate before spawning teammates."
	if got != want {
		t.Fatalf("team missing copy = %q, want %q", got, want)
	}
}
