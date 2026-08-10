package session

import (
	"errors"
	"io/fs"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// UserFacingError maps repository diagnostics to stable localized copy. The
// original error should still be retained by callers that need diagnostics.
func UserFacingError(lang i18n.Language, err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrNoSessions):
		return i18n.Text(lang, i18n.KeyAuxSessionNoSessions)
	case errors.Is(err, ErrSessionDeleted):
		return i18n.Text(lang, i18n.KeyAuxSessionDeleted)
	case errors.Is(err, ErrCorruptSessionMetadata):
		return i18n.Text(lang, i18n.KeyAuxSessionMetadataCorrupt)
	case errors.Is(err, ErrIncompatibleSessionMetadata):
		return i18n.Text(lang, i18n.KeyAuxSessionMetadataIncompatible)
	case strings.Contains(err.Error(), "exists in") && strings.Contains(err.Error(), "projects"):
		return i18n.Text(lang, i18n.KeyAuxSessionAmbiguous)
	case errors.Is(err, fs.ErrNotExist):
		return i18n.Text(lang, i18n.KeyAuxSessionNotFound)
	default:
		return i18n.Text(lang, i18n.KeyAuxSessionFailed)
	}
}
