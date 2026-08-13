package i18n

import (
	"strings"
	"testing"
)

func TestREPLReceiptKeysCoverAllLanguages(t *testing.T) {
	keys := []Key{
		KeyREPLBackgroundStarted, KeyREPLQueryFailed, KeyREPLClearConversation,
		KeyREPLResumeCompleted, KeyREPLExportCompleted, KeyREPLCompactionBoundary,
		KeyREPLTUIForkOpened, KeyREPLTUIImageUnsupported, KeyREPLTUIContextWindowRange,
	}
	for _, lang := range AllLanguages() {
		for _, key := range keys {
			if got := Text(lang, key); got == "" || strings.HasPrefix(got, "[") {
				t.Fatalf("%s is missing for %s: %q", key, lang.Code(), got)
			}
		}
	}
}

func TestREPLReceiptFormattingPreservesRuntimeValues(t *testing.T) {
	got := Format(LangZH, KeyREPLBackgroundFailed, "session-42", "raw failure")
	if !strings.Contains(got, "session-42") || !strings.Contains(got, "raw failure") {
		t.Fatalf("formatted receipt did not preserve runtime values: %q", got)
	}
}

func TestREPLTUISemanticKeysCoverAllLanguages(t *testing.T) {
	tested := 0
	for key := range semanticTranslations {
		if !strings.HasPrefix(string(key), "repl.tui.") {
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
	if tested < 80 {
		t.Fatalf("tested only %d REPL TUI keys; new keys may not be registered", tested)
	}
}

func TestREPLTUIFormattingPreservesTechnicalRuntimeValues(t *testing.T) {
	tests := []struct {
		lang Language
		key  Key
		args []any
		want []string
	}{
		{lang: LangZH, key: KeyREPLTUIModelSwitchedReasoning, args: []any{"openai", "gpt-x", "high"}, want: []string{"openai", "gpt-x", "high"}},
		{lang: LangJA, key: KeyREPLTUIOAuthFailed, args: []any{"anthropic", "raw oauth error"}, want: []string{"anthropic", "raw oauth error"}},
		{lang: LangEN, key: KeyREPLTUILifecycleSaveFailed, args: []any{"raw save error"}, want: []string{"raw save error"}},
		{lang: LangDE, key: KeyREPLTUIAgentPath, args: []any{"/tmp/agent-1"}, want: []string{"/tmp/agent-1"}},
	}
	for _, tt := range tests {
		got := Format(tt.lang, tt.key, tt.args...)
		if strings.Contains(got, "%!") {
			t.Errorf("Format(%s, %s) contains a formatting diagnostic: %q", tt.lang, tt.key, got)
		}
		for _, want := range tt.want {
			if !strings.Contains(got, want) {
				t.Errorf("Format(%s, %s) = %q, missing %q", tt.lang, tt.key, got, want)
			}
		}
	}
}

func TestREPLTUIAgentMetricsUseIdiomaticLabels(t *testing.T) {
	toolLabels := map[Language]string{
		LangEN: "tool Bash", LangZH: "工具 Bash", LangDE: "Tool Bash",
		LangJA: "ツール Bash", LangKO: "도구 Bash", LangRU: "инструмент Bash",
	}
	tokenLabels := map[Language]string{
		LangEN: "42 tokens", LangZH: "42 个 Token", LangDE: "42 Token",
		LangJA: "42 トークン", LangKO: "토큰 42개", LangRU: "42 токенов",
	}
	for _, lang := range AllLanguages() {
		if got := Format(lang, KeyREPLTUIToolName, "Bash"); got != toolLabels[lang] {
			t.Errorf("tool label in %s = %q, want %q", lang.Code(), got, toolLabels[lang])
		}
		if got := Format(lang, KeyREPLTUITokenCount, 42); got != tokenLabels[lang] {
			t.Errorf("token label in %s = %q, want %q", lang.Code(), got, tokenLabels[lang])
		}
	}
}
