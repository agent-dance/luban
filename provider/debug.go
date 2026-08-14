package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/prompt"
	"github.com/agent-dance/luban/types"
)

// OpenDebugFile opens path for append and restricts it to the current user.
// Debug output can contain system prompts, local file contents, and tool data.
func OpenDebugFile(path string) (*os.File, error) {
	if strings.TrimSpace(path) == "" {
		return nil, i18n.NewError(i18n.KeyProviderDebugPathEmpty)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyProviderDebugFileOpenFailed, err, path)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, i18n.WrapError(i18n.KeyProviderDebugPermissionsFailed, err, path)
	}
	return file, nil
}

// DebugPhase identifies which side of an LLM exchange a debug event describes.
type DebugPhase string

const (
	DebugPhaseRequest  DebugPhase = "request"
	DebugPhaseResponse DebugPhase = "response"
)

// DebugCallKind identifies the runtime operation that initiated an LLM call.
type DebugCallKind string

const (
	DebugCallConversation DebugCallKind = "conversation"
	DebugCallCompaction   DebugCallKind = "compaction"
)

// DebugObserver receives opt-in, developer-facing snapshots of LLM calls.
// HTTP headers and credentials are intentionally outside this abstraction.
type DebugObserver func(DebugEvent)

// DebugEvent is one half of a request/response pair. ID correlates both halves.
type DebugEvent struct {
	ID       uint64         `json:"id"`
	Phase    DebugPhase     `json:"phase"`
	Kind     DebugCallKind  `json:"kind"`
	Provider string         `json:"provider"`
	Model    string         `json:"model"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Request  *DebugRequest  `json:"request,omitempty"`
	Response *DebugResponse `json:"response,omitempty"`
}

// DebugRequest is the complete provider-neutral payload handed to the active
// provider after prompt construction, history preparation, and compaction.
type DebugRequest struct {
	Stream                   bool                         `json:"stream"`
	Model                    string                       `json:"model"`
	MaxTokens                int                          `json:"max_tokens"`
	MaxOutputTokensOverride  int                          `json:"max_output_tokens_override,omitempty"`
	EffectiveMaxOutputTokens int                          `json:"effective_max_output_tokens,omitempty"`
	CatalogMaxOutputTokens   int                          `json:"catalog_max_output_tokens,omitempty"`
	System                   string                       `json:"system,omitempty"`
	SystemBlocks             []prompt.SystemPromptBlock   `json:"system_blocks,omitempty"`
	Messages                 []types.Message              `json:"messages"`
	Tools                    []types.ToolDefinition       `json:"tools,omitempty"`
	ExtraToolSchemas         []types.ServerToolDefinition `json:"extra_tool_schemas,omitempty"`
	ToolChoice               *ToolChoice                  `json:"tool_choice,omitempty"`
	Thinking                 *ThinkingConfig              `json:"thinking,omitempty"`
	TaskBudget               *TaskBudget                  `json:"task_budget,omitempty"`
	Conversation             string                       `json:"conversation,omitempty"`
	PreviousResponseID       string                       `json:"previous_response_id,omitempty"`
	Truncation               string                       `json:"truncation,omitempty"`
	PromptCacheKey           string                       `json:"prompt_cache_key,omitempty"`
	ReasoningEffort          string                       `json:"reasoning_effort,omitempty"`
	ServiceTier              ServiceTier                  `json:"service_tier,omitempty"`
	UsePromptCache           bool                         `json:"use_prompt_cache,omitempty"`
}

// DebugResponse is the semantic model result reconstructed from the provider
// stream. Token-level deltas are collapsed into content and tool-use blocks.
type DebugResponse struct {
	Message           *types.Message                   `json:"message,omitempty"`
	Usage             *types.Usage                     `json:"usage,omitempty"`
	StopReason        *types.StopReason                `json:"stop_reason,omitempty"`
	ResponseID        string                           `json:"response_id,omitempty"`
	SystemFingerprint string                           `json:"system_fingerprint,omitempty"`
	Error             string                           `json:"error,omitempty"`
	FailureDiagnostic *types.ProviderFailureDiagnostic `json:"failure_diagnostic,omitempty"`
}

type debugCallContext struct {
	kind     DebugCallKind
	metadata map[string]any
}

type debugCallContextKey struct{}

// WithDebugCall marks why an LLM request is being made. Nested calls preserve
// existing metadata and let the most specific non-empty values win.
func WithDebugCall(ctx context.Context, kind DebugCallKind, metadata map[string]any) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	current := debugCallContextFrom(ctx)
	if kind != "" {
		current.kind = kind
	}
	if len(metadata) > 0 {
		merged := cloneDebugMetadata(current.metadata)
		if merged == nil {
			merged = make(map[string]any, len(metadata))
		}
		for key, value := range metadata {
			merged[key] = value
		}
		current.metadata = merged
	}
	return context.WithValue(ctx, debugCallContextKey{}, current)
}

func debugCallContextFrom(ctx context.Context) debugCallContext {
	if ctx != nil {
		if value, ok := ctx.Value(debugCallContextKey{}).(debugCallContext); ok {
			if value.kind == "" {
				value.kind = DebugCallConversation
			}
			value.metadata = cloneDebugMetadata(value.metadata)
			return value
		}
	}
	return debugCallContext{kind: DebugCallConversation}
}

func cloneDebugMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func newDebugRequest(params Params, model string) *DebugRequest {
	if params.Model != "" {
		model = params.Model
	}
	effectiveMaxOutputTokens := params.MaxTokens
	if params.MaxOutputTokensOverride > 0 {
		effectiveMaxOutputTokens = params.MaxOutputTokensOverride
	}
	return &DebugRequest{
		Stream:                   true,
		Model:                    model,
		MaxTokens:                params.MaxTokens,
		MaxOutputTokensOverride:  params.MaxOutputTokensOverride,
		EffectiveMaxOutputTokens: effectiveMaxOutputTokens,
		CatalogMaxOutputTokens:   LookupMaxOutput(model),
		System:                   params.JoinedSystemPrompt(),
		SystemBlocks:             params.SystemTextBlocks(),
		Messages:                 sanitizeDebugMessages(params.Messages),
		Tools:                    params.Tools,
		ExtraToolSchemas:         params.ExtraToolSchemas,
		ToolChoice:               params.ToolChoice,
		Thinking:                 params.Thinking,
		TaskBudget:               params.TaskBudget,
		Conversation:             params.Conversation,
		PreviousResponseID:       params.PreviousResponseID,
		Truncation:               params.Truncation,
		PromptCacheKey:           params.PromptCacheKey,
		ReasoningEffort:          params.ReasoningEffort,
		ServiceTier:              params.ServiceTier,
		UsePromptCache:           params.UsePromptCache,
	}
}

// sanitizeDebugMessages removes opaque provider continuation state before an
// opt-in debug observer receives the request. Encrypted reasoning is necessary
// on the wire but is neither useful diagnostic text nor safe log material.
func sanitizeDebugMessages(messages []types.Message) []types.Message {
	result := make([]types.Message, len(messages))
	for i, message := range messages {
		result[i] = message
		result[i].ClearProviderContinuation()
		result[i].Content = make([]types.ContentBlock, len(message.Content))
		for j, block := range message.Content {
			switch thinking := block.(type) {
			case types.ThinkingBlock:
				result[i].Content[j] = sanitizeDebugThinkingBlock(thinking)
				continue
			case *types.ThinkingBlock:
				if thinking == nil {
					result[i].Content[j] = thinking
					continue
				}
				cloned := sanitizeDebugThinkingBlock(*thinking)
				result[i].Content[j] = &cloned
				continue
			}
			result[i].Content[j] = block
		}
	}
	return result
}

func sanitizeDebugThinkingBlock(thinking types.ThinkingBlock) types.ThinkingBlock {
	thinking.Signature = ""
	thinking.SignatureKind = ""
	thinking.SignatureModel = ""
	thinking.ProviderItemID = ""
	thinking.ProviderStatus = ""
	return thinking
}

type debugResponseBlock struct {
	kind        types.ContentType
	toolType    types.ToolDefinitionType
	text        strings.Builder
	thinking    strings.Builder
	partialJSON strings.Builder
	rawInput    strings.Builder
	id          string
	name        string
	rawJSON     json.RawMessage
}

func newDebugResponse(events []types.StreamEvent, responseError string) *DebugResponse {
	response := &DebugResponse{Error: responseError}
	blocks := make(map[int]*debugResponseBlock)

	for _, event := range events {
		if event.SystemFingerprint != "" {
			response.SystemFingerprint = event.SystemFingerprint
		}
		switch event.Type {
		case types.EventMessageStart:
			mergeDebugUsage(&response.Usage, event.Usage)
		case types.EventContentBlockStart:
			block := debugBlock(blocks, event.Index)
			if event.ContentBlock != nil {
				applyDebugDelta(block, event.ContentBlock)
			}
		case types.EventContentBlockDelta:
			block := debugBlock(blocks, event.Index)
			if event.Delta != nil {
				applyDebugDelta(block, event.Delta)
			}
		case types.EventMessageDelta:
			mergeDebugUsage(&response.Usage, event.Usage)
			if event.StopReason != nil {
				stopReason := *event.StopReason
				response.StopReason = &stopReason
			}
		case types.EventMessageStop:
			response.ResponseID = event.ResponseID
		case types.EventError:
			if event.Error != nil {
				response.Error = event.Error.Error()
				response.FailureDiagnostic = event.Error.FailureDiagnostic.Clone()
			}
		}
	}

	content := buildDebugContent(blocks)
	if len(content) > 0 {
		response.Message = &types.Message{Role: types.RoleAssistant, Content: content}
	}
	return response
}

func debugBlock(blocks map[int]*debugResponseBlock, index int) *debugResponseBlock {
	block := blocks[index]
	if block == nil {
		block = &debugResponseBlock{}
		blocks[index] = block
	}
	return block
}

func applyDebugDelta(block *debugResponseBlock, delta *types.ContentDelta) {
	switch delta.Type {
	case types.ContentTypeText, "text_delta":
		block.kind = types.ContentTypeText
	case types.ContentTypeThinking, "thinking_delta", "signature_delta":
		block.kind = types.ContentTypeThinking
	case types.ContentTypeToolUse, "input_json_delta", "input_text_delta", "tool_state_final":
		block.kind = types.ContentTypeToolUse
	default:
		if delta.Type != "" && block.kind == "" {
			block.kind = delta.Type
		}
	}
	block.text.WriteString(delta.Text)
	block.thinking.WriteString(delta.Thinking)
	if delta.ToolType == types.ToolDefinitionTypeCustom || delta.Type == "input_text_delta" {
		block.toolType = types.ToolDefinitionTypeCustom
	}
	if delta.Type == "tool_state_final" {
		if block.toolType == types.ToolDefinitionTypeCustom {
			block.rawInput.Reset()
			block.rawInput.WriteString(delta.PartialText)
		} else if delta.PartialJSON != "" {
			block.partialJSON.Reset()
			block.partialJSON.WriteString(delta.PartialJSON)
		}
	} else {
		block.partialJSON.WriteString(delta.PartialJSON)
		block.rawInput.WriteString(delta.PartialText)
	}
	if delta.ID != "" {
		block.id = delta.ID
	}
	if delta.Name != "" {
		block.name = delta.Name
	}
	// Signatures may contain encrypted provider reasoning. Do not copy them
	// into the developer-facing debug response.
	if len(delta.RawJSON) > 0 {
		block.rawJSON = append(block.rawJSON[:0], delta.RawJSON...)
	}
}

func buildDebugContent(blocks map[int]*debugResponseBlock) []types.ContentBlock {
	indexes := make([]int, 0, len(blocks))
	for index := range blocks {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	content := make([]types.ContentBlock, 0, len(indexes))
	for _, index := range indexes {
		block := blocks[index]
		switch block.kind {
		case types.ContentTypeText:
			content = append(content, types.TextBlock{Type: types.ContentTypeText, Text: block.text.String()})
		case types.ContentTypeThinking:
			content = append(content, types.ThinkingBlock{Type: types.ContentTypeThinking, Thinking: block.thinking.String()})
		case types.ContentTypeToolUse:
			if block.toolType == types.ToolDefinitionTypeCustom {
				content = append(content, types.ToolUseBlock{
					Type: types.ContentTypeToolUse, ID: block.id, Name: block.name,
					ToolType: types.ToolDefinitionTypeCustom, RawInput: block.rawInput.String(),
				})
				continue
			}
			input := make(map[string]any)
			raw := block.partialJSON.String()
			if raw != "" {
				if err := json.Unmarshal([]byte(raw), &input); err != nil {
					input = map[string]any{"_partial_json": raw, "_parse_error": err.Error()}
				}
			}
			content = append(content, types.ToolUseBlock{Type: types.ContentTypeToolUse, ID: block.id, Name: block.name, Input: input})
		default:
			raw := block.rawJSON
			if len(raw) == 0 {
				raw, _ = json.Marshal(map[string]any{
					"type":     block.kind,
					"text":     block.text.String(),
					"thinking": block.thinking.String(),
				})
			}
			content = append(content, types.UnknownBlock{Type: block.kind, Raw: raw})
		}
	}
	return content
}

func mergeDebugUsage(target **types.Usage, update *types.Usage) {
	if update == nil {
		return
	}
	if *target == nil {
		copied := *update
		*target = &copied
		return
	}
	current := *target
	if update.InputTokens != 0 {
		current.InputTokens = update.InputTokens
	}
	if update.OutputTokens != 0 {
		current.OutputTokens = update.OutputTokens
	}
	if update.CacheCreationInputTokens != 0 {
		current.CacheCreationInputTokens = update.CacheCreationInputTokens
	}
	if update.CacheReadInputTokens != 0 {
		current.CacheReadInputTokens = update.CacheReadInputTokens
	}
	if update.ServerToolUse.WebSearchRequests != 0 {
		current.ServerToolUse.WebSearchRequests = update.ServerToolUse.WebSearchRequests
	}
	if update.ServerToolUse.WebFetchRequests != 0 {
		current.ServerToolUse.WebFetchRequests = update.ServerToolUse.WebFetchRequests
	}
}

// FormatDebugEvent renders a debug exchange half as readable JSON.
func FormatDebugEvent(event DebugEvent) (string, error) {
	var payload any
	label := "request"
	switch event.Phase {
	case DebugPhaseRequest:
		payload = event.Request
	case DebugPhaseResponse:
		label = "response"
		payload = event.Response
	default:
		return "", i18n.NewError(i18n.KeyLogDebugUnknownPhase, event.Phase)
	}
	if payload == nil {
		return "", i18n.NewError(i18n.KeyLogDebugPayloadNil, event.Phase)
	}
	body := struct {
		Metadata map[string]any `json:"metadata,omitempty"`
		Payload  any            `json:"payload"`
	}{Metadata: event.Metadata, Payload: payload}
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return "", i18n.WrapError(i18n.KeyLogDebugMarshalFailed, err, event.Phase)
	}
	kind := event.Kind
	if kind == "" {
		kind = DebugCallConversation
	}
	return fmt.Sprintf("[Debug] %s %s #%d (%s/%s)\n%s", kind, label, event.ID, event.Provider, event.Model, data), nil
}

// NewDebugWriterObserver writes complete formatted events serially to w.
func NewDebugWriterObserver(w io.Writer) DebugObserver {
	if w == nil {
		w = io.Discard
	}
	var mu sync.Mutex
	return func(event DebugEvent) {
		formatted, err := FormatDebugEvent(event)
		if err != nil {
			formatted = "[Debug] " + err.Error()
		}
		mu.Lock()
		defer mu.Unlock()
		_, _ = fmt.Fprintln(w, formatted)
	}
}
