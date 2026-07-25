package compact

import (
	"math"
	"os"
	"strconv"
)

const (
	// ErrorThresholdBufferTokens matches ERROR_THRESHOLD_BUFFER_TOKENS in TS.
	ErrorThresholdBufferTokens = 20000

	// ManualCompactBufferTokens reserves room for a user to run /compact when
	// automatic compaction is disabled. Matches MANUAL_COMPACT_BUFFER_TOKENS.
	ManualCompactBufferTokens = 3000
)

// TokenWarningState is the TS-equivalent calculateTokenWarningState result.
type TokenWarningState struct {
	UsedTokens                  int
	EffectiveInputWindowTokens  int
	ThresholdTokens             int
	WarningThresholdTokens      int
	ErrorThresholdTokens        int
	AutoCompactThresholdTokens  int
	BlockingLimitTokens         int
	PercentLeft                 int
	IsAboveWarningThreshold     bool
	IsAboveErrorThreshold       bool
	IsAboveAutoCompactThreshold bool
	IsAtBlockingLimit           bool
	AutoCompactEnabled          bool
	WarningSuppressed           bool
}

type TokenWarningOptions struct {
	MaxTokens          int
	MaxOutputTokens    int
	TokenUsage         int
	AutoCompactEnabled bool
	SuppressWarning    bool
}

// CalculateTokenWarningState mirrors TS calculateTokenWarningState using the
// effective input window after output-token reservation.
func CalculateTokenWarningState(opts TokenWarningOptions) TokenWarningState {
	effectiveWindow := EffectiveInputWindowSize(opts.MaxTokens, opts.MaxOutputTokens)
	autoCompactThreshold := AutoCompactThresholdForWindow(effectiveWindow)
	if override := autoCompactPercentOverride(effectiveWindow); override > 0 && override < autoCompactThreshold {
		autoCompactThreshold = override
	}

	threshold := effectiveWindow
	if opts.AutoCompactEnabled {
		threshold = autoCompactThreshold
	}
	if threshold < 0 {
		threshold = 0
	}

	percentLeft := 0
	if threshold > 0 {
		percentLeft = int(math.Round((float64(threshold-opts.TokenUsage) / float64(threshold)) * 100))
		if percentLeft < 0 {
			percentLeft = 0
		}
	}

	warningThreshold := threshold - WarningThresholdBufferTokens
	if warningThreshold < 0 {
		warningThreshold = 0
	}
	errorThreshold := threshold - ErrorThresholdBufferTokens
	if errorThreshold < 0 {
		errorThreshold = 0
	}

	blockingLimit := effectiveWindow - ManualCompactBufferTokens
	if override := blockingLimitOverride(); override > 0 {
		blockingLimit = override
	}
	if blockingLimit < 0 {
		blockingLimit = 0
	}

	state := TokenWarningState{
		UsedTokens:                 opts.TokenUsage,
		EffectiveInputWindowTokens: effectiveWindow,
		ThresholdTokens:            threshold,
		WarningThresholdTokens:     warningThreshold,
		ErrorThresholdTokens:       errorThreshold,
		AutoCompactThresholdTokens: autoCompactThreshold,
		BlockingLimitTokens:        blockingLimit,
		PercentLeft:                percentLeft,
		AutoCompactEnabled:         opts.AutoCompactEnabled,
		WarningSuppressed:          opts.SuppressWarning,
		IsAtBlockingLimit:          opts.TokenUsage >= blockingLimit,
	}
	if !opts.SuppressWarning {
		state.IsAboveWarningThreshold = opts.TokenUsage >= warningThreshold
		state.IsAboveErrorThreshold = opts.TokenUsage >= errorThreshold
		state.IsAboveAutoCompactThreshold = opts.AutoCompactEnabled && opts.TokenUsage >= autoCompactThreshold
	}
	return state
}

// EffectiveInputWindowSize returns context window capacity available for input
// after reserving output tokens.
func EffectiveInputWindowSize(maxTokens, maxOutputTokens int) int {
	outputReserve := maxOutputTokens
	if outputReserve <= 0 || outputReserve > WarningThresholdBufferTokens {
		outputReserve = WarningThresholdBufferTokens
	}
	eff := maxTokens - outputReserve
	if eff < 0 {
		return 0
	}
	return eff
}

func AutoCompactThresholdForWindow(effectiveWindow int) int {
	t := effectiveWindow - AutocompactBufferTokens
	if t < 0 {
		return 0
	}
	return t
}

func autoCompactPercentOverride(effectiveWindow int) int {
	raw := os.Getenv("LUBAN_AUTOCOMPACT_PCT_OVERRIDE")
	if raw == "" {
		return 0
	}
	parsed, err := strconv.ParseFloat(raw, 64)
	if err != nil || parsed <= 0 || parsed > 100 {
		return 0
	}
	return int(math.Floor(float64(effectiveWindow) * (parsed / 100)))
}

func blockingLimitOverride() int {
	raw := os.Getenv("LUBAN_CODE_BLOCKING_LIMIT_OVERRIDE")
	if raw == "" {
		return 0
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}
