package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/types"
)

// setupGitRepo creates a temporary git repository for testing
func setupGitRepo(t *testing.T) string {
	tmpDir := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git user
	for _, cfg := range []struct{ key, val string }{
		{"user.email", "test@example.com"},
		{"user.name", "Test User"},
	} {
		cmd := exec.Command("git", "config", cfg.key, cfg.val)
		cmd.Dir = tmpDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to configure git: %v", err)
		}
	}

	// Create and commit initial file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", "test.txt")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}

	return tmpDir
}

func TestGitStatusTool(t *testing.T) {
	repo := setupGitRepo(t)
	tool := &GitStatusTool{}

	// Test with clean status
	result, err := tool.Execute(context.Background(), map[string]any{
		"directory": repo,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("Tool returned error: %s", result.Content)
	}

	// Modify file to get dirty status
	testFile := filepath.Join(repo, "test.txt")
	if err := os.WriteFile(testFile, []byte("modified content"), 0644); err != nil {
		t.Fatalf("Failed to modify file: %v", err)
	}

	result, err = tool.Execute(context.Background(), map[string]any{
		"directory": repo,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("Tool returned error: %s", result.Content)
	}
}

func TestGitDiffTool(t *testing.T) {
	repo := setupGitRepo(t)
	tool := &GitDiffTool{}

	// Modify file
	testFile := filepath.Join(repo, "test.txt")
	if err := os.WriteFile(testFile, []byte("modified content"), 0644); err != nil {
		t.Fatalf("Failed to modify file: %v", err)
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"directory": repo,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("Tool returned error: %s", result.Content)
	}
}

func TestGitLogTool(t *testing.T) {
	repo := setupGitRepo(t)
	tool := &GitLogTool{}

	result, err := tool.Execute(context.Background(), map[string]any{
		"directory": repo,
		"count":     5,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("Tool returned error: %s", result.Content)
	}
}

func TestGitCommitTool(t *testing.T) {
	repo := setupGitRepo(t)
	tool := &GitCommitTool{}

	// Modify file
	testFile := filepath.Join(repo, "test.txt")
	if err := os.WriteFile(testFile, []byte("new content"), 0644); err != nil {
		t.Fatalf("Failed to modify file: %v", err)
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"directory": repo,
		"message":   "Test commit",
		"all":       true,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("Tool returned error: %s", result.Content)
	}
}

func TestGitBranchTool(t *testing.T) {
	repo := setupGitRepo(t)
	tool := &GitBranchTool{}

	// List branches
	result, err := tool.Execute(context.Background(), map[string]any{
		"directory": repo,
		"action":    "list",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("Tool returned error: %s", result.Content)
	}

	// Create branch
	result, err = tool.Execute(context.Background(), map[string]any{
		"directory": repo,
		"action":    "create",
		"branch":    "test-branch",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("Tool returned error: %s", result.Content)
	}
}

func TestGitCheckoutTool(t *testing.T) {
	repo := setupGitRepo(t)

	// Create a branch first
	cmd := exec.Command("git", "branch", "test-branch")
	cmd.Dir = repo
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create branch: %v", err)
	}

	tool := &GitCheckoutTool{}

	result, err := tool.Execute(context.Background(), map[string]any{
		"directory": repo,
		"ref":       "test-branch",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("Tool returned error: %s", result.Content)
	}
}

func TestGitStageTool(t *testing.T) {
	repo := setupGitRepo(t)
	tool := &GitStageTool{}

	// Create new file
	newFile := filepath.Join(repo, "new.txt")
	if err := os.WriteFile(newFile, []byte("new file"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"directory": repo,
		"files":     "new.txt",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("Tool returned error: %s", result.Content)
	}
}

func TestGitPushTool(t *testing.T) {
	repo := setupGitRepo(t)
	tool := &GitPushTool{}

	// Push will fail without remote, but we test the tool structure
	result, err := tool.Execute(context.Background(), map[string]any{
		"directory": repo,
		"remote":    "origin",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	// Error expected since no remote configured - but tool should return error result, not Go error
	_ = result
}

func TestGitPullTool(t *testing.T) {
	repo := setupGitRepo(t)
	tool := &GitPullTool{}

	// Pull will fail without remote, but we test the tool structure
	result, err := tool.Execute(context.Background(), map[string]any{
		"directory": repo,
		"remote":    "origin",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	// Error expected since no remote configured - but tool should return error result, not Go error
	_ = result
}

func TestGitCloneTool(t *testing.T) {
	tool := &GitCloneTool{}
	destDir := t.TempDir()

	// Clone this repo (will fail without valid URL, but test structure)
	result, err := tool.Execute(context.Background(), map[string]any{
		"repository": "https://invalid-url-that-will-fail.example.com/repo.git",
		"directory":  filepath.Join(destDir, "cloned"),
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	// Error expected for invalid URL - tool should return error result, not Go error
	_ = result
}

// Test schema validation
func TestGitToolSchemas(t *testing.T) {
	tests := []struct {
		name string
		tool types.Tool
	}{
		{"GitStatus", &GitStatusTool{}},
		{"GitDiff", &GitDiffTool{}},
		{"GitLog", &GitLogTool{}},
		{"GitCommit", &GitCommitTool{}},
		{"GitPush", &GitPushTool{}},
		{"GitPull", &GitPullTool{}},
		{"GitBranch", &GitBranchTool{}},
		{"GitCheckout", &GitCheckoutTool{}},
		{"GitStage", &GitStageTool{}},
		{"GitClone", &GitCloneTool{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema := test.tool.Schema()
			if schema.Type != "object" {
				t.Errorf("Expected schema type 'object', got '%s'", schema.Type)
			}
			if len(test.tool.Name()) == 0 {
				t.Error("Tool name is empty")
			}
			if len(test.tool.Description()) == 0 {
				t.Error("Tool description is empty")
			}
		})
	}
}
