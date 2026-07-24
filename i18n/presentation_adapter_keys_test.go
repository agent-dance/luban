package i18n

import "testing"

func TestPresentationAdapterKeysAreCompleteForEveryLanguage(t *testing.T) {
	cases := []struct {
		key  Key
		args []any
	}{
		{KeyAdapterAggregateAction, nil},
		{KeyAdapterAggregateMembers, []any{"a,b"}},
		{KeyAdapterAggregateSummary, []any{"Read", 2}},
		{KeyAdapterActionShell, nil},
		{KeyAdapterActionFileRead, nil},
		{KeyAdapterActionFileWrite, nil},
		{KeyAdapterActionSearch, nil},
		{KeyAdapterActionWeb, nil},
		{KeyAdapterActionMCP, nil},
		{KeyAdapterActionAgent, nil},
		{KeyAdapterActionDecision, nil},
		{KeyAdapterActionMessage, nil},
		{KeyAdapterCommandRunning, []any{"status", "show", " target"}},
		{KeyAdapterCommandUnstructured, nil},
		{KeyAdapterCommandTerminal, []any{"status", "succeeded"}},
		{KeyAdapterCommandDisplayRisk, []any{"receipt", "low"}},
		{KeyAdapterCommandNext, []any{"continue"}},
		{KeyAdapterCommandEvidenceRefs, []any{"transcript#4"}},
		{KeyAdapterCommandMoreRetained, nil},
		{KeyAdapterCommandSensitiveHidden, nil},
		{KeyAdapterReviewNext, nil},
	}
	for _, tc := range cases {
		for _, lang := range AllLanguages() {
			got := Format(lang, tc.key, tc.args...)
			if got == "" || got == "["+string(tc.key)+"]" {
				t.Fatalf("key %s is missing for %s: %q", tc.key, lang.Code(), got)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatalf("semantic catalog is incomplete: %v", err)
	}
}
