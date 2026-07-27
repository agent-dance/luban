package pierbackend

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
	"github.com/agent-dance/luban/i18n"
)

type runtimeSourceIntegrityError struct {
	cause error
}

func (err runtimeSourceIntegrityError) Error() string {
	return err.cause.Error()
}

func (err runtimeSourceIntegrityError) Unwrap() error {
	return err.cause
}

func isRuntimeSourceIntegrityError(err error) bool {
	var target runtimeSourceIntegrityError
	return errors.As(err, &target)
}

// requirePristineGitSourceRoot rejects every filesystem entry under a pinned
// source root that is not the exact checked-out commit: tracked/index changes,
// untracked files, and ignored build/runtime output all invalidate provenance.
// Runtime directories are allowed only when they live outside sourceRoot.
func requirePristineGitSourceRoot(ctx context.Context, repositoryRoot, sourceRoot string) error {
	dirty, err := inspectGitSourceRoot(ctx, repositoryRoot, sourceRoot)
	if err != nil {
		return i18n.WrapInternalError(i18n.KeyBenchmarkSourceInspectFailed, err)
	}
	if dirty {
		return i18n.NewError(i18n.KeyBenchmarkSourceNotPristine)
	}
	return nil
}

func inspectGitSourceRoot(ctx context.Context, repositoryRoot, sourceRoot string) (bool, error) {
	absoluteSource, err := pathWithin(repositoryRoot, sourceRoot)
	if err != nil {
		return false, err
	}
	relativeSource, err := filepath.Rel(repositoryRoot, absoluteSource)
	if err != nil {
		return false, err
	}
	relativeSource = filepath.ToSlash(relativeSource)
	command := exec.CommandContext(ctx, "git",
		"--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false",
		"-C", repositoryRoot, "--literal-pathspecs", "status",
		"--porcelain=v2", "-z", "--untracked-files=all", "--ignored=matching",
		"--ignore-submodules=none", "--", relativeSource,
	)
	command.Env = sanitizedProcessEnvironment(nil, "")
	command.Env = replaceEnvironment(command.Env, "GIT_CONFIG_NOSYSTEM", "1")
	command.Env = replaceEnvironment(command.Env, "GIT_CONFIG_GLOBAL", os.DevNull)
	command.Env = replaceEnvironment(command.Env, "GIT_OPTIONAL_LOCKS", "0")
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	commandErr := command.Run()
	if commandErr != nil || stderr.Len() != 0 {
		diagnostic := strings.TrimSpace(stderr.String())
		if diagnostic == "" {
			if commandErr != nil {
				diagnostic = commandErr.Error()
			} else {
				diagnostic = "git status emitted an empty diagnostic"
			}
		}
		return false, errors.Join(commandErr, errors.New(diagnostic))
	}
	return stdout.Len() != 0, nil
}

func requireSourceTreeUnchanged(ctx context.Context, repositoryRoot, sourceRoot string, before harness.TreeInventory) error {
	return requireSourceTreeUnchangedWithKey(ctx, repositoryRoot, sourceRoot, before, i18n.KeyBenchmarkSourceMutatedDuringPreflight)
}

func requireSourceTreeUnchangedWithKey(ctx context.Context, repositoryRoot, sourceRoot string, before harness.TreeInventory, mutationKey i18n.Key) error {
	dirty, err := inspectGitSourceRoot(ctx, repositoryRoot, sourceRoot)
	if err != nil {
		return i18n.WrapInternalError(i18n.KeyBenchmarkSourceInspectFailed, err)
	}
	if dirty {
		return i18n.NewError(mutationKey)
	}
	after, err := harness.HashTree(filepath.Join(repositoryRoot, sourceRoot))
	if err != nil {
		return i18n.WrapInternalError(i18n.KeyBenchmarkSourceInspectFailed, err)
	}
	if after.SchemaVersion != before.SchemaVersion || after.SHA256 != before.SHA256 ||
		after.RawSHA256 != before.RawSHA256 || len(after.Files) != len(before.Files) {
		return i18n.NewError(mutationKey)
	}
	return nil
}
