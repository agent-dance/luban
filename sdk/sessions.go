package sdk

import (
	"regexp"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/store/session"
)

// safeSessionID matches session IDs that are safe for use as filesystem names.
// Accepts alphanumeric characters plus hyphens and underscores; must start
// with an alphanumeric character to prevent leading-dot or leading-slash tricks.
var safeSessionID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// SessionInfo holds metadata about a stored session.
type SessionInfo struct {
	ID           string    `json:"id"`
	ProjectDir   string    `json:"project_dir,omitempty"`
	Title        string    `json:"title,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
	MessageCount int       `json:"message_count"`
	Model        string    `json:"model,omitempty"`
}

// validateSessionID checks that an ID is safe for filesystem use,
// rejecting path traversal attempts and unsafe characters.
func validateSessionID(id string) error {
	if id == "" {
		return i18n.NewError(i18n.KeySDKSessionsIDRequired)
	}
	if len(id) > 128 {
		return i18n.NewError(i18n.KeySDKSessionsIDTooLong, 128)
	}
	if !safeSessionID.MatchString(id) {
		return i18n.NewError(i18n.KeySDKSessionsIDInvalid, id)
	}
	return nil
}

// ListSessions returns metadata for all saved sessions ordered newest-first.
// It returns an empty list when no project session store exists yet.
func ListSessions() ([]SessionInfo, error) {
	repo := session.DefaultRepository()
	results, err := repo.Search(session.SearchOptions{AllProjects: true})
	if err != nil {
		return nil, i18n.WrapError(i18n.KeySDKSessionsListFailed, err)
	}
	out := make([]SessionInfo, len(results))
	for i, result := range results {
		out[i] = SessionInfo{
			ID:           result.ID,
			ProjectDir:   result.ProjectDir,
			Title:        result.Title,
			CreatedAt:    result.CreatedAt,
			UpdatedAt:    result.UpdatedAt,
			MessageCount: result.MessageCount,
			Model:        result.Model,
		}
	}
	return out, nil
}

// GetSession returns detailed metadata for a single session, including the
// message count and a title derived from the first user message.
// Returns an error if the session does not exist or the ID is invalid.
func GetSession(id string) (*SessionInfo, error) {
	if err := validateSessionID(id); err != nil {
		return nil, err
	}

	repo := session.DefaultRepository()
	meta, ref, err := repo.GetMeta(id, "")
	if err != nil {
		return nil, err
	}
	return &SessionInfo{
		ID:           id,
		ProjectDir:   ref.ProjectDir,
		Title:        meta.Title,
		CreatedAt:    meta.CreatedAt,
		UpdatedAt:    meta.UpdatedAt,
		MessageCount: meta.MessageCount,
		Model:        meta.Model,
	}, nil
}

// DeleteSession removes a session and its stored messages.
// Returns an error if the session does not exist or the ID is invalid.
func DeleteSession(id string) error {
	if err := validateSessionID(id); err != nil {
		return err
	}
	return session.DefaultRepository().Delete(id, "")
}
