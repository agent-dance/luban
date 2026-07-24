package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// GitStatusTool shows the status of a git repository
type GitStatusTool struct{}

func (t *GitStatusTool) Name() string {
	return "GitStatus"
}

func (t *GitStatusTool) Description() string {
	return "Show the status of a git repository"
}

func (t *GitStatusTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"directory": map[string]any{
				"type":        "string",
				"description": "Path to git repository (defaults to current directory)",
			},
		},
		Required: []string{},
	}
}

func (t *GitStatusTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	dir := GetStringField(input, "directory", ".")

	cmd := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain")
	output, err := cmd.CombinedOutput()

	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolGitStatusFailed, err), nil
	}

	return ResponseJSON(map[string]any{
		"status": string(output),
	})
}

// GitDiffTool shows differences in a git repository
type GitDiffTool struct{}

func (t *GitDiffTool) Name() string {
	return "GitDiff"
}

func (t *GitDiffTool) Description() string {
	return "Show differences in a git repository"
}

func (t *GitDiffTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"directory": map[string]any{
				"type":        "string",
				"description": "Path to git repository",
			},
			"ref": map[string]any{
				"type":        "string",
				"description": "Reference to compare against (e.g., HEAD, main)",
			},
			"file": map[string]any{
				"type":        "string",
				"description": "Specific file to show diff for (optional)",
			},
		},
		Required: []string{},
	}
}

func (t *GitDiffTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	dir := GetStringField(input, "directory", ".")
	ref := GetStringField(input, "ref", "HEAD")
	file := GetStringField(input, "file", "")

	args := []string{"-C", dir, "diff", ref}
	if file != "" {
		args = append(args, "--", file)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolGitDiffFailed, err), nil
	}

	return ResponseJSON(map[string]any{
		"diff": string(output),
	})
}

// GitLogTool shows commit history
type GitLogTool struct{}

func (t *GitLogTool) Name() string {
	return "GitLog"
}

func (t *GitLogTool) Description() string {
	return "Show commit history of a git repository"
}

func (t *GitLogTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"directory": map[string]any{
				"type":        "string",
				"description": "Path to git repository",
			},
			"count": map[string]any{
				"type":        "number",
				"description": "Number of commits to show (default: 10)",
			},
			"format": map[string]any{
				"type":        "string",
				"description": "Format string for log output (default: short log format)",
			},
		},
		Required: []string{},
	}
}

func (t *GitLogTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	dir := GetStringField(input, "directory", ".")
	count := GetIntField(input, "count", 10)

	args := []string{
		"-C", dir, "log",
		"--oneline",
		"-n", fmt.Sprintf("%d", count),
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolGitLogFailed, err), nil
	}

	return ResponseJSON(map[string]any{
		"commits": string(output),
	})
}

// GitCommitTool commits changes
type GitCommitTool struct{}

func (t *GitCommitTool) Name() string {
	return "GitCommit"
}

func (t *GitCommitTool) Description() string {
	return "Commit changes in a git repository"
}

func (t *GitCommitTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"directory": map[string]any{
				"type":        "string",
				"description": "Path to git repository",
			},
			"message": map[string]any{
				"type":        "string",
				"description": "Commit message",
			},
			"all": map[string]any{
				"type":        "boolean",
				"description": "Stage all changes before committing",
			},
		},
		Required: []string{"message"},
	}
}

func (t *GitCommitTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	dir := GetStringField(input, "directory", ".")
	message, err := MustGetStringField(input, "message")
	if err != nil {
		return ErrorResponse(err), nil
	}

	all := GetBoolField(input, "all", false)

	args := []string{"-C", dir, "commit"}
	if all {
		args = append(args, "-a")
	}
	args = append(args, "-m", message)

	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolGitCommitFailed, err), nil
	}

	return ResponseJSON(map[string]any{
		"status": string(output),
	})
}

// GitPushTool pushes commits to remote
type GitPushTool struct{}

func (t *GitPushTool) Name() string {
	return "GitPush"
}

func (t *GitPushTool) Description() string {
	return "Push commits to a remote repository"
}

func (t *GitPushTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"directory": map[string]any{
				"type":        "string",
				"description": "Path to git repository",
			},
			"remote": map[string]any{
				"type":        "string",
				"description": "Remote name (default: origin)",
			},
			"branch": map[string]any{
				"type":        "string",
				"description": "Branch name (default: current branch)",
			},
		},
		Required: []string{},
	}
}

func (t *GitPushTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	dir := GetStringField(input, "directory", ".")
	remote := GetStringField(input, "remote", "origin")
	branch := GetStringField(input, "branch", "")

	args := []string{"-C", dir, "push", remote}
	if branch != "" {
		args = append(args, branch)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolGitPushFailed, err), nil
	}

	return ResponseJSON(map[string]any{
		"status": string(output),
	})
}

// GitPullTool pulls commits from remote
type GitPullTool struct{}

func (t *GitPullTool) Name() string {
	return "GitPull"
}

func (t *GitPullTool) Description() string {
	return "Pull commits from a remote repository"
}

func (t *GitPullTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"directory": map[string]any{
				"type":        "string",
				"description": "Path to git repository",
			},
			"remote": map[string]any{
				"type":        "string",
				"description": "Remote name (default: origin)",
			},
			"branch": map[string]any{
				"type":        "string",
				"description": "Branch name (default: current branch)",
			},
		},
		Required: []string{},
	}
}

func (t *GitPullTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	dir := GetStringField(input, "directory", ".")
	remote := GetStringField(input, "remote", "origin")
	branch := GetStringField(input, "branch", "")

	args := []string{"-C", dir, "pull", remote}
	if branch != "" {
		args = append(args, branch)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolGitPullFailed, err), nil
	}

	return ResponseJSON(map[string]any{
		"status": string(output),
	})
}

// GitBranchTool manages branches
type GitBranchTool struct{}

func (t *GitBranchTool) Name() string {
	return "GitBranch"
}

func (t *GitBranchTool) Description() string {
	return "Manage branches in a git repository"
}

func (t *GitBranchTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"directory": map[string]any{
				"type":        "string",
				"description": "Path to git repository",
			},
			"action": map[string]any{
				"type":        "string",
				"description": "Action: list, create, delete, rename",
			},
			"branch": map[string]any{
				"type":        "string",
				"description": "Branch name for create/delete/rename",
			},
			"new_branch": map[string]any{
				"type":        "string",
				"description": "New branch name for rename",
			},
		},
		Required: []string{"action"},
	}
}

func (t *GitBranchTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	dir := GetStringField(input, "directory", ".")
	action, err := MustGetStringField(input, "action")
	if err != nil {
		return ErrorResponse(err), nil
	}

	var args []string
	args = []string{"-C", dir, "branch"}

	switch strings.ToLower(action) {
	case "list":
		args = append(args, "-a")
	case "create":
		branch, err := MustGetStringField(input, "branch")
		if err != nil {
			return ErrorResponse(err), nil
		}
		args = append(args, branch)
	case "delete":
		branch, err := MustGetStringField(input, "branch")
		if err != nil {
			return ErrorResponse(err), nil
		}
		args = append(args, "-d", branch)
	case "rename":
		oldBranch, err := MustGetStringField(input, "branch")
		if err != nil {
			return ErrorResponse(err), nil
		}
		newBranch, err := MustGetStringField(input, "new_branch")
		if err != nil {
			return ErrorResponse(err), nil
		}
		args = append(args, "-m", oldBranch, newBranch)
	default:
		return toolRuntimeErrorf(i18n.KeyToolGitUnknownAction, action), nil
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolGitBranchFailed, err), nil
	}

	return ResponseJSON(map[string]any{
		"output": string(output),
	})
}

// GitCheckoutTool checks out branches or commits
type GitCheckoutTool struct{}

func (t *GitCheckoutTool) Name() string {
	return "GitCheckout"
}

func (t *GitCheckoutTool) Description() string {
	return "Checkout a branch or commit in a git repository"
}

func (t *GitCheckoutTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"directory": map[string]any{
				"type":        "string",
				"description": "Path to git repository",
			},
			"ref": map[string]any{
				"type":        "string",
				"description": "Branch or commit to checkout",
			},
			"create": map[string]any{
				"type":        "boolean",
				"description": "Create new branch if it doesn't exist",
			},
		},
		Required: []string{"ref"},
	}
}

func (t *GitCheckoutTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	dir := GetStringField(input, "directory", ".")
	ref, err := MustGetStringField(input, "ref")
	if err != nil {
		return ErrorResponse(err), nil
	}

	create := GetBoolField(input, "create", false)

	args := []string{"-C", dir, "checkout"}
	if create {
		args = append(args, "-b")
	}
	args = append(args, ref)

	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolGitCheckoutFailed, err), nil
	}

	return ResponseJSON(map[string]any{
		"status": string(output),
	})
}

// GitStageTool stages files for commit
type GitStageTool struct{}

func (t *GitStageTool) Name() string {
	return "GitStage"
}

func (t *GitStageTool) Description() string {
	return "Stage files for commit in a git repository"
}

func (t *GitStageTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"directory": map[string]any{
				"type":        "string",
				"description": "Path to git repository",
			},
			"files": map[string]any{
				"type":        "string",
				"description": "Files to stage (space-separated, or '.' for all)",
			},
		},
		Required: []string{"files"},
	}
}

func (t *GitStageTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	dir := GetStringField(input, "directory", ".")
	files, err := MustGetStringField(input, "files")
	if err != nil {
		return ErrorResponse(err), nil
	}

	args := []string{"-C", dir, "add"}
	if files == "." {
		args = append(args, ".")
	} else {
		// Split space-separated files
		for _, file := range strings.Fields(files) {
			args = append(args, file)
		}
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolGitAddFailed, err), nil
	}

	return ResponseJSON(map[string]any{
		"status": string(output),
	})
}

// GitCloneTool clones a repository
type GitCloneTool struct{}

func (t *GitCloneTool) Name() string {
	return "GitClone"
}

func (t *GitCloneTool) Description() string {
	return "Clone a git repository"
}

func (t *GitCloneTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"repository": map[string]any{
				"type":        "string",
				"description": "Repository URL or path",
			},
			"directory": map[string]any{
				"type":        "string",
				"description": "Target directory (optional)",
			},
			"depth": map[string]any{
				"type":        "number",
				"description": "Shallow clone depth (optional)",
			},
		},
		Required: []string{"repository"},
	}
}

func (t *GitCloneTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	repo, err := MustGetStringField(input, "repository")
	if err != nil {
		return ErrorResponse(err), nil
	}

	args := []string{"clone"}

	depth := GetIntField(input, "depth", 0)
	if depth > 0 {
		args = append(args, "--depth", fmt.Sprintf("%d", depth))
	}

	args = append(args, repo)

	dir := GetStringField(input, "directory", "")
	if dir != "" {
		args = append(args, dir)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolGitCloneFailed, err), nil
	}

	return ResponseJSON(map[string]any{
		"status": string(output),
	})
}
