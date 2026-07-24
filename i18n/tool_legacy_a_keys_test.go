package i18n

import (
	"strings"
	"testing"
)

func TestToolLegacyAKeysCoverEveryLanguage(t *testing.T) {
	if len(toolLegacyAKeys) == 0 {
		t.Fatal("tool legacy A catalog is empty")
	}
	seen := make(map[Key]struct{}, len(toolLegacyAKeys))
	for _, key := range toolLegacyAKeys {
		if _, duplicate := seen[key]; duplicate {
			t.Fatalf("duplicate key in focused catalog: %q", key)
		}
		seen[key] = struct{}{}
		translations, ok := semanticTranslations[key]
		if !ok {
			t.Fatalf("missing semantic catalog entry for %q", key)
		}
		for _, lang := range AllLanguages() {
			if strings.TrimSpace(translations[lang]) == "" {
				t.Errorf("missing %s translation for %q", lang, key)
			}
		}
	}
}

func TestToolLegacyAEnglishCompatibility(t *testing.T) {
	tests := []struct {
		key  Key
		args []any
		want string
	}{
		{KeyToolLegacyAAgentTeamMissing, []any{"core"}, `Team "core" does not exist. Create it with TeamCreate before spawning teammates.`},
		{KeyToolLegacyAConfigUpdated, []any{"theme", "dark"}, "Config updated: theme = dark"},
		{KeyToolLegacyAFilePDFPagesExtracted, []any{3, "/tmp/a.pdf", "42KB"}, "PDF pages extracted: 3 page(s) from /tmp/a.pdf (42KB)"},
		{KeyToolLegacyAImagePlaceholder, []any{"image/png", 12}, "[image: image/png, 12 bytes base64]"},
		{KeyToolLegacyAExitPlanCommitRollback, []any{"rollback failed", "commit failed"}, "commit plan exit: commit failed; restore original plan: rollback failed"},
	}
	for _, test := range tests {
		if got := Format(LangEN, test.key, test.args...); got != test.want {
			t.Errorf("Format(%q) = %q, want %q", test.key, got, test.want)
		}
	}
}
