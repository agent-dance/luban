package shell

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestBashWritePermissionSuggestsPersistentRule(t *testing.T) {
	root := t.TempDir()
	tool := &BashTool{CWD: root}
	request := types.ToolPermissionRequest{Runtime: types.ToolRuntimeContext{
		ProjectRoot: root,
		AllowedDirs: []string{root},
	}}

	decision, err := tool.CheckPermissions(context.Background(), map[string]any{"command": "mkdir build"}, request)
	if err != nil || decision.Behavior != types.PermissionBehaviorPassthrough || len(decision.Suggestions) == 0 {
		t.Fatalf("write Bash decision = %+v, err=%v", decision, err)
	}
}
