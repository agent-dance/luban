package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestPostSamplingHookSeesMessagesForQueryAndAssistantMessage(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "post-sampling-input.json")
	runner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookPostSampling,
		Command: testHookCaptureCommand(inputPath),
		Timeout: 5,
	}})
	prov := newParityFakeProvider([]parityProviderTurn{{Events: parityTextEvents("sampled answer")}})
	q := New(prov, registry.New(), Config{MaxTurns: 3, MaxTokens: 1024, HookRunner: runner, SessionID: "session-post", AgentID: "agent-reviewer", AgentType: "reviewer"})

	var summary stream.Event
	if err := q.Run(context.Background(), "hello", func(event stream.Event) {
		if event.Type == stream.EventHookSummary {
			summary = event
		}
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	input := readHookInput(t, inputPath)
	if input.HookEventName != hooks.HookPostSampling {
		t.Fatalf("hook_event_name = %q, want PostSampling", input.HookEventName)
	}
	if len(input.Messages) != 2 {
		t.Fatalf("messages len = %d, want messagesForQuery + assistant", len(input.Messages))
	}
	if !strings.HasPrefix(input.TurnID, "session-post:query-") || !strings.HasSuffix(input.TurnID, ":turn-1") || input.WorkUnitID != "agent-reviewer" || input.AgentID != "agent-reviewer" {
		t.Fatalf("hook input identity = turn %q work %q actor %q", input.TurnID, input.WorkUnitID, input.AgentID)
	}
	if summary.HookSummary == nil || summary.HookSummary.HookExecutionID != input.HookExecutionID || input.HookConfigID != "config-1" {
		t.Fatalf("hook summary identity = %#v", summary)
	}
	if summary.TurnID != input.TurnID || summary.WorkUnitID != input.WorkUnitID || summary.ActorID != input.AgentID {
		t.Fatalf("hook event/input causality diverged: event=%#v input=%#v", summary, input)
	}
	assertHookMessage(t, input.Messages[0], "user", "hello")
	assertHookMessage(t, input.Messages[1], "assistant", "sampled answer")
}

func TestToolHooksEmitCorrelatedLosslessExecutionEvidence(t *testing.T) {
	dir := t.TempDir()
	prePath := filepath.Join(dir, "pre.json")
	postPath := filepath.Join(dir, "post.json")
	runner := hooks.NewRunner([]hooks.Hook{
		{Type: hooks.HookPreToolUse, Command: testHookCaptureCommand(prePath), Timeout: 5},
		{Type: hooks.HookPostToolUse, Command: testHookCaptureCommand(postPath), Timeout: 5},
	})
	prov := newParityFakeProvider([]parityProviderTurn{
		{Events: postSamplingToolUseEvents("tool-correlated", "Echo", `{"text":"run"}`)},
		{Events: parityTextEvents("done")},
	})
	reg := registry.New()
	reg.Register(parityTool{name: "Echo", echoPrefix: "tool:", concurrentSafe: true})
	q := New(prov, reg, Config{
		MaxTurns: 3, MaxTokens: 1024, HookRunner: runner,
		SessionID: "session-tools", AgentID: "agent-reviewer", AgentType: "reviewer",
	})

	var summaries []stream.Event
	if err := q.Run(context.Background(), "use tool", func(event stream.Event) {
		if event.Type == stream.EventHookSummary {
			summaries = append(summaries, event)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 {
		t.Fatalf("tool hook summaries = %d, want pre and post: %#v", len(summaries), summaries)
	}
	preInput := readHookInput(t, prePath)
	postInput := readHookInput(t, postPath)
	for _, input := range []hooks.HookInput{preInput, postInput} {
		if input.SessionID != "session-tools" || input.ToolUseID != "tool-correlated" || input.AgentID != "agent-reviewer" || input.WorkUnitID == "" || input.TurnID == "" {
			t.Fatalf("tool hook lost correlation: %+v", input)
		}
	}
	for _, summary := range summaries {
		if summary.TurnID == "" || summary.ActorID != "agent-reviewer" || summary.WorkUnitID == "" || summary.ToolUseID != "tool-correlated" || summary.HookSummary == nil || summary.HookSummary.ToolUseID != "tool-correlated" {
			t.Fatalf("hook event lost correlation: %#v", summary)
		}
		if summary.HookSummary.Metadata["hook_input"] == nil || summary.HookSummary.Metadata["hook_output"] == nil || summary.HookSummary.Metadata["hook_config"] == nil {
			t.Fatalf("hook summary omitted exact evidence: %#v", summary.HookSummary.Metadata)
		}
	}
	if summaries[0].HookSummary.HookExecutionID != preInput.HookExecutionID || summaries[1].HookSummary.HookExecutionID != postInput.HookExecutionID {
		t.Fatalf("hook stdin/event IDs diverged: summaries=%#v pre=%q post=%q", summaries, preInput.HookExecutionID, postInput.HookExecutionID)
	}
}

func TestHookPermissionDenySummaryIsBlockedAndCarriesSourceToolID(t *testing.T) {
	summary := newHookExecutionSummary(hooks.HookPreToolUse, hooks.HookExecution{
		ExecutionID: "hook:turn:PreToolUse:tool-write:config-1",
		Input:       hooks.HookInput{ToolUseID: "write", ToolName: "Write"},
		Output:      hooks.HookOutput{PermissionBehavior: "deny"},
	}, hookSummaryDefaults{Blocked: "hook denied tool"})
	if summary.ToolUseID != "write" || summary.Status != "blocked" {
		t.Fatalf("deny hook summary = %#v, want blocked source tool write", summary)
	}
}

func TestPostSamplingEmitsEvidenceForEachHookConfiguration(t *testing.T) {
	dir := t.TempDir()
	passedInputPath := filepath.Join(dir, "passed.json")
	failedInputPath := filepath.Join(dir, "failed.json")
	runner := hooks.NewRunner([]hooks.Hook{
		{
			Type:    hooks.HookPostSampling,
			Command: testHookCaptureCommand(passedInputPath),
			Timeout: 5,
		},
		{
			Type:    hooks.HookPostSampling,
			Command: testHookCaptureAndFailCommand(failedInputPath, "config failed"),
			Timeout: 5,
		},
	})
	prov := newParityFakeProvider([]parityProviderTurn{{Events: parityTextEvents("done")}})
	q := New(prov, registry.New(), Config{MaxTurns: 3, MaxTokens: 1024, HookRunner: runner, SessionID: "session-configs"})

	var summaries []stream.Event
	if err := q.Run(context.Background(), "hello", func(event stream.Event) {
		if event.Type == stream.EventHookSummary {
			summaries = append(summaries, event)
		}
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("hook summaries = %d, want one per configuration: %#v", len(summaries), summaries)
	}
	if summaries[0].HookSummary.Status != "passed" || summaries[1].HookSummary.Status != "failed" {
		t.Fatalf("hook statuses = %q, %q, want passed, failed", summaries[0].HookSummary.Status, summaries[1].HookSummary.Status)
	}
	if !strings.Contains(summaries[1].HookSummary.Summary, "config failed") {
		t.Fatalf("failed hook summary = %q, want stderr", summaries[1].HookSummary.Summary)
	}
	if summaries[0].HookSummary.HookExecutionID == summaries[1].HookSummary.HookExecutionID {
		t.Fatalf("configuration execution IDs collided: %#v", summaries)
	}
	for index, summary := range summaries {
		configID := fmt.Sprintf("config-%d", index+1)
		if got := summary.HookSummary.Metadata["config_id"]; got != configID {
			t.Fatalf("summary %d config_id = %#v, want %q", index, got, configID)
		}
	}
	passedInput := readHookInput(t, passedInputPath)
	failedInput := readHookInput(t, failedInputPath)
	if passedInput.HookExecutionID != summaries[0].HookSummary.HookExecutionID || failedInput.HookExecutionID != summaries[1].HookSummary.HookExecutionID {
		t.Fatalf("hook stdin/event identity diverged: passed=%#v failed=%#v summaries=%#v", passedInput, failedInput, summaries)
	}
}

func TestPostSamplingNonBlockingFailureDoesNotInterruptToolExecution(t *testing.T) {
	runner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookPostSampling,
		Command: testFailingHookCommand("nonblocking failure"),
		Timeout: 5,
	}})
	prov := newParityFakeProvider([]parityProviderTurn{
		{Events: postSamplingToolUseEvents("tool_1", "Echo", `{"text":"run"}`)},
		{Events: parityTextEvents("done")},
	})
	reg := registry.New()
	reg.Register(parityTool{name: "Echo", echoPrefix: "tool:", concurrentSafe: true})
	q := New(prov, reg, Config{MaxTurns: 3, MaxTokens: 1024, HookRunner: runner})

	var summaries []stream.Event
	if err := q.Run(context.Background(), "use tool", func(event stream.Event) {
		if event.Type == stream.EventHookSummary {
			summaries = append(summaries, event)
		}
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(prov.Calls) != 2 {
		t.Fatalf("CreateStream calls = %d, want tool result continuation", len(prov.Calls))
	}
	if got := joinedMessageText(q.Messages()); !strings.Contains(got, "done") {
		t.Fatalf("final messages missing continuation response: %q", got)
	}
	if len(summaries) == 0 || summaries[0].HookSummary == nil || summaries[0].HookSummary.Status != "failed" || !strings.Contains(summaries[0].HookSummary.Summary, "nonblocking failure") {
		t.Fatalf("non-blocking hook failure summary = %#v", summaries)
	}
}

func TestPostSamplingBlockingFailureStopsBeforeToolExecution(t *testing.T) {
	runner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookPostSampling,
		Command: testHookOutputCommand(`{"block":true,"system_reminder":"blocked by policy"}`),
		Timeout: 5,
	}})
	prov := newParityFakeProvider([]parityProviderTurn{
		{Events: postSamplingToolUseEvents("tool_1", "Echo", `{"text":"run"}`)},
		{Events: parityTextEvents("unexpected")},
	})
	reg := registry.New()
	reg.Register(parityTool{name: "Echo", echoPrefix: "tool:", concurrentSafe: true})
	q := New(prov, reg, Config{MaxTurns: 3, MaxTokens: 1024, HookRunner: runner})

	err := q.Run(context.Background(), "use tool", func(stream.Event) {})
	if err == nil || !strings.Contains(err.Error(), "post-sampling hook blocked continuation") {
		t.Fatalf("Run err = %v, want post-sampling block", err)
	}
	if len(prov.Calls) != 1 {
		t.Fatalf("CreateStream calls = %d, want no tool-result continuation", len(prov.Calls))
	}
	if got := joinedMessageText(q.Messages()); strings.Contains(got, "tool:run") {
		t.Fatalf("tool executed despite blocking post-sampling hook: %q", got)
	}
}

func TestStopFailureHookRunsOnceAfterTransientRecoveryExhausted(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "stop-failure-inputs.jsonl")
	runner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookStopFailure,
		Command: testHookAppendInputCommand(inputPath),
		Timeout: 5,
	}})
	prov := newParityFakeProvider([]parityProviderTurn{
		{Error: &types.APIError{Type: "overloaded_error", Message: "try later"}},
		{Error: &types.APIError{Type: "overloaded_error", Message: "try later"}},
		{Error: &types.APIError{Type: "overloaded_error", Message: "try later"}},
		{Error: &types.APIError{Type: "overloaded_error", Message: "try later"}},
		{Error: &types.APIError{Type: "overloaded_error", Message: "try later"}},
		{Error: &types.APIError{Type: "overloaded_error", Message: "try later"}},
		{Error: &types.APIError{Type: "overloaded_error", Message: "try later"}},
		{Error: &types.APIError{Type: "overloaded_error", Message: "try later"}},
		{Error: &types.APIError{Type: "overloaded_error", Message: "try later"}},
		{Error: &types.APIError{Type: "overloaded_error", Message: "try later"}},
		{Error: &types.APIError{Type: "overloaded_error", Message: "try later"}},
	})
	q := New(prov, registry.New(), Config{MaxTurns: 3, MaxTokens: 1024, HookRunner: runner})

	if err := q.Run(context.Background(), "hello", func(stream.Event) {}); err == nil {
		t.Fatal("Run succeeded, want exhausted API error")
	}
	data, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatalf("read stop failure inputs: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("StopFailure hook runs = %d, want 1\n%s", len(lines), string(data))
	}
	input := readHookInputFromBytes(t, []byte(lines[0]))
	if input.HookEventName != hooks.HookStopFailure {
		t.Fatalf("hook_event_name = %q, want StopFailure", input.HookEventName)
	}
	if input.Result != "try later" {
		t.Fatalf("result = %q, want try later", input.Result)
	}
}

func TestTurnSideEffectsRunAtFinalTurnAndSkipBareOrSimpleMode(t *testing.T) {
	t.Run("final turn", func(t *testing.T) {
		effects := &recordingTurnSideEffects{}
		prov := newParityFakeProvider([]parityProviderTurn{{Events: parityTextEvents("done")}})
		q := New(prov, registry.New(), Config{MaxTurns: 3, MaxTokens: 1024, TurnSideEffects: effects})

		if err := q.Run(context.Background(), "hello", func(stream.Event) {}); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got := effects.calls.Load(); got != 1 {
			t.Fatalf("side effect calls = %d, want 1", got)
		}
		if got := joinedMessageText(effects.messages); !strings.Contains(got, "done") {
			t.Fatalf("side effect messages missing assistant final answer: %q", got)
		}
	})

	t.Run("bare mode", func(t *testing.T) {
		effects := &recordingTurnSideEffects{}
		prov := newParityFakeProvider([]parityProviderTurn{{Events: parityTextEvents("done")}})
		q := New(prov, registry.New(), Config{MaxTurns: 3, MaxTokens: 1024, TurnSideEffects: effects, BareMode: true})

		if err := q.Run(context.Background(), "hello", func(stream.Event) {}); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got := effects.calls.Load(); got != 0 {
			t.Fatalf("side effect calls = %d, want 0", got)
		}
	})

	t.Run("simple env", func(t *testing.T) {
		t.Setenv("LUBAN_CODE_SIMPLE", "true")
		effects := &recordingTurnSideEffects{}
		prov := newParityFakeProvider([]parityProviderTurn{{Events: parityTextEvents("done")}})
		q := New(prov, registry.New(), Config{MaxTurns: 3, MaxTokens: 1024, TurnSideEffects: effects})

		if err := q.Run(context.Background(), "hello", func(stream.Event) {}); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got := effects.calls.Load(); got != 0 {
			t.Fatalf("side effect calls = %d, want 0", got)
		}
	})
}

type recordingTurnSideEffects struct {
	calls    atomic.Int32
	messages []types.Message
}

func (r *recordingTurnSideEffects) StartTurnSideEffects(_ context.Context, messages []types.Message, _ TurnSideEffectOptions) {
	r.calls.Add(1)
	r.messages = append([]types.Message(nil), messages...)
}

func postSamplingToolUseEvents(id, name, input string) []types.StreamEvent {
	return []types.StreamEvent{
		{Type: types.EventMessageStart},
		{Type: types.EventContentBlockStart, Index: 0, ContentBlock: &types.ContentDelta{Type: types.ContentTypeToolUse, ID: id, Name: name}},
		{Type: types.EventContentBlockDelta, Index: 0, Delta: &types.ContentDelta{Type: "input_json_delta", PartialJSON: input}},
		{Type: types.EventContentBlockStop, Index: 0},
		{Type: types.EventMessageDelta, StopReason: stopReasonForParity(types.StopReasonToolUse)},
		{Type: types.EventMessageStop},
	}
}

func assertHookMessage(t *testing.T, raw any, role, text string) {
	t.Helper()
	msg, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("hook message %#v is %T, want object", raw, raw)
	}
	if got, _ := msg["role"].(string); got != role {
		t.Fatalf("message role = %q, want %q in %#v", got, role, msg)
	}
	content, ok := msg["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("message content = %#v, want non-empty array", msg["content"])
	}
	block, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("content[0] = %#v, want object", content[0])
	}
	if got, _ := block["text"].(string); got != text {
		t.Fatalf("message text = %q, want %q in %#v", got, text, msg)
	}
}

func readHookInputFromBytes(t *testing.T, data []byte) hooks.HookInput {
	t.Helper()
	var input hooks.HookInput
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatalf("unmarshal hook input: %v\n%s", err, string(data))
	}
	return input
}
