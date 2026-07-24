package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestStopHookRunsWhenAssistantHasNoToolUse(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "stop-input.json")
	runner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookStop,
		Command: fmt.Sprintf("cat > %q", inputPath),
		Timeout: 5,
	}})
	prov := newParityFakeProvider([]parityProviderTurn{{Events: parityTextEvents("done")}})
	q := New(prov, registry.New(), Config{MaxTurns: 3, MaxTokens: 1024, HookRunner: runner, SessionID: "session-stop"})

	var hookSummaries []Event
	if err := q.Run(context.Background(), "hi", func(evt Event) {
		if evt.Type == EventHookSummary {
			hookSummaries = append(hookSummaries, evt)
		}
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(hookSummaries) != 1 || hookSummaries[0].HookSummary == nil || hookSummaries[0].HookSummary.Status != "passed" {
		t.Fatalf("hook summaries = %#v, want one passed summary", hookSummaries)
	}
	if !strings.HasPrefix(hookSummaries[0].TurnID, "session-stop:query-") || !strings.HasSuffix(hookSummaries[0].TurnID, ":turn-1") || hookSummaries[0].ActorID != "assistant" || hookSummaries[0].WorkUnitID != hookSummaries[0].TurnID || hookSummaries[0].HookSummary.HookExecutionID != "hook:"+hookSummaries[0].TurnID+":Stop:config-1" {
		t.Fatalf("stop hook summary lost execution identity: %#v", hookSummaries[0])
	}

	input := readHookInput(t, inputPath)
	if input.HookEventName != hooks.HookStop {
		t.Fatalf("hook_event_name = %q, want Stop", input.HookEventName)
	}
	if input.LastAssistantMessage != "done" {
		t.Fatalf("last_assistant_message = %q, want done", input.LastAssistantMessage)
	}
	if input.StopHookActive {
		t.Fatal("first Stop hook should not be marked active")
	}
	if input.TurnID != hookSummaries[0].TurnID || input.WorkUnitID != hookSummaries[0].WorkUnitID || input.AgentID != hookSummaries[0].ActorID {
		t.Fatalf("stop hook input/event causality diverged: input=%#v event=%#v", input, hookSummaries[0])
	}
}

func TestStopHookIdentityIsUniqueAcrossUserQueries(t *testing.T) {
	runner := hooks.NewRunner([]hooks.Hook{{Type: hooks.HookStop, Command: "true", Timeout: 5}})
	prov := newParityFakeProvider([]parityProviderTurn{{Events: parityTextEvents("first")}, {Events: parityTextEvents("second")}})
	q := New(prov, registry.New(), Config{MaxTurns: 3, MaxTokens: 1024, HookRunner: runner, SessionID: "session-repeat"})
	var summaries []Event
	for _, prompt := range []string{"one", "two"} {
		if err := q.Run(context.Background(), prompt, func(event Event) {
			if event.Type == EventHookSummary {
				summaries = append(summaries, event)
			}
		}); err != nil {
			t.Fatalf("Run(%q): %v", prompt, err)
		}
	}
	if len(summaries) != 2 || summaries[0].TurnID == summaries[1].TurnID || summaries[0].HookSummary.HookExecutionID == summaries[1].HookSummary.HookExecutionID {
		t.Fatalf("cross-query hook identities collided: %#v", summaries)
	}
	for _, summary := range summaries {
		if summary.WorkUnitID != summary.TurnID || !strings.HasSuffix(summary.TurnID, ":turn-1") {
			t.Fatalf("cross-query identity is not internally causal: %#v", summary)
		}
	}
}

func TestStopHookBlockingContinuesWithStopHookActive(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "stop-hook.sh")
	firstInput := filepath.Join(dir, "first.json")
	activeInput := filepath.Join(dir, "active.json")
	script := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
payload="$(cat)"
if grep -q '"stop_hook_active":true' <<<"$payload"; then
  printf '%%s' "$payload" > %q
  exit 0
fi
printf '%%s' "$payload" > %q
printf 'revise final answer' >&2
exit 2
`, activeInput, firstInput)
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write hook script: %v", err)
	}
	runner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookStop,
		Command: scriptPath,
		Timeout: 5,
	}})
	prov := newParityFakeProvider([]parityProviderTurn{
		{Events: parityTextEvents("draft")},
		{Events: parityTextEvents("final")},
	})
	q := New(prov, registry.New(), Config{MaxTurns: 3, MaxTokens: 1024, HookRunner: runner})

	if err := q.Run(context.Background(), "hi", func(Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.Calls) != 2 {
		t.Fatalf("CreateStream calls = %d, want 2", len(prov.Calls))
	}
	if got := joinedMessageText(prov.Calls[1].Messages); !strings.Contains(got, "Stop hook feedback:\nrevise final answer") {
		t.Fatalf("second request messages missing stop hook feedback: %q", got)
	}

	first := readHookInput(t, firstInput)
	if first.StopHookActive {
		t.Fatal("first blocking hook input unexpectedly active")
	}
	active := readHookInput(t, activeInput)
	if !active.StopHookActive {
		t.Fatal("second Stop hook input should carry stop_hook_active=true")
	}
}

func TestStopHookPreventContinuationEndsWithoutNewRequest(t *testing.T) {
	runner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookStop,
		Command: `printf '%s\n' '{"preventContinuation":true,"stopReason":"complete"}'`,
		Timeout: 5,
	}})
	prov := newParityFakeProvider([]parityProviderTurn{
		{Events: parityTextEvents("done")},
		{Events: parityTextEvents("unexpected")},
	})
	q := New(prov, registry.New(), Config{MaxTurns: 3, MaxTokens: 1024, HookRunner: runner})

	if err := q.Run(context.Background(), "hi", func(Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.Calls) != 1 {
		t.Fatalf("CreateStream calls = %d, want 1", len(prov.Calls))
	}
	if got := joinedMessageText(q.Messages()); strings.Contains(got, "Stop hook feedback") {
		t.Fatalf("preventContinuation should not append blocking feedback: %q", got)
	}
}

func TestStopHookSkippedOnAPIError(t *testing.T) {
	dir := t.TempDir()
	touched := filepath.Join(dir, "ran")
	runner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookStop,
		Command: fmt.Sprintf("touch %q", touched),
		Timeout: 5,
	}})
	prov := newParityFakeProvider([]parityProviderTurn{{
		Error: &types.APIError{Type: "invalid_request_error", Message: "bad request"},
	}})
	q := New(prov, registry.New(), Config{MaxTurns: 3, MaxTokens: 1024, HookRunner: runner})

	if err := q.Run(context.Background(), "hi", func(Event) {}); err == nil {
		t.Fatal("Run succeeded, want API error")
	}
	if _, err := os.Stat(touched); !os.IsNotExist(err) {
		t.Fatalf("Stop hook ran after API error; stat err=%v", err)
	}
}

func TestSubagentStopUsesMappedStopHook(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "subagent-stop-input.json")
	runner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookStop,
		Matcher: "reviewer",
		Command: fmt.Sprintf("cat > %q", inputPath),
		Timeout: 5,
	}}).WithHookTypeMapped(hooks.HookStop, hooks.HookSubagentStop)
	prov := newParityFakeProvider([]parityProviderTurn{{Events: parityTextEvents("subagent done")}})
	q := New(prov, registry.New(), Config{
		MaxTurns:            3,
		MaxTokens:           1024,
		HookRunner:          runner,
		AgentID:             "agent-123",
		AgentType:           "reviewer",
		AgentTranscriptPath: "/tmp/agent-123.jsonl",
	})

	if err := q.Run(context.Background(), "hi", func(Event) {}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	input := readHookInput(t, inputPath)
	if input.HookEventName != hooks.HookSubagentStop {
		t.Fatalf("hook_event_name = %q, want SubagentStop", input.HookEventName)
	}
	if input.AgentID != "agent-123" || input.AgentType != "reviewer" {
		t.Fatalf("agent fields = id %q type %q, want agent-123 reviewer", input.AgentID, input.AgentType)
	}
	if input.AgentTranscriptPath != "/tmp/agent-123.jsonl" {
		t.Fatalf("agent_transcript_path = %q", input.AgentTranscriptPath)
	}
}

func readHookInput(t *testing.T, path string) hooks.HookInput {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read hook input %s: %v", path, err)
	}
	var input hooks.HookInput
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatalf("unmarshal hook input %s: %v\n%s", path, err, string(data))
	}
	return input
}

func joinedMessageText(messages []types.Message) string {
	var parts []string
	for _, msg := range messages {
		parts = append(parts, msg.GetText())
	}
	return strings.Join(parts, "\n")
}
