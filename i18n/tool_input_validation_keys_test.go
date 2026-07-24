package i18n

import (
	"errors"
	"strings"
	"testing"
)

type localizedToolInputValidationFixture struct {
	tool       string
	field      string
	errorCalls *int
}

func (e localizedToolInputValidationFixture) Error() string {
	if e.errorCalls != nil {
		*e.errorCalls++
	}
	return "raw schema validation detail"
}

func (e localizedToolInputValidationFixture) LocalizedToolInputValidation(lang Language) string {
	issue := Format(lang, KeyToolInputValidationUnexpectedParameter, e.field)
	return Format(lang, KeyToolInputValidationFailedSingle, e.tool, issue)
}

func TestToolInputValidationCatalogCoversEveryLanguageAndPreservesRawValues(t *testing.T) {
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
	errorCalls := 0
	fixture := localizedToolInputValidationFixture{tool: "RawTool_17", field: "raw_field_23", errorCalls: &errorCalls}
	for _, lang := range AllLanguages() {
		for _, key := range toolInputValidationKeys {
			if got := Text(lang, key); got == "" || got == "["+string(key)+"]" {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
		got := FormatToolInputValidationError(lang, fixture)
		for _, raw := range []string{fixture.tool, fixture.field, "InputValidationError"} {
			if !strings.Contains(got, raw) {
				t.Errorf("FormatToolInputValidationError(%s) omitted %q: %q", lang.Code(), raw, got)
			}
		}
	}
	if errorCalls != 0 {
		t.Fatalf("structured validation detail rendered through Error %d times, want 0", errorCalls)
	}
}

func TestToolInputValidationEnglishCompatibility(t *testing.T) {
	fixture := localizedToolInputValidationFixture{tool: "StrictContract", field: "extra"}
	want := "<tool_use_error>InputValidationError: StrictContract failed due to the following issue:\nAn unexpected parameter `extra` was provided</tool_use_error>"
	if got := FormatToolInputValidationError(LangEN, fixture); got != want {
		t.Fatalf("English validation result = %q, want %q", got, want)
	}
}

func TestToolInputValidationFormatterPreservesRawSchemaError(t *testing.T) {
	raw := errors.New("schema path $.payload: raw failure 41")
	for _, lang := range AllLanguages() {
		got := FormatToolInputValidationError(lang, raw)
		if !strings.Contains(got, raw.Error()) {
			t.Errorf("FormatToolInputValidationError(%s) omitted raw schema error: %q", lang.Code(), got)
		}
	}
}
