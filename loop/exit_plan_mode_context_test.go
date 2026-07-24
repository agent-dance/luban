package loop

import (
	"testing"

	"github.com/agent-dance/luban/types"
)

// TS ref: src/components/permissions/ExitPlanModePermissionRequest/
// ExitPlanModePermissionRequest.tsx:374-386 and src/screens/REPL.tsx:3067-3068.
func TestExitPlanModeClearContextRestartsFromApprovedPlan(t *testing.T) {
	message, restart := planModeContextRestart([]types.ToolResultBlock{{
		Content:  "User has approved your plan.\n\n## Approved Plan:\nDo the work.",
		Metadata: map[string]string{"clearContext": "true", "restartExecution": "true"},
	}})
	if !restart || message == "" {
		t.Fatalf("clear-context result did not request restart: restart=%v message=%q", restart, message)
	}
}

func TestExitPlanModeRejectedResultDoesNotClearContext(t *testing.T) {
	_, restart := planModeContextRestart([]types.ToolResultBlock{{
		Content: "rejected", IsError: true,
		Metadata: map[string]string{"clearContext": "true", "restartExecution": "true"},
	}})
	if restart {
		t.Fatal("rejected plan must preserve context")
	}
}
