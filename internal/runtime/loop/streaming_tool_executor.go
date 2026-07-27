package loop

import (
	"context"
	"fmt"
	"sync"
	"time"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/internal/contracts/workspacerevision"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type streamingToolStatus string

const (
	streamingToolQueued    streamingToolStatus = "queued"
	streamingToolExecuting streamingToolStatus = "executing"
	streamingToolCompleted streamingToolStatus = "completed"
	streamingToolYielded   streamingToolStatus = "yielded"
)

type streamingToolEventType string

const (
	streamingToolEventProgress streamingToolEventType = "progress"
	streamingToolEventResult   streamingToolEventType = "result"
)

type StreamingToolEvent struct {
	Type   streamingToolEventType
	Index  int
	Tool   types.ToolUseBlock
	Result *types.ToolResultBlock
}

type streamingToolResult struct {
	result              types.ToolResultBlock
	reminders           []string
	preventContinuation bool
	err                 error
}

type trackedStreamingTool struct {
	index             int
	tool              types.ToolUseBlock
	assistantMessage  types.Message
	status            streamingToolStatus
	concurrencySafe   bool
	pendingProgress   []StreamingToolEvent
	result            *streamingToolResult
	cancel            context.CancelFunc
	completionNoticed bool
	queuedAt          time.Time
	startedAt         time.Time
	revisionUpstream  *trackedStreamingTool
	revisionReceipt   workspacerevision.Receipt
	revisionPrepared  bool
}

type StreamingToolExecutor struct {
	ctx           context.Context
	cancel        context.CancelFunc
	reg           *registry.Registry
	runner        *hooks.Runner
	permHandler   permission.PermissionHandler
	sessionID     string
	baseContext   executioncontract.ToolExecutionContext
	hookCollector *toolHookCollector

	mu             sync.Mutex
	notify         chan struct{}
	tools          []*trackedStreamingTool
	discarded      bool
	bashErrored    bool
	erroredTool    string
	remainingLimit int
	roundStarted   time.Time
	metrics        toolExecutionMetrics
	activeChildren int
}

func NewStreamingToolExecutor(ctx context.Context, reg *registry.Registry, runner *hooks.Runner, permHandler permission.PermissionHandler, sessionID string, execContext executioncontract.ToolExecutionContext) *StreamingToolExecutor {
	hookCollector := &toolHookCollector{}
	childCtx, cancel := context.WithCancel(withToolHookCollector(ctx, hookCollector))
	e := &StreamingToolExecutor{
		ctx:            childCtx,
		cancel:         cancel,
		reg:            reg,
		runner:         runner,
		permHandler:    permHandler,
		sessionID:      sessionID,
		baseContext:    execContext,
		hookCollector:  hookCollector,
		remainingLimit: maxToolUseConcurrency(),
		notify:         make(chan struct{}),
	}
	return e
}

func (e *StreamingToolExecutor) AddTool(tool types.ToolUseBlock, assistantMessage types.Message) {
	e.mu.Lock()
	if e.discarded {
		e.mu.Unlock()
		return
	}
	tracked := &trackedStreamingTool{
		index:            len(e.tools),
		tool:             tool,
		assistantMessage: assistantMessage,
		status:           streamingToolQueued,
		concurrencySafe:  isConcurrentSafe(e.reg, tool.Name, tool.Input),
		queuedAt:         time.Now(),
	}
	if len(e.tools) > 0 {
		upstream := e.tools[len(e.tools)-1]
		if isAdjacentRevisionFusion(e.reg, upstream.tool, tool) {
			tracked.revisionUpstream = upstream
			e.metrics.RevisionFusionCount++
		}
	}
	if e.roundStarted.IsZero() {
		e.roundStarted = tracked.queuedAt
	}
	if len(e.tools) == 0 || !tracked.concurrencySafe || !e.tools[len(e.tools)-1].concurrencySafe {
		e.metrics.BatchCount++
	}
	e.tools = append(e.tools, tracked)
	e.processQueueLocked()
	e.mu.Unlock()
}

func (e *StreamingToolExecutor) Discard() {
	e.mu.Lock()
	if e.discarded {
		e.mu.Unlock()
		return
	}
	e.discarded = true
	for _, tool := range e.tools {
		if tool.cancel != nil {
			tool.cancel()
		}
	}
	e.cancel()
	e.signalLocked()
	e.mu.Unlock()
}

func (e *StreamingToolExecutor) RemainingResults(ctx context.Context) (toolExecutionResults, []StreamingToolEvent, error) {
	var combined toolExecutionResults
	var events []StreamingToolEvent

	for {
		e.mu.Lock()
		if e.discarded {
			e.mu.Unlock()
			return combined, events, nil
		}
		e.processQueueLocked()
		infraErr := e.firstErrorLocked()
		chunk, chunkEvents := e.completedResultsLocked()
		combined.Results = append(combined.Results, chunk.Results...)
		combined.Reminders = append(combined.Reminders, chunk.Reminders...)
		combined.HookSummaries = append(combined.HookSummaries, chunk.HookSummaries...)
		combined.PreventContinuation = combined.PreventContinuation || chunk.PreventContinuation
		events = append(events, chunkEvents...)
		if infraErr != nil {
			combined.Metrics = e.metricsSnapshotLocked()
			e.mu.Unlock()
			e.Discard()
			return combined, events, infraErr
		}
		if !e.hasUnfinishedLocked() {
			combined.Metrics = e.metricsSnapshotLocked()
			e.mu.Unlock()
			return combined, events, nil
		}
		wake := e.notify
		e.mu.Unlock()

		if len(chunk.Results) > 0 || len(chunkEvents) > 0 {
			continue
		}
		if err := e.wait(ctx, wake); err != nil {
			e.Discard()
			return combined, events, err
		}
	}
}

func (e *StreamingToolExecutor) wait(ctx context.Context, wake <-chan struct{}) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-e.ctx.Done():
		return e.ctx.Err()
	case <-wake:
		return nil
	}
}

func (e *StreamingToolExecutor) signalLocked() {
	close(e.notify)
	e.notify = make(chan struct{})
}

func (e *StreamingToolExecutor) completedResultsLocked() (toolExecutionResults, []StreamingToolEvent) {
	if e.discarded {
		return toolExecutionResults{}, nil
	}

	var out toolExecutionResults
	out.HookSummaries = e.hookCollector.drain()
	var events []StreamingToolEvent
	for _, tool := range e.tools {
		for len(tool.pendingProgress) > 0 {
			events = append(events, tool.pendingProgress[0])
			tool.pendingProgress = tool.pendingProgress[1:]
		}
	}

	for _, tool := range e.tools {
		if tool.status == streamingToolYielded {
			continue
		}
		if tool.status == streamingToolCompleted && tool.result != nil {
			tool.status = streamingToolYielded
			out.Results = append(out.Results, tool.result.result)
			out.Reminders = append(out.Reminders, tool.result.reminders...)
			out.PreventContinuation = out.PreventContinuation || tool.result.preventContinuation
			events = append(events, StreamingToolEvent{Type: streamingToolEventResult, Index: tool.index, Tool: tool.tool, Result: &tool.result.result})
			continue
		}
		break
	}
	return out, events
}

func (e *StreamingToolExecutor) hasUnfinishedLocked() bool {
	for _, tool := range e.tools {
		if tool.status != streamingToolYielded {
			return true
		}
	}
	return false
}

func (e *StreamingToolExecutor) firstErrorLocked() error {
	for _, tool := range e.tools {
		if tool.status == streamingToolCompleted && tool.result != nil && tool.result.err != nil {
			return tool.result.err
		}
	}
	return nil
}

func (e *StreamingToolExecutor) canExecuteLocked(concurrencySafe bool) bool {
	executing := 0
	for _, tool := range e.tools {
		if tool.status != streamingToolExecuting {
			continue
		}
		executing++
		if !concurrencySafe || !tool.concurrencySafe {
			return false
		}
	}
	if executing >= e.remainingLimit {
		return false
	}
	return executing == 0 || concurrencySafe
}

func (e *StreamingToolExecutor) processQueueLocked() {
	for _, tool := range e.tools {
		if tool.status != streamingToolQueued {
			continue
		}
		if tool.revisionUpstream != nil && !tool.revisionPrepared {
			upstream := tool.revisionUpstream
			if upstream.status == streamingToolQueued || upstream.status == streamingToolExecuting {
				break
			}
			tool.revisionPrepared = true
			if upstream.result == nil {
				break
			}
			boundContext, skipped := revisionBarrierExecutionContext(e.ctx, upstream.result.result, tool.tool)
			if skipped != nil {
				tool.status = streamingToolCompleted
				tool.result = &streamingToolResult{result: *skipped}
				e.metrics.RevisionBarrierSkips++
				e.signalLocked()
				continue
			}
			tool.revisionReceipt, _ = workspacerevision.FromContext(boundContext)
		}
		if !e.canExecuteLocked(tool.concurrencySafe) {
			if !tool.concurrencySafe {
				break
			}
			continue
		}
		e.startToolLocked(tool)
	}
}

func (e *StreamingToolExecutor) startToolLocked(tool *trackedStreamingTool) {
	tool.status = streamingToolExecuting
	tool.startedAt = time.Now()
	e.metrics.QueueDuration += tool.startedAt.Sub(tool.queuedAt)
	e.activeChildren++
	if e.activeChildren > e.metrics.PeakFanout {
		e.metrics.PeakFanout = e.activeChildren
	}
	toolCtx, cancel := context.WithCancel(e.ctx)
	if tool.revisionReceipt.Valid() {
		toolCtx = workspacerevision.WithReceipt(toolCtx, tool.revisionReceipt)
	}
	tool.cancel = cancel
	go e.executeTool(toolCtx, tool)
}

func (e *StreamingToolExecutor) executeTool(ctx context.Context, tool *trackedStreamingTool) {
	defer func() {
		e.mu.Lock()
		tool.completionNoticed = true
		e.processQueueLocked()
		e.signalLocked()
		e.mu.Unlock()
	}()

	e.mu.Lock()
	discarded := e.discarded
	bashErrored := e.bashErrored
	erroredTool := e.erroredTool
	e.mu.Unlock()

	if discarded {
		e.completeTool(tool, streamingFallbackToolResult(tool.tool), nil, false, nil)
		return
	}
	if bashErrored {
		e.completeTool(tool, siblingCancelledToolResult(tool.tool, erroredTool), nil, false, nil)
		return
	}

	execContext := e.baseContext
	execContext.AssistantMessage = tool.assistantMessage
	executed, err := executeOneTool(ctx, e.reg, e.runner, e.permHandler, e.sessionID, execContext, tool.tool)
	result := executed.Result
	if err != nil && (ctx.Err() != nil || e.ctx.Err() != nil) {
		result = cancelledToolResult(tool.tool)
		if e.isSiblingError() {
			result = siblingCancelledToolResult(tool.tool, e.erroredTool)
			err = nil
		}
	}
	if tool.tool.Name == "Bash" && result.IsError {
		e.mu.Lock()
		if !e.bashErrored {
			e.bashErrored = true
			e.erroredTool = describeStreamingTool(tool.tool)
			for _, sibling := range e.tools {
				if sibling.index == tool.index || sibling.status == streamingToolYielded || sibling.status == streamingToolCompleted {
					continue
				}
				if sibling.cancel != nil {
					sibling.cancel()
				}
			}
		}
		e.mu.Unlock()
	}
	e.completeTool(tool, result, executed.Reminders, executed.PreventContinuation, err)
}

func (e *StreamingToolExecutor) completeTool(tool *trackedStreamingTool, result types.ToolResultBlock, reminders []string, preventContinuation bool, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.discarded {
		result = streamingFallbackToolResult(tool.tool)
		err = nil
	}
	if e.bashErrored && tool.tool.Name != "Bash" && result.ToolUseID == tool.tool.ID && !result.IsError && tool.status == streamingToolExecuting {
		result = siblingCancelledToolResult(tool.tool, e.erroredTool)
		err = nil
	}
	tool.result = &streamingToolResult{
		result:              result,
		reminders:           reminders,
		preventContinuation: preventContinuation,
		err:                 err,
	}
	if isRevisionMismatchResult(result) {
		e.metrics.RevisionMismatchCount++
	}
	if !tool.startedAt.IsZero() {
		elapsed := time.Since(tool.startedAt)
		physicalOperations, physicalLatency := measuredToolOperationFacts(e.reg, tool.tool.Name, result, elapsed)
		e.metrics.PhysicalChildOperations += physicalOperations
		e.metrics.TotalChildLatency += physicalLatency
		if e.activeChildren > 0 {
			e.activeChildren--
		}
	}
	tool.status = streamingToolCompleted
	e.signalLocked()
}

func (e *StreamingToolExecutor) metricsSnapshotLocked() toolExecutionMetrics {
	metrics := e.metrics
	if !e.roundStarted.IsZero() {
		metrics.CriticalPathDuration = time.Since(e.roundStarted)
	}
	for _, tool := range e.tools {
		if tool.result == nil {
			continue
		}
		metrics.ErrorCount += countToolResultErrors([]types.ToolResultBlock{tool.result.result})
	}
	return metrics
}

func (e *StreamingToolExecutor) isSiblingError() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.bashErrored
}

func streamingFallbackToolResult(tool types.ToolUseBlock) types.ToolResultBlock {
	return types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: tool.ID,
		ToolType:  tool.ToolType,
		Content:   i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeStreamingToolDiscarded),
		IsError:   true,
		Outcome:   types.ToolOutcomeCancelled,
	}
}

func siblingCancelledToolResult(tool types.ToolUseBlock, erroredTool string) types.ToolResultBlock {
	lang := i18n.DetectOrLoadLanguage()
	msg := i18n.Text(lang, i18n.KeyRuntimeParallelToolCancelled)
	if erroredTool != "" {
		msg = i18n.Format(lang, i18n.KeyRuntimeParallelNamedToolCancelled, erroredTool)
	}
	return types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: tool.ID,
		ToolType:  tool.ToolType,
		Content:   msg,
		IsError:   true,
		Outcome:   types.ToolOutcomeCancelled,
	}
}

func describeStreamingTool(tool types.ToolUseBlock) string {
	for _, key := range []string{"command", "file_path", "pattern"} {
		if value, ok := tool.Input[key].(string); ok && value != "" {
			if len(value) > 40 {
				value = value[:40] + "..."
			}
			return fmt.Sprintf("%s(%s)", tool.Name, value)
		}
	}
	return tool.Name
}
