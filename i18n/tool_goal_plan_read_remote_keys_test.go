package i18n

import (
	"strings"
	"testing"
)

func TestToolDomainKeysCoverEveryLanguage(t *testing.T) {
	prefixes := []string{
		"tool.git.", "tool.goal.", "tool.network.", "tool.plan.",
		"tool.read.", "tool.remote_trigger.",
	}
	tested := 0
	for key := range semanticTranslations {
		matched := false
		for _, prefix := range prefixes {
			if strings.HasPrefix(string(key), prefix) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		tested++
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Errorf("%s is missing for %s: %q", key, lang.Code(), got)
			}
		}
	}
	if tested < 65 {
		t.Fatalf("tested %d tool-domain keys; want at least 65", tested)
	}
}

func TestToolDomainEnglishContractsAndRawValues(t *testing.T) {
	tests := []struct {
		key  Key
		args []any
		want string
	}{
		{KeyToolGoalMarked, []any{"complete", "ship it"}, "Goal marked complete: ship it"},
		{KeyToolReadPagesInvalid, []any{"bad"}, `Invalid pages parameter: "bad". Use formats like "1-5", "3", or "10-20". Pages are 1-indexed.`},
		{KeyToolRemoteTriggerIDInvalid, nil, `trigger_id must match ^[\w-]+$`},
	}
	for _, test := range tests {
		if got := Format(LangEN, test.key, test.args...); got != test.want {
			t.Errorf("Format(LangEN, %s) = %q, want %q", test.key, got, test.want)
		}
	}

	raw := `upstream said: code="E_RAW"`
	if got := Format(LangZH, KeyToolRemoteRequestFailed, raw); !strings.Contains(got, raw) {
		t.Fatalf("localized error lost raw external value: %q", got)
	}
}
