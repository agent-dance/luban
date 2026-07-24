package skills

import (
	"errors"

	"github.com/agent-dance/luban/i18n"
)

// UserFacingError maps stable catalog sentinels to localized copy. Errors
// without a public semantic category use a generic message so validation,
// filesystem, and storage diagnostics cannot leak through presentation code.
// This display projection does not wrap or replace err; callers retain the
// original error chain for diagnostics.
func UserFacingError(lang i18n.Language, err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrSkillNotFound):
		return i18n.Text(lang, i18n.KeyAuxSkillNotFound)
	case errors.Is(err, ErrOverrideRevisionConflict):
		return i18n.Text(lang, i18n.KeyAuxSkillRevisionConflict)
	case errors.Is(err, ErrManagedOverrideReadOnly):
		return i18n.Text(lang, i18n.KeyAuxSkillManagedReadOnly)
	case errors.Is(err, ErrUnsupportedOverrideScope), errors.Is(err, ErrInvalidSkillScope):
		return i18n.Text(lang, i18n.KeyAuxSkillInvalidScope)
	case errors.Is(err, ErrInvalidOverrideSession):
		return i18n.Text(lang, i18n.KeyAuxSkillInvalidSession)
	case errors.Is(err, ErrSkillOverrideStoreMissing):
		return i18n.Text(lang, i18n.KeySkillsUserErrorStoreUnavailable)
	case errors.Is(err, ErrSkillProjectGenerationChanged):
		return i18n.Text(lang, i18n.KeySkillsUserErrorCatalogChanged)
	case errors.Is(err, ErrInvalidSkillID), errors.Is(err, ErrInvalidSkillLocator):
		return i18n.Text(lang, i18n.KeySkillsUserErrorInvalidIdentifier)
	case errors.Is(err, ErrInvalidSkillDigest):
		return i18n.Text(lang, i18n.KeySkillsUserErrorInvalidContent)
	case errors.Is(err, ErrInvalidCatalogRevision), errors.Is(err, ErrInvalidSkillRevision):
		return i18n.Text(lang, i18n.KeySkillsUserErrorInvalidCatalogState)
	case errors.Is(err, ErrInvalidVisibility):
		return i18n.Text(lang, i18n.KeySkillsUserErrorInvalidVisibility)
	case errors.Is(err, ErrInvalidCatalogPolicy):
		return i18n.Text(lang, i18n.KeySkillsUserErrorInvalidPolicy)
	default:
		return i18n.Text(lang, i18n.KeyAuxSkillFailed)
	}
}
