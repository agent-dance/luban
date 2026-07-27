package provider

import (
	"net/http"
	"strings"
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
	baseURL             string
	apiKey              string
	defaultModel        string
	headers             map[string]string
	semantics           ResponsesSemantics
	chatGPTCodexBackend bool
	firstPartyEndpoint  bool
	publicAPIEndpoint   bool
	disableStrictTools  bool
	cacheRouting        CacheRoutingMode
	cacheUserNamespace  string
	cacheRoutingShards  int
	webSocketCapability CapabilitySupport
	credentialEpoch     uint64
	timeoutClient       *http.Client
}

func (p *ResponsesProvider) snapshotRequestProfile() responsesRequestProfile {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return responsesRequestProfile{
		baseURL:             p.baseURL,
		apiKey:              p.apiKey,
		defaultModel:        p.model,
		headers:             cloneHeaders(p.headers),
		semantics:           p.semantics,
		chatGPTCodexBackend: p.chatGPTCodexBackend,
		firstPartyEndpoint:  p.firstPartyEndpoint,
		publicAPIEndpoint:   p.publicAPIEndpoint,
		disableStrictTools:  p.disableStrictTools,
		cacheRouting:        p.cacheRouting,
		cacheUserNamespace:  p.cacheUserNamespace,
		cacheRoutingShards:  p.cacheRoutingShards,
		webSocketCapability: p.responsesWebSocket,
		credentialEpoch:     p.wsCredentialEpoch,
		timeoutClient:       p.client,
	}
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
	if definitionsHaveCustomTools(params.Tools) && (!supportsOpenAIResponsesCustomTools(profile.semantics, model) || responsesLite) {
		return nil, "", false, unsupportedCustomToolsError(p, params)
	}

	body := map[string]any{
		"model": model,
		"store": false,
	}
	if params.ServiceTier != "" {
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
	}

	cacheKey := scopedPromptCacheKey(profile.cacheUserNamespace, params.PromptCacheKey, model, profile.cacheRoutingShards)
	promptCacheEnabled := profile.publicAPIEndpoint && profile.cacheRouting == CacheRoutingPromptCacheKey && params.UsePromptCache && cacheKey != ""
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
			body["tool_choice"] = map[string]string{
				"type": responseToolChoiceType(params.Tools, params.ToolChoice.Name),
				"name": params.ToolChoice.Name,
			}
		default:
			body["tool_choice"] = "auto"
		}
	}
	if prevID != "" {
		body["previous_response_id"] = prevID
	}
	if profile.cacheRouting == CacheRoutingPromptCacheKey && params.UsePromptCache && cacheKey != "" {
		body["prompt_cache_key"] = cacheKey
	}
	if !profile.chatGPTCodexBackend && !responsesLite && params.Truncation != "" {
		body["truncation"] = params.Truncation
	}
	if params.ReasoningEffort != "" || responsesLite {
		reasoning := map[string]string{}
		if params.ReasoningEffort != "" {
			reasoning["effort"] = reasoningEffortForRequest(params.ReasoningEffort)
		}
		if profile.firstPartyEndpoint {
			reasoning["context"] = "all_turns"
		}
		body["reasoning"] = reasoning
	}
	if outputConfig := outputConfigBody(params.TaskBudget); !responsesLite && len(outputConfig) > 0 {
		body["output_config"] = outputConfig
	}

	p.omitUnsupportedFields(model, body)
	return body, model, responsesLite, nil
}
