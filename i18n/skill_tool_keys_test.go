package i18n

import (
	"strings"
	"testing"
)

func TestSkillToolSemanticKeysCoverEveryLanguage(t *testing.T) {
	cases := map[Key][]any{
		KeySkillToolInvalidInput:       nil,
		KeySkillToolRequired:           nil,
		KeySkillToolInvalidSelector:    nil,
		KeySkillToolExplicitUserOrigin: nil,
		KeySkillToolUnavailable:        nil,
		KeySkillToolRecursive:          {"review", "session"},
		KeySkillToolNotFound:           {"review"},
		KeySkillToolAvailable:          {"review, deploy"},
		KeySkillToolNoneInstalled:      nil,
		KeySkillToolAmbiguous:          {"review", "skill:a, skill:b"},
		KeySkillToolShadowed:           {"review", "skill:a"},
		KeySkillToolPolicyDeniedModel:  {"review"},
		KeySkillToolPolicyDeniedUser:   {"review"},
		KeySkillToolStale:              {"review", uint64(2)},
		KeySkillToolRegistryFailure:    {"review"},
		KeySkillToolDenyRule:           {"review", "review"},
	}
	for key, args := range cases {
		for _, lang := range AllLanguages() {
			if got := Format(lang, key, args...); got == "" || strings.HasPrefix(got, "[") || strings.Contains(got, "%!") {
				t.Errorf("Format(%s, %q) = %q, want a valid registered translation", lang.Code(), key, got)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}
