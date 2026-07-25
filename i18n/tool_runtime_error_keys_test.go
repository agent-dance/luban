package i18n

import (
	"strings"
	"testing"
)

func TestToolRuntimeKeysCoverEveryLanguage(t *testing.T) {
	tested := 0
	for key := range semanticTranslations {
		if !strings.HasPrefix(string(key), "tool.runtime.") {
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
	if tested == 0 {
		t.Fatal("no tool runtime semantic keys were registered")
	}
}

func TestToolRuntimeFormattingPreservesRawValues(t *testing.T) {
	got := Format(LangZH, KeyToolRuntimeInvalidInput, `unknown field "raw_id"`)
	if !strings.Contains(got, `unknown field "raw_id"`) {
		t.Fatalf("formatted tool error lost raw value: %q", got)
	}
}

func TestToolRuntimeEditInvalidDataCoversEveryLanguage(t *testing.T) {
	for _, lang := range AllLanguages() {
		got := Text(lang, KeyToolRuntimeEditInvalidData)
		if got == "" || got == "["+string(KeyToolRuntimeEditInvalidData)+"]" {
			t.Fatalf("Edit invalid-data copy is missing for %s: %q", lang.Code(), got)
		}
	}
}

func TestToolRuntimeNotebookInvalidDataCoversEveryLanguage(t *testing.T) {
	for _, lang := range AllLanguages() {
		got := Text(lang, KeyToolRuntimeNotebookInvalidData)
		if got == "" || got == "["+string(KeyToolRuntimeNotebookInvalidData)+"]" {
			t.Fatalf("NotebookEdit invalid-data copy is missing for %s: %q", lang.Code(), got)
		}
	}
}

func TestToolRuntimeBashSedReadRequiredCoversEveryLanguage(t *testing.T) {
	const path = "/workspace/raw-target.txt"
	for _, lang := range AllLanguages() {
		got := Format(lang, KeyToolRuntimeBashSedReadRequired, path)
		if got == "" || got == "["+string(KeyToolRuntimeBashSedReadRequired)+"]" || !strings.Contains(got, path) {
			t.Fatalf("bash sed read-required copy for %s lost semantic text or raw path: %q", lang.Code(), got)
		}
	}
}

func TestWorktreeCleanupIncompleteIsLocalizedWithoutPrivateCauseSlot(t *testing.T) {
	for _, lang := range AllLanguages() {
		got := Format(lang, KeyToolRuntimeWorktreeCleanupIncomplete, "/raw/worktree", "/raw/original")
		if !strings.Contains(got, "/raw/worktree") || !strings.Contains(got, "/raw/original") {
			t.Fatalf("%s cleanup message lost external path parameters: %q", lang.Code(), got)
		}
		if strings.Contains(got, "%!") {
			t.Fatalf("%s cleanup message has an unresolved formatting slot: %q", lang.Code(), got)
		}
	}
}
