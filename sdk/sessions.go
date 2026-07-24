package sdk

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/session"
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

// sessionsDirOverride is a test hook; when non-nil it replaces the real
// sessionsDir lookup. Set this in test code only.
var sessionsDirOverride func() (string, error)

// sessionsDir returns the path to the session storage directory.
// Defaults to the LUBAN Code session directory.
func sessionsDir() (string, error) {
	if sessionsDirOverride != nil {
		return sessionsDirOverride()
	}
	if brand.HomeDir() == "" {
		return "", i18n.NewError(i18n.KeySDKSessionsHomeUnavailable)
	}
	return brand.SessionsDir(), nil
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
// Returns nil, nil when the sessions directory does not yet exist.
func ListSessions() ([]SessionInfo, error) {
	if sessionsDirOverride == nil {
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

	dir, err := sessionsDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, i18n.WrapError(i18n.KeySDKSessionsListFailed, err)
	}

	var sessions []SessionInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".jsonl")
		fi, err := entry.Info()
		if err != nil {
			slog.Warn(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogSDKSessionStatFailed), "name", entry.Name(), "error", err)
			continue
		}
		sessions = append(sessions, SessionInfo{
			ID:        id,
			UpdatedAt: fi.ModTime(),
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}

// GetSession returns detailed metadata for a single session, including the
// message count and a title derived from the first user message.
// Returns an error if the session does not exist or the ID is invalid.
func GetSession(id string) (*SessionInfo, error) {
	if err := validateSessionID(id); err != nil {
		return nil, err
	}

	if sessionsDirOverride == nil {
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

	dir, err := sessionsDir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, id+".jsonl")

	// Defence-in-depth: verify the resolved path stays under the sessions dir.
	if rel, err := filepath.Rel(dir, path); err != nil || strings.HasPrefix(rel, "..") {
		return nil, i18n.NewError(i18n.KeySDKSessionsPathEscapes)
	}

	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, i18n.NewError(i18n.KeySDKSessionsNotFound, id)
		}
		return nil, i18n.WrapError(i18n.KeySDKSessionsStatFailed, err)
	}

	count, title, readErr := readSessionMessages(path)
	if readErr != nil {
		slog.Warn(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogSDKSessionPartialRead), "id", id, "error", readErr)
	}

	return &SessionInfo{
		ID:           id,
		Title:        title,
		UpdatedAt:    stat.ModTime(),
		MessageCount: count,
	}, nil
}

// DeleteSession removes a session and its stored messages.
// Returns an error if the session does not exist or the ID is invalid.
func DeleteSession(id string) error {
	if err := validateSessionID(id); err != nil {
		return err
	}

	if sessionsDirOverride == nil {
		return session.DefaultRepository().Delete(id, "")
	}

	dir, err := sessionsDir()
	if err != nil {
		return err
	}

	path := filepath.Join(dir, id+".jsonl")

	// Defence-in-depth: verify the resolved path stays under the sessions dir.
	if rel, err := filepath.Rel(dir, path); err != nil || strings.HasPrefix(rel, "..") {
		return i18n.NewError(i18n.KeySDKSessionsPathEscapes)
	}

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return i18n.NewError(i18n.KeySDKSessionsNotFound, id)
		}
		return i18n.WrapError(i18n.KeySDKSessionsDeleteFailed, err)
	}

	slog.Info(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogSDKSessionDeleted), "id", id)
	return nil
}

// ─── internal helpers ─────────────────────────────────────────────────────────

// readSessionMessages reads a JSONL session file and returns the total number
// of messages and a title derived from the first user message.
func readSessionMessages(path string) (count int, title string, _ error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	for dec.More() {
		var msg struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if err := dec.Decode(&msg); err != nil {
			// Return partial results on corrupt entry.
			return count, title, i18n.WrapError(i18n.KeySDKSessionsDecodeEntry, err, count+1)
		}
		count++
		if title == "" && msg.Role == "user" {
			title = extractSessionTitle(msg.Content)
		}
	}
	return count, title, nil
}

// extractSessionTitle derives a short title from raw JSON message content.
// Handles both plain string content and arrays of content blocks.
func extractSessionTitle(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// Plain string content.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return sessionTruncate(s, 80)
	}

	// Array of content blocks (Anthropic API format).
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				return sessionTruncate(b.Text, 80)
			}
		}
	}

	return ""
}

// sessionTruncate returns s trimmed of whitespace and capped at maxLen runes,
// appending "…" when truncation occurs.
func sessionTruncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}
