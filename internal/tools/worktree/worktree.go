package worktree

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/store/secureio"
	"github.com/agent-dance/luban/types"
)

var validWorktreeSlugSegment = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

const maxWorktreeSlugLength = 64

// WorktreeState holds shared state across EnterWorktree / ExitWorktree calls.
type WorktreeState struct {
	mu                 sync.Mutex
	SessionID          string // owning runtime session
	Active             bool
	exiting            bool
	Path               string // worktree directory
	Name               string // user-visible worktree slug
	Branch             string // branch name
	OriginalDir        string // where we came from
	OriginalHeadCommit string
	RepoRoot           string // canonical repo root
	StateFile          string // persisted session state
	CreatedHere        bool   // true if EnterWorktree created this worktree (vs entered an existing one)
	HookBased          bool
	TmuxSessionName    string
	writeFile          func(string, []byte, os.FileMode) error
	LifecycleFactory   LifecycleFactory
	lifecycle          LifecyclePublisher
}

type persistedWorktreeState struct {
	SessionID          string `json:"session_id"`
	Active             bool   `json:"active"`
	Path               string `json:"path"`
	Name               string `json:"name,omitempty"`
	Branch             string `json:"branch"`
	OriginalDir        string `json:"original_dir"`
	OriginalHeadCommit string `json:"original_head_commit,omitempty"`
	RepoRoot           string `json:"repo_root"`
	CreatedHere        bool   `json:"created_here"`
	HookBased          bool   `json:"hook_based,omitempty"`
	TmuxSessionName    string `json:"tmux_session_name,omitempty"`
}

func normalizeSlug(slug string) (string, error) {
	if len(slug) > maxWorktreeSlugLength {
		return "", i18n.NewError(i18n.KeyToolWorktreeNameTooLong, maxWorktreeSlugLength, len(slug))
	}
	for _, segment := range strings.Split(slug, "/") {
		if segment == "." || segment == ".." {
			return "", i18n.NewError(i18n.KeyToolWorktreeNamePathSegment, slug)
		}
		if !validWorktreeSlugSegment.MatchString(segment) {
			return "", i18n.NewError(i18n.KeyToolWorktreeNameCharacters, slug)
		}
	}
	return strings.ReplaceAll(slug, "/", "+"), nil
}

func NormalizeSlug(slug string) (string, error) {
	return normalizeSlug(slug)
}

func (s *WorktreeState) loadFromDisk(repoRoot string) bool {
	if strings.TrimSpace(repoRoot) == "" {
		return false
	}
	stateFile := worktreeStateFilePath(repoRoot, s.SessionID)
	data, err := secureio.ReadPrivateRuntimeRegularFile(stateFile)
	if err != nil {
		return false
	}
	var persisted persistedWorktreeState
	if err := json.Unmarshal(data, &persisted); err != nil || !persisted.Active {
		return false
	}
	owner := strings.TrimSpace(s.SessionID)
	if owner == "" || strings.TrimSpace(persisted.SessionID) != owner {
		return false
	}
	if info, err := os.Stat(persisted.Path); err != nil || !info.IsDir() {
		_ = os.Remove(stateFile)
		return false
	}
	s.SessionID = persisted.SessionID
	s.Active = persisted.Active
	s.Path = persisted.Path
	s.Name = persisted.Name
	s.Branch = persisted.Branch
	s.OriginalDir = persisted.OriginalDir
	s.OriginalHeadCommit = persisted.OriginalHeadCommit
	s.RepoRoot = persisted.RepoRoot
	s.CreatedHere = persisted.CreatedHere
	s.HookBased = persisted.HookBased
	s.TmuxSessionName = persisted.TmuxSessionName
	s.StateFile = stateFile
	s.lifecycle = s.newLifecycle(repoRoot)
	return true
}

func (s *WorktreeState) saveToDiskLocked() error {
	if strings.TrimSpace(s.StateFile) == "" {
		return nil
	}
	if err := secureio.EnsurePrivateRuntimeDirectory(filepath.Dir(s.StateFile)); err != nil {
		return err
	}
	body := persistedWorktreeState{
		SessionID:          s.SessionID,
		Active:             s.Active,
		Path:               s.Path,
		Name:               s.Name,
		Branch:             s.Branch,
		OriginalDir:        s.OriginalDir,
		OriginalHeadCommit: s.OriginalHeadCommit,
		RepoRoot:           s.RepoRoot,
		CreatedHere:        s.CreatedHere,
		HookBased:          s.HookBased,
		TmuxSessionName:    s.TmuxSessionName,
	}
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return err
	}
	writeFile := s.writeFile
	if writeFile == nil {
		writeFile = func(path string, data []byte, _ os.FileMode) error {
			return secureio.AtomicWritePrivateRuntimeFile(path, data)
		}
	}
	return writeFile(s.StateFile, append(data, '\n'), 0600)
}

func (s *WorktreeState) clearLocked() {
	stateFile := s.StateFile
	if strings.TrimSpace(stateFile) != "" {
		// Write an inactive tombstone before unlinking. If the process stops
		// between the two operations, an explicit resume observes inactive state
		// instead of resurrecting a session whose cleanup already ran.
		body, err := json.MarshalIndent(persistedWorktreeState{
			SessionID: s.SessionID,
			Active:    false,
		}, "", "  ")
		if err == nil {
			writeFile := s.writeFile
			if writeFile == nil {
				writeFile = func(path string, data []byte, _ os.FileMode) error {
					return secureio.AtomicWritePrivateRuntimeFile(path, data)
				}
			}
			_ = writeFile(stateFile, append(body, '\n'), 0o600)
		}
	}
	s.Active = false
	s.exiting = false
	s.Path = ""
	s.Name = ""
	s.Branch = ""
	s.OriginalDir = ""
	s.OriginalHeadCommit = ""
	s.RepoRoot = ""
	s.StateFile = ""
	s.CreatedHere = false
	s.HookBased = false
	s.TmuxSessionName = ""
	s.lifecycle = nil
	if strings.TrimSpace(stateFile) != "" {
		_ = os.Remove(stateFile)
	}
}

// ─── EnterWorktreeTool ─────────────────────────────────────────────────────

// EnterWorktreeTool creates an isolated git worktree and records state.
type EnterWorktreeTool struct {
	State      *WorktreeState
	Manager    *WorktreeManager
	Runtime    *WorktreeRuntime
	SessionID  func() string
	HookBridge WorktreeHookBridge
}

type EnterWorktreeOutput struct {
	WorktreePath   string `json:"worktreePath"`
	WorktreeBranch string `json:"worktreeBranch,omitempty"`
	Message        string `json:"message"`
}

func (t *EnterWorktreeTool) Name() string { return "EnterWorktree" }

func (t *EnterWorktreeTool) Description() string {
	return runtimeText(i18n.KeyToolPromptWorktreeEnterDescription)
}

func (t *EnterWorktreeTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": runtimeText(i18n.KeyToolPromptWorktreeName),
			},
			"path": map[string]any{
				"type":        "string",
				"description": runtimeText(i18n.KeyToolPromptWorktreePath),
			},
			"base_ref": map[string]any{
				"type":        "string",
				"description": runtimeText(i18n.KeyToolPromptWorktreeBaseRef),
			},
		},
	)
}

// ─── ExitWorktreeTool ──────────────────────────────────────────────────────

// ExitWorktreeTool leaves the current worktree, optionally removing it.
type ExitWorktreeTool struct {
	State                   *WorktreeState
	Manager                 *WorktreeManager
	Runtime                 *WorktreeRuntime
	SessionID               func() string
	HookBridge              WorktreeHookBridge
	killTmuxSessionOverride func(context.Context, string) error
	sleepOverride           func(context.Context, time.Duration) error
}

type ExitWorktreeOutput struct {
	Action            string `json:"action"`
	OriginalCWD       string `json:"originalCwd"`
	WorktreePath      string `json:"worktreePath"`
	WorktreeBranch    string `json:"worktreeBranch,omitempty"`
	TmuxSessionName   string `json:"tmuxSessionName,omitempty"`
	DiscardedFiles    *int   `json:"discardedFiles,omitempty"`
	DiscardedCommits  *int   `json:"discardedCommits,omitempty"`
	CleanupIncomplete bool   `json:"cleanupIncomplete,omitempty"`
	CleanupIssueCount int    `json:"cleanupIssueCount,omitempty"`
	Message           string `json:"message"`
}

func (t *ExitWorktreeTool) Name() string { return "ExitWorktree" }

func (t *ExitWorktreeTool) Description() string {
	return runtimeText(i18n.KeyToolPromptWorktreeExitDescription)
}

func (t *ExitWorktreeTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"keep", "remove"},
				"description": runtimeText(i18n.KeyToolPromptWorktreeAction),
			},
			"discard_changes": map[string]any{
				"type":        "boolean",
				"description": runtimeText(i18n.KeyToolPromptWorktreeDiscardChanges),
			},
		},
		"action",
	)
}

func (s *WorktreeState) newLifecycle(repoRoot string) LifecyclePublisher {
	if s == nil || s.LifecycleFactory == nil {
		return nil
	}
	return s.LifecycleFactory(repoRoot)
}

func publishWorktreeLifecycle(lifecycle LifecyclePublisher, eventType LifecycleEventType, repoRoot, branch, path, status string, createdHere bool) {
	if lifecycle == nil {
		return
	}
	toolName := "EnterWorktree"
	if eventType == LifecycleExit {
		toolName = "ExitWorktree"
	}
	_ = lifecycle.PublishWorktreeLifecycle(context.Background(), LifecycleEvent{
		Type: eventType, EntityID: worktreeSessionID(repoRoot, branch, path),
		ToolName: toolName, Status: status, RepoRoot: repoRoot, Branch: branch,
		Path: path, CreatedHere: createdHere,
	})
}

// worktreeSessionID derives a deterministic short identifier for a worktree
// session. The marker is included in ExitWorktree output so the manager-aware
// cleanup path can audit each kept session without scraping prose.
func worktreeSessionID(repoRoot, branch, path string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(repoRoot))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(branch))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(path))
	return fmt.Sprintf("wt-%x", h.Sum64())
}
