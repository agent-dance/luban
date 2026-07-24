package skills

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestUserFacingErrorUsesSemanticCopy(t *testing.T) {
	tests := []struct {
		err  error
		key  i18n.Key
		name string
	}{
		{ErrSkillNotFound, i18n.KeyAuxSkillNotFound, "not found"},
		{ErrOverrideRevisionConflict, i18n.KeyAuxSkillRevisionConflict, "revision"},
		{ErrManagedOverrideReadOnly, i18n.KeyAuxSkillManagedReadOnly, "managed"},
		{ErrUnsupportedOverrideScope, i18n.KeyAuxSkillInvalidScope, "scope"},
		{ErrInvalidSkillScope, i18n.KeyAuxSkillInvalidScope, "invalid scope"},
		{ErrInvalidOverrideSession, i18n.KeyAuxSkillInvalidSession, "session"},
		{ErrSkillOverrideStoreMissing, i18n.KeySkillsUserErrorStoreUnavailable, "store unavailable"},
		{ErrSkillProjectGenerationChanged, i18n.KeySkillsUserErrorCatalogChanged, "project generation"},
		{ErrInvalidSkillID, i18n.KeySkillsUserErrorInvalidIdentifier, "skill ID"},
		{ErrInvalidSkillLocator, i18n.KeySkillsUserErrorInvalidIdentifier, "skill locator"},
		{ErrInvalidSkillDigest, i18n.KeySkillsUserErrorInvalidContent, "skill digest"},
		{ErrInvalidCatalogRevision, i18n.KeySkillsUserErrorInvalidCatalogState, "catalog revision"},
		{ErrInvalidSkillRevision, i18n.KeySkillsUserErrorInvalidCatalogState, "skill revision"},
		{ErrInvalidVisibility, i18n.KeySkillsUserErrorInvalidVisibility, "visibility"},
		{ErrInvalidCatalogPolicy, i18n.KeySkillsUserErrorInvalidPolicy, "policy"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, lang := range i18n.AllLanguages() {
				wrapped := fmt.Errorf("internal storage context: %w", tc.err)
				if got, want := UserFacingError(lang, wrapped), i18n.Text(lang, tc.key); got != want {
					t.Fatalf("UserFacingError(%s) = %q, want %q", lang.Code(), got, want)
				}
			}
		})
	}
	if got := UserFacingError(i18n.LangZH, nil); got != "" {
		t.Fatalf("nil error = %q", got)
	}
}

func TestUserFacingErrorDoesNotExposeUnknownDiagnostics(t *testing.T) {
	diagnostic := "parse /secret/project/settings.json: injected storage failure"
	err := errors.New(diagnostic)
	for _, lang := range i18n.AllLanguages() {
		got := UserFacingError(lang, err)
		if want := i18n.Text(lang, i18n.KeyAuxSkillFailed); got != want {
			t.Fatalf("UserFacingError(%s) = %q, want %q", lang.Code(), got, want)
		}
		if strings.Contains(got, diagnostic) || strings.Contains(got, "/secret/project/settings.json") {
			t.Fatalf("UserFacingError(%s) leaked diagnostic: %q", lang.Code(), got)
		}
	}
}
