package loop

import (
	"context"
	"errors"
	"fmt"

	"github.com/agent-dance/luban/cost"
	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

type preparedMessagesForQuery struct {
	Messages []types.Message
}

func (q *QueryLoop) prepareMessagesForQuery(ctx context.Context, state *QueryState, turnCount int, taskBudgetTotal int, cacheActive bool, onEvent func(stream.Event), snapshots ...QueryConfigSnapshot) (preparedMessagesForQuery, error) {
	snapshot := newQueryConfigSnapshot(q.config, q.thinkingConfig)
	if len(snapshots) > 0 {
		snapshot = snapshots[0]
	}
	messagesForQuery := compact.GetMessagesAfterCompactBoundaryForScope(state.Messages, q.internalControlScope)
	// Durable audit-only blocks must be removed before any estimator, budgeter,
	// microcompactor, semantic compactor, or hook can observe the model view.
	messagesForQuery = compact.StripProviderPrivateBlocks(messagesForQuery)
	// Preserve the actual pre-replacement model-visible source before
	// microcompact, result budgeting, or staged-collapse projections can remove
	// exact skill envelopes. Only this source can prove which bodies may be
	// considered for bounded post-compact reattachment.
	postCompactSkillRecoverySource := compact.StripProviderPrivateBlocks(messagesForQuery)
	lossyProviderProjection := false

	messagesForQuery, records, replacementErrs := compact.ApplyToolResultBudget(messagesForQuery, q.contentReplacementState, q.resultStore, nil)
	for _, err := range replacementErrs {
		onEvent(stream.Event{Type: stream.EventError, Text: err.Error(), TurnCount: turnCount})
	}
	if len(records) > 0 {
		state.Messages = q.installContentReplacementRecords(state.Messages, records)
	}
	messagesForQuery = compact.StripProviderPrivateBlocks(messagesForQuery)
	if admission, ok := q.progressiveProjectionAdmission(state, snapshot, messagesForQuery); ok {
		seenBefore, replacementsBefore := cloneContentReplacementMaps(q.contentReplacementState)
		progressive := compact.ApplyProgressiveToolResultProjection(messagesForQuery, q.contentReplacementState, admission)
		if progressive.Changed {
			projectedEstimate := q.ctxWindow.EstimateProviderRequest(q.providerParamsBase(state, snapshot, progressive.Messages))
			projectedProviderTokens := q.ctxWindow.ProviderAdjustedInputTokens(projectedEstimate)
			if !projectedEstimate.Complete || projectedProviderTokens >= admission.RawRequestTokens {
				q.contentReplacementState.SeenIDs = seenBefore
				q.contentReplacementState.Replacements = replacementsBefore
				progressive.Messages = messagesForQuery
				progressive.Records = nil
				progressive.Changed = false
				progressive.Decision = compact.ProgressiveDecisionKeepAnomaly
				q.progressiveAnomalies++
				if q.progressiveAnomalies >= compact.NormalizeProgressiveConfig(snapshot.ProgressiveContext).MaxConsecutiveAnomalies {
					q.progressiveCircuitOpen = true
				}
			}
		}
		messagesForQuery = progressive.Messages
		if len(progressive.Records) > 0 {
			state.Messages = q.installContentReplacementRecords(state.Messages, progressive.Records)
		}
		if progressive.Changed {
			q.progressiveAnomalies = 0
			q.invalidateProviderProjectionContinuation()
		}
		if progressive.ProjectedTools > 0 {
			q.progressiveProjectionSeq++
			onEvent(stream.Event{
				Type: stream.EventProgress, TurnCount: turnCount,
				Progress: &stream.ProgressEvent{
					Stage: "progressive_context_projection",
					Metadata: map[string]any{
						"projection_sequence":            q.progressiveProjectionSeq,
						"trigger":                        progressive.Trigger,
						"decision":                       progressive.Decision,
						"shadow":                         progressive.Shadow,
						"applied":                        progressive.Changed,
						"projection_count":               progressive.ProjectedTools,
						"rewrite_count":                  progressive.RewrittenTools,
						"index_count":                    progressive.IndexedTools,
						"original_bytes":                 progressive.OriginalBytes,
						"projected_bytes":                progressive.ProjectedBytes,
						"bytes_saved":                    progressive.BytesSaved,
						"original_tokens":                progressive.OriginalTokens,
						"projected_tokens":               progressive.ProjectedTokens,
						"tokens_saved":                   progressive.TokensSaved,
						"request_tokens_before":          progressive.RawRequestTokens,
						"request_tokens_after":           progressive.ProjectedRequestTokens,
						"cache_break_cost_usd":           progressive.CacheBreakCostUSD,
						"gross_cache_break_cost_usd":     progressive.GrossCacheBreakCostUSD,
						"avoided_compact_input_cost_usd": progressive.AvoidedCompactInputCostUSD,
						"estimated_net_savings_usd":      progressive.EstimatedNetSavingsUSD,
						"avoids_immediate_compaction":    progressive.AvoidsImmediateCompaction,
					},
				},
			})
		}
		pending := compact.ProgressiveProjectionPending{}
		if !q.progressiveCircuitOpen {
			pending = compact.PendingProgressiveToolResultProjection(messagesForQuery, q.contentReplacementState, admission)
		}
		q.emitProgressivePending(turnCount, pending, onEvent)
	} else {
		q.emitProgressivePending(turnCount, compact.ProgressiveProjectionPending{}, onEvent)
	}
	if q.toolBudget != nil {
		messagesForQuery = q.toolBudget.Apply(messagesForQuery)
	}
	if postCompactSkillBodyProjectionLost(postCompactSkillRecoverySource, messagesForQuery) {
		lossyProviderProjection = true
	}

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

	installLossyProjection := func() error {
		installed, installErr := q.installPostCompactVisibleHistory(postCompactSkillRecoverySource, messagesForQuery)
		if installErr != nil {
			return installErr
		}
		messagesForQuery = installed
		if q.config.SkillManager != nil {
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
	autoCompactKeepRecent := 0
	autoCompactMaxGrowthTokens := 0
	autoCompactMinThresholdPercent := 0
	providerName := ""
	providerModel := snapshot.Model
	if q.provider != nil {
		providerName = q.provider.Name()
		if providerModel == "" {
			providerModel = q.provider.ModelID()
		}
	}
	progressiveConfig := compact.NormalizeProgressiveConfig(snapshot.ProgressiveContext)
	if compact.ProgressiveProviderCompactPolicyEnabled(progressiveConfig, providerName, providerModel, snapshot.SessionID) {
		autoCompactKeepRecent = progressiveConfig.AutoCompactKeepRecent
		autoCompactMaxGrowthTokens = progressiveConfig.AutoCompactMaxGrowthTokens
		autoCompactMinThresholdPercent = progressiveConfig.AutoCompactMinThresholdPercent
	}
	result, attempted, err := q.runAutoCompaction(ctx, messagesForQuery, requestEstimate, autoCompactKeepRecent, autoCompactMaxGrowthTokens, autoCompactMinThresholdPercent, turnCount, txn.eventSink(onEvent))
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
	if q.ctxWindow != nil && requestEstimate != nil && !state.AutoCompactTracking.Compacted {
		q.ctxWindow.UpdateLocalEstimate(*requestEstimate)
	}

	return preparedMessagesForQuery{Messages: compact.StripProviderPrivateBlocks(messagesForQuery)}, nil
}

func (q *QueryLoop) emitProgressivePending(turnCount int, pending compact.ProgressiveProjectionPending, onEvent func(stream.Event)) {
	if q == nil || onEvent == nil {
		return
	}
	pending.Tools = max(pending.Tools, 0)
	pending.TokensSaved = max(pending.TokensSaved, 0)
	positive := pending.Tools > 0 && pending.TokensSaved > 0
	changed := pending.Tools != q.progressivePendingTools || pending.TokensSaved != q.progressivePendingTokens
	if !q.progressivePendingKnown && !positive || q.progressivePendingKnown && !changed {
		return
	}
	q.progressiveProjectionSeq++
	q.progressivePendingKnown = true
	q.progressivePendingTools = pending.Tools
	q.progressivePendingTokens = pending.TokensSaved
	onEvent(stream.Event{
		Type: stream.EventProgress, TurnCount: turnCount,
		Progress: &stream.ProgressEvent{
			Stage: "progressive_context_projection",
			Metadata: map[string]any{
				"projection_sequence": q.progressiveProjectionSeq,
				"pending_only":        true,
				"pending_tools":       pending.Tools,
				"pending_tokens":      pending.TokensSaved,
			},
		},
	})
}

func cloneContentReplacementMaps(state *compact.ContentReplacementState) (map[string]struct{}, map[string]string) {
	seen := make(map[string]struct{}, len(state.SeenIDs))
	for id := range state.SeenIDs {
		seen[id] = struct{}{}
	}
	replacements := make(map[string]string, len(state.Replacements))
	for id, replacement := range state.Replacements {
		replacements[id] = replacement
	}
	return seen, replacements
}

func (q *QueryLoop) progressiveProjectionAdmission(state *QueryState, snapshot QueryConfigSnapshot, messages []types.Message) (compact.ProgressiveProjectionAdmission, bool) {
	if q == nil || q.ctxWindow == nil || q.contentReplacementState == nil || q.progressiveCircuitOpen || progressiveContextCompactionKilled() {
		return compact.ProgressiveProjectionAdmission{}, false
	}
	config := compact.NormalizeProgressiveConfig(snapshot.ProgressiveContext)
	providerName := ""
	model := snapshot.Model
	if q.provider != nil {
		providerName = q.provider.Name()
		if model == "" {
			model = q.provider.ModelID()
		}
	}
	if !compact.ProgressiveEnabledForSession(config, providerName, model, snapshot.SessionID) {
		return compact.ProgressiveProjectionAdmission{}, false
	}
	rawEstimate := q.ctxWindow.EstimateProviderRequest(q.providerParamsBase(state, snapshot, messages))
	maxGrowthTokens := 0
	minThresholdPercent := 0
	providerScopedPolicy := compact.ProgressiveProviderCompactPolicyEnabled(config, providerName, model, snapshot.SessionID)
	if providerScopedPolicy {
		maxGrowthTokens = config.AutoCompactMaxGrowthTokens
		minThresholdPercent = config.AutoCompactMinThresholdPercent
	}
	rawRequestTokens := q.ctxWindow.AutoCompactDecisionTokens(rawEstimate, maxGrowthTokens)
	pricing, pricingKnown := cost.LookupPricing(model)
	usedTools, usedProjectedTokens := compact.ProgressiveProjectionBudgetUsage(q.contentReplacementState, q.ctxWindow.Counter)
	allowedTools := make(map[string]struct{}, len(config.ToolAllowlist))
	for _, toolName := range config.ToolAllowlist {
		if compact.ProgressiveToolEnabled(config, toolName) {
			allowedTools[toolName] = struct{}{}
		}
	}
	remainingTools := config.MaxProjectedTools - usedTools
	remainingProjectedTokens := config.MaxProjectedTokens - usedProjectedTokens
	if remainingTools <= 0 || remainingProjectedTokens <= 0 {
		return compact.ProgressiveProjectionAdmission{}, false
	}
	autoCompactThreshold := q.ctxWindow.AutoCompactThresholdWithMinPercent(minThresholdPercent)
	if q.autoCompactGuarded() {
		autoCompactThreshold = 0
	}
	return compact.ProgressiveProjectionAdmission{
		Enabled:                 true,
		Shadow:                  config.Shadow,
		Pressure:                q.ctxWindow.ShouldProgressiveProjectionWithPolicy(rawEstimate, autoCompactThreshold, maxGrowthTokens),
		Counter:                 q.ctxWindow.Counter,
		RawRequestTokens:        rawRequestTokens,
		RawRequestEstimateKnown: rawEstimate.Complete,
		AutoCompactThreshold:    autoCompactThreshold,
		PreviousCacheReadTokens: q.ctxWindow.PreviousCacheReadTokens(),
		PreviousUsageKnown:      q.ctxWindow.ProviderUsageKnown(),
		Pricing: compact.ProgressiveTokenPricing{
			InputPerMtok: pricing.InputPerMtok, CacheReadPerMtok: pricing.CacheReadPerMtok, Known: pricingKnown,
		},
		MinTokenSavings:            config.MinTokenSavings,
		ReuseHorizon:               config.ReuseHorizon,
		CacheRecoveryRequests:      config.CacheRecoveryRequests,
		ImminentCompactResetsCache: providerScopedPolicy,
		RequireConsumedMutation:    providerScopedPolicy && config.RequireConsumedMutation,
		MinNetSavingsUSD:           config.MinNetSavingsUSD,
		RemainingTools:             remainingTools,
		RemainingProjectedTokens:   remainingProjectedTokens,
		AllowedTools:               allowedTools,
	}, true
}

func (q *QueryLoop) runAutoCompaction(ctx context.Context, messages []types.Message, requestEstimate *compact.ModelContextTokenEstimate, keepRecent, maxGrowthTokens, minThresholdPercent, turnCount int, onEvent func(stream.Event)) (*compact.CompactionResult, bool, error) {
	if q.ctxWindow == nil || q.compactor == nil {
		return nil, false, nil
	}
	if q.autoCompactGuarded() {
		return nil, false, nil
	}
	shouldSnip := q.ctxWindow.ShouldSnip(messages)
	autoCompactThreshold := q.ctxWindow.AutoCompactThresholdWithMinPercent(minThresholdPercent)
	if requestEstimate != nil {
		shouldSnip = q.ctxWindow.ShouldSnipEstimateWithPolicy(*requestEstimate, autoCompactThreshold, maxGrowthTokens)
	}
	if !shouldSnip && (requestEstimate != nil || !q.ctxWindow.ShouldCompact()) {
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
		"trigger":                "auto",
		"turn":                   turnCount,
		"message_count":          len(messages),
		"estimated_input_tokens": estimatedInput,
		"estimate_complete":      estimateComplete,
		"unknown_overheads":      unknownOverheads,
	})
	var attempted bool
	compactorInput := executioncontract.CloneMessages(messages)
	result, _, err := q.runCompactionAgainst(ctx, "auto", turnCount, onEvent, compactorInput, func() (*compact.CompactionResult, error) {
		result, didAttempt, compactErr := compact.AutoCompactIfNeeded(ctx, executioncontract.CloneMessages(compactorInput), compact.AutoCompactOptions{
			Window:          q.ctxWindow,
			Compactor:       q.compactor,
			KeepRecent:      keepRecent,
			Trigger:         "auto",
			RequestEstimate: requestEstimate,
			Threshold:       autoCompactThreshold,
			MaxGrowthTokens: maxGrowthTokens,
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
	case QuerySourceCompact:
		return true
	}
	if q.config.AgentID != "" || q.config.QueryScope.IsSubagent {
		return true
	}
	return false
}
