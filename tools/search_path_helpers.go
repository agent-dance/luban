// Package tools — search_path_helpers.go provides path-resolution helpers
// shared by Glob/Grep tools to match TS behaviour:
//
//   - expandTildePath: resolves leading "~" / "~/foo" / "~user/foo" to the
//     real home directory. TS uses os.homedir(); Go's filepath.Abs leaves
//     the literal "~" intact, breaking model paths like "~/Documents".
//
//   - isUNCPath: detects Windows UNC paths (\\host\share or //host/share).
//     We refuse to stat these — opening them can trigger an SMB handshake
//     that leaks NTLM credentials over the network.
//
//   - globEnvHiddenEnabled / globEnvNoIgnoreEnabled: env-var toggles
//     CLAUDE_CODE_GLOB_HIDDEN / CLAUDE_CODE_GLOB_NO_IGNORE that flip the
//     ripgrep --hidden / --no-ignore flags for the Glob tool. Default to
//     "true" so existing behaviour is preserved.
package tools

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// expandTildePath resolves leading "~" or "~user" against the appropriate
// home directory. Returns the input unchanged when no expansion is needed
// (e.g. relative path, no leading tilde, or lookup failure).
func expandTildePath(p string) string {
	if p == "" || p[0] != '~' {
		return p
	}
	rest := p[1:]
	// "~", "~/...", "~\..."
	if rest == "" || rest[0] == '/' || rest[0] == filepath.Separator {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(rest, string(filepath.Separator)))
		}
		return p
	}
	// "~name/..." — split at first separator.
	idx := strings.IndexAny(rest, "/\\")
	var name, tail string
	if idx == -1 {
		name = rest
	} else {
		name = rest[:idx]
		tail = rest[idx+1:]
	}
	if u, err := user.Lookup(name); err == nil {
		return filepath.Join(u.HomeDir, tail)
	}
	return p
}

// isUNCPath reports whether p is a Windows UNC path. Both \\host\share and
// //host/share are recognised, since the model may pass either separator.
func isUNCPath(p string) bool {
	if len(p) < 2 {
		return false
	}
	if (p[0] == '\\' && p[1] == '\\') || (p[0] == '/' && p[1] == '/') {
		return true
	}
	return false
}

// globEnvHiddenEnabled returns whether ripgrep --hidden should be passed
// when running glob. Default true; opt-out via CLAUDE_CODE_GLOB_HIDDEN=0|false.
func globEnvHiddenEnabled() bool {
	return envFlagDefaultTrue("CLAUDE_CODE_GLOB_HIDDEN")
}

// globEnvNoIgnoreEnabled returns whether ripgrep --no-ignore should be
// passed. Default true; opt-out via CLAUDE_CODE_GLOB_NO_IGNORE=0|false.
func globEnvNoIgnoreEnabled() bool {
	return envFlagDefaultTrue("CLAUDE_CODE_GLOB_NO_IGNORE")
}

func envFlagDefaultTrue(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if v == "0" || v == "false" || v == "no" || v == "off" {
		return false
	}
	return true
}

// GlobSearchTimeout returns the per-invocation timeout for ripgrep / native
// glob. Reads CLAUDE_CODE_GLOB_TIMEOUT_SECONDS first; default is 60s under
// WSL (which is 3-5x slower walking large NTFS-mounted repos via the
// translation layer) and 20s elsewhere. Mirrors src/utils/ripgrep.ts:128-133.
func GlobSearchTimeout() time.Duration {
	if v := strings.TrimSpace(os.Getenv("CLAUDE_CODE_GLOB_TIMEOUT_SECONDS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	if isLikelyWSL() {
		return 60 * time.Second
	}
	return 20 * time.Second
}

// isLikelyWSL detects whether we're running inside Windows Subsystem for
// Linux. The most reliable signal is /proc/version containing "microsoft"
// or "WSL"; fall back to WSL-specific env vars.
func isLikelyWSL() bool {
	if v := os.Getenv("WSL_DISTRO_NAME"); v != "" {
		return true
	}
	if v := os.Getenv("WSL_INTEROP"); v != "" {
		return true
	}
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	lower := strings.ToLower(string(data))
	return strings.Contains(lower, "microsoft") || strings.Contains(lower, "wsl")
}
