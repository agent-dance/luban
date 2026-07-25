package agent

import (
	"context"
	"encoding/json"
	"errors"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type lifecycleErrorProvider struct{}

func (p lifecycleErrorProvider) Name() string    { return "lifecycle-error" }
func (p lifecycleErrorProvider) ModelID() string { return "lifecycle-error-model" }

func (p lifecycleErrorProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	return nil, errors.New("provider failed during background run")
}

func TestAgentLifecycleForegroundPersistsCompletedOutputAndMetadata(t *testing.T) {
	background := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(func() { _ = background.Shutdown(context.Background()) })
	tool := &AgentTool{
		Provider:   &mockProvider{responses: []string{"foreground lifecycle done"}},
		Registry:   registry.New(),
		Background: background,
	}

	result, err := tool.Execute(context.Background(), agentExecuteInput("foreground task", map[string]any{
		"name": "foreground-helper",
	}))
	if err != nil {
		t.Fatalf("Agent.Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected agent error: %s", result.Content)
	}
	var completed agentCompletedToolResult
	if err := json.Unmarshal([]byte(result.Content), &completed); err != nil {
		t.Fatalf("expected completed JSON result, got %q: %v", result.Content, err)
	}
	if completed.Status != "completed" || completed.AgentID == "" {
		t.Fatalf("unexpected completed payload: %+v", completed)
	}

	snap, ok := background.Snapshot(completed.AgentID)
	if !ok {
		t.Fatalf("expected foreground agent task snapshot for %s", completed.AgentID)
	}
	if snap.Status != "completed" || snap.Result != "foreground lifecycle done" {
		t.Fatalf("unexpected foreground snapshot: %+v", snap)
	}
	output, err := ReadTaskOutput(snap.OutputPath, 64*1024)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if !strings.Contains(output.Content, "foreground lifecycle done") {
		t.Fatalf("expected output file to contain foreground result, got: %s", output.Content)
	}

	record, ok := background.store.Get(completed.AgentID)
	if !ok {
		t.Fatalf("expected persisted runtime record for %s", completed.AgentID)
	}
	if record.Status != "completed" || record.Result != "foreground lifecycle done" || record.OutputPath != snap.OutputPath {
		t.Fatalf("runtime record diverged from snapshot: record=%+v snapshot=%+v", record, snap)
	}
	if record.AgentInput == nil || record.AgentInput.Name != "foreground-helper" {
		t.Fatalf("expected persisted agent input/name, got %+v", record.AgentInput)
	}
	if record.AgentMetadata == nil || record.AgentMetadata.AgentType == "" {
		t.Fatalf("expected persisted agent metadata, got %+v", record.AgentMetadata)
	}
}

func TestAgentLifecycleBackgroundErrorPersistsFailedTaskOutput(t *testing.T) {
	background := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(func() { _ = background.Shutdown(context.Background()) })
	tool := &AgentTool{
		Provider:   lifecycleErrorProvider{},
		Registry:   registry.New(),
		Background: background,
	}

	result, err := tool.Execute(context.Background(), agentExecuteInput("background failure", map[string]any{
		"run_in_background": true,
	}))
	if err != nil {
		t.Fatalf("Agent.Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("launch should succeed before background failure is observed: %s", result.Content)
	}
	var launch agentAsyncToolResult
	if err := json.Unmarshal([]byte(result.Content), &launch); err != nil {
		t.Fatalf("expected async launch JSON, got %q: %v", result.Content, err)
	}

	snap, status := background.Wait(launch.AgentID, 2*time.Second)
	if status != "success" {
		t.Fatalf("expected failed task to reach terminal state, wait status=%s snapshot=%+v", status, snap)
	}
	publicError := i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeErrorPublicSummary)
	if snap.Status != "failed" || snap.Error != publicError {
		t.Fatalf("expected failed snapshot with safe runtime projection, got %+v", snap)
	}
	if strings.Contains(snap.Error, "provider failed during background run") {
		t.Fatalf("failed snapshot exposed private provider error: %+v", snap)
	}
	record, ok := background.store.Get(launch.AgentID)
	if !ok {
		t.Fatalf("expected persisted failed record for %s", launch.AgentID)
	}
	if record.Status != "failed" || record.ExitCode == nil || *record.ExitCode != -1 || record.Error != publicError {
		t.Fatalf("unexpected failed runtime record: %+v", record)
	}

	outputResult, err := ReadTaskOutput(snap.OutputPath, 64*1024)
	if err != nil {
		t.Fatalf("ReadTaskOutput: %v", err)
	}
	if !strings.Contains(outputResult.Content, publicError) {
		t.Fatalf("expected safe failed output, got: %s", outputResult.Content)
	}
	if strings.Contains(outputResult.Content, "provider failed during background run") {
		t.Fatalf("background output exposed private provider error: %s", outputResult.Content)
	}
}

func TestAgentResumeWorktreeValidatesMissingPathMetadata(t *testing.T) {
	root := t.TempDir()
	background := NewBackgroundTaskManager(root)
	t.Cleanup(func() { _ = background.Shutdown(context.Background()) })
	tool := &AgentTool{
		Provider:   &mockProvider{responses: []string{"unused"}},
		Registry:   registry.New(),
		Background: background,
	}
	missingWorktree := filepath.Join(root, ".luban-code", "worktrees", "missing-agent")
	record := runtimestore.RuntimeTaskRecord{
		ID:          "agent-missing-worktree",
		Type:        agentcontract.TaskTypeLocalAgent,
		Status:      "completed",
		Description: "worktree task",
		Prompt:      "resume me",
		AgentAlias:  "worktree-helper",
		AgentInput: &agentcontract.Input{
			Name:        "worktree-helper",
			Description: "worktree task",
			Prompt:      "resume me",
		},
		AgentMetadata: &agentcontract.SessionMetadata{
			AgentType:          "general-purpose",
			Provider:           "mock",
			Model:              "mock-model",
			ApprovalRouting:    agentcontract.ApprovalFailClosed,
			CWD:                missingWorktree,
			Isolation:          "worktree",
			WorktreePath:       missingWorktree,
			PermissionSnapshot: &types.ToolRuntimeContext{ProjectRoot: missingWorktree, AllowedDirs: []string{missingWorktree}, PermissionMode: permissionModeDefault},
		},
	}
	trustAgentResumeForTest(background, record.ID, *record.AgentInput, *record.AgentMetadata)

	err := tool.RestoreAgentSession(record.ID, record)
	if err == nil {
		t.Fatalf("expected missing worktree metadata validation error")
	}
	want := toolRuntimeFormat(i18n.KeyToolAgentDeepPersistedWorktreeBranchMissing, missingWorktree)
	if err.Error() != want {
		t.Fatalf("expected missing branch metadata error, got: %v", err)
	}
}

func TestAgentResumeWorktreePreservesExistingPathMetadata(t *testing.T) {
	root := t.TempDir()
	worktreePath := t.TempDir()
	background := NewBackgroundTaskManager(root)
	t.Cleanup(func() { _ = background.Shutdown(context.Background()) })
	tool := &AgentTool{
		Provider:   &mockProvider{responses: []string{"resumed worktree"}},
		Registry:   registry.New(),
		Background: background,
	}
	record := runtimestore.RuntimeTaskRecord{
		ID:          "agent-existing-worktree",
		Type:        agentcontract.TaskTypeLocalAgent,
		Status:      "completed",
		Description: "worktree task",
		Prompt:      "resume me",
		AgentAlias:  "worktree-helper",
		AgentInput: &agentcontract.Input{
			Name:        "worktree-helper",
			Description: "worktree task",
			Prompt:      "resume me",
		},
		AgentMetadata: &agentcontract.SessionMetadata{
			AgentType:          "general-purpose",
			Provider:           "mock",
			Model:              "mock-model",
			ApprovalRouting:    agentcontract.ApprovalFailClosed,
			CWD:                worktreePath,
			Isolation:          "worktree",
			WorktreePath:       worktreePath,
			WorktreeBranch:     "agent-existing-worktree",
			WorktreeHeadCommit: "abc123",
			PermissionSnapshot: &types.ToolRuntimeContext{ProjectRoot: worktreePath, AllowedDirs: []string{worktreePath}, PermissionMode: permissionModeDefault},
		},
	}
	trustAgentResumeForTest(background, record.ID, *record.AgentInput, *record.AgentMetadata)

	err := tool.RestoreAgentSession(record.ID, record)
	if err != nil {
		t.Fatalf("RestoreAgentSession: %v", err)
	}
	if restoredAgentSessionForTest(background, record.ID) == nil {
		t.Fatal("expected restored session")
	}
	if _, ok := background.Snapshot(record.ID); !ok {
		t.Fatalf("expected restored session and snapshot")
	}
	restored, ok := background.store.Get(record.ID)
	if !ok {
		t.Fatalf("expected restored runtime record")
	}
	if restored.AgentMetadata == nil {
		t.Fatalf("expected restored metadata")
	}
	if restored.AgentMetadata.WorktreePath != worktreePath || restored.AgentMetadata.CWD != worktreePath {
		t.Fatalf("expected worktree metadata to be preserved, got %+v", restored.AgentMetadata)
	}
	if restored.OutputPath == "" {
		t.Fatalf("expected restored record to have output path")
	}
}
