package prompt

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

const maxGitStatusChars = 2000

// GitContextOptions controls Git context collection.
type GitContextOptions struct {
	CWD                    string
	Timeout                time.Duration
	GitPath                string
	DisableGitInstructions bool
}

// LoadGitContext returns a git status snapshot for cwd.
// It returns an empty string for non-git directories, disabled git instructions,
// missing git, command failures, and timeouts.
func LoadGitContext(cwd string) string {
	return LoadGitContextWithOptions(GitContextOptions{CWD: cwd})
}

// LoadGitContextWithOptions returns a formatted git context using explicit options.
func LoadGitContextWithOptions(opts GitContextOptions) string {
	if opts.DisableGitInstructions || gitInstructionsDisabled() {
		return ""
	}
	cwd := strings.TrimSpace(opts.CWD)
	if cwd == "" {
		if current, err := os.Getwd(); err == nil {
			cwd = current
		}
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	gitPath := strings.TrimSpace(opts.GitPath)
	if gitPath == "" {
		gitPath = "git"
	}
	runner := gitRunner{cwd: cwd, timeout: timeout, gitPath: gitPath}

	if inside := runner.run("rev-parse", "--is-inside-work-tree"); inside != "true" {
		return ""
	}

	branch := runner.run("branch", "--show-current")
	if branch == "" {
		branch = runner.run("rev-parse", "--abbrev-ref", "HEAD")
	}
	mainBranch := runner.defaultBranch()
	status := runner.run("--no-optional-locks", "status", "--short")
	log := runner.run("--no-optional-locks", "log", "--oneline", "-n", "5")
	userName := runner.optional("config", "user.name")

	if runner.failed {
		return ""
	}
	status = truncateGitStatus(status)
	if strings.TrimSpace(status) == "" {
		status = "(clean)"
	}

	var parts []string
	parts = append(parts, "This is the git status at the start of the conversation. Note that this status is a snapshot in time, and will not update during the conversation.")
	parts = append(parts, "Current branch: "+emptyPlaceholder(branch))
	parts = append(parts, "Main branch (you will usually use this for PRs): "+emptyPlaceholder(mainBranch))
	if userName != "" {
		parts = append(parts, "Git user: "+userName)
	}
	parts = append(parts, "Status:\n"+status)
	if log != "" {
		parts = append(parts, "Recent commits:\n"+log)
	} else {
		parts = append(parts, "Recent commits:\n(none)")
	}
	return strings.Join(parts, "\n\n")
}

type gitRunner struct {
	cwd     string
	timeout time.Duration
	gitPath string
	failed  bool
}

func (r *gitRunner) run(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.gitPath, args...)
	cmd.Dir = r.cwd
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		r.failed = true
		return ""
	}
	return strings.TrimRight(out.String(), "\r\n")
}

func (r *gitRunner) optional(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.gitPath, args...)
	cmd.Dir = r.cwd
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimRight(out.String(), "\r\n")
}

func (r *gitRunner) defaultBranch() string {
	if ref := r.optional("symbolic-ref", "--short", "refs/remotes/origin/HEAD"); ref != "" {
		return strings.TrimPrefix(ref, "origin/")
	}
	for _, candidate := range []string{"main", "master"} {
		ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
		cmd := exec.CommandContext(ctx, r.gitPath, "show-ref", "--verify", "--quiet", "refs/heads/"+candidate)
		cmd.Dir = r.cwd
		err := cmd.Run()
		cancel()
		if err == nil {
			return candidate
		}
	}
	return "main"
}

func truncateGitStatus(status string) string {
	if len(status) <= maxGitStatusChars {
		return status
	}
	return status[:maxGitStatusChars] + "\n... (truncated because it exceeds 2k characters. If you need more information, run \"git status\" using Bash)"
}

func emptyPlaceholder(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(unknown)"
	}
	return strings.TrimSpace(value)
}

func gitInstructionsDisabled() bool {
	return isTruthyPromptEnv(os.Getenv("LUBAN_CODE_DISABLE_GIT_INSTRUCTIONS"))
}
