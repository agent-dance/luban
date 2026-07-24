package loop

import (
	"reflect"

	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// compactionInstallTransaction snapshots every loop-owned field changed by a
// visible-history installation or post-compact cleanup. QueryState is captured
// separately because Run publishes it back to q.messages even when the query
// returns an error and the engine subsequently persists that view.
type compactionInstallTransaction struct {
	loopState           manualCompactInstallPreimage
	queryState          QueryState
	taskBudgetRemaining *int
	windowState         compact.CompactionTrackerSnapshot
	hasWindowState      bool
	lifecycle           manualCompactLifecycleBuffer
}

func (q *QueryLoop) beginCompactionInstallTransaction(state *QueryState) (*compactionInstallTransaction, error) {
	loopState, err := q.captureManualCompactInstallPreimage()
	if err != nil {
		return nil, err
	}
	loopState.visible.messages = cloneMessages(loopState.visible.messages)
	txn := &compactionInstallTransaction{loopState: loopState}
	if state != nil {
		txn.queryState = *state
		txn.queryState.Messages = cloneMessages(state.Messages)
		if state.TaskBudgetRemaining != nil {
			remaining := *state.TaskBudgetRemaining
			txn.taskBudgetRemaining = &remaining
			txn.queryState.TaskBudgetRemaining = txn.taskBudgetRemaining
		}
	}
	if q.ctxWindow != nil {
		txn.windowState = q.ctxWindow.CaptureCompactionTracker()
		txn.hasWindowState = true
	}
	return txn, nil
}

func (txn *compactionInstallTransaction) rollback(q *QueryLoop, state *QueryState) {
	if txn == nil || q == nil {
		return
	}
	q.restoreManualCompactInstallPreimage(txn.loopState)
	if txn.hasWindowState && q.ctxWindow != nil {
		q.ctxWindow.RestoreCompactionTracker(txn.windowState)
	}
	if state != nil {
		*state = txn.queryState
		state.Messages = cloneMessages(txn.queryState.Messages)
		if txn.taskBudgetRemaining != nil {
			remaining := *txn.taskBudgetRemaining
			state.TaskBudgetRemaining = &remaining
		}
	}
}

func (txn *compactionInstallTransaction) eventSink(onEvent func(Event)) func(Event) {
	if txn == nil || onEvent == nil {
		return onEvent
	}
	return txn.lifecycle.record
}

func (txn *compactionInstallTransaction) publish(onEvent func(Event)) {
	if txn != nil {
		txn.lifecycle.publish(onEvent)
	}
}

func (txn *compactionInstallTransaction) publishNoop(onEvent func(Event)) {
	if txn == nil || onEvent == nil {
		return
	}
	for _, event := range txn.lifecycle.snapshot() {
		if event.Type == EventCompactBoundary {
			continue
		}
		if event.Type == EventProgress && event.Progress != nil {
			switch event.Progress.Stage {
			case "compact_success", "auto_compact_success":
				continue
			}
		}
		onEvent(event)
	}
}

// classifyCompactionReplacement distinguishes a semantic no-op from a changed
// replacement. Every changed replacement must carry a boundary authenticated
// for this exact live loop scope; otherwise it cannot enter visible history.
func (q *QueryLoop) classifyCompactionReplacement(input []types.Message, result *compact.CompactionResult) (replacement []types.Message, changed bool, valid bool) {
	if result == nil {
		return nil, false, true
	}
	replacement = compact.BuildPostCompactMessages(result)
	if result.BoundaryMarker == nil {
		if reflect.DeepEqual(replacement, input) {
			return replacement, false, true
		}
		return replacement, true, false
	}
	if _, ok := compact.ParseCompactBoundaryMessageForScope(*result.BoundaryMarker, q.internalControlScope, false); !ok {
		return replacement, true, false
	}
	return replacement, true, true
}

func (txn *compactionInstallTransaction) fail(q *QueryLoop, state *QueryState, onEvent func(Event), trigger string, turnCount int, cause error) error {
	txn.rollback(q, state)
	if trigger == "auto" && q != nil && q.ctxWindow != nil {
		q.ctxWindow.RecordCompactFailure()
		if state != nil {
			state.AutoCompactTracking.ConsecutiveFailures = q.ctxWindow.ConsecutiveFailures()
		}
	}
	semanticErr := i18n.WrapInternalError(i18n.KeyRuntimeCompactionCommitFailed, cause)
	if txn != nil {
		txn.lifecycle.publishTransactionFailure(q, onEvent, trigger, turnCount, semanticErr)
	} else if q != nil {
		q.emitCompactProgress(onEvent, turnCount, "compact_failed", "failed", trigger, "", semanticErr)
	}
	return semanticErr
}

// publishTransactionFailure retains non-terminal progress and usage evidence,
// but cannot publish a boundary or a success terminal event for a replacement
// that never committed. Exactly one semantic compact_failed terminal follows.
func (b *manualCompactLifecycleBuffer) publishTransactionFailure(q *QueryLoop, onEvent func(Event), trigger string, turnCount int, failure error) {
	if onEvent == nil {
		return
	}
	for _, event := range b.snapshot() {
		if event.Type == EventCompactBoundary {
			continue
		}
		if event.Type == EventProgress && event.Progress != nil {
			switch event.Progress.Stage {
			case "compact_end", "compact_failed", "compact_cancelled", "compact_success", "auto_compact_success":
				continue
			}
		}
		if event.Type == EventProviderUsage && event.Metadata["kind"] == "compaction" {
			metadata := make(map[string]any, len(event.Metadata))
			for key, value := range event.Metadata {
				metadata[key] = value
			}
			metadata["status"] = "failure"
			event.Metadata = metadata
		}
		onEvent(event)
	}
	q.emitCompactProgress(onEvent, turnCount, "compact_failed", "failed", trigger, "", failure)
}
