package i18n

import (
	"strings"
	"testing"
)

func TestSkillsScreenReaderSemanticKeysCoverEveryLanguage(t *testing.T) {
	cases := map[Key][]any{
		KeyScreenReaderSkillCatalogUnavailable: nil,
		KeyScreenReaderSkillInvokerUnavailable: nil,
		KeyScreenReaderSkillLookupFailed:       {"catalog error"},
		KeyScreenReaderSkillInvalidSelector:    {"/skill:bad"},
		KeyScreenReaderSkillNotFound:           {"/review"},
		KeyScreenReaderSkillAmbiguous:          {"/review", "skill:project:a, skill:user:b"},
		KeyScreenReaderSkillUnavailable:        {"/review"},
		KeyScreenReaderSkillInvocationFailed:   {"/review", "invoke error"},
		KeyScreenReaderSkillInvocationRejected: {"/review"},
		KeyScreenReaderSkillEmptyEnvelope:      {"/review"},
		KeyScreenReaderSkillTranscriptInvoke:   {"/review", "arguments provided"},
		KeyScreenReaderSkillArgumentsProvided:  nil,
		KeyScreenReaderSkillArgumentsOmitted:   nil,
	}

	for key, args := range cases {
		for _, lang := range AllLanguages() {
			got := Format(lang, key, args...)
			if got == "" || strings.HasPrefix(got, "[") || strings.Contains(got, "%!") {
				t.Errorf("Format(%s, %q) = %q, want a valid registered translation", lang.Code(), key, got)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}
