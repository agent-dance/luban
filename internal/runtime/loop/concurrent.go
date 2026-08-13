package loop

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/approvalcommit"
	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

// isConcurrentSafe checks if a tool can be safely run in parallel
func isConcurrentSafe(reg *registry.Registry, toolName string, input map[string]any) bool {
	if reg == nil || reg.Get(toolName) == nil {
		return false
	}
	return reg.ToolMetadata(toolName, input).ConcurrencySafe
}

// executeToolsConcurrently runs concurrent-safe tools in parallel and
// non-concurrent tools sequentially, preserving the original order.
// The context is passed to each tool execution so cancellation propagates.
// Infrastructure errors (e.g. context cancellation) are returned via the error
// return value to abort the loop, rather than being fed to the LLM.
// runPreToolHooks runs PreToolUse hooks for a single tool use.
// Returns (blocked, modifiedInput, systemReminders).
// modifiedInput is nil if no hook modified the input.
func runPreToolHooks(ctx context.Context, runner *hooks.Runner, tu types.ToolUseBlock) (blocked bool, modifiedInput map[string]any, reminders []string, preventContinuation bool, permissionBehavior string) {
	if runner == nil {
		return false, nil, nil, false, ""
	}
	executions := runner.RunDetailed(ctx, hooks.HookPreToolUse, correlatedHookInput(ctx, tu, tu.Input, ""))
	recordToolHookExecutions(ctx, hooks.HookPreToolUse, executions)
	outputs := hookExecutionOutputs(executions)
	for _, out := range outputs {
		if out.Block {
			blocked = true
		}
		if out.ModifiedInput != nil {
			modifiedInput = out.Snapshot().ModifiedInput
		}
		if out.SystemReminder != "" {
			reminders = append(reminders, out.SystemReminder)
		}
		if out.AdditionalContext != "" {
			reminders = append(reminders, out.AdditionalContext)
		}
		reminders = append(reminders, out.AdditionalContexts...)
		if out.PreventContinuation {
			preventContinuation = true
			if out.StopReason != "" {
				reminders = append(reminders, out.StopReason)
			}
		}
		if out.PermissionBehavior != "" {
			permissionBehavior = out.PermissionBehavior
		}
	}
	return blocked, modifiedInput, reminders, preventContinuation, permissionBehavior
}

// runPostToolHooks runs PostToolUse hooks for a single tool use.
// Returns any system reminders produced.
func runPostToolHooks(ctx context.Context, runner *hooks.Runner, hookType hooks.HookType, tu types.ToolUseBlock, input map[string]any, resultContent string) ([]string, bool) {
	if runner == nil {
		return nil, false
	}
	executions := runner.RunDetailed(ctx, hookType, correlatedHookInput(ctx, tu, input, resultContent))
	recordToolHookExecutions(ctx, hookType, executions)
	outputs := hookExecutionOutputs(executions)
	var reminders []string
	preventContinuation := false
	for _, out := range outputs {
		if out.SystemReminder != "" {
			reminders = append(reminders, out.SystemReminder)
		}
		if out.AdditionalContext != "" {
			reminders = append(reminders, out.AdditionalContext)
		}
		reminders = append(reminders, out.AdditionalContexts...)
		if out.PreventContinuation {
			preventContinuation = true
			if out.StopReason != "" {
				reminders = append(reminders, out.StopReason)
			}
		}
	}
	return reminders, preventContinuation
}

func correlatedHookInput(ctx context.Context, tool types.ToolUseBlock, input map[string]any, result string) hooks.HookInput {
	hookInput := hooks.HookInput{ToolName: tool.Name, ToolUseID: tool.ID, ToolInput: input, Result: result}
	if exec, ok := executioncontract.ToolExecutionContextFromContext(ctx); ok {
		hookInput.SessionID = exec.SessionID
		hookInput.AgentID = exec.ActorID
		hookInput.AgentType = exec.ActorType
		hookInput.TurnID = exec.TurnID
		hookInput.WorkUnitID = exec.WorkUnitID
	}
	return hookInput
}

// buildPermissionDeniedMessage constructs the tool result content for a denied permission check.
func buildPermissionDeniedMessage(err error, toolName string) string {
	lang := i18n.DetectOrLoadLanguage()
	if err != nil {
		return i18n.Format(lang, i18n.KeyRuntimePermissionCheckFailed, toolName, err)
	}
	return i18n.Format(lang, i18n.KeyRuntimePermissionDenied, toolName)
}

type toolExecutionResults struct {
	Results             []types.ToolResultBlock
	Reminders           []string
	PreventContinuation bool
	HookSummaries       []stream.HookSummaryEvent
	Metrics             toolExecutionMetrics
}

// toolExecutionMetrics remains internal until query.go projects it into the
// content-free stream.ToolRoundMetricsEvent envelope.
type toolExecutionMetrics struct {
	PhysicalChildOperations int
	PeakFanout              int
	BatchCount              int
	QueueDuration           time.Duration
	CriticalPathDuration    time.Duration
	TotalChildLatency       time.Duration
	ErrorCount              int
	RevisionFusionCount     int
	RevisionBarrierSkips    int
	RevisionMismatchCount   int
}

func countToolResultErrors(results []types.ToolResultBlock) int {
	errors := 0
	for _, result := range results {
		switch result.Outcome {
		case types.ToolOutcomeFailed, types.ToolOutcomeDenied, types.ToolOutcomeCancelled, types.ToolOutcomeTimedOut:
			errors++
		default:
			if result.IsError {
				errors++
			}
		}
	}
	return errors
}

type toolHookCollectorKey struct{}

type toolHookCollector struct {
	mu        sync.Mutex
	summaries []stream.HookSummaryEvent
}

func (c *toolHookCollector) add(summaries ...stream.HookSummaryEvent) {
	if c == nil || len(summaries) == 0 {
		return
	}
	c.mu.Lock()
	c.summaries = append(c.summaries, summaries...)
	c.mu.Unlock()
}

func (c *toolHookCollector) drain() []stream.HookSummaryEvent {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	out := append([]stream.HookSummaryEvent(nil), c.summaries...)
	c.summaries = nil
	c.mu.Unlock()
	return out
}

func withToolHookCollector(ctx context.Context, collector *toolHookCollector) context.Context {
	return context.WithValue(ctx, toolHookCollectorKey{}, collector)
}

func recordToolHookExecutions(ctx context.Context, hookType hooks.HookType, executions []hooks.HookExecution) {
	collector, _ := ctx.Value(toolHookCollectorKey{}).(*toolHookCollector)
	if collector == nil {
		return
	}
	for _, execution := range executions {
		collector.add(newHookExecutionSummary(hookType, execution.Snapshot(), localizedToolHookSummaryDefaults()))
	}
}

type singleToolExecutionResult struct {
	Result              types.ToolResultBlock
	Reminders           []string
	PreventContinuation bool
}

func maxToolUseConcurrency() int {
	const defaultMaxToolUseConcurrency = 10
	raw := os.Getenv("LUBAN_CODE_MAX_TOOL_USE_CONCURRENCY")
	if raw == "" {
		return defaultMaxToolUseConcurrency
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultMaxToolUseConcurrency
	}
	return n
}

func cancelledToolResult(tu types.ToolUseBlock) types.ToolResultBlock {
	return types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: tu.ID,
		Content:   i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeToolExecutionCancelled),
		IsError:   true,
		Outcome:   types.ToolOutcomeCancelled,
	}
}

func executeOneTool(ctx context.Context, reg *registry.Registry, runner *hooks.Runner, permHandler permission.PermissionHandler, sessionID string, execContext executioncontract.ToolExecutionContext, tu types.ToolUseBlock) (singleToolExecutionResult, error) {
	if ctx.Err() != nil {
		return singleToolExecutionResult{Result: cancelledToolResult(tu)}, ctx.Err()
	}

	var reminders []string
	toolPreventContinuation := false
	execForTool := execContext
	if execForTool.SessionID == "" {
		execForTool.SessionID = sessionID
	}
	if execForTool.HasRuntimeOwner() && (!execForTool.IsRuntimeOwned() || !execForTool.RuntimeIdentityMatches()) {
		return singleToolExecutionResult{Result: types.ToolResultBlock{
			Type: types.ContentTypeToolResult, ToolUseID: tu.ID,
			Content: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeToolPermissionRequired, tu.Name),
			IsError: true, Outcome: types.ToolOutcomeDenied,
		}}, nil
	}
	execForTool.ToolUse = tu
	toolContext := executioncontract.WithToolExecutionContext(ctx, execForTool)
	// The provider-committed input must satisfy the authoritative local contract
	// before any normalizer or hook can observe it. Hooks that intentionally
	// modify input are validated again below before permission or execution.
	if tool := reg.Get(tu.Name); tool != nil {
		if validationErr := types.ValidateToolInput(tool, tu.Input); validationErr != nil {
			return singleToolExecutionResult{Result: types.ToolResultBlock{
				Type: types.ContentTypeToolResult, ToolUseID: tu.ID,
				Content: i18n.FormatToolInputValidationError(i18n.DetectOrLoadLanguage(), validationErr),
				IsError: true, Outcome: types.ToolOutcomeFailed,
			}}, nil
		}
	}
	if normalizer, ok := reg.Get(tu.Name).(interface {
		NormalizeToolInput(context.Context, map[string]any) (map[string]any, error)
	}); ok {
		normalized, err := normalizer.NormalizeToolInput(toolContext, tu.Input)
		if err != nil {
			return singleToolExecutionResult{Result: types.ToolResultBlock{
				Type: types.ContentTypeToolResult, ToolUseID: tu.ID,
				Content: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeToolInputNormalization, err), IsError: true,
				Outcome: types.ToolOutcomeFailed,
			}}, nil
		}
		if normalized != nil {
			tu.Input = normalized
			execForTool.ToolUse = tu
			toolContext = executioncontract.WithToolExecutionContext(ctx, execForTool)
		}
	}

	blocked, modifiedInput, preReminders, prePrevent, permissionBehavior := runPreToolHooks(toolContext, runner, tu)
	reminders = append(reminders, preReminders...)
	toolPreventContinuation = toolPreventContinuation || prePrevent
	switch strings.ToLower(permissionBehavior) {
	case "deny", "block":
		blocked = true
	}
	if blocked {
		result := types.ToolResultBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: tu.ID,
			Content:   i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeToolBlockedByHook),
			IsError:   true,
			Outcome:   types.ToolOutcomeDenied,
		}
		failureReminders, failurePrevent := runPostToolHooks(toolContext, runner, hooks.HookPostToolUseFailure, tu, tu.Input, result.TextContent())
		reminders = append(reminders, failureReminders...)
		toolPreventContinuation = toolPreventContinuation || failurePrevent
		return singleToolExecutionResult{Result: result, Reminders: reminders, PreventContinuation: toolPreventContinuation}, nil
	}

	input := tu.Input
	if modifiedInput != nil {
		input = modifiedInput
	}
	execForTool.ToolUse = tu
	execForTool.ToolUse.Input = input
	toolContext = executioncontract.WithToolExecutionContext(ctx, execForTool)
	if tool := reg.Get(tu.Name); tool != nil {
		if validationErr := types.ValidateToolInput(tool, input); validationErr != nil {
			result := types.ToolResultBlock{
				Type:      types.ContentTypeToolResult,
				ToolUseID: tu.ID,
				Content:   i18n.FormatToolInputValidationError(i18n.DetectOrLoadLanguage(), validationErr),
				IsError:   true,
				Outcome:   types.ToolOutcomeFailed,
			}
			failureReminders, failurePrevent := runPostToolHooks(toolContext, runner, hooks.HookPostToolUseFailure, tu, input, result.TextContent())
			reminders = append(reminders, failureReminders...)
			toolPreventContinuation = toolPreventContinuation || failurePrevent
			return singleToolExecutionResult{Result: result, Reminders: reminders, PreventContinuation: toolPreventContinuation}, nil
		}
	}

	approvalSessionID := sessionID
	if execForTool.HasRuntimeOwner() {
		if ownerSessionID, _, _, _, ok := execForTool.ActiveRuntimeOwnerIdentity(); ok {
			approvalSessionID = ownerSessionID
		}
	}
	if approvalSessionID == "" {
		approvalSessionID = execForTool.SessionID
	}
	if approvalSessionID == "" {
		approvalSessionID = "ephemeral:" + tu.ID
	}
	approvalTurnID := execForTool.TurnID
	if approvalTurnID == "" {
		approvalTurnID = "turn:" + tu.ID
	}
	approvalEpoch := execForTool.ApprovalEpoch()
	if approvalEpoch == "" {
		approvalEpoch = "dispatch:" + approvalSessionID + ":" + approvalTurnID + ":" + tu.ID
	}
	permissionRequest := types.ToolPermissionRequest{
		SessionID: approvalSessionID, TurnID: approvalTurnID, ToolUseID: tu.ID,
		ApprovalEpoch: approvalEpoch,
	}
	toolPermission, permissionErr := reg.CheckToolPermissions(toolContext, tu.Name, input, permissionRequest)
	if permissionErr != nil {
		outcome := types.ToolOutcomeDenied
		if errors.Is(permissionErr, context.DeadlineExceeded) {
			outcome = types.ToolOutcomeTimedOut
		} else if errors.Is(permissionErr, context.Canceled) {
			outcome = types.ToolOutcomeCancelled
		}
		result := types.ToolResultBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: tu.ID,
			Content:   buildPermissionDeniedMessage(permissionErr, tu.Name),
			IsError:   true,
			Outcome:   outcome,
		}
		failureReminders, failurePrevent := runPostToolHooks(toolContext, runner, hooks.HookPostToolUseFailure, tu, input, result.TextContent())
		reminders = append(reminders, failureReminders...)
		toolPreventContinuation = toolPreventContinuation || failurePrevent
		return singleToolExecutionResult{Result: result, Reminders: reminders, PreventContinuation: toolPreventContinuation}, nil
	}
	if toolPermission.UpdatedInput != nil {
		input = toolPermission.UpdatedInput
		execForTool.ToolUse = tu
		execForTool.ToolUse.Input = input
		toolContext = executioncontract.WithToolExecutionContext(ctx, execForTool)
	}
	approvedInput := hooks.HookInput{ToolInput: input}.Snapshot().ToolInput
	approvedPermission := toolPermission
	defer func() {
		reg.RevokePermissionGrant(toolPermission.PermissionGrant)
	}()
	if toolPermission.Behavior == types.PermissionBehaviorDeny {
		message := toolPermission.Message
		if message == "" {
			message = buildPermissionDeniedMessage(nil, tu.Name)
		}
		result := types.ToolResultBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: tu.ID,
			Content:   message,
			IsError:   true,
			Outcome:   types.ToolOutcomeDenied,
		}
		attachToolPolicyDecision(&result, toolPermission.PolicyDecision)
		failureReminders, failurePrevent := runPostToolHooks(toolContext, runner, hooks.HookPostToolUseFailure, tu, input, result.TextContent())
		reminders = append(reminders, failureReminders...)
		toolPreventContinuation = toolPreventContinuation || failurePrevent
		return singleToolExecutionResult{Result: result, Reminders: reminders, PreventContinuation: toolPreventContinuation}, nil
	}

	hookAllowed := strings.EqualFold(permissionBehavior, "allow")
	toolAllowed := toolPermission.Behavior == types.PermissionBehaviorAllow
	needsPrompt := toolPermission.Behavior == types.PermissionBehaviorAsk
	requiresDefaultHandler := false
	if toolPermission.Behavior == types.PermissionBehaviorPassthrough && tu.Name == "Bash" {
		_, requiresDefaultHandler = reg.Get(tu.Name).(types.ToolPermissionChecker)
	}
	forceHandler := false
	if hookAllowed && !toolAllowed {
		if mandatory, ok := permHandler.(interface{ CheckHookGrantedPermissions() bool }); ok {
			forceHandler = mandatory.CheckHookGrantedPermissions()
		}
	}
	shouldUseHandler := forceHandler || (!toolAllowed && (!hookAllowed || toolPermission.Required))
	if (needsPrompt || requiresDefaultHandler) && shouldUseHandler && permHandler == nil {
		message := toolPermission.Message
		if message == "" {
			message = i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeToolPermissionRequired, tu.Name)
		}
		result := types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: tu.ID, Content: message, IsError: true, Outcome: types.ToolOutcomeDenied}
		attachToolPolicyDecision(&result, toolPermission.PolicyDecision)
		failureReminders, failurePrevent := runPostToolHooks(toolContext, runner, hooks.HookPostToolUseFailure, tu, input, result.TextContent())
		reminders = append(reminders, failureReminders...)
		toolPreventContinuation = toolPreventContinuation || failurePrevent
		return singleToolExecutionResult{Result: result, Reminders: reminders, PreventContinuation: toolPreventContinuation}, nil
	}

	if permHandler != nil && shouldUseHandler {
		permissionToolUse := tu
		permissionToolUse.Input = input
		decision, permErr := permHandler.Check(toolContext, buildPermissionRequest(sessionID, execForTool, permissionToolUse, toolPermission))
		if permErr != nil || decision == permission.PermissionDeny {
			message := buildPermissionDeniedMessage(permErr, tu.Name)
			if permErr == nil && toolPermission.Message != "" {
				message = toolPermission.Message
			}
			outcome := types.ToolOutcomeDenied
			if errors.Is(permErr, context.Canceled) {
				outcome = types.ToolOutcomeCancelled
			} else if errors.Is(permErr, context.DeadlineExceeded) {
				outcome = types.ToolOutcomeTimedOut
			} else if permErr != nil {
				outcome = types.ToolOutcomeFailed
			}
			result := types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: tu.ID, Content: message, IsError: true, Outcome: outcome}
			if mapper, ok := reg.Get(tu.Name).(types.ToolPermissionRejectionMapper); ok && permErr == nil {
				result = mapper.MapToolPermissionRejection(input, tu.ID, message)
				result.Outcome = types.ToolOutcomeDenied
			}
			attachToolPolicyDecision(&result, toolPermission.PolicyDecision)
			failureReminders, failurePrevent := runPostToolHooks(toolContext, runner, hooks.HookPostToolUseFailure, tu, input, result.TextContent())
			reminders = append(reminders, failureReminders...)
			toolPreventContinuation = toolPreventContinuation || failurePrevent
			return singleToolExecutionResult{Result: result, Reminders: reminders, PreventContinuation: toolPreventContinuation}, nil
		}
	}

	// Detach permission-form input before binding the commit. A handler may keep
	// its request map after returning; later mutation must fail to affect the
	// exact input digest that Registry validates and executes.
	input = hooks.HookInput{ToolInput: input}.Snapshot().ToolInput
	if !reflect.DeepEqual(approvedInput, input) {
		revalidationExec := execForTool
		revalidationExec.ToolUse = tu
		revalidationExec.ToolUse.Input = input
		revalidationContext := executioncontract.WithToolExecutionContext(ctx, revalidationExec)
		finalPermission, finalErr := reg.CheckToolPermissions(revalidationContext, tu.Name, input, permissionRequest)
		if finalErr != nil {
			return singleToolExecutionResult{}, finalErr
		}
		if finalPermission.UpdatedInput != nil && !reflect.DeepEqual(finalPermission.UpdatedInput, input) {
			reg.RevokePermissionGrant(finalPermission.PermissionGrant)
			message := finalPermission.Message
			if message == "" {
				message = i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeToolPermissionRequired, tu.Name)
			}
			result := types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: tu.ID, Content: message, IsError: true, Outcome: types.ToolOutcomeDenied}
			attachToolPolicyDecision(&result, finalPermission.PolicyDecision)
			return singleToolExecutionResult{Result: result, Reminders: reminders, PreventContinuation: toolPreventContinuation}, nil
		}
		if tu.Name == "Bash" || permissionRevalidationIsStronger(approvedPermission, finalPermission) {
			reg.RevokePermissionGrant(finalPermission.PermissionGrant)
			message := finalPermission.Message
			if message == "" {
				message = i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeToolPermissionRequired, tu.Name)
			}
			result := types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: tu.ID, Content: message, IsError: true, Outcome: types.ToolOutcomeDenied}
			attachToolPolicyDecision(&result, finalPermission.PolicyDecision)
			return singleToolExecutionResult{Result: result, Reminders: reminders, PreventContinuation: toolPreventContinuation}, nil
		}
		reg.RevokePermissionGrant(toolPermission.PermissionGrant)
		toolPermission = finalPermission
	}
	execForTool.ToolUse = tu
	execForTool.ToolUse.Input = input
	policyCode := toolPermission.ExecutionPolicyCode
	if toolPermission.PolicyDecision != nil {
		if policyCode == "" {
			policyCode = toolPermission.PolicyDecision.Code
		}
	}
	if ctx.Err() != nil {
		return singleToolExecutionResult{Result: cancelledToolResult(tu)}, ctx.Err()
	}
	binding := toolPermission.PermissionBinding
	ownerBindingValid := true
	if execForTool.HasRuntimeOwner() {
		ownerSessionID, _, _, _, ownerOK := execForTool.ActiveRuntimeOwnerIdentity()
		if ownerOK {
			ownerBindingValid = binding.SessionID == ownerSessionID
		} else {
			// Session-less embedded loops still carry the private active-run
			// capability. Bind the fallback identity explicitly to that epoch;
			// callers without either owner form fail closed below.
			ownerBindingValid = binding.SessionID != "" && binding.ApprovalEpoch == execForTool.ApprovalEpoch()
		}
	}
	if execForTool.HasRuntimeOwner() && (!execForTool.IsRuntimeOwned() || !execForTool.RuntimeIdentityMatches() || !ownerBindingValid) {
		result := types.ToolResultBlock{
			Type: types.ContentTypeToolResult, ToolUseID: tu.ID,
			Content: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeToolPermissionRequired, tu.Name),
			IsError: true, Outcome: types.ToolOutcomeDenied,
		}
		return singleToolExecutionResult{Result: result, Reminders: reminders, PreventContinuation: toolPreventContinuation}, nil
	}
	toolPermission.PermissionGrant = reg.AuthorizePermissionGrant(
		toolPermission.PermissionGrant, tu.Name, input, binding, policyCode,
	)
	executionContext := approvalcommit.WithPending(executioncontract.WithToolExecutionContext(ctx, execForTool), approvalcommit.Pending{
		Token: toolPermission.PermissionGrant, Binding: binding, PolicyCode: policyCode,
	})
	postHookInput := hooks.HookInput{ToolInput: input}.Snapshot().ToolInput
	result, err := reg.ExecuteToolWithError(executionContext, tu.Name, input)
	if err != nil {
		outcome := types.ToolOutcomeFailed
		if errors.Is(err, context.DeadlineExceeded) {
			outcome = types.ToolOutcomeTimedOut
		} else if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			outcome = types.ToolOutcomeCancelled
		}
		result := types.ToolResultBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: tu.ID,
			Content:   i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeToolExecutionFailed, err),
			IsError:   true,
			Outcome:   outcome,
		}
		failureReminders, failurePrevent := runPostToolHooks(executionContext, runner, hooks.HookPostToolUseFailure, tu, postHookInput, result.TextContent())
		reminders = append(reminders, failureReminders...)
		toolPreventContinuation = toolPreventContinuation || failurePrevent
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return singleToolExecutionResult{Result: result, Reminders: reminders, PreventContinuation: toolPreventContinuation}, err
		}
		return singleToolExecutionResult{Result: result, Reminders: reminders, PreventContinuation: toolPreventContinuation}, nil
	}
	result.ToolUseID = tu.ID
	if result.Outcome == "" {
		result.Outcome = inferredToolOutcome(result)
	}

	hookType := hooks.HookPostToolUse
	if result.IsError {
		hookType = hooks.HookPostToolUseFailure
	}
	postReminders, postPrevent := runPostToolHooks(executionContext, runner, hookType, tu, postHookInput, result.TextContent())
	reminders = append(reminders, postReminders...)
	toolPreventContinuation = toolPreventContinuation || postPrevent
	result = compact.NormalizeEmptyToolResult(result, tu.Name)
	return singleToolExecutionResult{Result: result, Reminders: reminders, PreventContinuation: toolPreventContinuation}, nil
}

func permissionRevalidationIsStronger(approved, final types.ToolPermissionResult) bool {
	if approved.PermissionBinding.PolicySnapshotDigest != final.PermissionBinding.PolicySnapshotDigest {
		return true
	}
	if approved.PermissionBinding.SandboxCapability != final.PermissionBinding.SandboxCapability {
		return true
	}
	strength := func(result types.ToolPermissionResult) int {
		switch result.Behavior {
		case types.PermissionBehaviorDeny:
			return 3
		case types.PermissionBehaviorAsk:
			return 2
		case types.PermissionBehaviorPassthrough:
			return 1
		default:
			return 0
		}
	}
	if strength(final) > strength(approved) || final.Required && !approved.Required {
		return true
	}
	approvedCode, finalCode := approved.ExecutionPolicyCode, final.ExecutionPolicyCode
	if approved.PolicyDecision != nil {
		if approvedCode == "" {
			approvedCode = approved.PolicyDecision.Code
		}
	}
	if final.PolicyDecision != nil {
		if finalCode == "" {
			finalCode = final.PolicyDecision.Code
		}
	}
	return strength(final) >= strength(approved) && finalCode != approvedCode
}

func attachToolPolicyDecision(result *types.ToolResultBlock, decision *types.PolicyDecision) {
	if result == nil || decision == nil {
		return
	}
	result.Data = decision.Clone()
}

func inferredToolOutcome(result types.ToolResultBlock) types.ToolOutcome {
	if strings.EqualFold(result.Metadata["partial"], "true") {
		return types.ToolOutcomePartial
	}
	if strings.EqualFold(result.Metadata["timed_out"], "true") || strings.EqualFold(result.Metadata["mcp.timeout"], "true") {
		return types.ToolOutcomeTimedOut
	}
	if result.IsError {
		return types.ToolOutcomeFailed
	}
	return types.ToolOutcomeSucceeded
}

func executeToolsConcurrentlyDetailed(ctx context.Context, reg *registry.Registry, runner *hooks.Runner, permHandler permission.PermissionHandler, sessionID string, execContext executioncontract.ToolExecutionContext, toolUses []types.ToolUseBlock, onResult func(int, types.ToolResultBlock)) (toolExecutionResults, error) {
	roundStarted := time.Now()
	// Admission is atomic for the provider-authorized batch. A malformed sibling
	// must not allow another sibling to reach hooks, permission prompts, or a
	// physical tool boundary.
	for index, toolUse := range toolUses {
		tool := reg.Get(toolUse.Name)
		if tool == nil {
			continue
		}
		if validationErr := types.ValidateToolInput(tool, toolUse.Input); validationErr != nil {
			results := make([]types.ToolResultBlock, len(toolUses))
			for siblingIndex, sibling := range toolUses {
				content := i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeToolExecutionCancelled)
				if siblingIndex == index {
					content = i18n.FormatToolInputValidationError(i18n.DetectOrLoadLanguage(), validationErr)
				}
				results[siblingIndex] = types.ToolResultBlock{
					Type: types.ContentTypeToolResult, ToolUseID: sibling.ID, ToolType: sibling.ToolType,
					Content: content, IsError: true, Outcome: types.ToolOutcomeFailed,
				}
				if onResult != nil {
					onResult(siblingIndex, results[siblingIndex])
				}
			}
			return toolExecutionResults{
				Results: results,
				Metrics: toolExecutionMetrics{ErrorCount: len(results)},
			}, nil
		}
	}
	enqueuedAt := make([]time.Time, len(toolUses))
	for index := range enqueuedAt {
		enqueuedAt[index] = roundStarted
	}
	hookCollector := &toolHookCollector{}
	ctx = withToolHookCollector(ctx, hookCollector)
	results := make([]types.ToolResultBlock, len(toolUses))
	var allReminders []string
	var remindersMu sync.Mutex
	preventContinuation := false
	var preventMu sync.Mutex
	var metricsMu sync.Mutex
	metrics := toolExecutionMetrics{}
	activeChildren := 0
	metricsSnapshot := func() toolExecutionMetrics {
		metricsMu.Lock()
		out := metrics
		metricsMu.Unlock()
		out.CriticalPathDuration = time.Since(roundStarted)
		out.ErrorCount = countToolResultErrors(results)
		return out
	}
	currentResults := func() toolExecutionResults {
		return toolExecutionResults{
			Results: results, Reminders: allReminders, PreventContinuation: preventContinuation,
			HookSummaries: hookCollector.drain(), Metrics: metricsSnapshot(),
		}
	}
	executeMeasured := func(executionCtx context.Context, index int) (singleToolExecutionResult, error) {
		startedAt := time.Now()
		metricsMu.Lock()
		metrics.QueueDuration += startedAt.Sub(enqueuedAt[index])
		activeChildren++
		if activeChildren > metrics.PeakFanout {
			metrics.PeakFanout = activeChildren
		}
		metricsMu.Unlock()

		executed, err := executeOneTool(executionCtx, reg, runner, permHandler, sessionID, execContext, toolUses[index])
		elapsed := time.Since(startedAt)
		physicalOperations, physicalLatency := measuredToolOperationFacts(reg, toolUses[index].Name, executed.Result, elapsed)
		metricsMu.Lock()
		metrics.PhysicalChildOperations += physicalOperations
		metrics.TotalChildLatency += physicalLatency
		activeChildren--
		metricsMu.Unlock()
		return executed, err
	}
	executeScheduled := func(executionCtx context.Context, index int) (singleToolExecutionResult, error) {
		if index > 0 && isAdjacentRevisionFusion(reg, toolUses[index-1], toolUses[index]) {
			boundContext, skipped := revisionBarrierExecutionContext(executionCtx, results[index-1], toolUses[index])
			if skipped != nil {
				skipped.ToolType = toolUses[index].ToolType
				metricsMu.Lock()
				metrics.RevisionBarrierSkips++
				metricsMu.Unlock()
				return singleToolExecutionResult{Result: *skipped}, nil
			}
			executionCtx = boundContext
		}
		executed, err := executeMeasured(executionCtx, index)
		executed.Result.ToolType = toolUses[index].ToolType
		if isRevisionMismatchResult(executed.Result) {
			metricsMu.Lock()
			metrics.RevisionMismatchCount++
			metricsMu.Unlock()
		}
		return executed, err
	}

	type toolBatch struct {
		indices    []int
		concurrent bool
	}

	var batches []toolBatch
	for i := 0; i < len(toolUses); {
		if !isConcurrentSafe(reg, toolUses[i].Name, toolUses[i].Input) {
			batches = append(batches, toolBatch{indices: []int{i}})
			i++
			continue
		}

		batch := toolBatch{concurrent: true}
		for i < len(toolUses) && isConcurrentSafe(reg, toolUses[i].Name, toolUses[i].Input) {
			batch.indices = append(batch.indices, i)
			i++
		}
		batches = append(batches, batch)
	}
	metrics.BatchCount = len(batches)
	for index := 1; index < len(toolUses); index++ {
		if isAdjacentRevisionFusion(reg, toolUses[index-1], toolUses[index]) {
			metrics.RevisionFusionCount++
		}
	}

	type callbackMsg struct {
		index  int
		result types.ToolResultBlock
	}

	cancelledResult := func(i int) types.ToolResultBlock {
		result := cancelledToolResult(toolUses[i])
		result.ToolType = toolUses[i].ToolType
		return result
	}

	fillRemainingCancelled := func(batchIndex int) {
		for _, batch := range batches[batchIndex:] {
			for _, i := range batch.indices {
				if i > 0 && isAdjacentRevisionFusion(reg, toolUses[i-1], toolUses[i]) {
					_, skipped := revisionBarrierExecutionContext(ctx, results[i-1], toolUses[i])
					if skipped != nil {
						results[i] = *skipped
						metrics.RevisionBarrierSkips++
					} else {
						results[i] = cancelledResult(i)
					}
				} else {
					results[i] = cancelledResult(i)
				}
				if onResult != nil {
					onResult(i, results[i])
				}
			}
		}
	}

	toolCtx, toolCancel := context.WithCancel(ctx)
	defer toolCancel()

	for batchIndex, batch := range batches {
		if !batch.concurrent {
			i := batch.indices[0]
			if ctx.Err() != nil {
				fillRemainingCancelled(batchIndex)
				return currentResults(), ctx.Err()
			}

			executed, err := executeScheduled(ctx, i)
			allReminders = append(allReminders, executed.Reminders...)
			preventContinuation = preventContinuation || executed.PreventContinuation
			result := executed.Result
			results[i] = result
			if onResult != nil {
				onResult(i, result)
			}
			if err != nil {
				fillRemainingCancelled(batchIndex + 1)
				if ctx.Err() != nil {
					return currentResults(), ctx.Err()
				}
				return currentResults(), err
			}
			continue
		}

		var wg sync.WaitGroup
		var mu sync.Mutex
		var infraErr error
		var infraErrOnce sync.Once
		callbackCh := make(chan callbackMsg, len(batch.indices))
		sem := make(chan struct{}, maxToolUseConcurrency())

		for _, idx := range batch.indices {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-toolCtx.Done():
					result := cancelledResult(i)
					mu.Lock()
					results[i] = result
					mu.Unlock()
					if onResult != nil {
						callbackCh <- callbackMsg{index: i, result: result}
					}
					return
				}
				executed, err := executeScheduled(toolCtx, i)
				if len(executed.Reminders) > 0 {
					remindersMu.Lock()
					allReminders = append(allReminders, executed.Reminders...)
					remindersMu.Unlock()
				}
				if executed.PreventContinuation {
					preventMu.Lock()
					preventContinuation = true
					preventMu.Unlock()
				}
				result := executed.Result
				mu.Lock()
				results[i] = result
				mu.Unlock()
				if onResult != nil {
					callbackCh <- callbackMsg{index: i, result: result}
				}
				if err != nil {
					infraErrOnce.Do(func() { infraErr = err; toolCancel() })
					return
				}
			}(idx)
		}

		done := make(chan struct{})
		go func() {
			defer close(done)
			for msg := range callbackCh {
				onResult(msg.index, msg.result)
			}
		}()

		wg.Wait()
		close(callbackCh)
		<-done

		if infraErr != nil {
			fillRemainingCancelled(batchIndex + 1)
			return currentResults(), infraErr
		}
	}

	return currentResults(), nil
}
