package i18n

import "testing"

func TestLLMActivityKeysAreLocalizedAndComplete(t *testing.T) {
	for _, key := range llmActivityKeys {
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == string(key) {
				t.Fatalf("missing %s translation for %s: %q", lang, key, got)
			}
		}
	}
	if got := Format(LangZH, KeyLLMActivityGeneratingToolInput, "ApplyPatch"); got != "正在生成 ApplyPatch 的工具输入" {
		t.Fatalf("Chinese tool-input stage = %q", got)
	}
	if got := Format(LangZH, KeyLLMActivityToolInputReceived, "正在生成 ApplyPatch 的工具输入", "10.0 KiB"); got != "正在生成 ApplyPatch 的工具输入 · 已接收工具输入 10.0 KiB" {
		t.Fatalf("Chinese tool-input received bytes = %q", got)
	}
	if got := Format(LangEN, KeyLLMActivityStageElapsed, "1m10s"); got != "stage 1m10s" {
		t.Fatalf("English stage elapsed = %q", got)
	}
}
