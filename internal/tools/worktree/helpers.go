package worktree

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/gitutil"
)

// worktree_helpers.go — WT-02 / WT-03 / WT-04 support utilities.
//
//   * applySparseCheckout configures sparse-checkout in a freshly-created
//     worktree, rolling back on any failure (WT-02).
//   * applyWorktreeIncludes copies / symlinks paths listed in
//     .worktreeinclude into the new worktree (WT-03).
//   * applyWorktreeSettingsAndHusky copies .luban-code/settings.local.json and
//     rewrites husky core.hooksPath (WT-04).

// worktreeSparseCheckoutPatterns returns the sparse-checkout patterns to
// apply to new worktrees. Sourced from the WORKTREE_SPARSE_CHECKOUT env
// var (comma-separated). Returns nil when not configured.
func worktreeSparseCheckoutPatterns() []string {
	raw := strings.TrimSpace(os.Getenv("WORKTREE_SPARSE_CHECKOUT"))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// applySparseCheckout runs `git sparse-checkout init/set` in the worktree.
// On any failure the caller is responsible for rolling back the worktree.
func applySparseCheckout(worktreePath string, patterns []string) error {
	for _, pattern := range patterns {
		pattern = filepath.Clean(strings.TrimSpace(pattern))
		if pattern == "" || pattern == "." || filepath.IsAbs(pattern) || pattern == ".." || strings.HasPrefix(pattern, ".."+string(filepath.Separator)) {
			return i18n.NewError(i18n.KeyToolWorktreeSparsePathInvalid, pattern)
		}
	}
	args := append([]string{"sparse-checkout", "set", "--cone", "--"}, patterns...)
	if out, err := gitutil.Run(worktreePath, args...); err != nil {
		return i18n.NewError(i18n.KeyToolWorktreeSparseConfigureFailed, strings.TrimSpace(out))
	}
	if out, err := gitutil.Run(worktreePath, "checkout", "HEAD"); err != nil {
		return i18n.NewError(i18n.KeyToolWorktreeSparseCheckoutFailed, strings.TrimSpace(out))
	}
	return nil
}

// applyWorktreeIncludes copies files and symlinks (or junction-copies on
// Windows) directories listed in `.worktreeinclude` into the new worktree.
// Failures are logged-and-swallowed so a single missing path doesn't block
// the whole worktree creation.
func applyWorktreeIncludes(repoRoot, worktreePath string, includes []string) {
	repoRoot = cleanWorktreePath(repoRoot)
	worktreePath = cleanWorktreePath(worktreePath)
	for _, rel := range includes {
		rel = filepath.Clean(strings.TrimSpace(strings.TrimPrefix(rel, "./")))
		if rel == "" || rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		src := filepath.Join(repoRoot, rel)
		dst := filepath.Join(worktreePath, rel)
		if !worktreePathWithin(repoRoot, src) || !worktreePathWithin(worktreePath, dst) {
			continue
		}
		info, err := os.Lstat(src)
		if err != nil {
			continue
		}
		resolvedSrc, err := filepath.EvalSymlinks(src)
		if err != nil || !worktreePathWithin(repoRoot, resolvedSrc) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			continue
		}
		resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(dst))
		if err != nil || !worktreePathWithin(worktreePath, resolvedParent) {
			continue
		}
		if existing, err := os.Lstat(dst); err == nil && existing.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if info.IsDir() {
			// Try a symlink first (cheap), fall back to copying.
			if err := os.Symlink(src, dst); err == nil {
				continue
			}
			_ = copyDir(src, dst)
			continue
		}
		_ = copyFile(src, dst, info.Mode())
	}
}

func worktreePathWithin(root, candidate string) bool {
	root = cleanWorktreePath(root)
	candidate = cleanWorktreePath(candidate)
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

// applyWorktreeSettingsAndHusky implements WT-04. It copies
// .luban-code/settings.local.json (when present) and rewrites husky's
// core.hooksPath in the new worktree to point at the original repo's
// hooks directory so pre-commit / pre-push hooks fire normally.
func applyWorktreeSettingsAndHusky(repoRoot, worktreePath string) {
	settingsSrc := filepath.Join(repoRoot, brand.ConfigDirName, "settings.local.json")
	if info, err := os.Stat(settingsSrc); err == nil && !info.IsDir() {
		settingsDst := filepath.Join(worktreePath, brand.ConfigDirName, "settings.local.json")
		if mkdirErr := os.MkdirAll(filepath.Dir(settingsDst), 0o755); mkdirErr == nil {
			_ = copyFile(settingsSrc, settingsDst, info.Mode())
		}
	}
	// Rewrite husky core.hooksPath to point at the original repo's
	// hooks directory. We only set this if the source repo has a
	// .husky directory — otherwise the default git hooks path is fine.
	huskyPath := filepath.Join(repoRoot, ".husky")
	if info, err := os.Stat(huskyPath); err == nil && info.IsDir() {
		_, _ = gitutil.Run(worktreePath, "config", "core.hooksPath", huskyPath)
	}
}
