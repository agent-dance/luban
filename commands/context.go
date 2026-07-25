package commands

import (
	"math"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/provider"
)

// ---------------------------------------------------------------------------
// /context  (/ctx)
// ---------------------------------------------------------------------------

type contextCmd struct{}

type contextWarningProvider interface {
	ContextWarningState() compact.TokenWarningState
}

type contextUsageDetailProvider interface {
	ContextUsageDetail() (int, compact.ContextInputUsage)
}

func (c *contextCmd) Name() string      { return "context" }
func (c *contextCmd) Aliases() []string { return []string{"ctx"} }
func (c *contextCmd) Description() string {
	return builtinCommandDescription("context")
}

func (c *contextCmd) Execute(ctx *Context, _ string) error {
	messages := ctx.QueryLoop.Messages()
	msgCount := len(messages)

	model := ctx.CurrentModel
	if model == "" {
		model = ctx.QueryLoop.Model()
	}
	if model == "" {
		model = i18n.Text(ctx.Language, i18n.KeyCommandContextUnknown)
	}
	maxCtx := provider.LookupMaxContext(model)
	currentContextTokens := 0
	warningState := compact.TokenWarningState{}
	usageKnown := false
	source := i18n.Text(ctx.Language, i18n.KeyCommandContextLoopTracker)
	measurement := compact.ContextUsageUnknown
	if detailProvider, ok := ctx.QueryLoop.(contextUsageDetailProvider); ok {
		maxTokens, usage := detailProvider.ContextUsageDetail()
		if maxTokens > 0 && usage.Measurement != compact.ContextUsageUnknown {
			maxCtx = maxTokens
			currentContextTokens = usage.UsedTokens
			usageKnown = true
			measurement = usage.Measurement
		}
	} else if maxTokens, usedTokens := ctx.QueryLoop.ContextUsage(); maxTokens > 0 {
		maxCtx = maxTokens
		currentContextTokens = usedTokens
		usageKnown = true
		measurement = compact.ContextUsageProviderReported
	}
	if usageKnown {
		if warningProvider, ok := ctx.QueryLoop.(contextWarningProvider); ok {
			state := warningProvider.ContextWarningState()
			if state.UsedTokens == currentContextTokens {
				warningState = state
			}
		}
	}
	if !usageKnown {
		maxCtx = 0
	}
	if warningState.EffectiveInputWindowTokens == 0 && maxCtx > 0 {
		warningState = compact.CalculateTokenWarningState(compact.TokenWarningOptions{
			MaxTokens:          maxCtx + compact.WarningThresholdBufferTokens,
			TokenUsage:         currentContextTokens,
			AutoCompactEnabled: compact.ShouldUseAutoCompact(),
		})
		warningState.EffectiveInputWindowTokens = maxCtx
	}

	var sb strings.Builder
	sb.WriteString(i18n.Format(ctx.Language, i18n.KeyCommandContextReport, model, msgCount))
	if maxCtx > 0 {
		pct := float64(currentContextTokens) / float64(maxCtx) * 100
		remaining := maxCtx - currentContextTokens
		if remaining < 0 {
			remaining = 0
		}
		usageKey := i18n.KeyCommandContextUsageExact
		remainingKey := i18n.KeyCommandContextRemaining
		switch measurement {
		case compact.ContextUsageLocalEstimate:
			usageKey = i18n.KeyCommandContextUsageEstimated
			remainingKey = i18n.KeyCommandContextRemainingEstimated
			source = i18n.Text(ctx.Language, i18n.KeyCommandContextSourceEstimate)
		case compact.ContextUsageLocalLowerBound:
			usageKey = i18n.KeyCommandContextUsageLowerBound
			remainingKey = i18n.KeyCommandContextRemainingUpperBound
			source = i18n.Text(ctx.Language, i18n.KeyCommandContextSourceLowerBound)
		default:
			source = i18n.Text(ctx.Language, i18n.KeyCommandContextSourceProvider)
		}
		sb.WriteString(i18n.Format(ctx.Language, usageKey, formatTokens(currentContextTokens), formatTokens(maxCtx), pct))
		remainingPercent := int(math.Round(float64(remaining) / float64(maxCtx) * 100))
		sb.WriteString(i18n.Format(ctx.Language, remainingKey, remainingPercent, formatTokens(remaining)))
		if warningState.IsAboveAutoCompactThreshold {
			sb.WriteString(i18n.Text(ctx.Language, i18n.KeyCommandContextAutoCompact))
		} else if warningState.IsAtBlockingLimit {
			sb.WriteString(i18n.Text(ctx.Language, i18n.KeyCommandContextBlocking))
		} else if warningState.IsAboveErrorThreshold {
			sb.WriteString(i18n.Text(ctx.Language, i18n.KeyCommandContextCritical))
		} else if warningState.IsAboveWarningThreshold {
			sb.WriteString(i18n.Text(ctx.Language, i18n.KeyCommandContextLow))
		}
	} else {
		sb.WriteString(i18n.Text(ctx.Language, i18n.KeyCommandContextUnavailable))
	}
	sb.WriteString(i18n.Format(ctx.Language, i18n.KeyCommandContextSource, source))

	ctx.OnEvent(sb.String())
	return nil
}
