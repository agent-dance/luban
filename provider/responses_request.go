package provider

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

type responsesTransport uint8

const (
	responsesTransportHTTP responsesTransport = iota
	responsesTransportWebSocket
)

func normalizeCapabilitySupport(value CapabilitySupport) CapabilitySupport {
	switch value {
	case CapabilityUnsupported, CapabilitySupported:
		return value
	default:
		return CapabilityUnknown
	}
}

// responsesRequestProfile is an immutable per-call snapshot. Credential
// refresh may mutate the provider while a response is streaming; one request
// must never mix endpoints, authentication generations, or wire semantics.
type responsesRequestProfile struct {
	providerName              string
	baseURL                   string
	apiKey                    string
	defaultModel              string
	maxTokens                 int
	headers                   map[string]string
	semantics                 ResponsesSemantics
	chatGPTCodexBackend       bool
	firstPartyEndpoint        bool
	publicAPIEndpoint         bool
	disableStrictTools        bool
	disablePromptCacheOptions bool
	cacheRouting              CacheRoutingMode
	cacheUserNamespace        string
	cacheRoutingShards        int
	webSocketCapability       CapabilitySupport
	credentialEpoch           uint64
	timeoutClient             *http.Client
}

func (p *ResponsesProvider) snapshotRequestProfile() responsesRequestProfile {
	p.mu.RLock()
	defer p.mu.RUnlock()
	profile := responsesRequestProfile{
		providerName:              p.name,
		baseURL:                   p.baseURL,
		apiKey:                    p.apiKey,
		defaultModel:              p.model,
		maxTokens:                 p.maxTokens,
		headers:                   cloneHeaders(p.headers),
		semantics:                 p.semantics,
		chatGPTCodexBackend:       p.chatGPTCodexBackend,
		firstPartyEndpoint:        p.firstPartyEndpoint,
		publicAPIEndpoint:         p.publicAPIEndpoint,
		disableStrictTools:        p.disableStrictTools,
		disablePromptCacheOptions: p.disablePromptCacheOptions,
		cacheRouting:              p.cacheRouting,
		cacheUserNamespace:        p.cacheUserNamespace,
		cacheRoutingShards:        p.cacheRoutingShards,
		webSocketCapability:       p.responsesWebSocket,
		credentialEpoch:           p.wsCredentialEpoch,
		timeoutClient:             p.client,
	}
	if p.forceHTTPFallback {
		profile.webSocketCapability = CapabilityUnsupported
	}
	return profile
}

func (profile responsesRequestProfile) modelFor(params Params) string {
	model := strings.TrimSpace(params.Model)
	if model == "" {
		model = profile.defaultModel
	}
	return model
}

func (profile responsesRequestProfile) webSocketEligible(params Params) bool {
	model := profile.modelFor(params)
	responsesLite := profile.chatGPTCodexBackend && isOpenAIResponsesLiteModel(model)
	return profile.webSocketCapability == CapabilitySupported &&
		profile.semantics == ResponsesSemanticsOpenAIPublic &&
		profile.publicAPIEndpoint &&
		!profile.chatGPTCodexBackend &&
		!responsesLite &&
		strings.TrimSpace(params.ContinuationLineage) != ""
}

// buildResponsesRequestBody is shared by HTTP/SSE and WebSocket transports.
// The input conversion is selected solely by prevID: empty replays the full
// committed history; non-empty emits only items after the last assistant turn.
func (p *ResponsesProvider) buildResponsesRequestBody(
	params Params,
	profile responsesRequestProfile,
	prevID string,
	transport responsesTransport,
) (map[string]any, string, bool, error) {
	systemPrompt := params.JoinedSystemPrompt()
	model := profile.modelFor(params)
	responsesLite := profile.chatGPTCodexBackend && isOpenAIResponsesLiteModel(model)
	if definitionsHaveCustomTools(params.Tools) && (!responsesCustomToolDefinitionsSupported(profile.semantics, model, params.Tools) || responsesLite) {
		return nil, "", false, unsupportedCustomToolsError(p, params)
	}

	body := map[string]any{"model": model}
	if profile.semantics != ResponsesSemanticsDeepSeek {
		body["store"] = false
	}
	if profile.semantics != ResponsesSemanticsDeepSeek && params.ServiceTier != "" {
		body["service_tier"] = string(params.ServiceTier)
	}
	if transport == responsesTransportWebSocket {
		body["type"] = "response.create"
	} else {
		body["stream"] = true
	}
	if profile.firstPartyEndpoint {
		body["include"] = []string{"reasoning.encrypted_content"}
	}
	// Match the Codex coding-client envelope. Low controls user-visible prose,
	// not reasoning effort, and materially reduces output tokens without
	// weakening the model's requested analysis tier.
	if (profile.publicAPIEndpoint || profile.semantics == ResponsesSemanticsDeepSeek) && !responsesLite {
		body["text"] = map[string]string{"verbosity": "low"}
	}
	if profile.chatGPTCodexBackend || responsesLite {
		prevID = ""
	}
	if profile.chatGPTCodexBackend && !responsesLite {
		body["tools"] = []map[string]any{}
		body["tool_choice"] = "auto"
		body["parallel_tool_calls"] = false
		body["include"] = []string{"reasoning.encrypted_content"}
	}
	if profile.publicAPIEndpoint && !responsesLite && params.MaxOutputTokensOverride > 0 {
		body["max_output_tokens"] = params.MaxOutputTokensOverride
	} else if profile.semantics == ResponsesSemanticsDeepSeek && !responsesLite {
		maxOutputTokens := params.MaxTokens
		if maxOutputTokens <= 0 {
			maxOutputTokens = profile.maxTokens
		}
		if params.MaxOutputTokensOverride > 0 {
			maxOutputTokens = params.MaxOutputTokensOverride
		}
		if maxOutputTokens > 0 {
			body["max_output_tokens"] = maxOutputTokens
		}
	}

	cacheKey := scopedPromptCacheKey(profile.cacheUserNamespace, params.PromptCacheKey, model, profile.cacheRoutingShards)
	promptCacheEnabled := profile.publicAPIEndpoint && !profile.disablePromptCacheOptions && profile.cacheRouting == CacheRoutingPromptCacheKey && params.UsePromptCache && cacheKey != ""
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

	tools := convertToolsToResponsesAPIForSemantics(params.Tools, !profile.disableStrictTools, profile.semantics)
	conversionParams := params
	conversionParams.Model = model
	input, err := convertMessagesToResponsesAPIForRequest(conversionParams, prevID, profile.semantics, responsesLite)
	if err != nil {
		return nil, "", false, err
	}
	if transport == responsesTransportHTTP && profile.semantics == ResponsesSemanticsOpenAIPublic {
		input, err = projectResponsesHTTPToolCalls(input)
		if err != nil {
			return nil, "", false, err
		}
	}
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

	if len(tools) > 0 && !responsesLite {
		body["tools"] = tools
		body["parallel_tool_calls"] = true
	}
	if params.ToolChoice != nil && !responsesLite {
		switch params.ToolChoice.Type {
		case "any":
			body["tool_choice"] = "required"
		case "tool":
			custom := responseToolChoiceType(params.Tools, params.ToolChoice.Name) == "custom"
			body["tool_choice"] = map[string]string{
				"type": responseToolChoiceType(params.Tools, params.ToolChoice.Name),
				"name": responsesToolNameForSemantics(profile.semantics, params.ToolChoice.Name, custom),
			}
		default:
			body["tool_choice"] = "auto"
		}
	}
	if profile.semantics != ResponsesSemanticsDeepSeek && prevID != "" {
		body["previous_response_id"] = prevID
	}
	if profile.cacheRouting == CacheRoutingPromptCacheKey && params.UsePromptCache && cacheKey != "" {
		body["prompt_cache_key"] = cacheKey
	}
	if profile.cacheRouting == CacheRoutingDeepSeekUserID && params.UsePromptCache {
		user := profile.cacheUserNamespace
		if user == "" {
			user = cacheKey
		}
		if user = deepSeekCacheUserID(user); user != "" {
			body["user"] = user
		}
	}
	if profile.semantics != ResponsesSemanticsDeepSeek && !profile.chatGPTCodexBackend && !responsesLite && params.Truncation != "" {
		body["truncation"] = params.Truncation
	}
	reasoningEffort := params.ReasoningEffort
	if profile.semantics == ResponsesSemanticsDeepSeek && params.Thinking != nil && !params.Thinking.Enabled {
		reasoningEffort = "none"
	}
	if reasoningEffort != "" || responsesLite {
		reasoning := map[string]string{}
		if reasoningEffort != "" {
			reasoning["effort"] = reasoningEffortForRequest(reasoningEffort)
		}
		if profile.firstPartyEndpoint {
			reasoning["context"] = "all_turns"
		}
		body["reasoning"] = reasoning
	}
	if outputConfig := outputConfigBody(params.TaskBudget); profile.semantics != ResponsesSemanticsDeepSeek && !responsesLite && len(outputConfig) > 0 {
		body["output_config"] = outputConfig
	}

	p.omitUnsupportedFields(model, body)
	return body, model, responsesLite, nil
}

// projectResponsesHTTPToolCalls strips provider item identity from completed
// calls when their outputs are replayed on stateless HTTP. Encrypted reasoning
// remains provider-native, while calls use the semantic full-history shape
// accepted with store=false. WebSocket continuation instead sends only new
// items behind previous_response_id.
func projectResponsesHTTPToolCalls(input []any) ([]any, error) {
	outputCallIDs := make(map[string]struct{})
	for _, raw := range input {
		item, ok := responsesInputItemMap(raw)
		if !ok {
			continue
		}
		switch item["type"] {
		case "function_call_output", "custom_tool_call_output":
			if callID, _ := item["call_id"].(string); callID != "" {
				outputCallIDs[callID] = struct{}{}
			}
		}
	}
	if len(outputCallIDs) == 0 {
		return input, nil
	}

	projected := append([]any(nil), input...)
	projectedCalls := make(map[string]struct{}, len(outputCallIDs))
	for index, raw := range projected {
		item, ok := responsesInputItemMap(raw)
		if !ok {
			continue
		}
		switch item["type"] {
		case "function_call", "custom_tool_call":
			callID, _ := item["call_id"].(string)
			if _, matched := outputCallIDs[callID]; !matched {
				continue
			}
			semantic := map[string]any{
				"type": item["type"], "call_id": callID,
				"name": item["name"],
			}
			if item["type"] == "custom_tool_call" {
				semantic["input"] = item["input"]
			} else {
				semantic["arguments"] = item["arguments"]
			}
			projected[index] = semantic
			projectedCalls[callID] = struct{}{}
		}
	}
	for callID := range outputCallIDs {
		if _, ok := projectedCalls[callID]; !ok {
			return nil, i18n.NewError(i18n.KeyProviderResponsesContinuationInvalid)
		}
	}
	return projected, nil
}

func responsesInputItemMap(raw any) (map[string]any, bool) {
	if item, ok := raw.(map[string]any); ok {
		return item, true
	}
	encoded, ok := raw.(json.RawMessage)
	if !ok {
		return nil, false
	}
	var item map[string]any
	if len(encoded) == 0 || json.Unmarshal(encoded, &item) != nil {
		return nil, false
	}
	return item, true
}
