package permissions

import "testing"

func TestResetSessionClearsCachedApprovalsWithoutChangingMode(t *testing.T) {
	checker := NewChecker(ModeAskAlways, nil)
	prompts := 0
	checker.SetPromptFunc(func(string, map[string]any) Decision {
		prompts++
		return DecisionAllow
	})
	input := map[string]any{"file_path": "out.txt"}

	if got := checker.Check("Write", input); got != DecisionAllow {
		t.Fatalf("first decision = %v, want allow", got)
	}
	if got := checker.Check("Write", input); got != DecisionAllow {
		t.Fatalf("cached decision = %v, want allow", got)
	}
	if prompts != 1 {
		t.Fatalf("prompts before reset = %d, want 1", prompts)
	}

	checker.ResetSession()
	if checker.Mode() != ModeAskAlways {
		t.Fatalf("mode changed during session reset: %v", checker.Mode())
	}
	if got := checker.Check("Write", input); got != DecisionAllow {
		t.Fatalf("post-reset decision = %v, want allow", got)
	}
	if prompts != 2 {
		t.Fatalf("cached approval leaked across session reset; prompts = %d, want 2", prompts)
	}
}
