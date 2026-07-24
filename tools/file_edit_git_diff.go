package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// EditGitDiff is the TS ToolUseDiff shape exposed by FileEditOutput.gitDiff.
type EditGitDiff struct {
	Filename   string  `json:"filename"`
	Status     string  `json:"status"`
	Additions  int     `json:"additions"`
	Deletions  int     `json:"deletions"`
	Changes    int     `json:"changes"`
	Patch      string  `json:"patch"`
	Repository *string `json:"repository"`
}

// EditGitDiffProvider makes remote diff computation injectable and testable.
type EditGitDiffProvider func(ctx context.Context, absPath string) (*EditGitDiff, error)

func defaultEditGitDiffProvider(ctx context.Context, absPath string) (*EditGitDiff, error) {
	if !IsRemoteGitDiffEnabled() || absPath == "" {
		return nil, nil
	}
	if _, err := exec.LookPath("git"); err != nil {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	repoRaw, err := exec.CommandContext(ctx, "git", "-C", filepath.Dir(absPath), "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return nil, nil
	}
	repoRoot := strings.TrimSpace(string(repoRaw))
	resolvedRepo := repoRoot
	if value, resolveErr := filepath.EvalSymlinks(repoRoot); resolveErr == nil {
		resolvedRepo = value
	}
	resolvedPath := absPath
	if value, resolveErr := filepath.EvalSymlinks(absPath); resolveErr == nil {
		resolvedPath = value
	}
	rel, err := filepath.Rel(resolvedRepo, resolvedPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return nil, nil
	}
	rel = filepath.ToSlash(rel)

	patchRaw, patchErr := exec.CommandContext(ctx, "git", "-C", repoRoot, "diff", "--no-color", "--no-ext-diff", "--unified=3", "--", rel).Output()
	patch := strings.TrimRight(string(patchRaw), "\n")
	status := "modified"
	if patchErr != nil {
		return nil, nil
	}
	if patch == "" {
		trackedErr := exec.CommandContext(ctx, "git", "-C", repoRoot, "ls-files", "--error-unmatch", "--", rel).Run()
		if trackedErr == nil {
			return nil, nil
		}
		status = "added"
		patchRaw, patchErr = exec.CommandContext(ctx, "git", "-C", repoRoot, "diff", "--no-color", "--no-ext-diff", "--no-index", "--", os.DevNull, resolvedPath).Output()
		if patchErr != nil {
			// git diff --no-index exits 1 when differences are present.
			if exitErr, ok := patchErr.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
				return nil, nil
			}
		}
		patch = strings.TrimRight(string(patchRaw), "\n")
	}

	additions, deletions := countUnifiedPatchChanges(patch)
	return &EditGitDiff{
		Filename:  rel,
		Status:    status,
		Additions: additions,
		Deletions: deletions,
		Changes:   additions + deletions,
		Patch:     patch,
	}, nil
}

func countUnifiedPatchChanges(patch string) (additions, deletions int) {
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			continue
		case strings.HasPrefix(line, "+"):
			additions++
		case strings.HasPrefix(line, "-"):
			deletions++
		}
	}
	return additions, deletions
}

func formatEditGitDiffError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%v", err)
}
