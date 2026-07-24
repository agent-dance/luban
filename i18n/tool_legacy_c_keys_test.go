package i18n

import (
	"strings"
	"testing"
)

func TestToolLegacyCKeysCoverEveryLanguage(t *testing.T) {
	tested := 0
	for key := range semanticTranslations {
		if !strings.HasPrefix(string(key), "tool.legacy_c.") {
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
		t.Fatal("no tool legacy C keys were registered")
	}
}

func TestToolLegacyCEnglishCompatibilityAndRawValues(t *testing.T) {
	tests := []struct {
		key  Key
		args []any
		want string
	}{
		{KeyToolLegacyCFoundTotalAcross, []any{2, "occurrences", 1, "file"}, "Found 2 total occurrences across 1 file."},
		{KeyToolLegacyCShutdownRejected, []any{"policy says no"}, `Shutdown rejected. Reason: "policy says no". Continuing to work.`},
		{KeyToolLegacyCTaskCreated, []any{"17", "raw subject"}, "Task #17 created successfully: raw subject"},
		{KeyToolLegacyCAttachmentMissing, []any{"raw.png", "/tmp/raw"}, `attachment "raw.png" does not exist. Current working directory: /tmp/raw`},
	}
	for _, test := range tests {
		if got := Format(LangEN, test.key, test.args...); got != test.want {
			t.Errorf("%s = %q, want %q", test.key, got, test.want)
		}
	}
}
