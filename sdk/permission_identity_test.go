package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/agent-dance/luban/engine"
)

func TestSDKPermissionChallengePreservesDecisionAuditFields(t *testing.T) {
	bridge := newPermissionBridge()
	var challenge PermissionRequestMsg
	handler := &SDKPermissionHandler{
		bridge:   bridge,
		newReqID: func() string { return "transport-request" },
		sendFn: func(msg any) error {
			out, ok := msg.(SDKControlRequestOut)
			if !ok {
				t.Fatalf("challenge = %T, want SDKControlRequestOut", msg)
			}
			if err := json.Unmarshal(out.Request, &challenge); err != nil {
				t.Fatalf("unmarshal challenge: %v", err)
			}
			bridge.deliver(out.RequestID, permissionResult{behavior: "deny"})
			return nil
		},
	}
	request := engine.PermissionRequest{
		SessionID: "session-decision", ExecutionSessionID: "agent-session", TurnID: "turn-2", DecisionID: "decision-2",
		ToolUseID: "toolu-2", ToolName: "Write", Input: map[string]any{"file_path": "/protected"},
		ActorID: "agent-2", ActorType: "security-reviewer", WorkUnitID: "review-2", Kind: "permission",
		Action: "write file", Target: "/protected", Impact: "modifies protected data", RiskReason: "protected path",
		RuleSource: "mandatory approval policy", ApprovalScope: "single tool call", Choices: []string{"allow_once", "reject"},
		Body: "full review body", ReviewDetails: []string{"path: /protected"}, PostMode: "default", Description: "write protected file",
		Mode: "default", AvoidPrompts: true, Message: "approval required", BlockedPath: "/protected",
	}
	if _, err := handler.Check(context.Background(), request); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if challenge.SessionID != request.SessionID || challenge.ExecutionSessionID != request.ExecutionSessionID || challenge.TurnID != request.TurnID || challenge.DecisionID != request.DecisionID {
		t.Fatalf("session/turn/decision identity = %+v", challenge)
	}
	if challenge.RequestID != "transport-request" {
		t.Fatalf("inner request ID = %q, want transport-request", challenge.RequestID)
	}
	if challenge.ToolUseID != request.ToolUseID || challenge.ActorID != request.ActorID || challenge.ActorType != request.ActorType || challenge.WorkUnitID != request.WorkUnitID {
		t.Fatalf("tool/actor/work identity = %+v", challenge)
	}
	if challenge.Action != request.Action || challenge.Target != request.Target || challenge.Impact != request.Impact || challenge.RiskReason != request.RiskReason || challenge.RuleSource != request.RuleSource || challenge.ApprovalScope != request.ApprovalScope {
		t.Fatalf("decision review details = %+v", challenge)
	}
	if len(challenge.Choices) != 2 || challenge.Body != request.Body || len(challenge.ReviewDetails) != 1 || challenge.PostMode != request.PostMode {
		t.Fatalf("decision body/choices = %+v", challenge)
	}
}

func TestSDKPermissionInterruptionRemovesPendingCorrelation(t *testing.T) {
	tests := []struct {
		name    string
		context func() (context.Context, context.CancelFunc)
		wantErr error
	}{
		{
			name: "cancelled",
			context: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, cancel
			},
			wantErr: context.Canceled,
		},
		{
			name: "timed out",
			context: func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			},
			wantErr: context.DeadlineExceeded,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bridge := newPermissionBridge()
			handler := &SDKPermissionHandler{
				bridge:   bridge,
				newReqID: func() string { return "interrupted-request" },
				sendFn:   func(any) error { return nil },
			}
			ctx, cancel := test.context()
			defer cancel()
			_, err := handler.Check(ctx, engine.PermissionRequest{ToolUseID: "toolu-interrupted", ToolName: "Write"})
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Check error = %v, want %v", err, test.wantErr)
			}

			bridge.mu.Lock()
			pending := len(bridge.pending)
			bridge.mu.Unlock()
			if pending != 0 {
				t.Fatalf("pending correlations = %d, want 0", pending)
			}
			if bridge.deliver("interrupted-request", permissionResult{behavior: "allow"}) {
				t.Fatal("late response was delivered after permission interruption")
			}
		})
	}
}
