package engine

import (
	"errors"
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
		{ErrSessionDeleted, i18n.KeyAuxEngineSessionDeleted, "deleted"},
		{ErrSessionNotFound, i18n.KeyAuxEngineSessionNotFound, "not found"},
		{ErrShutdown, i18n.KeyAuxEngineShutdown, "shutdown"},
		{ErrNoProvider, i18n.KeyAuxEngineNoProvider, "no provider"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, lang := range i18n.AllLanguages() {
				if got, want := UserFacingError(lang, tc.err), i18n.Text(lang, tc.key); got != want {
					t.Fatalf("UserFacingError(%s) = %q, want %q", lang.Code(), got, want)
				}
			}
		})
	}
	if got := UserFacingError(i18n.LangZH, errors.New("provider unavailable")); got != "provider unavailable" {
		t.Fatalf("external Provider error = %q", got)
	}
}

func TestUserFacingErrorRendersSemanticBoundaryErrorInRequestedLanguage(t *testing.T) {
	cause := errors.New("internal save diagnostic")
	err := i18n.WrapInternalError(i18n.KeyEngineSessionSaveFailed, cause)

	for _, lang := range i18n.AllLanguages() {
		if got, want := UserFacingError(lang, err), i18n.Text(lang, i18n.KeyEngineSessionSaveFailed); got != want {
			t.Fatalf("UserFacingError(%s) = %q, want %q", lang.Code(), got, want)
		}
	}
	if !errors.Is(err, cause) {
		t.Fatal("semantic boundary error did not preserve its cause")
	}
	if got := UserFacingError(i18n.LangEN, err); strings.Contains(got, cause.Error()) {
		t.Fatalf("internal diagnostic leaked into user-facing copy: %q", got)
	}
}
