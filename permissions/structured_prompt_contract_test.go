package permissions_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/permissions"
)

func TestCLIPermissionHandlerForwardsCompleteStructuredPrompt(t *testing.T) {
	installNoopSafetyChecks(t)
	checker := permissions.NewChecker(permissions.ModeAskAlways, nil)
	var got permissions.PromptRequest
	checker.SetStructuredPromptFunc(func(_ context.Context, req permissions.PromptRequest) permissions.PromptResponse {
		got = req
		return permissions.PromptResponse{
			Decision: permissions.DecisionAllowOnce,
			Outcome:  permissions.PromptOutcomeApproved,
			Choice:   "allow_once",
		}
	})
	handler := permissions.NewCLIPermissionHandler(checker)
	request := permission.PermissionRequest{
		SessionID:          "session-42",
		ExecutionSessionID: "agent-session",
		DecisionID:         "decision:session-42:toolu-7",
		ToolUseID:          "toolu-7",
		ToolName:           "Write",
		Input:              map[string]any{"file_path": "/workspace/out.txt"},
		ActorID:            "agent-writer",
		ActorType:          "executor",
		WorkUnitID:         "work-output",
		Kind:               "permission",
		Action:             "Write a file",
		Target:             "/workspace/out.txt",
		Impact:             "Replaces the current file contents",
		RiskReason:         "Existing data can be overwritten",
		RuleSource:         "project policy: protected outputs",
		ApprovalScope:      "this invocation",
		Choices:            []string{"allow_once", "reject", "always_allow"},
		Body:               "full review body",
		ReviewDetails:      []string{"Allowed prompts: Bash(run tests)"},
		PostMode:           "acceptEdits",
	}

	decision, err := handler.Check(context.Background(), request)
	if err != nil {
		t.Fatalf("handler.Check: %v", err)
	}
	if decision != permission.PermissionAllowOnce {
		t.Fatalf("execution decision = %v, want allow once", decision)
	}
	if got.DecisionID != request.DecisionID || got.SessionID != request.SessionID || got.ExecutionSessionID != request.ExecutionSessionID || got.ToolUseID != request.ToolUseID {
		t.Fatalf("prompt lost stable decision identity: %+v", got)
	}
	if got.ActorID != request.ActorID || got.ActorType != request.ActorType || got.WorkUnitID != request.WorkUnitID {
		t.Fatalf("prompt lost actor/work-unit identity: %+v", got)
	}
	if got.Action != request.Action || got.Target != request.Target || got.Impact != request.Impact || got.RiskReason != request.RiskReason {
		t.Fatalf("prompt lost action/impact/risk detail: %+v", got)
	}
	if got.RuleSource != request.RuleSource || got.ApprovalScope != request.ApprovalScope {
		t.Fatalf("prompt lost rule source or approval scope: %+v", got)
	}
	if !reflect.DeepEqual(got.Choices, request.Choices) || got.Body != request.Body || !reflect.DeepEqual(got.ReviewDetails, request.ReviewDetails) || got.PostMode != request.PostMode {
		t.Fatalf("prompt lost choices or body: %+v", got)
	}
	request.ReviewDetails[0] = "mutated-after-check"
	if got.ReviewDetails[0] == request.ReviewDetails[0] {
		t.Fatal("prompt retained the engine request's mutable review-details slice")
	}
}

func TestPlanPromptKeepsFullBodyAndExecuteStayChoices(t *testing.T) {
	installNoopSafetyChecks(t)
	checker := permissions.NewChecker(permissions.ModeAskAlways, nil)
	plan := "# Plan\n" + strings.Repeat("- verify every implementation invariant\n", 512) + "END OF PLAN"
	var got permissions.PromptRequest
	checker.SetStructuredPromptFunc(func(_ context.Context, req permissions.PromptRequest) permissions.PromptResponse {
		got = req
		return permissions.PromptResponse{
			Decision: permissions.DecisionDeny,
			Outcome:  permissions.PromptOutcomeRejected,
			Choice:   "stay_in_plan",
		}
	})
	handler := permissions.NewCLIPermissionHandler(checker)

	decision, err := handler.Check(context.Background(), permission.PermissionRequest{
		SessionID:     "session-plan",
		DecisionID:    "decision:session-plan:toolu-plan",
		ToolUseID:     "toolu-plan",
		ToolName:      "ExitPlanMode",
		Kind:          "plan",
		Action:        "Execute the approved plan",
		Target:        "/workspace/PLAN.md",
		Impact:        "Leave plan mode and begin implementation",
		RiskReason:    "Implementation can modify the workspace and run commands",
		RuleSource:    "plan mode gate",
		ApprovalScope: "this plan transition",
		Choices:       []string{"execute", "stay_in_plan"},
		Body:          plan,
		Input:         map[string]any{"plan": plan},
	})
	if err != nil {
		t.Fatalf("handler.Check: %v", err)
	}
	if decision != permission.PermissionDeny {
		t.Fatalf("stay in plan decision = %v, want deny execution", decision)
	}
	if got.Kind != permissions.PromptKindPlan || got.Body != plan {
		t.Fatalf("plan prompt lost kind or full body: kind=%q body-bytes=%d want=%d", got.Kind, len(got.Body), len(plan))
	}
	if want := []string{"execute", "stay_in_plan"}; !reflect.DeepEqual(got.Choices, want) {
		t.Fatalf("plan choices = %v, want %v", got.Choices, want)
	}
}

func TestPromptOutcomesRemainDistinct(t *testing.T) {
	outcomes := []permissions.PromptOutcome{
		permissions.PromptOutcomeApproved,
		permissions.PromptOutcomeRejected,
		permissions.PromptOutcomeEscaped,
		permissions.PromptOutcomeCancelled,
		permissions.PromptOutcomeTimedOut,
		permissions.PromptOutcomeShutdown,
	}
	seen := make(map[permissions.PromptOutcome]bool, len(outcomes))
	for _, outcome := range outcomes {
		if outcome == "" || seen[outcome] {
			t.Fatalf("prompt outcome %q is empty or aliases another result", outcome)
		}
		seen[outcome] = true
	}
}

func TestContextCancellationIsNotReportedAsExplicitRejection(t *testing.T) {
	installNoopSafetyChecks(t)
	checker := permissions.NewChecker(permissions.ModeAskAlways, nil)
	checker.SetStructuredPromptFunc(func(ctx context.Context, _ permissions.PromptRequest) permissions.PromptResponse {
		<-ctx.Done()
		outcome := permissions.PromptOutcomeCancelled
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			outcome = permissions.PromptOutcomeTimedOut
		}
		return permissions.PromptResponse{Decision: permissions.DecisionDeny, Outcome: outcome}
	})
	handler := permissions.NewCLIPermissionHandler(checker)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	decision, err := handler.Check(ctx, permission.PermissionRequest{
		DecisionID: "decision-cancelled",
		ToolName:   "Lifecycle",
		Input:      map[string]any{"value": "x"},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled prompt error = %v, want context.Canceled (decision=%v)", err, decision)
	}

	deadlineCtx, deadlineCancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer deadlineCancel()
	<-deadlineCtx.Done()
	decision, err = handler.Check(deadlineCtx, permission.PermissionRequest{
		DecisionID: "decision-timeout",
		ToolName:   "Lifecycle",
		Input:      map[string]any{"value": "x"},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timed-out prompt error = %v, want context.DeadlineExceeded (decision=%v)", err, decision)
	}

	rejectingChecker := permissions.NewChecker(permissions.ModeAskAlways, nil)
	rejectingChecker.SetStructuredPromptFunc(func(_ context.Context, _ permissions.PromptRequest) permissions.PromptResponse {
		return permissions.PromptResponse{Decision: permissions.DecisionDeny, Outcome: permissions.PromptOutcomeRejected}
	})
	decision, err = permissions.NewCLIPermissionHandler(rejectingChecker).Check(context.Background(), permission.PermissionRequest{
		DecisionID: "decision-rejected",
		ToolName:   "Lifecycle",
		Input:      map[string]any{"value": "x"},
	})
	if err != nil || decision != permission.PermissionDeny {
		t.Fatalf("explicit rejection = (%v, %v), want (PermissionDeny, nil)", decision, err)
	}
}
