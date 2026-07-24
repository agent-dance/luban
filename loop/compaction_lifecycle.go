package loop

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/observability"
	"github.com/agent-dance/luban/types"
)

const compactKeepaliveInterval = 5 * time.Second

func (q *QueryLoop) runCompaction(ctx context.Context, trigger string, turnCount int, onEvent func(Event), run func() (*compact.CompactionResult, error)) (result *compact.CompactionResult, err error) {
	result, _, err = q.runCompactionAgainst(ctx, trigger, turnCount, onEvent, q.messages, run)
	return result, err
}

func (q *QueryLoop) updatePostCompactContext(result *compact.CompactionResult) {
	if q == nil || q.ctxWindow == nil || result == nil {
		return
	}
	retained := result.TruePostCompactTokenCount
	if retained == 0 {
		retained = result.PostCompactTokenCount
	}
	q.ctxWindow.UpdatePostCompactUsage(retained)
}

// runCompactionAgainst validates the compactor result against the exact input
// supplied to that invocation. Callers may prepare a lossy provider projection
// before semantic compaction, so q.messages is not necessarily the right
// comparison baseline.
func (q *QueryLoop) runCompactionAgainst(
	ctx context.Context,
	trigger string,
	turnCount int,
	onEvent func(Event),
	exactInput []types.Message,
	run func() (*compact.CompactionResult, error),
) (result *compact.CompactionResult, semanticNoop bool, err error) {
	// Freeze the semantic baseline before invoking third-party compactor code.
	// A shallow slice copy is insufficient because Content blocks contain maps,
	// nested messages, and mutable byte slices.
	input := cloneMessages(exactInput)
	q.compactStatus = "compacting"
	q.emitCompactProgress(onEvent, turnCount, "compact_start", "compacting", trigger, "")
	stopKeepalive := q.startCompactKeepalive(ctx, trigger, turnCount, onEvent)
	restoreProgress := q.installCompactorProgress(trigger, turnCount, onEvent)
	var telemetryUsage *types.Usage
	restoreTelemetry := q.installCompactorTelemetry(trigger, turnCount, onEvent, func(event compact.CompactionTelemetryEvent) {
		if telemetryUsage != nil || event.CompactionUsage == nil || !isTerminalCompactTelemetry(event.Kind) {
			return
		}
		copied := *event.CompactionUsage
		telemetryUsage = &copied
	})
	defer func() {
		restoreTelemetry()
		restoreProgress()
		stopKeepalive()
		q.compactStatus = ""
		stage, status := "compact_end", "idle"
		switch {
		case errors.Is(err, context.Canceled):
			stage, status = "compact_cancelled", "cancelled"
		case err != nil:
			stage, status = "compact_failed", "failed"
		}
		q.emitCompactProgress(onEvent, turnCount, stage, status, trigger, "", err)
	}()

	result, err = run()
	var resultUsage *types.Usage
	if result != nil && result.CompactionUsage != nil {
		copied := *result.CompactionUsage
		resultUsage = &copied
	}
	authorizedBoundary := false
	if err == nil && result != nil && stripPostCompactSkillProviderAttachments(result) {
		counter := compact.NewContextWindow(0)
		result.PostCompactTokenCount = counter.EstimateMessages(compact.BuildPostCompactMessages(result))
		result.TruePostCompactTokenCount = result.PostCompactTokenCount
	}
	if err == nil && result != nil {
		replacement := compact.BuildPostCompactMessages(result)
		if reflect.DeepEqual(replacement, input) {
			// A boundaryless deep-equal result is a semantic no-op. Normalize it
			// to nil so no caller can accidentally install a pre-compaction
			// projection as a durable history replacement.
			result = nil
			semanticNoop = true
		} else if !q.authorizeChangedCompactionResult(result) {
			err = i18n.NewError(i18n.KeyLoopCompactionResultRejected)
		} else {
			authorizedBoundary = true
		}
	}
	if err == nil && result != nil && trigger == "manual" {
		prepared, skillErr := q.preparePostCompactSkillHistory(q.messages, compact.BuildPostCompactMessages(result))
		if skillErr != nil {
			err = skillErr
		} else {
			result.PreparedMessages = prepared
			counter := compact.NewContextWindow(0)
			result.PostCompactTokenCount = counter.EstimateMessages(compact.BuildPostCompactMessages(result))
			result.TruePostCompactTokenCount = result.PostCompactTokenCount
		}
	}
	if err == nil && result != nil && !q.compactionResultHasCurrentBoundary(result) {
		err = i18n.NewError(i18n.KeyLoopCompactionResultRejected)
		authorizedBoundary = false
	}
	usage := telemetryUsage
	if resultUsage != nil {
		usage = resultUsage
	}
	status := "success"
	if err != nil {
		status = "failure"
	}
	q.emitCompactionUsage(onEvent, turnCount, trigger, status, usage)
	recordCompactionMetric(trigger, input, result, err)
	if err != nil {
		result = nil
	} else if result != nil && authorizedBoundary && onEvent != nil {
		// Emit only after the private installation boundary has authenticated the
		// compactor descriptors and all result normalization has completed. This
		// keeps metadata, summary, and hook display evidence on one exact result
		// while preserving start/boundary/end lifecycle ordering.
		onEvent(newCompactBoundaryEvent(result, trigger, turnCount))
	}
	return result, semanticNoop, err
}

// authorizeChangedCompactionResult accepts ordinary unsealed descriptors from
// compact producers, but never rebinds an already-authenticated foreign bearer
// into the current QueryLoop scope.
func (q *QueryLoop) authorizeChangedCompactionResult(result *compact.CompactionResult) bool {
	if q == nil || result == nil || result.BoundaryMarker == nil {
		return false
	}
	q.ensureInternalControlScope()
	if compactionResultContainsForeignControl(result, q.internalControlScope) {
		return false
	}
	compact.AuthorizeCompactionResultForScope(messagecontrol.Runtime(), q.internalControlScope, result)
	return q.compactionResultHasCurrentBoundary(result)
}

func compactionResultContainsForeignControl(result *compact.CompactionResult, scope messagecontrol.Scope) bool {
	if result == nil {
		return false
	}
	if result.BoundaryMarker != nil && messageContainsForeignControl(*result.BoundaryMarker, scope) {
		return true
	}
	for _, messages := range [][]types.Message{
		result.SummaryMessages,
		result.MessagesToKeep,
		result.Attachments,
		result.HookResults,
		result.PreparedMessages,
	} {
		for _, message := range messages {
			if messageContainsForeignControl(message, scope) {
				return true
			}
		}
	}
	return false
}

func messageContainsForeignControl(message types.Message, scope messagecontrol.Scope) bool {
	if message.HasInternalControlProvenance() && !message.HasInternalControlProvenanceForScope(scope, false) {
		return true
	}
	for _, block := range message.Content {
		if contentBlockContainsForeignControl(block, scope) {
			return true
		}
	}
	return false
}

func contentBlockContainsForeignControl(block types.ContentBlock, scope messagecontrol.Scope) bool {
	switch typed := block.(type) {
	case types.ContentReplacementBlock:
		return typed.HasInternalReplacementProvenance() && !typed.HasInternalReplacementProvenanceForScope(scope, false)
	case *types.ContentReplacementBlock:
		// Durable control walkers use value blocks. An authenticated pointer
		// would evade commit-time rebinding, so reject it even when its embedded
		// scope currently matches.
		return typed == nil || typed.HasInternalReplacementProvenance()
	case types.ToolResultBlock:
		for _, nested := range typed.ContentBlocks {
			if contentBlockContainsForeignControl(nested, scope) {
				return true
			}
		}
		for _, nested := range typed.NewMessages {
			if messageContainsForeignControl(nested, scope) {
				return true
			}
		}
	case *types.ToolResultBlock:
		if typed == nil {
			return true
		}
		for _, nested := range typed.ContentBlocks {
			if contentBlockContainsForeignControl(nested, scope) {
				return true
			}
		}
		for _, nested := range typed.NewMessages {
			if messageContainsForeignControl(nested, scope) {
				return true
			}
		}
	}
	return false
}

func (q *QueryLoop) compactionResultHasCurrentBoundary(result *compact.CompactionResult) bool {
	if q == nil || result == nil || result.BoundaryMarker == nil {
		return false
	}
	if _, ok := compact.ParseCompactBoundaryMessageForScope(*result.BoundaryMarker, q.internalControlScope, false); !ok {
		return false
	}
	replacement := compact.BuildPostCompactMessages(result)
	return len(replacement) > 0 && compact.IsCompactBoundaryMessageForScope(replacement[0], q.internalControlScope, false)
}

func recordCompactionMetric(trigger string, before []types.Message, result *compact.CompactionResult, err error) {
	outcome := observability.CompactionOutcomeSuccess
	switch {
	case errors.Is(err, context.Canceled):
		outcome = observability.CompactionOutcomeCancelled
	case err != nil:
		outcome = observability.CompactionOutcomeFailure
	case result == nil || (result.BoundaryMarker == nil && len(result.SummaryMessages) == 0):
		outcome = observability.CompactionOutcomeNoop
	}
	afterTokens := 0
	afterMessages := 0
	beforeTokens := 0
	if result != nil {
		beforeTokens = result.PreCompactTokenCount
		afterTokens = result.TruePostCompactTokenCount
		if afterTokens == 0 {
			afterTokens = result.PostCompactTokenCount
		}
		afterMessages = len(compact.BuildPostCompactMessages(result))
	}
	observability.RecordCompaction(observability.CompactionObservation{
		Trigger:        observability.CompactionTrigger(trigger),
		Outcome:        outcome,
		BeforeTokens:   beforeTokens,
		AfterTokens:    afterTokens,
		BeforeMessages: len(before),
		AfterMessages:  afterMessages,
	})
}

func (q *QueryLoop) installCompactorTelemetry(trigger string, turnCount int, onEvent func(Event), observe func(compact.CompactionTelemetryEvent)) func() {
	sc, ok := q.compactor.(*compact.SummaryCompactor)
	if !ok {
		return func() {}
	}
	previous := sc.OnTelemetry
	sc.OnTelemetry = func(event compact.CompactionTelemetryEvent) {
		if previous != nil {
			previous(event)
		}
		if event.Trigger == "" {
			event.Trigger = trigger
		}
		if observe != nil {
			observe(event)
		}
		q.emitCompactTelemetry(onEvent, turnCount, event)
	}
	return func() {
		sc.OnTelemetry = previous
	}
}

func isTerminalCompactTelemetry(kind compact.CompactionTelemetryKind) bool {
	switch kind {
	case compact.CompactionTelemetrySuccess,
		compact.CompactionTelemetryFailure,
		compact.CompactionTelemetryAutoSuccess,
		compact.CompactionTelemetryAutoFailure:
		return true
	default:
		return false
	}
}

func (q *QueryLoop) emitCompactionUsage(onEvent func(Event), turnCount int, trigger, status string, usage *types.Usage) {
	if onEvent == nil || usage == nil {
		return
	}
	copied := *usage
	metadata := map[string]any{
		"kind":    "compaction",
		"trigger": trigger,
		"status":  status,
	}
	if q.provider != nil {
		if providerName := q.provider.Name(); providerName != "" {
			metadata["provider"] = providerName
		}
		if model := q.provider.ModelID(); model != "" {
			metadata["model"] = model
		}
	}
	if _, ok := metadata["model"]; !ok && q.config.Model != "" {
		metadata["model"] = q.config.Model
	}
	onEvent(Event{
		Type:      EventProviderUsage,
		Usage:     &copied,
		TurnCount: turnCount,
		Metadata:  metadata,
	})
}

func (q *QueryLoop) installCompactorProgress(trigger string, turnCount int, onEvent func(Event)) func() {
	sc, ok := q.compactor.(*compact.SummaryCompactor)
	if !ok {
		return func() {}
	}
	previous := sc.OnProgress
	sc.OnProgress = func(event compact.CompactProgressEvent) {
		if previous != nil {
			previous(event)
		}
		if event.Trigger == "" {
			event.Trigger = trigger
		}
		q.emitCompactProgress(onEvent, turnCount, event.Type, "compacting", event.Trigger, event.HookType)
	}
	return func() {
		sc.OnProgress = previous
	}
}

func (q *QueryLoop) startCompactKeepalive(ctx context.Context, trigger string, turnCount int, onEvent func(Event)) func() {
	if onEvent == nil {
		return func() {}
	}
	done := make(chan struct{})
	exited := make(chan struct{})
	ticker := time.NewTicker(compactKeepaliveInterval)
	go func() {
		defer close(exited)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				q.emitCompactProgress(onEvent, turnCount, "compact_keepalive", "compacting", trigger, "")
			}
		}
	}()
	return func() {
		close(done)
		// A manual compaction transaction buffers every lifecycle event until
		// its durable CAS has completed. Join the keepalive producer before the
		// QueryLoop returns so no progress event can arrive after that buffer has
		// been committed or rolled back.
		<-exited
	}
}

func (q *QueryLoop) emitCompactProgress(onEvent func(Event), turnCount int, stage, status, trigger, hookType string, terminalErr ...error) {
	if onEvent == nil {
		return
	}
	metadata := map[string]any{
		"status": status,
	}
	if trigger != "" {
		metadata["trigger"] = trigger
	}
	if hookType != "" {
		metadata["hook_type"] = hookType
	}
	if len(terminalErr) > 0 && terminalErr[0] != nil {
		displayError := terminalErr[0].Error()
		if compact.HasUserErrorCategory(terminalErr[0]) {
			displayError = compact.FormatCompactUserError(i18n.DetectOrLoadLanguage(), terminalErr[0])
		}
		metadata["error"] = displayError
	}
	onEvent(Event{
		Type:      EventProgress,
		TurnCount: turnCount,
		Progress: &ProgressEvent{
			Stage:    stage,
			Message:  status,
			Metadata: metadata,
		},
	})
}

func (q *QueryLoop) emitCompactTelemetry(onEvent func(Event), turnCount int, event compact.CompactionTelemetryEvent) {
	if onEvent == nil {
		return
	}
	metadata := compactTelemetryMetadata(event)
	metadata["turn_id"] = fmt.Sprintf("turn_%d", turnCount)
	if q.config.SessionID != "" {
		metadata["query_id"] = q.config.SessionID
	}
	onEvent(Event{
		Type:      EventProgress,
		TurnCount: turnCount,
		// i18n:allow display-literal identifier -- Internal telemetry event name; renderers localize known progress states.
		Progress: &ProgressEvent{
			Stage:    string(event.Kind),
			Message:  "compaction_telemetry",
			Metadata: metadata,
		},
	})
}

func compactTelemetryMetadata(event compact.CompactionTelemetryEvent) map[string]any {
	metadata := map[string]any{
		"kind": string(event.Kind),
	}
	if event.Trigger != "" {
		metadata["trigger"] = event.Trigger
	}
	if event.PreCompactTokenCount > 0 {
		metadata["pre_compact_token_count"] = event.PreCompactTokenCount
	}
	if event.PostCompactTokenCount > 0 {
		metadata["post_compact_token_count"] = event.PostCompactTokenCount
	}
	if event.TruePostCompactTokenCount > 0 {
		metadata["true_post_compact_token_count"] = event.TruePostCompactTokenCount
	}
	if event.AutoCompactThreshold > 0 {
		metadata["auto_compact_threshold"] = event.AutoCompactThreshold
	}
	metadata["post_compact_would_retrigger"] = event.PostCompactWouldRetrigger
	if event.OriginalMessageCount > 0 {
		metadata["original_message_count"] = event.OriginalMessageCount
	}
	if event.CompactedMessageCount > 0 {
		metadata["compacted_message_count"] = event.CompactedMessageCount
	}
	if event.CompactionUsage != nil {
		metadata["compact_input_tokens"] = event.CompactionUsage.InputTokens
		metadata["compact_output_tokens"] = event.CompactionUsage.OutputTokens
		metadata["cache_creation_input_tokens"] = event.CompactionUsage.CacheCreationInputTokens
		metadata["cache_read_input_tokens"] = event.CompactionUsage.CacheReadInputTokens
	}
	if event.PTLAttempt > 0 {
		metadata["ptl_retry_attempt"] = event.PTLAttempt
		metadata["ptl_dropped_messages"] = event.PTLDroppedMessages
		metadata["ptl_remaining_messages"] = event.PTLRemainingMessages
	}
	if event.ConsecutiveFailureCount > 0 {
		metadata["auto_compact_consecutive_failures"] = event.ConsecutiveFailureCount
	}
	if event.MaxConsecutiveFailureCount > 0 {
		metadata["auto_compact_max_consecutive_failures"] = event.MaxConsecutiveFailureCount
	}
	if event.ErrorType != "" {
		metadata["error_type"] = event.ErrorType
	}
	return metadata
}

func (q *QueryLoop) emitCacheSharingTelemetry(onEvent func(Event), turnCount int, success bool, fallbackReason string, cacheDropCount int) {
	if onEvent == nil {
		return
	}
	metadata := map[string]any{
		"kind":                  "cache_sharing",
		"cache_sharing_success": success,
		"cache_drop_count":      cacheDropCount,
		"turn_id":               fmt.Sprintf("turn_%d", turnCount),
	}
	if fallbackReason != "" {
		metadata["cache_sharing_fallback"] = fallbackReason
	}
	if q.config.SessionID != "" {
		metadata["query_id"] = q.config.SessionID
	}
	onEvent(Event{
		Type:      EventProgress,
		TurnCount: turnCount,
		// i18n:allow display-literal identifier -- Internal telemetry event name; renderers localize known progress states.
		Progress: &ProgressEvent{
			Stage:    "cache_sharing",
			Message:  "compaction_telemetry",
			Metadata: metadata,
		},
	})
}

// RunPostCompactCleanup centralizes the local cache resets that must happen
// after any successful manual, automatic, or reactive compaction.
func (q *QueryLoop) RunPostCompactCleanup(ctx context.Context, messages []types.Message) error {
	installedOnLoop := reflect.DeepEqual(q.messages, messages)
	prepared, err := q.ensurePostCompactSkillState(messages)
	if err != nil {
		return err
	}
	if installedOnLoop {
		q.messages = prepared
	}
	messages = prepared
	if q.config.PostCompactCleanup != nil {
		if err := q.config.PostCompactCleanup(ctx); err != nil {
			return err
		}
	}
	q.microcompactCfg.LastActivity = time.Time{}
	q.toolBudget = compact.NewToolResultBudget()
	q.contentReplacementState = compact.ReconstructContentReplacementStateForScope(messages, q.internalControlScope, true)
	if q.cacheBreakDetector != nil {
		q.cacheBreakDetector.NotifyCompaction()
	}
	if err := compact.ResetSessionMemoryCompactionTracking(ctx, nil); err != nil {
		return i18n.WrapError(i18n.KeyLoopPostCompactResetTrackingFailed, err)
	}
	return nil
}
