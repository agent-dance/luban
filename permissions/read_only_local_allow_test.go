package permissions

import (
	"context"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/types"
)

func TestToolLocalReadOnlyAllowSkipsRuleBasedFallback(t *testing.T) {
	for _, toolName := range []string{"Read", "Glob", "Grep"} {
		t.Run(toolName, func(t *testing.T) {
			checker := NewChecker(ModeRuleBased, nil)
			prompts := 0
			checker.SetStructuredPromptFunc(func(context.Context, PromptRequest) PromptResponse {
				prompts++
				return PromptResponse{Decision: DecisionDeny, Outcome: PromptOutcomeRejected}
			})

			decision, err := NewCLIPermissionHandler(checker).Check(context.Background(), permission.PermissionRequest{
				ToolName: toolName,
				Input: map[string]any{
					"file_path": "/workspace/note.txt",
				},
				ToolLocalReadOnlyAllow: true,
			})
			if err != nil || decision != permission.PermissionAllow {
				t.Fatalf("local read-only allow decision=%v err=%v, want allow", decision, err)
			}
			if prompts != 0 {
				t.Fatalf("local read-only allow used %d prompts, want none", prompts)
			}
		})
	}
}

func TestToolLocalReadOnlyAllowDoesNotBypassAskAlways(t *testing.T) {
	checker := NewChecker(ModeAskAlways, nil)
	prompts := 0
	checker.SetStructuredPromptFunc(func(context.Context, PromptRequest) PromptResponse {
		prompts++
		return PromptResponse{Decision: DecisionDeny, Outcome: PromptOutcomeRejected}
	})

	decision, err := NewCLIPermissionHandler(checker).Check(context.Background(), permission.PermissionRequest{
		ToolName:               "Read",
		Input:                  map[string]any{"file_path": "/workspace/note.txt"},
		ToolLocalReadOnlyAllow: true,
	})
	if err != nil || decision != permission.PermissionDeny {
		t.Fatalf("ask-always local read-only decision=%v err=%v, want deny", decision, err)
	}
	if prompts != 1 {
		t.Fatalf("ask-always local read-only prompts=%d, want one", prompts)
	}
}

func TestToolLocalReadOnlyAllowDoesNotBypassExplicitRules(t *testing.T) {
	t.Run("deny", func(t *testing.T) {
		checker := NewChecker(ModeRuleBased, []Rule{{Tool: "Read", Decision: DecisionDeny}})
		prompts := 0
		checker.SetStructuredPromptFunc(func(context.Context, PromptRequest) PromptResponse {
			prompts++
			return PromptResponse{Decision: DecisionAllowOnce, Outcome: PromptOutcomeApproved, Choice: "allow_once"}
		})
		decision, err := NewCLIPermissionHandler(checker).Check(context.Background(), permission.PermissionRequest{
			ToolName: "Read", Input: map[string]any{"file_path": "/workspace/note.txt"}, ToolLocalReadOnlyAllow: true,
		})
		if err != nil || decision != permission.PermissionDeny {
			t.Fatalf("explicit deny decision=%v err=%v, want deny", decision, err)
		}
		if prompts != 0 {
			t.Fatalf("explicit deny used %d prompts, want none", prompts)
		}
	})

	t.Run("ask", func(t *testing.T) {
		checker := NewChecker(ModeRuleBased, []Rule{{Tool: "Read", Decision: DecisionAsk}})
		prompts := 0
		checker.SetStructuredPromptFunc(func(context.Context, PromptRequest) PromptResponse {
			prompts++
			return PromptResponse{Decision: DecisionAllowOnce, Outcome: PromptOutcomeApproved, Choice: "allow_once"}
		})
		decision, err := NewCLIPermissionHandler(checker).Check(context.Background(), permission.PermissionRequest{
			ToolName: "Read", Input: map[string]any{"file_path": "/workspace/note.txt"}, ToolLocalReadOnlyAllow: true,
		})
		if err != nil || decision != permission.PermissionAllowOnce {
			t.Fatalf("explicit ask decision=%v err=%v, want allow once", decision, err)
		}
		if prompts != 1 {
			t.Fatalf("explicit ask used %d prompts, want one", prompts)
		}
	})
}

func TestToolLocalReadOnlyAllowDoesNotBypassRequiredAsk(t *testing.T) {
	checker := NewChecker(ModeRuleBased, nil)
	prompts := 0
	checker.SetStructuredPromptFunc(func(context.Context, PromptRequest) PromptResponse {
		prompts++
		return PromptResponse{Decision: DecisionAllowOnce, Outcome: PromptOutcomeApproved, Choice: "allow_once"}
	})

	decision, err := NewCLIPermissionHandler(checker).Check(context.Background(), permission.PermissionRequest{
		ToolName: "Read", Input: map[string]any{"file_path": "/workspace/note.txt"},
		ToolLocalReadOnlyAllow: true, Required: true,
	})
	if err != nil || decision != permission.PermissionAllowOnce {
		t.Fatalf("required local read-only decision=%v err=%v, want allow once", decision, err)
	}
	if prompts != 1 {
		t.Fatalf("required local read-only prompts=%d, want one", prompts)
	}
}

func TestToolLocalReadOnlyAllowDoesNotBypassSnapshotRules(t *testing.T) {
	t.Run("deny", func(t *testing.T) {
		checker := NewChecker(ModeRuleBased, nil)
		prompts := 0
		checker.SetStructuredPromptFunc(func(context.Context, PromptRequest) PromptResponse {
			prompts++
			return PromptResponse{Decision: DecisionAllowOnce, Outcome: PromptOutcomeApproved, Choice: "allow_once"}
		})
		snapshot := types.ToolRuntimeContext{
			PermissionMode: "default",
			DeniedRules:    []types.PermissionRuleValue{{ToolName: "Read"}},
		}
		decision, err := NewCLIPermissionHandler(checker).Check(context.Background(), permission.PermissionRequest{
			ToolName: "Read", Input: map[string]any{"file_path": "/workspace/note.txt"},
			ToolLocalReadOnlyAllow: true, PermissionSnapshot: &snapshot,
		})
		if err != nil || decision != permission.PermissionDeny {
			t.Fatalf("snapshot deny decision=%v err=%v, want deny", decision, err)
		}
		if prompts != 0 {
			t.Fatalf("snapshot deny used %d prompts, want none", prompts)
		}
	})

	t.Run("ask", func(t *testing.T) {
		checker := NewChecker(ModeRuleBased, nil)
		prompts := 0
		checker.SetStructuredPromptFunc(func(context.Context, PromptRequest) PromptResponse {
			prompts++
			return PromptResponse{Decision: DecisionAllowOnce, Outcome: PromptOutcomeApproved, Choice: "allow_once"}
		})
		snapshot := types.ToolRuntimeContext{
			PermissionMode: "default",
			AskRules:       []types.PermissionRuleValue{{ToolName: "Read"}},
		}
		decision, err := NewCLIPermissionHandler(checker).Check(context.Background(), permission.PermissionRequest{
			ToolName: "Read", Input: map[string]any{"file_path": "/workspace/note.txt"},
			ToolLocalReadOnlyAllow: true, PermissionSnapshot: &snapshot,
		})
		if err != nil || decision != permission.PermissionAllowOnce {
			t.Fatalf("snapshot ask decision=%v err=%v, want allow once", decision, err)
		}
		if prompts != 1 {
			t.Fatalf("snapshot ask used %d prompts, want one", prompts)
		}
	})
}

func TestReadOnlyRequestWithoutToolLocalAllowStillUsesFallback(t *testing.T) {
	checker := NewChecker(ModeRuleBased, nil)
	prompts := 0
	checker.SetStructuredPromptFunc(func(context.Context, PromptRequest) PromptResponse {
		prompts++
		return PromptResponse{Decision: DecisionDeny, Outcome: PromptOutcomeRejected}
	})

	decision, err := NewCLIPermissionHandler(checker).Check(context.Background(), permission.PermissionRequest{
		ToolName: "Glob", Input: map[string]any{"pattern": "**/*", "path": "/outside"},
	})
	if err != nil || decision != permission.PermissionDeny {
		t.Fatalf("unproven read-only decision=%v err=%v, want deny", decision, err)
	}
	if prompts != 1 {
		t.Fatalf("unproven read-only prompts=%d, want one", prompts)
	}
}

func TestToolLocalReadOnlyAllowCannotAuthorizeWrite(t *testing.T) {
	checker := NewChecker(ModeRuleBased, nil)
	prompts := 0
	checker.SetStructuredPromptFunc(func(context.Context, PromptRequest) PromptResponse {
		prompts++
		return PromptResponse{Decision: DecisionDeny, Outcome: PromptOutcomeRejected}
	})

	decision, err := NewCLIPermissionHandler(checker).Check(context.Background(), permission.PermissionRequest{
		ToolName: "Write", Input: map[string]any{"file_path": "/workspace/note.txt"},
		ToolLocalReadOnlyAllow: true,
	})
	if err != nil || decision != permission.PermissionDeny {
		t.Fatalf("forged write read-only proof decision=%v err=%v, want deny", decision, err)
	}
	if prompts != 1 {
		t.Fatalf("forged write read-only proof prompts=%d, want one", prompts)
	}
}
