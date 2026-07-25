package permissions

import (
	"sync"
	"testing"
)

func TestRuleBasedFallthroughToAsk(t *testing.T) {
	installNoopSafetyChecks(t)
	rules := []Rule{
		{Tool: "Read", Decision: DecisionAllow},
	}
	c := NewChecker(ModeRuleBased, rules)
	// No promptFunc → fallthrough to askOrCache → DecisionDeny
	if d := checkDecision(c, "Bash", map[string]any{"command": "mkdir build"}); d != DecisionDeny {
		t.Errorf("expected Deny for unmatched tool, got %d", d)
	}
}

func TestDenyDecisionNotCached(t *testing.T) {
	installNoopSafetyChecks(t)
	callCount := 0
	c := NewChecker(ModeAskAlways, nil)
	setStructuredPromptDecision(c, func(toolName string, input map[string]any) Decision {
		callCount++
		return DecisionDeny
	})

	checkDecision(c, "Bash", map[string]any{"command": "mkdir build"})
	checkDecision(c, "Bash", map[string]any{"command": "mkdir build"})

	if callCount != 2 {
		t.Errorf("expected 2 calls (deny not cached), got %d", callCount)
	}
}

func TestCacheKeyFilePathExact(t *testing.T) {
	installNoopSafetyChecks(t)
	callCount := 0
	c := NewChecker(ModeAskAlways, nil)
	setStructuredPromptDecision(c, func(toolName string, input map[string]any) Decision {
		callCount++
		return DecisionAllow
	})

	// Different files in the same directory must each be prompted independently
	// (no directory-level caching — prevents permission bypass).
	checkDecision(c, "Write", map[string]any{"file_path": "/tmp/a.txt"})
	checkDecision(c, "Write", map[string]any{"file_path": "/tmp/b.txt"})

	if callCount != 2 {
		t.Errorf("expected 2 calls (each file prompted separately), got %d", callCount)
	}

	// Same file should be cached after first approval.
	checkDecision(c, "Write", map[string]any{"file_path": "/tmp/a.txt"})
	if callCount != 2 {
		t.Errorf("expected 2 calls (same file cached), got %d", callCount)
	}
}

func TestCacheKeyUnknownToolIncludesInput(t *testing.T) {
	installNoopSafetyChecks(t)
	callCount := 0
	c := NewChecker(ModeAskAlways, nil)
	setStructuredPromptDecision(c, func(toolName string, input map[string]any) Decision {
		callCount++
		return DecisionAllow
	})

	checkDecision(c, "CustomTool", map[string]any{"x": "1"})
	checkDecision(c, "CustomTool", map[string]any{"y": "2"})

	if callCount != 2 {
		t.Errorf("expected 2 calls (different inputs require separate approval), got %d", callCount)
	}

	checkDecision(c, "CustomTool", map[string]any{"x": "1"})
	if callCount != 2 {
		t.Errorf("expected 2 calls (identical input cached), got %d", callCount)
	}
}

func TestPermissionsConcurrentCheck(t *testing.T) {
	installNoopSafetyChecks(t)
	c := NewChecker(ModeAskAlways, nil)
	setStructuredPromptDecision(c, func(toolName string, input map[string]any) Decision {
		return DecisionAllow
	})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			checkDecision(c, "Bash", map[string]any{"command": "ls"})
		}()
	}
	wg.Wait()
	// If we get here without race detector errors, the test passes
}
