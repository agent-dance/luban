package session

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestUserFacingErrorUsesSemanticCopy(t *testing.T) {
	tests := []struct {
		err  error
		key  i18n.Key
		name string
	}{
		{ErrNoSessions, i18n.KeyAuxSessionNoSessions, "no sessions"},
		{ErrSessionDeleted, i18n.KeyAuxSessionDeleted, "deleted"},
		{ErrCorruptSessionMetadata, i18n.KeyAuxSessionMetadataCorrupt, "corrupt metadata"},
		{ErrIncompatibleSessionMetadata, i18n.KeyAuxSessionMetadataIncompatible, "incompatible metadata"},
		{fs.ErrNotExist, i18n.KeyAuxSessionNotFound, "not found"},
		{errors.New("session x exists in 2 projects"), i18n.KeyAuxSessionAmbiguous, "ambiguous"},
		{errors.New("database unavailable"), i18n.KeyAuxSessionFailed, "fallback"},
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
}
