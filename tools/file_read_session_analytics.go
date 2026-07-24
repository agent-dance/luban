// Package tools — file_read_session_analytics.go classifies file paths
// into session-file types so analytics can distinguish memory and
// transcript reads from regular file reads. Mirrors TS
// detectSessionFileType.
package tools

import (
	"os"
	"path/filepath"
	"strings"
)

// SessionFileKind is the analytics label for a session-file read.
// Empty string means "not a session file".
type SessionFileKind string

const (
	SessionFileMemory     SessionFileKind = "session_memory"
	SessionFileTranscript SessionFileKind = "session_transcript"
)

// detectSessionFileType reports the analytics kind for absPath, or ""
// when the path is not a recognised session file. Path comparison is
// case-insensitive and slash-normalised so both POSIX and Windows shapes
// classify consistently.
func detectSessionFileType(absPath string) SessionFileKind {
	if absPath == "" {
		return ""
	}
	configDir := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR"))
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
		configDir = filepath.Join(home, ".claude")
	}
	cleanPath := filepath.Clean(absPath)
	cleanConfig := filepath.Clean(configDir)
	rel, err := filepath.Rel(cleanConfig, cleanPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return ""
	}
	uniform := strings.ToLower(strings.ReplaceAll(rel, "\\", "/"))

	if strings.Contains("/"+uniform, "/session-memory/") && strings.HasSuffix(uniform, ".md") {
		return SessionFileMemory
	}
	if strings.Contains("/"+uniform, "/projects/") && strings.HasSuffix(uniform, ".jsonl") {
		return SessionFileTranscript
	}
	return ""
}

func fileExtensionForReadAnalytics(filePath string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filePath)), ".")
	if ext == "" {
		return ""
	}
	if len(ext) > 10 {
		return "other"
	}
	return ext
}
