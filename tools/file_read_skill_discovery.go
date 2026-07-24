// Package tools — file_read_skill_discovery.go implements the conditional
// skill auto-discovery surface for FileReadTool. After a dedup miss we
// walk upward from the read path looking for sibling skill directories
// ("skills/", ".claude/skills/") or SKILL.md files, and feed any matches
// into the FileReadSkillActivator before file I/O (which the TS version implements as
// activateConditionalSkillsForFile).
//
// The walk is intentionally bounded so we never accidentally scan a parent
// of the user's home directory:
//   - max 5 levels above the file's directory
//   - stop at any common repo-root marker (.git, go.mod, package.json,
//     pyproject.toml, Cargo.toml)
//   - stop once the parent equals the filesystem root or the user's home dir
//
// Mirrors the relevant fragments of TS skillsManager.activateConditionalSkillsForFile
// + discoverSkillDirsForPaths().
package tools

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	// maxSkillDiscoveryDepth bounds how far up we walk before giving up.
	maxSkillDiscoveryDepth = 5
)

// repoRootMarkers terminate the upward walk once any are found in a directory.
var repoRootMarkers = []string{
	".git",
	"go.mod",
	"package.json",
	"pyproject.toml",
	"Cargo.toml",
	"deno.json",
	"deno.jsonc",
}

// skillDirCandidates are the relative subpaths under any visited directory
// that we treat as candidate skill roots when present.
var skillDirCandidates = []string{
	filepath.Join(".luban-code", "skills"),
	filepath.Join(".deepseek-code", "skills"),
	filepath.Join(".claude", "skills"),
	"skills",
}

// DiscoverSkillDirsForPaths walks upward from each input path looking for
// nearby skill directories. Returns absolute, deduplicated paths to the
// discovered skill directories (not the individual SKILL.md files).
//
// The function is best-effort: file system errors are swallowed, and any
// path that cannot be made absolute is skipped.
func DiscoverSkillDirsForPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string

	homeDir, _ := os.UserHomeDir()

	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		dir := filepath.Dir(filepath.Clean(abs))

		for depth := 0; depth <= maxSkillDiscoveryDepth; depth++ {
			// Check skill directory candidates under this directory.
			for _, candidate := range skillDirCandidates {
				cand := filepath.Join(dir, candidate)
				if info, err := os.Stat(cand); err == nil && info.IsDir() {
					key := filepath.Clean(cand)
					if _, dup := seen[key]; !dup {
						seen[key] = struct{}{}
						out = append(out, key)
					}
				}
			}

			// Also surface sibling SKILL.md files: their containing dir is
			// itself a (single-skill) skill source.
			skillFile := filepath.Join(dir, "SKILL.md")
			if info, err := os.Stat(skillFile); err == nil && !info.IsDir() {
				key := filepath.Clean(dir)
				if _, dup := seen[key]; !dup {
					seen[key] = struct{}{}
					out = append(out, key)
				}
			}

			// Stop walking when we hit a repo root marker, the home dir, or
			// the filesystem root.
			if hasRepoRootMarker(dir) {
				break
			}
			if homeDir != "" && filepath.Clean(dir) == filepath.Clean(homeDir) {
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return out
}

// hasRepoRootMarker reports whether dir contains any well-known
// repository-root marker file or directory.
func hasRepoRootMarker(dir string) bool {
	for _, marker := range repoRootMarkers {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

// dynamicSkillTriggerEntry describes a single trigger registered by a
// conditional skill. When a Read happens against any path under one of
// these directories (or matching a basename), the host activates the
// associated skill.
type dynamicSkillTriggerEntry struct {
	// Directory triggers fire on any read whose path is under Dir.
	Dir string
	// Basename triggers fire on any read whose path basename matches.
	Basename string
	// Skill is the qualified skill name to activate.
	Skill string
}

// dynamicSkillDirTriggers is the package-wide registry of skill
// triggers. Mirrors TS dynamicSkillDirTriggers Set + the activation hook
// invoked by activateConditionalSkillsForFile.
var (
	dynamicSkillDirTriggersMu sync.RWMutex
	dynamicSkillDirTriggers   []dynamicSkillTriggerEntry
)

// RegisterDynamicSkillDirTrigger adds a directory-based skill trigger.
// The provided dir is normalised to an absolute path; an empty dir is
// rejected (no-op). Skills can register multiple triggers for the same
// skill name.
func RegisterDynamicSkillDirTrigger(dir, skill string) {
	if dir == "" || skill == "" {
		return
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	dynamicSkillDirTriggersMu.Lock()
	dynamicSkillDirTriggers = append(dynamicSkillDirTriggers, dynamicSkillTriggerEntry{
		Dir: filepath.Clean(abs), Skill: skill,
	})
	dynamicSkillDirTriggersMu.Unlock()
}

// RegisterDynamicSkillBasenameTrigger adds a basename-based trigger.
// For example, registering ("Gemfile", "ruby") would activate the ruby
// skill on the first Read of any Gemfile.
func RegisterDynamicSkillBasenameTrigger(basename, skill string) {
	if basename == "" || skill == "" {
		return
	}
	dynamicSkillDirTriggersMu.Lock()
	dynamicSkillDirTriggers = append(dynamicSkillDirTriggers, dynamicSkillTriggerEntry{
		Basename: basename, Skill: skill,
	})
	dynamicSkillDirTriggersMu.Unlock()
}

// LookupDynamicSkillsForPath returns the set of skill names whose
// triggers match absPath. Mirrors TS activateConditionalSkillsForFile.
// An empty result is normal — most reads do not activate anything.
func LookupDynamicSkillsForPath(absPath string) []string {
	if absPath == "" {
		return nil
	}
	dynamicSkillDirTriggersMu.RLock()
	defer dynamicSkillDirTriggersMu.RUnlock()
	if len(dynamicSkillDirTriggers) == 0 {
		return nil
	}
	uniform := strings.ReplaceAll(filepath.Clean(absPath), "\\", "/")
	base := filepath.Base(absPath)
	seen := map[string]struct{}{}
	var out []string
	for _, t := range dynamicSkillDirTriggers {
		if t.Dir != "" {
			dirUni := strings.ReplaceAll(t.Dir, "\\", "/")
			if strings.HasPrefix(uniform, dirUni+"/") || uniform == dirUni {
				if _, dup := seen[t.Skill]; !dup {
					seen[t.Skill] = struct{}{}
					out = append(out, t.Skill)
				}
			}
		}
		if t.Basename != "" && t.Basename == base {
			if _, dup := seen[t.Skill]; !dup {
				seen[t.Skill] = struct{}{}
				out = append(out, t.Skill)
			}
		}
	}
	return out
}

// ResetDynamicSkillTriggersForTest clears the registry. Test-only.
func ResetDynamicSkillTriggersForTest() {
	dynamicSkillDirTriggersMu.Lock()
	dynamicSkillDirTriggers = nil
	dynamicSkillDirTriggersMu.Unlock()
}

func fileReadSimpleMode() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CLAUDE_CODE_SIMPLE"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// discoverSkillsBeforeRead matches the TS timing: after a dedup miss and
// before the inner file operation. Discovery is best-effort and never turns a
// successful Read into an error.
func (t *FileReadTool) discoverSkillsBeforeRead(ctx context.Context, absPath string) {
	if t == nil || t.SkillManager == nil || fileReadSimpleMode() {
		return
	}
	dirs := DiscoverSkillDirsForPaths([]string{absPath})
	if len(dirs) > 0 {
		addSkillDirectoriesForExecution(ctx, t.SkillManager, dirs)
		t.listenerMu.Lock()
		if t.dynamicSkillDirs == nil {
			t.dynamicSkillDirs = make(map[string]struct{}, len(dirs))
		}
		for _, dir := range dirs {
			t.dynamicSkillDirs[dir] = struct{}{}
		}
		t.listenerMu.Unlock()
	}
	for _, name := range LookupDynamicSkillsForPath(absPath) {
		activateConditionalNameForExecution(ctx, t.SkillManager, name)
	}
	activateConditionalPathForExecution(ctx, t.SkillManager, absPath)
}

// DynamicSkillDirTriggers returns the discovered directories still available
// for attachment/listing consumers. It is a snapshot and safe for concurrent
// callers.
func (t *FileReadTool) DynamicSkillDirTriggers() []string {
	if t == nil {
		return nil
	}
	t.listenerMu.Lock()
	defer t.listenerMu.Unlock()
	dirs := make([]string, 0, len(t.dynamicSkillDirs))
	for dir := range t.dynamicSkillDirs {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	return dirs
}
