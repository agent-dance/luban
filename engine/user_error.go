package engine

import (
	"errors"

	"github.com/agent-dance/luban/i18n"
)

// UserFacingError maps runtime diagnostics to stable localized copy. Callers
// should retain the original error separately when diagnostics are required.
func UserFacingError(lang i18n.Language, err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrSessionDeleted):
		return i18n.Text(lang, i18n.KeyAuxEngineSessionDeleted)
	case errors.Is(err, ErrSessionNotFound):
		return i18n.Text(lang, i18n.KeyAuxEngineSessionNotFound)
	case errors.Is(err, ErrShutdown):
		return i18n.Text(lang, i18n.KeyAuxEngineShutdown)
	case errors.Is(err, ErrNoProvider):
		return i18n.Text(lang, i18n.KeyAuxEngineNoProvider)
	}

	// First-party runtime boundaries use semantic errors so an internal cause
	// remains available to errors.Is/As without leaking diagnostic-only copy.
	// Render those errors in the language captured by the caller rather than a
	// potentially changed process-global language.
	var localized interface {
		Localized(i18n.Language) string
	}
	if errors.As(err, &localized) {
		return localized.Localized(lang)
	}

	// Providers and other runtime integrations are intentionally open-ended.
	// Preserve unknown external diagnostics rather than replacing them with an
	// unhelpful generic message.
	return err.Error()
}
