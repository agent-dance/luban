package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestToolWorktreeHookKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolWorktreeHookKeys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}

func TestToolWorktreeHookEnglishCompatibilityAndCause(t *testing.T) {
	cause := errors.New("raw-cause")
	if got := Format(LangEN, KeyToolWorktreeHookEncodePayload, "WorktreeCreate", cause); got != "encode WorktreeCreate hook payload: raw-cause" {
		t.Fatalf("encode payload = %q", got)
	}
	if got := Format(LangEN, KeyToolWorktreeHookOutputFormat); got != "WorktreeCreate hook output must be JSON or a single path" {
		t.Fatalf("output format = %q", got)
	}
	err := WrapError(KeyToolWorktreeHookReadSettings, cause, "/raw/settings.json")
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "/raw/settings.json") || !strings.Contains(err.Error(), cause.Error()) {
		t.Fatalf("semantic worktree hook error lost path/cause: %v", err)
	}
}
