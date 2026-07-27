package i18n

import (
	"strings"
	"testing"
)

func TestToolInspectKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolInspectKeys {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if strings.TrimSpace(got) == "" || got == "["+string(key)+"]" {
				t.Fatalf("%s translation for %s = %q", lang.Code(), key, got)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestToolInspectEnglishRuntimeContract(t *testing.T) {
	cases := map[Key]string{
		KeyToolInspectChooseRequestsOrCursor: "provide requests or cursor, but not both",
		KeyToolInspectRequestsRequired:       "at least one Inspect request is required",
		KeyToolInspectCursorInvalid:          "Inspect cursor is invalid, expired, already consumed, or belongs to another workspace",
	}
	for key, want := range cases {
		if got := Text(LangEN, key); got != want {
			t.Errorf("Text(LangEN, %s) = %q, want %q", key, got, want)
		}
	}
}
