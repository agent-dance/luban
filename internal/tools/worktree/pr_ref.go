package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/gitutil"
)

// worktree_pr_ref.go — WT-01: support `pr:<num>` and `pr:<owner>/<repo>#<num>`
// references for EnterWorktree.
//
// Without this, reviewers cannot spawn isolated PR worktrees from Go and
// the common review workflow regresses vs the TS implementation.

// prRefShortPattern matches "pr:<num>".
var prRefShortPattern = regexp.MustCompile(`^pr:([0-9]+)$`)

// prRefFullPattern matches "pr:<owner>/<repo>#<num>".
var prRefFullPattern = regexp.MustCompile(`^pr:([^/]+)/([^#]+)#([0-9]+)$`)

// parsedPRRef is a structured form of a parsed PR reference.
type parsedPRRef struct {
	Number int
	Owner  string
	Repo   string
	Remote string // resolved remote name (origin or owner/repo upstream)
}

// isPRRef reports whether the supplied base_ref looks like a PR reference.
func isPRRef(baseRef string) bool {
	s := strings.ToLower(strings.TrimSpace(baseRef))
	return strings.HasPrefix(s, "pr:")
}

// parsePRRef parses a PR reference string. Returns nil if the input is not
// a PR reference.
func parsePRRef(baseRef string) (*parsedPRRef, error) {
	s := strings.TrimSpace(baseRef)
	if !strings.HasPrefix(strings.ToLower(s), "pr:") {
		return nil, nil
	}
	if m := prRefShortPattern.FindStringSubmatch(s); m != nil {
		num, err := strconv.Atoi(m[1])
		if err != nil || num <= 0 {
			return nil, i18n.NewError(i18n.KeyToolWorktreePRNumberInvalid, baseRef)
		}
		return &parsedPRRef{Number: num, Remote: "origin"}, nil
	}
	if m := prRefFullPattern.FindStringSubmatch(s); m != nil {
		num, err := strconv.Atoi(m[3])
		if err != nil || num <= 0 {
			return nil, i18n.NewError(i18n.KeyToolWorktreePRNumberInvalid, baseRef)
		}
		return &parsedPRRef{
			Owner:  m[1],
			Repo:   m[2],
			Number: num,
			Remote: "origin",
		}, nil
	}
	return nil, i18n.NewError(i18n.KeyToolWorktreePRReferenceInvalid, baseRef)
}

// preparePRRef fetches the PR head ref into a local refspec and returns the
// resolved local ref name (e.g. `refs/pr/<num>/head`) for use with `git
// worktree add`. Caller must run this from inside the repo root.
func preparePRRef(repoRoot string, ref *parsedPRRef) (string, error) {
	if ref == nil {
		return "", i18n.NewError(i18n.KeyToolWorktreePRReferenceNil)
	}
	localRef := fmt.Sprintf("refs/pr/%d/head", ref.Number)
	remote := ref.Remote
	if remote == "" {
		remote = "origin"
	}
	refSpec := fmt.Sprintf("pull/%d/head:%s", ref.Number, localRef)
	if out, err := gitutil.Run(repoRoot, "fetch", remote, refSpec); err != nil {
		return "", i18n.NewError(i18n.KeyToolWorktreePRFetchFailed, remote, refSpec, out)
	}
	return localRef, nil
}

// suggestedPRWorktreeName returns a friendly slug for a PR worktree.
func suggestedPRWorktreeName(ref *parsedPRRef) string {
	if ref == nil {
		return ""
	}
	return fmt.Sprintf("pr-%d", ref.Number)
}

// worktreeIncludeFile reads `.worktreeinclude` from repoRoot and returns
// the list of paths (one per non-comment, non-blank line). WT-03.
func worktreeIncludeFile(repoRoot string) []string {
	data, err := os.ReadFile(filepath.Join(repoRoot, ".worktreeinclude"))
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}
