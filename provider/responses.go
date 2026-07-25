package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// ResponsesProvider implements Provider for the OpenAI Responses API (/v1/responses).
// It is stateless. The official public API can chain conversations by passing
// PreviousResponseID in Params and reading ResponseID from EventMessageStop.
// Custom and ChatGPT-backed HTTP endpoints receive full input because their
// response IDs are not necessarily valid for HTTP chaining.
type ResponsesProvider struct {
	mu                  sync.RWMutex
	name                string
	baseURL             string
	apiKey              string
	model               string
	maxTokens           int
	timeout             time.Duration
	headers             map[string]string
	chatGPTCodexBackend bool
	firstPartyEndpoint  bool
	publicAPIEndpoint   bool
	disableStrictTools  bool
	cacheRouting        CacheRoutingMode
	cacheUserNamespace  string
	cacheRoutingShards  int
	unsupportedFields   sync.Map
	client              *http.Client
}

// NewResponses creates a Provider for the OpenAI Responses API.
func NewResponses(cfg Config) *ResponsesProvider {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	// Strip only trailing slashes; the configured API version remains part of
	// the base URL before we append /responses.
	baseURL = strings.TrimSuffix(baseURL, "/")

	model := cfg.Model
	if model == "" {
		model = CatalogDefaultModel("openai", "gpt-5.6-sol")
	}
	maxTokens := cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = 16384
	}
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout == 0 {
		timeout = 600 * time.Second
	}
	bearerToken := cfg.APIKey
	if authToken := strings.TrimSpace(cfg.AuthToken); authToken != "" {
		bearerToken = authToken
	}
	cacheUserNamespace := promptCacheUserNamespace(cfg)
	providerName := CanonicalProviderName(cfg.ProviderName)
	if providerName == "" {
		providerName = "openai"
	}

	return &ResponsesProvider{
		name:                providerName,
		baseURL:             baseURL,
		apiKey:              bearerToken,
		model:               model,
		maxTokens:           maxTokens,
		timeout:             timeout,
		headers:             cloneHeaders(cfg.Headers),
		chatGPTCodexBackend: isOpenAIChatGPTCodexBaseURL(baseURL),
		firstPartyEndpoint:  isFirstPartyOpenAIResponsesBaseURL(baseURL),
		publicAPIEndpoint:   isOpenAIPublicAPIBaseURL(baseURL),
		disableStrictTools:  cfg.DisableStrictTools,
		cacheRouting:        cacheRoutingModeForResponses(cfg.CacheRoutingPreference),
		cacheUserNamespace:  cacheUserNamespace,
		cacheRoutingShards:  promptCacheRoutingShardCount(cfg.ProviderName),
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func cacheRoutingModeForResponses(preference CacheRoutingPreference) CacheRoutingMode {
	if preference == CacheRoutingOff {
		return CacheRoutingNone
	}
	return CacheRoutingPromptCacheKey
}

func (p *ResponsesProvider) Name() string { return p.name }
func (p *ResponsesProvider) ModelID() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.model
}

// ApplyCredentialConfig updates request authentication and routing after an
// OAuth refresh without rebuilding the provider wrapper.
func (p *ResponsesProvider) ApplyCredentialConfig(cfg Config) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	bearerToken := cfg.APIKey
	if authToken := strings.TrimSpace(cfg.AuthToken); authToken != "" {
		bearerToken = authToken
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.baseURL = baseURL
	p.apiKey = bearerToken
	p.headers = cloneHeaders(cfg.Headers)
	p.chatGPTCodexBackend = isOpenAIChatGPTCodexBaseURL(baseURL)
	p.firstPartyEndpoint = isFirstPartyOpenAIResponsesBaseURL(baseURL)
	p.publicAPIEndpoint = isOpenAIPublicAPIBaseURL(baseURL)
	p.disableStrictTools = cfg.DisableStrictTools
	p.cacheUserNamespace = promptCacheUserNamespace(cfg)
	p.cacheRoutingShards = promptCacheRoutingShardCount(cfg.ProviderName)
}

// APIFormat returns the API protocol used by this provider.
func (p *ResponsesProvider) APIFormat() string { return "responses" }

// Capabilities implements CapabilityProvider for ResponsesProvider.
func (p *ResponsesProvider) Capabilities() ProviderCapabilities {
	p.mu.RLock()
	model := p.model
	p.mu.RUnlock()
	return ProviderCapabilities{
		Thinking:     false,
		ToolUse:      true,
		CacheControl: false,
		CacheRouting: p.cacheRouting,
		SystemParts:  true,
		Vision:       true,
		MaxContext:   LookupMaxContext(model),
	}
}

// CreateStream implements Provider.CreateStream using the Responses API.
func (p *ResponsesProvider) CreateStream(ctx context.Context, params Params) (<-chan types.StreamEvent, error) {
	p.mu.RLock()
	baseURL := p.baseURL
	apiKey := p.apiKey
	defaultModel := p.model
	headers := cloneHeaders(p.headers)
	chatGPTCodexBackend := p.chatGPTCodexBackend
	firstPartyEndpoint := p.firstPartyEndpoint
	publicAPIEndpoint := p.publicAPIEndpoint
	disableStrictTools := p.disableStrictTools
	cacheRouting := p.cacheRouting
	cacheUserNamespace := p.cacheUserNamespace
	cacheRoutingShards := p.cacheRoutingShards
	client := p.client
	p.mu.RUnlock()

	systemPrompt := params.JoinedSystemPrompt()
	model := params.Model
	if model == "" {
		model = defaultModel
	}
	responsesLite := firstPartyEndpoint && isOpenAIResponsesLiteModel(model)

	// Only the official public API has a stable HTTP response-chaining contract.
	// Custom Responses-compatible endpoints may return response IDs while only
	// accepting them on a different transport (for example, WebSocket v2).
	// Sending full history is the portable HTTP behavior for those endpoints.
	prevID := ""
	if publicAPIEndpoint {
		prevID = params.PreviousResponseID
	}

	// Build request body
	body := map[string]any{
		"model":  model,
		"stream": true,
	}
	if chatGPTCodexBackend || responsesLite {
		body["store"] = false
		prevID = ""
	}
	if chatGPTCodexBackend && !responsesLite {
		body["tools"] = []map[string]any{}
		body["tool_choice"] = "auto"
		body["parallel_tool_calls"] = false
		body["include"] = []string{}
	}
	// Codex 0.144.x no longer sends the ordinary MaxTokens value through the
	// Responses request. Preserve the local output-limit recovery only for the
	// public API, where max_output_tokens is documented, and only when the loop
	// explicitly escalates the limit.
	if publicAPIEndpoint && !responsesLite && params.MaxOutputTokensOverride > 0 {
		body["max_output_tokens"] = params.MaxOutputTokensOverride
	}

	cacheKey := scopedPromptCacheKey(cacheUserNamespace, params.PromptCacheKey, model, cacheRoutingShards)
	promptCacheEnabled := publicAPIEndpoint && cacheRouting == CacheRoutingPromptCacheKey && params.UsePromptCache && cacheKey != ""
	cachePolicy := openAIPromptCachePolicy{}
	if promptCacheEnabled {
		cachePolicy = applyOpenAIPromptCachePolicy(body, model)
	}

	var cacheableDeveloperInput any
	if systemPrompt != "" && !responsesLite {
		if cachePolicy.Options && prevID == "" {
			if content, ok := openAIStaticSystemContent(params.SystemTextBlocks(), "input_text"); ok {
				cacheableDeveloperInput = map[string]any{
					"type":    "message",
					"role":    "developer",
					"content": content,
				}
			} else {
				body["instructions"] = systemPrompt
			}
		} else {
			body["instructions"] = systemPrompt
		}
	}

	tools := convertToolsToResponsesAPIWithStrictMode(params.Tools, !disableStrictTools)

	// Convert messages to Responses API input format. Responses Lite carries
	// tools and instructions as leading developer input items and sends the full
	// HTTP history, matching current Codex HTTP fallback behavior.
	input := convertMessagesToResponsesAPIForParams(params, prevID)
	if cacheableDeveloperInput != nil {
		input = append([]any{cacheableDeveloperInput}, input...)
	}
	if responsesLite {
		prefix := []any{map[string]any{
			"type":  "additional_tools",
			"role":  "developer",
			"tools": tools,
		}}
		if systemPrompt != "" {
			content := any([]map[string]string{{
				"type": "input_text",
				"text": systemPrompt,
			}})
			if cachePolicy.Options {
				if cacheContent, ok := openAIStaticSystemContent(params.SystemTextBlocks(), "input_text"); ok {
					content = cacheContent
				}
			}
			prefix = append(prefix, map[string]any{
				"type":    "message",
				"role":    "developer",
				"content": content,
			})
		}
		input = append(prefix, input...)
		body["tool_choice"] = "auto"
		body["parallel_tool_calls"] = false
		body["include"] = []string{"reasoning.encrypted_content"}
	}
	if len(input) > 0 {
		body["input"] = input
	}

	// Tools
	if len(tools) > 0 && !responsesLite {
		body["tools"] = tools
		body["parallel_tool_calls"] = true
	}

	// Tool choice
	if params.ToolChoice != nil && !responsesLite {
		switch params.ToolChoice.Type {
		case "any":
			body["tool_choice"] = "required"
		case "tool":
			body["tool_choice"] = map[string]string{
				"type": "function",
				"name": params.ToolChoice.Name,
			}
		default:
			body["tool_choice"] = "auto"
		}
	}

	// Previous response ID for chaining
	if prevID != "" {
		body["previous_response_id"] = prevID
	}

	// Prompt cache key for sticky routing / prompt cache reuse.
	// Align with Codex CLI: keep this enabled independently from previous_response_id,
	// so that when chaining breaks we still preserve prompt-cache affinity.
	if cacheRouting == CacheRoutingPromptCacheKey && params.UsePromptCache && cacheKey != "" {
		body["prompt_cache_key"] = cacheKey
	}

	// Truncation strategy
	if !chatGPTCodexBackend && !responsesLite && params.Truncation != "" {
		body["truncation"] = params.Truncation
	}

	// Reasoning effort
	if params.ReasoningEffort != "" || responsesLite {
		reasoning := map[string]string{}
		if params.ReasoningEffort != "" {
			reasoning["effort"] = reasoningEffortForRequest(params.ReasoningEffort)
		}
		if responsesLite {
			reasoning["context"] = "all_turns"
		}
		body["reasoning"] = reasoning
		if chatGPTCodexBackend {
			body["include"] = []string{"reasoning.encrypted_content"}
		}
	}
	if outputConfig := outputConfigBody(params.TaskBudget); !responsesLite && len(outputConfig) > 0 {
		body["output_config"] = outputConfig
	}

	// Build endpoint URL
	endpoint := baseURL + "/responses"
	p.omitUnsupportedFields(model, body)

	var resp *http.Response
	for {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, i18n.WrapError(i18n.KeyProviderRequestEncodeFailed, err)
		}
		req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
		if err != nil {
			return nil, i18n.WrapError(i18n.KeyProviderRequestBuildFailed, err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		if responsesLite {
			req.Header.Set("x-openai-internal-codex-responses-lite", "true")
		}

		resp, err = client.Do(req)
		if err != nil {
			return nil, i18n.WrapError(i18n.KeyProviderRequestFailed, err, "Responses API")
		}
		if resp.StatusCode == http.StatusOK {
			break
		}

		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		if !firstPartyEndpoint && (resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnprocessableEntity) {
			if field := unsupportedResponsesRequestField(bodyBytes, body); field != "" {
				p.rememberUnsupportedField(model, field)
				delete(body, field)
				continue
			}
		}
		return nil, parseResponsesHTTPError(resp.StatusCode, bodyBytes)
	}

	ch := make(chan types.StreamEvent, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		p.processResponsesStream(ctx, resp.Body, ch)
	}()

	return ch, nil
}

var responsesOptionalGatewayFields = []string{
	"max_output_tokens",
	"prompt_cache_key",
	"truncation",
	"output_config",
	"reasoning",
	"parallel_tool_calls",
}

func unsupportedResponsesFieldKey(model, field string) string {
	return strings.TrimSpace(model) + "\x00" + field
}

func (p *ResponsesProvider) rememberUnsupportedField(model, field string) {
	p.unsupportedFields.Store(unsupportedResponsesFieldKey(model, field), struct{}{})
}

func (p *ResponsesProvider) omitUnsupportedFields(model string, body map[string]any) {
	for _, field := range responsesOptionalGatewayFields {
		if _, rejected := p.unsupportedFields.Load(unsupportedResponsesFieldKey(model, field)); rejected {
			delete(body, field)
		}
	}
}

// unsupportedResponsesRequestField identifies an optional gateway field that a
// custom Responses endpoint explicitly rejected. It deliberately avoids
// retrying value-validation errors, authentication failures, or generic 400s.
func unsupportedResponsesRequestField(responseBody []byte, requestBody map[string]any) string {
	lower := strings.ToLower(string(responseBody))
	unsupported := false
	for _, marker := range []string{
		"unsupported parameter",
		"unsupported field",
		"parameter is not supported",
		"field is not supported",
		"is unsupported",
		"unknown parameter",
		"unknown field",
		"unrecognized parameter",
		"unrecognized field",
		"unexpected parameter",
		"unexpected field",
		"extra inputs are not permitted",
		"extra fields not permitted",
		"parameter is not allowed",
		"field is not allowed",
		"extra_forbidden",
	} {
		if strings.Contains(lower, marker) {
			unsupported = true
			break
		}
	}
	if !unsupported {
		return ""
	}
	for _, field := range responsesOptionalGatewayFields {
		if _, sent := requestBody[field]; sent && strings.Contains(lower, field) {
			return field
		}
	}
	return ""
}

// processResponsesStream reads SSE events from the Responses API and maps them to StreamEvents.
func (p *ResponsesProvider) processResponsesStream(ctx context.Context, body io.Reader, ch chan<- types.StreamEvent) {
	send := func(evt types.StreamEvent) bool {
		select {
		case ch <- evt:
			return true
		case <-ctx.Done():
			return false
		}
	}

	// Track output items by their index for mapping to stream events
	type outputItem struct {
		itemType string // "message", "function_call"
		blockIdx int    // our block index for StreamEvent
		callID   string
		name     string
	}
	outputItems := make(map[int]*outputItem) // keyed by output_index
	nextBlockIdx := 0
	messageStarted := false

	completedNormally := false
	events := parseSSE(body)
	for sse := range events {
		if ctx.Err() != nil {
			return
		}

		// Some proxies (e.g. codex-lb) emit SSE without an "event:" line,
		// putting the event type only in the JSON "type" field. Fall back to
		// extracting it from the data payload when the SSE event type is empty.
		eventType := sse.Type
		if eventType == "" && sse.Data != "" {
			var peek struct {
				Type string `json:"type"`
			}
			if json.Unmarshal([]byte(sse.Data), &peek) == nil && peek.Type != "" {
				eventType = peek.Type
			}
		}

		switch eventType {
		case "response.created", "response.in_progress":
			if !messageStarted {
				if !send(types.StreamEvent{
					Type: types.EventMessageStart,
				}) {
					return
				}
				messageStarted = true
			}

		case "response.output_item.added":
			var data struct {
				OutputIndex int `json:"output_index"`
				Item        struct {
					Type   string `json:"type"`
					ID     string `json:"id"`
					CallID string `json:"call_id"`
					Name   string `json:"name"`
				} `json:"item"`
			}
			if err := json.Unmarshal([]byte(sse.Data), &data); err != nil {
				continue
			}

			item := &outputItem{
				itemType: data.Item.Type,
				blockIdx: nextBlockIdx,
			}

			if data.Item.Type == "function_call" {
				item.callID = data.Item.CallID
				item.name = data.Item.Name
				// Emit content_block_start for tool_use
				if !send(types.StreamEvent{
					Type:  types.EventContentBlockStart,
					Index: nextBlockIdx,
					ContentBlock: &types.ContentDelta{
						Type: types.ContentTypeToolUse,
						ID:   data.Item.CallID,
						Name: data.Item.Name,
					},
				}) {
					return
				}
				nextBlockIdx++
			} else if data.Item.Type == "reasoning" {
				if !send(types.StreamEvent{
					Type:         types.EventContentBlockStart,
					Index:        nextBlockIdx,
					ContentBlock: &types.ContentDelta{Type: types.ContentTypeThinking},
				}) {
					return
				}
				nextBlockIdx++
			}

			outputItems[data.OutputIndex] = item

		case "response.content_part.added":
			var data struct {
				OutputIndex  int `json:"output_index"`
				ContentIndex int `json:"content_index"`
				Part         struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"part"`
			}
			if err := json.Unmarshal([]byte(sse.Data), &data); err != nil {
				continue
			}

			item := outputItems[data.OutputIndex]
			if item == nil {
				item = &outputItem{itemType: "message", blockIdx: nextBlockIdx}
				outputItems[data.OutputIndex] = item
			}

			if data.Part.Type == "output_text" || data.Part.Type == "text" {
				if !send(types.StreamEvent{
					Type:         types.EventContentBlockStart,
					Index:        item.blockIdx,
					ContentBlock: &types.ContentDelta{Type: types.ContentTypeText},
				}) {
					return
				}
				if item.blockIdx >= nextBlockIdx {
					nextBlockIdx = item.blockIdx + 1
				}
			}

		case "response.output_text.delta":
			var data struct {
				OutputIndex  int    `json:"output_index"`
				ContentIndex int    `json:"content_index"`
				Delta        string `json:"delta"`
			}
			if err := json.Unmarshal([]byte(sse.Data), &data); err != nil {
				continue
			}

			item := outputItems[data.OutputIndex]
			idx := 0
			if item != nil {
				idx = item.blockIdx
			}

			if !send(types.StreamEvent{
				Type:  types.EventContentBlockDelta,
				Index: idx,
				Delta: &types.ContentDelta{Type: "text_delta", Text: data.Delta},
			}) {
				return
			}

		case "response.content_part.delta":
			// Generic content part delta — handles text output
			var data struct {
				OutputIndex  int `json:"output_index"`
				ContentIndex int `json:"content_index"`
				Delta        struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(sse.Data), &data); err != nil {
				continue
			}

			item := outputItems[data.OutputIndex]
			idx := 0
			if item != nil {
				idx = item.blockIdx
			}

			if data.Delta.Text != "" {
				if !send(types.StreamEvent{
					Type:  types.EventContentBlockDelta,
					Index: idx,
					Delta: &types.ContentDelta{Type: "text_delta", Text: data.Delta.Text},
				}) {
					return
				}
			}

		case "response.function_call_arguments.delta":
			var data struct {
				OutputIndex int    `json:"output_index"`
				Delta       string `json:"delta"`
			}
			if err := json.Unmarshal([]byte(sse.Data), &data); err != nil {
				continue
			}

			item := outputItems[data.OutputIndex]
			idx := 0
			if item != nil {
				idx = item.blockIdx
			}

			if !send(types.StreamEvent{
				Type:  types.EventContentBlockDelta,
				Index: idx,
				Delta: &types.ContentDelta{
					Type:        "input_json_delta",
					PartialJSON: data.Delta,
				},
			}) {
				return
			}

		case "response.reasoning.delta", "response.reasoning_text.delta", "response.reasoning_summary_text.delta":
			var data struct {
				OutputIndex int    `json:"output_index"`
				Delta       string `json:"delta"`
			}
			if err := json.Unmarshal([]byte(sse.Data), &data); err != nil {
				continue
			}

			item := outputItems[data.OutputIndex]
			idx := 0
			if item != nil {
				idx = item.blockIdx
			}
			if data.Delta != "" {
				if !send(types.StreamEvent{
					Type:  types.EventContentBlockDelta,
					Index: idx,
					Delta: &types.ContentDelta{Type: "thinking_delta", Thinking: data.Delta},
				}) {
					return
				}
			}

		case "response.function_call_arguments.done":
			// No action needed — block_stop will handle finalization

		case "response.output_item.done":
			var data struct {
				OutputIndex int `json:"output_index"`
			}
			if err := json.Unmarshal([]byte(sse.Data), &data); err != nil {
				continue
			}

			item := outputItems[data.OutputIndex]
			if item != nil {
				if !send(types.StreamEvent{
					Type:  types.EventContentBlockStop,
					Index: item.blockIdx,
				}) {
					return
				}
			}

		case "response.content_part.done":
			// Content part finalized within an output item
			var data struct {
				OutputIndex int `json:"output_index"`
			}
			if err := json.Unmarshal([]byte(sse.Data), &data); err != nil {
				continue
			}
			// For message output items, the content_part.done signals text block end
			// But we let output_item.done handle the EventContentBlockStop

		case "response.completed":
			var data struct {
				Response struct {
					ID     string `json:"id"`
					Status string `json:"status"`
					Usage  struct {
						InputTokens        int `json:"input_tokens"`
						OutputTokens       int `json:"output_tokens"`
						InputTokensDetails struct {
							CachedTokens     int `json:"cached_tokens"`
							CacheWriteTokens int `json:"cache_write_tokens"`
						} `json:"input_tokens_details"`
					} `json:"usage"`
					IncompleteDetails struct {
						Reason string `json:"reason"`
					} `json:"incomplete_details"`
					Output []struct {
						Type   string `json:"type"`
						Status string `json:"status"`
					} `json:"output"`
				} `json:"response"`
			}
			if err := json.Unmarshal([]byte(sse.Data), &data); err != nil {
				// response.completed is critical — don't silently swallow parse errors
				send(types.StreamEvent{
					Type: types.EventError,
					Error: &types.APIError{
						Type: "parse_error",
						Message: i18n.Format(
							i18n.DetectOrLoadLanguage(),
							i18n.KeyProviderResponsesCompletedParseFailed,
							err,
						),
					},
				})
				completedNormally = true // don't also emit stream_interrupted
				return
			}

			// Store response ID for next turn's chaining
			responseID := data.Response.ID

			// Determine stop reason from output items
			sr := types.StopReasonEndTurn
			for _, out := range data.Response.Output {
				if out.Type == "function_call" {
					sr = types.StopReasonToolUse
					break
				}
			}
			if data.Response.Status == "incomplete" ||
				data.Response.IncompleteDetails.Reason == "max_output_tokens" ||
				data.Response.IncompleteDetails.Reason == "max_tokens" {
				sr = types.StopReasonMaxTokens
			}

			// Emit usage. OpenAI cached tokens remain a detail of input_tokens.
			usage := normalizeOpenAIUsage(
				data.Response.Usage.InputTokens,
				data.Response.Usage.OutputTokens,
				data.Response.Usage.InputTokensDetails.CachedTokens,
				data.Response.Usage.InputTokensDetails.CacheWriteTokens,
			)

			if !send(types.StreamEvent{
				Type:       types.EventMessageDelta,
				StopReason: &sr,
				Usage:      usage,
			}) {
				return
			}

			completedNormally = true
			send(types.StreamEvent{Type: types.EventMessageStop, ResponseID: responseID})
			return

		case "response.failed":
			var data struct {
				Response struct {
					StatusCode int `json:"status_code"`
					Error      struct {
						Message    string `json:"message"`
						Code       string `json:"code"`
						Status     int    `json:"status"`
						StatusCode int    `json:"status_code"`
					} `json:"error"`
				} `json:"response"`
			}
			if err := json.Unmarshal([]byte(sse.Data), &data); err != nil {
				send(types.StreamEvent{
					Type: types.EventError,
					Error: &types.APIError{
						Type: "parse_error",
						Message: i18n.Format(
							i18n.DetectOrLoadLanguage(),
							i18n.KeyProviderResponsesFailedParseFailed,
							err,
						),
					},
				})
				completedNormally = true
				return
			}
			errMsg := data.Response.Error.Message
			status := data.Response.Error.Status
			if status == 0 {
				status = data.Response.Error.StatusCode
			}
			if status == 0 {
				status = data.Response.StatusCode
			}
			errType, status := classifyResponsesAPIError(data.Response.Error.Code, errMsg, status)

			send(types.StreamEvent{
				Type: types.EventError,
				Error: &types.APIError{
					Type:    errType,
					Message: errMsg,
					Status:  status,
				},
			})
			completedNormally = true // explicit failure, not an interruption
			return

		case "error":
			var data struct {
				Message string `json:"message"`
				Code    string `json:"code"`
			}
			if err := json.Unmarshal([]byte(sse.Data), &data); err != nil {
				continue
			}
			send(types.StreamEvent{
				Type: types.EventError,
				Error: &types.APIError{
					Type:    "stream_error",
					Message: data.Message,
				},
			})
			completedNormally = true // explicit error, not an interruption
			return

		default:
			// Unknown event types (e.g. codex.rate_limits, future API additions)
			// are silently ignored. This is intentional — proxies and API updates
			// may introduce new event types that don't affect core functionality.
		}
	}

	// If we never got response.completed, the stream was interrupted.
	if !completedNormally && messageStarted {
		send(types.StreamEvent{
			Type: types.EventError,
			Error: &types.APIError{
				Type:    "stream_interrupted",
				Message: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeResponsesStreamIncomplete),
			},
		})
		send(types.StreamEvent{Type: types.EventMessageStop})
	}
}

// ── Message conversion for Responses API ────────────────────────────────────

func outputConfigBody(taskBudget *TaskBudget) map[string]any {
	taskBudgetBody := taskBudgetBody(taskBudget)
	if len(taskBudgetBody) == 0 {
		return nil
	}
	return map[string]any{
		"task_budget": taskBudgetBody,
	}
}

func taskBudgetBody(taskBudget *TaskBudget) map[string]any {
	if taskBudget == nil || taskBudget.Total <= 0 {
		return nil
	}
	body := map[string]any{
		"total": taskBudget.Total,
	}
	if taskBudget.Remaining != nil {
		remaining := *taskBudget.Remaining
		if remaining < 0 {
			remaining = 0
		}
		body["remaining"] = remaining
	}
	return body
}

func classifyResponsesAPIError(code, message string, status int) (string, int) {
	errCode := strings.ToLower(code)
	msgLower := strings.ToLower(message)

	if strings.Contains(errCode, "previous_response_not_found") ||
		((strings.Contains(errCode, "previous_response") || strings.Contains(msgLower, "previous_response") || strings.Contains(msgLower, "previous response")) &&
			(strings.Contains(errCode, "not_found") || strings.Contains(msgLower, "not found") || strings.Contains(msgLower, "expired") || strings.Contains(msgLower, "does not exist"))) {
		return "previous_response_not_found", status
	}
	if status == http.StatusTooManyRequests || strings.Contains(errCode, "rate_limit") {
		if status == 0 {
			status = http.StatusTooManyRequests
		}
		return "rate_limit_error", status
	}
	if status >= 500 || errCode == "server_error" || errCode == "internal_error" {
		if status == 0 {
			status = http.StatusInternalServerError
		}
		return "server_error", status
	}
	if errCode == "context_window_full" || errCode == "prompt_too_long" {
		return errCode, status
	}
	return "api_error", status
}

// convertMessagesToResponsesAPIForParams converts messages to Responses input,
// sending only the new turn when the request chains from a previous response.
func convertMessagesToResponsesAPIForParams(params Params, prevResponseID string) []any {
	if prevResponseID != "" {
		return convertNewMessagesForResponsesAPIWithParams(params)
	}
	return convertAllMessagesForResponsesAPIWithParams(params)
}

func convertAllMessagesForResponsesAPIWithParams(params Params) []any {
	var input []any
	for _, msg := range params.Messages {
		role := msg.Role
		if role == types.RoleDeveloper && !params.isTrustedDeveloperMessage(msg) {
			role = types.RoleUser
		}
		switch role {
		case types.RoleUser:
			input = append(input, convertUserMessageToResponsesAPI(msg)...)
		case types.RoleAssistant:
			input = append(input, convertAssistantMessageToResponsesAPI(msg)...)
		case types.RoleDeveloper:
			input = append(input, convertDeveloperMessageToResponsesAPI(msg)...)
		}
	}
	return input
}

func convertNewMessagesForResponsesAPIWithParams(params Params) []any {
	msgs := params.Messages
	// Find the last assistant message
	lastAssistantIdx := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == types.RoleAssistant {
			lastAssistantIdx = i
			break
		}
	}

	// Only send messages after the last assistant message
	startIdx := lastAssistantIdx + 1
	if startIdx >= len(msgs) {
		return nil
	}

	var input []any
	for _, msg := range msgs[startIdx:] {
		role := msg.Role
		if role == types.RoleDeveloper && !params.isTrustedDeveloperMessage(msg) {
			role = types.RoleUser
		}
		switch role {
		case types.RoleUser:
			input = append(input, convertUserMessageToResponsesAPI(msg)...)
		case types.RoleAssistant:
			input = append(input, convertAssistantMessageToResponsesAPI(msg)...)
		case types.RoleDeveloper:
			input = append(input, convertDeveloperMessageToResponsesAPI(msg)...)
		}
	}
	return input
}

// convertDeveloperMessageToResponsesAPI projects internal catalog instructions
// as native Responses developer input items. Persistence-only metadata stays on
// the local Message and is deliberately excluded from the provider wire shape.
func convertDeveloperMessageToResponsesAPI(msg types.Message) []any {
	text := msg.GetText()
	if text == "" {
		return nil
	}
	return []any{map[string]any{
		"role":    "developer",
		"content": text,
	}}
}

// convertUserMessageToResponsesAPI converts a user message.
// Tool results become function_call_output items; text becomes a user message.
// Image blocks are converted to input_image content parts with data URIs.
func convertUserMessageToResponsesAPI(msg types.Message) []any {
	var functionOutputs []any
	var followUps []any
	var textParts []string
	var imageParts []types.ImageBlock
	flushUserParts := func() {
		if len(textParts) == 0 && len(imageParts) == 0 {
			return
		}
		followUps = append(followUps, buildUserItem(textParts, imageParts))
		textParts = nil
		imageParts = nil
	}

	for _, block := range msg.Content {
		switch b := block.(type) {
		case types.ToolResultBlock:
			flushUserParts()
			functionOutputs = append(functionOutputs, map[string]any{
				"type":    "function_call_output",
				"call_id": b.ToolUseID,
				"output":  b.TextContent(),
			})
			if b.HasStructuredContent() {
				for _, contentBlock := range b.ContentBlocks {
					switch typed := contentBlock.(type) {
					case types.TextBlock:
						if typed.Text != "" {
							textParts = append(textParts, typed.Text)
						}
					case types.ImageBlock:
						imageParts = append(imageParts, typed)
					case types.DocumentBlock:
						textParts = append(textParts, "[document]")
					case types.ToolReferenceBlock:
						if typed.ToolName != "" {
							textParts = append(textParts, "[tool:"+typed.ToolName+"]")
						}
					}
				}
				flushUserParts()
			}
		case types.TextBlock:
			textParts = append(textParts, b.Text)
		case types.ImageBlock:
			imageParts = append(imageParts, b)
		case types.DocumentBlock:
			textParts = append(textParts, "[document]")
		}
	}

	flushUserParts()

	// If no content was extracted, create a minimal user message
	if len(functionOutputs) == 0 && len(followUps) == 0 {
		text := msg.GetText()
		if text != "" {
			followUps = append(followUps, map[string]any{
				"role":    "user",
				"content": text,
			})
		}
	}

	// Keep all sibling function outputs contiguous before supplemental user
	// content, matching the Chat Completions tool-call ordering contract.
	return append(functionOutputs, followUps...)
}

// buildUserItem constructs a Responses API user input item from text and image parts.
// When images are present, content is a multipart array with input_text and input_image entries.
// When only text is present, content is a plain string.
func buildUserItem(textParts []string, imageParts []types.ImageBlock) map[string]any {
	if len(imageParts) == 0 {
		// Text-only: use simple string content
		return map[string]any{
			"role":    "user",
			"content": strings.Join(textParts, "\n"),
		}
	}

	// Multipart content: input_text + input_image items
	var content []map[string]string
	if len(textParts) > 0 {
		content = append(content, map[string]string{
			"type": "input_text",
			"text": strings.Join(textParts, "\n"),
		})
	}
	for _, img := range imageParts {
		mediaType := "image/png"
		data := ""
		if img.Source != nil {
			if img.Source.MediaType != "" {
				mediaType = img.Source.MediaType
			}
			data = img.Source.Data
		}
		// Responses API requires data URI format: data:<mime>;base64,<data>
		dataURI := fmt.Sprintf("data:%s;base64,%s", mediaType, data)
		content = append(content, map[string]string{
			"type":      "input_image",
			"image_url": dataURI,
		})
	}

	return map[string]any{
		"role":    "user",
		"content": content,
	}
}

// convertAssistantMessageToResponsesAPI converts an assistant message to input items.
func convertAssistantMessageToResponsesAPI(msg types.Message) []any {
	var items []any

	text := msg.GetText()
	if text != "" {
		items = append(items, map[string]any{
			"role":    "assistant",
			"content": text,
		})
	}

	// Include tool uses as function_call items
	for _, tu := range msg.GetToolUses() {
		args, err := json.Marshal(tu.Input)
		if err != nil {
			continue
		}
		items = append(items, map[string]any{
			"type":      "function_call",
			"call_id":   tu.ID,
			"name":      tu.Name,
			"arguments": string(args),
		})
	}

	return items
}

// ── Tool conversion for Responses API ───────────────────────────────────────

func convertToolsToResponsesAPIWithStrictMode(tools []types.ToolDefinition, strictMode bool) []map[string]any {
	result := make([]map[string]any, 0, len(tools))
	for _, t := range canonicalToolDefinitions(tools) {
		schema := t.InputSchema
		if schema.Properties == nil {
			schema.Properties = map[string]any{}
		}
		tool := map[string]any{
			"type":        "function",
			"name":        t.Name,
			"description": t.Description,
			"parameters":  schema,
		}
		if strictMode && t.Strict {
			tool["strict"] = true
		}
		result = append(result, tool)
	}
	return result
}

// ── Error handling ──────────────────────────────────────────────────────────

// parseResponsesHTTPError converts an HTTP error from the Responses API into an *types.APIError.
func parseResponsesHTTPError(status int, body []byte) *types.APIError {
	errType := "api_error"
	switch status {
	case 429:
		errType = "rate_limit_error"
	case 529, 503:
		errType = "overloaded_error"
	}

	message := string(body)
	// Try to extract message from JSON error response
	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    any    `json:"code"`
			Param   string `json:"param"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
		message = errResp.Error.Message
		if errResp.Error.Type != "" {
			errType = errResp.Error.Type
		}
		code := strings.ToLower(fmt.Sprint(errResp.Error.Code))
		param := strings.ToLower(errResp.Error.Param)
		msg := strings.ToLower(message)
		if (strings.Contains(param, "previous_response") || strings.Contains(msg, "previous response") || strings.Contains(msg, "previous_response")) &&
			(strings.Contains(msg, "not found") || strings.Contains(msg, "expired") || strings.Contains(msg, "does not exist") || strings.Contains(code, "not_found")) {
			errType = "previous_response_not_found"
		}
	}

	return &types.APIError{
		Status:  status,
		Type:    errType,
		Message: message,
	}
}
