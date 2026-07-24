// Package tools — file_write_remote_diff.go implements the optional
// per-file git-diff payload that FileWrite attaches when running in
// remote-driver mode with the quartz_lantern feature flag. Mirrors
// fetchSingleFileGitDiff in TS FileWriteTool.
//
// The actual git invocation is best-effort: failures are swallowed and
// the result simply omits the diff. Production callers wire the
// FileWriteTool.RemoteGitDiff hook from the harness bootstrap; tests can
// exercise it via SetFileWriteRemoteGitDiffHookForTest.
package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// remoteGitDiffEnabled reflects the harness-side feature flag. The TS
// equivalent reads CLAUDE_CODE_REMOTE + tengu_quartz_lantern. Default
// off; override via SetRemoteGitDiffEnabled.
var (
	remoteGitDiffMu      sync.RWMutex
	remoteGitDiffEnabled bool
)

// SetRemoteGitDiffEnabled toggles the remote git-diff payload. Should be
// called from harness bootstrap once feature flags resolve.
func SetRemoteGitDiffEnabled(enabled bool) {
	remoteGitDiffMu.Lock()
	remoteGitDiffEnabled = enabled
	remoteGitDiffMu.Unlock()
}

// IsRemoteGitDiffEnabled reports the current toggle state.
func IsRemoteGitDiffEnabled() bool {
	remoteGitDiffMu.RLock()
	defer remoteGitDiffMu.RUnlock()
	return remoteGitDiffEnabled
}

// fetchSingleFileGitDiff returns `git diff -- <absPath>` (working tree
// vs HEAD) for the file. Returns "" on any failure or when feature flag
// is off. The 5-second timeout matches TS.
func fetchSingleFileGitDiff(ctx context.Context, absPath string) string {
	if !IsRemoteGitDiffEnabled() {
		return ""
	}
	if absPath == "" {
		return ""
	}
	if _, err := exec.LookPath("git"); err != nil {
		return ""
	}
	dir := filepath.Dir(absPath)
	if _, err := os.Stat(dir); err != nil {
		return ""
	}

	timeout := 5 * time.Second
	ctxTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctxTimeout, "git", "-C", dir, "diff", "--no-color", "--", absPath)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\n")
}

// attachRemoteGitDiff is invoked by FileWriteTool.Execute when the flag
// is on. Returns the diff string (possibly empty) for inclusion in the
// result payload. The TS shape stores it under "remoteGitDiff".
func attachRemoteGitDiff(ctx context.Context, absPath string) string {
	defer func() { _ = recover() }()
	diff := fetchSingleFileGitDiff(ctx, absPath)
	if len(diff) > 100*1024 {
		diff = diff[:100*1024] + fmt.Sprintf("\n... (truncated to %d bytes)", 100*1024)
	}
	return diff
}
