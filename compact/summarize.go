package compact

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

// CompactMaxOutputTokens is the maximum number of output tokens for the
// summarization LLM call. Matches COMPACT_MAX_OUTPUT_TOKENS (= 20_000) in the
// original TS (src/utils/context.ts). The p99.99 of compact output is ~17,387
// tokens, so 20K provides adequate headroom without waste.
const CompactMaxOutputTokens = 20000

const compactSummarySchemaV2 = "compact-summary/v2"

type compactSummaryEnvelopeV2 struct {
	Schema  string `json:"schema"`
	Summary string `json:"summary"`
}

// SummarizeFunc is the function signature for the summarization LLM call.
// The customInstructions parameter allows callers (e.g. /compact <args>) to
// inject additional summarization instructions. Pass "" for default behavior.
type SummarizeFunc func(ctx context.Context, text string, customInstructions string) (string, error)

// MessageSummarizeFunc is the structured summarization path. It receives the
// conversation messages directly so providers can preserve tool_use inputs,
// tool_result content, message IDs, and mixed content blocks instead of relying
// on a flattened text transcript.
type MessageSummarizeFunc func(ctx context.Context, messages []types.Message, customInstructions string) (string, error)

// PartialMessageSummarizeFunc is the structured summarization path for partial
// compaction. Direction selects the from/up_to prompt variant.
type PartialMessageSummarizeFunc func(ctx context.Context, messages []types.Message, customInstructions string, direction PartialCompactDirection) (string, error)

// GetStructuredCompactPrompt returns the compact prompt without the flattened
// transcript separator. Structured requests send the conversation messages and
// compact prompt as distinct provider messages, so the extra separator used by
// GetCompactPrompt() would be misleading.
func GetStructuredCompactPrompt(customInstructions string) string {
	return buildCompactPrompt(customInstructions, false)
}

// NewLLMStructuredSummarizeFunc creates a MessageSummarizeFunc that calls the
// provider with conversation messages and the compact prompt separated. The
// request deliberately sends no tools and explicitly disables thinking so the
// compact model can only produce a text summary.
func NewLLMStructuredSummarizeFunc(p provider.Provider) MessageSummarizeFunc {
	return func(ctx context.Context, messages []types.Message, customInstructions string) (string, error) {
		ctx = provider.WithDebugCall(ctx, provider.DebugCallCompaction, nil)
		requestMessages := cloneMessages(messages)
		requestMessages = append(requestMessages, types.UserMessage(GetStructuredCompactPrompt(customInstructions)))

		params := provider.Params{
			System:    CompactSystemPrompt,
			Messages:  requestMessages,
			MaxTokens: CompactMaxOutputTokens,
			Tools:     nil,
			Thinking:  &provider.ThinkingConfig{Enabled: false},
		}

		return streamCompactSummary(ctx, p, params)
	}
}

// NewLLMStructuredPartialSummarizeFunc creates a structured summarizer using
// partial-compaction prompts.
func NewLLMStructuredPartialSummarizeFunc(p provider.Provider) PartialMessageSummarizeFunc {
	return func(ctx context.Context, messages []types.Message, customInstructions string, direction PartialCompactDirection) (string, error) {
		ctx = provider.WithDebugCall(ctx, provider.DebugCallCompaction, map[string]any{"direction": direction})
		requestMessages := cloneMessages(messages)
		requestMessages = append(requestMessages, types.UserMessage(GetStructuredPartialCompactPrompt(customInstructions, direction)))

		params := provider.Params{
			System:    CompactSystemPrompt,
			Messages:  requestMessages,
			MaxTokens: CompactMaxOutputTokens,
			Tools:     nil,
			Thinking:  &provider.ThinkingConfig{Enabled: false},
		}

		return streamCompactSummary(ctx, p, params)
	}
}

// NewLLMSummarizeFunc creates the flattened fallback SummarizeFunc that calls
// the provider to summarize text. The customInstructions parameter is passed
// through to GetCompactPrompt() so that user-specified compact instructions are
// included in the prompt. This is retained for providers/tests that cannot use
// the structured message path.
func NewLLMSummarizeFunc(p provider.Provider) SummarizeFunc {
	return func(ctx context.Context, text string, customInstructions string) (string, error) {
		ctx = provider.WithDebugCall(ctx, provider.DebugCallCompaction, nil)
		userMsg := GetCompactPrompt(customInstructions) + text

		params := provider.Params{
			System:    CompactSystemPrompt,
			Messages:  []types.Message{types.UserMessage(userMsg)},
			MaxTokens: CompactMaxOutputTokens,
		}

		return streamCompactSummary(ctx, p, params)
	}
}

// NewLLMPartialSummarizeFunc creates the flattened fallback summarizer using
// partial-compaction prompts.
func NewLLMPartialSummarizeFunc(p provider.Provider) PartialMessageSummarizeFunc {
	return func(ctx context.Context, messages []types.Message, customInstructions string, direction PartialCompactDirection) (string, error) {
		ctx = provider.WithDebugCall(ctx, provider.DebugCallCompaction, map[string]any{"direction": direction})
		userMsg := GetPartialCompactPrompt(customInstructions, direction) + buildSummarizeText(messages)

		params := provider.Params{
			System:    CompactSystemPrompt,
			Messages:  []types.Message{types.UserMessage(userMsg)},
			MaxTokens: CompactMaxOutputTokens,
		}

		return streamCompactSummary(ctx, p, params)
	}
}

func streamCompactSummary(ctx context.Context, p provider.Provider, params provider.Params) (string, error) {
	ch, err := p.CreateStream(ctx, params)
	if err != nil {
		if isCompactUserAbortCause(err) {
			return "", compactUserAbortError(err)
		}
		return "", i18n.WrapError(i18n.KeyCompactSummaryAPICallFailed, err)
	}

	var result strings.Builder
	var stopReason *types.StopReason
	for evt := range ch {
		if isCompactUserAbortCause(ctx.Err()) {
			return "", compactUserAbortError(ctx.Err())
		}
		switch evt.Type {
		case types.EventContentBlockDelta:
			if evt.Delta != nil && evt.Delta.Text != "" {
				result.WriteString(evt.Delta.Text)
			}
		case types.EventMessageStart:
			if evt.Message != nil {
				recordCompactUsage(ctx, &evt.Message.Usage)
			}
		case types.EventMessageDelta:
			recordCompactUsage(ctx, evt.Usage)
			if evt.StopReason != nil {
				stopReason = evt.StopReason
			}
		case types.EventError:
			if evt.Error != nil {
				if isCompactUserAbortCause(evt.Error) {
					return "", compactUserAbortError(evt.Error)
				}
				return "", i18n.WrapError(i18n.KeyCompactSummaryStreamFailed, evt.Error)
			}
		}
	}

	summary := result.String()
	if stopReason != nil && *stopReason == types.StopReasonMaxTokens {
		return "", compactError(ErrCompactIncomplete, MessageIncomplete, nil)
	}
	if summary == "" {
		return "", compactError(ErrCompactNoSummary, MessageNoSummary, nil)
	}
	if startsWithAPIErrorPrefix(summary) {
		return "", compactSummaryAPIError(summary)
	}
	return parseCompactSummaryEnvelope(summary)
}

func parseCompactSummaryEnvelope(raw string) (string, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	var envelope compactSummaryEnvelopeV2
	if err := decoder.Decode(&envelope); err != nil {
		return "", compactError(ErrCompactNoSummary, MessageNoSummary, nil)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return "", compactError(ErrCompactNoSummary, MessageNoSummary, nil)
	}
	if envelope.Schema != compactSummarySchemaV2 || strings.TrimSpace(envelope.Summary) == "" {
		return "", compactError(ErrCompactNoSummary, MessageNoSummary, nil)
	}
	return strings.TrimSpace(envelope.Summary), nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func cloneMessages(messages []types.Message) []types.Message {
	if len(messages) == 0 {
		return nil
	}
	out := make([]types.Message, len(messages))
	copy(out, messages)
	return out
}
