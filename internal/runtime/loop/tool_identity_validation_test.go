package loop

import (
	"context"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type identityValidationProvider struct {
	toolUses []types.ToolUseBlock
	turn     atomic.Int32
}

type identityReuseProvider struct {
	turn atomic.Int32
}

func (p *identityReuseProvider) Name() string    { return "identity-reuse" }
func (p *identityReuseProvider) ModelID() string { return "identity-reuse-model" }
func (p *identityReuseProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	if p.turn.Add(1) <= 2 {
		return makeStreamChan(aggregateToolUseEvents(types.ToolUseBlock{
			Type: types.ContentTypeToolUse, ID: "toolu-reused", Name: "IdentitySideEffect", Input: map[string]any{},
		})...), nil
	}
	return makeStreamChan(parityTextEvents("unexpected")...), nil
}

func (p *identityValidationProvider) Name() string    { return "identity-validation" }
func (p *identityValidationProvider) ModelID() string { return "identity-validation-model" }
func (p *identityValidationProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	if p.turn.Add(1) == 1 {
		return makeStreamChan(aggregateToolUseEvents(p.toolUses...)...), nil
	}
	return makeStreamChan(parityTextEvents("done")...), nil
}

type identitySideEffectTool struct {
	executions atomic.Int32
}

func (t *identitySideEffectTool) Name() string        { return "IdentitySideEffect" }
func (t *identitySideEffectTool) Description() string { return "identity validation side-effect probe" }
func (t *identitySideEffectTool) Schema() types.JSONSchema {
	return types.JSONSchema{Type: "object"}
}
func (t *identitySideEffectTool) Execute(context.Context, map[string]any) (types.ToolResult, error) {
	t.executions.Add(1)
	return types.ToolResult{Content: "executed"}, nil
}
func (t *identitySideEffectTool) IsConcurrentSafe() bool { return true }

type identityPermissionProbe struct {
	checks atomic.Int32
}

func (p *identityPermissionProbe) Check(context.Context, permission.PermissionRequest) (permission.PermissionDecision, error) {
	p.checks.Add(1)
	return permission.PermissionAllowOnce, nil
}

type identityPostSamplingProbe struct {
	runs atomic.Int32
}

func (p *identityPostSamplingProbe) RunPostSampling(context.Context, []types.Message, PostSamplingOptions) PostSamplingResult {
	p.runs.Add(1)
	return PostSamplingResult{}
}
func (*identityPostSamplingProbe) RunStopFailure(context.Context, types.Message, StopFailureOptions) {
}

func TestQueryLoopRejectsMalformedToolUseIdentityBeforeSideEffects(t *testing.T) {
	tests := []struct {
		name      string
		toolUses  []types.ToolUseBlock
		wantKind  string
		wantID    string
		wantIndex int
	}{
		{
			name: "missing",
			toolUses: []types.ToolUseBlock{{
				Type: types.ContentTypeToolUse, Name: "IdentitySideEffect", Input: map[string]any{},
			}},
			wantKind: "missing_tool_use_id", wantIndex: 0,
		},
		{
			name: "duplicate",
			toolUses: []types.ToolUseBlock{
				{Type: types.ContentTypeToolUse, ID: "toolu-duplicate", Name: "IdentitySideEffect", Input: map[string]any{}},
				{Type: types.ContentTypeToolUse, ID: "toolu-duplicate", Name: "IdentitySideEffect", Input: map[string]any{}},
			},
			wantKind: "duplicate_tool_use_id", wantID: "toolu-duplicate", wantIndex: 1,
		},
	}

	for _, test := range tests {
		for _, streaming := range []bool{false, true} {
			t.Run(test.name+map[bool]string{false: "/batch", true: "/streaming"}[streaming], func(t *testing.T) {
				tool := &identitySideEffectTool{}
				permission := &identityPermissionProbe{}
				postSampling := &identityPostSamplingProbe{}
				reg := registry.New()
				reg.Register(tool)
				query := New(&identityValidationProvider{toolUses: test.toolUses}, reg, Config{
					MaxTurns: 2, SessionID: "session-identity", StreamingToolExecution: streaming,
					PermissionHandler: permission, PostSamplingRunner: postSampling,
				})

				var emitted []stream.Event
				err := query.Run(context.Background(), "run malformed tools", func(event stream.Event) {
					emitted = append(emitted, event)
				})
				if err == nil || !strings.Contains(err.Error(), test.wantKind) {
					t.Fatalf("Run error = %v, want %s", err, test.wantKind)
				}
				if got := tool.executions.Load(); got != 0 {
					t.Fatalf("tool executions = %d, want 0", got)
				}
				if got := permission.checks.Load(); got != 0 {
					t.Fatalf("permission checks = %d, want 0", got)
				}
				if got := postSampling.runs.Load(); got != 0 {
					t.Fatalf("post-sampling hook runs = %d, want 0", got)
				}

				var structured *stream.Event
				for i := range emitted {
					if emitted[i].Type == stream.EventToolUse || emitted[i].Type == stream.EventToolResult {
						t.Fatalf("malformed identity became a tool event: %#v", emitted[i])
					}
					if emitted[i].Type == stream.EventError && emitted[i].Error != nil && emitted[i].Error.Type == "invalid_tool_use_identity" {
						structured = &emitted[i]
					}
				}
				if structured == nil {
					t.Fatalf("missing structured identity error in %#v", emitted)
				}
				if structured.ToolUseID != test.wantID || structured.Metadata["reason"] != test.wantKind || structured.Metadata["index"] != test.wantIndex || structured.Metadata["outcome"] != string(types.ToolOutcomeFailed) {
					t.Fatalf("structured identity event = %#v", structured)
				}
			})
		}
	}
}

func TestQueryLoopRejectsLaterTurnToolUseIDReuseBeforeDownstreamSideEffects(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "batch", true: "streaming"}[streaming], func(t *testing.T) {
			tool := &identitySideEffectTool{}
			permission := &identityPermissionProbe{}
			postSampling := &identityPostSamplingProbe{}
			reg := registry.New()
			reg.Register(tool)
			query := New(&identityReuseProvider{}, reg, Config{
				MaxTurns: 3, SessionID: "session-reuse", StreamingToolExecution: streaming,
				PermissionHandler: permission, PostSamplingRunner: postSampling,
			})

			var emitted []stream.Event
			err := query.Run(context.Background(), "reuse a tool ID", func(event stream.Event) {
				emitted = append(emitted, event)
			})
			if err == nil || !strings.Contains(err.Error(), "reused_tool_use_id") {
				t.Fatalf("Run error = %v, want reused_tool_use_id", err)
			}
			if got := tool.executions.Load(); got != 1 {
				t.Fatalf("tool executions = %d, want only first turn", got)
			}
			if got := permission.checks.Load(); got != 1 {
				t.Fatalf("permission checks = %d, want only first turn", got)
			}
			if got := postSampling.runs.Load(); got != 1 {
				t.Fatalf("post-sampling runs = %d, want only first turn", got)
			}

			var identityError *stream.Event
			toolUseEvents := 0
			for i := range emitted {
				if emitted[i].Type == stream.EventToolUse {
					toolUseEvents++
				}
				if emitted[i].Type == stream.EventError && emitted[i].Metadata["reason"] == "reused_tool_use_id" {
					identityError = &emitted[i]
				}
			}
			if identityError == nil || identityError.ToolUseID != "toolu-reused" || identityError.TurnCount != 2 {
				t.Fatalf("structured reuse error = %#v; events=%#v", identityError, emitted)
			}
			if toolUseEvents != 1 {
				t.Fatalf("tool use events = %d, want only first turn", toolUseEvents)
			}
		})
	}
}

func TestSetMessagesReservesHistoricalToolUseIDsForResumedSession(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "batch", true: "streaming"}[streaming], func(t *testing.T) {
			tool := &identitySideEffectTool{}
			permission := &identityPermissionProbe{}
			postSampling := &identityPostSamplingProbe{}
			reg := registry.New()
			reg.Register(tool)
			query := New(&identityValidationProvider{toolUses: []types.ToolUseBlock{{
				Type: types.ContentTypeToolUse, ID: "toolu-resumed", Name: "IdentitySideEffect", Input: map[string]any{},
			}}}, reg, Config{
				MaxTurns: 2, SessionID: "session-resumed", StreamingToolExecution: streaming,
				PermissionHandler: permission, PostSamplingRunner: postSampling,
			})
			query.SetMessages([]types.Message{
				{Role: types.RoleAssistant, Content: []types.ContentBlock{types.ToolUseBlock{
					Type: types.ContentTypeToolUse, ID: "toolu-resumed", Name: "IdentitySideEffect", Input: map[string]any{},
				}}},
				types.ToolResultMessage(types.ToolResultBlock{
					Type: types.ContentTypeToolResult, ToolUseID: "toolu-resumed", Content: "historical result",
				}),
			})

			err := query.Run(context.Background(), "continue", func(stream.Event) {})
			if err == nil || !strings.Contains(err.Error(), "reused_tool_use_id") {
				t.Fatalf("Run error = %v, want resumed-session identity refusal", err)
			}
			if got := tool.executions.Load(); got != 0 {
				t.Fatalf("tool executions = %d, want 0", got)
			}
			if got := permission.checks.Load(); got != 0 {
				t.Fatalf("permission checks = %d, want 0", got)
			}
			if got := postSampling.runs.Load(); got != 0 {
				t.Fatalf("post-sampling runs = %d, want 0", got)
			}
		})
	}
}

func TestToolUseIdentityLedgerSurvivesInternalCompactedMessageReplacement(t *testing.T) {
	query := New(&identityValidationProvider{}, registry.New(), Config{})
	query.SetMessages([]types.Message{{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{types.ToolUseBlock{
			Type: types.ContentTypeToolUse, ID: "tool-before-compact", Name: "IdentitySideEffect", Input: map[string]any{},
		}},
	}})

	// ForceCompact replaces q.messages directly rather than calling SetMessages.
	// Model that replacement without coupling this identity invariant to the
	// summarizer/provider fixture.
	query.messages = []types.Message{types.UserMessage("compacted summary")}
	if _, ok := query.seenToolUseIDs["tool-before-compact"]; !ok {
		t.Fatal("internal compacted message replacement discarded session-lifetime identity")
	}
}

func TestSetMessagesWithRuntimeLedgersUnionsCompactedAndPersistedIdentities(t *testing.T) {
	query := New(&identityValidationProvider{}, registry.New(), Config{})
	query.SetMessagesWithRuntimeLedgers([]types.Message{{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{types.ToolUseBlock{
			Type: types.ContentTypeToolUse, ID: "tool-visible", Name: "IdentitySideEffect", Input: map[string]any{},
		}},
	}}, []string{"tool-compacted", "tool-visible", "tool-compacted", ""}, nil)
	if got, want := query.SeenToolUseIDs(), []string{"tool-compacted", "tool-visible"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("restored tool identity union = %v, want %v", got, want)
	}
}

func TestMessageUpdatePreservesCompactedSessionToolUseLedger(t *testing.T) {
	tool := &identitySideEffectTool{}
	permission := &identityPermissionProbe{}
	postSampling := &identityPostSamplingProbe{}
	reg := registry.New()
	reg.Register(tool)
	query := New(&identityValidationProvider{toolUses: []types.ToolUseBlock{{
		Type: types.ContentTypeToolUse, ID: "toolu-before-compact", Name: "IdentitySideEffect", Input: map[string]any{},
	}}}, reg, Config{
		MaxTurns: 2, SessionID: "session-mcp-ledger",
		PermissionHandler: permission, PostSamplingRunner: postSampling,
	})
	query.SetMessagesWithRuntimeLedgers(
		[]types.Message{types.UserMessage("compacted transcript")},
		[]string{"toolu-before-compact"},
		nil,
	)

	next := append(query.Messages(), types.UserMessage("same-session message"))
	query.SetMessagesPreservingToolUseLedger(next)

	err := query.Run(context.Background(), "continue after message update", func(stream.Event) {})
	if err == nil || !strings.Contains(err.Error(), "reused_tool_use_id") {
		t.Fatalf("Run error = %v, want persisted identity refusal after same-session message update", err)
	}
	if got := tool.executions.Load(); got != 0 {
		t.Fatalf("tool executions = %d, want 0", got)
	}
	if got := permission.checks.Load(); got != 0 {
		t.Fatalf("permission checks = %d, want 0", got)
	}
	if got := postSampling.runs.Load(); got != 0 {
		t.Fatalf("post-sampling runs = %d, want 0", got)
	}
}
