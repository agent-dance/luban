package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestToolSourceSinkResidualKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolSourceSinkResidualKeys {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if got == "" || got == "["+string(key)+"]" {
				t.Fatalf("%s is missing for %s", key, lang.Code())
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatalf("semantic catalog is incomplete: %v", err)
	}
}

func TestToolSourceSinkResidualEnglishCompatibility(t *testing.T) {
	cases := map[string]string{
		Text(LangEN, KeyCompactSummaryNoSummarizer):                                         "no summarizer configured",
		Format(LangEN, KeyCompactSummaryPTLRetriesExhausted, 3):                             "compact summary prompt-too-long retry exhausted after 3 attempts",
		Text(LangEN, KeyCompactSummaryPTLHistoryPreserved):                                  "compact summary input exceeds the context window; conversation history was preserved",
		Format(LangEN, KeyToolSourceSinkReadDirectory, "/tmp/dir"):                          `EISDIR: illegal operation on a directory, read "/tmp/dir"`,
		Format(LangEN, KeyToolSourceSinkParseMarshal, "raw-cause"):                          "failed to marshal input: raw-cause",
		Format(LangEN, KeyToolSourceSinkConfigCreateDirectory, "raw-cause"):                 "failed to create config directory: raw-cause",
		Format(LangEN, KeyToolSourceSinkAtomicCreateTemporary, "raw-cause"):                 "create temp file: raw-cause",
		Format(LangEN, KeyToolSourceSinkReadImageEmpty, "/tmp/a.png"):                       "Image file is empty: /tmp/a.png",
		Format(LangEN, KeyToolSourceSinkReadPNGTooLarge, 20000, 17000, 16384):               "PNG dimensions 20000x17000 exceed the maximum allowed (16384). Refusing to decode.",
		Format(LangEN, KeyToolSourceSinkNotebookCellSource, "raw-cause"):                    "failed to parse notebook cell source: raw-cause",
		Format(LangEN, KeyToolSourceSinkSearchInvalidRegex, "raw-cause"):                    "invalid regex pattern: raw-cause",
		Text(LangEN, KeyToolSourceSinkSearchInvalidContext):                                 "ripgrep usage error: context values must be non-negative integers",
		Format(LangEN, KeyToolSourceSinkSearchOutsideAllowed, "/private/repository"):        "path is outside allowed directories: /private/repository",
		Format(LangEN, KeyToolSourceSinkMCPReadSettings, "/tmp/settings.json", "raw-cause"): `read MCP settings /tmp/settings.json: raw-cause`,
		Format(LangEN, KeyToolSourceSinkMCPNotConfigured, "docs"):                           `MCP server "docs" not configured`,
		Format(LangEN, KeyToolSourceSinkMCPConnectTimeout, "docs"):                          `connect MCP server "docs": timed out after 30s`,
		Format(LangEN, KeyToolSourceSinkMCPListTools, "docs", "raw-cause"):                  `list tools from MCP server "docs": raw-cause`,
		Text(LangEN, KeyToolSourceSinkWorktreeRuntimeMissing):                               `worktree runtime is not configured`,
		Format(LangEN, KeyToolSourceSinkWorktreeCWDUnavailable, "/tmp/wt", "raw"):           `worktree cwd "/tmp/wt" is unavailable: raw`,
		Format(LangEN, KeyToolSourceSinkWorktreePersistSession, "raw-cause"):                `persist worktree session: raw-cause`,
	}
	for got, want := range cases {
		if got != want {
			t.Fatalf("English compatibility changed: got %q want %q", got, want)
		}
	}
}

func TestToolSourceSinkResidualErrorsLocalizeAtRenderAndPreserveCauses(t *testing.T) {
	previous := detectedLanguageCache.Load()
	t.Cleanup(func() { detectedLanguageCache.Store(previous) })

	cause := errors.New("raw-source-sink-cause")
	err := WrapError(KeyToolSourceSinkAtomicWriteTemporary, cause)
	if !errors.Is(err, cause) {
		t.Fatal("semantic error did not preserve its cause")
	}

	detectedLanguageCache.Store(int32(LangEN))
	english := err.Error()
	detectedLanguageCache.Store(int32(LangZH))
	chinese := err.Error()
	if english == chinese || !strings.Contains(chinese, cause.Error()) {
		t.Fatalf("runtime localization failed: en=%q zh=%q", english, chinese)
	}
}
