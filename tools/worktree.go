package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

const gitTimeout = 10 * time.Second

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
	OriginalBranch     string
	OriginalHeadCommit string
	BaseRef            string
	RepoRoot           string // canonical repo root
	StateFile          string // persisted session state
	CreatedHere        bool   // true if EnterWorktree created this worktree (vs entered an existing one)
	HookBased          bool
	TmuxSessionName    string
	CurrentDir         string
	writeFile          func(string, []byte, os.FileMode) error
	runtime            *WorktreeRuntime
	lifecycle          *RuntimeLifecycle
}

type persistedWorktreeState struct {
	SessionID          string `json:"session_id,omitempty"`
	Active             bool   `json:"active"`
	Path               string `json:"path"`
	Name               string `json:"name,omitempty"`
	Branch             string `json:"branch"`
	OriginalDir        string `json:"original_dir"`
	OriginalBranch     string `json:"original_branch,omitempty"`
	OriginalHeadCommit string `json:"original_head_commit,omitempty"`
	BaseRef            string `json:"base_ref,omitempty"`
	RepoRoot           string `json:"repo_root"`
	CreatedHere        bool   `json:"created_here"`
	HookBased          bool   `json:"hook_based,omitempty"`
	TmuxSessionName    string `json:"tmux_session_name,omitempty"`
	CurrentDir         string `json:"current_dir,omitempty"`
}

// gitNoPromptEnv returns the environment variables that prevent git from
// blocking on credential / SSH prompts. Mirrors the TS GIT_NO_PROMPT_ENV
// constants. Without these a single git invocation against a private remote
// can hang the agent for the full timeout window with no recovery path.
func gitNoPromptEnv() []string {
	env := os.Environ()
	env = append(env,
		"GIT_TERMINAL_PROMPT=0",
		// On POSIX systems /bin/true is a noop. On Windows there is no
		// equivalent; the empty value works because git just exec's the
		// program and an empty exec is treated as "no askpass".
		"GIT_ASKPASS=true",
		"SSH_ASKPASS=true",
		// SSH_ASKPASS_REQUIRE=never disables interactive SSH askpass on
		// modern OpenSSH builds.
		"SSH_ASKPASS_REQUIRE=never",
		// BatchMode=yes forbids any password / passphrase prompts; the
		// connection just fails fast instead of blocking on a TTY read.
		"GIT_SSH_COMMAND=ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new",
	)
	return env
}

// runGit runs a git command with a 10-second timeout, rooted at dir (if non-empty).
// It returns combined stdout+stderr and any error.
func runGit(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = gitNoPromptEnv()

	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

func validateWorktreeSlug(slug string) error {
	if len(slug) > maxWorktreeSlugLength {
		return i18n.NewError(i18n.KeyToolWorktreeNameTooLong, maxWorktreeSlugLength, len(slug))
	}
	for _, segment := range strings.Split(slug, "/") {
		if segment == "." || segment == ".." {
			return i18n.NewError(i18n.KeyToolWorktreeNamePathSegment, slug)
		}
		if !validWorktreeSlugSegment.MatchString(segment) {
			return i18n.NewError(i18n.KeyToolWorktreeNameCharacters, slug)
		}
	}
	return nil
}

func flattenWorktreeSlug(slug string) string {
	return strings.ReplaceAll(slug, "/", "+")
}

func canonicalGitRoot() (string, error) {
	cwd, _ := os.Getwd()
	return canonicalGitRootFrom(cwd)
}

func canonicalGitRootFrom(cwd string) (string, error) {
	cwd = cleanWorktreePath(cwd)
	if cwd == "" {
		return "", i18n.NewError(i18n.KeyToolWorktreeWorkingDirectoryEmpty)
	}
	commonDir, err := runGit(cwd, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err == nil && strings.TrimSpace(commonDir) != "" {
		root := cleanWorktreePath(filepath.Dir(strings.TrimSpace(commonDir)))
		if root != "" {
			return root, nil
		}
	}
	topLevel, err := runGit(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return cleanWorktreePath(topLevel), nil
}

func stateFilePathForRepo(repoRoot string) string {
	return filepath.Join(repoRoot, brand.ConfigDirName, "worktree-session.json")
}

func (s *WorktreeState) loadFromDisk(repoRoot string) bool {
	if strings.TrimSpace(repoRoot) == "" {
		return false
	}
	stateFile := worktreeStateFilePath(repoRoot, s.SessionID)
	data, err := os.ReadFile(stateFile)
	if err != nil && strings.TrimSpace(s.SessionID) == "" {
		// Read the legacy singleton file only for legacy, unscoped callers.
		stateFile = stateFilePathForRepo(repoRoot)
		data, err = os.ReadFile(stateFile)
	}
	if err != nil {
		return false
	}
	var persisted persistedWorktreeState
	if err := json.Unmarshal(data, &persisted); err != nil || !persisted.Active {
		return false
	}
	if owner := strings.TrimSpace(s.SessionID); owner != "" && strings.TrimSpace(persisted.SessionID) != "" && owner != strings.TrimSpace(persisted.SessionID) {
		return false
	}
	if info, err := os.Stat(persisted.Path); err != nil || !info.IsDir() {
		_ = os.Remove(stateFile)
		return false
	}
	s.SessionID = firstNonEmptyWorktreeHookValue(persisted.SessionID, s.SessionID)
	s.Active = persisted.Active
	s.Path = persisted.Path
	s.Name = persisted.Name
	s.Branch = persisted.Branch
	s.OriginalDir = persisted.OriginalDir
	s.OriginalBranch = persisted.OriginalBranch
	s.OriginalHeadCommit = persisted.OriginalHeadCommit
	s.BaseRef = persisted.BaseRef
	s.RepoRoot = persisted.RepoRoot
	s.CreatedHere = persisted.CreatedHere
	s.HookBased = persisted.HookBased
	s.TmuxSessionName = persisted.TmuxSessionName
	s.CurrentDir = firstNonEmptyWorktreeHookValue(persisted.CurrentDir, persisted.Path)
	s.StateFile = stateFile
	s.lifecycle = NewRuntimeLifecycle(repoRoot)
	return true
}

func (s *WorktreeState) saveToDiskLocked() error {
	if strings.TrimSpace(s.StateFile) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.StateFile), 0755); err != nil {
		return err
	}
	body := persistedWorktreeState{
		SessionID:          s.SessionID,
		Active:             s.Active,
		Path:               s.Path,
		Name:               s.Name,
		Branch:             s.Branch,
		OriginalDir:        s.OriginalDir,
		OriginalBranch:     s.OriginalBranch,
		OriginalHeadCommit: s.OriginalHeadCommit,
		BaseRef:            s.BaseRef,
		RepoRoot:           s.RepoRoot,
		CreatedHere:        s.CreatedHere,
		HookBased:          s.HookBased,
		TmuxSessionName:    s.TmuxSessionName,
		CurrentDir:         s.CurrentDir,
	}
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return err
	}
	writeFile := s.writeFile
	if writeFile == nil {
		writeFile = atomicWriteFile
	}
	return writeFile(s.StateFile, append(data, '\n'), 0644)
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
				writeFile = atomicWriteFile
			}
			_ = writeFile(stateFile, append(body, '\n'), 0o644)
		}
	}
	s.Active = false
	s.exiting = false
	s.Path = ""
	s.Name = ""
	s.Branch = ""
	s.OriginalDir = ""
	s.OriginalBranch = ""
	s.OriginalHeadCommit = ""
	s.BaseRef = ""
	s.RepoRoot = ""
	s.StateFile = ""
	s.CreatedHere = false
	s.HookBased = false
	s.TmuxSessionName = ""
	s.CurrentDir = ""
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
	return "Creates an isolated worktree (via git or configured hooks) and switches the session into it"
}

func (t *EnterWorktreeTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Optional name for the worktree. Each \"/\"-separated segment may contain only letters, digits, dots, underscores, and dashes; max 64 chars total. A random name is generated if not provided.",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Path to an existing worktree of the current repository. Mutually exclusive with name.",
			},
			"base_ref": map[string]any{
				"type":        "string",
				"description": "Base for a newly-created worktree: fresh, head, pr:<number>, or pr:<owner>/<repo>#<number>.",
			},
		},
	)
}

func (t *EnterWorktreeTool) ToolContract() types.ToolContract {
	outputSchema := types.StrictObjectSchema(
		map[string]any{
			"worktreePath": map[string]any{"type": "string"},
			"worktreeBranch": map[string]any{
				"type": "string",
			},
			"message": map[string]any{"type": "string"},
		},
		"worktreePath", "message",
	)
	return types.ToolContract{
		OutputSchema:       &outputSchema,
		Strict:             true,
		MaxResultSizeChars: 100_000,
	}
}

// findWorktreeInPorcelain parses `git worktree list --porcelain` output and
// returns whether absPath is registered, and the branch (best-effort).
//
// Porcelain format: paragraphs separated by blank lines. Each paragraph starts
// with `worktree <path>` and may contain `branch refs/heads/<name>`.
func findWorktreeInPorcelain(out, absPath string) (bool, string) {
	target := filepath.Clean(absPath)
	var (
		curPath   string
		curBranch string
	)
	finalize := func() (bool, string, bool) {
		if curPath != "" && filepath.Clean(curPath) == target {
			return true, curBranch, true
		}
		return false, "", false
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if matched, branch, done := finalize(); done {
				return matched, branch
			}
			curPath = ""
			curBranch = ""
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			// New paragraph — finalize the previous one before starting fresh.
			if matched, branch, done := finalize(); done {
				return matched, branch
			}
			curPath = strings.TrimPrefix(line, "worktree ")
			curBranch = ""
		} else if strings.HasPrefix(line, "branch ") {
			ref := strings.TrimPrefix(line, "branch ")
			curBranch = strings.TrimPrefix(ref, "refs/heads/")
		}
	}
	if matched, branch, done := finalize(); done {
		return matched, branch
	}
	return false, ""
}

// resolveAbsPath returns the absolute path for p, without requiring it to exist.
func resolveAbsPath(p string) (string, error) {
	// filepath.Abs resolves relative to cwd and cleans the path.
	// We import "path/filepath" indirectly via the standard library.
	cwd, err := os.Getwd()
	if err != nil {
		return p, err
	}
	if strings.HasPrefix(p, "/") {
		return p, nil
	}
	return cwd + "/" + strings.TrimPrefix(p, "./"), nil
}

// ─── ExitWorktreeTool ──────────────────────────────────────────────────────

// ExitWorktreeTool leaves the current worktree, optionally removing it.
type ExitWorktreeTool struct {
	State                *WorktreeState
	Manager              *WorktreeManager
	Runtime              *WorktreeRuntime
	SessionID            func() string
	HookBridge           WorktreeHookBridge
	KillTmuxSession      func(context.Context, string) error
	AnalyticsHook        func(string, map[string]any)
	CacheInvalidator     func()
	CleanupErrorObserver func(error)
	// CleanupRuntimeEventObserver receives the structured, audience-projectable
	// cleanup event. Its private cause is diagnostic-only; implementations must
	// project the event before presenting or serializing it.
	CleanupRuntimeEventObserver func(types.RuntimeEvent)
	Sleep                       func(context.Context, time.Duration) error
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
	return "Exits a worktree session created by EnterWorktree and restores the original working directory"
}

func (t *ExitWorktreeTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"keep", "remove"},
				"description": `"keep" leaves the worktree and branch on disk; "remove" deletes both.`,
			},
			"discard_changes": map[string]any{
				"type":        "boolean",
				"description": `Required true when action is "remove" and the worktree has uncommitted files or unmerged commits. The tool will refuse and list them otherwise.`,
			},
		},
		"action",
	)
}

func (t *ExitWorktreeTool) ToolContract() types.ToolContract {
	outputSchema := types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"action":            map[string]any{"type": "string", "enum": []string{"keep", "remove"}},
			"originalCwd":       map[string]any{"type": "string"},
			"worktreePath":      map[string]any{"type": "string"},
			"worktreeBranch":    map[string]any{"type": "string"},
			"tmuxSessionName":   map[string]any{"type": "string"},
			"discardedFiles":    map[string]any{"type": "number"},
			"discardedCommits":  map[string]any{"type": "number"},
			"cleanupIncomplete": map[string]any{"type": "boolean"},
			"cleanupIssueCount": map[string]any{"type": "number"},
			"message":           map[string]any{"type": "string"},
		},
		Required: []string{"action", "originalCwd", "worktreePath", "message"},
	}
	return types.ToolContract{OutputSchema: &outputSchema, Strict: true, MaxResultSizeChars: 100_000}
}

func publishWorktreeLifecycle(lifecycle *RuntimeLifecycle, eventType RuntimeLifecycleEventType, repoRoot, branch, path, status string, createdHere bool) {
	if lifecycle == nil {
		return
	}
	toolName := "EnterWorktree"
	if eventType == LifecycleWorktreeExit {
		toolName = "ExitWorktree"
	}
	_ = lifecycle.Publish(context.Background(), RuntimeLifecycleEvent{
		Type:     eventType,
		EntityID: worktreeSessionID(repoRoot, branch, path),
		ToolName: toolName,
		Status:   status,
		Payload: map[string]any{
			"repo_root":    repoRoot,
			"branch":       branch,
			"path":         path,
			"created_here": createdHere,
		},
	})
}

// unmergedCommits returns the short log of commits on `branch` that are not
// reachable from any obvious upstream. The second return is false if we
// couldn't determine an upstream — callers should treat that as "no
// information" and proceed.
func unmergedCommits(repoRoot, branch string) (string, bool) {
	candidates := []string{
		"origin/" + branch,
		"origin/HEAD",
	}
	for _, ref := range candidates {
		if _, err := runGit(repoRoot, "rev-parse", "--verify", "--quiet", ref); err != nil {
			continue
		}
		out, err := runGit(repoRoot, "log", "--oneline", ref+".."+branch)
		if err != nil {
			continue
		}
		return strings.TrimSpace(out), true
	}
	return "", false
}

// clearState zeros the WorktreeState under the lock.
func (t *ExitWorktreeTool) clearState() {
	t.State.mu.Lock()
	defer t.State.mu.Unlock()
	t.State.clearLocked()
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
