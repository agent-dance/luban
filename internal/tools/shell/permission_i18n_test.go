package shell

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestBashSecurityRulesUseSemanticReasonKeys(t *testing.T) {
	for _, rule := range bashSecurityRules {
		if rule.ReasonKey == "" {
			t.Errorf("security rule %q has no semantic reason key", rule.Name)
			continue
		}
		for _, lang := range i18n.AllLanguages() {
			message := i18n.Format(lang, rule.ReasonKey, rule.ReasonArgs...)
			if message == "" || strings.HasPrefix(message, "[") {
				t.Errorf("security rule %q reason in %s = %q", rule.Name, lang.Code(), message)
			}
		}
	}
}

func TestBashPermissionDetectionReturnsSemanticReasonKeys(t *testing.T) {
	tests := []struct {
		command string
		want    i18n.Key
	}{
		{"echo hi > >(tee output.txt)", i18n.KeyToolPermissionBashProcessSubstitution},
		{"cd one && cd two && pwd", i18n.KeyToolPermissionBashMultipleDirectories},
		{"cd repo && git status", i18n.KeyToolPermissionBashCDAndGit},
		{"cd .luban-code && echo x > settings.json", i18n.KeyToolPermissionBashCDAndRedirect},
	}
	for _, test := range tests {
		needed, got := bashPermissionApprovalReason(test.command)
		if !needed || got != test.want {
			t.Errorf("bashPermissionApprovalReason(%q) = (%v, %q), want (true, %q)", test.command, needed, got, test.want)
		}
	}
}

func TestCompoundCDViolationsUseSemanticReasonKeys(t *testing.T) {
	violations := findCompoundCDViolations("cd one && cd two")
	if len(violations) != 1 || violations[0].ReasonKey != i18n.KeyBashSecurityCompoundMultipleCD {
		t.Fatalf("violations = %#v", violations)
	}
	if got := compoundCDViolationReasons(violations); got == "" || strings.HasPrefix(got, "[") {
		t.Fatalf("localized violations = %q", got)
	}
}
