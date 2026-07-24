package types

import (
	"errors"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestToolInputValidationErrorLocalizesStructuredDetail(t *testing.T) {
	err := ValidateToolInput(contractTestTool{}, map[string]any{
		"value":   "ok",
		"z_field": true,
		"a_field": true,
	})
	if err == nil {
		t.Fatal("ValidateToolInput returned nil")
	}
	var validationErr *ToolInputValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("ValidateToolInput error type = %T, want *ToolInputValidationError", err)
	}

	english := validationErr.LocalizedToolInputValidation(i18n.LangEN)
	wantEnglish := "ContractTest failed due to the following issues:\nAn unexpected parameter `a_field` was provided\nAn unexpected parameter `z_field` was provided"
	if english != wantEnglish {
		t.Fatalf("English validation detail = %q, want %q", english, wantEnglish)
	}
	chinese := validationErr.LocalizedToolInputValidation(i18n.LangZH)
	if chinese == english {
		t.Fatalf("localized validation detail did not change: %q", chinese)
	}
	for _, raw := range []string{"ContractTest", "a_field", "z_field"} {
		if !strings.Contains(chinese, raw) {
			t.Errorf("localized validation detail omitted raw value %q: %q", raw, chinese)
		}
	}
}
