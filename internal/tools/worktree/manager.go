package worktree

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/gitutil"
)

// worktreeRef is the stable representation of one git worktree porcelain
// record. Locked and Prunable retain the optional reason emitted by git.
type worktreeRef struct {
	Path           string
	Branch         string
	Head           string
	Locked         bool
	LockedReason   string
	Prunable       bool
	PrunableReason string
}

type worktreeCreateResult struct {
	worktreeRef
	Created bool
}

// WorktreeManager owns session state and serializes git worktree mutations per
// canonical repository. Session state is keyed explicitly; two registries in
// the same process never share the active flag merely because they share a repo.
type WorktreeManager struct {
	mu         sync.Mutex
	states     map[string]*WorktreeState
	pathOwners map[string]string
	repoLocks  map[string]*sync.Mutex
}

func NewWorktreeManager() *WorktreeManager {
	return &WorktreeManager{
		states:     make(map[string]*WorktreeState),
		pathOwners: make(map[string]string),
		repoLocks:  make(map[string]*sync.Mutex),
	}
}

// Register binds an explicitly injected state to a non-empty session. If the
// session is already registered, its original state remains authoritative.
func (m *WorktreeManager) register(sessionID string, state *WorktreeState) *WorktreeState {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || state == nil {
		return nil
	}
	state.mu.Lock()
	owner := strings.TrimSpace(state.SessionID)
	if owner != "" && owner != sessionID {
		state.mu.Unlock()
		return nil
	}
	state.SessionID = sessionID
	state.mu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if registered := m.states[sessionID]; registered != nil {
		return registered
	}
	m.states[sessionID] = state
	return state
}

func (m *WorktreeManager) forget(sessionID string) {
	m.mu.Lock()
	delete(m.states, sessionID)
	for path, owner := range m.pathOwners {
		if owner == sessionID {
			delete(m.pathOwners, path)
		}
	}
	m.mu.Unlock()
}

func (m *WorktreeManager) claimPath(sessionID, path string) error {
	path = cleanWorktreePath(path)
	m.mu.Lock()
	defer m.mu.Unlock()
	if owner := m.pathOwners[path]; owner != "" && owner != sessionID {
		return i18n.NewError(i18n.KeyToolWorktreePathClaimed, path, owner)
	}
	m.pathOwners[path] = sessionID
	return nil
}

func (m *WorktreeManager) releasePath(sessionID, path string) {
	path = cleanWorktreePath(path)
	m.mu.Lock()
	if owner := m.pathOwners[path]; owner == sessionID {
		delete(m.pathOwners, path)
	}
	m.mu.Unlock()
}

func (m *WorktreeManager) repoLock(repoRoot string) *sync.Mutex {
	repoRoot = cleanWorktreePath(repoRoot)
	m.mu.Lock()
	defer m.mu.Unlock()
	lock := m.repoLocks[repoRoot]
	if lock == nil {
		lock = &sync.Mutex{}
		m.repoLocks[repoRoot] = lock
	}
	return lock
}

func (m *WorktreeManager) list(repoRoot string) ([]worktreeRef, error) {
	repoRoot = cleanWorktreePath(repoRoot)
	out, err := gitutil.Run(repoRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, i18n.NewError(i18n.KeyToolWorktreeListFailed, strings.TrimSpace(out))
	}
	return parseWorktreeRefs(out), nil
}

func parseWorktreeRefs(out string) []worktreeRef {
	refs := make([]worktreeRef, 0)
	var current worktreeRef
	flush := func() {
		if current.Path != "" {
			current.Path = cleanWorktreePath(current.Path)
			refs = append(refs, current)
		}
		current = worktreeRef{}
	}
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			flush()
			continue
		}
		switch {
		case strings.HasPrefix(line, "worktree "):
			if current.Path != "" {
				flush()
			}
			current.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			current.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			current.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "locked":
			current.Locked = true
		case strings.HasPrefix(line, "locked "):
			current.Locked = true
			current.LockedReason = strings.TrimPrefix(line, "locked ")
		case line == "prunable":
			current.Prunable = true
		case strings.HasPrefix(line, "prunable "):
			current.Prunable = true
			current.PrunableReason = strings.TrimPrefix(line, "prunable ")
		}
	}
	flush()
	return refs
}

func (m *WorktreeManager) create(repoRoot, name, baseRef string, sparsePaths []string) (worktreeCreateResult, error) {
	repoRoot = cleanWorktreePath(repoRoot)
	if _, err := normalizeSlug(name); err != nil {
		return worktreeCreateResult{}, err
	}
	if repoRoot == "" {
		return worktreeCreateResult{}, i18n.NewError(i18n.KeyToolWorktreeRepositoryRootEmpty)
	}
	lock := m.repoLock(repoRoot)
	lock.Lock()
	defer lock.Unlock()

	path, branch := worktreePathAndBranch(repoRoot, name)
	refs, err := m.list(repoRoot)
	if err != nil {
		return worktreeCreateResult{}, err
	}
	for _, ref := range refs {
		if ref.Path != path {
			continue
		}
		if ref.Branch != branch {
			return worktreeCreateResult{}, i18n.NewError(i18n.KeyToolWorktreeBranchMismatch, path, ref.Branch, branch)
		}
		if ref.Head == "" {
			ref.Head, _ = gitutil.Run(path, "rev-parse", "HEAD")
		}
		return worktreeCreateResult{worktreeRef: ref, Created: false}, nil
	}
	if _, err := os.Lstat(path); err == nil {
		return worktreeCreateResult{}, i18n.NewError(i18n.KeyToolWorktreePathUnregistered, path)
	} else if !os.IsNotExist(err) {
		return worktreeCreateResult{}, i18n.WrapError(i18n.KeyToolWorktreeInspectPathFailed, err, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return worktreeCreateResult{}, i18n.WrapError(i18n.KeyToolWorktreeCreateParentFailed, err)
	}
	head, err := gitutil.Run(repoRoot, "rev-parse", "--verify", baseRef)
	if err != nil || strings.TrimSpace(head) == "" {
		return worktreeCreateResult{}, i18n.NewError(i18n.KeyToolWorktreeResolveBaseRefFailed, baseRef, strings.TrimSpace(head))
	}
	args := []string{"worktree", "add"}
	if len(sparsePaths) > 0 {
		args = append(args, "--no-checkout")
	}
	args = append(args, "-B", branch, path, baseRef)
	if out, err := gitutil.Run(repoRoot, args...); err != nil {
		return worktreeCreateResult{}, i18n.NewError(i18n.KeyToolWorktreeCreateFailed, strings.TrimSpace(out))
	}
	rollback := func(cause error) (worktreeCreateResult, error) {
		removeOut, removeErr := gitutil.Run(repoRoot, "worktree", "remove", "--force", path)
		_, _ = gitutil.Run(repoRoot, "branch", "-D", branch)
		if removeErr != nil {
			return worktreeCreateResult{}, i18n.WrapError(i18n.KeyToolWorktreeRollbackFailed, cause, strings.TrimSpace(removeOut))
		}
		return worktreeCreateResult{}, i18n.WrapError(i18n.KeyToolWorktreeRolledBack, cause)
	}
	if len(sparsePaths) > 0 {
		if err := applySparseCheckout(path, sparsePaths); err != nil {
			return rollback(err)
		}
	}
	return worktreeCreateResult{
		worktreeRef: worktreeRef{Path: path, Branch: branch, Head: strings.TrimSpace(head)},
		Created:     true,
	}, nil
}

func worktreePathAndBranch(repoRoot, name string) (string, string) {
	flatName, _ := normalizeSlug(name)
	path := cleanWorktreePath(filepath.Join(repoRoot, brand.ConfigDirName, "worktrees", flatName))
	return path, "worktree-" + flatName
}

func (m *WorktreeManager) remove(repoRoot, path string, force bool) error {
	repoRoot = cleanWorktreePath(repoRoot)
	path = cleanWorktreePath(path)
	lock := m.repoLock(repoRoot)
	lock.Lock()
	defer lock.Unlock()
	if !force {
		status, err := gitutil.Run(path, "status", "--porcelain")
		if err != nil {
			return i18n.NewError(i18n.KeyToolWorktreeInspectChangesFailed, strings.TrimSpace(status))
		}
		if strings.TrimSpace(status) != "" {
			return i18n.NewError(i18n.KeyToolWorktreeUncommittedChanges)
		}
	}
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)
	if out, err := gitutil.Run(repoRoot, args...); err != nil {
		return i18n.NewError(i18n.KeyToolWorktreeRemoveFailed, strings.TrimSpace(out))
	}
	return nil
}
