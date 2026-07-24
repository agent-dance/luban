package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
)

// WorktreeRef is the stable representation of one git worktree porcelain
// record. Locked and Prunable retain the optional reason emitted by git.
type WorktreeRef struct {
	Path           string
	Branch         string
	Head           string
	Locked         bool
	LockedReason   string
	Prunable       bool
	PrunableReason string
}

type WorktreeCreateResult struct {
	WorktreeRef
	Created bool
	BaseRef string
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

var globalWorktreeManager = NewWorktreeManager()

func DefaultWorktreeManager() *WorktreeManager { return globalWorktreeManager }

// StateForSession returns the session's stable state. fallback is used only
// when creating the entry, which preserves the legacy Enter/Exit shared-state
// constructor while still allowing explicitly keyed sessions.
func (m *WorktreeManager) StateForSession(sessionID string, fallback *WorktreeState) *WorktreeState {
	if m == nil {
		if fallback != nil {
			return fallback
		}
		return &WorktreeState{}
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		if fallback != nil {
			sessionID = stateID(fallback)
		} else {
			sessionID = "default"
		}
	}
	if fallback != nil {
		candidate := fallback
		candidate.mu.Lock()
		owner := strings.TrimSpace(candidate.SessionID)
		if owner == "" {
			candidate.SessionID = sessionID
		} else if owner != sessionID {
			fallback = nil
		}
		candidate.mu.Unlock()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if state := m.states[sessionID]; state != nil {
		return state
	}
	if fallback == nil {
		fallback = &WorktreeState{SessionID: sessionID}
	}
	m.states[sessionID] = fallback
	return fallback
}

func (m *WorktreeManager) Register(sessionID string, state *WorktreeState) {
	if m == nil || state == nil {
		return
	}
	_ = m.StateForSession(sessionID, state)
}

func (m *WorktreeManager) Forget(sessionID string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	delete(m.states, sessionID)
	for path, owner := range m.pathOwners {
		if owner == sessionID {
			delete(m.pathOwners, path)
		}
	}
	m.mu.Unlock()
}

func (m *WorktreeManager) ListActive() []*WorktreeState {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	snapshot := make([]*WorktreeState, 0, len(m.states))
	for _, state := range m.states {
		snapshot = append(snapshot, state)
	}
	m.mu.Unlock()
	active := snapshot[:0]
	for _, state := range snapshot {
		state.mu.Lock()
		on := state.Active
		state.mu.Unlock()
		if on {
			active = append(active, state)
		}
	}
	return active
}

func (m *WorktreeManager) CountActive() int { return len(m.ListActive()) }

func (m *WorktreeManager) ClaimPath(sessionID, path string) error {
	if m == nil {
		return nil
	}
	path = cleanWorktreePath(path)
	m.mu.Lock()
	defer m.mu.Unlock()
	if owner := m.pathOwners[path]; owner != "" && owner != sessionID {
		return i18n.NewError(i18n.KeyToolWorktreePathClaimed, path, owner)
	}
	m.pathOwners[path] = sessionID
	return nil
}

func (m *WorktreeManager) ReleasePath(sessionID, path string) {
	if m == nil {
		return
	}
	path = cleanWorktreePath(path)
	m.mu.Lock()
	if owner := m.pathOwners[path]; owner == sessionID || sessionID == "" {
		delete(m.pathOwners, path)
	}
	m.mu.Unlock()
}

func (m *WorktreeManager) SessionShutdown(state *WorktreeState) bool {
	if m == nil || state == nil {
		return false
	}
	state.mu.Lock()
	wasActive := state.Active
	sessionID, path := state.SessionID, state.Path
	if wasActive {
		state.clearLocked()
	}
	state.mu.Unlock()
	m.ReleasePath(sessionID, path)
	m.Forget(sessionID)
	return wasActive
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

func (m *WorktreeManager) List(repoRoot string) ([]WorktreeRef, error) {
	repoRoot = cleanWorktreePath(repoRoot)
	out, err := runGit(repoRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, i18n.NewError(i18n.KeyToolWorktreeListFailed, strings.TrimSpace(out))
	}
	return parseWorktreeRefs(out), nil
}

func parseWorktreeRefs(out string) []WorktreeRef {
	refs := make([]WorktreeRef, 0)
	var current WorktreeRef
	flush := func() {
		if current.Path != "" {
			current.Path = cleanWorktreePath(current.Path)
			refs = append(refs, current)
		}
		current = WorktreeRef{}
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

func (m *WorktreeManager) Create(repoRoot, name, baseRef string, sparsePaths []string) (WorktreeCreateResult, error) {
	repoRoot = cleanWorktreePath(repoRoot)
	if err := validateWorktreeSlug(name); err != nil {
		return WorktreeCreateResult{}, err
	}
	if repoRoot == "" {
		return WorktreeCreateResult{}, i18n.NewError(i18n.KeyToolWorktreeRepositoryRootEmpty)
	}
	lock := m.repoLock(repoRoot)
	lock.Lock()
	defer lock.Unlock()

	path, branch := worktreePathAndBranch(repoRoot, name)
	refs, err := m.List(repoRoot)
	if err != nil {
		return WorktreeCreateResult{}, err
	}
	for _, ref := range refs {
		if ref.Path != path {
			continue
		}
		if ref.Branch != branch {
			return WorktreeCreateResult{}, i18n.NewError(i18n.KeyToolWorktreeBranchMismatch, path, ref.Branch, branch)
		}
		if ref.Head == "" {
			ref.Head, _ = runGit(path, "rev-parse", "HEAD")
		}
		return WorktreeCreateResult{WorktreeRef: ref, Created: false, BaseRef: baseRef}, nil
	}
	if _, err := os.Lstat(path); err == nil {
		return WorktreeCreateResult{}, i18n.NewError(i18n.KeyToolWorktreePathUnregistered, path)
	} else if !os.IsNotExist(err) {
		return WorktreeCreateResult{}, i18n.WrapError(i18n.KeyToolWorktreeInspectPathFailed, err, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return WorktreeCreateResult{}, i18n.WrapError(i18n.KeyToolWorktreeCreateParentFailed, err)
	}
	head, err := runGit(repoRoot, "rev-parse", "--verify", baseRef)
	if err != nil || strings.TrimSpace(head) == "" {
		return WorktreeCreateResult{}, i18n.NewError(i18n.KeyToolWorktreeResolveBaseRefFailed, baseRef, strings.TrimSpace(head))
	}
	args := []string{"worktree", "add"}
	if len(sparsePaths) > 0 {
		args = append(args, "--no-checkout")
	}
	args = append(args, "-B", branch, path, baseRef)
	if out, err := runGit(repoRoot, args...); err != nil {
		return WorktreeCreateResult{}, i18n.NewError(i18n.KeyToolWorktreeCreateFailed, strings.TrimSpace(out))
	}
	rollback := func(cause error) (WorktreeCreateResult, error) {
		removeOut, removeErr := runGit(repoRoot, "worktree", "remove", "--force", path)
		_, _ = runGit(repoRoot, "branch", "-D", branch)
		if removeErr != nil {
			return WorktreeCreateResult{}, i18n.WrapError(i18n.KeyToolWorktreeRollbackFailed, cause, strings.TrimSpace(removeOut))
		}
		return WorktreeCreateResult{}, i18n.WrapError(i18n.KeyToolWorktreeRolledBack, cause)
	}
	if len(sparsePaths) > 0 {
		if err := applySparseCheckout(path, sparsePaths); err != nil {
			return rollback(err)
		}
	}
	return WorktreeCreateResult{
		WorktreeRef: WorktreeRef{Path: path, Branch: branch, Head: strings.TrimSpace(head)},
		Created:     true,
		BaseRef:     baseRef,
	}, nil
}

func worktreePathAndBranch(repoRoot, name string) (string, string) {
	flatName := flattenWorktreeSlug(name)
	path := cleanWorktreePath(filepath.Join(repoRoot, brand.ConfigDirName, "worktrees", flatName))
	return path, "worktree-" + flatName
}

func (m *WorktreeManager) Remove(repoRoot, path string, force bool) error {
	repoRoot = cleanWorktreePath(repoRoot)
	path = cleanWorktreePath(path)
	lock := m.repoLock(repoRoot)
	lock.Lock()
	defer lock.Unlock()
	if !force {
		status, err := runGit(path, "status", "--porcelain")
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
	if out, err := runGit(repoRoot, args...); err != nil {
		return i18n.NewError(i18n.KeyToolWorktreeRemoveFailed, strings.TrimSpace(out))
	}
	return nil
}

func stateID(state *WorktreeState) string {
	stateIDMu.Lock()
	defer stateIDMu.Unlock()
	if id := stateIDs[state]; id != "" {
		return id
	}
	stateIDCounter++
	id := fmt.Sprintf("wt-%d", stateIDCounter)
	stateIDs[state] = id
	return id
}

var (
	stateIDMu      sync.Mutex
	stateIDs       = make(map[*WorktreeState]string)
	stateIDCounter int
)
