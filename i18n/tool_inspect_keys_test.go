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
		KeyToolInspectRequestsRequired: "at least one Inspect request is required",
		KeyToolInspectCursorInvalid:    "Inspect cursor is invalid, expired, already consumed, or belongs to another workspace",
	}
	for key, want := range cases {
		if got := Text(LangEN, key); got != want {
			t.Errorf("Text(LangEN, %s) = %q, want %q", key, got, want)
		}
	}
}

func TestToolInspectDiscriminatedInputCopyDoesNotAdvertiseNullSentinels(t *testing.T) {
	for _, key := range []Key{
		KeyToolInspectInputOperationDescription,
		KeyToolInspectInputModeDescription,
		KeyToolInspectInputRequestsDescription,
		KeyToolInspectInputCursorDescription,
		KeyToolInspectInputPageDescription,
	} {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if strings.Contains(strings.ToLower(got), "null") {
				t.Fatalf("%s translation for %s still advertises a null sentinel: %q", lang.Code(), key, got)
			}
		}
	}
}
