package app

import (
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/runtimeevent"
	"math"
	"strings"

	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// briefTurnRenderer buffers raw assistant text for terminal/JSON renderers.
// If the turn emits SendUserMessage, the typed Brief output replaces that
// detail text; otherwise the text is flushed at the end of the turn.
type briefTurnRenderer struct {
	renderer       presentation.Renderer
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

func (b *briefTurnRenderer) toolUseEvent(ctx presentation.ToolEventContext, toolUse types.ToolUseBlock) {
	name := toolUse.Name
	b.briefUse = name == "SendUserMessage"
	if toolUse.ID != "" {
		if b.toolCalls == nil {
			b.toolCalls = make(map[string]types.ToolUseBlock)
		}
		b.toolCalls[toolUse.ID] = toolUse
	}
	if semantic, ok := b.renderer.(presentation.SemanticToolRenderer); ok && !b.briefUse {
		if b.semanticGroups == nil {
			b.semanticGroups = newSemanticToolAggregationBuffer()
		}
		if b.semanticGroups.Start(ctx, toolUse) {
			semantic.RenderToolPresentation(semanticToolCallPresentation(ctx, toolUse))
		}
		return
	}
	presentation.DispatchToolCallEvent(b.renderer, ctx, toolUse)
}

func (b *briefTurnRenderer) toolResultEvent(ctx presentation.ToolEventContext, result types.ToolResultBlock) {
	if presentation.IsSendUserMessageResult(result) {
		b.pendingText.Reset()
		b.briefSent = true
		presentation.DispatchToolResultEvent(b.renderer, ctx, result)
		b.briefUse = false
		return
	}
	if b.briefUse {
		b.flushText()
	}
	if semantic, ok := b.renderer.(presentation.SemanticToolRenderer); ok {
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
	presentation.DispatchToolResultEvent(b.renderer, ctx, result)
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
	if semantic, ok := b.renderer.(presentation.SemanticToolRenderer); ok && b.semanticGroups != nil {
		for _, presentation := range b.semanticGroups.Flush() {
			semantic.RenderToolPresentation(presentation)
		}
	}
}

func toolEventContext(event stream.Event) presentation.ToolEventContext {
	sessionID := ""
	if queryMarker := strings.Index(event.TurnID, ":query-"); queryMarker > 0 {
		// queryTurnIdentity embeds the durable session identity in every
		// top-level turn ID. Recover it here because stream.Event intentionally
		// does not duplicate SessionID.
		sessionID = event.TurnID[:queryMarker]
	}
	return presentation.ToolEventContext{
		SessionID: sessionID, ProjectRoot: event.ProjectRoot, TurnID: event.TurnID,
		ActorID: event.ActorID, ActorType: event.ActorType, WorkUnitID: event.WorkUnitID,
	}
}

func modelContextPercent(usedTokens, capacityTokens int) int {
	if capacityTokens <= 0 {
		return 0
	}
	percent := int(math.Round(float64(max(usedTokens, 0)) / float64(capacityTokens) * 100))
	return min(max(percent, 0), 100)
}

func dispatchHookSummary(renderer presentation.Renderer, event stream.Event) {
	if event.HookSummary == nil {
		return
	}
	structured, ok := renderer.(presentation.StructuredHookRenderer)
	if !ok {
		return
	}
	structured.RenderHookSummary(toolEventContext(event), presentation.HookSummary{
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
func makeEventHandler(r presentation.Renderer, verbose bool) func(stream.Event) {
	brief := &briefTurnRenderer{renderer: r}
	return func(event stream.Event) {
		brief.beginTurn(event.TurnCount)
		switch event.Type {
		case stream.EventText:
			brief.text(event.Text)
		case stream.EventThinking:
			if verbose {
				r.Thinking(event.Text)
			}
		case stream.EventToolUse:
			if event.ToolUse != nil {
				brief.toolUseEvent(toolEventContext(event), *event.ToolUse)
			}
		case stream.EventToolResult:
			if event.ToolResult != nil {
				brief.toolResultEvent(toolEventContext(event), *event.ToolResult)
			}
		case stream.EventHookSummary:
			dispatchHookSummary(r, event)
		case stream.EventTurnEnd:
			brief.finishTurn()
			r.Usage(event.Usage)
			if machine, ok := r.(interface {
				RenderTurnEnd(presentation.ToolEventContext, int, string)
			}); ok {
				machine.RenderTurnEnd(toolEventContext(event), event.TurnCount, event.TerminalReason)
			}
		case stream.EventError:
			brief.flushSemanticGroups()
			brief.flushText()
			presentation.DispatchRuntimeErrorEvent(r, toolEventContext(event), event.ToolUseID, event.Text, event.Error, event.Metadata)
		case stream.EventSystemWarning:
			brief.flushSemanticGroups()
			brief.flushText()
			presentation.DispatchRuntimeWarningEvent(r, runtimeevent.SystemWarningRuntimeEvent(event), i18n.LangEN, false)
		case stream.EventRequestStart, stream.EventRequestRetry, stream.EventRequestFirstToken, stream.EventRequestEnd, stream.EventRequestFailed:
			if machine, ok := r.(interface {
				RenderRequestMetrics(presentation.ToolEventContext, stream.EventType, *stream.RequestStatusEvent)
			}); ok {
				machine.RenderRequestMetrics(toolEventContext(event), event.Type, event.RequestStatus)
			}
		case stream.EventToolRoundMetrics:
			if machine, ok := r.(interface {
				RenderToolRoundMetrics(presentation.ToolEventContext, *stream.ToolRoundMetricsEvent)
			}); ok {
				machine.RenderToolRoundMetrics(toolEventContext(event), event.ToolRound)
			}
		case stream.EventProgress:
			if event.Progress != nil && event.Progress.Stage == "progressive_context_projection" {
				if machine, ok := r.(interface {
					RenderProgressiveContextMetrics(presentation.ToolEventContext, int, *stream.ProgressEvent)
				}); ok {
					machine.RenderProgressiveContextMetrics(toolEventContext(event), event.TurnCount, event.Progress)
				}
			} else if event.Progress != nil && event.Progress.Stage == "context_update_shadow" {
				if machine, ok := r.(interface {
					RenderContextUpdateMetrics(presentation.ToolEventContext, int, *stream.ProgressEvent)
				}); ok {
					machine.RenderContextUpdateMetrics(toolEventContext(event), event.TurnCount, event.Progress)
				}
			}
		case stream.EventCompactBoundary:
			if machine, ok := r.(interface {
				RenderCompactionMetrics(presentation.ToolEventContext, int, *stream.CompactBoundaryEvent)
			}); ok {
				machine.RenderCompactionMetrics(toolEventContext(event), event.TurnCount, event.Compact)
			}
		}
	}
}
