package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/agent-dance/luban/i18n"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type agentSnapshotPathPermissionHandler struct {
	mu       sync.Mutex
	requests []permission.PermissionRequest
}

func (h *agentSnapshotPathPermissionHandler) Check(_ context.Context, request permission.PermissionRequest) (permission.PermissionDecision, error) {
	h.mu.Lock()
	h.requests = append(h.requests, request)
	h.mu.Unlock()

	snapshot := request.PermissionSnapshot
	if snapshot == nil {
		return permission.PermissionDeny, nil
	}
	path := ""
	for _, key := range []string{"file_path", "path"} {
		if value, ok := request.Input[key].(string); ok && value != "" {
			path = value
			break
		}
	}
	if path == "" || !toolbase.PathWithinAllowedDirs(path, snapshot.AllowedDirs) {
		return permission.PermissionDeny, nil
	}
	return permission.PermissionAllow, nil
}

func (h *agentSnapshotPathPermissionHandler) snapshotRequests() []permission.PermissionRequest {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]permission.PermissionRequest(nil), h.requests...)
}

func TestAgentCWDCannotExpandParentAllowedDirectories(t *testing.T) {
	parentRoot := t.TempDir()
	outsideRoot := t.TempDir()
	reg := registry.New()
	tool := &AgentTool{
		Provider: &captureAgentProvider{},
		Registry: reg,
	}
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: types.ToolRuntimeContext{
		ProjectRoot:    parentRoot,
		AllowedDirs:    []string{parentRoot},
		PermissionMode: "bypassPermissions",
	}})

	bundle, err := tool.buildSubAgentLoopWithOptions("agent-cwd-escape", agentcontract.Input{
		Prompt: "inspect outside",
		CWD:    outsideRoot,
	}, agentLoopOptions{Profile: &agentProfile{Name: "general-purpose"}})
	if err == nil {
		runAgentCleanup(bundle.Cleanup)
		t.Fatal("subagent cwd outside the parent permission scope was accepted")
	}
	if err.Error() != i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolAgentDeepCWDOutsideParentScope) {
		t.Fatalf("cwd escape error = %v", err)
	}
}

func TestAgentCWDMayNarrowParentAllowedDirectories(t *testing.T) {
	parentRoot := t.TempDir()
	childRoot := parentRoot + "/child"
	if err := os.MkdirAll(childRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	reg := registry.New()
	tool := &AgentTool{Provider: &captureAgentProvider{}, Registry: reg}
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: types.ToolRuntimeContext{
		ProjectRoot: parentRoot,
		AllowedDirs: []string{parentRoot},
	}})

	bundle, err := tool.buildSubAgentLoopWithOptions("agent-cwd-narrow", agentcontract.Input{
		Prompt: "inspect child",
		CWD:    childRoot,
	}, agentLoopOptions{Profile: &agentProfile{Name: "general-purpose"}})
	if err != nil {
		t.Fatalf("narrow child cwd: %v", err)
	}
	defer runAgentCleanup(bundle.Cleanup)
	if got := bundle.Metadata.CWD; got != childRoot {
		t.Fatalf("child cwd = %q, want %q", got, childRoot)
	}
}

func TestAgentWorktreeUsesOneEffectiveChildPermissionSnapshot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	repo := t.TempDir()
	runGitCommand(t, repo, "init")
	runGitCommand(t, repo, "config", "user.email", "test@example.com")
	runGitCommand(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "note.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCommand(t, repo, "add", "note.txt")
	runGitCommand(t, repo, "commit", "-m", "initial")

	parentPermissions := &agentSnapshotPathPermissionHandler{}
	parentSnapshot := types.ToolRuntimeContext{
		SessionID:      "parent-session",
		ProjectRoot:    repo,
		AllowedDirs:    []string{repo},
		PermissionMode: permissionModeDefault,
		AllowedTools:   map[string]bool{"Write": true, "Glob": true},
		DeniedRules:    []types.PermissionRuleValue{{ToolName: "Write", RuleContent: filepath.Join(repo, "blocked.txt")}},
	}
	tool := &AgentTool{
		Provider:          &captureAgentProvider{},
		Registry:          registry.New(),
		PermissionHandler: parentPermissions,
	}
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: parentSnapshot})

	bundle, err := tool.buildSubAgentLoopWithOptions("agent-worktree-permissions", agentcontract.Input{
		Prompt:    "inspect worktree",
		Isolation: "worktree",
	}, agentLoopOptions{Profile: &agentProfile{Name: "general-purpose"}})
	if err != nil {
		t.Fatalf("build worktree agent: %v", err)
	}
	defer runAgentCleanup(bundle.Cleanup)
	t.Cleanup(func() {
		_, _ = cleanupAgentWorktreeIfClean(bundle.Metadata)
	})

	worktree := filepath.Clean(bundle.Metadata.WorktreePath)
	if worktree == "" || bundle.PermissionHandler == nil || bundle.Metadata.PermissionSnapshot == nil {
		t.Fatalf("incomplete worktree permission bundle: metadata=%+v handler=%T", bundle.Metadata, bundle.PermissionHandler)
	}
	wantSnapshot := cloneToolRuntimeContext(parentSnapshot)
	wantSnapshot.ProjectRoot = worktree
	wantSnapshot.AllowedDirs = []string{worktree}
	if !reflect.DeepEqual(*bundle.Metadata.PermissionSnapshot, wantSnapshot) {
		t.Fatalf("persisted child snapshot = %+v, want %+v", *bundle.Metadata.PermissionSnapshot, wantSnapshot)
	}
	if !reflect.DeepEqual(bundle.PermissionHandler.snapshot, wantSnapshot) {
		t.Fatalf("permission handler snapshot = %+v, want %+v", bundle.PermissionHandler.snapshot, wantSnapshot)
	}

	insideRequests := []permission.PermissionRequest{
		{SessionID: "child-session", ToolName: "Write", Input: map[string]any{"file_path": filepath.Join(worktree, "result.txt")}},
		{SessionID: "child-session", ToolName: "Glob", Input: map[string]any{"path": worktree, "pattern": "**/*"}},
	}
	for _, request := range insideRequests {
		decision, checkErr := bundle.PermissionHandler.Check(context.Background(), request)
		if checkErr != nil || decision != permission.PermissionAllow {
			t.Fatalf("worktree-local %s decision=%v err=%v, want allow without boundary request", request.ToolName, decision, checkErr)
		}
	}

	parentRequests := []permission.PermissionRequest{
		{SessionID: "child-session", ToolName: "Write", Input: map[string]any{"file_path": filepath.Join(repo, "parent-result.txt")}},
		{SessionID: "child-session", ToolName: "Glob", Input: map[string]any{"path": repo, "pattern": "**/*"}},
	}
	for _, request := range parentRequests {
		decision, checkErr := bundle.PermissionHandler.Check(context.Background(), request)
		if checkErr != nil || decision != permission.PermissionDeny {
			t.Fatalf("parent-checkout %s decision=%v err=%v, want boundary denial", request.ToolName, decision, checkErr)
		}
	}

	for _, request := range parentPermissions.snapshotRequests() {
		if request.PermissionSnapshot == nil || !reflect.DeepEqual(*request.PermissionSnapshot, wantSnapshot) {
			t.Fatalf("forwarded %s snapshot = %+v, want %+v", request.ToolName, request.PermissionSnapshot, wantSnapshot)
		}
	}
}
