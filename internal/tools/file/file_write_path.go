package file

import (
	"context"
	"os"
	"strings"

	"github.com/agent-dance/luban/types"
)

func (t *FileWriteTool) runtimeSnapshot() types.ToolRuntimeContext {
	if t != nil && t.Runtime != nil {
		return t.Runtime.ToolRuntimeContext()
	}
	return types.ToolRuntimeContext{}
}

func (t *FileWriteTool) writeBaseDir() string {
	if root := strings.TrimSpace(t.runtimeSnapshot().ProjectRoot); root != "" {
		return root
	}
	cwd, err := os.Getwd()
	if err != nil || strings.TrimSpace(cwd) == "" {
		return "."
	}
	return cwd
}

func (t *FileWriteTool) expandPath(raw string) (string, error) {
	return expandReadPath(raw, t.writeBaseDir())
}

// BackfillObservableInput expands file_path before hooks and permission rules
// see it, matching FileWriteTool.backfillObservableInput in TS.
func (t *FileWriteTool) BackfillObservableInput(input map[string]any) (map[string]any, error) {
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

func (t *FileWriteTool) NormalizeToolInput(_ context.Context, input map[string]any) (map[string]any, error) {
	return t.BackfillObservableInput(input)
}
