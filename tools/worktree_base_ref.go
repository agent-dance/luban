package tools

import (
	"os"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
)

// worktree_base_ref.go — resolves the base ref for a new worktree.
//
// Mirrors TS baseRefResolver.ts. The setting comes from worktree.baseRef:
//   "fresh" (default) — branch from origin/<default-branch> so the worktree
//                       starts identical to upstream main.
//   "head"            — branch from local HEAD so the worktree starts where
//                       the user already is.
//
// Result is cached per process so repeated EnterWorktree calls do not
// re-shell out to git.

type baseRefCache struct {
	mu     sync.Mutex
	values map[string]string // key = setting, value = resolved ref
}

var globalBaseRefCache = &baseRefCache{values: make(map[string]string)}

// ResolveBaseRef returns the git ref to branch from when creating a worktree.
// `setting` is the value of the `worktree.baseRef` config option:
//   - "fresh" or "" → origin/<default-branch> (preferred), or origin's
//     remote tracking branch as a fallback, or HEAD as a last resort.
//   - "head"        → "HEAD"
//
// Any other value is rejected so typos surface immediately.
func ResolveBaseRef(setting string) (string, error) {
	cwd, _ := os.Getwd()
	repoRoot, _ := canonicalGitRootFrom(cwd)
	return ResolveBaseRefAt(repoRoot, "", setting)
}

// ResolveBaseRefAt resolves worktree.baseRef against an explicit repository
// and scopes its cache by session and repo. This avoids cross-session results
// leaking when multiple repositories use different default branches.
func ResolveBaseRefAt(repoRoot, sessionID, setting string) (string, error) {
	setting = strings.ToLower(strings.TrimSpace(setting))
	if setting == "" {
		setting = "fresh"
	}

	switch setting {
	case "head":
		return "HEAD", nil
	case "fresh":
		return resolveFreshBaseRefAt(repoRoot, sessionID)
	default:
		return "", i18n.NewError(i18n.KeyToolWorktreeBaseRefInvalid, setting, "fresh", "head")
	}
}

func resolveFreshBaseRefAt(repoRoot, sessionID string) (string, error) {
	cacheKey := cleanWorktreePath(repoRoot) + "\x00" + strings.TrimSpace(sessionID) + "\x00fresh"
	globalBaseRefCache.mu.Lock()
	if v, ok := globalBaseRefCache.values[cacheKey]; ok {
		globalBaseRefCache.mu.Unlock()
		return v, nil
	}
	globalBaseRefCache.mu.Unlock()

	// Try `git symbolic-ref refs/remotes/origin/HEAD` first — this gives
	// us e.g. "refs/remotes/origin/main" when the remote has a HEAD
	// pointer (the common case for cloned repos).
	if out, err := runGit(repoRoot, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); err == nil {
		ref := strings.TrimSpace(out)
		// Strip the leading "refs/remotes/" so callers can use it as a branch ref.
		ref = strings.TrimPrefix(ref, "refs/remotes/")
		if ref != "" {
			cacheBaseRef(cacheKey, ref)
			return ref, nil
		}
	}

	// Fallback: ask git for the upstream of the current branch.
	if out, err := runGit(repoRoot, "rev-parse", "--abbrev-ref", "@{upstream}"); err == nil {
		ref := strings.TrimSpace(out)
		if ref != "" && !strings.Contains(ref, "fatal:") {
			cacheBaseRef(cacheKey, ref)
			return ref, nil
		}
	}

	// Last-resort fallback: HEAD. Better than failing the whole flow.
	cacheBaseRef(cacheKey, "HEAD")
	return "HEAD", nil
}

func cacheBaseRef(key, value string) {
	globalBaseRefCache.mu.Lock()
	globalBaseRefCache.values[key] = value
	globalBaseRefCache.mu.Unlock()
}

// resetBaseRefCacheForTests clears the cache so tests can change git
// state and re-resolve. Not part of the public API.
func resetBaseRefCacheForTests() {
	globalBaseRefCache.mu.Lock()
	globalBaseRefCache.values = make(map[string]string)
	globalBaseRefCache.mu.Unlock()
}

func clearBaseRefCacheForSession(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	globalBaseRefCache.mu.Lock()
	for key := range globalBaseRefCache.values {
		parts := strings.Split(key, "\x00")
		if len(parts) >= 3 && parts[1] == sessionID {
			delete(globalBaseRefCache.values, key)
		}
	}
	globalBaseRefCache.mu.Unlock()
	ResetScopedBaseRefCache(sessionID)
}
