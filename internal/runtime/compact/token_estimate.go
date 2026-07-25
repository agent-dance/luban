package compact

import (
	"encoding/json"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

// EstimatedMediaTokensPerBlock is a deliberately provider-neutral budgeting
// estimate, not a provider-reported token count. It matches the existing MCP
// rich-content budget and keeps image/document blocks from being counted as
// zero before the provider returns authoritative usage.
const EstimatedMediaTokensPerBlock = 1600

// TokenOverheadKind makes every non-message component of a provider request
// explicit. Callers can distinguish an included estimate from an unavailable
// component instead of treating missing overhead as zero.
type TokenOverheadKind string

const (
	TokenOverheadSystemPrompt TokenOverheadKind = "system_prompt"
	TokenOverheadToolSchema   TokenOverheadKind = "tool_schema"
	TokenOverheadToolPayload  TokenOverheadKind = "tool_payload"
	TokenOverheadMedia        TokenOverheadKind = "media"
	TokenOverheadProtocol     TokenOverheadKind = "protocol"
)

type TokenEstimateBasis string

const (
	TokenEstimateEstimated  TokenEstimateBasis = "estimated"
	TokenEstimateConfigured TokenEstimateBasis = "configured"
	TokenEstimateUnknown    TokenEstimateBasis = "unknown"
)

type TokenOverheadEstimate struct {
	Kind   TokenOverheadKind  `json:"kind"`
	Tokens int                `json:"tokens,omitempty"`
	Basis  TokenEstimateBasis `json:"basis"`
}

// ModelContextOverhead supplies request components not present in []Message.
// Nil means unknown, while a non-nil zero means the caller proved absence.
type ModelContextOverhead struct {
	SystemPromptTokens *int
	ToolSchemaTokens   *int
	MediaTokens        *int
}

// ModelContextTokenEstimate is intentionally not a bare integer. Complete is
// false whenever a request component could not be measured.
type ModelContextTokenEstimate struct {
	MessageContentTokens int                     `json:"message_content_tokens"`
	ToolPayloadTokens    int                     `json:"tool_payload_tokens"`
	Overheads            []TokenOverheadEstimate `json:"overheads"`
	KnownTotalTokens     int                     `json:"known_total_tokens"`
	Complete             bool                    `json:"complete"`
	UnknownOverheads     []TokenOverheadKind     `json:"unknown_overheads,omitempty"`
}

type ContextUsageMeasurement string

const (
	ContextUsageUnknown          ContextUsageMeasurement = "unknown"
	ContextUsageProviderReported ContextUsageMeasurement = "provider_reported"
	ContextUsageLocalEstimate    ContextUsageMeasurement = "local_estimate"
	ContextUsageLocalLowerBound  ContextUsageMeasurement = "local_lower_bound"
)

// ContextInputUsage preserves whether the current input-side value is exact,
// a complete local estimate, or only a known lower bound.
type ContextInputUsage struct {
	UsedTokens  int
	Measurement ContextUsageMeasurement
}

func (cw *ContextWindow) CurrentInputUsage() ContextInputUsage {
	if cw == nil {
		return ContextInputUsage{Measurement: ContextUsageUnknown}
	}
	cw.usageMu.RLock()
	defer cw.usageMu.RUnlock()
	if cw.requestEstimateLive {
		measurement := ContextUsageLocalLowerBound
		if cw.requestEstimate.Complete {
			measurement = ContextUsageLocalEstimate
		}
		return ContextInputUsage{UsedTokens: max(cw.requestEstimate.KnownTotalTokens, 0), Measurement: measurement}
	}
	if !cw.providerUsageKnown {
		return ContextInputUsage{Measurement: ContextUsageUnknown}
	}
	return ContextInputUsage{UsedTokens: cw.UsedInput, Measurement: ContextUsageProviderReported}
}

func (cw *ContextWindow) currentInputTokens() int {
	if cw == nil {
		return 0
	}
	cw.usageMu.RLock()
	defer cw.usageMu.RUnlock()
	if cw.requestEstimateLive {
		return cw.requestEstimate.KnownTotalTokens
	}
	return cw.UsedInput
}

func (cw *ContextWindow) reportedInputTokens() int {
	if cw == nil {
		return 0
	}
	cw.usageMu.RLock()
	defer cw.usageMu.RUnlock()
	return cw.UsedInput
}

// EstimateMessagesDetailed estimates every request component available at the
// conversation layer. System prompts and tool schemas must be supplied by the
// request builder; media remains unknown unless its provider token cost is
// supplied. Protocol framing is represented separately and included.
func (cw *ContextWindow) EstimateMessagesDetailed(messages []types.Message, supplied ModelContextOverhead) ModelContextTokenEstimate {
	estimate := ModelContextTokenEstimate{Complete: true}
	if cw == nil || cw.Counter == nil {
		estimate.Complete = false
		estimate.UnknownOverheads = []TokenOverheadKind{TokenOverheadSystemPrompt, TokenOverheadToolSchema, TokenOverheadProtocol}
		return estimate
	}

	protocolTokens := 0
	hasMedia := false
	toolPayloadUnknown := false
	for _, message := range messages {
		// Role/envelope delimiters and the content-array framing are provider
		// protocol input even when the human-readable text is empty.
		protocolTokens += 4
		for _, block := range message.Content {
			protocolTokens += 2
			switch typed := block.(type) {
			case types.TextBlock:
				estimate.MessageContentTokens += cw.Counter.Count(typed.Text)
			case types.ThinkingBlock:
				estimate.MessageContentTokens += cw.Counter.Count(typed.Thinking)
			case types.ToolUseBlock:
				estimate.ToolPayloadTokens += cw.Counter.Count(typed.Name)
				if payload, err := json.Marshal(typed.Input); err == nil {
					estimate.ToolPayloadTokens += cw.Counter.Count(string(payload))
				} else {
					toolPayloadUnknown = true
				}
			case types.ToolResultBlock:
				estimate.ToolPayloadTokens += cw.Counter.Count(typed.TextContent())
				hasMedia = hasMedia || typed.HasMediaContent()
			case types.ToolReferenceBlock:
				estimate.ToolPayloadTokens += cw.Counter.Count(typed.ToolName)
			case types.ImageBlock, types.DocumentBlock:
				hasMedia = true
			case types.UnknownBlock:
				estimate.MessageContentTokens += cw.Counter.Count(string(typed.Raw))
			}
		}
	}

	estimate.Overheads = append(estimate.Overheads,
		tokenOverhead(TokenOverheadSystemPrompt, supplied.SystemPromptTokens, TokenEstimateConfigured),
		tokenOverhead(TokenOverheadToolSchema, supplied.ToolSchemaTokens, TokenEstimateConfigured),
		TokenOverheadEstimate{Kind: TokenOverheadProtocol, Tokens: protocolTokens, Basis: TokenEstimateEstimated},
	)
	if toolPayloadUnknown {
		estimate.Overheads = append(estimate.Overheads, TokenOverheadEstimate{Kind: TokenOverheadToolPayload, Basis: TokenEstimateUnknown})
	}
	if hasMedia || supplied.MediaTokens != nil {
		estimate.Overheads = append(estimate.Overheads, tokenOverhead(TokenOverheadMedia, supplied.MediaTokens, TokenEstimateConfigured))
	}

	estimate.KnownTotalTokens = estimate.MessageContentTokens + estimate.ToolPayloadTokens
	for _, overhead := range estimate.Overheads {
		if overhead.Basis == TokenEstimateUnknown {
			estimate.Complete = false
			estimate.UnknownOverheads = append(estimate.UnknownOverheads, overhead.Kind)
			continue
		}
		estimate.KnownTotalTokens += max(overhead.Tokens, 0)
	}
	return estimate
}

// EstimateProviderRequest accounts for the actual request envelope assembled
// by the query loop: messages, system prompt, visible tool/server schemas,
// media, and protocol framing. The result remains a local estimate; provider
// usage replaces it as soon as an API response reports authoritative input.
func (cw *ContextWindow) EstimateProviderRequest(params provider.Params) ModelContextTokenEstimate {
	systemTokens := 0
	if cw != nil && cw.Counter != nil {
		systemTokens = cw.Counter.Count(params.JoinedSystemPrompt())
	}
	mediaTokens := countRequestMediaBlocks(params.Messages) * EstimatedMediaTokensPerBlock

	var toolSchemaTokens *int
	if payload, err := json.Marshal(struct {
		Tools       []types.ToolDefinition       `json:"tools,omitempty"`
		ServerTools []types.ServerToolDefinition `json:"server_tools,omitempty"`
	}{Tools: params.Tools, ServerTools: params.ExtraToolSchemas}); err == nil {
		count := 0
		if cw != nil && cw.Counter != nil {
			count = cw.Counter.Count(string(payload))
		}
		toolSchemaTokens = &count
	}

	return cw.EstimateMessagesDetailed(params.Messages, ModelContextOverhead{
		SystemPromptTokens: &systemTokens,
		ToolSchemaTokens:   toolSchemaTokens,
		MediaTokens:        &mediaTokens,
	})
}

func countRequestMediaBlocks(messages []types.Message) int {
	count := 0
	var countBlocks func([]types.ContentBlock)
	countBlocks = func(blocks []types.ContentBlock) {
		for _, block := range blocks {
			switch typed := block.(type) {
			case types.ImageBlock, types.DocumentBlock:
				count++
			case types.ToolResultBlock:
				countBlocks(typed.ContentBlocks)
			}
		}
	}
	for _, message := range messages {
		countBlocks(message.Content)
	}
	return count
}

func tokenOverhead(kind TokenOverheadKind, tokens *int, basis TokenEstimateBasis) TokenOverheadEstimate {
	if tokens == nil {
		return TokenOverheadEstimate{Kind: kind, Basis: TokenEstimateUnknown}
	}
	return TokenOverheadEstimate{Kind: kind, Tokens: max(*tokens, 0), Basis: basis}
}
