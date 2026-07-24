package tools

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// worktree_reaper.go — WT-05 startup reaper for stale agent ephemeral
// worktrees.
//
// When an agent session crashes mid-task, its ephemeral worktree
// directory remains in `git worktree list` and the branch holds the slot.
// Subsequent `EnterWorktree` calls with the same name fail because the
// worktree path / branch already exist. CleanupStaleAgentWorktrees scans
// the worktree list at boot and drops entries that match the ephemeral
// pattern AND whose owning session is no longer alive.

// ephemeralWorktreePattern matches the agent-ephemeral name slug that
// EnterWorktree synthesises by default. Mirrors EPHEMERAL_WORKTREE_PATTERNS
// from the TS scheduler.
var ephemeralWorktreePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?:^|/)wt-(\d+)$`),                // legacy short slug
	regexp.MustCompile(`(?:^|/)agent-([0-9a-f]{12,})$`),   // agent-<hash>
	regexp.MustCompile(`(?:^|/)worktree-(.+)$`),           // TS EnterWorktree branch slug
	regexp.MustCompile(`(?:^|/)luban-(?:wt|agent)-(.+)$`), // LUBAN Code branch slug
	regexp.MustCompile(`(?:^|/)deepseek-wt-(.+)$`),        // deepseek branch slug
	regexp.MustCompile(`(?:^|/)deepseek-agent-(.+)$`),     // legacy DeepSeek agent slug
}

// CleanupStaleAgentWorktrees removes worktrees that look like agent
// ephemeral entries and whose owning session is no longer tracked. The
// caller passes a `nowAlive` predicate to identify still-running sessions
// (typically: pid alive + manager has the state registered). Returns the
// list of paths that were removed.
func CleanupStaleAgentWorktrees(repoRoot string, nowAlive func(path string) bool) []string {
	cleanupMu.Lock()
	defer cleanupMu.Unlock()

	listOut, listErr := runGit(repoRoot, "worktree", "list", "--porcelain")
	if listErr != nil {
		return nil
	}
	entries := parseWorktreePorcelain(listOut)
	removed := make([]string, 0)
	for _, entry := range entries {
		if entry.Path == repoRoot {
			continue
		}
		if !isEphemeralWorktreeName(entry.Path) {
			continue
		}
		// Don't reap sessions that are still alive.
		if nowAlive != nil && nowAlive(entry.Path) {
			continue
		}
		// Don't reap worktrees with uncommitted changes — only orphans.
		if status, err := runGit(entry.Path, "status", "--porcelain"); err == nil && strings.TrimSpace(status) != "" {
			continue
		}
		_, _ = runGit(repoRoot, "worktree", "remove", entry.Path, "--force")
		if entry.Branch != "" {
			_, _ = runGit(repoRoot, "branch", "-D", entry.Branch)
		}
		removed = append(removed, entry.Path)
	}
	return removed
}

var cleanupMu sync.Mutex

type porcelainWorktree struct {
	Path   string
	Branch string
}

// parseWorktreePorcelain parses `git worktree list --porcelain` output.
func parseWorktreePorcelain(out string) []porcelainWorktree {
	entries := make([]porcelainWorktree, 0)
	var cur porcelainWorktree
	flush := func() {
		if cur.Path != "" {
			entries = append(entries, cur)
		}
		cur = porcelainWorktree{}
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			flush()
			continue
		}
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		}
	}
	flush()
	return entries
}

// isEphemeralWorktreeName reports whether path matches one of the agent
// ephemeral patterns.
func isEphemeralWorktreeName(path string) bool {
	base := filepath.Base(filepath.Clean(path))
	parent := filepath.Base(filepath.Dir(filepath.Clean(path)))
	candidate := parent + "/" + base
	for _, pat := range ephemeralWorktreePatterns {
		if pat.MatchString(candidate) || pat.MatchString(base) {
			return true
		}
	}
	return false
}

// PIDAlive reports whether a pid embedded in a path is currently alive.
// Useful as the `nowAlive` predicate for CleanupStaleAgentWorktrees when
// agent worktree names embed a pid.
func PIDAlive(path string) bool {
	// Try to parse a numeric suffix from the path.
	base := filepath.Base(path)
	for i := len(base); i > 0; i-- {
		if base[i-1] < '0' || base[i-1] > '9' {
			if i == len(base) {
				return false
			}
			pidStr := base[i:]
			pid, err := strconv.Atoi(pidStr)
			if err != nil {
				return false
			}
			return schedulerProcessAlive(pid)
		}
	}
	return false
}

// _ keeps the time import alive for future heartbeat checks.
var _ = time.Now
