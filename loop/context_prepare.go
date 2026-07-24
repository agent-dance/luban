package loop

import (
	"context"
	"errors"
	"fmt"

	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

type preparedMessagesForQuery struct {
	Messages []types.Message
}

func (q *QueryLoop) prepareMessagesForQuery(ctx context.Context, state *QueryState, turnCount int, taskBudgetTotal int, cacheActive bool, onEvent func(Event), snapshots ...QueryConfigSnapshot) (preparedMessagesForQuery, error) {
	snapshot := newQueryConfigSnapshot(q.config, q.thinkingConfig)
	if len(snapshots) > 0 {
		snapshot = snapshots[0]
	}
	messagesForQuery := compact.GetMessagesAfterCompactBoundaryForScope(state.Messages, q.internalControlScope, false)
	// Preserve the actual pre-replacement model-visible source before
	// microcompact, result budgeting, or staged-collapse projections can remove
	// exact skill envelopes. Only this source can prove which bodies may be
	// considered for bounded post-compact reattachment.
	postCompactSkillRecoverySource := compact.StripContentReplacementBlocks(messagesForQuery)
	lossyProviderProjection := false

	messagesForQuery, records, replacementErrs := compact.ApplyToolResultBudget(messagesForQuery, q.contentReplacementState, q.resultStore, nil)
	for _, err := range replacementErrs {
		onEvent(Event{Type: EventError, Text: err.Error(), TurnCount: turnCount})
	}
	if len(records) > 0 {
		state.Messages = q.installContentReplacementRecords(state.Messages, records)
	}
	messagesForQuery = compact.StripContentReplacementBlocks(messagesForQuery)
	if q.toolBudget != nil {
		messagesForQuery = q.toolBudget.Apply(messagesForQuery)
	}
	if postCompactSkillBodyProjectionLost(postCompactSkillRecoverySource, messagesForQuery) {
		lossyProviderProjection = true
	}

	preAutoCompactTokensFreed := 0
	stagedCollapseProjected := false
	if q.microcompactCfg.KeepRecent > 0 {
		microcompactResult := compact.MicrocompactWithResult(messagesForQuery, q.microcompactCfg)
		messagesForQuery = microcompactResult.Messages
		if microcompactResult.Changed && q.ctxWindow != nil {
			q.ctxWindow.RecordMicrocompactSuccess()
		}
		if microcompactResult.TimeBasedTriggered && q.cacheBreakDetector != nil {
			q.cacheBreakDetector.NotifyCompaction()
		}
		if microcompactResult.TimeBasedTriggered && q.cachedMicrocompactState != nil {
			q.cachedMicrocompactState.Reset()
		}
		if microcompactResult.TimeBasedTriggered && microcompactResult.Changed {
			lossyProviderProjection = true
		}
		if !microcompactResult.TimeBasedTriggered && cacheActive && q.provider.Name() == "anthropic" {
			cachedResult := compact.CachedMicrocompact(messagesForQuery, q.microcompactCfg, q.cachedMicrocompactState)
			messagesForQuery = cachedResult.Messages
			if cachedResult.Changed && q.cacheBreakDetector != nil {
				q.cacheBreakDetector.NotifyCompaction()
			}
			if cachedResult.Changed && q.ctxWindow != nil {
				q.ctxWindow.RecordMicrocompactSuccess()
			}
			if cachedResult.Changed {
				q.emitCacheSharingTelemetry(onEvent, turnCount, true, "", len(cachedResult.DeletedToolIDs))
			}
		} else if cacheActive && q.microcompactCfg.ShouldUseCachedMicrocompact() && q.provider.Name() != "anthropic" {
			q.emitCacheSharingTelemetry(onEvent, turnCount, false, "provider_unsupported", 0)
		}
	}

	// Go supports only explicit staged context-collapse marker projection here,
	// not the full TS context-collapse store. The pre-provider-call order is:
	// microcompact -> staged collapse projection -> semantic autoCompact.
	beforeCollapseTokens := 0
	if q.ctxWindow != nil {
		beforeCollapseTokens = q.ctxWindow.EstimateMessages(messagesForQuery)
	}
	projection := compact.ProjectStagedContextCollapseForScope(messagesForQuery, q.internalControlScope)
	messagesForQuery = projection.Messages
	if projection.Projected > 0 {
		lossyProviderProjection = true
		stagedCollapseProjected = true
		if q.ctxWindow != nil {
			after := q.ctxWindow.EstimateMessages(messagesForQuery)
			if beforeCollapseTokens > after {
				preAutoCompactTokensFreed += beforeCollapseTokens - after
			}
		}
	}
	installLossyProjection := func() error {
		installed, installErr := q.installPostCompactVisibleHistory(postCompactSkillRecoverySource, messagesForQuery)
		if installErr != nil {
			return installErr
		}
		messagesForQuery = installed
		if stagedCollapseProjected {
			if cleanupErr := q.RunPostCompactCleanup(ctx, installed); cleanupErr != nil {
				return cleanupErr
			}
		}
		// A staged collapse is an ephemeral marker and must always be consumed
		// into durable state before the next save. Other provider projections
		// preserve the historical provider-only behavior without a live catalog.
		if stagedCollapseProjected || q.config.SkillManager != nil {
			state.Messages = installed
		}
		return nil
	}
	var requestEstimate *compact.ModelContextTokenEstimate
	if q.ctxWindow != nil {
		estimate := q.ctxWindow.EstimateMessagesDetailed(messagesForQuery, compact.ModelContextOverhead{})
		if q.provider != nil {
			estimate = q.ctxWindow.EstimateProviderRequest(q.providerParamsBase(state, snapshot, messagesForQuery))
		}
		requestEstimate = &estimate
	}
	txn, err := q.beginCompactionInstallTransaction(state)
	if err != nil {
		return preparedMessagesForQuery{}, i18n.WrapInternalError(i18n.KeyLoopQuerySnapshotSkillCatalogFailed, err)
	}
	result, attempted, err := q.runAutoCompaction(ctx, messagesForQuery, requestEstimate, preAutoCompactTokensFreed, turnCount, txn.eventSink(onEvent))
	if err != nil {
		if ctx.Err() != nil {
			txn.publish(onEvent)
			return preparedMessagesForQuery{}, ctx.Err()
		}
		if semantic, ok := i18n.DescribeSemanticError(err); ok && semantic.Key == i18n.KeyLoopCompactionResultRejected {
			return preparedMessagesForQuery{}, txn.fail(q, state, onEvent, "auto", turnCount, err)
		}
		state.AutoCompactTracking.Compacted = false
		state.AutoCompactTracking.TurnCounter = turnCount
		state.AutoCompactTracking.TurnID = fmt.Sprintf("turn_%d", turnCount)
		if q.ctxWindow != nil {
			state.AutoCompactTracking.ConsecutiveFailures = q.ctxWindow.ConsecutiveFailures()
		}
		if lossyProviderProjection {
			if installErr := installLossyProjection(); installErr != nil {
				return preparedMessagesForQuery{}, txn.fail(q, state, onEvent, "auto", turnCount, installErr)
			}
		}
		txn.publish(onEvent)
		onEvent(NewSystemWarningEvent(i18n.KeyRuntimeAutoCompactFailed, nil, err, nil, turnCount))
	} else if attempted && result != nil {
		replacement, changed, valid := q.classifyCompactionReplacement(messagesForQuery, result)
		if !valid {
			return preparedMessagesForQuery{}, txn.fail(q, state, onEvent, "auto", turnCount, errors.New("changed auto-compaction result has no authenticated boundary"))
		}
		if !changed {
			txn.rollback(q, state)
			if lossyProviderProjection {
				if installErr := installLossyProjection(); installErr != nil {
					return preparedMessagesForQuery{}, txn.fail(q, state, onEvent, "auto", turnCount, installErr)
				}
			}
			txn.publishNoop(onEvent)
			attempted = false
		} else {
			installed, installErr := q.installPostCompactVisibleHistory(postCompactSkillRecoverySource, replacement)
			if installErr != nil {
				return preparedMessagesForQuery{}, txn.fail(q, state, onEvent, "auto", turnCount, installErr)
			}
			if cleanupErr := q.RunPostCompactCleanup(ctx, installed); cleanupErr != nil {
				return preparedMessagesForQuery{}, txn.fail(q, state, onEvent, "auto", turnCount, cleanupErr)
			}
			state.Messages = installed
			messagesForQuery = installed
			state.recordTaskBudgetCompaction(taskBudgetTotal, result.PreCompactTokenCount)
			state.AutoCompactTracking.Compacted = true
			state.AutoCompactTracking.TurnCounter = turnCount
			state.AutoCompactTracking.TurnID = fmt.Sprintf("turn_%d", turnCount)
			if q.ctxWindow != nil {
				state.AutoCompactTracking.ConsecutiveFailures = q.ctxWindow.ConsecutiveFailures()
			}
			q.updatePostCompactContext(result)
			txn.publish(onEvent)
		}
	} else if q.ctxWindow != nil {
		if attempted {
			txn.rollback(q, state)
		}
		state.AutoCompactTracking.Compacted = false
		state.AutoCompactTracking.TurnCounter = turnCount
		state.AutoCompactTracking.TurnID = fmt.Sprintf("turn_%d", turnCount)
		state.AutoCompactTracking.ConsecutiveFailures = q.ctxWindow.ConsecutiveFailures()
		if lossyProviderProjection {
			if installErr := installLossyProjection(); installErr != nil {
				return preparedMessagesForQuery{}, txn.fail(q, state, onEvent, "auto", turnCount, installErr)
			}
		}
		if attempted {
			txn.publishNoop(onEvent)
		} else {
			txn.publish(onEvent)
		}
	} else if lossyProviderProjection {
		if installErr := installLossyProjection(); installErr != nil {
			return preparedMessagesForQuery{}, txn.fail(q, state, onEvent, "auto", turnCount, installErr)
		}
		txn.publish(onEvent)
	} else {
		txn.publish(onEvent)
	}

	return preparedMessagesForQuery{Messages: compact.StripContentReplacementBlocks(messagesForQuery)}, nil
}

func (q *QueryLoop) runAutoCompaction(ctx context.Context, messages []types.Message, requestEstimate *compact.ModelContextTokenEstimate, preAutoCompactTokensFreed int, turnCount int, onEvent func(Event)) (*compact.CompactionResult, bool, error) {
	if q.ctxWindow == nil || q.compactor == nil {
		return nil, false, nil
	}
	if q.autoCompactGuarded() {
		return nil, false, nil
	}
	shouldSnip := q.ctxWindow.ShouldSnip(messages)
	if requestEstimate != nil {
		shouldSnip = q.ctxWindow.ShouldSnipEstimate(*requestEstimate)
	}
	if !shouldSnip && !q.ctxWindow.ShouldCompact() {
		return nil, false, nil
	}
	estimatedInput := q.ctxWindow.EstimateMessages(messages)
	estimateComplete := false
	var unknownOverheads []compact.TokenOverheadKind
	if requestEstimate != nil {
		estimatedInput = requestEstimate.KnownTotalTokens
		estimateComplete = requestEstimate.Complete
		unknownOverheads = append([]compact.TokenOverheadKind(nil), requestEstimate.UnknownOverheads...)
	}
	ctx = provider.WithDebugCall(ctx, provider.DebugCallCompaction, map[string]any{
		"trigger":                  "auto",
		"turn":                     turnCount,
		"message_count":            len(messages),
		"estimated_input_tokens":   estimatedInput,
		"estimate_complete":        estimateComplete,
		"unknown_overheads":        unknownOverheads,
		"tokens_freed_before_auto": preAutoCompactTokensFreed,
	})
	var attempted bool
	compactorInput := cloneMessages(messages)
	result, _, err := q.runCompactionAgainst(ctx, "auto", turnCount, onEvent, compactorInput, func() (*compact.CompactionResult, error) {
		result, didAttempt, compactErr := compact.AutoCompactIfNeeded(ctx, cloneMessages(compactorInput), compact.AutoCompactOptions{
			Window:                    q.ctxWindow,
			Compactor:                 q.compactor,
			Trigger:                   "auto",
			PreAutoCompactTokensFreed: preAutoCompactTokensFreed,
			RequestEstimate:           requestEstimate,
			OnTelemetry: func(event compact.CompactionTelemetryEvent) {
				q.emitCompactTelemetry(onEvent, turnCount, event)
			},
		})
		attempted = didAttempt
		return result, compactErr
	})
	return result, attempted, err
}

func (q *QueryLoop) autoCompactGuarded() bool {
	if !compact.ShouldUseAutoCompact() {
		return true
	}
	switch q.config.QuerySource {
	case QuerySourceCompact, QuerySourceSessionMemory:
		return true
	}
	if q.config.AgentID != "" || q.config.QueryScope.IsSubagent {
		return true
	}
	return false
}
