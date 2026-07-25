package i18n

import (
	"strings"
	"testing"
)

func TestToolAgentProfileCopyCoversEveryLanguage(t *testing.T) {
	for _, lang := range AllLanguages() {
		if got := strings.TrimSpace(Text(lang, KeyToolAgentProfileDescriptionMissing)); got == "" {
			t.Fatalf("missing profile-description copy for %s", lang.Code())
		}
		if got := strings.TrimSpace(Text(lang, KeyToolAgentProfileNoTools)); got == "" {
			t.Fatalf("missing no-tools copy for %s", lang.Code())
		}
		if got := strings.TrimSpace(Text(lang, KeyToolAgentProfileAllTools)); got == "" {
			t.Fatalf("missing all-tools copy for %s", lang.Code())
		}
		if got := Format(lang, KeyToolAgentProfileAllToolsExcept, "Write"); !strings.Contains(got, "Write") {
			t.Fatalf("all-tools-except copy for %s lost tool ID: %q", lang.Code(), got)
		}
		line := Format(lang, KeyToolAgentProfileLine, "reviewer", "review changes", "Read")
		for _, raw := range []string{"reviewer", "review changes", "Read"} {
			if !strings.Contains(line, raw) {
				t.Fatalf("profile line for %s lost %q: %q", lang.Code(), raw, line)
			}
		}
	}
}
