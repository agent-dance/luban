package i18n

import (
	"strings"
	"testing"
)

func TestToolSegmentKeysCoverEveryRuntimeLanguage(t *testing.T) {
	keys := []Key{
		KeyToolSegmentReadFiles,
		KeyToolSegmentUsedTools,
		KeyToolSegmentIssues,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			if text := Text(lang, key); text == "" || strings.HasPrefix(text, "[") {
				t.Fatalf("Text(%s, %q) = %q", lang.Code(), key, text)
			}
		}
	}
	if got := Text(LangZH, KeyToolSegmentReadFiles); got != "已读取文件" {
		t.Fatalf("Chinese read segment label = %q", got)
	}
	if got := Format(LangEN, KeyToolSegmentUsedTools, 3); got != "Used 3 tools" {
		t.Fatalf("English mixed segment label = %q", got)
	}
	if got := Format(LangZH, KeyToolSegmentIssues, "已使用 3 个工具", 1); got != "已使用 3 个工具 — 1 项异常" {
		t.Fatalf("Chinese issue segment label = %q", got)
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}
