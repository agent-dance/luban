package i18n

import (
	"strings"
	"testing"
)

func TestProgressiveStatusSemanticCopyCoversEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyTUIStatusCompactionCount, KeyTUIStatusProgressiveSavings,
		KeyTUIStatusProgressivePending, KeyTUIStatusProgressiveSavingsAndPending,
	}
	for _, language := range AllLanguages() {
		for _, key := range keys {
			if got := Text(language, key); strings.TrimSpace(got) == "" || got == string(key) {
				t.Fatalf("missing %s translation for %s: %q", key, language, got)
			}
		}
	}
	if got := Format(LangZH, KeyTUIStatusProgressiveSavings, "18.1K"); got != "渐进压缩  ✓已省18.1K" {
		t.Fatalf("Chinese progressive status = %q", got)
	}
	if got := Format(LangZH, KeyTUIStatusProgressivePending, 2, "4.2K"); got != "渐进压缩  …2项预计省4.2K" {
		t.Fatalf("Chinese progressive pending status = %q", got)
	}
	if got := Format(LangZH, KeyTUIStatusProgressiveSavingsAndPending, "18.1K", 2, "4.2K"); got != "渐进压缩  ✓已省18.1K │ …2项预计省4.2K" {
		t.Fatalf("Chinese progressive combined status = %q", got)
	}
}
