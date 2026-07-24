package i18n

import (
	"strings"
	"testing"
)

func TestRootRuntimeErrorKeysCoverEveryLanguage(t *testing.T) {
	tested := 0
	for key := range semanticTranslations {
		if !strings.HasPrefix(string(key), "root.") {
			continue
		}
		tested++
		for _, lang := range AllLanguages() {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
	if tested < 80 {
		t.Fatalf("tested only %d root runtime keys", tested)
	}
}

func TestRootRuntimeLabelsLocalizeKnownCodesAndPreserveExtensions(t *testing.T) {
	if got := RootGoalStatusLabel(LangZH, "blocked"); got != "受阻" {
		t.Fatalf("blocked = %q", got)
	}
	if got := RootAgentPhaseLabel(LangJA, "mcp_ready"); got != "MCP 準備完了" {
		t.Fatalf("mcp_ready = %q", got)
	}
	if got := RootAgentQueueReasonLabel(LangDE, "dependency:active_run"); got != "wartet auf den aktiven Lauf" {
		t.Fatalf("queue reason = %q", got)
	}
	if got := RootAgentTerminalReasonLabel(LangKO, "process_restart"); got != "프로세스 재시작으로 중단됨" {
		t.Fatalf("terminal reason = %q", got)
	}
	stored := Format(LangDE, KeyRootGoalReasonEvaluatorFailed, "raw provider detail")
	if got := RootGoalEvaluatorReasonLabel(LangZH, stored); got != "目标评估失败：raw provider detail" {
		t.Fatalf("persisted evaluator reason = %q", got)
	}
	for _, got := range []string{
		RootGoalStatusLabel(LangZH, "extension_status"),
		RootAgentPhaseLabel(LangZH, "extension_phase"),
		RootAgentQueueReasonLabel(LangZH, "extension:reason"),
		RootAgentTerminalReasonLabel(LangZH, "extension_reason"),
	} {
		if !strings.HasPrefix(got, "extension") {
			t.Fatalf("extension identifier was translated: %q", got)
		}
	}
}

func TestRootGoalEvaluatorReasonStateLabelRendersStructuredReasons(t *testing.T) {
	if got := RootGoalEvaluatorReasonStateLabel(LangZH, "old English", "evaluator_unavailable", "", ""); got != Text(LangZH, KeyRootGoalReasonEvaluatorUnavailable) {
		t.Fatalf("unavailable reason = %q", got)
	}
	wantDetail := Format(LangZH, KeyLoopGoalEvaluatorProviderCallFailed, "raw provider detail")
	want := Format(LangZH, KeyRootGoalReasonEvaluatorFailed, wantDetail)
	if got := RootGoalEvaluatorReasonStateLabel(LangZH, "old English", "evaluator_failed", string(KeyLoopGoalEvaluatorProviderCallFailed), "raw provider detail"); got != want {
		t.Fatalf("failed reason = %q, want %q", got, want)
	}
	if got := RootGoalEvaluatorReasonStateLabel(LangJA, "old English", "model_marked_complete", "", ""); got != Text(LangJA, KeyToolGoalReasonComplete) {
		t.Fatalf("model-complete reason = %q", got)
	}
	if got := RootGoalEvaluatorReasonStateLabel(LangKO, "evaluator authored", "", "", ""); got != "evaluator authored" {
		t.Fatalf("authored reason changed: %q", got)
	}
}

func TestRootRuntimeFormattingPreservesRawIdentifiers(t *testing.T) {
	got := Format(LangZH, KeyRootForkTerminalUnavailable, "session-7", "luban", "session-7")
	for _, want := range []string{"session-7", "luban", "--session-id"} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted fork message %q missing %q", got, want)
		}
	}
}

func TestRootGoalTransitionFormattingUsesLocaleWordOrder(t *testing.T) {
	for _, lang := range AllLanguages() {
		got := Format(lang, KeyRootGoalTransitionInvalid,
			RootGoalActionLabel(lang, "edit"), RootGoalStatusLabel(lang, "blocked"))
		if strings.Contains(got, "%!") || strings.TrimSpace(got) == "" {
			t.Fatalf("Format(%s, transition) = %q", lang.Code(), got)
		}
	}
}
