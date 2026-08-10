package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/runtime/engine"
	"github.com/agent-dance/luban/sdk"
	"github.com/agent-dance/luban/types"
)

func TestSDKEventAdapterOwnsRuntimeProjection(t *testing.T) {
	runtimeWarning := types.NewRuntimeEvent(
		types.RuntimeEventKindWarning,
		types.RuntimeIdentity{SessionID: "session-sdk", TurnID: "turn-sdk", ToolUseID: "tool-sdk"},
		types.ToolOutcomeFailed,
		i18n.KeyRuntimeWarningPublicSummary,
		nil,
		"runtime.warning",
		errors.New("private warning cause"),
	)
	event := sdkEventFromStream(stream.Event{
		Type: stream.EventToolResult, TurnID: "turn-sdk", ActorID: "agent-sdk", WorkUnitID: "work-sdk",
		ToolResult: &types.ToolResultBlock{
			ToolUseID: "tool-sdk", Content: "completed", Outcome: types.ToolOutcomePartial,
			ContentBlocks: []types.ContentBlock{types.TextBlock{Type: types.ContentTypeText, Text: "completed"}},
			Metadata:      map[string]string{"source": "runtime"},
			Usage:         &types.Usage{InputTokens: 11, OutputTokens: 7},
		},
		RuntimeEvent: &runtimeWarning,
	})

	if event.Type != sdk.EventToolResult || event.TurnID != "turn-sdk" || event.ActorID != "agent-sdk" || event.WorkUnitID != "work-sdk" {
		t.Fatalf("SDK event identity = %+v", event)
	}
	if event.ToolResult == nil || event.ToolResult.ToolUseID != "tool-sdk" || event.ToolResult.Outcome != sdk.ToolOutcomePartial || event.ToolResult.Content != "completed" {
		t.Fatalf("SDK tool result = %+v", event.ToolResult)
	}
	if event.ToolResult.Usage == nil || event.ToolResult.Usage.InputTokens != 11 || len(event.ToolResult.ContentBlocks) != 1 {
		t.Fatalf("SDK tool result evidence = %+v", event.ToolResult)
	}
	if event.RuntimeEvent == nil || event.RuntimeEvent.Kind != string(types.RuntimeEventKindWarning) || event.RuntimeEvent.EventID == "" || event.RuntimeEvent.PrivateCause == nil {
		t.Fatalf("SDK runtime event = %+v", event.RuntimeEvent)
	}
}

func TestSDKPermissionAdapterProjectsOnlyPublicRequest(t *testing.T) {
	request := sdkPermissionRequest(permission.PermissionRequest{
		SessionID: "session-sdk", ExecutionSessionID: "execution-sdk", TurnID: "turn-sdk",
		DecisionID: "decision-sdk", ToolUseID: "tool-sdk", ToolName: "Write",
		Input: map[string]any{"file_path": "/tmp/output"}, ActorID: "agent-sdk", ActorType: "executor",
		WorkUnitID: "work-sdk", Action: "write", Target: "/tmp/output", Choices: []string{"allow", "deny"},
		Suggestions: []types.PermissionUpdate{{
			Type: types.PermissionUpdateAddRules, Destination: types.PermissionDestinationSession,
			Rules: []types.PermissionRuleValue{{ToolName: "Write", RuleContent: "/tmp/**"}},
		}},
	})

	if request.SessionID != "session-sdk" || request.ExecutionSessionID != "execution-sdk" || request.DecisionID != "decision-sdk" || request.ToolName != "Write" {
		t.Fatalf("SDK permission identity = %+v", request)
	}
	if len(request.Suggestions) != 1 || request.Suggestions[0].Type != "addRules" || len(request.Suggestions[0].Rules) != 1 || request.Suggestions[0].Rules[0].RuleContent != "/tmp/**" {
		t.Fatalf("SDK permission suggestions = %+v", request.Suggestions)
	}
}

func TestSDKPermissionAdapterRejectsUnknownDecision(t *testing.T) {
	adapter := appSDKPermissionAdapter{handler: sdkPermissionHandlerFunc(func(context.Context, sdk.PermissionRequest) (sdk.PermissionDecision, error) {
		return sdk.PermissionDecision(99), nil
	})}
	decision, err := adapter.Check(context.Background(), permission.PermissionRequest{ToolName: "Bash"})
	if err != nil {
		t.Fatal(err)
	}
	if decision != permission.PermissionDeny {
		t.Fatalf("unknown SDK permission decision = %v, want deny", decision)
	}
}

type sdkPermissionHandlerFunc func(context.Context, sdk.PermissionRequest) (sdk.PermissionDecision, error)

func (fn sdkPermissionHandlerFunc) Check(ctx context.Context, request sdk.PermissionRequest) (sdk.PermissionDecision, error) {
	return fn(ctx, request)
}

func TestSDKRuntimeErrorMapsEngineSentinelsToSemanticCopy(t *testing.T) {
	err := sdkRuntimeError(engine.ErrSessionNotFound)
	if !errors.Is(err, engine.ErrSessionNotFound) {
		t.Fatalf("mapped error lost engine cause: %v", err)
	}
	localized, ok := err.(interface{ Localized(i18n.Language) string })
	if !ok || localized.Localized(i18n.LangZH) != i18n.Text(i18n.LangZH, i18n.KeyAuxEngineSessionNotFound) {
		t.Fatalf("mapped error is not semantic: %T %v", err, err)
	}
}

func TestSDKRequestStatusAdapterMapsEveryPhaseWithoutRawError(t *testing.T) {
	const secret = "upstream authorization=private-token"
	tests := []struct {
		eventType stream.EventType
		status    string
	}{
		{stream.EventRequestStart, "started"},
		{stream.EventRequestRetry, "retrying"},
		{stream.EventRequestFirstToken, "streaming"},
		{stream.EventRequestEnd, "completed"},
		{stream.EventRequestFailed, "failed"},
	}
	for _, test := range tests {
		t.Run(string(test.eventType), func(t *testing.T) {
			event := sdkEventFromStream(stream.Event{
				Type: test.eventType,
				RequestStatus: &stream.RequestStatusEvent{
					RequestID: "request-9", StartedAt: "2026-07-26T00:00:00Z", EndedAt: "2026-07-26T00:00:01Z",
					Attempt: 2, MaxRetries: 3, RetryCount: 1,
					RetryDelayMilliseconds: 250, RetryKind: "stream", RequestMilliseconds: 40,
					FirstTokenMilliseconds: 80, TotalMilliseconds: 120,
					InputTokens: 100, CacheReadInputTokens: 70, CacheWriteInputTokens: 10, OutputTokens: 20,
					Error: secret,
				},
			})
			status := event.RequestStatus
			if status == nil || status.RequestID != "request-9" || status.Phase != string(test.eventType) || status.Status != test.status || status.Attempt != 2 || status.MaxAttempts != 4 || status.RetryCount != 1 || status.RetryKind != "stream" {
				t.Fatalf("request status = %+v", status)
			}
			if status.StartedAt == "" || status.EndedAt == "" || status.InputTokens != 100 || status.CacheReadInputTokens != 70 || status.CacheWriteInputTokens != 10 || status.OutputTokens != 20 {
				t.Fatalf("request status lost timing/usage = %+v", status)
			}
			if test.eventType == stream.EventRequestRetry && status.ErrorCode != "provider_request_retry" {
				t.Fatalf("retry error projection = %+v", status)
			}
			if test.eventType == stream.EventRequestFailed && status.ErrorCode != "provider_request_failed" {
				t.Fatalf("failed error projection = %+v", status)
			}
			encoded, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), "private-token") {
				t.Fatalf("SDK event leaked raw request error: %s", encoded)
			}
		})
	}
}

func TestSDKToolRoundMetricsAdapterIsLossless(t *testing.T) {
	event := sdkEventFromStream(stream.Event{
		Type: stream.EventToolRoundMetrics,
		ToolRound: &stream.ToolRoundMetricsEvent{
			RoundID: "turn-3", LogicalModelVisibleCalls: 5,
			PhysicalChildOperations: 7, Fanout: 3, BatchCount: 2,
			QueueMilliseconds: 12, CriticalPathMilliseconds: 40,
			TotalChildLatencyMilliseconds: 70, ErrorCount: 1,
			RevisionFusionCount: 2, RevisionBarrierSkips: 1, RevisionMismatchCount: 1,
		},
	})
	if event.Type != sdk.EventToolRoundMetrics || event.ToolRound == nil {
		t.Fatalf("SDK tool round event = %+v", event)
	}
	if event.ToolRound.RoundID != "turn-3" || event.ToolRound.LogicalModelVisibleCalls != 5 || event.ToolRound.PhysicalChildOperations != 7 || event.ToolRound.Fanout != 3 || event.ToolRound.ErrorCount != 1 || event.ToolRound.RevisionFusionCount != 2 || event.ToolRound.RevisionBarrierSkips != 1 || event.ToolRound.RevisionMismatchCount != 1 {
		t.Fatalf("SDK tool round metrics = %+v", event.ToolRound)
	}
}

func TestSDKFlightProgressAdapterPreservesContentFreeDisposition(t *testing.T) {
	event := sdkEventFromStream(stream.Event{
		Type: stream.EventProgress,
		Progress: &stream.ProgressEvent{
			Stage: "agentic_flight", Disposition: "incomplete_unverified", Blocker: "verification_missing",
			MutationEpoch: 5, VerifiedEpoch: 4,
		},
	})
	if event.Progress == nil || event.Progress.Stage != "agentic_flight" ||
		event.Progress.Disposition != "incomplete_unverified" || event.Progress.Blocker != "verification_missing" ||
		event.Progress.MutationEpoch != 5 || event.Progress.VerifiedEpoch != 4 {
		t.Fatalf("SDK flight progress = %+v", event.Progress)
	}
}

func TestSDKCompactBoundaryAdapterOwnsNestedDTO(t *testing.T) {
	converted := sdkCompactBoundary(&stream.CompactBoundaryEvent{
		Trigger: "manual",
		PreservedSegment: &stream.PreservedSegmentMetadata{
			StartIndex: 3,
			Count:      7,
			Anchor:     "assistant",
			Direction:  "tail",
		},
	})
	if converted == nil || converted.PreservedSegment == nil {
		t.Fatalf("compact boundary = %+v", converted)
	}
	if converted.PreservedSegment.StartIndex != 3 || converted.PreservedSegment.Count != 7 ||
		converted.PreservedSegment.Anchor != "assistant" || converted.PreservedSegment.Direction != "tail" {
		t.Fatalf("preserved segment = %+v", converted.PreservedSegment)
	}
}
