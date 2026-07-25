package gitutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
)

const commandTimeout = 10 * time.Second

func noPromptEnv() []string {
	return append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=true",
		"SSH_ASKPASS=true",
		"SSH_ASKPASS_REQUIRE=never",
		"GIT_SSH_COMMAND=ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new",
	)
}

func Run(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = noPromptEnv()

	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

func CanonicalRoot(cwd string) (string, error) {
	cwd = cleanPath(cwd)
	if cwd == "" {
		return "", i18n.NewError(i18n.KeyToolWorktreeWorkingDirectoryEmpty)
	}
	commonDir, err := Run(cwd, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err == nil && strings.TrimSpace(commonDir) != "" {
		if root := cleanPath(filepath.Dir(strings.TrimSpace(commonDir))); root != "" {
			return root, nil
		}
	}
	topLevel, err := Run(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return cleanPath(topLevel), nil
}

func cleanPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = filepath.Clean(resolved)
	}
	return path
}
