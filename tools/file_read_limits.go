package tools

import (
	"context"
	"os"
	"strconv"
	"strings"
)

const fileReadMaxOutputTokensEnv = "CLAUDE_CODE_FILE_READ_MAX_OUTPUT_TOKENS"

// FileReadingLimits is the effective text/notebook Read budget.
type FileReadingLimits struct {
	MaxTokens              int
	MaxSizeBytes           int64
	IncludeMaxSizeInPrompt bool
	TargetedRangeNudge     bool
}

// FileReadingLimitsOverride is partial so runtime/session overrides can change
// one cap without silently zeroing the other.
type FileReadingLimitsOverride struct {
	MaxTokens              *int
	MaxSizeBytes           *int64
	IncludeMaxSizeInPrompt *bool
	TargetedRangeNudge     *bool
}

type fileReadingLimitsContextKey struct{}
type fileReadMessageIDContextKey struct{}

// WithFileReadingLimits attaches a per-call Read budget override. Session
// owners can instead set FileReadTool.ReadingLimitsProvider.
func WithFileReadingLimits(ctx context.Context, override FileReadingLimitsOverride) context.Context {
	return context.WithValue(ctx, fileReadingLimitsContextKey{}, override)
}

// WithFileReadMessageID adds the optional parent assistant message ID used by
// tengu_session_file_read analytics.
func WithFileReadMessageID(ctx context.Context, messageID string) context.Context {
	return context.WithValue(ctx, fileReadMessageIDContextKey{}, strings.TrimSpace(messageID))
}

func fileReadMessageID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(fileReadMessageIDContextKey{}).(string)
	return strings.TrimSpace(value)
}

func defaultFileReadingLimits() FileReadingLimits {
	return FileReadingLimits{
		MaxTokens:    defaultReadMaxTokens,
		MaxSizeBytes: defaultReadMaxSizeBytes,
	}
}

func validFileReadingLimitsOverride(override FileReadingLimitsOverride) bool {
	return (override.MaxTokens != nil && *override.MaxTokens > 0) ||
		(override.MaxSizeBytes != nil && *override.MaxSizeBytes > 0) ||
		override.IncludeMaxSizeInPrompt != nil || override.TargetedRangeNudge != nil
}

func applyFileReadingLimitsOverride(limits *FileReadingLimits, override FileReadingLimitsOverride) {
	if limits == nil {
		return
	}
	if override.MaxTokens != nil && *override.MaxTokens > 0 {
		limits.MaxTokens = *override.MaxTokens
	}
	if override.MaxSizeBytes != nil && *override.MaxSizeBytes > 0 {
		limits.MaxSizeBytes = *override.MaxSizeBytes
	}
	if override.IncludeMaxSizeInPrompt != nil {
		limits.IncludeMaxSizeInPrompt = *override.IncludeMaxSizeInPrompt
	}
	if override.TargetedRangeNudge != nil {
		limits.TargetedRangeNudge = *override.TargetedRangeNudge
	}
}

func mergeFileReadingLimitsOverride(dst *FileReadingLimitsOverride, src FileReadingLimitsOverride) {
	if dst == nil {
		return
	}
	if src.MaxTokens != nil && *src.MaxTokens > 0 {
		dst.MaxTokens = src.MaxTokens
	}
	if src.MaxSizeBytes != nil && *src.MaxSizeBytes > 0 {
		dst.MaxSizeBytes = src.MaxSizeBytes
	}
	if src.IncludeMaxSizeInPrompt != nil {
		dst.IncludeMaxSizeInPrompt = src.IncludeMaxSizeInPrompt
	}
	if src.TargetedRangeNudge != nil {
		dst.TargetedRangeNudge = src.TargetedRangeNudge
	}
}

func (t *FileReadTool) resolveFileReadingLimits(ctx context.Context) FileReadingLimits {
	limits := defaultFileReadingLimits()
	if t != nil && t.DefaultReadingLimitsProvider != nil {
		applyFileReadingLimitsOverride(&limits, t.DefaultReadingLimitsProvider())
	}
	if raw := strings.TrimSpace(os.Getenv(fileReadMaxOutputTokensEnv)); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limits.MaxTokens = parsed
		}
	}

	var runtimeOverride FileReadingLimitsOverride
	hasRuntimeOverride := false
	if t != nil && t.ReadingLimitsProvider != nil {
		if override := t.ReadingLimitsProvider(ctx); override != nil && validFileReadingLimitsOverride(*override) {
			runtimeOverride = *override
			hasRuntimeOverride = true
		}
	}
	if ctx != nil {
		if override, ok := ctx.Value(fileReadingLimitsContextKey{}).(FileReadingLimitsOverride); ok && validFileReadingLimitsOverride(override) {
			mergeFileReadingLimitsOverride(&runtimeOverride, override)
			hasRuntimeOverride = true
		}
	}
	if hasRuntimeOverride {
		applyFileReadingLimitsOverride(&limits, runtimeOverride)
		t.emitAnalytics("tengu_file_read_limits_override", map[string]any{
			"hasMaxTokens":    runtimeOverride.MaxTokens != nil,
			"hasMaxSizeBytes": runtimeOverride.MaxSizeBytes != nil,
		})
	}
	return limits
}
