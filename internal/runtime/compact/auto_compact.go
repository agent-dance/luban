package compact

import (
	"context"
	"os"
	"strings"

	"github.com/agent-dance/luban/types"
)

// TriggeredCompactor is implemented by compactors that can record why a
// compact boundary was created.
type TriggeredCompactor interface {
	CompactWithTrigger(ctx context.Context, messages []types.Message, keepRecent int, trigger string) (*CompactionResult, error)
}

// AutoCompactOptions controls the pre-provider-call autocompact decision.
type AutoCompactOptions struct {
	Window     *ContextWindow
	Compactor  Compactor
	KeepRecent int
	Trigger    string
	// RequestEstimate accounts for the exact provider.Params envelope planned
	// by the caller. Nil uses the message estimate.
	RequestEstimate *ModelContextTokenEstimate
	OnTelemetry     func(CompactionTelemetryEvent)
}

// AutoCompactIfNeeded runs full compaction before a provider request when the
// current message view is over the autocompact threshold. It owns the
// consecutive-failure circuit breaker so callers do not retry forever.
func AutoCompactIfNeeded(ctx context.Context, messages []types.Message, opts AutoCompactOptions) (*CompactionResult, bool, error) {
	if !ShouldUseAutoCompact() {
		return nil, false, nil
	}
	if opts.Window == nil || opts.Compactor == nil {
		return nil, false, nil
	}
	if opts.Window.ConsecutiveFailures() >= MaxConsecutiveAutocompactFailures {
		return nil, false, nil
	}
	estimate := opts.Window.EstimateMessages(messages)
	if opts.RequestEstimate != nil {
		estimate = opts.RequestEstimate.KnownTotalTokens
	}
	if !opts.Window.shouldCompactEstimate(estimate) {
		return nil, false, nil
	}

	trigger := opts.Trigger
	if trigger == "" {
		trigger = "auto"
	}
	threshold := opts.Window.autoCompactThreshold()
	preEstimate := estimate
	emitAutoTelemetry(opts.OnTelemetry, CompactionTelemetryEvent{
		Kind:                       CompactionTelemetryAutoAttempt,
		Trigger:                    trigger,
		PreCompactTokenCount:       preEstimate,
		AutoCompactThreshold:       threshold,
		OriginalMessageCount:       len(messages),
		ConsecutiveFailureCount:    opts.Window.ConsecutiveFailures(),
		MaxConsecutiveFailureCount: MaxConsecutiveAutocompactFailures,
	})

	var (
		result *CompactionResult
		err    error
	)
	if triggered, ok := opts.Compactor.(TriggeredCompactor); ok {
		result, err = triggered.CompactWithTrigger(ctx, messages, opts.KeepRecent, trigger)
	} else {
		result, err = opts.Compactor.Compact(ctx, messages, opts.KeepRecent)
	}
	if err != nil {
		opts.Window.RecordCompactFailure()
		emitAutoFailure(opts.OnTelemetry, trigger, preEstimate, threshold, len(messages), opts.Window.ConsecutiveFailures(), err)
		return nil, true, err
	}
	opts.Window.RecordCompactSuccess()
	emitAutoSuccess(opts.OnTelemetry, trigger, preEstimate, threshold, len(messages), result)
	return result, true, nil
}

func emitAutoTelemetry(fn func(CompactionTelemetryEvent), event CompactionTelemetryEvent) {
	if fn != nil {
		fn(event)
	}
}

func emitAutoSuccess(fn func(CompactionTelemetryEvent), trigger string, preEstimate, threshold, originalMessages int, result *CompactionResult) {
	event := CompactionTelemetryEvent{
		Kind:                      CompactionTelemetryAutoSuccess,
		Trigger:                   trigger,
		PreCompactTokenCount:      preEstimate,
		AutoCompactThreshold:      threshold,
		OriginalMessageCount:      originalMessages,
		CompactedMessageCount:     len(BuildPostCompactMessages(result)),
		CompactionUsage:           UsageMetricsFromUsage(nil),
		PostCompactWouldRetrigger: isPostCompactLikelyToRetrigger(result, threshold),
	}
	if result != nil {
		event.PostCompactTokenCount = result.PostCompactTokenCount
		event.TruePostCompactTokenCount = result.TruePostCompactTokenCount
		event.CompactionUsage = UsageMetricsFromUsage(result.CompactionUsage)
	}
	emitAutoTelemetry(fn, event)
}

func emitAutoFailure(fn func(CompactionTelemetryEvent), trigger string, preEstimate, threshold, originalMessages, consecutiveFailures int, err error) {
	event := CompactionTelemetryEvent{
		Kind:                       CompactionTelemetryAutoFailure,
		Trigger:                    trigger,
		PreCompactTokenCount:       preEstimate,
		AutoCompactThreshold:       threshold,
		OriginalMessageCount:       originalMessages,
		ConsecutiveFailureCount:    consecutiveFailures,
		MaxConsecutiveFailureCount: MaxConsecutiveAutocompactFailures,
	}
	if err != nil {
		event.ErrorType = errCompactTelemetryType(err)
	}
	emitAutoTelemetry(fn, event)
}

// ShouldUseAutoCompact applies the process-level auto-compact gate shared by
// the loop and compact package.
func ShouldUseAutoCompact() bool {
	return !isEnvTruthy(os.Getenv("LUBAN_CODE_DISABLE_AUTO_COMPACT"))
}

func isEnvTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	default:
		return false
	}
}

// ShouldSnip reports whether a cheap history snip should run before
// microcompact/autocompact. It uses the message estimate because pre-call
// preparation may have stripped or compacted the session view since the last
// provider usage update.
func (cw *ContextWindow) ShouldSnip(messages []types.Message) bool {
	if cw == nil {
		return false
	}
	if cw.ConsecutiveFailures() >= MaxConsecutiveAutocompactFailures {
		return false
	}
	return cw.EstimateMessages(messages) > cw.autoCompactThreshold()
}

// ShouldSnipEstimate applies the cheap threshold gate to a complete planned
// provider request rather than message text alone.
func (cw *ContextWindow) ShouldSnipEstimate(estimate ModelContextTokenEstimate) bool {
	if cw == nil || cw.ConsecutiveFailures() >= MaxConsecutiveAutocompactFailures {
		return false
	}
	return estimate.KnownTotalTokens > cw.autoCompactThreshold()
}

func (cw *ContextWindow) shouldCompactEstimate(estimate int) bool {
	if cw == nil {
		return false
	}
	if cw.ConsecutiveFailures() >= MaxConsecutiveAutocompactFailures {
		return false
	}
	used := estimate
	if reported := cw.reportedInputTokens(); reported > used {
		used = reported
	}
	return used > cw.autoCompactThreshold()
}
