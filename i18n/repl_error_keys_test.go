package i18n

import (
	"strings"
	"testing"
)

func TestREPLErrorKeysCoverAllLanguages(t *testing.T) {
	tested := 0
	for key := range semanticTranslations {
		if !strings.HasPrefix(string(key), "repl.error.") {
			continue
		}
		tested++
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if got == "" || got == "["+string(key)+"]" {
				t.Errorf("%s is missing for %s: %q", key, lang.Code(), got)
			}
		}
	}
	if tested < 60 {
		t.Fatalf("tested only %d REPL error keys; new keys may not be registered", tested)
	}
}

func TestREPLErrorFormattingPreservesRuntimeValues(t *testing.T) {
	tests := []struct {
		lang Language
		key  Key
		args []any
		want []string
	}{
		{lang: LangZH, key: KeyREPLErrorUsage, args: []any{"/mode auto|ask|plan"}, want: []string{"/mode auto|ask|plan"}},
		{lang: LangJA, key: KeyREPLErrorFollowUpTaskUnresolved, args: []any{"task-42"}, want: []string{"task-42"}},
		{lang: LangDE, key: KeyREPLErrorDecisionWrongSession, args: []any{"decision-1", "session-old", "session-new"}, want: []string{"decision-1", "session-old", "session-new"}},
		{lang: LangRU, key: KeyREPLErrorRollbackModeFailedClosed, args: []any{"raw rollback error", "plan"}, want: []string{"raw rollback error", "plan"}},
	}
	for _, tt := range tests {
		got := Format(tt.lang, tt.key, tt.args...)
		for _, want := range tt.want {
			if !strings.Contains(got, want) {
				t.Errorf("Format(%s, %s) = %q, missing %q", tt.lang, tt.key, got, want)
			}
		}
	}
}
