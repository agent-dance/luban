package permissions

import (
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestFeatureGates(t *testing.T) {
	checker := NewChecker(ModeAllowAll, nil)

	// Test initial state: all gates disabled
	if checker.IsFeatureEnabled("file_read") {
		t.Error("expected file_read to be disabled initially")
	}

	// Test SetFeatureGate
	checker.SetFeatureGate("file_read", true)
	if !checker.IsFeatureEnabled("file_read") {
		t.Error("expected file_read to be enabled after SetFeatureGate")
	}

	// Test SetFeatureGate disable
	checker.SetFeatureGate("file_read", false)
	if checker.IsFeatureEnabled("file_read") {
		t.Error("expected file_read to be disabled after SetFeatureGate(false)")
	}

	// Test SetFeatureGates bulk update
	gates := map[string]bool{
		"file_read":  true,
		"file_write": true,
		"shell_exec": false,
	}
	checker.SetFeatureGates(gates)

	if !checker.IsFeatureEnabled("file_read") {
		t.Error("expected file_read to be enabled")
	}
	if !checker.IsFeatureEnabled("file_write") {
		t.Error("expected file_write to be enabled")
	}
	if checker.IsFeatureEnabled("shell_exec") {
		t.Error("expected shell_exec to be disabled")
	}
}

func TestFeatureGatesThreadSafety(t *testing.T) {
	checker := NewChecker(ModeAllowAll, nil)

	// Spawn multiple goroutines to test thread safety
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(idx int) {
			for j := 0; j < 100; j++ {
				if idx%2 == 0 {
					checker.SetFeatureGate("test_gate", true)
				} else {
					_ = checker.IsFeatureEnabled("test_gate")
				}
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// If we get here without deadlock, thread safety test passes
	t.Log("thread safety test passed")
}

func TestPermissionCheckWithFeatureGates(t *testing.T) {
	rules := []Rule{
		{Tool: "FileRead", Decision: DecisionAllow},
		{Tool: "FileWrite", Decision: DecisionDeny},
	}

	checker := NewChecker(ModeRuleBased, rules)

	// Without feature gates, rules should work normally
	if checker.Check("FileRead", nil) != DecisionAllow {
		t.Error("expected FileRead to be allowed by rule")
	}

	if checker.Check("FileWrite", nil) != DecisionDeny {
		t.Error("expected FileWrite to be denied by rule")
	}

	// Feature gates can be used by tools to conditionally check permissions
	// They don't affect Check() directly but can be queried by tools
	checker.SetFeatureGate("file_read", false)
	checker.SetFeatureGate("file_write", true)

	if checker.IsFeatureEnabled("file_read") {
		t.Error("expected file_read feature gate to be disabled")
	}
	if !checker.IsFeatureEnabled("file_write") {
		t.Error("expected file_write feature gate to be enabled")
	}
}

func TestModeAllowAllWithFeatureGates(t *testing.T) {
	checker := NewChecker(ModeAllowAll, nil)
	checker.SetFeatureGate("anything", true)

	// ModeAllowAll should always allow regardless of feature gates
	if checker.Check("AnyTool", nil) != DecisionAllow {
		t.Error("expected ModeAllowAll to allow all tools")
	}
}

func TestModeAskAlwaysWithFeatureGates(t *testing.T) {
	checker := NewChecker(ModeAskAlways, nil)
	checker.SetFeatureGate("interactive", true)

	// Without a prompt function, should deny in AskAlways mode
	if checker.Check("SomeTool", nil) != DecisionDeny {
		t.Error("expected AskAlways mode without prompt to deny")
	}

	// With a prompt function, should ask
	checker.SetPromptFunc(func(toolName string, input map[string]any) Decision {
		return DecisionAllow
	})

	if checker.Check("SomeTool", nil) != DecisionAllow {
		t.Error("expected AskAlways mode with prompt to allow")
	}
}

func TestModeAskAlwaysAutoAllowsReadOnlyBash(t *testing.T) {
	installNoopSafetyChecks(t)
	checker := NewChecker(ModeAskAlways, nil)

	if got := checker.Check("Bash", map[string]any{"command": "git status --short"}); got != DecisionAllow {
		t.Fatalf("expected read-only bash to auto-allow, got %d", got)
	}
}

func TestModeAskAlwaysAutoAllowsReadOnlyPowerShell(t *testing.T) {
	installNoopSafetyChecks(t)
	checker := NewChecker(ModeAskAlways, nil)

	if got := checker.Check("PowerShell", map[string]any{"command": "Get-ChildItem | Select-String TODO"}); got != DecisionAllow {
		t.Fatalf("expected read-only PowerShell to auto-allow, got %d", got)
	}
}

func TestRuleBasedAskRulePrompts(t *testing.T) {
	installNoopSafetyChecks(t)
	checker := NewChecker(ModeRuleBased, []Rule{
		{Tool: "Bash", Pattern: "echo", Decision: DecisionAsk},
	})

	promptCount := 0
	checker.SetPromptFunc(func(toolName string, input map[string]any) Decision {
		promptCount++
		return DecisionAllowOnce
	})

	if got := checker.Check("Bash", map[string]any{"command": "echo hi"}); got != DecisionAllow {
		t.Fatalf("expected ask rule to prompt and allow, got %d", got)
	}
	if promptCount != 1 {
		t.Fatalf("expected prompt to be called once, got %d", promptCount)
	}
}

func TestAlwaysAllowCacheDoesNotCrossGenericToolTargets(t *testing.T) {
	installNoopSafetyChecks(t)
	checker := NewChecker(ModeAskAlways, nil)
	promptCount := 0
	checker.SetPromptFunc(func(string, map[string]any) Decision {
		promptCount++
		return DecisionAllow
	})
	first := map[string]any{"url": "https://example.com/a"}
	second := map[string]any{"url": "https://example.com/b"}
	if got := checker.Check("WebFetch", first); got != DecisionAllow {
		t.Fatalf("first WebFetch permission = %d, want allow", got)
	}
	if got := checker.Check("WebFetch", first); got != DecisionAllow {
		t.Fatalf("cached WebFetch permission = %d, want allow", got)
	}
	if got := checker.Check("WebFetch", second); got != DecisionAllow {
		t.Fatalf("second WebFetch permission = %d, want allow", got)
	}
	if promptCount != 2 {
		t.Fatalf("generic target cache prompted %d times, want once per distinct URL", promptCount)
	}
}

// ── Task 8: Frozen mode tests ──────────────────────────────────────────────

func TestSetModeFrozen(t *testing.T) {
	checker := NewChecker(ModeAskAlways, nil)
	checker.SetPromptFunc(func(toolName string, input map[string]any) Decision {
		return DecisionAllow
	})

	// Before any Check(), SetMode(ModeAllowAll) should succeed
	if err := checker.SetMode(ModeAllowAll); err != nil {
		t.Fatalf("expected SetMode(ModeAllowAll) to succeed before Check(), got: %v", err)
	}

	// Reset back to AskAlways so Check() doesn't just auto-allow
	if err := checker.SetMode(ModeAskAlways); err != nil {
		t.Fatalf("expected SetMode(ModeAskAlways) to succeed, got: %v", err)
	}

	// Trigger a Check() to freeze the session
	_ = checker.Check("SomeTool", nil)

	// After Check(), SetMode(ModeAllowAll) must be rejected
	err := checker.SetMode(ModeAllowAll)
	if err == nil {
		t.Fatal("expected SetMode(ModeAllowAll) to fail after Check(), but got nil")
	}
	if err.Error() != permissionText(i18n.KeyPermissionModeAllowAllFrozen) {
		t.Fatalf("unexpected error message: %v", err)
	}

	// Non-AllowAll modes should still be accepted after freeze
	if err := checker.SetMode(ModeRuleBased); err != nil {
		t.Fatalf("expected SetMode(ModeRuleBased) to succeed after freeze, got: %v", err)
	}
}

func TestSetModeFromUserCanEnterAllowAllAfterFreeze(t *testing.T) {
	checker := NewChecker(ModeAskAlways, nil)
	checker.SetPromptFunc(func(toolName string, input map[string]any) Decision {
		return DecisionAllow
	})

	_ = checker.Check("SomeTool", nil)

	if err := checker.SetModeFromUser(ModeAllowAll); err != nil {
		t.Fatalf("expected user-requested ModeAllowAll after freeze to succeed, got: %v", err)
	}
	if got := checker.Mode(); got != ModeAllowAll {
		t.Fatalf("mode = %v, want ModeAllowAll", got)
	}
}

func TestSetModeUnfrozen(t *testing.T) {
	checker := NewChecker(ModeRuleBased, nil)

	// Before any Check(), all mode changes should be allowed
	if err := checker.SetMode(ModeAllowAll); err != nil {
		t.Fatalf("expected SetMode(ModeAllowAll) to succeed before any Check(), got: %v", err)
	}
	if err := checker.SetMode(ModeAskAlways); err != nil {
		t.Fatalf("expected SetMode(ModeAskAlways) to succeed before any Check(), got: %v", err)
	}
	if err := checker.SetMode(ModeRuleBased); err != nil {
		t.Fatalf("expected SetMode(ModeRuleBased) to succeed before any Check(), got: %v", err)
	}
}

func TestSetModeFrozenThreadSafety(t *testing.T) {
	checker := NewChecker(ModeAskAlways, nil)
	checker.SetPromptFunc(func(toolName string, input map[string]any) Decision {
		return DecisionAllow
	})

	done := make(chan bool, 20)

	// Spawn goroutines that call Check() and SetMode concurrently
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 50; j++ {
				_ = checker.Check("Tool", nil)
			}
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 50; j++ {
				_ = checker.SetMode(ModeRuleBased)
			}
			done <- true
		}()
	}

	for i := 0; i < 20; i++ {
		<-done
	}
	// If we reach here without data race or deadlock, the test passes
}

// ── Task 4: Tool whitelist/blacklist tests ─────────────────────────────────

func TestDisallowedToolsDenyInAllowAll(t *testing.T) {
	checker := NewChecker(ModeAllowAll, nil)
	checker.SetDisallowedTools([]string{"Bash"})

	// Bash is disallowed — must be denied even in ModeAllowAll
	if d := checker.Check("Bash", nil); d != DecisionDeny {
		t.Errorf("expected Bash to be denied (disallowed list), got %d", d)
	}
	// Write is not disallowed — should be allowed in ModeAllowAll
	if d := checker.Check("Write", nil); d != DecisionAllow {
		t.Errorf("expected Write to be allowed, got %d", d)
	}
}

func TestAllowedToolsWhitelistInAllowAll(t *testing.T) {
	checker := NewChecker(ModeAllowAll, nil)
	checker.SetAllowedTools([]string{"Read", "Write"})

	// Bash is NOT in the whitelist — must be denied
	if d := checker.Check("Bash", nil); d != DecisionDeny {
		t.Errorf("expected Bash to be denied (not in allowed list), got %d", d)
	}
	// Read IS in the whitelist — should be allowed
	if d := checker.Check("Read", nil); d != DecisionAllow {
		t.Errorf("expected Read to be allowed, got %d", d)
	}
	// Write IS in the whitelist — should be allowed
	if d := checker.Check("Write", nil); d != DecisionAllow {
		t.Errorf("expected Write to be allowed, got %d", d)
	}
}

func TestNilListsAllowAll(t *testing.T) {
	checker := NewChecker(ModeAllowAll, nil)
	// Both lists are nil → all tools should be allowed (existing behavior unchanged)
	if d := checker.Check("Bash", nil); d != DecisionAllow {
		t.Errorf("expected Bash to be allowed (nil lists), got %d", d)
	}
	if d := checker.Check("Read", nil); d != DecisionAllow {
		t.Errorf("expected Read to be allowed (nil lists), got %d", d)
	}
	if d := checker.Check("Write", nil); d != DecisionAllow {
		t.Errorf("expected Write to be allowed (nil lists), got %d", d)
	}
	if d := checker.Check("WebFetch", nil); d != DecisionAllow {
		t.Errorf("expected WebFetch to be allowed (nil lists), got %d", d)
	}
}

func TestDisallowedToolsTakePrecedenceOverAllowed(t *testing.T) {
	checker := NewChecker(ModeAllowAll, nil)
	// Bash is in both lists — disallowed should take precedence (checked first)
	checker.SetAllowedTools([]string{"Bash", "Read"})
	checker.SetDisallowedTools([]string{"Bash"})

	if d := checker.Check("Bash", nil); d != DecisionDeny {
		t.Errorf("expected Bash to be denied (disallowed takes precedence), got %d", d)
	}
	if d := checker.Check("Read", nil); d != DecisionAllow {
		t.Errorf("expected Read to be allowed, got %d", d)
	}
}

func TestDisallowedToolsInAskAlwaysMode(t *testing.T) {
	checker := NewChecker(ModeAskAlways, nil)
	checker.SetPromptFunc(func(toolName string, input map[string]any) Decision {
		return DecisionAllow // prompt would allow
	})
	checker.SetDisallowedTools([]string{"Bash"})

	// Bash is disallowed — must be denied even though prompt would allow
	if d := checker.Check("Bash", nil); d != DecisionDeny {
		t.Errorf("expected Bash to be denied by disallowed list in AskAlways, got %d", d)
	}
	// Other tools go through normal prompt flow
	if d := checker.Check("Read", nil); d != DecisionAllow {
		t.Errorf("expected Read to be allowed via prompt, got %d", d)
	}
}

func TestAllowedToolsInAskAlwaysMode(t *testing.T) {
	checker := NewChecker(ModeAskAlways, nil)
	checker.SetPromptFunc(func(toolName string, input map[string]any) Decision {
		return DecisionAllow
	})
	checker.SetAllowedTools([]string{"Read", "Write"})

	// Bash is NOT in the whitelist — denied before prompt is consulted
	if d := checker.Check("Bash", nil); d != DecisionDeny {
		t.Errorf("expected Bash to be denied (not in allowed list), got %d", d)
	}
	// Read IS in the whitelist — goes through prompt
	if d := checker.Check("Read", nil); d != DecisionAllow {
		t.Errorf("expected Read to be allowed, got %d", d)
	}
}

func TestSetAllowedToolsNilClearsWhitelist(t *testing.T) {
	checker := NewChecker(ModeAllowAll, nil)
	checker.SetAllowedTools([]string{"Read"})

	// Bash should be denied while whitelist is active
	if d := checker.Check("Bash", nil); d != DecisionDeny {
		t.Errorf("expected Bash to be denied with whitelist, got %d", d)
	}

	// Clear the whitelist — all tools should be allowed again
	checker.SetAllowedTools(nil)
	if d := checker.Check("Bash", nil); d != DecisionAllow {
		t.Errorf("expected Bash to be allowed after clearing whitelist, got %d", d)
	}
}

func TestSetDisallowedToolsNilClearsBlacklist(t *testing.T) {
	checker := NewChecker(ModeAllowAll, nil)
	checker.SetDisallowedTools([]string{"Bash"})

	// Bash should be denied while blacklist is active
	if d := checker.Check("Bash", nil); d != DecisionDeny {
		t.Errorf("expected Bash to be denied with blacklist, got %d", d)
	}

	// Clear the blacklist — Bash should be allowed again
	checker.SetDisallowedTools(nil)
	if d := checker.Check("Bash", nil); d != DecisionAllow {
		t.Errorf("expected Bash to be allowed after clearing blacklist, got %d", d)
	}
}

// ── W4: cacheKey path differentiation for FileDelete, etc. ─────────────────

func TestCacheKeyDifferentiatesFilePaths(t *testing.T) {
	checker := NewChecker(ModeAskAlways, nil)
	promptCount := 0
	checker.SetPromptFunc(func(toolName string, input map[string]any) Decision {
		promptCount++
		return DecisionAllow
	})

	// First FileDelete call for "foo.go"
	_ = checker.Check("FileDelete", map[string]any{"file_path": "foo.go"})
	if promptCount != 1 {
		t.Fatalf("expected 1 prompt call, got %d", promptCount)
	}

	// Second FileDelete call for "foo.go" should be cached (no new prompt)
	_ = checker.Check("FileDelete", map[string]any{"file_path": "foo.go"})
	if promptCount != 1 {
		t.Fatalf("expected still 1 prompt call (cached), got %d", promptCount)
	}

	// FileDelete call for "bar.go" should NOT be cached (new prompt)
	_ = checker.Check("FileDelete", map[string]any{"file_path": "bar.go"})
	if promptCount != 2 {
		t.Fatalf("expected 2 prompt calls (different path), got %d", promptCount)
	}
}

func TestCacheKeyDifferentiatesFileAppend(t *testing.T) {
	checker := NewChecker(ModeAskAlways, nil)
	promptCount := 0
	checker.SetPromptFunc(func(toolName string, input map[string]any) Decision {
		promptCount++
		return DecisionAllow
	})

	_ = checker.Check("FileAppend", map[string]any{"file_path": "log.txt"})
	_ = checker.Check("FileAppend", map[string]any{"file_path": "other.txt"})
	if promptCount != 2 {
		t.Fatalf("expected 2 prompt calls for different FileAppend paths, got %d", promptCount)
	}
}

func TestModeAllowAllSendMessageDoesNotPrompt(t *testing.T) {
	installNoopSafetyChecks(t)
	checker := NewChecker(ModeAllowAll, nil)
	promptCount := 0
	checker.SetPromptFunc(func(toolName string, input map[string]any) Decision {
		promptCount++
		return DecisionAllow
	})

	input := map[string]any{
		"to":      "worker-1",
		"message": "hello teammate",
	}
	if got := checker.Check("SendMessage", input); got != DecisionAllow {
		t.Fatalf("expected SendMessage to be auto-allowed in allow-all mode, got %d", got)
	}
	if got := checker.Check("SendMessage", input); got != DecisionAllow {
		t.Fatalf("expected second SendMessage to be auto-allowed in allow-all mode, got %d", got)
	}
	if promptCount != 0 {
		t.Fatalf("expected allow-all mode to bypass prompting, got %d prompts", promptCount)
	}
}

func TestCacheKeyDifferentiatesSendMessageTargets(t *testing.T) {
	installNoopSafetyChecks(t)
	checker := NewChecker(ModeAskAlways, nil)
	promptCount := 0
	checker.SetPromptFunc(func(toolName string, input map[string]any) Decision {
		promptCount++
		return DecisionAllow
	})

	_ = checker.Check("SendMessage", map[string]any{
		"to":      "worker-1",
		"message": "hello one",
		"summary": "Say hello",
	})
	_ = checker.Check("SendMessage", map[string]any{
		"to":      "worker-1",
		"message": "hello one",
		"summary": "Say hello",
	})
	_ = checker.Check("SendMessage", map[string]any{
		"to":      "worker-2",
		"message": "hello one",
		"summary": "Say hello",
	})
	if promptCount != 2 {
		t.Fatalf("expected SendMessage cache key to differentiate targets, got %d prompt calls", promptCount)
	}
}
