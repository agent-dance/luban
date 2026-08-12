package i18n

import "testing"

func TestRootSurfaceKeysAreLocalizedAndComplete(t *testing.T) {
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatalf("semantic catalog is incomplete: %v", err)
	}
	if got := Format(LangJA, KeyModelPickerProviderHint, "provider-id"); got == "" || got == "Provider: provider-id. Press Enter to select reasoning effort, or Esc to go back." {
		t.Fatalf("model picker hint was not localized: %q", got)
	}
	if got := Format(LangRU, KeyImagePasted, 7, "image/png", "12 KB"); got == "" {
		t.Fatal("clipboard image feedback is missing")
	}
	for _, lang := range AllLanguages() {
		if got := Format(lang, KeyImageOpenFailed, "raw cause"); got == "" || got == "["+string(KeyImageOpenFailed)+"]" {
			t.Fatalf("image open failure is missing for %s: %q", lang.Code(), got)
		}
	}
}

func TestActivityRunningCountIsLocalizedForEveryLanguage(t *testing.T) {
	for _, lang := range AllLanguages() {
		got := Format(lang, KeyActivityRunning, 3)
		if got == "" || got == "[tui.activity.running]" {
			t.Fatalf("running count is missing for %s: %q", lang.Code(), got)
		}
		if got == Format(lang, KeyActivityRunning, 4) {
			t.Fatalf("running count does not interpolate for %s: %q", lang.Code(), got)
		}
	}
}

func TestTranscriptSelectionHintsAreLocalizedForEveryLanguage(t *testing.T) {
	keys := []Key{KeyTranscriptSelectionHintOption, KeyTranscriptSelectionHintGeneric}
	for _, lang := range AllLanguages() {
		for _, key := range keys {
			got := Text(lang, key)
			if got == "" || got == "["+string(key)+"]" {
				t.Fatalf("transcript selection hint %s is missing for %s: %q", key, lang.Code(), got)
			}
		}
	}
	if got := Text(LangZH, KeyTranscriptSelectionHintOption); got != "提示：按住 Option（Alt）并拖动以选择文字" {
		t.Fatalf("Chinese Option selection hint = %q", got)
	}
}

func TestLLMRequestStatusKeysAreLocalizedAndComplete(t *testing.T) {
	keys := []Key{KeyLLMRequestProblem, KeyLLMRequestRetrying, KeyLLMRequestRequestRetrying, KeyLLMRequestReconnecting, KeyLLMRequestProblemDetail, KeyLLMRequestRetryCount, KeyLLMRequestError, KeyLLMRequestMetrics, KeyLLMRequestInterruptStatus, KeyAssistantWorkedFor}
	for _, lang := range AllLanguages() {
		for _, key := range keys {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Fatalf("LLM request key %s is missing for %s: %q", key, lang.Code(), got)
			}
		}
	}
	if got := Format(LangZH, KeyLLMRequestMetrics, "120ms", "800ms"); got != "建立连接 120ms · 首 token 800ms" {
		t.Fatalf("Chinese LLM metrics = %q", got)
	}
	if got := Format(LangZH, KeyLLMRequestRetrying, 2, 10, "2.0s"); got != "第 2/10 次重试 · 2.0s 后继续" {
		t.Fatalf("Chinese retry status = %q", got)
	}
	if got := Format(LangZH, KeyLLMRequestRetryCount, 2, 5); got != "重试 2/5" {
		t.Fatalf("Chinese retry count = %q", got)
	}
	if got := Format(LangZH, KeyLLMRequestProblemDetail, "连接已重置"); got != "问题：连接已重置" {
		t.Fatalf("Chinese retry problem = %q", got)
	}
	if got := Format(LangZH, KeyLLMRequestInterruptStatus, "1.2s"); got != "(1.2s • Ctrl+C 中断)" {
		t.Fatalf("Chinese interrupt status = %q", got)
	}
	if got := Format(LangZH, KeyAssistantWorkedFor, "8m 02s"); got != "工作耗时 8m 02s" {
		t.Fatalf("Chinese assistant duration = %q", got)
	}
}

func TestModelLimitEditKeysAreLocalizedAndComplete(t *testing.T) {
	keys := []Key{
		KeyModelLimitEditTitle,
		KeyModelLimitEditCurrent,
		KeyModelLimitEditInput,
		KeyModelLimitEditHint,
		KeyModelLimitEditUnknown,
		KeyModelLimitEditOverridden,
	}
	for _, lang := range AllLanguages() {
		for _, key := range keys {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Fatalf("model limit edit key %s is missing for %s: %q", key, lang.Code(), got)
			}
		}
	}
	if got := Format(LangZH, KeyModelLimitEditTitle, "openai", "gpt-5.5"); got != "编辑模型限制：openai/gpt-5.5" {
		t.Fatalf("Chinese model limit editor title = %q", got)
	}
	if got := Format(LangZH, KeyModelLimitEditOverridden, "1M"); got != "1M（已覆盖）" {
		t.Fatalf("Chinese overridden context = %q", got)
	}
}
