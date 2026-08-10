package i18n

import (
	"strings"
	"testing"
)

func TestLoopFlightKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range loopFlightKeys {
		for _, lang := range AllLanguages() {
			copy := Text(lang, key)
			if copy == "" || copy == "["+string(key)+"]" || strings.Contains(copy, "%!") {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, copy)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestLoopWorkspaceUnknownCopyPreservesRecoveryToolIDs(t *testing.T) {
	for _, lang := range AllLanguages() {
		copy := Text(lang, KeyLoopVisibleFlightWorkspaceUnknown)
		for _, toolID := range []string{"ApplyPatch", "Run"} {
			if !strings.Contains(copy, toolID) {
				t.Errorf("Text(%s, %q) omitted %q: %q", lang.Code(), KeyLoopVisibleFlightWorkspaceUnknown, toolID, copy)
			}
		}
	}
	english := Text(LangEN, KeyLoopVisibleFlightWorkspaceUnknown)
	for _, boundary := range []string{"no-op patch", "lacks authority"} {
		if !strings.Contains(english, boundary) {
			t.Errorf("workspace-unknown recovery copy omitted %q: %q", boundary, english)
		}
	}
}

func TestLoopInvestigationNudgePreservesActionToolIDs(t *testing.T) {
	for _, lang := range AllLanguages() {
		copy := Text(lang, KeyLoopVisibleFlightInvestigationNudge)
		for _, toolID := range []string{"ApplyPatch"} {
			if !strings.Contains(copy, toolID) {
				t.Errorf("Text(%s, %q) omitted %q: %q", lang.Code(), KeyLoopVisibleFlightInvestigationNudge, toolID, copy)
			}
		}
	}
}

func TestLoopVerificationConvergenceNudgePreservesBoundaries(t *testing.T) {
	for _, lang := range AllLanguages() {
		copy := Text(lang, KeyLoopVisibleFlightVerificationConvergence)
		if copy == "" || copy == "["+string(KeyLoopVisibleFlightVerificationConvergence)+"]" {
			t.Errorf("Text(%s, %q) = %q", lang.Code(), KeyLoopVisibleFlightVerificationConvergence, copy)
		}
	}
	english := Text(LangEN, KeyLoopVisibleFlightVerificationConvergence)
	for _, boundary := range []string{"code-related failure", "environmental"} {
		if !strings.Contains(english, boundary) {
			t.Errorf("verification convergence copy omitted %q: %q", boundary, english)
		}
	}
}
