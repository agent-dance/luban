package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/internal/contracts/interaction"
	"github.com/agent-dance/luban/internal/contracts/stream"

	"github.com/agent-dance/luban/internal/presentation"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/observability"

	"github.com/grindlemire/go-tui"

	"github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

// Compile-time check that TuiRenderer implements presentation.Renderer.
var _ presentation.Renderer = (*TuiRenderer)(nil)

// TuiRenderer implements presentation.Renderer by bridging to go-tui's reactive state.
// When external code (engine, commands, permissions) calls Renderer methods,
// TuiRenderer updates State[T] → go-tui auto-redraws → terminal output.
type TuiRenderer struct {
	app          *tui.App
	enqueue      func(func()) bool
	state        *AppState
	catalog      *provider.ModelCatalog // optional; enriches Banner with context/pricing
	goodbyeOnce  sync.Once              // prevents double Goodbye() from spawning multiple stop goroutines
	cacheDebug   ui.CacheBreakDebugDetector
	briefMu      sync.Mutex
	decisionMu   sync.Mutex // serializes terminal audit commits, never user waits
	decisionOnce sync.Once
	decisions    *decisionBroker
	briefText    bool
	briefTurn    int
	currentTurn  int
}

// RuntimeLanguage returns the active state language for final-boundary
// RuntimeEvent projection.
func (r *TuiRenderer) RuntimeLanguage() i18n.Language {
	if r == nil || r.state == nil {
		return i18n.DetectOrLoadLanguage()
	}
	return r.state.Language.Get()
}

func (r *TuiRenderer) VisibleSessionID() string {
	return r.state.SessionID.Get()
}

// HasSubagentObservation reports whether this dynamic renderer can present a
// background Agent completion by updating the original tool observation. It is
// intentionally narrow so linear renderers can keep their notification
// fallback while the full-screen TUI avoids appending a duplicate transcript
// row for work that already has a stable card.
func (r *TuiRenderer) HasSubagentObservation(sessionID, parentToolUseID string) bool {
	if r == nil || r.state == nil || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(parentToolUseID) == "" {
		return false
	}
	observation, ok := r.state.GetObservation(toolObservationID(sessionID, parentToolUseID))
	return ok && strings.EqualFold(strings.TrimSpace(observation.ToolName), "Agent")
}

// AcknowledgeSubagentResult clears the transient result attention only after
// the model follow-up has successfully consumed that detached result.
func (r *TuiRenderer) AcknowledgeSubagentResult(taskID string) bool {
	if r == nil || r.state == nil || strings.TrimSpace(taskID) == "" {
		return false
	}
	var acknowledgeErr error
	if !r.queueUpdateAndWait(func() {
		acknowledgeErr = r.state.AcknowledgeActivity("background:" + strings.TrimSpace(taskID))
	}) {
		return false
	}
	return acknowledgeErr == nil
}

// NewTuiRenderer creates a TuiRenderer backed by the given go-tui App and state.
// catalog may be nil if no model metadata enrichment is desired.
func NewTuiRenderer(app *tui.App, state *AppState, catalog *provider.ModelCatalog) *TuiRenderer {
	return &TuiRenderer{app: app, state: state, catalog: catalog, enqueue: app.QueueUpdateLossless}
}

func (r *TuiRenderer) queueUpdate(fn func()) bool {
	if r.enqueue != nil {
		return r.enqueue(func() { r.batch(fn) })
	}
	return false
}

func (r *TuiRenderer) queueEpochUpdate(epoch uint64, fn func()) bool {
	return r.queueUpdate(func() {
		if !r.state.AdmitEpoch(epoch) {
			observability.RecordGenerationDrop(observability.GenerationSurfaceTUIEpoch)
			return
		}
		fn()
	})
}

func (r *TuiRenderer) queueContextUpdate(ctx presentation.ToolEventContext, fn func()) bool {
	return r.queueUpdate(func() {
		if !r.AdmitContextGeneration(ctx) {
			observability.RecordGenerationDrop(observability.GenerationSurfaceTUITool)
			return
		}
		fn()
	})
}

// CommitContextGeneration is an ordered reducer barrier. Because the commit
// closure is queued after all old-generation event closures and waits for its
// own execution, no pending event can be reclassified under the new durable
// generation.
func (r *TuiRenderer) CommitContextGeneration(ctx presentation.ToolEventContext, generation uint64, persisted bool) bool {
	if (persisted && generation == 0) || (!persisted && generation != 0) {
		return false
	}
	committed := false
	if !r.queueUpdateAndWait(func() {
		if !r.AdmitContextGeneration(ctx) {
			observability.RecordGenerationDrop(observability.GenerationSurfaceTUITool)
			return
		}
		committed = r.state.PublishContextGenerationState(ctx.SessionID, ctx.SessionEpoch, generation, persisted)
	}) {
		return false
	}
	return committed
}

func (r *TuiRenderer) queueUpdateAndWait(fn func()) bool {
	done := make(chan struct{})
	if !r.queueUpdate(func() {
		fn()
		close(done)
	}) {
		return false
	}
	select {
	case <-done:
		return true
	case <-r.state.stopCh:
		return false
	}
}

func (r *TuiRenderer) batch(fn func()) {
	if r.app != nil {
		r.app.Batch(fn)
		return
	}
	fn()
}

func (r *TuiRenderer) AdmitSessionEpoch(epoch uint64) bool {
	return r.state.AdmitEpoch(epoch)
}

func (r *TuiRenderer) AdmitContextGeneration(ctx presentation.ToolEventContext) bool {
	if r.state.SessionID.Get() != ctx.SessionID {
		return false
	}
	return r.state.AdmitRuntimeGeneration(ctx.SessionEpoch, ctx.ContextGeneration, ctx.ContextGenerationPersisted)
}

// --- Text output (presentation.Renderer interface) ---

func (r *TuiRenderer) Text(s string) {
	r.TextAtEpoch(r.state.SessionEpoch.Get(), s)
}

func (r *TuiRenderer) TextAtEpoch(epoch uint64, s string) {
	r.briefMu.Lock()
	r.briefText = true
	r.briefTurn = r.currentTurn
	turnCount := r.currentTurn
	r.briefMu.Unlock()
	r.queueEpochUpdate(epoch, func() {
		r.state.SetLLMActivity(LLMStageResponse, "")
		r.state.AppendOrStreamTextForTurn(s, turnCount)
	})
}

func (r *TuiRenderer) TextAtContext(ctx presentation.ToolEventContext, s string) {
	r.briefMu.Lock()
	r.briefText = true
	r.briefTurn = r.currentTurn
	turnCount := r.currentTurn
	r.briefMu.Unlock()
	r.queueContextUpdate(ctx, func() {
		r.state.SetLLMActivity(LLMStageResponse, "")
		r.state.AppendOrStreamTextForTurn(s, turnCount)
	})
}

func (r *TuiRenderer) SetRenderTurn(turnCount int) {
	r.briefMu.Lock()
	r.currentTurn = turnCount
	r.briefMu.Unlock()
}

// finalizeCurrentStream ends the active streaming assistant message's
// StreamRenderer (if any). This must be called before any non-Text event
// (ToolCall, Thinking, Error, etc.) to ensure the previous text block gets
// its final Markdown render pass.
func (r *TuiRenderer) finalizeCurrentStream() {
	r.state.FinalizeStream()
}

func (r *TuiRenderer) Thinking(s string) {
	r.ThinkingAtEpoch(r.state.SessionEpoch.Get(), s)
}

func (r *TuiRenderer) ThinkingAtEpoch(epoch uint64, s string) {
	r.briefMu.Lock()
	turnCount := r.currentTurn
	r.briefMu.Unlock()
	r.queueEpochUpdate(epoch, func() {
		r.state.SetLLMActivity(LLMStageThinking, "")
		r.finalizeCurrentStream()
		r.state.AppendOrMergeThinkingForTurn(s, turnCount)
	})
}

func (r *TuiRenderer) ThinkingAtContext(ctx presentation.ToolEventContext, s string) {
	r.briefMu.Lock()
	turnCount := r.currentTurn
	r.briefMu.Unlock()
	r.queueContextUpdate(ctx, func() {
		r.state.SetLLMActivity(LLMStageThinking, "")
		r.finalizeCurrentStream()
		r.state.AppendOrMergeThinkingForTurn(s, turnCount)
	})
}

func (r *TuiRenderer) Error(s string) {
	r.ErrorAtEpoch(r.state.SessionEpoch.Get(), s)
}

func (r *TuiRenderer) ErrorAtEpoch(epoch uint64, s string) {
	r.resetBriefText()
	r.queueEpochUpdate(epoch, func() {
		r.state.AppendMessage(Message{
			Kind:      MsgError,
			Text:      s,
			Timestamp: time.Now(),
		})
	})
}

func (r *TuiRenderer) ErrorAtContext(ctx presentation.ToolEventContext, s string) {
	r.resetBriefText()
	r.queueContextUpdate(ctx, func() {
		r.state.AppendMessage(Message{Kind: MsgError, Text: s, Timestamp: time.Now()})
	})
}

func (r *TuiRenderer) RuntimeErrorEvent(ctx presentation.ToolEventContext, toolUseID, text string, apiError *types.APIError, metadata map[string]any) {
	event := presentation.NewRuntimeErrorEvent(ctx, toolUseID, text, apiError, metadata)
	r.queueEpochUpdate(ctx.SessionEpoch, func() {
		if !r.AdmitContextGeneration(ctx) {
			observability.RecordGenerationDrop(observability.GenerationSurfaceTUITool)
			return
		}
		if err := r.state.ApplyRuntimeEvent(event); err != nil {
			r.state.AppendMessage(Message{Kind: MsgError, Text: i18n.Text(r.state.Language.Get(), i18n.KeyRuntimeErrorPublicSummary), Timestamp: time.Now(), ToolUseID: toolUseID, WorkUnitID: ctx.WorkUnitID, ActorID: ctx.ActorID, Outcome: OutcomeFailed})
		}
	})
}

func (r *TuiRenderer) RenderHookSummary(ctx presentation.ToolEventContext, summary presentation.HookSummary) {
	r.queueEpochUpdate(ctx.SessionEpoch, func() {
		if !r.AdmitContextGeneration(ctx) {
			observability.RecordGenerationDrop(observability.GenerationSurfaceTUITool)
			return
		}
		if err := r.state.ApplyHookSummary(toolObservationContext(ctx, OutcomeRunning), ctx.SessionEpoch, summary); err != nil {
			r.state.AppendMessage(Message{Kind: MsgError, Text: i18n.Format(r.state.Language.Get(), i18n.KeyRuntimeHookEvidenceRetention, err), Timestamp: time.Now(), WorkUnitID: ctx.WorkUnitID, ActorID: ctx.ActorID, Outcome: OutcomeFailed})
		}
	})
}

func (r *TuiRenderer) Info(s string) {
	r.InfoAtEpoch(r.state.SessionEpoch.Get(), s)
}

func (r *TuiRenderer) InfoAtEpoch(epoch uint64, s string) {
	r.queueEpochUpdate(epoch, func() {
		r.state.AppendMessage(Message{
			Kind:      MsgInfo,
			Text:      s,
			Timestamp: time.Now(),
		})
	})
}

func (r *TuiRenderer) InfoAtContext(ctx presentation.ToolEventContext, s string) {
	r.queueContextUpdate(ctx, func() {
		r.state.AppendMessage(Message{Kind: MsgInfo, Text: s, Timestamp: time.Now()})
	})
}

// TryInfoForVisibleSession performs the identity check and message commit in
// one renderer-queue transaction. A session switch between producer lookup
// and delivery therefore cannot retarget an old notification to the new UI.
func (r *TuiRenderer) TryInfoForVisibleSession(sessionID, message string) bool {
	if r == nil || r.state == nil || strings.TrimSpace(sessionID) == "" {
		return false
	}
	delivered := false
	if !r.queueUpdateAndWait(func() {
		if r.state.SessionID.Get() != sessionID {
			observability.RecordGenerationDrop(observability.GenerationSurfaceNotificationSession)
			return
		}
		r.state.AppendMessage(Message{Kind: MsgInfo, Text: message, Timestamp: time.Now()})
		delivered = true
	}) {
		return false
	}
	return delivered
}

func (r *TuiRenderer) Success(s string) {
	r.queueUpdate(func() {
		r.state.AppendMessage(Message{
			Kind:      MsgSuccess,
			Text:      s,
			Timestamp: time.Now(),
		})
	})
}

func (r *TuiRenderer) Warning(s string) {
	r.queueUpdate(func() {
		r.state.AppendMessage(Message{
			Kind:      MsgWarning,
			Text:      s,
			Timestamp: time.Now(),
		})
	})
}

func (r *TuiRenderer) Bold(s string) {
	r.queueUpdate(func() {
		r.state.AppendMessage(Message{
			Kind:      MsgInfo,
			Text:      s,
			Timestamp: time.Now(),
		})
	})
}

// --- Structured output ---

// RenderToolCall receives the complete identity-aware tool event from the TUI
// adapter. The observation reducer owns correlation; the renderer never scans
// neighboring messages.
func (r *TuiRenderer) RenderToolCall(ctx presentation.ToolEventContext, call types.ToolUseBlock) {
	r.queueUpdate(func() {
		if !r.AdmitContextGeneration(ctx) {
			observability.RecordGenerationDrop(observability.GenerationSurfaceTUITool)
			return
		}
		r.state.SetLLMActivity(LLMStageToolExecution, call.Name)
		r.finalizeCurrentStream()
		if err := r.state.ApplyToolCall(toolObservationContext(ctx, OutcomeRunning), call); err != nil {
			r.state.AppendMessage(Message{Kind: MsgError, Text: i18n.Format(r.state.Language.Get(), i18n.KeyRuntimeToolCallPresentation, err), Timestamp: time.Now(), ToolUseID: call.ID, WorkUnitID: ctx.WorkUnitID, ActorID: ctx.ActorID})
		}
	})
}

// RenderToolResult updates the observation identified by ToolUseID and retains
// the complete result in the detail store before projecting a summary.
func (r *TuiRenderer) RenderToolResult(ctx presentation.ToolEventContext, result types.ToolResultBlock) {
	outcome := observationOutcomeForResult(result)
	r.queueUpdate(func() {
		if !r.AdmitContextGeneration(ctx) {
			observability.RecordGenerationDrop(observability.GenerationSurfaceTUITool)
			return
		}
		r.finalizeCurrentStream()
		if outcome == OutcomeUnknown {
			r.appendInvalidToolResult(result, ctx)
			return
		}
		if err := r.state.ApplyToolResult(toolObservationContext(ctx, outcome), result); err != nil {
			r.state.AppendMessage(Message{Kind: MsgError, Text: i18n.Format(r.state.Language.Get(), i18n.KeyRuntimeToolResultRetention, err), Timestamp: time.Now(), ToolUseID: result.ToolUseID, WorkUnitID: ctx.WorkUnitID, ActorID: ctx.ActorID, Outcome: outcome})
		}
	})
}

func (r *TuiRenderer) appendInvalidToolResult(result types.ToolResultBlock, ctx presentation.ToolEventContext) {
	cause := i18n.Text(r.state.Language.Get(), i18n.KeyRuntimeEventInvalid)
	r.state.AppendMessage(Message{
		Kind: MsgError, Text: i18n.Format(r.state.Language.Get(), i18n.KeyRuntimeToolResultRetention, cause),
		Timestamp: time.Now(), ToolUseID: result.ToolUseID, WorkUnitID: ctx.WorkUnitID, ActorID: ctx.ActorID,
	})
}

func observationOutcomeForResult(result types.ToolResultBlock) ObservationOutcome {
	switch result.Outcome {
	case types.ToolOutcomeSucceeded:
		return OutcomeSucceeded
	case types.ToolOutcomeFailed:
		return OutcomeFailed
	case types.ToolOutcomePartial:
		return OutcomePartial
	case types.ToolOutcomeDenied:
		return OutcomeDenied
	case types.ToolOutcomeCancelled:
		return OutcomeCancelled
	case types.ToolOutcomeTimedOut:
		return OutcomeTimedOut
	default:
		return OutcomeUnknown
	}
}

func toolObservationContext(ctx presentation.ToolEventContext, outcome ObservationOutcome) ToolEventContext {
	return ToolEventContext{
		SessionID:  ctx.SessionID,
		TurnID:     ctx.TurnID,
		WorkUnitID: ctx.WorkUnitID,
		ActorID:    ctx.ActorID,
		ActorType:  ctx.ActorType,
		Outcome:    outcome,
	}
}

// RenderSendUserMessage bypasses generic tool call/result rows and replaces
// redundant same-turn assistant text with the typed Brief output.
func (r *TuiRenderer) RenderSendUserMessage(output interaction.SendUserMessageOutput, options presentation.SendUserMessageRenderOptions) {
	r.RenderSendUserMessageEvent(presentation.ToolEventContext{SessionEpoch: r.state.SessionEpoch.Get()}, output, options)
}

func (r *TuiRenderer) RenderSendUserMessageEvent(ctx presentation.ToolEventContext, output interaction.SendUserMessageOutput, options presentation.SendUserMessageRenderOptions) {
	r.renderSendUserMessageEvent(ctx, types.ToolResultBlock{}, output, options, false)
}

func (r *TuiRenderer) RenderHiddenToolCall(ctx presentation.ToolEventContext, call types.ToolUseBlock) {
	r.queueEpochUpdate(ctx.SessionEpoch, func() {
		if ctx.SessionID != "" && !r.AdmitContextGeneration(ctx) {
			observability.RecordGenerationDrop(observability.GenerationSurfaceTUITool)
			return
		}
		if err := r.state.ApplyHiddenToolCall(toolObservationContext(ctx, OutcomeRunning), call); err != nil {
			r.state.AppendMessage(Message{Kind: MsgError, Text: i18n.Format(r.state.Language.Get(), i18n.KeyRuntimeHiddenToolCall, err), ToolUseID: call.ID})
		}
	})
}

func (r *TuiRenderer) RenderSendUserMessageToolEvent(ctx presentation.ToolEventContext, result types.ToolResultBlock, output interaction.SendUserMessageOutput, options presentation.SendUserMessageRenderOptions) {
	r.renderSendUserMessageEvent(ctx, result, output, options, true)
}

func (r *TuiRenderer) renderSendUserMessageEvent(ctx presentation.ToolEventContext, result types.ToolResultBlock, output interaction.SendUserMessageOutput, options presentation.SendUserMessageRenderOptions, retain bool) {
	outcome := OutcomeUnknown
	if retain {
		outcome = observationOutcomeForResult(result)
		if outcome == OutcomeUnknown {
			r.queueEpochUpdate(ctx.SessionEpoch, func() {
				if ctx.SessionID != "" && !r.AdmitContextGeneration(ctx) {
					observability.RecordGenerationDrop(observability.GenerationSurfaceTUITool)
					return
				}
				r.appendInvalidToolResult(result, ctx)
			})
			return
		}
	}
	r.briefMu.Lock()
	options.TurnCount = r.currentTurn
	options.DropAssistantText = r.briefText && r.briefTurn == r.currentTurn
	r.briefText = false
	r.briefMu.Unlock()
	r.queueEpochUpdate(ctx.SessionEpoch, func() {
		if ctx.SessionID != "" && !r.AdmitContextGeneration(ctx) {
			observability.RecordGenerationDrop(observability.GenerationSurfaceTUITool)
			return
		}
		if retain {
			observation, err := r.state.ApplyHiddenToolResult(toolObservationContext(ctx, outcome), result)
			if err != nil {
				r.state.AppendMessage(Message{Kind: MsgError, Text: i18n.Format(r.state.Language.Get(), i18n.KeyRuntimeHiddenToolResult, err), ToolUseID: result.ToolUseID})
			}
			r.state.AppendSendUserMessage(output, options, observation)
			return
		}
		r.state.AppendSendUserMessage(output, options)
	})
}

func (r *TuiRenderer) resetBriefText() {
	r.briefMu.Lock()
	r.briefText = false
	r.briefMu.Unlock()
}

// Usage accumulates completed-turn token/cache usage into session totals.
// Cost tracking still flows through CostSummary(), but token counters belong
// here because TurnEnd provides the final per-turn Usage object.
func (r *TuiRenderer) Usage(u *types.Usage) {
	r.UsageAtEpoch(r.state.SessionEpoch.Get(), u)
}

func (r *TuiRenderer) UsageAtEpoch(epoch uint64, u *types.Usage) {
	r.resetBriefText()
	if u == nil {
		return
	}
	cacheDebugMessage := ""
	if ui.CacheBreakDebugEnabled() {
		cacheDebugMessage = r.cacheDebug.CheckInLanguage(r.state.Language.Get(), u)
	}
	r.queueEpochUpdate(epoch, func() {
		r.batch(func() {
			r.state.AccumulateSessionUsage(u)
			if cacheDebugMessage != "" {
				r.state.AppendMessage(Message{
					Kind:      MsgInfo,
					Text:      cacheDebugMessage,
					Timestamp: time.Now(),
				})
			}
		})
	})
}

func (r *TuiRenderer) UsageAtContext(ctx presentation.ToolEventContext, u *types.Usage) {
	r.resetBriefText()
	if u == nil {
		return
	}
	cacheDebugMessage := ""
	if ui.CacheBreakDebugEnabled() {
		cacheDebugMessage = r.cacheDebug.CheckInLanguage(r.state.Language.Get(), u)
	}
	r.queueContextUpdate(ctx, func() {
		r.batch(func() {
			r.state.AccumulateSessionUsage(u)
			if cacheDebugMessage != "" {
				r.state.AppendMessage(Message{Kind: MsgInfo, Text: cacheDebugMessage, Timestamp: time.Now()})
			}
		})
	})
}

func (r *TuiRenderer) FreezeAggregatesAtEpoch(epoch uint64, sessionID, turnID string) {
	r.queueEpochUpdate(epoch, func() {
		r.state.FreezeObservationAggregates(sessionID, turnID)
	})
}

func (r *TuiRenderer) FreezeAggregatesAtContext(ctx presentation.ToolEventContext) {
	r.queueContextUpdate(ctx, func() {
		r.state.FreezeObservationAggregates(ctx.SessionID, ctx.TurnID)
	})
}

// --- Chrome ---

func (r *TuiRenderer) Banner(provider, model string) {
	// Pre-compute catalog enrichment outside QueueUpdate to avoid blocking event loop
	var ctxK string
	var costIn, costOut float64
	costCurrency := "USD"
	canSeeImages := false
	if r.catalog != nil {
		if info, ok := r.catalog.ResolveForProvider(provider, model); ok {
			if info.ContextWindow > 0 {
				ctxK = fmtContextWindow(info.ContextWindow)
			}
			costIn = info.CostPer1MIn
			costOut = info.CostPer1MOut
			costCurrency = info.BillingCurrency()
			canSeeImages = info.CanSeeImages
		}
	}
	r.queueUpdate(func() {
		r.batch(func() {
			r.state.Provider.Set(provider)
			r.state.Model.Set(model)
			r.state.ContextWindowK.Set(ctxK)
			r.state.ModelCostIn.Set(costIn)
			r.state.ModelCostOut.Set(costOut)
			r.state.ModelCostCurrency.Set(costCurrency)
			r.state.ModelCanSeeImages.Set(canSeeImages)
		})
	})
}

// SetReasoningEffort updates the reasoning effort displayed in terminal chrome.
func (r *TuiRenderer) SetReasoningEffort(effort string) {
	r.queueUpdate(func() {
		r.state.ReasoningEffort.Set(effort)
	})
}

func (r *TuiRenderer) SessionInfo(id string, tools []string) {
	r.queueUpdate(func() {
		r.batch(func() {
			r.state.ApplySessionInfo(id, tools)
		})
	})
}

func (r *TuiRenderer) Prompt() string {
	return "> "
}

func (r *TuiRenderer) Newline() {
	// In TUI mode, newlines are managed by the layout system.
	// This is a no-op.
}

func (r *TuiRenderer) Goodbye() {
	r.goodbyeOnce.Do(func() {
		// Synchronize with the event loop: ensure the goodbye message is committed
		// to state before we stop the app.
		done := make(chan struct{})
		r.queueUpdate(func() {
			r.state.AppendMessage(Message{
				Kind:      MsgInfo,
				Text:      i18n.Text(r.state.Language.Get(), i18n.KeyGoodbye),
				Timestamp: time.Now(),
			})
			close(done)
		})
		go func() {
			// Wait for the goodbye message to be committed, with a timeout
			// to prevent goroutine leak if QueueUpdate never executes
			// (e.g. app already stopped due to Ctrl+C race).
			select {
			case <-done: // message committed to state
			case <-time.After(500 * time.Millisecond): // timeout: app likely dead
			}
			// Wait ~3 frames for the goodbye message to paint before stopping.
			time.Sleep(50 * time.Millisecond)
			r.app.Stop()
		}()
	})
}

// --- Cost / Context ---

func (r *TuiRenderer) CostSummary(turnCost, cumulativeCost float64, inputTokens, outputTokens int) {
	r.CostSummaryAtEpoch(r.state.SessionEpoch.Get(), turnCost, cumulativeCost, inputTokens, outputTokens)
}

func (r *TuiRenderer) CostSummaryAtEpoch(epoch uint64, turnCost, cumulativeCost float64, inputTokens, outputTokens int) {
	r.queueEpochUpdate(epoch, func() {
		r.state.SetSessionCost(cumulativeCost)
	})
}

func (r *TuiRenderer) CostSummaryAtContext(ctx presentation.ToolEventContext, turnCost, cumulativeCost float64, inputTokens, outputTokens int) {
	r.queueContextUpdate(ctx, func() { r.state.SetSessionCost(cumulativeCost) })
}

func (r *TuiRenderer) CostKnownAtEpoch(epoch uint64, known bool) {
	r.queueEpochUpdate(epoch, func() {
		r.state.SetSessionCostKnown(known)
	})
}

func (r *TuiRenderer) CostKnownAtContext(ctx presentation.ToolEventContext, known bool) {
	r.queueContextUpdate(ctx, func() { r.state.SetSessionCostKnown(known) })
}

// SessionUsageAtContext replaces the durable session ledger from the tracker.
// It includes auxiliary and discarded provider calls that do not flow through
// the ordinary turn renderer.
func (r *TuiRenderer) SessionUsageAtContext(ctx presentation.ToolEventContext, usage presentation.SessionUsageProjection) {
	r.queueContextUpdate(ctx, func() {
		r.batch(func() {
			r.state.ApplySessionUsageProjection(usage)
		})
	})
}

func (r *TuiRenderer) ContextBar(usedTokens, maxTokens int) {
	r.ContextBarAtEpoch(r.state.SessionEpoch.Get(), usedTokens, maxTokens)
}

func (r *TuiRenderer) ContextBarAtEpoch(epoch uint64, usedTokens, maxTokens int) {
	r.queueEpochUpdate(epoch, func() {
		r.batch(func() {
			r.state.UsedTokens.Set(usedTokens)
			r.state.MaxTokens.Set(maxTokens)
			measurement := presentation.ContextMeasurementProviderReported
			if maxTokens <= 0 {
				measurement = presentation.ContextMeasurementUnknown
			}
			r.state.ContextMeasurement.Set(measurement)
		})
	})
}

func (r *TuiRenderer) ContextBarAtContext(ctx presentation.ToolEventContext, usedTokens, maxTokens int) {
	r.queueContextUpdate(ctx, func() {
		r.batch(func() {
			r.state.UsedTokens.Set(usedTokens)
			r.state.MaxTokens.Set(maxTokens)
			measurement := presentation.ContextMeasurementProviderReported
			if maxTokens <= 0 {
				measurement = presentation.ContextMeasurementUnknown
			}
			r.state.ContextMeasurement.Set(measurement)
		})
	})
}

func (r *TuiRenderer) ModelContext(context presentation.ModelContextProjection) {
	r.ModelContextAtEpoch(r.state.SessionEpoch.Get(), context)
}

func (r *TuiRenderer) ModelContextAtEpoch(epoch uint64, context presentation.ModelContextProjection) {
	r.queueEpochUpdate(epoch, func() {
		r.batch(func() {
			r.state.UsedTokens.Set(context.UsedTokens)
			r.state.MaxTokens.Set(context.CapacityTokens)
			r.state.ContextMeasurement.Set(context.Measurement)
		})
	})
}

func (r *TuiRenderer) ModelContextAtContext(ctx presentation.ToolEventContext, context presentation.ModelContextProjection) {
	r.queueContextUpdate(ctx, func() {
		r.batch(func() {
			r.state.UsedTokens.Set(context.UsedTokens)
			r.state.MaxTokens.Set(context.CapacityTokens)
			r.state.ContextMeasurement.Set(context.Measurement)
		})
	})
}

// SetProviderStatus updates the provider connection status indicator.
func (r *TuiRenderer) SetProviderStatus(status ProviderStatus) {
	r.SetProviderStatusAtEpoch(r.state.SessionEpoch.Get(), status)
}

func (r *TuiRenderer) SetProviderStatusAtEpoch(epoch uint64, status ProviderStatus) {
	r.queueEpochUpdate(epoch, func() {
		r.state.ProvStatus.Set(status)
	})
}

func (r *TuiRenderer) SetProviderStatusAtContext(ctx presentation.ToolEventContext, status ProviderStatus) {
	r.queueContextUpdate(ctx, func() { r.state.ProvStatus.Set(status) })
}

// GoalStatusAtEpoch applies the query loop's final goal projection without
// performing persistence I/O on the UI event loop.
func (r *TuiRenderer) GoalStatusAtEpoch(epoch uint64, event stream.GoalStatusEvent) {
	r.queueEpochUpdate(epoch, func() {
		r.state.SetGoalView(goalViewFromStatusEvent(event))
	})
}

func (r *TuiRenderer) GoalStatusAtContext(ctx presentation.ToolEventContext, event stream.GoalStatusEvent) {
	r.queueContextUpdate(ctx, func() { r.state.SetGoalView(goalViewFromStatusEvent(event)) })
}

func goalViewFromStatusEvent(event stream.GoalStatusEvent) *GoalViewState {
	view := &GoalViewState{Status: event.Status, Objective: event.Objective, Revision: event.Revision}
	for _, criterion := range event.Criteria {
		view.Criteria = append(view.Criteria, GoalCriterionViewState{
			ID: criterion.ID, Text: criterion.Text, Status: criterion.Status, Reason: criterion.Reason,
		})
	}
	return normalizeGoalView(view)
}

func (r *TuiRenderer) LLMRequestStatusAtEpoch(epoch uint64, eventType stream.EventType, event stream.RequestStatusEvent) {
	r.queueEpochUpdate(epoch, func() { r.applyLLMRequestStatus(eventType, event) })
}

func (r *TuiRenderer) LLMRequestStatusAtContext(ctx presentation.ToolEventContext, eventType stream.EventType, event stream.RequestStatusEvent) {
	r.queueContextUpdate(ctx, func() { r.applyLLMRequestStatus(eventType, event) })
}

func (r *TuiRenderer) LLMActivityAtEpoch(epoch uint64, stage LLMCallStage, detail string, toolInputBytes ...int) {
	r.queueEpochUpdate(epoch, func() { r.state.SetLLMActivity(stage, detail, toolInputBytes...) })
}

func (r *TuiRenderer) LLMActivityAtContext(ctx presentation.ToolEventContext, stage LLMCallStage, detail string, toolInputBytes ...int) {
	r.queueContextUpdate(ctx, func() { r.state.SetLLMActivity(stage, detail, toolInputBytes...) })
}

func (r *TuiRenderer) applyLLMRequestStatus(eventType stream.EventType, event stream.RequestStatusEvent) {
	now := time.Now()
	current := r.state.LLMCall.Get()
	workStartedAt := now.Add(-time.Duration(event.TotalMilliseconds) * time.Millisecond)
	if current != nil && !current.WorkStartedAt.IsZero() {
		workStartedAt = current.WorkStartedAt
	}
	switch eventType {
	case stream.EventRequestStart:
		if current != nil && current.Phase == LLMCallRetrying {
			// Match Codex's StreamError status lifecycle: keep the reconnect
			// header and underlying problem visible while the replacement request
			// waits for output. The new request ID is still adopted so its first
			// token can prove recovery and clear the problem state.
			updated := *current
			updated.RequestID = event.RequestID
			updated.Attempt = event.Attempt
			if event.MaxRetries > 0 {
				updated.MaxRetries = event.MaxRetries
			}
			if event.RetryCount > 0 {
				updated.RetryCount = event.RetryCount
			}
			updated.RequestDuration = time.Duration(event.RequestMilliseconds) * time.Millisecond
			updated.HasRequestDuration = true
			updated.TotalDuration = time.Duration(event.TotalMilliseconds) * time.Millisecond
			updated.UpdatedAt = now
			updated.WorkStartedAt = workStartedAt
			r.state.SetLLMCall(&updated)
			return
		}
		stage := LLMStageWaitingFirstToken
		stageStartedAt := now
		if current != nil && (current.Stage == LLMStageToolExecution || current.Stage == LLMStageWaitingAfterTools) {
			stage = LLMStageWaitingAfterTools
			if current.Stage == stage && !current.StageStartedAt.IsZero() {
				stageStartedAt = current.StageStartedAt
			}
		}
		r.state.SetLLMCall(&LLMCallStatus{
			RequestID: event.RequestID, Phase: LLMCallWorking, Stage: stage, StageStartedAt: stageStartedAt,
			Attempt: event.Attempt, MaxRetries: event.MaxRetries, RetryCount: event.RetryCount,
			RequestDuration: time.Duration(event.RequestMilliseconds) * time.Millisecond, HasRequestDuration: true,
			TotalDuration: time.Duration(event.TotalMilliseconds) * time.Millisecond, UpdatedAt: now, WorkStartedAt: workStartedAt,
		})
	case stream.EventRequestRetry:
		retryCount := event.RetryCount
		if retryCount <= 0 && event.Attempt > 0 {
			// RequestRetry.Attempt historically represented the failed request
			// that caused this retry. Keep older event producers unambiguous while
			// RetryCount remains the canonical counter for current producers.
			retryCount = event.Attempt
		}
		r.state.SetLLMCall(&LLMCallStatus{
			RequestID: event.RequestID, Phase: LLMCallRetrying, Attempt: event.Attempt, MaxRetries: event.MaxRetries, RetryCount: retryCount,
			RetryDelay: time.Duration(event.RetryDelayMilliseconds) * time.Millisecond, TotalDuration: time.Duration(event.TotalMilliseconds) * time.Millisecond,
			UpdatedAt: now, WorkStartedAt: workStartedAt, Error: event.Error, RetryKind: event.RetryKind,
		})
	case stream.EventRequestFirstToken:
		if current == nil || current.RequestID != event.RequestID {
			return
		}
		updated := *current
		updated.Phase = LLMCallWorking
		updated.FirstTokenDuration = time.Duration(event.FirstTokenMilliseconds) * time.Millisecond
		updated.HasFirstToken = true
		updated.TotalDuration = time.Duration(event.TotalMilliseconds) * time.Millisecond
		updated.UpdatedAt = now
		if updated.Stage == LLMStagePreparing || updated.Stage == LLMStageWaitingFirstToken {
			// The first-token metric proves the wait ended, but does not say
			// whether the content is reasoning, text, or tool input. The next
			// content event supplies that honest stage boundary.
			updated.Stage = ""
			updated.StageDetail = ""
			updated.StageStartedAt = time.Time{}
		}
		r.state.SetLLMCall(&updated)
	case stream.EventRequestFailed:
		if current == nil || current.RequestID != event.RequestID {
			return
		}
		updated := *current
		updated.Phase = LLMCallProblem
		updated.TotalDuration = time.Duration(event.TotalMilliseconds) * time.Millisecond
		updated.UpdatedAt = now
		updated.Error = event.Error
		r.state.SetLLMCall(&updated)
	case stream.EventRequestEnd:
		// A completed stream may be a tool-use response. Keep the execution
		// status visible across tool work and subsequent model requests; the
		// query's terminal settlement is the sole clearing boundary.
		if current != nil && current.RequestID == event.RequestID && current.Phase == LLMCallRetrying {
			updated := *current
			updated.Phase = LLMCallWorking
			updated.TotalDuration = time.Duration(event.TotalMilliseconds) * time.Millisecond
			updated.UpdatedAt = now
			r.state.SetLLMCall(&updated)
		}
	}
}

func (r *TuiRenderer) ActivityAtEpoch(epoch uint64, event ActivityEvent) {
	r.queueEpochUpdate(epoch, func() {
		event.Epoch = epoch
		if event.SessionID == "" {
			event.SessionID = r.state.SessionID.Get()
		}
		_ = r.state.ApplyActivity(event)
	})
}

func (r *TuiRenderer) ActivityAtContext(ctx presentation.ToolEventContext, event ActivityEvent) {
	r.queueContextUpdate(ctx, func() {
		event.Epoch = ctx.SessionEpoch
		if event.SessionID == "" {
			event.SessionID = ctx.SessionID
		}
		_ = r.state.ApplyActivity(event)
	})
}

func (r *TuiRenderer) CompactionProgressAtEpoch(epoch uint64, ctx presentation.ToolEventContext, progress stream.ProgressEvent) {
	r.queueEpochUpdate(epoch, func() {
		if !r.AdmitContextGeneration(ctx) {
			observability.RecordGenerationDrop(observability.GenerationSurfaceTUITool)
			return
		}
		if !r.state.ApplyCompactionProgress(ctx.SessionID, epoch, progress) {
			return
		}
		trigger := compactionMetadataString(progress.Metadata, "trigger", "unknown")
		turnID := ctx.TurnID
		if turnID == "" {
			turnID = ctx.SessionID + ":turn"
		}
		activityID := compactionActivityIdentity(ctx.SessionID, turnID, trigger)
		lifecycle, outcome := ActivityLifecycleRunning, OutcomeRunning
		switch strings.ToLower(strings.TrimSpace(progress.Stage)) {
		case "compact_end":
			lifecycle, outcome = ActivityLifecycleCompleted, OutcomeSucceeded
		case "compact_failed":
			lifecycle, outcome = ActivityLifecycleFailed, OutcomeFailed
		case "compact_cancelled":
			lifecycle, outcome = ActivityLifecycleCancelled, OutcomeCancelled
		}
		control := ActivityControl{Cancelable: lifecycle == ActivityLifecycleRunning}
		if lifecycle == ActivityLifecycleFailed || lifecycle == ActivityLifecycleCancelled {
			evidence, err := json.MarshalIndent(struct {
				Kind        string         `json:"kind"`
				SessionID   string         `json:"session_id"`
				ProjectRoot string         `json:"project_root,omitempty"`
				TurnID      string         `json:"turn_id"`
				WorkUnitID  string         `json:"work_unit_id"`
				ActorID     string         `json:"actor_id"`
				ActorType   string         `json:"actor_type"`
				Trigger     string         `json:"trigger"`
				Stage       string         `json:"stage"`
				Message     string         `json:"message"`
				Metadata    map[string]any `json:"metadata,omitempty"`
			}{"context_compaction_terminal", ctx.SessionID, ctx.ProjectRoot, turnID, ctx.WorkUnitID, ctx.ActorID, ctx.ActorType, trigger, progress.Stage, progress.Message, progress.Metadata}, "", "  ")
			if err == nil {
				ref, observationID, retainErr := r.state.RecordActivityEvidenceContextForEpoch(ToolEventContext{
					SessionID: ctx.SessionID, TurnID: turnID, WorkUnitID: ctx.WorkUnitID, ActorID: ctx.ActorID,
				}, epoch, activityID, i18n.Format(r.state.Language.Get(), i18n.KeyTUICompactionTerminal,
					observationOutcomeLabelInLanguage(r.state.Language.Get(), outcome), i18n.TUICompactionTriggerLabel(r.state.Language.Get(), trigger)),
					"compaction:"+activityID+":"+outcome.String(), outcome, evidence)
				if retainErr == nil {
					control.JumpTarget = observationID
					control.DetailRefs = []DetailRef{ref}
				}
			}
		}
		message := progress.Message
		if stage, _, ok := compactionProgressStage(progress.Stage); ok {
			if stage == CompactionProgressCompleted || stage == CompactionProgressFailed || stage == CompactionProgressCancelled {
				message = i18n.TUIOutcomeLabel(r.state.Language.Get(), string(stage))
			} else {
				message = compactProgressStageText(r.state.Language.Get(), stage)
			}
		}
		_ = r.state.ApplyActivity(ActivityEvent{
			ID: activityID, SessionID: ctx.SessionID, Epoch: epoch, TurnID: turnID, WorkUnitID: ctx.WorkUnitID,
			Actor: ActivityActor{ID: ctx.ActorID, Type: ctx.ActorType}, Kind: ActivityBackground,
			Name: i18n.Text(r.state.Language.Get(), i18n.KeyRuntimeContextCompaction), Phase: ActivityPhaseExecuting, Lifecycle: lifecycle, Outcome: outcome,
			Progress: ActivityProgress{Current: progress.Current, Total: progress.Total, Message: message}, Control: control,
		})
	})
}

// ProgressiveContextMetricsAtEpoch publishes the current eligible pending
// snapshot and accumulates only applied, non-shadow replacement receipts.
// Rejected projections never appear as realized user-visible savings.
func (r *TuiRenderer) ProgressiveContextMetricsAtEpoch(epoch uint64, ctx presentation.ToolEventContext, progress stream.ProgressEvent) {
	r.queueEpochUpdate(epoch, func() {
		if !r.AdmitContextGeneration(ctx) {
			observability.RecordGenerationDrop(observability.GenerationSurfaceTUITool)
			return
		}
		sequence := compactionMetadataInt(progress.Metadata, "projection_sequence")
		identity := fmt.Sprintf("%s:%s:%d", ctx.SessionID, ctx.TurnID, sequence)
		r.state.ApplyProgressiveContextMetrics(ctx.SessionID, epoch, identity, progress)
	})
}

func (r *TuiRenderer) CompactionBoundaryAtEpoch(epoch uint64, ctx presentation.ToolEventContext, boundary stream.CompactBoundaryEvent) {
	r.queueEpochUpdate(epoch, func() {
		if !r.AdmitContextGeneration(ctx) {
			observability.RecordGenerationDrop(observability.GenerationSurfaceTUITool)
			return
		}
		boundaryIdentity := presentation.CompactionBoundaryIdentity(ctx, boundary)
		if !r.state.MarkSessionCompactedBoundary(boundaryIdentity) {
			return
		}
		r.state.ApplyCompactionBoundary(ctx.SessionID, epoch, boundary)
		trigger := strings.ToLower(strings.TrimSpace(boundary.Trigger))
		if trigger == "" {
			trigger = "unknown"
		}
		lang := r.state.Language.Get()
		triggerLabel := i18n.TUICompactionTriggerLabel(lang, trigger)
		turnID := ctx.TurnID
		if turnID == "" {
			turnID = ctx.SessionID + ":turn"
		}
		activityID := compactionActivityIdentity(ctx.SessionID, turnID, trigger)
		retained := boundary.TruePostCompactTokenCount
		if retained == 0 {
			retained = boundary.PostCompactTokenCount
		}
		if retained > 0 && r.state.MaxTokens.Get() > 0 {
			r.state.UsedTokens.Set(retained)
			r.state.ContextMeasurement.Set(presentation.ContextMeasurementLocalEstimate)
		} else {
			r.state.UsedTokens.Set(0)
			r.state.ContextMeasurement.Set(presentation.ContextMeasurementUnknown)
		}
		discarded := boundary.PreCompactTokenCount - retained
		if discarded < 0 {
			discarded = 0
		}
		evidence, err := json.MarshalIndent(struct {
			SchemaVersion   int                         `json:"schema_version"`
			Kind            string                      `json:"kind"`
			SessionID       string                      `json:"session_id"`
			ProjectRoot     string                      `json:"project_root,omitempty"`
			TurnID          string                      `json:"turn_id"`
			WorkUnitID      string                      `json:"work_unit_id"`
			ActorID         string                      `json:"actor_id"`
			ActorType       string                      `json:"actor_type"`
			Boundary        stream.CompactBoundaryEvent `json:"boundary"`
			RetainedTokens  int                         `json:"retained_token_estimate"`
			DiscardedTokens int                         `json:"discarded_token_estimate"`
			RetainedRange   string                      `json:"retained_range"`
			DiscardedRange  string                      `json:"discarded_range"`
		}{
			SchemaVersion: 1, Kind: "context_compaction_boundary", SessionID: ctx.SessionID, ProjectRoot: ctx.ProjectRoot,
			TurnID: turnID, WorkUnitID: ctx.WorkUnitID, ActorID: ctx.ActorID, ActorType: ctx.ActorType, Boundary: boundary,
			RetainedTokens: retained, DiscardedTokens: discarded,
			RetainedRange:  i18n.Format(lang, i18n.KeyTUICompactionRetainedRange, retained),
			DiscardedRange: i18n.Format(lang, i18n.KeyTUICompactionDiscardedRange, retained, boundary.PreCompactTokenCount),
		}, "", "  ")
		if err != nil {
			return
		}
		summary := i18n.Format(lang, i18n.KeyTUICompactionSummary, triggerLabel, boundary.PreCompactTokenCount, retained, retained, discarded)
		ref, observationID, err := r.state.RecordActivityEvidenceContextForEpoch(ToolEventContext{
			SessionID: ctx.SessionID, TurnID: turnID, WorkUnitID: ctx.WorkUnitID, ActorID: ctx.ActorID,
		}, epoch, activityID, summary, "compaction:"+activityID+":boundary", OutcomeSucceeded, evidence)
		if err != nil {
			return
		}
		_ = r.state.ApplyActivity(ActivityEvent{
			ID: activityID, SessionID: ctx.SessionID, Epoch: epoch, TurnID: turnID, WorkUnitID: ctx.WorkUnitID,
			Actor: ActivityActor{ID: ctx.ActorID, Type: ctx.ActorType}, Kind: ActivityBackground,
			Name: i18n.Text(r.state.Language.Get(), i18n.KeyRuntimeContextCompaction), Phase: ActivityPhaseExecuting, Lifecycle: ActivityLifecycleCompleted, Outcome: OutcomeSucceeded,
			Progress: ActivityProgress{Message: summary}, Control: ActivityControl{JumpTarget: observationID, DetailRefs: []DetailRef{ref}},
		})
	})
}

func compactionMetadataString(metadata map[string]any, key, fallback string) string {
	if value, ok := metadata[key].(string); ok && strings.TrimSpace(value) != "" {
		return strings.ToLower(strings.TrimSpace(value))
	}
	return fallback
}

func compactionActivityIdentity(sessionID, turnID, trigger string) string {
	return "progress:compaction:" + sessionID + ":" + turnID + ":" + trigger
}

// --- Spinner ---

func (r *TuiRenderer) SpinnerStart(toolName string) func() {
	return r.SpinnerStartAtEpoch(r.state.SessionEpoch.Get(), toolName)
}

func (r *TuiRenderer) SpinnerStartAtEpoch(epoch uint64, toolName string) func() {
	// The stable-ID Activity reducer is the fullscreen progress authority.
	// The renderer interface's name-only spinner hook remains a no-op here.
	return func() {}
}

func (r *TuiRenderer) DecisionRequest(ctx context.Context, request permissions.PromptRequest) permissions.PromptResponse {
	request = clonePromptRequest(request)
	requestEpoch := r.state.SessionEpoch.Get()
	requestSession := request.SessionID
	if requestSession == "" {
		requestSession = r.state.SessionID.Get()
	}
	admitted := false
	var waiter *decisionWaiter
	var blockedAgent *Activity
	var startPump bool
	registration := decisionRegistrationNew
	published := r.queueUpdateAndWait(func() {
		select {
		case <-r.state.stopCh:
			return
		default:
		}
		if !r.state.AdmitEpoch(requestEpoch) || r.state.SessionID.Get() != requestSession {
			observability.RecordGenerationDrop(observability.GenerationSurfaceTUIDecision)
			return
		}
		if ctx.Err() != nil {
			return
		}
		admitted = true
		becameActive := false
		waiter, registration, becameActive, startPump = r.decisionBroker().register(request, requestSession, requestEpoch)
		if registration != decisionRegistrationNew {
			return
		}
		var detailRefs []DetailRef
		jumpTarget := ""
		if payload, err := json.Marshal(request); err == nil {
			activityID := "decision:" + request.DecisionID
			if ref, observationID, retainErr := r.state.RecordActivityEvidenceContextForEpoch(ToolEventContext{
				SessionID: requestSession, TurnID: request.TurnID, WorkUnitID: request.WorkUnitID,
				ActorID: request.ActorID, ActorType: request.ActorType,
			}, requestEpoch, activityID, i18n.Format(r.state.Language.Get(), i18n.KeyRuntimeDecisionEvidenceName, request.Action), activityID+":request", OutcomeRunning, payload); retainErr == nil {
				detailRefs = []DetailRef{ref}
				jumpTarget = observationID
			}
		}
		_ = r.state.ApplyActivity(ActivityEvent{
			ID: "decision:" + request.DecisionID, SessionID: requestSession, Epoch: requestEpoch,
			TurnID: request.TurnID, WorkUnitID: request.WorkUnitID, Actor: ActivityActor{ID: request.ActorID, Type: request.ActorType},
			Kind: ActivityDecision, Name: request.Action, Lifecycle: ActivityLifecycleBlocked, Outcome: OutcomeRunning,
			Attention: ActivityAttention{Kind: ActivityAttentionNeedsInput, Severity: ActivityAttentionSeverityWarning, Unread: true, DecisionID: request.DecisionID},
			Control:   ActivityControl{JumpTarget: jumpTarget, DetailRefs: detailRefs},
		})
		agent, agentFound := r.state.AgentActivityByCorrelation(request.ActorID, request.WorkUnitID)
		if !agentFound {
			agent, agentFound = r.state.AgentActivityByCorrelation(request.ActorID, "")
		}
		if agentFound && !isTerminalActivityLifecycle(agent.Lifecycle) {
			copy := agent
			blockedAgent = &copy
			_ = r.state.ApplyActivity(ActivityEvent{
				ID: agent.ID, RunID: agent.RunID, Attempt: agent.Attempt,
				BatchID: agent.BatchID, ParentRunID: agent.ParentRunID, AgentPath: agent.AgentPath,
				SessionID: requestSession, Epoch: requestEpoch, TurnID: request.TurnID, WorkUnitID: agent.WorkUnitID,
				Actor: agent.Actor, Kind: agent.Kind, Name: agent.Name,
				Lifecycle: ActivityLifecycleBlocked, Outcome: OutcomeRunning,
				Attention: ActivityAttention{Kind: ActivityAttentionNeedsInput, Severity: ActivityAttentionSeverityWarning, Unread: true, DecisionID: request.DecisionID, Message: request.Message},
				Control:   agent.Control,
			})
		}
		if becameActive {
			r.state.DecisionSelected.Set(0)
			active := clonePromptRequest(request)
			r.state.DecisionReq.Set(&active)
		}
	})
	if startPump {
		go r.runDecisionResponsePump(r.decisionBroker())
	}
	if !published && waiter == nil {
		return decisionResponse(request.DecisionID, permissions.PromptOutcomeShutdown, "")
	}
	if !published {
		if registration == decisionRegistrationNew {
			r.decisionBroker().resolve(waiter, decisionResponse(request.DecisionID, permissions.PromptOutcomeShutdown, ""))
		} else {
			return decisionResponse(request.DecisionID, permissions.PromptOutcomeShutdown, "")
		}
	}
	if !admitted {
		response := decisionResponseForContext(ctx)
		response.DecisionID = request.DecisionID
		return response
	}
	if registration == decisionRegistrationCollision {
		return decisionResponse(request.DecisionID, permissions.PromptOutcomeCancelled, "")
	}
	if registration == decisionRegistrationCoalesced {
		return waitForCoalescedDecision(ctx, r.state.stopCh, r.decisionBroker(), waiter, request.DecisionID)
	}

	var response permissions.PromptResponse
	select {
	case response = <-waiter.response:
	case <-ctx.Done():
		fallback := decisionResponseForContext(ctx)
		fallback.DecisionID = request.DecisionID
		r.decisionBroker().resolve(waiter, fallback)
		response = <-waiter.response
	case <-r.state.stopCh:
		r.decisionBroker().resolve(waiter, decisionResponse(request.DecisionID, permissions.PromptOutcomeShutdown, ""))
		response = <-waiter.response
	}
	response.DecisionID = request.DecisionID

	committed := false
	commitDone := make(chan struct{})
	var commitOnce sync.Once
	commitResolution := func() {
		commitOnce.Do(func() {
			defer close(commitDone)
			r.decisionMu.Lock()
			defer r.decisionMu.Unlock()
			broker := r.decisionBroker()
			defer func() {
				final := response
				if !committed {
					final = decisionResponse(request.DecisionID, permissions.PromptOutcomeCancelled, "")
				}
				broker.finalize(waiter, final)
			}()
			transition := broker.complete(waiter)
			publishNextDecisionOverlay(r.state, broker, transition)
			if !r.state.AdmitEpoch(requestEpoch) || r.state.SessionID.Get() != requestSession {
				observability.RecordGenerationDrop(observability.GenerationSurfaceTUIDecision)
				return
			}
			committed = true
			history := append([]DecisionRecord(nil), r.state.DecisionHistory.Get()...)
			history = append(history, DecisionRecord{Prompt: clonePromptRequest(request), Response: response, ResolvedAt: time.Now()})
			r.state.DecisionHistory.Set(history)
			r.state.DecisionReceipt.Set(formatDecisionReceiptInLanguage(r.state.Language.Get(), request, response))
			outcome, lifecycle := OutcomeDenied, ActivityLifecycleFailed
			switch response.Outcome {
			case permissions.PromptOutcomeApproved:
				outcome, lifecycle = OutcomeSucceeded, ActivityLifecycleCompleted
			case permissions.PromptOutcomeRejected:
			case permissions.PromptOutcomeEscaped:
				outcome, lifecycle = OutcomeEscaped, ActivityLifecycleCancelled
			case permissions.PromptOutcomeCancelled:
				outcome, lifecycle = OutcomeCancelled, ActivityLifecycleCancelled
			case permissions.PromptOutcomeTimedOut:
				outcome, lifecycle = OutcomeTimedOut, ActivityLifecycleCancelled
			case permissions.PromptOutcomeShutdown:
				outcome, lifecycle = OutcomeShutdown, ActivityLifecycleCancelled
			}
			_ = r.state.ApplyActivity(ActivityEvent{
				ID: "decision:" + request.DecisionID, SessionID: requestSession, Epoch: requestEpoch,
				TurnID: request.TurnID, WorkUnitID: request.WorkUnitID, Actor: ActivityActor{ID: request.ActorID, Type: request.ActorType},
				Kind: ActivityDecision, Name: request.Action, Lifecycle: lifecycle, Outcome: outcome,
			})
			if blockedAgent != nil {
				current, ok := r.state.Activities.GetRun(blockedAgent.ID, blockedAgent.RunID)
				if ok && !isTerminalActivityLifecycle(current.Lifecycle) {
					if pending, pendingOK := pendingDecisionRequestForAgent(broker, request.ActorID, requestSession, requestEpoch); pendingOK {
						// A keyed response can resolve a queued decision before the
						// visible one. Keep the agent blocked until every decision for
						// it has committed a terminal audit, and project one remaining
						// request as the actionable attention target.
						_ = r.state.ApplyActivity(ActivityEvent{
							ID: current.ID, RunID: current.RunID, Attempt: current.Attempt,
							BatchID: current.BatchID, ParentRunID: current.ParentRunID, AgentPath: current.AgentPath,
							SessionID: requestSession, Epoch: requestEpoch, TurnID: current.TurnID, WorkUnitID: current.WorkUnitID,
							Actor: current.Actor, Kind: current.Kind, Name: current.Name,
							Lifecycle: ActivityLifecycleBlocked, Outcome: OutcomeRunning,
							Attention: ActivityAttention{Kind: ActivityAttentionNeedsInput, Severity: ActivityAttentionSeverityWarning, Unread: true, DecisionID: pending.DecisionID, Message: pending.Message},
							Control:   current.Control,
						})
					} else if current.Attention.DecisionID == request.DecisionID {
						restoredLifecycle := blockedAgent.Lifecycle
						if restoredLifecycle == ActivityLifecycleBlocked {
							restoredLifecycle = ActivityLifecycleRunning
						}
						_ = r.state.ApplyActivity(ActivityEvent{
							ID: blockedAgent.ID, RunID: blockedAgent.RunID, Attempt: blockedAgent.Attempt,
							BatchID: blockedAgent.BatchID, ParentRunID: blockedAgent.ParentRunID, AgentPath: blockedAgent.AgentPath,
							SessionID: requestSession, Epoch: requestEpoch, TurnID: request.TurnID, WorkUnitID: blockedAgent.WorkUnitID,
							Actor: blockedAgent.Actor, Kind: blockedAgent.Kind, Name: blockedAgent.Name,
							Lifecycle: restoredLifecycle, Outcome: blockedAgent.Outcome,
							Attention: ActivityAttention{Kind: ActivityAttentionNone}, Control: blockedAgent.Control,
						})
					}
				}
			}
			if r.app != nil {
				r.app.RequestFullRedraw()
			}
		})
	}
	queued := r.queueUpdate(commitResolution)
	if queued {
		select {
		case <-commitDone:
		case <-r.state.stopCh:
			// The event queue no longer owns progress after shutdown. Commit the
			// terminal audit synchronously; sync.Once arbitrates with a callback
			// that may already have started.
			r.batch(commitResolution)
			<-commitDone
		}
	} else {
		// Once admission succeeded, a rejected terminal queue must still remove
		// the waiter and persist a fail-closed audit. Queue rejection is not a
		// license to leave an invisible active broker entry behind.
		response = decisionResponse(request.DecisionID, permissions.PromptOutcomeShutdown, "")
		r.batch(commitResolution)
		<-commitDone
	}
	if !committed {
		if final, ok := r.decisionBroker().finalResponse(waiter); ok {
			return final
		}
		return decisionResponse(request.DecisionID, permissions.PromptOutcomeCancelled, "")
	}

	if final, ok := r.decisionBroker().finalResponse(waiter); ok {
		return final
	}
	return response
}

func waitForCoalescedDecision(ctx context.Context, stop <-chan struct{}, broker *decisionBroker, waiter *decisionWaiter, decisionID string) permissions.PromptResponse {
	if waiter == nil {
		return decisionResponse(decisionID, permissions.PromptOutcomeCancelled, "")
	}
	select {
	case <-waiter.terminal:
		if response, ok := broker.finalResponse(waiter); ok {
			return response
		}
	default:
	}
	select {
	case <-waiter.terminal:
		if response, ok := broker.finalResponse(waiter); ok {
			return response
		}
	case <-ctx.Done():
		response := decisionResponseForContext(ctx)
		response.DecisionID = decisionID
		return response
	case <-stop:
		return decisionResponse(decisionID, permissions.PromptOutcomeShutdown, "")
	}
	return decisionResponse(decisionID, permissions.PromptOutcomeCancelled, "")
}

func (r *TuiRenderer) decisionBroker() *decisionBroker {
	r.decisionOnce.Do(func() { r.decisions = newDecisionBroker() })
	return r.decisions
}

func pendingDecisionRequestForAgent(broker *decisionBroker, actorID, sessionID string, epoch uint64) (permissions.PromptRequest, bool) {
	if broker == nil || strings.TrimSpace(actorID) == "" {
		return permissions.PromptRequest{}, false
	}
	broker.mu.Lock()
	defer broker.mu.Unlock()
	for index := len(broker.waiters) - 1; index >= 0; index-- {
		waiter := broker.waiters[index]
		if waiter != nil && waiter.request.ActorID == actorID && waiter.sessionID == sessionID && waiter.epoch == epoch {
			return clonePromptRequest(waiter.request), true
		}
	}
	return permissions.PromptRequest{}, false
}

func (r *TuiRenderer) runDecisionResponsePump(broker *decisionBroker) {
	for {
		if broker.stopPumpIfIdle() {
			return
		}
		select {
		case response := <-r.state.DecisionResp:
			broker.deliver(response)
		case <-broker.wakePump:
		case <-r.state.stopCh:
			broker.resolveAll(permissions.PromptOutcomeShutdown)
			broker.markPumpStopped()
			return
		}
	}
}

func publishNextDecisionOverlay(state *AppState, broker *decisionBroker, transition decisionBrokerTransition) {
	if !transition.wasActive {
		return
	}
	state.DecisionSelected.Set(0)
	if transition.next == nil {
		state.DecisionReq.Set(nil)
		return
	}
	if !state.AdmitEpoch(transition.next.epoch) || state.SessionID.Get() != transition.next.sessionID {
		observability.RecordGenerationDrop(observability.GenerationSurfaceTUIDecision)
		state.DecisionReq.Set(nil)
		broker.resolve(transition.next, decisionResponse(transition.next.request.DecisionID, permissions.PromptOutcomeCancelled, ""))
		return
	}
	next := clonePromptRequest(transition.next.request)
	state.DecisionReq.Set(&next)
}

// fmtK formats a token count as e.g. "1.2K" or "450".
func fmtK(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

// fmtCost formats a USD cost amount concisely for the banner.
// Examples: 3.00 → "3", 15.00 → "15", 0.15 → "0.15", 0.00 → "0".
// Uses math.Round to avoid floating-point precision issues where e.g.
// 2.9999999999999996 would fail an exact float64(int(v)) comparison.
func fmtCost(v float64) string {
	if v == 0 {
		return "0"
	}
	// If it's effectively a whole number (within float64 rounding), no decimals.
	rounded := math.Round(v)
	if math.Abs(v-rounded) < 1e-9 {
		return fmt.Sprintf("%d", int(rounded))
	}
	// Otherwise, use minimal precision (strip trailing zeros)
	s := fmt.Sprintf("%.2f", v)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}

func fmtCostPair(in, out float64, currency string) string {
	symbol := provider.CostCurrencySymbol(currency)
	return fmt.Sprintf("%s%s/%s%s", symbol, fmtCost(in), symbol, fmtCost(out))
}

// FmtContextWindow formats a context window token count as e.g. "200K", "1M", "2M".
// Exported for use by repl_tui.go and other packages.
func FmtContextWindow(tokens int) string {
	return fmtContextWindow(tokens)
}

// fmtContextWindow formats a context window token count as e.g. "200K", "1M", "2M".
func fmtContextWindow(tokens int) string {
	return provider.FormatContextWindow(tokens)
}
