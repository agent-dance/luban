package i18n

import (
	"strings"
	"testing"
)

func TestTerminalSemanticKeysLocalizeCopyAndPreserveTechnicalValues(t *testing.T) {
	for _, lang := range AllLanguages() {
		got := Format(lang, KeyScreenReaderToolStarted, "Bash", "tool-7", "session-3", "/repo", "turn-2", "work-1", "agent-1", "worker")
		for _, value := range []string{"Bash", "tool-7", "session-3", "/repo", "turn-2", "work-1", "agent-1", "worker"} {
			if !strings.Contains(got, value) {
				t.Fatalf("%s translation omitted technical value %q: %q", lang.Code(), value, got)
			}
		}
		if strings.Contains(got, "[") {
			t.Fatalf("%s translation fell back to a missing key: %q", lang.Code(), got)
		}
	}
	if got := Format(LangZH, KeyTerminalTaskHint); !strings.Contains(got, "输入任务") || strings.Contains(got, "Type a task") {
		t.Fatalf("Chinese task hint was not localized: %q", got)
	}
	for key, want := range map[Key]string{
		KeyScreenReaderEvidence:      "工具证据",
		KeyScreenReaderHookSummary:   "Hook 摘要",
		KeyScreenReaderErrorEvidence: "运行时错误证据",
		KeyScreenReaderErrorMetadata: "运行时错误元数据",
	} {
		if got := Format(LangZH, key, "raw-value"); !strings.Contains(got, want) || !strings.Contains(got, "raw-value") {
			t.Errorf("Chinese %s copy is incomplete: %q", key, got)
		}
	}
}
