package main

import (
	"math"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/types"
	"github.com/agent-dance/luban/ui"
)

// briefTurnRenderer buffers raw assistant text for terminal/JSON renderers.
// If the turn emits SendUserMessage, the typed Brief output replaces that
// detail text; otherwise the text is flushed at the end of the turn.
type briefTurnRenderer struct {
	renderer       ui.Renderer
	pendingText    strings.Builder
	briefUse       bool
	briefSent      bool
	turnCount      int
	hasTurn        bool
	toolCalls      map[string]types.ToolUseBlock
	semanticGroups *semanticToolAggregationBuffer
}

func (b *briefTurnRenderer) beginTurn(turnCount int) {
	if turnCount == 0 {
		return
	}
	if b.hasTurn && b.turnCount != turnCount {
		b.finishTurn()
	}
	b.turnCount = turnCount
	b.hasTurn = true
}

func (b *briefTurnRenderer) text(value string) {
	if b.renderer != nil {
		b.pendingText.WriteString(value)
	}
}

func (b *briefTurnRenderer) toolUseEvent(ctx ui.ToolEventContext, toolUse types.ToolUseBlock) {
	name := toolUse.Name
	b.briefUse = name == "SendUserMessage" || name == "Brief"
	if toolUse.ID != "" {
		if b.toolCalls == nil {
			b.toolCalls = make(map[string]types.ToolUseBlock)
		}
		b.toolCalls[toolUse.ID] = toolUse
	}
	if semantic, ok := b.renderer.(ui.SemanticToolRenderer); ok && !b.briefUse {
		if b.semanticGroups == nil {
			b.semanticGroups = newSemanticToolAggregationBuffer()
		}
		if b.semanticGroups.Start(ctx, toolUse) {
			semantic.RenderToolPresentation(semanticToolCallPresentation(ctx, toolUse))
		}
		return
	}
	ui.DispatchToolCallEvent(b.renderer, ctx, toolUse)
}

func (b *briefTurnRenderer) toolResultEvent(ctx ui.ToolEventContext, result types.ToolResultBlock) {
	if ui.IsSendUserMessageResult(result) {
		b.pendingText.Reset()
		b.briefSent = true
		ui.DispatchToolResultEvent(b.renderer, ctx, result)
		b.briefUse = false
		return
	}
	if b.briefUse {
		b.flushText()
	}
	if semantic, ok := b.renderer.(ui.SemanticToolRenderer); ok {
		call := b.toolCalls[result.ToolUseID]
		if call.ID == "" {
			call = types.ToolUseBlock{ID: result.ToolUseID, Name: "Tool"}
		}
		presentation := semanticToolResultPresentation(ctx, call, result)
		if b.semanticGroups == nil || b.semanticGroups.Complete(ctx, call, result, presentation) {
			semantic.RenderToolPresentation(presentation)
		}
		delete(b.toolCalls, result.ToolUseID)
		b.briefUse = false
		return
	}
	ui.DispatchToolResultEvent(b.renderer, ctx, result)
	b.briefUse = false
}

func (b *briefTurnRenderer) flushText() {
	if b.renderer != nil && b.pendingText.Len() > 0 {
		b.renderer.Text(b.pendingText.String())
	}
	b.pendingText.Reset()
}

func (b *briefTurnRenderer) finishTurn() {
	b.flushSemanticGroups()
	if !b.briefSent {
		b.flushText()
	} else {
		b.pendingText.Reset()
	}
	b.briefSent = false
	b.briefUse = false
	b.hasTurn = false
	b.turnCount = 0
}

func (b *briefTurnRenderer) flushSemanticGroups() {
	if semantic, ok := b.renderer.(ui.SemanticToolRenderer); ok && b.semanticGroups != nil {
		for _, presentation := range b.semanticGroups.Flush() {
			semantic.RenderToolPresentation(presentation)
		}
	}
}

func toolEventContext(event loop.Event) ui.ToolEventContext {
	sessionID := ""
	if queryMarker := strings.Index(event.TurnID, ":query-"); queryMarker > 0 {
		// queryTurnIdentity embeds the durable session identity in every
		// top-level turn ID. Recover it here because loop.Event intentionally
		// does not duplicate SessionID.
		sessionID = event.TurnID[:queryMarker]
	}
	return ui.ToolEventContext{
		SessionID: sessionID, ProjectRoot: event.ProjectRoot, TurnID: event.TurnID,
		ActorID: event.ActorID, ActorType: event.ActorType, WorkUnitID: event.WorkUnitID,
	}
}

func effectiveContextProjection(event *loop.ContextUsageEvent) ui.EffectiveContextProjection {
	if event == nil {
		return ui.EffectiveContextProjection{Scope: ui.UsageScopeModelContext}
	}
	percent := modelContextPercent(event.UsedTokens, event.CapacityTokens)
	measurement := ui.ContextMeasurement(event.Measurement)
	if measurement == "" {
		measurement = ui.ContextMeasurementUnknown
	}
	return ui.EffectiveContextProjection{
		Scope: ui.UsageScopeModelContext, Known: event.CapacityTokens > 0,
		UsedTokens: event.UsedTokens, CapacityTokens: event.CapacityTokens, PercentUsed: percent,
		Measurement: measurement, EstimateComplete: event.EstimateComplete,
		UnknownOverheads: append([]string(nil), event.UnknownOverheads...),
	}
}

func modelContextPercent(usedTokens, capacityTokens int) int {
	if capacityTokens <= 0 {
		return 0
	}
	percent := int(math.Round(float64(max(usedTokens, 0)) / float64(capacityTokens) * 100))
	return min(max(percent, 0), 100)
}

func dispatchHookSummary(renderer ui.Renderer, event loop.Event) {
	if event.HookSummary == nil {
		return
	}
	structured, ok := renderer.(ui.StructuredHookRenderer)
	if !ok {
		return
	}
	structured.RenderHookSummary(toolEventContext(event), ui.HookSummary{
		ExecutionID: event.HookSummary.HookExecutionID,
		ToolUseID:   event.HookSummary.ToolUseID,
		Name:        event.HookSummary.HookName,
		Status:      event.HookSummary.Status,
		Summary:     event.HookSummary.Summary,
		Metadata:    event.HookSummary.Metadata,
	})
}

// makeEventHandler returns an event callback that writes to the terminal
// via the given Renderer. Used by print mode (-p).
func makeEventHandler(r ui.Renderer, verbose bool) func(loop.Event) {
	brief := &briefTurnRenderer{renderer: r}
	return func(event loop.Event) {
		brief.beginTurn(event.TurnCount)
		switch event.Type {
		case loop.EventText:
			brief.text(event.Text)
		case loop.EventThinking:
			if verbose {
				r.Thinking(event.Text)
			}
		case loop.EventToolUse:
			if event.ToolUse != nil {
				brief.toolUseEvent(toolEventContext(event), *event.ToolUse)
			}
		case loop.EventToolResult:
			if event.ToolResult != nil {
				brief.toolResultEvent(toolEventContext(event), *event.ToolResult)
			}
		case loop.EventHookSummary:
			dispatchHookSummary(r, event)
		case loop.EventTurnEnd:
			brief.finishTurn()
			r.Usage(event.Usage)
		case loop.EventError:
			brief.flushSemanticGroups()
			brief.flushText()
			ui.DispatchRuntimeErrorEvent(r, toolEventContext(event), event.ToolUseID, event.Text, event.Error, event.Metadata)
		case loop.EventSystemWarning:
			brief.flushSemanticGroups()
			brief.flushText()
			ui.DispatchRuntimeWarningEvent(r, event.SystemWarningRuntimeEvent(), i18n.LangEN, false)
		}
	}
}

// makeREPLEventHandler returns an event handler for interactive mode
// (always shows thinking, unlike print mode which respects --verbose).
func makeREPLEventHandler(r ui.Renderer) func(loop.Event) {
	return makeREPLEventHandlerWithCost(r, nil, nil, nil)
}

// makeREPLEventHandlerWithCost returns an event handler for interactive mode
// that also records cost via tracker (may be nil) and calls r.CostSummary on
// each TurnEnd.  getDuration is kept for API compatibility but is unused;
// per-turn wall-clock time is tracked internally between consecutive TurnEnd
// events so multi-turn tool-use loops report accurate per-turn durations.
// getContextUsage (optional) returns (maxTokens, usedTokens) for the context bar.
func makeREPLEventHandlerWithCost(r ui.Renderer, tracker *ui.CostTracker, getDuration func() time.Duration, getContextUsage func() (int, int)) func(loop.Event) {
	// turnStart is reset after each TurnEnd so successive turns each get their
	// own elapsed duration rather than a cumulative value from query start.
	turnStart := time.Now()
	brief := &briefTurnRenderer{renderer: r}
	return func(event loop.Event) {
		brief.beginTurn(event.TurnCount)
		switch event.Type {
		case loop.EventText:
			brief.text(event.Text)
		case loop.EventThinking:
			r.Thinking(event.Text)
		case loop.EventToolUse:
			if event.ToolUse != nil {
				brief.toolUseEvent(toolEventContext(event), *event.ToolUse)
			}
		case loop.EventToolResult:
			if event.ToolResult != nil {
				brief.toolResultEvent(toolEventContext(event), *event.ToolResult)
			}
		case loop.EventHookSummary:
			dispatchHookSummary(r, event)
		case loop.EventGoalEvaluation, loop.EventProviderUsage:
			recordAuxiliaryUsageEvent(tracker, event)
		case loop.EventContextUsage:
			if structured, ok := r.(ui.EffectiveContextRenderer); ok {
				structured.EffectiveContext(effectiveContextProjection(event.ContextUsage))
			}
		case loop.EventTurnEnd:
			brief.finishTurn()
			recorded := recordTurnUsageEvent(tracker, event, time.Since(turnStart))
			maxTok, usedTok := 0, 0
			if getContextUsage != nil {
				maxTok, usedTok = getContextUsage()
			}
			if structured, ok := r.(ui.StructuredUsageRenderer); ok && recorded {
				structured.UsageSemantics(ui.BuildUsageSemanticsSnapshot(event.Usage, tracker, usedTok, maxTok))
			} else {
				r.Usage(event.Usage)
			}
			if recorded {
				last := tracker.LastTurn()
				if last != nil {
					if _, ok := r.(ui.StructuredUsageRenderer); ok {
						// The scope-safe event above already includes request and
						// cumulative cost; do not emit a second ambiguous ledger.
					} else {
						r.CostSummary(last.CostUSD, tracker.TotalCost(), last.InputTokens, last.OutputTokens)
					}
				}
			}
			// Update context bar with live token usage
			if maxTok > 0 && event.Usage != nil {
				if _, ok := r.(ui.StructuredUsageRenderer); !ok || !recorded {
					r.ContextBar(usedTok, maxTok)
				}
			}
			// Reset per-turn timer for the next turn.
			turnStart = time.Now()
		case loop.EventError:
			brief.flushSemanticGroups()
			brief.flushText()
			ui.DispatchRuntimeErrorEvent(r, toolEventContext(event), event.ToolUseID, event.Text, event.Error, event.Metadata)
		case loop.EventSystemWarning:
			brief.flushSemanticGroups()
			brief.flushText()
			ui.DispatchRuntimeWarningEvent(r, event.SystemWarningRuntimeEvent(), i18n.LangEN, false)
		}
	}
}
