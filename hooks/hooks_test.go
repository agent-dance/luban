package hooks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHookTypeFromFilename(t *testing.T) {
	tests := []struct {
		filename string
		expected HookType
	}{
		{"pre-tool-use-check", HookPreToolUse},
		{"pretooluse-lint", HookPreToolUse},
		{"post-tool-use-log", HookPostToolUse},
		{"posttooluse-audit", HookPostToolUse},
		{"session-start-init", HookSessionStart},
		{"sessionstart-setup", HookSessionStart},
		{"session-end-cleanup", HookSessionEnd},
		{"sessionend-save", HookSessionEnd},
		{"user-prompt-filter", HookUserPromptSubmit},
		{"userprompt-validate", HookUserPromptSubmit},
		// T5: stop, pre-query, post-query, notification prefixes
		{"stop-on-finish", HookStop},
		{"stop", HookStop},
		{"pre-query-validate", HookPreQuery},
		{"prequery-check", HookPreQuery},
		{"post-query-log", HookPostQuery},
		{"postquery-audit", HookPostQuery},
		{"notification-alert", HookNotification},
		{"notification", HookNotification},
		{"pre-compact-prepare", HookPreCompact},
		{"precompact-prepare", HookPreCompact},
		{"post-compact-cleanup", HookPostCompact},
		{"postcompact-cleanup", HookPostCompact},
		{"subagent-start-reviewer", HookSubagentStart},
		{"subagentstart-reviewer", HookSubagentStart},
		{"subagent-stop-reviewer", HookSubagentStop},
		{"subagentstop-reviewer", HookSubagentStop},
		{"random-script", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := hookTypeFromFilename(tt.filename)
		if got != tt.expected {
			t.Errorf("hookTypeFromFilename(%q) = %q, want %q", tt.filename, got, tt.expected)
		}
	}
}

func TestLoadFromDirInfersType(t *testing.T) {
	dir := t.TempDir()

	// Create hook scripts with naming convention
	os.WriteFile(filepath.Join(dir, "pre-tool-use-check.sh"), []byte("#!/bin/bash\necho ok"), 0755)
	os.WriteFile(filepath.Join(dir, "post-tool-use-log.sh"), []byte("#!/bin/bash\necho ok"), 0755)
	os.WriteFile(filepath.Join(dir, "random.sh"), []byte("#!/bin/bash\necho skip"), 0755)

	runner, err := LoadFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Should have 2 hooks (random.sh skipped due to no matching type)
	if len(runner.hooks) != 2 {
		t.Errorf("expected 2 hooks, got %d", len(runner.hooks))
	}

	// Verify types were inferred
	foundPre, foundPost := false, false
	for _, h := range runner.hooks {
		if h.Type == HookPreToolUse {
			foundPre = true
		}
		if h.Type == HookPostToolUse {
			foundPost = true
		}
	}
	if !foundPre {
		t.Error("expected PreToolUse hook")
	}
	if !foundPost {
		t.Error("expected PostToolUse hook")
	}
}

func TestLoadFromDirNonexistent(t *testing.T) {
	runner, err := LoadFromDir("/nonexistent/path")
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.hooks) != 0 {
		t.Error("expected empty runner for nonexistent dir")
	}
}

func TestLoadFromSettingsNonexistent(t *testing.T) {
	runner, err := LoadFromSettings("/nonexistent/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.hooks) != 0 {
		t.Error("expected empty runner for nonexistent settings")
	}
}

func TestHookExecutionIdentityDistinguishesToolsWithinOneTurn(t *testing.T) {
	base := HookInput{SessionID: "session", TurnID: "session:query-q:turn-1", ToolName: "Bash"}
	first := base
	first.ToolUseID = "tool-1"
	second := base
	second.ToolUseID = "tool-2"
	firstID := hookExecutionID(HookPreToolUse, first, "config-1")
	secondID := hookExecutionID(HookPreToolUse, second, "config-1")
	if firstID == secondID || !strings.Contains(firstID, "tool-tool-1") || !strings.Contains(secondID, "tool-tool-2") {
		t.Fatalf("same-turn tool hook identities collided: first=%q second=%q", firstID, secondID)
	}
}

func TestRunDetailedSnapshotsInputAndHookConfiguration(t *testing.T) {
	headers := map[string]string{"X-Evidence": "original"}
	runner := NewRunner([]Hook{{
		Type:    HookPreToolUse,
		Command: "true",
		Timeout: 5,
		Headers: headers,
	}})
	// A runner owns its loaded configuration; callers may reuse or mutate the
	// source maps after construction without rewriting future evidence.
	headers["X-Evidence"] = "mutated-before-run"

	nestedInput := map[string]any{"value": "original"}
	nestedMessage := map[string]any{"text": "original"}
	input := HookInput{
		ToolInput: map[string]any{"nested": nestedInput},
		Messages:  []any{nestedMessage},
	}
	executions := runner.RunDetailed(context.Background(), HookPreToolUse, input)
	if len(executions) != 1 {
		t.Fatalf("executions = %d, want 1", len(executions))
	}

	nestedInput["value"] = "mutated-after-run"
	nestedMessage["text"] = "mutated-after-run"
	input.ToolInput["added"] = true

	gotInput := executions[0].Input.ToolInput["nested"].(map[string]any)
	gotMessage := executions[0].Input.Messages[0].(map[string]any)
	if gotInput["value"] != "original" || gotMessage["text"] != "original" {
		t.Fatalf("execution input was rewritten after capture: input=%v messages=%v", gotInput, gotMessage)
	}
	if _, ok := executions[0].Input.ToolInput["added"]; ok {
		t.Fatalf("execution input shares the caller's top-level map: %v", executions[0].Input.ToolInput)
	}
	if got := executions[0].Hook.Headers["X-Evidence"]; got != "original" {
		t.Fatalf("captured hook header = %q, want original", got)
	}
}

func TestRunDetailedRedactsSensitiveHookHeadersFromEvidence(t *testing.T) {
	runner := NewRunner([]Hook{{
		Type:    HookPreToolUse,
		Command: "true",
		Timeout: 5,
		Headers: map[string]string{
			"Authorization": "Bearer top-secret",
			"X-API-Key":     "api-secret",
			"X-Evidence":    "visible",
		},
	}})

	executions := runner.RunDetailed(context.Background(), HookPreToolUse, HookInput{})
	if len(executions) != 1 {
		t.Fatalf("executions = %d, want 1", len(executions))
	}
	got := executions[0].Hook.Headers
	if got["Authorization"] != "[REDACTED]" || got["X-API-Key"] != "[REDACTED]" {
		t.Fatalf("sensitive headers leaked into evidence: %#v", got)
	}
	if got["X-Evidence"] != "visible" {
		t.Fatalf("non-sensitive header = %q, want visible", got["X-Evidence"])
	}
}

func TestHookInputSnapshotClonesTypedMapsSlicesAndPointers(t *testing.T) {
	type payload struct {
		Label string
	}
	type payloadList []*payload
	type payloadMap map[string]payloadList

	originalPayload := &payload{Label: "original"}
	original := payloadMap{"items": {originalPayload}}
	input := HookInput{ToolInput: map[string]any{"typed": original}}

	snapshot := input.Snapshot()
	originalPayload.Label = "mutated"
	original["items"] = append(original["items"], &payload{Label: "added"})

	got, ok := snapshot.ToolInput["typed"].(payloadMap)
	if !ok {
		t.Fatalf("snapshot typed payload has type %T, want %T", snapshot.ToolInput["typed"], original)
	}
	if len(got["items"]) != 1 || got["items"][0] == originalPayload || got["items"][0].Label != "original" {
		t.Fatalf("snapshot shares typed map/slice/pointer storage: %#v", got)
	}
}

func TestCommandHookPreservesContextErrorWhenProcessHasNoStderr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	output := executeCommandHook(ctx, Hook{Command: "true", Timeout: 5}, HookInput{})
	if !strings.Contains(output.ExecutionError, context.Canceled.Error()) {
		t.Fatalf("execution error = %q, want context cancellation evidence", output.ExecutionError)
	}
	if !strings.Contains(output.Stderr, context.Canceled.Error()) {
		t.Fatalf("stderr = %q, want context cancellation evidence", output.Stderr)
	}
	if output.StderrBytes != int64(len(output.Stderr)) {
		t.Fatalf("stderr byte count = %d, want %d", output.StderrBytes, len(output.Stderr))
	}
}

func TestCommandHookReportsRawOutputAndTruncation(t *testing.T) {
	command := "head -c 1048593 /dev/zero | tr '\\0' x; head -c 1048607 /dev/zero | tr '\\0' y >&2"
	output := executeCommandHook(context.Background(), Hook{Command: command, Timeout: 5}, HookInput{})

	if !output.StdoutTruncated || !output.StderrTruncated {
		t.Fatalf("truncation flags = stdout %t stderr %t, want both true", output.StdoutTruncated, output.StderrTruncated)
	}
	if output.StdoutBytes != 1048593 || output.StderrBytes != 1048607 {
		t.Fatalf("raw byte counts = stdout %d stderr %d", output.StdoutBytes, output.StderrBytes)
	}
	if len(output.Stdout) != hookOutputLimit || len(output.Stderr) != hookOutputLimit {
		t.Fatalf("captured lengths = stdout %d stderr %d, want %d", len(output.Stdout), len(output.Stderr), hookOutputLimit)
	}
	if output.Stdout[0] != 'x' || output.Stderr[0] != 'y' {
		t.Fatalf("captured raw output prefixes = stdout %q stderr %q", output.Stdout[:1], output.Stderr[:1])
	}
}

func TestLoadFromSettingsValid(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	os.WriteFile(settingsPath, []byte(`{"hooks":[{"type":"PreToolUse","command":"echo hi","timeout":5}]}`), 0644)

	runner, err := LoadFromSettings(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.hooks) != 1 {
		t.Errorf("expected 1 hook, got %d", len(runner.hooks))
	}
	if runner.hooks[0].Type != HookPreToolUse {
		t.Errorf("expected PreToolUse, got %s", runner.hooks[0].Type)
	}
}

func TestLoadFromSettingsRichEventMap(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	err := os.WriteFile(settingsPath, []byte(`{
		"hooks": {
			"PreToolUse": [
				{
					"matcher": "Bash",
					"hooks": [
						{"type": "command", "command": "echo hi", "timeout": 5}
					]
				}
			],
			"Notification": [
				{
					"hooks": [
						{"type": "notification"}
					]
				}
			]
		}
	}`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	runner, err := LoadFromSettings(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.hooks) != 2 {
		t.Fatalf("expected 2 hooks, got %d", len(runner.hooks))
	}
	if !runner.HasHooks(HookPreToolUse) {
		t.Fatal("expected PreToolUse hook")
	}
	if !runner.HasHooks(HookNotification) {
		t.Fatal("expected Notification hook")
	}
	for _, hook := range runner.hooks {
		if hook.Type == HookPreToolUse {
			if hook.Matcher != "Bash" || hook.Command != "echo hi" || hook.Timeout != 5 {
				t.Fatalf("unexpected PreToolUse hook: %+v", hook)
			}
		}
	}
}

func TestHasHooks(t *testing.T) {
	runner := NewRunner([]Hook{
		{Type: HookPreToolUse, Command: "echo"},
	})

	if !runner.HasHooks(HookPreToolUse) {
		t.Error("expected HasHooks=true for PreToolUse")
	}
	if runner.HasHooks(HookPostToolUse) {
		t.Error("expected HasHooks=false for PostToolUse")
	}
}

func TestRunSubagentMatcherUsesAgentType(t *testing.T) {
	runner := NewRunner([]Hook{
		{
			Type:    HookSubagentStart,
			Matcher: "reviewer",
			Command: "echo 'reviewer-started'",
			Timeout: 5,
		},
	})

	missing := runner.Run(context.Background(), HookSubagentStart, HookInput{AgentType: "planner"})
	if len(missing) != 0 {
		t.Fatalf("expected matcher to skip planner agent, got %d outputs", len(missing))
	}
	matched := runner.Run(context.Background(), HookSubagentStart, HookInput{AgentType: "reviewer"})
	if len(matched) != 1 {
		t.Fatalf("expected reviewer hook to run once, got %d outputs", len(matched))
	}
	if !strings.Contains(matched[0].SystemReminder, "reviewer-started") {
		t.Fatalf("expected hook output to become reminder, got %#v", matched[0])
	}
}

func TestRunnerWithHookTypeMapped(t *testing.T) {
	runner := NewRunner([]Hook{
		{Type: HookStop, Command: "echo stop"},
		{Type: HookPreToolUse, Command: "echo pre"},
	})
	mapped := runner.WithHookTypeMapped(HookStop, HookSubagentStop)
	if runner.HasHooks(HookSubagentStop) {
		t.Fatal("original runner should not be mutated")
	}
	if !mapped.HasHooks(HookSubagentStop) {
		t.Fatal("expected mapped runner to contain SubagentStop")
	}
	if !mapped.HasHooks(HookPreToolUse) {
		t.Fatal("expected unrelated hook type to remain")
	}
}

func TestRunExecutesMatchingHooks(t *testing.T) {
	runner := NewRunner([]Hook{
		{Type: HookPreToolUse, Command: "echo '{\"system_reminder\":\"test-reminder\"}'", Timeout: 5},
		{Type: HookPostToolUse, Command: "echo skipped", Timeout: 5},
	})

	outputs := runner.Run(context.Background(), HookPreToolUse, HookInput{})
	if len(outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(outputs))
	}
	if outputs[0].SystemReminder != "test-reminder" {
		t.Errorf("expected 'test-reminder', got '%s'", outputs[0].SystemReminder)
	}
}

func TestRunRespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	runner := NewRunner([]Hook{
		{Type: HookPreToolUse, Command: "sleep 10", Timeout: 30},
	})

	start := time.Now()
	outputs := runner.Run(ctx, HookPreToolUse, HookInput{})
	elapsed := time.Since(start)

	// Should complete quickly, not wait 10 seconds
	if elapsed > 3*time.Second {
		t.Errorf("expected fast cancellation, took %v", elapsed)
	}
	// Hook should have non-zero exit code
	if len(outputs) == 1 && outputs[0].ExitCode == 0 {
		// Context was cancelled, so the command should have been killed
		t.Log("hook was killed by context cancellation")
	}
}

func TestRunPlainTextOutput(t *testing.T) {
	runner := NewRunner([]Hook{
		{Type: HookPreToolUse, Command: "echo 'plain text reminder'", Timeout: 5},
	})

	outputs := runner.Run(context.Background(), HookPreToolUse, HookInput{})
	if len(outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(outputs))
	}
	if outputs[0].SystemReminder == "" {
		t.Error("expected plain text to become system reminder")
	}
}

// TestExecuteHookUnknownKindReturnsError verifies that D1: an unrecognised
// HookKind is not silently routed to the command executor but instead returns
// an error output with Block=true (D1).
func TestExecuteHookUnknownKindReturnsError(t *testing.T) {
	runner := NewRunner(nil)
	hook := Hook{
		Type: HookPreToolUse,
		Kind: HookKind("websocket"), // not a known kind
	}
	input := HookInput{Type: HookPreToolUse}

	output := runner.executeHook(context.Background(), hook, input)

	if !output.Block {
		t.Error("expected Block=true for unknown hook kind")
	}
	if output.ExitCode != -1 {
		t.Errorf("expected ExitCode=-1, got %d", output.ExitCode)
	}
	if output.Stderr == "" {
		t.Error("expected non-empty Stderr describing unknown kind")
	}
}

// TestRunMatcherFiltering verifies C1: a hook with Matcher="Bash" only fires
// when input.ToolName=="Bash" and is skipped for other tool names.
func TestRunMatcherFiltering(t *testing.T) {
	runner := NewRunner([]Hook{
		{
			Type:    HookPreToolUse,
			Matcher: "Bash",
			Command: "echo '{\"system_reminder\":\"bash-hook\"}'",
			Timeout: 5,
		},
		{
			Type:    HookPreToolUse,
			Matcher: "",
			Command: "echo '{\"system_reminder\":\"no-matcher-hook\"}'",
			Timeout: 5,
		},
	})

	// Only hooks matching "Bash" or with no matcher should fire.
	outputs := runner.Run(context.Background(), HookPreToolUse, HookInput{ToolName: "Bash"})
	if len(outputs) != 2 {
		t.Fatalf("expected 2 outputs for ToolName=Bash, got %d", len(outputs))
	}

	// With a different tool name, only the no-matcher hook should fire.
	outputs2 := runner.Run(context.Background(), HookPreToolUse, HookInput{ToolName: "Read"})
	if len(outputs2) != 1 {
		t.Fatalf("expected 1 output for ToolName=Read (Bash-only hook skipped), got %d", len(outputs2))
	}
	if outputs2[0].SystemReminder != "no-matcher-hook" {
		t.Errorf("expected no-matcher-hook reminder, got %q", outputs2[0].SystemReminder)
	}

	// With no tool name at all, only the no-matcher hook should fire.
	outputs3 := runner.Run(context.Background(), HookPreToolUse, HookInput{ToolName: ""})
	if len(outputs3) != 1 {
		t.Fatalf("expected 1 output for empty ToolName, got %d", len(outputs3))
	}
}

// TestLoadFromDirRejectsUnsafeFilenames verifies C4: filenames containing shell
// metacharacters are silently skipped and never loaded as hooks.
func TestLoadFromDirRejectsUnsafeFilenames(t *testing.T) {
	dir := t.TempDir()

	// Safe filename — should be loaded.
	os.WriteFile(filepath.Join(dir, "pre-tool-use-check.sh"), []byte("#!/bin/bash\necho ok"), 0755)

	// Unsafe filenames — must be rejected.
	unsafeNames := []string{
		"pre-tool-use-evil;id.sh",
		"pre-tool-use-evil|cat.sh",
		"pre-tool-use-evil`whoami`.sh",
		"pre-tool-use-evil$(cmd).sh",
		"pre tool use spaces.sh",
	}
	for _, name := range unsafeNames {
		os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/bash\necho injected"), 0755)
	}

	runner, err := LoadFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Only the one safe file should have been loaded.
	if len(runner.hooks) != 1 {
		t.Errorf("expected exactly 1 hook (unsafe names skipped), got %d", len(runner.hooks))
	}
	if runner.hooks[0].Type != HookPreToolUse {
		t.Errorf("expected PreToolUse hook, got %s", runner.hooks[0].Type)
	}
}
