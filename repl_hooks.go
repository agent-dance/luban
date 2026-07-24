package main

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/agent-dance/luban/hooks"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/tui"
	"github.com/agent-dance/luban/ui"
)

var replHookSequence atomic.Uint64

type replHookRunResult struct {
	Blocked    bool
	Reason     string
	Executions int
}

func newREPLHookContext(sessionID string, hookType hooks.HookType, actorID, actorType string) ui.ToolEventContext {
	sessionID = strings.TrimSpace(sessionID)
	sequence := replHookSequence.Add(1)
	turnID := fmt.Sprintf("%s:query-repl-hook-%d", sessionID, sequence)
	return ui.ToolEventContext{
		SessionID:  sessionID,
		TurnID:     turnID,
		WorkUnitID: fmt.Sprintf("%s:%s", turnID, hookType),
		ActorID:    actorID,
		ActorType:  actorType,
	}
}

// runObservedREPLHooks is the single REPL boundary for hook execution. It
// preserves one summary per actual config, including raw evidence, before a
// caller makes any control-flow decision from a blocking result.
func runObservedREPLHooks(
	ctx context.Context,
	runner *hooks.Runner,
	renderer ui.StructuredHookRenderer,
	hookType hooks.HookType,
	input hooks.HookInput,
	eventContext ui.ToolEventContext,
) replHookRunResult {
	if runner == nil {
		return replHookRunResult{}
	}
	input.SessionID = eventContext.SessionID
	input.TurnID = eventContext.TurnID
	input.WorkUnitID = eventContext.WorkUnitID
	input.AgentID = eventContext.ActorID
	input.AgentType = eventContext.ActorType

	result := replHookRunResult{}
	parentObserver := hooks.ExecutionObserverFromContext(ctx)
	ctx = hooks.WithCorrelation(ctx, input)
	ctx = hooks.WithExecutionObserver(ctx, func(observedType hooks.HookType, execution hooks.HookExecution) {
		if parentObserver != nil {
			parentObserver(observedType, execution)
		}
		summary := loop.NewHookExecutionSummary(observedType, execution)
		result.Executions++
		if renderer != nil {
			renderer.RenderHookSummary(eventContext, ui.HookSummary{
				ExecutionID: summary.HookExecutionID,
				ToolUseID:   summary.ToolUseID,
				Name:        summary.HookName,
				Status:      summary.Status,
				Summary:     summary.Summary,
				Metadata:    summary.Metadata,
			})
		}
		if !result.Blocked && (summary.Status == "blocked" || summary.Status == "prevented") {
			result.Blocked = true
			result.Reason = summary.Summary
		}
	})
	runner.RunDetailedObserved(ctx, hookType, input)
	return result
}

// tuiREPLHookRenderer keeps lifecycle evidence durable even after the
// fullscreen event loop has stopped accepting queued updates. SessionEnd runs
// after all in-flight work has drained and before lifecycle metadata is saved.
type tuiREPLHookRenderer struct {
	app *tui.App
}

func (r tuiREPLHookRenderer) RenderHookSummary(ctx ui.ToolEventContext, summary ui.HookSummary) {
	if r.app == nil || r.app.State() == nil {
		return
	}
	apply := func() {
		if err := r.app.State().ApplyHookSummary(tui.ToolEventContext{
			SessionID: ctx.SessionID, TurnID: ctx.TurnID, WorkUnitID: ctx.WorkUnitID,
			ActorID: ctx.ActorID, ActorType: ctx.ActorType,
		}, ctx.SessionEpoch, summary); err != nil {
			r.app.State().AppendMessage(tui.Message{Kind: tui.MsgError, Text: i18n.Format(r.app.State().Language.Get(), i18n.KeyRuntimeHookEvidenceRetentionFailed, err)})
		}
	}
	if r.app.UpdateSync(apply) {
		return
	}
	if r.app.State().AdmitEpoch(ctx.SessionEpoch) {
		apply()
	}
}
