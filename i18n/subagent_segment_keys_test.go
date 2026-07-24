package i18n

import (
	"strings"
	"testing"
)

func TestSubagentSegmentKeysCoverEveryRuntimeLanguage(t *testing.T) {
	keys := []Key{
		KeySubagentSegmentMessages,
		KeySubagentSegmentTurns,
		KeySubagentSegmentWaiting,
		KeySubagentSegmentResultPendingView,
		KeySubagentSegmentResultSummary,
		KeySubagentSegmentUsage,
		KeySubagentSegmentUsageUnknownCost,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if text := Text(lang, key); text == "" || strings.HasPrefix(text, "[") {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, text)
			}
		}
	}
	if got := Format(LangEN, KeySubagentSegmentMessages, 3); got != "Messages: 3" {
		t.Fatalf("English message count = %q", got)
	}
	if got := Format(LangZH, KeySubagentSegmentTurns, 3); got != "轮次：3" {
		t.Fatalf("Chinese turn count = %q", got)
	}
	if got := Format(LangZH, KeySubagentSegmentUsage, "19.3K", 80, "420", 0.1234); got != "输入 19.3K · 缓存 80% · 输出 420 · $0.1234" {
		t.Fatalf("Chinese subagent usage = %q", got)
	}
	if got := Format(LangEN, KeySubagentSegmentUsageUnknownCost, "19.3K", 80, "420"); got != "in 19.3K · 80% cached · out 420 · cost unknown" {
		t.Fatalf("English subagent usage with unknown cost = %q", got)
	}
	checks := map[Key]string{
		KeySubagentSegmentResultPendingView: "结果待查看",
		KeySubagentSegmentTurns:             "轮次：%d",
		KeySubagentSegmentResultSummary:     "结果摘要",
	}
	for key, want := range checks {
		if got := Text(LangZH, key); got != want {
			t.Errorf("Text(zh, %q) = %q, want %q", key, got, want)
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}
