package hooks

import (
	"context"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"
)

// ExecutionObserver receives one immutable evidence record for every concrete
// hook configuration that actually ran. The hook type is supplied separately
// so consumers do not need to infer it from configuration metadata.
type ExecutionObserver func(HookType, HookExecution)

type executionObserverContextKey struct{}
type correlationContextKey struct{}

// WithExecutionObserver scopes hook evidence delivery to an execution tree.
// Keeping the observer on context avoids mutable Runner-level sinks crossing a
// session switch while an origin task is still running.
func WithExecutionObserver(ctx context.Context, observer ExecutionObserver) context.Context {
	return context.WithValue(ctx, executionObserverContextKey{}, observer)
}

// ExecutionObserverFromContext returns the execution-scoped evidence sink.
func ExecutionObserverFromContext(ctx context.Context) ExecutionObserver {
	if ctx == nil {
		return nil
	}
	observer, _ := ctx.Value(executionObserverContextKey{}).(ExecutionObserver)
	return observer
}

// WithCorrelation attaches stable causal identity inherited by observed hook
// call sites. Event-specific input supplied at the call site wins over these
// defaults.
func WithCorrelation(ctx context.Context, correlation HookInput) context.Context {
	return context.WithValue(ctx, correlationContextKey{}, correlation.Snapshot())
}

// CorrelateInput fills missing stable identity from the execution context.
func CorrelateInput(ctx context.Context, input HookInput) HookInput {
	input = input.Snapshot()
	if ctx == nil {
		return input
	}
	correlation, _ := ctx.Value(correlationContextKey{}).(HookInput)
	fill := func(target *string, fallback string) {
		if *target == "" {
			*target = fallback
		}
	}
	fill(&input.ToolName, correlation.ToolName)
	fill(&input.ToolUseID, correlation.ToolUseID)
	fill(&input.SessionID, correlation.SessionID)
	fill(&input.ProjectRoot, correlation.ProjectRoot)
	fill(&input.TurnID, correlation.TurnID)
	fill(&input.WorkUnitID, correlation.WorkUnitID)
	fill(&input.AgentID, correlation.AgentID)
	fill(&input.AgentType, correlation.AgentType)
	fill(&input.TeammateName, correlation.TeammateName)
	fill(&input.TeamName, correlation.TeamName)
	fill(&input.TaskID, correlation.TaskID)
	fill(&input.TaskOwner, correlation.TaskOwner)
	fill(&input.Owner, correlation.Owner)
	if execution, ok := executioncontract.ToolExecutionContextFromContext(ctx); ok {
		fill(&input.ToolName, execution.ToolUse.Name)
		fill(&input.ToolUseID, execution.ToolUse.ID)
		fill(&input.SessionID, execution.SessionID)
		fill(&input.ProjectRoot, execution.ProjectRoot)
		fill(&input.TurnID, execution.TurnID)
		fill(&input.WorkUnitID, execution.WorkUnitID)
		fill(&input.AgentID, execution.ActorID)
		fill(&input.AgentType, execution.ActorType)
	}
	return input
}

// RunDetailedObserved executes hooks with inherited causal identity and emits
// one detached evidence record per matching configuration.
func (r *Runner) RunDetailedObserved(ctx context.Context, hookType HookType, input HookInput) []HookExecution {
	executions := r.RunDetailed(ctx, hookType, CorrelateInput(ctx, input))
	observer := ExecutionObserverFromContext(ctx)
	if observer == nil {
		return executions
	}
	for _, execution := range executions {
		observer(hookType, execution.Snapshot())
	}
	return executions
}

// RunBlockingDetailed is the identity/evidence-preserving counterpart of
// RunBlocking. It retains every execution even when a lifecycle transition is
// refused or the context is cancelled after a hook returns.
func (r *Runner) RunBlockingDetailed(ctx context.Context, hookType HookType, input HookInput) ([]HookExecution, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, nil
	}
	executions := r.RunDetailedObserved(ctx, hookType, input)
	if err := ctx.Err(); err != nil {
		return executions, err
	}
	for _, execution := range executions {
		output := execution.Output
		if !output.Block {
			continue
		}
		reason := firstHookReason(output)
		return executions, &BlockingError{HookType: hookType, Reason: reason, Output: output}
	}
	return executions, nil
}
