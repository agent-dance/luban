package app

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/google/uuid"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/store/session"
)

type resolvedSession struct {
	Ref        session.Ref
	Resumed    bool
	SessionCWD string
}

// ResolveSession determines the session ref to use and, for resumed sessions,
// returns the stored working directory when available.
func ResolveSession(sessionID string, resume bool, repo *session.Repository, currentCWD string, w io.Writer) (resolvedSession, error) {
	lang := i18n.DetectOrLoadLanguage()
	currentProjectDir := repo.ProjectDirForCWD(currentCWD)

	if trimmed := strings.TrimSpace(sessionID); trimmed != "" {
		ref, err := repo.Resolve(trimmed, currentProjectDir)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				return resolvedSession{}, i18n.WrapInternalErrorInLanguage(lang, i18n.KeyStartupResolveSession, err, trimmed)
			}
			return resolvedSession{Ref: session.Ref{ID: trimmed, ProjectDir: currentProjectDir}}, nil
		}
		meta, _, err := repo.GetMeta(ref.ID, ref.ProjectDir)
		if err != nil {
			return resolvedSession{}, fmt.Errorf("%s", i18n.Format(lang, i18n.KeyStartupLoadSessionMetadata, ref.ID, session.UserFacingError(lang, err)))
		}
		return resolvedSession{
			Ref:        ref,
			Resumed:    true,
			SessionCWD: meta.CWD,
		}, nil
	}

	if resume {
		ref, err := repo.ResolveLatest(currentProjectDir)
		if err != nil {
			if !errors.Is(err, session.ErrNoSessions) {
				return resolvedSession{}, fmt.Errorf("%s", i18n.Format(lang, i18n.KeyStartupResolveLatestSession, session.UserFacingError(lang, err)))
			}
			fmt.Fprint(w, i18n.Format(lang, i18n.KeyStartupLatestSessionWarning, session.UserFacingError(lang, err)))
		} else {
			meta, _, err := repo.GetMeta(ref.ID, ref.ProjectDir)
			if err != nil {
				return resolvedSession{}, fmt.Errorf("%s", i18n.Format(lang, i18n.KeyStartupLoadSessionMetadata, ref.ID, session.UserFacingError(lang, err)))
			}
			return resolvedSession{
				Ref:        ref,
				Resumed:    true,
				SessionCWD: meta.CWD,
			}, nil
		}
	}

	return resolvedSession{
		Ref: session.Ref{
			ID:         uuid.NewString(),
			ProjectDir: currentProjectDir,
		},
	}, nil
}
