package agent

// agent_resume.go bundles the resume-path helpers for a paused async
// agent. The actual replay-from-transcript logic lives in
// AgentTool.RestoreAgentSessionFromRecord (agent.go); this file holds the
// small platform-specific helpers it depends on.
//
// touchAgentWorktreePath bumps the worktree directory mtime so that
// stale-cleanup sweepers (which delete worktrees older than N days) leave a
// directory still referenced by a paused agent untouched. Without the
// bump the worktree gets garbage collected and resume fails with
// "worktree missing", silently destroying user work.

import (
	"errors"
	"os"
	"time"
)

// touchAgentWorktreePath sets atime+mtime of path to now. Returns the
// underlying error when path exists but cannot be stat/chtimes-ed.
// Returns nil when path is empty or does not exist (resume can still
// proceed; the worktree will be re-created at run time).
func touchAgentWorktreePath(path string) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	now := time.Now()
	return os.Chtimes(path, now, now)
}
