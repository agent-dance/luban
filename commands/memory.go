package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
)

// ---------------------------------------------------------------------------
// /memory
// ---------------------------------------------------------------------------

// memoryCmd opens the nearest LUBAN Code instruction file, creating one in
// the current project when no project or legacy file exists.
type memoryCmd struct{}

func (c *memoryCmd) Name() string        { return "memory" }
func (c *memoryCmd) Aliases() []string   { return []string{"mem"} }
func (c *memoryCmd) Description() string { return builtinCommandDescription("memory") }

func (c *memoryCmd) Execute(ctx *Context, _ string) error {
	path, created, err := resolveInstructionsFileInLanguage(ctx.Language, ctx.CWD)
	if err != nil {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyCommandMemoryResolveError, err))
		return nil
	}

	if created {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyCommandMemoryCreated, path))
	} else {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyCommandMemoryOpening, path))
	}

	editor, source, err := resolveEditor(ctx.Language)
	if err != nil {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyCommandMemoryEditorError, err))
		return nil
	}

	var editorErr error
	if ctx.OpenFileEditor != nil {
		editorErr = ctx.OpenFileEditor(path)
	} else {
		cmd := exec.Command(editor, path)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		editorErr = cmd.Run()
	}
	if editorErr != nil {
		ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyCommandMemoryEditorExited, editorErr))
		return nil
	}

	var hint string
	if source != "" {
		hint = i18n.Format(ctx.Language, i18n.KeyCommandMemoryEditorHintSource, source)
	} else {
		hint = i18n.Text(ctx.Language, i18n.KeyCommandMemoryEditorHint)
	}

	rel := relativePath(ctx.CWD, path)
	ctx.OnEvent(i18n.Format(ctx.Language, i18n.KeyCommandMemoryOpened, rel, hint))
	return nil
}

// resolveInstructionsFile walks up from cwd looking for LUBAN Code instructions,
// then falls back through DeepSeek Code, AGENTS.md, and Claude instructions.
// If none exists it creates cwd/LUBAN.md.
// Returns the resolved path, whether it was just created, and any error.
func resolveInstructionsFile(cwd string) (path string, created bool, err error) {
	return resolveInstructionsFileInLanguage(i18n.DetectOrLoadLanguage(), cwd)
}

func resolveInstructionsFileInLanguage(lang i18n.Language, cwd string) (path string, created bool, err error) {
	// Walk up from cwd.
	dir := cwd
	for {
		current := filepath.Join(dir, brand.InstructionsFile)
		if _, statErr := os.Stat(current); statErr == nil {
			return current, false, nil
		}
		legacyDeepSeek := filepath.Join(dir, brand.LegacyDeepSeekInstructionsFile)
		if _, statErr := os.Stat(legacyDeepSeek); statErr == nil {
			migrated, migrateErr := migrateInstructionsFile(current, legacyDeepSeek)
			return current, migrated, migrateErr
		}
		for _, name := range []string{brand.AgentsFile, brand.LegacyInstructionsFile} {
			candidate := filepath.Join(dir, name)
			if _, statErr := os.Stat(candidate); statErr == nil {
				return candidate, false, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	if home, homeErr := os.UserHomeDir(); homeErr == nil {
		current := filepath.Join(home, brand.ConfigDirName, brand.InstructionsFile)
		globalCandidates := []string{
			current,
			filepath.Join(home, brand.ConfigDirName, brand.AgentsFile),
		}
		for _, candidate := range globalCandidates {
			if _, statErr := os.Stat(candidate); statErr == nil {
				return candidate, false, nil
			}
		}
		legacyDeepSeek := filepath.Join(home, brand.LegacyDeepSeekConfigDirName, brand.LegacyDeepSeekInstructionsFile)
		if _, statErr := os.Stat(legacyDeepSeek); statErr == nil {
			migrated, migrateErr := migrateInstructionsFile(current, legacyDeepSeek)
			return current, migrated, migrateErr
		}
		for _, candidate := range []string{
			filepath.Join(home, brand.LegacyDeepSeekConfigDirName, brand.AgentsFile),
			filepath.Join(home, brand.LegacyConfigDirName, brand.LegacyInstructionsFile),
		} {
			if _, statErr := os.Stat(candidate); statErr == nil {
				return candidate, false, nil
			}
		}
	}

	// Nothing found — create cwd/LUBAN.md.
	target := filepath.Join(cwd, brand.InstructionsFile)
	f, createErr := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if createErr != nil {
		if os.IsExist(createErr) {
			// Race: another process created it between our Stat and OpenFile.
			return target, false, nil
		}
		return "", false, createErr
	}
	_, _ = f.WriteString(i18n.Text(lang, i18n.KeyRuntimeProjectInstructionsTemplate))
	f.Close()
	return target, true, nil
}

func migrateInstructionsFile(target, legacy string) (bool, error) {
	data, err := os.ReadFile(legacy)
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return false, err
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return false, nil
		}
		return false, err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(target)
		return false, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(target)
		return false, err
	}
	return true, nil
}

// resolveClaudeMD is retained for compatibility with older tests and callers.
func resolveClaudeMD(cwd string) (path string, created bool, err error) {
	return resolveInstructionsFile(cwd)
}

// resolveEditor returns the editor binary and the env-var source string used
// to pick it (e.g. "$VISUAL", "$EDITOR", or "" for a hard-coded fallback).
// Returns an error when no editor can be found.
func resolveEditor(lang i18n.Language) (bin, source string, err error) {
	if v := os.Getenv("VISUAL"); v != "" {
		return v, "$VISUAL", nil
	}
	if v := os.Getenv("EDITOR"); v != "" {
		return v, "$EDITOR", nil
	}
	// Hard-coded fallbacks: prefer vim, then nano.
	for _, fb := range []string{"vim", "nano", "vi"} {
		if p, lookErr := exec.LookPath(fb); lookErr == nil {
			return p, "", nil
		}
	}
	return "", "", fmt.Errorf("%s", i18n.Text(lang, i18n.KeyCommandMemoryNoEditor))
}

// relativePath returns path relative to base, falling back to the absolute
// path if the relative form cannot be computed.
func relativePath(base, path string) string {
	if base == "" {
		return path
	}
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
}
