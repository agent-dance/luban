package permissions_test

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/types"
)

// stubPrompt returns a fixed decision without any I/O.
func stubPrompt(d permissions.Decision) func(string, map[string]any) permissions.Decision {
	return func(_ string, _ map[string]any) permissions.Decision { return d }
}

func installNoopSafetyChecks(t *testing.T) {
	t.Helper()
	permissions.SetSafetyConfig(permissions.SafetyConfig{
		ShellPolicyAnalyzer: func(string, types.PolicyContext) types.PolicyDecision {
			return types.PolicyDecision{Disposition: types.PolicyAllow}
		},
	})
	t.Cleanup(func() { permissions.SetSafetyConfig(permissions.SafetyConfig{}) })
}

func TestCLIPermissionHandler_Allow(t *testing.T) {
	installNoopSafetyChecks(t)
	checker := permissions.NewChecker(permissions.ModeAskAlways, nil)
	setStructuredPromptDecision(checker, stubPrompt(permissions.DecisionAllow))

	h := permissions.NewCLIPermissionHandler(checker)
	dec, err := h.Check(context.Background(), permission.PermissionRequest{ToolName: "Bash", Input: map[string]any{"command": "echo hi"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec != permission.PermissionAllow {
		t.Fatalf("expected PermissionAllow, got %v", dec)
	}
}

func TestCLIPermissionHandler_Deny(t *testing.T) {
	installNoopSafetyChecks(t)
	checker := permissions.NewChecker(permissions.ModeAskAlways, nil)
	setStructuredPromptDecision(checker, stubPrompt(permissions.DecisionDeny))

	h := permissions.NewCLIPermissionHandler(checker)
	dec, err := h.Check(context.Background(), permission.PermissionRequest{ToolName: "Bash", Input: map[string]any{"command": "rm -rf /"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec != permission.PermissionDeny {
		t.Fatalf("expected PermissionDeny, got %v", dec)
	}
}

func TestCLIPermissionHandler_AllowOnce(t *testing.T) {
	installNoopSafetyChecks(t)
	checker := permissions.NewChecker(permissions.ModeAskAlways, nil)
	setStructuredPromptDecision(checker, stubPrompt(permissions.DecisionAllowOnce))

	h := permissions.NewCLIPermissionHandler(checker)
	dec, err := h.Check(context.Background(), permission.PermissionRequest{ToolName: "Write", Input: map[string]any{"file_path": "/tmp/test.txt"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec != permission.PermissionAllowOnce {
		t.Fatalf("expected PermissionAllowOnce, got %v", dec)
	}
}

func TestCLIPermissionHandler_AllowAll(t *testing.T) {
	installNoopSafetyChecks(t)
	// ModeAllowAll checker → every call allowed, no prompt needed.
	checker := permissions.NewChecker(permissions.ModeAllowAll, nil)
	h := permissions.NewCLIPermissionHandler(checker)

	dec, err := h.Check(context.Background(), permission.PermissionRequest{ToolName: "Bash", Input: map[string]any{"command": "ls"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec != permission.PermissionAllow {
		t.Fatalf("expected PermissionAllow, got %v", dec)
	}
}

func TestCLIPermissionHandler_NoPromptFunc_Deny(t *testing.T) {
	installNoopSafetyChecks(t)
	// ModeAskAlways with no promptFunc → deny for safety.
	checker := permissions.NewChecker(permissions.ModeAskAlways, nil)
	h := permissions.NewCLIPermissionHandler(checker)

	dec, err := h.Check(context.Background(), permission.PermissionRequest{ToolName: "Bash", Input: map[string]any{"command": "mkdir build"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec != permission.PermissionDeny {
		t.Fatalf("expected PermissionDeny when no promptFunc, got %v", dec)
	}
}

func TestCLIPermissionHandler_AvoidPromptsDeniesPromptedRequest(t *testing.T) {
	installNoopSafetyChecks(t)
	prompted := false
	checker := permissions.NewChecker(permissions.ModeAskAlways, nil)
	setStructuredPromptDecision(checker, func(string, map[string]any) permissions.Decision {
		prompted = true
		return permissions.DecisionAllow
	})
	h := permissions.NewCLIPermissionHandler(checker)

	dec, err := h.Check(context.Background(), permission.PermissionRequest{
		ToolName:     "Write",
		Input:        map[string]any{"file_path": "/tmp/test.txt"},
		Mode:         "dontAsk",
		AvoidPrompts: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec != permission.PermissionDeny {
		t.Fatalf("expected PermissionDeny when prompts are avoided, got %v", dec)
	}
	if prompted {
		t.Fatal("expected prompt function not to be called")
	}
}

func TestCLIPermissionHandler_AcceptEditsModeOverridesAskAlways(t *testing.T) {
	installNoopSafetyChecks(t)
	checker := permissions.NewChecker(permissions.ModeAskAlways, nil)
	setStructuredPromptDecision(checker, stubPrompt(permissions.DecisionDeny))
	h := permissions.NewCLIPermissionHandler(checker)

	dec, err := h.Check(context.Background(), permission.PermissionRequest{
		ToolName: "Write",
		Input:    map[string]any{"file_path": "/tmp/test.txt"},
		Mode:     "acceptEdits",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec != permission.PermissionAllow {
		t.Fatalf("expected PermissionAllow from acceptEdits override, got %v", dec)
	}
}

func TestCLIPermissionHandler_ExplicitDefaultOverridesForegroundAllowAll(t *testing.T) {
	installNoopSafetyChecks(t)
	prompted := false
	checker := permissions.NewChecker(permissions.ModeAllowAll, nil)
	setStructuredPromptDecision(checker, func(string, map[string]any) permissions.Decision {
		prompted = true
		return permissions.DecisionDeny
	})
	h := permissions.NewCLIPermissionHandler(checker)

	dec, err := h.Check(context.Background(), permission.PermissionRequest{
		ToolName: "Read",
		Input:    map[string]any{"file_path": "go.mod"},
		Mode:     "default",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec != permission.PermissionDeny {
		t.Fatalf("expected explicit default request to use rule-based policy, got %v", dec)
	}
	if !prompted {
		t.Fatal("expected rule-based default request to use its configured prompt")
	}
}

func TestCLIPermissionHandler_RestrictiveChildModeOverridesForegroundAllowAll(t *testing.T) {
	installNoopSafetyChecks(t)
	prompted := false
	checker := permissions.NewChecker(permissions.ModeAllowAll, nil)
	setStructuredPromptDecision(checker, func(string, map[string]any) permissions.Decision {
		prompted = true
		return permissions.DecisionDeny
	})
	h := permissions.NewCLIPermissionHandler(checker)

	dec, err := h.Check(context.Background(), permission.PermissionRequest{
		ToolName:     "Read",
		Input:        map[string]any{"file_path": "go.mod"},
		Mode:         "dontAsk",
		AvoidPrompts: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec != permission.PermissionDeny {
		t.Fatalf("expected pinned child dontAsk to override foreground ModeAllowAll, got %v", dec)
	}
	if prompted {
		t.Fatal("expected child dontAsk not to prompt")
	}
}
