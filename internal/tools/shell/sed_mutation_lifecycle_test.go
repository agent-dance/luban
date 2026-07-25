package shell

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/contracts/filemutation"
	"github.com/agent-dance/luban/permissions"
)

type recordingMutationCoordinator struct {
	validated   [][]filemutation.Target
	committed   [][]filemutation.Target
	invalidated [][]filemutation.Target
}

func (c *recordingMutationCoordinator) Lock([]filemutation.Target) func() { return func() {} }

func (c *recordingMutationCoordinator) ValidateFullRead(_ context.Context, targets []filemutation.Target) error {
	c.validated = append(c.validated, cloneMutationTargets(targets))
	return nil
}

func (c *recordingMutationCoordinator) Commit(_ context.Context, targets []filemutation.Target, _ string) error {
	c.committed = append(c.committed, cloneMutationTargets(targets))
	return nil
}

func (c *recordingMutationCoordinator) Invalidate(_ context.Context, targets []filemutation.Target) {
	c.invalidated = append(c.invalidated, cloneMutationTargets(targets))
}

func cloneMutationTargets(targets []filemutation.Target) []filemutation.Target {
	return append([]filemutation.Target(nil), targets...)
}

type completedBackgroundRunner struct {
	exitCode int
}

func (r completedBackgroundRunner) StartShellCommand(_ context.Context, _, _ string, _ *exec.Cmd, _ time.Duration, completion func(error, int)) (string, string, error) {
	completion(nil, r.exitCode)
	return "task-1", "/tmp/task-1.output", nil
}

func inPlaceSedCommand(script, target string) string {
	if runtime.GOOS == "darwin" {
		return "sed -i '' '" + script + "' " + target
	}
	return "sed -i '" + script + "' " + target
}

func TestBashSedMutationEvidenceCommitsOnlyAfterSuccessfulExit(t *testing.T) {
	requireBashAvailable(t)
	root := t.TempDir()
	target := filepath.Join(root, "state.txt")
	if err := os.WriteFile(target, []byte("alpha\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	coordinator := &recordingMutationCoordinator{}
	tool := &BashTool{
		CWD: root, AllowedDirs: []string{root}, FileMutations: coordinator,
		PermissionRules: []permissions.Rule{{Tool: "Bash", Pattern: "sed *", Decision: permissions.DecisionAllow}},
	}
	result, err := executeApprovedBashForTest(t, tool, map[string]any{
		"command": inPlaceSedCommand("s/alpha/beta/", "state.txt"),
	})
	if err != nil || result.IsError {
		t.Fatalf("successful sed result=%#v err=%v", result, err)
	}
	if len(coordinator.validated) != 1 || len(coordinator.committed) != 1 || len(coordinator.invalidated) != 0 {
		t.Fatalf("successful mutation lifecycle: validate=%v commit=%v invalidate=%v", coordinator.validated, coordinator.committed, coordinator.invalidated)
	}
	if got, readErr := os.ReadFile(target); readErr != nil || string(got) != "beta\n" {
		t.Fatalf("successful sed content=%q err=%v", got, readErr)
	}
}

func TestBashSedMutationEvidenceInvalidatesAfterPartialCommandFailure(t *testing.T) {
	requireBashAvailable(t)
	root := t.TempDir()
	target := filepath.Join(root, "state.txt")
	if err := os.WriteFile(target, []byte("alpha\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	coordinator := &recordingMutationCoordinator{}
	tool := &BashTool{
		CWD: root, AllowedDirs: []string{root}, FileMutations: coordinator,
		PermissionRules: []permissions.Rule{{Tool: "Bash", Pattern: "*", Decision: permissions.DecisionAllow}},
	}
	command := inPlaceSedCommand("s/alpha/beta/", "state.txt") + "; false"
	result, err := executeApprovedBashForTest(t, tool, map[string]any{"command": command})
	if err != nil || !result.IsError {
		t.Fatalf("failing compound sed result=%#v err=%v", result, err)
	}
	if len(coordinator.validated) != 1 || len(coordinator.committed) != 0 || len(coordinator.invalidated) != 1 {
		t.Fatalf("failed mutation lifecycle: validate=%v commit=%v invalidate=%v", coordinator.validated, coordinator.committed, coordinator.invalidated)
	}
	if got, readErr := os.ReadFile(target); readErr != nil || string(got) != "beta\n" {
		t.Fatalf("fixture did not exercise a partial mutation: content=%q err=%v", got, readErr)
	}
}

func TestBashBackgroundSedInvalidatesEvidenceOnNonzeroCompletion(t *testing.T) {
	requireBashAvailable(t)
	root := t.TempDir()
	target := filepath.Join(root, "state.txt")
	if err := os.WriteFile(target, []byte("alpha\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	coordinator := &recordingMutationCoordinator{}
	tool := &BashTool{
		CWD: root, AllowedDirs: []string{root}, FileMutations: coordinator,
		Background:      completedBackgroundRunner{exitCode: 7},
		PermissionRules: []permissions.Rule{{Tool: "Bash", Pattern: "sed *", Decision: permissions.DecisionAllow}},
	}
	result, err := executeApprovedBashForTest(t, tool, map[string]any{
		"command":           inPlaceSedCommand("s/alpha/beta/", "state.txt"),
		"run_in_background": true,
	})
	if err != nil || result.IsError {
		t.Fatalf("background launch result=%#v err=%v", result, err)
	}
	if len(coordinator.validated) != 1 || len(coordinator.committed) != 0 || len(coordinator.invalidated) != 1 {
		t.Fatalf("background failed mutation lifecycle: validate=%v commit=%v invalidate=%v", coordinator.validated, coordinator.committed, coordinator.invalidated)
	}
}
