package session

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/agent-dance/luban/brand"
)

var unsafeProjectKey = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// ConfigHomeDir returns the root configuration directory used by the Go
// runtime. Session storage lives under this root.
func ConfigHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return brand.ConfigDirName
	}
	return filepath.Join(home, brand.ConfigDirName)
}

// ProjectsRootDir returns the project-scoped session storage root.
func ProjectsRootDir() string {
	return filepath.Join(ConfigHomeDir(), "projects")
}

// LegacyDir returns the pre-project-scoped session storage directory.
func LegacyDir() string {
	return filepath.Join(ConfigHomeDir(), "sessions")
}

// DefaultDir is kept for backward compatibility with older call sites. New code
// should prefer ProjectsRootDir / ProjectDirForCWD.
func DefaultDir() string {
	return LegacyDir()
}

// ProjectKeyForCWD returns a stable filesystem-safe project key for cwd. The
// hash suffix preserves uniqueness across paths that sanitize to the same text.
func ProjectKeyForCWD(cwd string) string {
	normalized := normalizeProjectPath(cwd)
	if normalized == "" {
		normalized = "default"
	}
	h := sha1.Sum([]byte(normalized))
	hash := hex.EncodeToString(h[:6])

	base := strings.Trim(normalized, "/")
	if base == "" {
		base = "root"
	}
	base = unsafeProjectKey.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = "project"
	}
	if len(base) > 96 {
		base = base[:96]
		base = strings.Trim(base, "-")
	}
	return base + "-" + hash
}

// ProjectDirForCWD returns the directory that stores transcripts for cwd.
func ProjectDirForCWD(cwd string) string {
	return filepath.Join(ProjectsRootDir(), ProjectKeyForCWD(cwd))
}

func normalizeProjectPath(cwd string) string {
	trimmed := strings.TrimSpace(cwd)
	if trimmed == "" {
		return ""
	}
	abs, err := filepath.Abs(trimmed)
	if err == nil {
		trimmed = abs
	}
	trimmed = filepath.Clean(trimmed)
	trimmed = filepath.ToSlash(trimmed)
	return trimmed
}
