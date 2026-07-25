package file

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/types"
	"golang.org/x/text/unicode/norm"
)

func (t *FileReadTool) runtimeSnapshot() types.ToolRuntimeContext {
	if t != nil && t.Runtime != nil {
		return t.Runtime.ToolRuntimeContext()
	}
	return types.ToolRuntimeContext{}
}

func (t *FileReadTool) readBaseDir() string {
	if root := strings.TrimSpace(t.runtimeSnapshot().ProjectRoot); root != "" {
		return root
	}
	cwd, err := os.Getwd()
	if err != nil || strings.TrimSpace(cwd) == "" {
		return "."
	}
	return cwd
}

func (t *FileReadTool) readAllowedDirs() []string {
	runtimeDirs := t.runtimeSnapshot().AllowedDirs
	if runtimeDirs != nil {
		return append([]string(nil), runtimeDirs...)
	}
	if t == nil {
		return nil
	}
	return append([]string(nil), t.AllowedDirs...)
}

// expandReadPath mirrors TS expandPath for Read: trim, empty-as-cwd, home
// expansion, relative-to-session-cwd resolution, native normalization, and NFC.
func expandReadPath(raw, baseDir string) (string, error) {
	if strings.ContainsRune(raw, '\x00') || strings.ContainsRune(baseDir, '\x00') {
		return "", i18n.NewError(i18n.KeyToolSourceSinkReadPathNullBytes)
	}
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		var err error
		baseDir, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return norm.NFC.String(filepath.Clean(baseDir)), nil
	}
	home, _ := os.UserHomeDir()
	switch {
	case trimmed == "~" && home != "":
		return norm.NFC.String(filepath.Clean(home)), nil
	case strings.HasPrefix(trimmed, "~/") && home != "":
		return norm.NFC.String(filepath.Join(home, filepath.FromSlash(trimmed[2:]))), nil
	}
	if runtime.GOOS == "windows" && len(trimmed) >= 4 && trimmed[0] == '/' && trimmed[2] == '/' && isASCIILetter(trimmed[1]) {
		trimmed = strings.ToUpper(trimmed[1:2]) + ":\\" + strings.ReplaceAll(trimmed[3:], "/", "\\")
	}
	if !filepath.IsAbs(trimmed) {
		trimmed = filepath.Join(baseDir, trimmed)
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", err
	}
	return norm.NFC.String(filepath.Clean(abs)), nil
}

func isASCIILetter(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func (t *FileReadTool) expandPath(raw string) (string, error) {
	return expandReadPath(raw, t.readBaseDir())
}

// BackfillObservableInput returns a defensive copy with file_path expanded so
// permission rules, hooks after permission preparation, and execution share a
// single unambiguous path.
func (t *FileReadTool) BackfillObservableInput(input map[string]any) (map[string]any, error) {
	updated := cloneToolInput(input)
	raw, ok := updated["file_path"].(string)
	if !ok {
		return updated, nil
	}
	expanded, err := t.expandPath(raw)
	if err != nil {
		return nil, err
	}
	updated["file_path"] = expanded
	return updated, nil
}

// NormalizeToolInput connects Read's observable path expansion to the main
// loop before PreToolUse hooks and permission checks. Direct registry dispatch
// uses BackfillObservableInput; both paths therefore observe the same path.
func (t *FileReadTool) NormalizeToolInput(_ context.Context, input map[string]any) (map[string]any, error) {
	return t.BackfillObservableInput(input)
}

func matchingReadPathRule(filePath string, rules []types.PermissionRuleValue) (types.PermissionRuleValue, bool) {
	for _, rule := range rules {
		if !strings.EqualFold(strings.TrimSpace(rule.ToolName), "Read") {
			continue
		}
		pattern := strings.TrimSpace(rule.RuleContent)
		if pattern == "" {
			continue
		}
		if pattern == filePath {
			return rule, true
		}
		if matched, err := toolbase.MatchGlob(filepath.ToSlash(pattern), filepath.ToSlash(filePath)); err == nil && matched {
			return rule, true
		}
	}
	return types.PermissionRuleValue{}, false
}
