package paths

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/agent-dance/luban/brand"
)

var unsafeProjectKey = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// processRuntimeScope is deliberately unique to this process invocation. It
// prevents two CLI processes working in the same repository from sharing
// mutable lock files or partial artifacts when no conversation identity is
// available yet.
var processRuntimeScope = fmt.Sprintf("process-%d-%d", os.Getpid(), time.Now().UTC().UnixNano())

// ConfigHomeDir returns the root configuration directory used by the Go
// runtime. Session storage lives under this root.
func ConfigHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return brand.ConfigDirName
	}
	return filepath.Join(home, brand.ConfigDirName)
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

// RuntimeProjectDir returns the external private-runtime namespace for a
// logical project. Runtime state must never be placed inside the user
// repository: doing so changes git status and makes generated artifacts look
// like source files to coding agents.
func RuntimeProjectDir(projectRoot string) string {
	return RuntimeProjectDirAt(ConfigHomeDir(), projectRoot)
}

// RuntimeProjectDirAt is RuntimeProjectDir with an explicit configuration
// home. Tests use it to keep all persistence inside t.TempDir().
func RuntimeProjectDirAt(configHome, projectRoot string) string {
	root := strings.TrimSpace(configHome)
	if root == "" {
		root = ConfigHomeDir()
	}
	root = canonicalizeRuntimePath(root)
	return filepath.Join(root, "runtime", ProjectKeyForCWD(projectRoot))
}

// RuntimeSessionDir returns the private runtime directory for one conversation
// or, when sessionID is empty, for this process invocation.
func RuntimeSessionDir(projectRoot, sessionID string) string {
	return RuntimeSessionDirAt(ConfigHomeDir(), projectRoot, sessionID)
}

// RuntimeSessionDirAt is RuntimeSessionDir with an explicit configuration
// home. The final component is collision-resistant even for session IDs that
// contain path separators or sanitize to the same text.
func RuntimeSessionDirAt(configHome, projectRoot, sessionID string) string {
	scope := strings.TrimSpace(sessionID)
	if scope == "" {
		scope = processRuntimeScope
	}
	return filepath.Join(RuntimeProjectDirAt(configHome, projectRoot), runtimeScopeKey(scope))
}

// RuntimeServiceDir returns a project-durable namespace for services such as
// schedules and project memory whose lifetime intentionally spans processes.
func RuntimeServiceDir(projectRoot, service string) string {
	return RuntimeServiceDirAt(ConfigHomeDir(), projectRoot, service)
}

// RuntimeServiceDirAt is RuntimeServiceDir with an explicit configuration
// home for hermetic tests.
func RuntimeServiceDirAt(configHome, projectRoot, service string) string {
	return filepath.Join(RuntimeProjectDirAt(configHome, projectRoot), "service-"+runtimeScopeKey(service))
}

func runtimeScopeKey(scope string) string {
	normalized := strings.TrimSpace(scope)
	if normalized == "" {
		normalized = "default"
	}
	h := sha1.Sum([]byte(normalized))
	hash := hex.EncodeToString(h[:6])
	base := unsafeProjectKey.ReplaceAllString(normalized, "-")
	base = strings.Trim(base, "-._")
	if base == "" {
		base = "scope"
	}
	if len(base) > 64 {
		base = strings.Trim(base[:64], "-._")
	}
	return base + "-" + hash
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
	trimmed = canonicalizeRuntimePath(trimmed)
	trimmed = filepath.ToSlash(trimmed)
	return trimmed
}

// canonicalizeRuntimePath resolves symlinks through the nearest existing
// ancestor, then appends any not-yet-created suffix. Unlike EvalSymlinks on the
// full path, its answer cannot change merely because a runtime directory was
// created between two calls (notably /var versus /private/var on macOS).
func canonicalizeRuntimePath(path string) string {
	cleaned := filepath.Clean(path)
	if abs, err := filepath.Abs(cleaned); err == nil {
		cleaned = filepath.Clean(abs)
	}
	ancestor := cleaned
	var suffix []string
	for {
		if _, err := os.Lstat(ancestor); err == nil {
			break
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return cleaned
		}
		suffix = append(suffix, filepath.Base(ancestor))
		ancestor = parent
	}
	if resolved, err := filepath.EvalSymlinks(ancestor); err == nil {
		ancestor = filepath.Clean(resolved)
	}
	for index := len(suffix) - 1; index >= 0; index-- {
		ancestor = filepath.Join(ancestor, suffix[index])
	}
	return filepath.Clean(ancestor)
}
