package web

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestWebSearchPermissionSuggestsLocalSettingsRule(t *testing.T) {
	decision, err := NewWebSearchTool().CheckPermissions(
		context.Background(),
		map[string]any{"query": "golang"},
		types.ToolPermissionRequest{},
	)
	if err != nil || decision.Behavior != types.PermissionBehaviorPassthrough {
		t.Fatalf("decision = %+v, err=%v", decision, err)
	}
	if len(decision.Suggestions) != 1 || decision.Suggestions[0].Destination != types.PermissionDestinationLocalSettings {
		t.Fatalf("missing local-settings suggestion: %+v", decision.Suggestions)
	}
}
