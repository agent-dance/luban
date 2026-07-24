package loop

import (
	"context"
	"errors"
	"strings"

	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

type recoverableFailureKind int

const (
	recoverableFailureNone recoverableFailureKind = iota
	recoverableFailurePromptTooLong
	recoverableFailureMediaSize
)

func classifyRecoverableFailure(err error) recoverableFailureKind {
	if err == nil {
		return recoverableFailureNone
	}
	var apiErr *types.APIError
	if errors.As(err, &apiErr) {
		msg := strings.ToLower(apiErr.Message)
		switch {
		case apiErr.Type == "context_window_full",
			apiErr.Type == "prompt_too_long",
			strings.Contains(msg, "context_window_full"),
			strings.Contains(msg, "context window") && (strings.Contains(msg, "full") || strings.Contains(msg, "exceed") || strings.Contains(msg, "overflow")),
			strings.Contains(msg, "prompt is too long"),
			strings.Contains(msg, "prompt_too_long"):
			return recoverableFailurePromptTooLong
		case isMediaSizeErrorMessage(apiErr.Type, msg):
			return recoverableFailureMediaSize
		}
	}
	msg := strings.ToLower(err.Error())
	if isMediaSizeErrorMessage("", msg) {
		return recoverableFailureMediaSize
	}
	if strings.Contains(msg, "context_window_full") ||
		strings.Contains(msg, "prompt_too_long") ||
		strings.Contains(msg, "prompt is too long") ||
		(strings.Contains(msg, "context window") && (strings.Contains(msg, "full") || strings.Contains(msg, "exceed") || strings.Contains(msg, "overflow"))) {
		return recoverableFailurePromptTooLong
	}
	return recoverableFailureNone
}

func isMediaSizeErrorMessage(errType, msg string) bool {
	errType = strings.ToLower(errType)
	return strings.Contains(errType, "media") && strings.Contains(errType, "size") ||
		strings.Contains(msg, "media size") ||
		strings.Contains(msg, "image too large") ||
		strings.Contains(msg, "document too large") ||
		strings.Contains(msg, "file too large") ||
		strings.Contains(msg, "image") && strings.Contains(msg, "too large") ||
		strings.Contains(msg, "media") && strings.Contains(msg, "too large")
}

func (q *QueryLoop) recoverFromTerminalProviderFailure(ctx context.Context, state *QueryState, failedMessages []types.Message, err error, turnCount int, onEvent func(Event)) (bool, error) {
	kind := classifyRecoverableFailure(err)
	if kind == recoverableFailureNone {
		return false, nil
	}

	if kind == recoverableFailurePromptTooLong && state.Transition != QueryTransitionCollapseDrainRetry {
		recoverySource := failedMessages
		drained := compact.RecoverFromContextCollapseOverflowForScope(recoverySource, q.internalControlScope)
		if drained.Committed == 0 {
			recoverySource = state.Messages
			drained = compact.RecoverFromContextCollapseOverflowForScope(recoverySource, q.internalControlScope)
		}
		if drained.Committed > 0 {
			txn, txnErr := q.beginCompactionInstallTransaction(state)
			if txnErr != nil {
				return false, i18n.WrapInternalError(i18n.KeyLoopQuerySnapshotSkillCatalogFailed, txnErr)
			}
			installed, installErr := q.installPostCompactVisibleHistory(recoverySource, drained.Messages)
			if installErr != nil {
				return false, txn.fail(q, state, onEvent, "context_collapse", turnCount, installErr)
			}
			if cleanupErr := q.RunPostCompactCleanup(ctx, installed); cleanupErr != nil {
				return false, txn.fail(q, state, onEvent, "context_collapse", turnCount, cleanupErr)
			}
			state.Messages = installed
			state.MaxOutputTokensOverride = 0
			state.Transition = QueryTransitionCollapseDrainRetry
			onEvent(NewSystemWarningEvent(i18n.KeyRuntimeContextOverflowDrain, []any{drained.Committed}, nil, nil, turnCount))
			return true, nil
		}
	}

	if state.HasAttemptedReactiveCompact {
		return false, nil
	}
	var attempted bool
	ctx = provider.WithDebugCall(ctx, provider.DebugCallCompaction, map[string]any{
		"trigger":       "reactive",
		"turn":          turnCount,
		"message_count": len(failedMessages),
	})
	txn, txnErr := q.beginCompactionInstallTransaction(state)
	if txnErr != nil {
		return false, i18n.WrapInternalError(i18n.KeyLoopQuerySnapshotSkillCatalogFailed, txnErr)
	}
	compactorInput := cloneMessages(failedMessages)
	result, _, compactErr := q.runCompactionAgainst(ctx, "reactive", turnCount, txn.eventSink(onEvent), compactorInput, func() (*compact.CompactionResult, error) {
		result, didAttempt, err := compact.TryReactiveCompact(ctx, cloneMessages(compactorInput), compact.ReactiveCompactOptions{
			Compactor:    q.compactor,
			HasAttempted: state.HasAttemptedReactiveCompact,
			MediaStrip:   kind == recoverableFailureMediaSize,
			Trigger:      "reactive",
		})
		attempted = didAttempt
		return result, err
	})
	if compactErr != nil {
		if semantic, ok := i18n.DescribeSemanticError(compactErr); ok && semantic.Key == i18n.KeyLoopCompactionResultRejected {
			return false, txn.fail(q, state, onEvent, "reactive", turnCount, compactErr)
		}
		txn.publish(onEvent)
		return false, err
	}
	if !attempted || result == nil {
		if attempted {
			txn.publishNoop(onEvent)
		} else {
			txn.publish(onEvent)
		}
		return false, nil
	}

	replacement := compact.BuildPostCompactMessages(result)
	installed, installErr := q.installPostCompactVisibleHistory(failedMessages, replacement)
	if installErr != nil {
		return false, txn.fail(q, state, onEvent, "reactive", turnCount, installErr)
	}
	if cleanupErr := q.RunPostCompactCleanup(ctx, installed); cleanupErr != nil {
		return false, txn.fail(q, state, onEvent, "reactive", turnCount, cleanupErr)
	}
	state.Messages = installed
	state.HasAttemptedReactiveCompact = true
	state.MaxOutputTokensOverride = 0
	state.Transition = QueryTransitionReactiveCompactRetry
	if q.ctxWindow != nil {
		q.ctxWindow.RecordCompactSuccess()
	}
	q.updatePostCompactContext(result)
	txn.publish(onEvent)
	recoveryKind := "reactive_compaction"
	if kind == recoverableFailureMediaSize {
		recoveryKind = "media_removal"
	}
	onEvent(NewSystemWarningEvent(
		i18n.KeyRuntimeProviderRejectionRetry,
		nil,
		err,
		map[string]any{"recovery_kind": recoveryKind},
		turnCount,
	))
	return true, nil
}
