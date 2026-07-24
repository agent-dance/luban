package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type failAfterPartialProvider struct{ calls int }

func (p *failAfterPartialProvider) Name() string    { return "fail-after-partial" }
func (p *failAfterPartialProvider) ModelID() string { return "fail-after-partial-model" }

func (p *failAfterPartialProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	p.calls++
	if p.calls == 1 {
		return eventStream(agentTextEvents("useful partial result")), nil
	}
	return nil, errors.New("continuation provider failed")
}

type failAfterMeasuredUsageProvider struct{ calls int }

func (p *failAfterMeasuredUsageProvider) Name() string    { return "fail-after-measured-usage" }
func (p *failAfterMeasuredUsageProvider) ModelID() string { return "fail-after-measured-usage-model" }

func (p *failAfterMeasuredUsageProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	p.calls++
	if p.calls == 1 {
		return eventStream(agentEventsWithUsage(agentToolEvents("Echo", "call_before_failure"), types.Usage{
			InputTokens:          11,
			OutputTokens:         2,
			CacheReadInputTokens: 7,
		})), nil
	}
	return nil, errors.New("provider failed after measured turn")
}

type agentPrivateRuntimeFailureProvider struct{ cause error }

func (p *agentPrivateRuntimeFailureProvider) Name() string    { return "private-runtime-failure" }
func (p *agentPrivateRuntimeFailureProvider) ModelID() string { return "private-runtime-failure-model" }
func (p *agentPrivateRuntimeFailureProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	return nil, p.cause
}

type agentPrivateWarningProvider struct {
	calls int
	cause error
}

func (p *agentPrivateWarningProvider) Name() string    { return "private-warning" }
func (p *agentPrivateWarningProvider) ModelID() string { return "private-warning-model" }
func (p *agentPrivateWarningProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	p.calls++
	if p.calls == 1 {
		return nil, p.cause
	}
	return eventStream(agentTextEvents("recovered result")), nil
}

func TestRunAgentQueryLoopPreservesPartialOutputWithoutMaskingFailure(t *testing.T) {
	p := &failAfterPartialProvider{}
	runner := hooks.NewRunner([]hooks.Hook{{
		Type:    hooks.HookSubagentStop,
		Matcher: "general-purpose",
		Command: `echo '{"system_reminder":"continue once","block":true}'`,
		Timeout: 1,
	}})
	subLoop := loop.New(p, registry.New(), loop.Config{MaxTurns: 1, HookRunner: runner, SessionID: "partial-failure"})
	summary, err := runAgentQueryLoop(context.Background(), subLoop, agentSessionMetadata{AgentType: "general-purpose"}, "partial-failure", "research", nil)
	if err == nil {
		t.Fatalf("partial continuation failure was reported as success: %+v", summary)
	}
	if !strings.Contains(summary.Output, "useful partial result") {
		t.Fatalf("partial output was discarded: %+v", summary)
	}
}

func TestAgentRuntimeErrorModelProjectionHidesPrivateEventMaterial(t *testing.T) {
	secret := "/Users/private/.config/provider token=sk-agent-secret"
	apiError := &types.APIError{Type: "private_provider_error", Message: secret}
	message, private := agentRuntimeErrorModelMessage("agent-private", loop.Event{
		Type: loop.EventError, Text: secret, Error: apiError,
		ProjectRoot: "/private/project", TurnID: "private-session:query-1:turn-1", ToolUseID: "private-tool",
		Metadata: map[string]any{"authorization": "Bearer private-token"},
	})
	if message != i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeErrorPublicSummary) {
		t.Fatalf("model runtime-error projection = %q", message)
	}
	if !errors.Is(private, apiError) {
		t.Fatal("private API cause was not retained in the RuntimeEvent error chain")
	}
	for _, raw := range []string{secret, "sk-agent-secret", "private_provider_error", "private-token", "private-session", "private-tool", "private-project"} {
		if strings.Contains(message, raw) {
			t.Fatalf("model runtime-error projection leaked %q: %q", raw, message)
		}
	}
}

func TestAgentRuntimeWarningModelProjectionHidesPrivateEventMaterial(t *testing.T) {
	secret := "/Users/private/.config/provider token=sk-agent-warning\x1b[2J"
	apiError := &types.APIError{Type: "private_warning", Message: secret}
	event := loop.NewSystemWarningEvent(
		i18n.KeyRuntimeAutoCompactFailed,
		nil,
		apiError,
		map[string]any{"authorization": "Bearer private-token", "project_root": "/private/project"},
		2,
	)
	event.Text = secret
	event.Error = &types.APIError{Type: "raw_warning", Message: secret}
	event.ProjectRoot = "/private/project"
	event.Metadata = map[string]any{"authorization": "Bearer raw-private-token"}
	message, private := agentRuntimeWarningModelMessage("agent-private", event)
	if message != i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeAutoCompactFailed) {
		t.Fatalf("model runtime-warning projection = %q", message)
	}
	if !errors.Is(private, apiError) {
		t.Fatal("private warning cause was not retained in the RuntimeEvent error chain")
	}
	for _, raw := range []string{secret, "sk-agent-warning", "private_warning", "raw_warning", "private-token", "raw-private-token", "/private/project", "\x1b[2J"} {
		if strings.Contains(message, raw) {
			t.Fatalf("model runtime-warning projection leaked %q: %q", raw, message)
		}
	}
}

func TestRunAgentQueryLoopProjectsSystemWarningForParentModel(t *testing.T) {
	secret := "/Users/private/.ssh/id_ed25519 token=sk-agent-loop-warning\x1b[31m"
	cause := &provider.FallbackTriggeredError{
		OriginalModel: "primary-model", FallbackModel: "fallback-model", Cause: errors.New(secret),
	}
	provider := &agentPrivateWarningProvider{cause: cause}
	subLoop := loop.New(provider, registry.New(), loop.Config{MaxTurns: 1, MaxTokens: 1024, SessionID: "agent-private-session", ProjectRoot: "/private/project"})
	summary, err := runAgentQueryLoop(context.Background(), subLoop, agentSessionMetadata{AgentType: "general-purpose"}, "agent-private", "inspect", nil)
	if err != nil {
		t.Fatalf("recovered warning became terminal failure: %v", err)
	}
	if !strings.Contains(summary.Output, i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeModelFallback, "primary-model", "fallback-model")) || !strings.Contains(summary.Output, "recovered result") {
		t.Fatalf("parent-model output omitted semantic warning or result: %q", summary.Output)
	}
	for _, raw := range []string{secret, "sk-agent-loop-warning", "/Users/private/.ssh/id_ed25519", "/private/project", "\x1b[31m"} {
		if strings.Contains(summary.Output, raw) {
			t.Fatalf("parent-model warning leaked %q: %q", raw, summary.Output)
		}
	}
}

func TestRunAgentQueryLoopHidesEventErrorTextButPreservesCause(t *testing.T) {
	secret := "/Users/private/.ssh/id_ed25519 token=sk-agent-loop-secret"
	privateCause := errors.New(secret)
	subLoop := loop.New(&agentPrivateRuntimeFailureProvider{cause: privateCause}, registry.New(), loop.Config{MaxTurns: 1, SessionID: "agent-private-session"})
	summary, err := runAgentQueryLoop(context.Background(), subLoop, agentSessionMetadata{AgentType: "general-purpose"}, "agent-private", "inspect", nil)
	if err == nil {
		t.Fatalf("private provider failure was reported as success: %+v", summary)
	}
	if !errors.Is(err, privateCause) {
		t.Fatalf("private provider cause was not retained: %v", err)
	}
	visible := summary.Output + "\n" + err.Error()
	if !strings.Contains(visible, i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeErrorPublicSummary)) {
		t.Fatalf("safe model projection missing from error: %q", visible)
	}
	for _, raw := range []string{secret, "sk-agent-loop-secret", "/Users/private/.ssh/id_ed25519"} {
		if strings.Contains(visible, raw) {
			t.Fatalf("agent model-visible failure leaked %q: %q", raw, visible)
		}
	}
}

func TestAgentToolFailurePropagatesUsageFromCompletedTurns(t *testing.T) {
	reg := registry.New()
	reg.Register(fakeTool{name: "Echo"})
	tool := &AgentTool{Provider: &failAfterMeasuredUsageProvider{}, Registry: reg}

	result, err := tool.Execute(context.Background(), agentExecuteInput("fail after one measured turn", nil))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected agent failure, got %+v", result)
	}
	want := types.Usage{InputTokens: 11, OutputTokens: 2, CacheReadInputTokens: 7}
	if result.Usage == nil || *result.Usage != want {
		t.Fatalf("failure ToolResult.Usage = %+v, want %+v", result.Usage, want)
	}
	block := types.MapToolResult(tool, result, "toolu_failed_agent")
	if block.Usage == nil || *block.Usage != want {
		t.Fatalf("failure ToolResultBlock.Usage = %+v, want %+v", block.Usage, want)
	}
}

func TestAgentOutputSchemaDeclaresEveryTypedUnionField(t *testing.T) {
	usage := agentToolUsage{InputTokens: 1, OutputTokens: 2}
	results := []AgentResult{
		AgentCompleted{
			AgentResultBase: AgentResultBase{Kind: AgentResultKindCompleted, TranscriptPath: "transcript", DurationMs: 1, TotalTokens: 3},
			AgentID:         "agent", AgentType: "general-purpose", Prompt: "prompt",
			Content: []agentToolContentBlock{{Type: "text", Text: "done"}}, ToolUseCount: 1, Usage: usage,
			CWD: "/tmp", Mode: "default", Isolation: "worktree", Model: "model",
			WorktreePath: "/tmp/worktree", WorktreeBranch: "branch", LatestToolUse: "Read", WireStatus: "completed",
		},
		AgentError{AgentResultBase: AgentResultBase{Kind: AgentResultKindError, TranscriptPath: "transcript", DurationMs: 1, TotalTokens: 2}, AgentID: "agent", AgentType: "general-purpose", Message: "failed", ExitReason: "provider", WireStatus: "error"},
		AgentAborted{AgentResultBase: AgentResultBase{Kind: AgentResultKindAborted, TranscriptPath: "transcript", DurationMs: 1, TotalTokens: 2}, AgentID: "agent", AgentType: "general-purpose", Reason: "cancelled", LatestToolUse: "Bash", WireStatus: "aborted"},
		AgentPartial{AgentResultBase: AgentResultBase{Kind: AgentResultKindPartial, TranscriptPath: "transcript", DurationMs: 1, TotalTokens: 2}, AgentID: "agent", AgentType: "general-purpose", TaskID: "task", OutputFile: "output", CanReadOutputFile: true, Description: "desc", Prompt: "prompt", IsAsync: true, Message: "running", SessionURL: "https://example.test", WireStatus: "remote_launched", LatestToolUse: "Read"},
		AgentIncomplete{AgentResultBase: AgentResultBase{Kind: AgentResultKindPartial, TranscriptPath: "transcript", DurationMs: 1, TotalTokens: 2}, AgentID: "agent", AgentType: "general-purpose", Prompt: "prompt", Content: []agentToolContentBlock{{Type: "text", Text: "partial"}}, Outcome: AgentRunOutcomePartial, Reason: "max_turns", ToolUseCount: 1, Usage: usage, LatestToolUse: "Bash", ArtifactRefs: []string{"artifact"}, VerificationRefs: []string{"verification"}, WireStatus: "partial"},
		AgentIncomplete{AgentResultBase: AgentResultBase{Kind: AgentResultKindTimedOut, TranscriptPath: "transcript", DurationMs: 1, TotalTokens: 2}, AgentID: "agent", AgentType: "general-purpose", Outcome: AgentRunOutcomeTimedOut, Reason: "deadline_exceeded", WireStatus: "timed_out"},
		AgentIncomplete{AgentResultBase: AgentResultBase{Kind: AgentResultKindCancelled, TranscriptPath: "transcript", DurationMs: 1, TotalTokens: 2}, AgentID: "agent", AgentType: "general-purpose", Outcome: AgentRunOutcomeCancelled, Reason: "context_cancelled", WireStatus: "cancelled"},
		AgentIncomplete{AgentResultBase: AgentResultBase{Kind: AgentResultKindInterrupted, TranscriptPath: "transcript", DurationMs: 1, TotalTokens: 2}, AgentID: "agent", AgentType: "general-purpose", Outcome: AgentRunOutcomeInterrupted, Reason: "runtime_interrupted", WireStatus: "interrupted"},
	}

	schema := agentOutputSchema()
	for _, result := range results {
		encoded, err := MarshalAgentResult(result)
		if err != nil {
			t.Fatalf("MarshalAgentResult(%T): %v", result, err)
		}
		var value map[string]any
		if err := json.Unmarshal(encoded, &value); err != nil {
			t.Fatalf("decode %T: %v", result, err)
		}
		properties := agentVariantPropertiesForTest(t, schema, string(result.ResultKind()))
		for field := range value {
			if _, ok := properties[field]; !ok {
				t.Errorf("%T emits undeclared strict output field %q", result, field)
			}
		}
	}
}

func agentVariantPropertiesForTest(t *testing.T, schema types.JSONSchema, kind string) map[string]any {
	t.Helper()
	for _, raw := range schema.AnyOf {
		variant, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		properties, _ := variant["properties"].(map[string]any)
		kindSchema, _ := properties["kind"].(map[string]any)
		enums, _ := kindSchema["enum"].([]string)
		if len(enums) == 1 && enums[0] == kind {
			return properties
		}
	}
	t.Fatalf("missing output schema variant %q", kind)
	return nil
}
