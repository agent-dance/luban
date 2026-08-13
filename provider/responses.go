package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

const (
	// Tool arguments are model output, not trusted request data. Keep a broad
	// fallback for uncommon tools while terminating the two compact agentic-v2
	// control envelopes before a degenerate generation can stream megabytes of
	// padding or repeated glob fragments.
	maxResponsesToolInputBytes        = 256 << 10
	maxResponsesInspectToolInputBytes = 32 << 10
	maxResponsesRunToolInputBytes     = 64 << 10
	maxResponsesApplyPatchInputBytes  = 1 << 20
)

// ResponsesProvider implements Provider for the OpenAI Responses API (/v1/responses).
// It is stateless. Every request disables provider storage and replays the
// complete committed input history. OpenAI reasoning items are round-tripped
// as opaque encrypted items; response IDs remain observable evidence but are
// never treated as retrievable state while store=false.
type ResponsesProvider struct {
	mu                        sync.RWMutex
	name                      string
	baseURL                   string
	apiKey                    string
	model                     string
	maxTokens                 int
	timeout                   time.Duration
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
	responsesWebSocket        CapabilitySupport
	forceHTTPFallback         bool
	unsupportedFields         sync.Map
	client                    *http.Client
	wsMu                      sync.Mutex
	wsSessions                map[string]*responsesWebSocketSession
	wsCredentialEpoch         uint64
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
	watchdogConfig := responsesStreamWatchdogConfig()
	if timeout <= 0 || timeout > maxResponsesInitialIdleTimeout {
		timeout = watchdogConfig.initialIdle
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
	semantics := resolveResponsesSemantics(cfg)

	return &ResponsesProvider{
		name:                      providerName,
		baseURL:                   baseURL,
		apiKey:                    bearerToken,
		model:                     model,
		maxTokens:                 maxTokens,
		timeout:                   timeout,
		headers:                   cloneHeaders(cfg.Headers),
		semantics:                 semantics,
		chatGPTCodexBackend:       semantics == ResponsesSemanticsOpenAICodex,
		firstPartyEndpoint:        semantics == ResponsesSemanticsOpenAIPublic || semantics == ResponsesSemanticsOpenAICodex,
		publicAPIEndpoint:         semantics == ResponsesSemanticsOpenAIPublic,
		disableStrictTools:        cfg.DisableStrictTools,
		disablePromptCacheOptions: cfg.DisablePromptCacheOptions,
		cacheRouting:              cacheRoutingModeForResponses(cfg.CacheRoutingPreference, semantics),
		cacheUserNamespace:        cacheUserNamespace,
		cacheRoutingShards:        promptCacheRoutingShardCount(cfg.ProviderName),
		responsesWebSocket:        normalizeCapabilitySupport(cfg.ResponsesWebSocket),
		wsCredentialEpoch:         1,
		// A request-wide Client.Timeout killed healthy long-running xhigh
		// reasoning at 600 seconds. Bound connection/header silence here; the
		// response body has its own semantic-progress watchdog below.
		client: newResponsesHTTPClient(timeout),
	}
}

func resolveResponsesSemantics(cfg Config) ResponsesSemantics {
	switch cfg.ResponsesSemantics {
	case ResponsesSemanticsOpenAIPublic, ResponsesSemanticsOpenAICodex, ResponsesSemanticsDeepSeek, ResponsesSemanticsCompatible:
		return cfg.ResponsesSemantics
	}
	if strings.TrimSpace(cfg.AuthToken) != "" {
		return ResponsesSemanticsOpenAICodex
	}
	if providerName := CanonicalProviderName(cfg.ProviderName); providerName != "" {
		if providerName == "openai" {
			return ResponsesSemanticsOpenAIPublic
		}
		if providerName == "deepseek" {
			return ResponsesSemanticsDeepSeek
		}
		return ResponsesSemanticsCompatible
	}
	// An unidentified direct constructor gets the conservative, generic
	// Responses contract. Built-in providers always set their identity; endpoint
	// location never strengthens or weakens the selected wire contract.
	return ResponsesSemanticsCompatible
}

func cacheRoutingModeForResponses(preference CacheRoutingPreference, semantics ResponsesSemantics) CacheRoutingMode {
	if preference == CacheRoutingOff {
		return CacheRoutingNone
	}
	if semantics == ResponsesSemanticsDeepSeek {
		return CacheRoutingDeepSeekUserID
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
	p.baseURL = baseURL
	p.apiKey = bearerToken
	p.headers = cloneHeaders(cfg.Headers)
	p.semantics = resolveResponsesSemantics(cfg)
	p.chatGPTCodexBackend = p.semantics == ResponsesSemanticsOpenAICodex
	p.firstPartyEndpoint = p.semantics == ResponsesSemanticsOpenAIPublic || p.semantics == ResponsesSemanticsOpenAICodex
	p.publicAPIEndpoint = p.semantics == ResponsesSemanticsOpenAIPublic
	p.disableStrictTools = cfg.DisableStrictTools
	p.disablePromptCacheOptions = cfg.DisablePromptCacheOptions
	p.cacheRouting = cacheRoutingModeForResponses(cfg.CacheRoutingPreference, p.semantics)
	p.cacheUserNamespace = promptCacheUserNamespace(cfg)
	p.cacheRoutingShards = promptCacheRoutingShardCount(cfg.ProviderName)
	p.responsesWebSocket = normalizeCapabilitySupport(cfg.ResponsesWebSocket)
	p.wsCredentialEpoch++
	if p.wsCredentialEpoch == 0 {
		p.wsCredentialEpoch = 1
	}
	p.mu.Unlock()

	// A refreshed bearer token, endpoint, header set, or semantics profile must
	// never inherit connection-local state created under the prior authority.
	p.resetResponsesWebSocketSessions()
}

// APIFormat returns the API protocol used by this provider.
func (p *ResponsesProvider) APIFormat() string { return "responses" }

// Capabilities implements CapabilityProvider for ResponsesProvider.
func (p *ResponsesProvider) Capabilities() ProviderCapabilities {
	p.mu.RLock()
	model := p.model
	semantics := p.semantics
	responsesWebSocket := p.responsesWebSocket
	forceHTTPFallback := p.forceHTTPFallback
	cacheRouting := p.cacheRouting
	p.mu.RUnlock()
	customTools := CapabilityUnsupported
	if supportsOpenAIResponsesCustomTools(semantics, model) {
		customTools = CapabilitySupported
	}
	if semantics != ResponsesSemanticsOpenAIPublic || forceHTTPFallback {
		responsesWebSocket = CapabilityUnsupported
	}
	return ProviderCapabilities{
		Thinking:           semantics == ResponsesSemanticsDeepSeek,
		ToolUse:            true,
		CustomTools:        customTools,
		ResponsesWebSocket: responsesWebSocket,
		ServiceTier:        serviceTierCapabilityForResponses(semantics),
		CacheControl:       false,
		CacheRouting:       cacheRouting,
		SystemParts:        true,
		Vision:             semantics != ResponsesSemanticsDeepSeek,
		MaxContext:         LookupMaxContext(model),
	}
}

// TryFallbackTransport permanently disables Responses WebSocket transport for
// this provider instance and clears its connection-local continuation state.
// A fresh HTTP request will replay the complete committed local history.
func (p *ResponsesProvider) TryFallbackTransport() (from, to string, activated bool) {
	p.mu.Lock()
	if p.forceHTTPFallback || p.responsesWebSocket != CapabilitySupported ||
		p.semantics != ResponsesSemanticsOpenAIPublic || !p.publicAPIEndpoint {
		p.mu.Unlock()
		return "", "", false
	}
	p.forceHTTPFallback = true
	p.mu.Unlock()
	p.resetResponsesWebSocketSessions()
	return "WebSocket", "HTTPS", true
}

func serviceTierCapabilityForResponses(semantics ResponsesSemantics) CapabilitySupport {
	if semantics == ResponsesSemanticsOpenAIPublic {
		return CapabilitySupported
	}
	return CapabilityUnsupported
}

// CreateStream selects WebSocket mode only for an explicitly verified public
// Responses endpoint and a loop-private continuation lineage. Every other
// profile remains on the stable HTTP/SSE transport.
func (p *ResponsesProvider) CreateStream(ctx context.Context, params Params) (<-chan types.StreamEvent, error) {
	if err := ValidateParams(p, params); err != nil {
		return nil, err
	}
	profile := p.snapshotRequestProfile()
	if profile.webSocketEligible(params) {
		stream, safeHTTPFallback, err := p.createResponsesWebSocketStream(ctx, params, profile)
		if err == nil {
			return stream, nil
		}
		if !safeHTTPFallback {
			return nil, err
		}
		if attemptErr := beginNestedTransportAttempt(ctx, err); attemptErr != nil {
			return nil, attemptErr
		}
	}
	return p.createResponsesHTTPStream(ctx, params, profile)
}

func (p *ResponsesProvider) createResponsesHTTPStream(ctx context.Context, params Params, profile responsesRequestProfile) (<-chan types.StreamEvent, error) {
	if err := ValidateParams(p, params); err != nil {
		return nil, err
	}
	body, model, responsesLite, err := p.buildResponsesRequestBody(params, profile, "", responsesTransportHTTP)
	if err != nil {
		return nil, err
	}
	endpoint := profile.baseURL + "/responses"
	var resp *http.Response
	for {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			diagnostic := baseResponsesFailureDiagnostic(ctx, profile.providerName, model, "https", endpoint, "")
			diagnostic.FailurePoint = types.ProviderFailureRequestEncode
			cause := &types.APIError{
				Type: "request_encode_error", Message: i18n.WrapError(i18n.KeyProviderRequestEncodeFailed, err).Error(),
				Stage: types.ProviderErrorStageConnect, Class: types.ProviderErrorClassPermanent,
				ReplaySafety: types.ProviderReplaySafe,
			}
			return nil, applyFailureDiagnosticToAPIError(cause, diagnostic)
		}
		req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
		if err != nil {
			diagnostic := baseResponsesFailureDiagnostic(ctx, profile.providerName, model, "https", endpoint, "")
			diagnostic.FailurePoint = types.ProviderFailureRequestBuild
			cause := &types.APIError{
				Type: "request_build_error", Message: i18n.WrapError(i18n.KeyProviderRequestBuildFailed, err).Error(),
				Stage: types.ProviderErrorStageConnect, Class: types.ProviderErrorClassPermanent,
				ReplaySafety: types.ProviderReplaySafe,
			}
			return nil, applyFailureDiagnosticToAPIError(cause, diagnostic)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		if profile.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+profile.apiKey)
		}
		for k, v := range profile.headers {
			req.Header.Set(k, v)
		}
		if params.ContinuationLineage != "" {
			req.Header.Set(responsesContinuationLineageHeader, params.ContinuationLineage)
			req.Header.Set(responsesContinuationEpochHeader, fmt.Sprintf("%d", params.ContinuationEpoch))
			if params.ContinuationReset {
				req.Header.Set(responsesContinuationResetHeader, "1")
			}
		}
		if responsesLite {
			req.Header.Set("x-openai-internal-codex-responses-lite", "true")
		}

		resp, err = profile.timeoutClient.Do(req)
		if err != nil {
			wrapped := i18n.WrapError(i18n.KeyProviderRequestFailed, err, "Responses API")
			if _, transportFailure := typedTransportAPIError(err, types.ProviderErrorStageConnect); !transportFailure {
				return nil, wrapped
			}
			cause := &types.APIError{
				Type: "transport_error", Message: wrapped.Error(),
				Stage: types.ProviderErrorStageConnect, Class: types.ProviderErrorClassTransport,
				ReplaySafety: types.ProviderReplaySafe,
			}
			diagnostic := baseResponsesFailureDiagnostic(ctx, profile.providerName, model, "https", endpoint, "")
			diagnostic.FailurePoint = types.ProviderFailureRequestTransport
			return nil, applyFailureDiagnosticToAPIError(cause, diagnostic)
		}
		if resp.StatusCode == http.StatusOK {
			break
		}

		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		if !profile.firstPartyEndpoint && (resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnprocessableEntity) {
			if field := unsupportedResponsesRequestField(bodyBytes, body); field != "" {
				p.rememberUnsupportedField(model, field)
				delete(body, field)
				cause := annotateProviderRequestError(
					parseResponsesHTTPError(resp.StatusCode, bodyBytes, resp.Header.Get("Retry-After")),
					profile.providerName, "responses", endpoint, resp.Header,
				)
				diagnostic := baseResponsesFailureDiagnostic(
					ctx, profile.providerName, model, "https", endpoint, providerRequestID(resp.Header),
				)
				diagnostic.FailurePoint = types.ProviderFailureRequestHTTPStatus
				diagnostic.HTTPStatus = resp.StatusCode
				diagnostic.Decision = "fallback"
				diagnostic.DroppedField = field
				applyFailureDiagnosticToAPIError(cause, diagnostic)
				if attemptErr := beginNestedTransportAttempt(ctx, cause); attemptErr != nil {
					return nil, attemptErr
				}
				continue
			}
		}
		apiErr := annotateProviderRequestError(
			parseResponsesHTTPError(resp.StatusCode, bodyBytes, resp.Header.Get("Retry-After")),
			profile.providerName, "responses", endpoint, resp.Header,
		)
		if profile.semantics == ResponsesSemanticsCompatible && responsesEndpointUnavailable(apiErr) {
			apiErr.SuggestedAPIFormat = "chat-completions"
		}
		diagnostic := baseResponsesFailureDiagnostic(
			ctx, profile.providerName, model, "https", endpoint, providerRequestID(resp.Header),
		)
		diagnostic.FailurePoint = types.ProviderFailureRequestHTTPStatus
		diagnostic.HTTPStatus = resp.StatusCode
		applyFailureDiagnosticToAPIError(apiErr, diagnostic)
		return nil, apiErr
	}

	watchdogBody := newStreamWatchdogBody(resp.Body, responsesStreamWatchdogConfig())
	streamCtx := withProviderFailureDiagnostic(ctx, baseResponsesFailureDiagnostic(
		ctx, profile.providerName, model, "https", endpoint, providerRequestID(resp.Header),
	))
	ch := make(chan types.StreamEvent, 64)
	go func() {
		defer close(ch)
		defer watchdogBody.Close()
		processResponsesStreamForRequest(streamCtx, watchdogBody, ch, model, profile.semantics, responsesLite, params.ServiceTier)
	}()

	return ch, nil
}

func newResponsesHTTPClient(responseHeaderTimeout time.Duration) *http.Client {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Client{}
	}
	cloned := transport.Clone()
	cloned.ResponseHeaderTimeout = responseHeaderTimeout
	return &http.Client{Transport: cloned}
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

func (p *ResponsesProvider) UnsupportedRequestFields(model string) []string {
	fields := make([]string, 0, len(responsesOptionalGatewayFields))
	for _, field := range responsesOptionalGatewayFields {
		if _, rejected := p.unsupportedFields.Load(unsupportedResponsesFieldKey(model, field)); rejected {
			fields = append(fields, field)
		}
	}
	return fields
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
func (p *ResponsesProvider) processResponsesStream(ctx context.Context, body io.Reader, ch chan<- types.StreamEvent, requestModels ...string) {
	p.mu.RLock()
	requestModel := p.model
	semantics := p.semantics
	p.mu.RUnlock()
	if len(requestModels) > 0 && strings.TrimSpace(requestModels[0]) != "" {
		requestModel = strings.TrimSpace(requestModels[0])
	}
	processResponsesStreamForRequest(ctx, body, ch, requestModel, semantics, semantics == ResponsesSemanticsOpenAICodex && isOpenAIResponsesLiteModel(requestModel))
}

func processResponsesStreamForRequest(ctx context.Context, body io.Reader, ch chan<- types.StreamEvent, requestModel string, semantics ResponsesSemantics, responsesLite bool, serviceTiers ...ServiceTier) {
	expectedServiceTier := ServiceTier("")
	if len(serviceTiers) > 0 {
		expectedServiceTier = serviceTiers[0]
	}
	parserCtx, cancelParser := context.WithCancel(ctx)
	defer cancelParser()
	processResponsesEventsForRequest(ctx, body, parseResponsesSSE(parserCtx, body), ch, requestModel, semantics, responsesLite, false, expectedServiceTier)
}

// processResponsesEventsForRequest is the transport-neutral Responses reducer.
// HTTP supplies SSE-framed events; WebSocket supplies one JSON event per text
// frame. Keeping a single reducer prevents protocol, usage, encrypted reasoning,
// and MessageStop commit semantics from drifting between transports.
func processResponsesEventsForRequest(
	ctx context.Context,
	activity any,
	events <-chan sseEvent,
	ch chan<- types.StreamEvent,
	requestModel string,
	semantics ResponsesSemantics,
	responsesLite bool,
	connectionScopedContinuation bool,
	expectedServiceTier ServiceTier,
) {
	encryptedReasoning := semantics == ResponsesSemanticsOpenAIPublic || semantics == ResponsesSemanticsOpenAICodex
	sendRaw := func(evt types.StreamEvent) bool {
		select {
		case ch <- evt:
			return true
		case <-ctx.Done():
			return false
		}
	}

	// Track output items by their index for mapping to stream events.
	type outputItem struct {
		itemType           string // "message", "function_call", "custom_tool_call", "reasoning"
		blockIdx           int    // our block index for StreamEvent
		callID             string
		name               string
		providerItemID     string
		providerStatus     string
		signature          string
		stopped            bool
		itemDone           bool
		functionInputBytes int
		functionInput      strings.Builder
		functionFinal      *string
		customInput        strings.Builder
		customFinal        *string
	}
	outputItems := make(map[int]*outputItem) // keyed by output_index
	completedOutputItems := make(map[int]json.RawMessage)
	nextBlockIdx := 0
	messageStarted := false
	responseIDSeen := ""
	wireSequence := 0
	currentWireEvent := "unknown"
	currentDataBytes := 0
	baseDiagnostic := providerFailureDiagnosticFromContext(ctx)
	if baseDiagnostic == nil {
		baseDiagnostic = &types.ProviderFailureDiagnostic{
			SchemaVersion: types.ProviderFailureDiagnosticSchema,
			Model:         safeProviderProtocolIdentifier(requestModel), APIFormat: "responses",
		}
	}
	buildFailureDiagnostic := func(point types.ProviderFailurePoint, outputIndex int, outputSet bool) *types.ProviderFailureDiagnostic {
		diagnostic := baseDiagnostic.Clone()
		diagnostic.FailurePoint = point
		diagnostic.Stage = types.ProviderErrorStageStream
		diagnostic.Class = types.ProviderErrorClassPermanent
		diagnostic.ReplaySafety = types.ProviderReplaySafe
		diagnostic.ResponseID = safeProviderProtocolIdentifier(responseIDSeen)
		diagnostic.WireSequence = wireSequence
		diagnostic.WireEvent = safeResponsesWireEvent(currentWireEvent)
		diagnostic.DataBytes = currentDataBytes
		diagnostic.OutputIndex = outputIndex
		diagnostic.OutputSet = outputSet
		diagnostic.MessageStarted = messageStarted
		diagnostic.TrackedItems = len(outputItems)
		diagnostic.CompletedItems = len(completedOutputItems)
		for _, tracked := range outputItems {
			if tracked != nil && tracked.stopped {
				diagnostic.StoppedItems++
			}
		}
		if outputSet {
			item := outputItems[outputIndex]
			diagnostic.ItemPresent = item != nil
			if item != nil {
				diagnostic.ItemType = safeResponsesItemType(item.itemType)
				diagnostic.ItemStatus = safeResponsesItemStatus(item.providerStatus)
				diagnostic.ItemStopped = item.stopped
				diagnostic.CallIDPresent = item.callID != ""
				diagnostic.NamePresent = item.name != ""
				diagnostic.ItemIDPresent = item.providerItemID != ""
				diagnostic.StreamInputBytes = item.functionInputBytes
				diagnostic.FinalSeen = item.functionFinal != nil
				if item.itemType == "custom_tool_call" {
					diagnostic.StreamInputBytes = item.customInput.Len()
					diagnostic.FinalSeen = item.customFinal != nil
				}
			}
		}
		return diagnostic
	}
	send := func(evt types.StreamEvent) bool {
		if evt.Type == types.EventError && evt.Error != nil && evt.Error.FailureDiagnostic == nil {
			point := types.ProviderFailureUnknown
			switch evt.Error.Type {
			case "stream_idle_timeout":
				point = types.ProviderFailureStreamIdleTimeout
			case "stream_interrupted":
				point = types.ProviderFailureStreamInterrupted
			case "invalid_continuation":
				point = types.ProviderFailureContinuationInvalid
			case "service_tier_mismatch":
				point = types.ProviderFailureServiceTierMismatch
			case "response_failed":
				point = types.ProviderFailureResponseFailed
			}
			evt.Error = applyFailureDiagnosticToAPIError(evt.Error, buildFailureDiagnostic(point, 0, false))
		}
		if evt.Type == types.EventError && evt.Error != nil && evt.Error.FailureDiagnostic != nil {
			contract := ClassifyAttemptError(evt.Error)
			evt.Error.FailureDiagnostic.Stage = contract.Stage
			evt.Error.FailureDiagnostic.Class = contract.Class
			evt.Error.FailureDiagnostic.ReplaySafety = contract.ReplaySafety
		}
		return sendRaw(evt)
	}
	sendProtocolError := func(point types.ProviderFailurePoint, custom bool, outputIndex int, outputSet bool) {
		errType := "invalid_tool_call"
		if custom {
			errType = "invalid_custom_tool_call"
		}
		diagnostic := buildFailureDiagnostic(point, outputIndex, outputSet)
		send(types.StreamEvent{Type: types.EventError, Error: applyFailureDiagnosticToAPIError(&types.APIError{
			Type: errType, Message: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyProviderResponsesCustomToolCallInvalid),
			Stage: types.ProviderErrorStageStream, Class: types.ProviderErrorClassPermanent,
			ReplaySafety: types.ProviderReplaySafe,
		}, diagnostic)})
	}
	sendKnownEventParseError := func(outputIndex int, outputSet bool, parseErrors ...error) {
		diagnostic := buildFailureDiagnostic(types.ProviderFailureSSEKnownEventParse, outputIndex, outputSet)
		var parseDiagnostic any = currentWireEvent
		if len(parseErrors) > 0 && parseErrors[0] != nil {
			parseDiagnostic = parseErrors[0]
		}
		message := i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyProviderResponsesKnownEventParseFailed, currentWireEvent)
		switch currentWireEvent {
		case "response.completed":
			message = i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyProviderResponsesCompletedParseFailed, parseDiagnostic)
		case "response.incomplete":
			message = i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyProviderResponsesIncompleteParseFailed, parseDiagnostic)
		case "response.failed":
			message = i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyProviderResponsesFailedParseFailed, parseDiagnostic)
		}
		send(types.StreamEvent{Type: types.EventError, Error: applyFailureDiagnosticToAPIError(&types.APIError{
			Type: "parse_error", Message: message,
			Stage: types.ProviderErrorStageStream, Class: types.ProviderErrorClassPermanent,
			ReplaySafety: types.ProviderReplaySafe,
		}, diagnostic)})
	}

	completedNormally := false
	for sse := range events {
		wireSequence++
		currentDataBytes = len(sse.Data)
		if ctx.Err() != nil {
			return
		}
		if sse.Err == nil {
			if watchdog, ok := activity.(*streamWatchdogBody); ok {
				watchdog.markActivity()
			}
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
		currentWireEvent = eventType

		switch eventType {
		case "response.created", "response.in_progress":
			var data struct {
				Response struct {
					ID string `json:"id"`
				} `json:"response"`
			}
			if err := json.Unmarshal([]byte(sse.Data), &data); err != nil {
				sendKnownEventParseError(0, false, err)
				completedNormally = true
				return
			}
			if data.Response.ID != "" {
				responseIDSeen = data.Response.ID
			}
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
					Type             string `json:"type"`
					ID               string `json:"id"`
					CallID           string `json:"call_id"`
					Name             string `json:"name"`
					Status           string `json:"status"`
					EncryptedContent string `json:"encrypted_content"`
				} `json:"item"`
			}
			if err := json.Unmarshal([]byte(sse.Data), &data); err != nil {
				sendKnownEventParseError(0, false, err)
				completedNormally = true
				return
			}

			item := &outputItem{
				itemType:       data.Item.Type,
				blockIdx:       data.OutputIndex,
				providerItemID: data.Item.ID,
				providerStatus: data.Item.Status,
			}
			if encryptedReasoning && data.Item.Type == "reasoning" {
				item.signature = data.Item.EncryptedContent
			}

			if data.Item.Type == "function_call" || data.Item.Type == "custom_tool_call" {
				item.callID = data.Item.CallID
				item.name = responsesLocalToolNameForSemantics(semantics, data.Item.Name, data.Item.Type == "custom_tool_call")
				toolType := types.ToolDefinitionTypeFunction
				if data.Item.Type == "custom_tool_call" {
					toolType = types.ToolDefinitionTypeCustom
				}
				// Emit content_block_start for tool_use
				if !send(types.StreamEvent{
					Type:  types.EventContentBlockStart,
					Index: item.blockIdx,
					ContentBlock: &types.ContentDelta{
						Type:           types.ContentTypeToolUse,
						ID:             data.Item.CallID,
						Name:           item.name,
						ToolType:       toolType,
						ProviderItemID: item.providerItemID,
						ProviderStatus: data.Item.Status,
					},
				}) {
					return
				}
				nextBlockIdx = max(nextBlockIdx, item.blockIdx+1)
			} else if data.Item.Type == "reasoning" {
				delta := &types.ContentDelta{
					Type:           types.ContentTypeThinking,
					ID:             item.providerItemID,
					ProviderStatus: item.providerStatus,
				}
				if encryptedReasoning {
					delta.Signature = item.signature
					delta.SignatureKind = types.ThinkingSignatureOpenAIEncryptedReasoning
					delta.SignatureModel = requestModel
				}
				if !send(types.StreamEvent{
					Type:         types.EventContentBlockStart,
					Index:        item.blockIdx,
					ContentBlock: delta,
				}) {
					return
				}
				nextBlockIdx = max(nextBlockIdx, item.blockIdx+1)
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
				sendKnownEventParseError(0, false)
				completedNormally = true
				return
			}

			item := outputItems[data.OutputIndex]
			if item == nil {
				item = &outputItem{itemType: "message", blockIdx: data.OutputIndex}
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
				sendKnownEventParseError(0, false)
				completedNormally = true
				return
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
				sendKnownEventParseError(0, false)
				completedNormally = true
				return
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
				sendKnownEventParseError(0, false)
				completedNormally = true
				return
			}
			item := outputItems[data.OutputIndex]
			if item == nil {
				sendProtocolError(types.ProviderFailureFunctionDeltaWithoutItem, false, data.OutputIndex, true)
				completedNormally = true
				return
			}
			if item.itemType != "function_call" {
				sendProtocolError(types.ProviderFailureFunctionDeltaWrongItem, false, data.OutputIndex, true)
				completedNormally = true
				return
			}
			if item.stopped {
				sendProtocolError(types.ProviderFailureFunctionDeltaAfterStop, false, data.OutputIndex, true)
				completedNormally = true
				return
			}
			if item.functionInputBytes+len(data.Delta) > responsesToolInputLimit(item.name) {
				diagnostic := buildFailureDiagnostic(types.ProviderFailureToolInputLimit, data.OutputIndex, true)
				sendResponsesToolInputLimitError(send, diagnostic)
				completedNormally = true
				return
			}
			item.functionInputBytes += len(data.Delta)
			item.functionInput.WriteString(data.Delta)

			if !send(types.StreamEvent{
				Type:  types.EventContentBlockDelta,
				Index: item.blockIdx,
				Delta: &types.ContentDelta{
					Type:        "input_json_delta",
					PartialJSON: data.Delta,
				},
			}) {
				return
			}

		case "response.custom_tool_call_input.delta":
			var data struct {
				OutputIndex int    `json:"output_index"`
				Delta       string `json:"delta"`
			}
			if err := json.Unmarshal([]byte(sse.Data), &data); err != nil {
				sendKnownEventParseError(0, false)
				completedNormally = true
				return
			}
			item := outputItems[data.OutputIndex]
			if item == nil {
				sendProtocolError(types.ProviderFailureCustomDeltaWithoutItem, true, data.OutputIndex, true)
				completedNormally = true
				return
			}
			if item.itemType != "custom_tool_call" {
				sendProtocolError(types.ProviderFailureCustomDeltaWrongItem, true, data.OutputIndex, true)
				completedNormally = true
				return
			}
			if item.stopped {
				sendProtocolError(types.ProviderFailureCustomDeltaAfterStop, true, data.OutputIndex, true)
				completedNormally = true
				return
			}
			if item.customInput.Len()+len(data.Delta) > responsesToolInputLimit(item.name) {
				diagnostic := buildFailureDiagnostic(types.ProviderFailureToolInputLimit, data.OutputIndex, true)
				sendResponsesToolInputLimitError(send, diagnostic)
				completedNormally = true
				return
			}
			item.customInput.WriteString(data.Delta)
			if !send(types.StreamEvent{
				Type:  types.EventContentBlockDelta,
				Index: item.blockIdx,
				Delta: &types.ContentDelta{Type: "input_text_delta", PartialText: data.Delta, ToolType: types.ToolDefinitionTypeCustom},
			}) {
				return
			}

		case "response.reasoning.delta", "response.reasoning_text.delta", "response.reasoning_summary_text.delta":
			var data struct {
				OutputIndex int    `json:"output_index"`
				Delta       string `json:"delta"`
			}
			if err := json.Unmarshal([]byte(sse.Data), &data); err != nil {
				sendKnownEventParseError(0, false)
				completedNormally = true
				return
			}
			item := outputItems[data.OutputIndex]
			idx := 0
			if item != nil {
				idx = item.blockIdx
			}
			if data.Delta != "" {
				thinkingKind := types.ThinkingKindRaw
				if eventType == "response.reasoning_summary_text.delta" {
					thinkingKind = types.ThinkingKindSummary
				}
				if !send(types.StreamEvent{
					Type:  types.EventContentBlockDelta,
					Index: idx,
					Delta: &types.ContentDelta{Type: "thinking_delta", Thinking: data.Delta, ThinkingKind: thinkingKind},
				}) {
					return
				}
			}

		case "response.function_call_arguments.done":
			var data struct {
				OutputIndex int     `json:"output_index"`
				ItemID      string  `json:"item_id"`
				Arguments   *string `json:"arguments"`
			}
			if err := json.Unmarshal([]byte(sse.Data), &data); err != nil {
				sendKnownEventParseError(0, false)
				completedNormally = true
				return
			}
			item := outputItems[data.OutputIndex]
			point := types.ProviderFailurePoint("")
			switch {
			case item == nil:
				point = types.ProviderFailureFunctionDoneWithoutItem
			case item.itemType != "function_call":
				point = types.ProviderFailureFunctionDoneWrongItem
			case item.stopped:
				point = types.ProviderFailureFunctionDoneAfterStop
			case data.Arguments == nil:
				point = types.ProviderFailureFunctionDoneMissingArguments
			case data.ItemID != "" && item.providerItemID != "" && data.ItemID != item.providerItemID:
				point = types.ProviderFailureFunctionDoneItemIDMismatch
			}
			if point != "" {
				sendProtocolError(point, false, data.OutputIndex, true)
				completedNormally = true
				return
			}
			if len(*data.Arguments) > responsesToolInputLimit(item.name) {
				diagnostic := buildFailureDiagnostic(types.ProviderFailureToolInputLimit, data.OutputIndex, true)
				diagnostic.FinalInputBytes = len(*data.Arguments)
				sendResponsesToolInputLimitError(send, diagnostic)
				completedNormally = true
				return
			}
			finalArguments := *data.Arguments
			item.functionFinal = &finalArguments
			if !send(types.StreamEvent{
				Type:  types.EventContentBlockDelta,
				Index: item.blockIdx,
				Delta: &types.ContentDelta{
					Type: "tool_state_final", ID: item.callID, Name: item.name,
					ToolType: types.ToolDefinitionTypeFunction, PartialJSON: finalArguments,
				},
			}) {
				return
			}

		case "response.custom_tool_call_input.done":
			var data struct {
				OutputIndex int     `json:"output_index"`
				Input       *string `json:"input"`
			}
			if err := json.Unmarshal([]byte(sse.Data), &data); err != nil {
				sendKnownEventParseError(0, false)
				completedNormally = true
				return
			}
			item := outputItems[data.OutputIndex]
			point := types.ProviderFailurePoint("")
			switch {
			case item == nil:
				point = types.ProviderFailureCustomDoneWithoutItem
			case item.itemType != "custom_tool_call":
				point = types.ProviderFailureCustomDoneWrongItem
			case item.stopped:
				point = types.ProviderFailureCustomDoneAfterStop
			case data.Input == nil:
				point = types.ProviderFailureCustomDoneMissingInput
			}
			if point != "" {
				sendProtocolError(point, true, data.OutputIndex, true)
				completedNormally = true
				return
			}
			if len(*data.Input) > responsesToolInputLimit(item.name) {
				diagnostic := buildFailureDiagnostic(types.ProviderFailureToolInputLimit, data.OutputIndex, true)
				diagnostic.FinalInputBytes = len(*data.Input)
				sendResponsesToolInputLimitError(send, diagnostic)
				completedNormally = true
				return
			}
			if item.customInput.Len() > 0 && item.customInput.String() != *data.Input {
				diagnostic := buildFailureDiagnostic(types.ProviderFailureCustomFinalMismatch, data.OutputIndex, true)
				diagnostic.InputMatchChecked = true
				diagnostic.InputMatched = false
				diagnostic.FinalInputBytes = len(*data.Input)
				send(types.StreamEvent{Type: types.EventError, Error: applyFailureDiagnosticToAPIError(&types.APIError{
					Type: "invalid_custom_tool_call", Message: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyProviderResponsesCustomToolCallInvalid),
					Stage: types.ProviderErrorStageStream, Class: types.ProviderErrorClassPermanent, ReplaySafety: types.ProviderReplaySafe,
				}, diagnostic)})
				completedNormally = true
				return
			}
			finalInput := *data.Input
			item.customFinal = &finalInput
			if !send(types.StreamEvent{
				Type:  types.EventContentBlockDelta,
				Index: item.blockIdx,
				Delta: &types.ContentDelta{
					Type: "tool_state_final", ID: item.callID, Name: item.name,
					ToolType: types.ToolDefinitionTypeCustom, PartialText: *data.Input,
				},
			}) {
				return
			}

		case "response.output_item.done":
			var rawData struct {
				OutputIndex int             `json:"output_index"`
				Item        json.RawMessage `json:"item"`
			}
			if err := json.Unmarshal([]byte(sse.Data), &rawData); err == nil && len(rawData.Item) > 0 {
				completedOutputItems[rawData.OutputIndex] = append(json.RawMessage(nil), rawData.Item...)
			} else if err != nil {
				sendKnownEventParseError(0, false)
				completedNormally = true
				return
			}
			var data struct {
				OutputIndex int `json:"output_index"`
				Item        struct {
					Type             string  `json:"type"`
					ID               string  `json:"id"`
					CallID           string  `json:"call_id"`
					Name             string  `json:"name"`
					Arguments        *string `json:"arguments"`
					Input            *string `json:"input"`
					Status           string  `json:"status"`
					EncryptedContent string  `json:"encrypted_content"`
				} `json:"item"`
			}
			if err := json.Unmarshal([]byte(sse.Data), &data); err != nil {
				sendKnownEventParseError(0, false)
				completedNormally = true
				return
			}

			item := outputItems[data.OutputIndex]
			if item == nil && (data.Item.Type == "function_call" || data.Item.Type == "custom_tool_call") {
				item = &outputItem{
					itemType:       data.Item.Type,
					blockIdx:       data.OutputIndex,
					callID:         data.Item.CallID,
					name:           responsesLocalToolNameForSemantics(semantics, data.Item.Name, data.Item.Type == "custom_tool_call"),
					providerItemID: data.Item.ID,
					providerStatus: data.Item.Status,
				}
				outputItems[data.OutputIndex] = item
				toolType := types.ToolDefinitionTypeFunction
				if data.Item.Type == "custom_tool_call" {
					toolType = types.ToolDefinitionTypeCustom
				}
				if !send(types.StreamEvent{
					Type:  types.EventContentBlockStart,
					Index: item.blockIdx,
					ContentBlock: &types.ContentDelta{
						Type:           types.ContentTypeToolUse,
						ID:             data.Item.CallID,
						Name:           item.name,
						ToolType:       toolType,
						ProviderItemID: data.Item.ID,
						ProviderStatus: data.Item.Status,
					},
				}) {
					return
				}
				nextBlockIdx = max(nextBlockIdx, item.blockIdx+1)
			}
			if item == nil && data.Item.Type == "reasoning" {
				item = &outputItem{itemType: "reasoning", blockIdx: data.OutputIndex}
				outputItems[data.OutputIndex] = item
				delta := &types.ContentDelta{
					Type:           types.ContentTypeThinking,
					ID:             data.Item.ID,
					ProviderStatus: data.Item.Status,
				}
				if encryptedReasoning {
					delta.SignatureKind = types.ThinkingSignatureOpenAIEncryptedReasoning
					delta.SignatureModel = requestModel
				}
				if !send(types.StreamEvent{
					Type:         types.EventContentBlockStart,
					Index:        item.blockIdx,
					ContentBlock: delta,
				}) {
					return
				}
				nextBlockIdx = max(nextBlockIdx, item.blockIdx+1)
			}
			if item != nil {
				if item.stopped && (item.itemType == "function_call" || item.itemType == "custom_tool_call") {
					point := types.ProviderFailureCompletedToolBatchConflict
					if item.itemType == "custom_tool_call" {
						point = types.ProviderFailureCustomDuplicateStop
					}
					sendProtocolError(point, item.itemType == "custom_tool_call", data.OutputIndex, true)
					completedNormally = true
					return
				}
				if item.itemType == "function_call" || item.itemType == "custom_tool_call" {
					custom := item.itemType == "custom_tool_call"
					wireName := responsesLocalToolNameForSemantics(semantics, data.Item.Name, custom)
					if data.Item.Status == "incomplete" {
						// output_item.done is provisional when the provider is about
						// to terminate the whole response as incomplete. Keep the item
						// open so response.incomplete can authoritatively map the batch
						// to max_tokens without ever authorizing a tool call.
						item.providerStatus = data.Item.Status
						item.itemDone = true
						continue
					}
					if data.Item.Status != "completed" {
						point := types.ProviderFailureFunctionInvalidStatus
						if custom {
							point = types.ProviderFailureCustomInvalidStatus
						}
						sendProtocolError(point, custom, data.OutputIndex, true)
						completedNormally = true
						return
					}
					identityConflict := data.Item.Type != item.itemType || data.Item.ID == "" ||
						data.Item.CallID == "" || wireName == "" ||
						(item.providerItemID != "" && item.providerItemID != data.Item.ID) ||
						(item.callID != "" && item.callID != data.Item.CallID) ||
						(item.name != "" && item.name != wireName)
					if identityConflict {
						sendProtocolError(types.ProviderFailureCompletedToolBatchConflict, custom, data.OutputIndex, true)
						completedNormally = true
						return
					}
					item.providerItemID = data.Item.ID
					item.callID = data.Item.CallID
					item.name = wireName
					item.providerStatus = data.Item.Status
				}
				if item.itemType == "reasoning" {
					changed := false
					if data.Item.ID != "" {
						changed = changed || data.Item.ID != item.providerItemID
						item.providerItemID = data.Item.ID
					}
					if data.Item.Status != "" {
						changed = changed || data.Item.Status != item.providerStatus
						item.providerStatus = data.Item.Status
					}
					if encryptedReasoning && data.Item.EncryptedContent != "" {
						changed = changed || data.Item.EncryptedContent != item.signature
						item.signature = data.Item.EncryptedContent
					}
					if changed {
						deltaType := "thinking_state_final"
						if encryptedReasoning {
							deltaType = "signature_delta"
						}
						delta := &types.ContentDelta{
							Type:           types.ContentType(deltaType),
							ID:             item.providerItemID,
							ProviderStatus: item.providerStatus,
						}
						if encryptedReasoning && item.signature != "" {
							delta.Signature = item.signature
							delta.SignatureKind = types.ThinkingSignatureOpenAIEncryptedReasoning
							delta.SignatureModel = requestModel
						}
						if !send(types.StreamEvent{
							Type:  types.EventContentBlockDelta,
							Index: item.blockIdx,
							Delta: delta,
						}) {
							return
						}
					}
				} else if item.itemType == "function_call" {
					if data.Item.Arguments == nil && item.functionFinal == nil && item.functionInput.Len() == 0 {
						sendProtocolError(types.ProviderFailureFunctionDoneMissingArguments, false, data.OutputIndex, true)
						completedNormally = true
						return
					}
					if data.Item.Arguments != nil && len(*data.Item.Arguments) > responsesToolInputLimit(item.name) {
						diagnostic := buildFailureDiagnostic(types.ProviderFailureToolInputLimit, data.OutputIndex, true)
						diagnostic.FinalInputBytes = len(*data.Item.Arguments)
						sendResponsesToolInputLimitError(send, diagnostic)
						completedNormally = true
						return
					}
					if data.Item.Arguments != nil && item.functionFinal != nil && *data.Item.Arguments != *item.functionFinal {
						diagnostic := buildFailureDiagnostic(types.ProviderFailureFunctionFinalMismatch, data.OutputIndex, true)
						diagnostic.InputMatchChecked = true
						diagnostic.InputMatched = false
						diagnostic.FinalInputBytes = len(*data.Item.Arguments)
						send(types.StreamEvent{Type: types.EventError, Error: applyFailureDiagnosticToAPIError(&types.APIError{
							Type: "invalid_tool_call", Message: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyProviderResponsesCustomToolCallInvalid),
							Stage: types.ProviderErrorStageStream, Class: types.ProviderErrorClassPermanent, ReplaySafety: types.ProviderReplaySafe,
						}, diagnostic)})
						completedNormally = true
						return
					}
					finalArguments := item.functionInput.String()
					if item.functionFinal != nil {
						finalArguments = *item.functionFinal
					}
					if data.Item.Arguments != nil {
						finalArguments = *data.Item.Arguments
					}
					item.functionFinal = &finalArguments
					if !send(types.StreamEvent{
						Type:  types.EventContentBlockDelta,
						Index: item.blockIdx,
						Delta: &types.ContentDelta{
							Type:           "tool_state_final",
							ID:             item.callID,
							Name:           item.name,
							PartialJSON:    finalArguments,
							ProviderStatus: "completed",
						},
					}) {
						return
					}
				} else if item.itemType == "custom_tool_call" {
					if data.Item.Input != nil && len(*data.Item.Input) > responsesToolInputLimit(item.name) {
						diagnostic := buildFailureDiagnostic(types.ProviderFailureToolInputLimit, data.OutputIndex, true)
						diagnostic.FinalInputBytes = len(*data.Item.Input)
						sendResponsesToolInputLimitError(send, diagnostic)
						completedNormally = true
						return
					}
					if data.Item.Input == nil && item.customFinal == nil {
						sendProtocolError(types.ProviderFailureCustomDoneMissingInput, true, data.OutputIndex, true)
						completedNormally = true
						return
					}
					if data.Item.Input != nil && ((item.customFinal != nil && *item.customFinal != *data.Item.Input) ||
						(item.customFinal == nil && item.customInput.Len() > 0 && item.customInput.String() != *data.Item.Input)) {
						diagnostic := buildFailureDiagnostic(types.ProviderFailureCustomFinalMismatch, data.OutputIndex, true)
						diagnostic.InputMatchChecked = true
						diagnostic.InputMatched = false
						diagnostic.FinalInputBytes = len(*data.Item.Input)
						send(types.StreamEvent{Type: types.EventError, Error: applyFailureDiagnosticToAPIError(&types.APIError{
							Type: "invalid_custom_tool_call", Message: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyProviderResponsesCustomToolCallInvalid),
							Stage: types.ProviderErrorStageStream, Class: types.ProviderErrorClassPermanent, ReplaySafety: types.ProviderReplaySafe,
						}, diagnostic)})
						completedNormally = true
						return
					}
					finalInput := ""
					if item.customFinal != nil {
						finalInput = *item.customFinal
					}
					if data.Item.Input != nil {
						finalInput = *data.Item.Input
					}
					item.customFinal = &finalInput
					if !send(types.StreamEvent{
						Type:  types.EventContentBlockDelta,
						Index: item.blockIdx,
						Delta: &types.ContentDelta{
							Type: "tool_state_final", ID: item.callID, Name: item.name,
							ToolType: types.ToolDefinitionTypeCustom, PartialText: finalInput,
							ProviderStatus: "completed",
						},
					}) {
						return
					}
				}
				item.itemDone = true
				if !send(types.StreamEvent{
					Type:  types.EventContentBlockStop,
					Index: item.blockIdx,
				}) {
					return
				}
				item.stopped = true
			}

		case "response.content_part.done":
			// Content part finalized within an output item
			var data struct {
				OutputIndex int `json:"output_index"`
			}
			if err := json.Unmarshal([]byte(sse.Data), &data); err != nil {
				sendKnownEventParseError(0, false)
				completedNormally = true
				return
			}
			// For message output items, the content_part.done signals text block end
			// But we let output_item.done handle the EventContentBlockStop

		case "response.completed", "response.incomplete":
			var data struct {
				Response struct {
					ID          string `json:"id"`
					Model       string `json:"model"`
					Status      string `json:"status"`
					ServiceTier string `json:"service_tier"`
					Usage       struct {
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
					Output []json.RawMessage `json:"output"`
				} `json:"response"`
			}
			if err := json.Unmarshal([]byte(sse.Data), &data); err != nil {
				sendKnownEventParseError(0, false, err)
				completedNormally = true // don't also emit stream_interrupted
				return
			}
			if eventType == "response.incomplete" && data.Response.Status == "" {
				data.Response.Status = "incomplete"
			}
			if data.Response.ID != "" {
				responseIDSeen = data.Response.ID
			}
			// Preserve usage even when a contract-bound request is rejected for
			// scheduling drift; the provider may already have billed this attempt.
			usage := normalizeOpenAIUsage(
				data.Response.Usage.InputTokens,
				data.Response.Usage.OutputTokens,
				data.Response.Usage.InputTokensDetails.CachedTokens,
				data.Response.Usage.InputTokensDetails.CacheWriteTokens,
			)
			if expectedServiceTier != "" && data.Response.ServiceTier != string(expectedServiceTier) {
				send(types.StreamEvent{Type: types.EventMessageDelta, Usage: usage})
				send(types.StreamEvent{
					Type: types.EventError,
					Error: &types.APIError{
						Type: "service_tier_mismatch",
						Message: i18n.Format(
							i18n.DetectOrLoadLanguage(),
							i18n.KeyProviderServiceTierMismatch,
							data.Response.ServiceTier,
							expectedServiceTier,
						),
					},
				})
				completedNormally = true
				return
			}
			responseOutput := data.Response.Output
			if len(responseOutput) == 0 && len(completedOutputItems) > 0 {
				responseOutput = make([]json.RawMessage, len(completedOutputItems))
				for outputIndex := range responseOutput {
					rawOutput, ok := completedOutputItems[outputIndex]
					if !ok {
						diagnostic := buildFailureDiagnostic(types.ProviderFailureCompletedOutputGap, outputIndex, true)
						send(types.StreamEvent{Type: types.EventError, Error: applyFailureDiagnosticToAPIError(&types.APIError{
							Type: "invalid_continuation", Message: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyProviderResponsesContinuationInvalid),
							Stage: types.ProviderErrorStageStream, Class: types.ProviderErrorClassPermanent, ReplaySafety: types.ProviderReplaySafe,
						}, diagnostic)})
						completedNormally = true
						return
					}
					responseOutput[outputIndex] = rawOutput
				}
			}
			responseIncomplete := eventType == "response.incomplete" || data.Response.Status == "incomplete"
			incompleteReason := strings.TrimSpace(data.Response.IncompleteDetails.Reason)
			responseTruncated := incompleteReason == "max_output_tokens" || incompleteReason == "max_tokens"
			if responseTruncated {
				// A provider-declared output limit is a committed, incomplete response,
				// not a transport failure. Commit only the safe observed prefix. In
				// particular, do not issue a tool receipt or continuation: the runtime
				// quarantines every provisional tool block for max_tokens responses.
				sr := types.StopReasonMaxTokens
				if !send(types.StreamEvent{Type: types.EventMessageDelta, StopReason: &sr, Usage: usage}) {
					return
				}
				completedNormally = true
				send(types.StreamEvent{Type: types.EventMessageStop, ResponseID: data.Response.ID})
				return
			}
			if responseIncomplete {
				// Unknown incomplete reasons are not interchangeable with output
				// exhaustion. Fail closed with a typed, content-free diagnostic.
				diagnostic := buildFailureDiagnostic(types.ProviderFailureResponseIncomplete, 0, false)
				diagnostic.CommitSeen = false
				diagnostic.IncompleteReason = safeResponsesIncompleteReason(incompleteReason)
				send(types.StreamEvent{Type: types.EventError, Error: applyFailureDiagnosticToAPIError(&types.APIError{
					Type: "response_incomplete", Message: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeResponsesStreamIncomplete),
					Stage: types.ProviderErrorStageStream, Class: types.ProviderErrorClassPermanent,
					ReplaySafety: types.ProviderReplaySafe,
				}, diagnostic)})
				completedNormally = true
				return
			}
			if data.Response.Status != "completed" {
				sendProtocolError(types.ProviderFailureCompletedToolBatchConflict, false, 0, false)
				completedNormally = true
				return
			}

			seenToolItems := make(map[int]struct{})
			toolBatchCalls := make([]types.ProviderToolCallCommit, 0)
			for outputIndex, rawOutput := range responseOutput {
				var output struct {
					Type      string  `json:"type"`
					ID        string  `json:"id"`
					CallID    string  `json:"call_id"`
					Name      string  `json:"name"`
					Status    string  `json:"status"`
					Arguments *string `json:"arguments"`
					Input     *string `json:"input"`
				}
				if json.Unmarshal(rawOutput, &output) != nil {
					sendProtocolError(types.ProviderFailureCompletedToolBatchConflict, false, outputIndex, true)
					completedNormally = true
					return
				}
				if output.Type != "function_call" && output.Type != "custom_tool_call" {
					continue
				}
				custom := output.Type == "custom_tool_call"
				item := outputItems[outputIndex]
				if item == nil || item.itemType != output.Type || !item.itemDone || !item.stopped {
					point := types.ProviderFailureCompletedFunctionItemUnfinalized
					if custom {
						point = types.ProviderFailureCompletedCustomItemUnfinalized
					}
					sendProtocolError(point, custom, outputIndex, true)
					completedNormally = true
					return
				}
				wireName := responsesLocalToolNameForSemantics(semantics, output.Name, custom)
				if (output.Status != "" && output.Status != "completed") || item.providerStatus != "completed" ||
					item.providerItemID == "" || item.callID == "" || item.name == "" ||
					(output.ID != "" && output.ID != item.providerItemID) ||
					(output.CallID != "" && output.CallID != item.callID) ||
					(output.Name != "" && wireName != item.name) {
					point := types.ProviderFailureCompletedFunctionStatusInvalid
					if custom {
						point = types.ProviderFailureCompletedCustomStatusInvalid
					}
					sendProtocolError(point, custom, outputIndex, true)
					completedNormally = true
					return
				}
				var rawPayload string
				if custom {
					if item.customFinal == nil || (output.Input != nil && *output.Input != *item.customFinal) {
						sendProtocolError(types.ProviderFailureCompletedToolBatchConflict, true, outputIndex, true)
						completedNormally = true
						return
					}
					rawPayload = *item.customFinal
				} else {
					if item.functionFinal == nil || (output.Arguments != nil && *output.Arguments != *item.functionFinal) {
						sendProtocolError(types.ProviderFailureCompletedToolBatchConflict, false, outputIndex, true)
						completedNormally = true
						return
					}
					rawPayload = *item.functionFinal
				}
				toolType := types.ToolDefinitionTypeFunction
				if custom {
					toolType = types.ToolDefinitionTypeCustom
				}
				toolBatchCalls = append(toolBatchCalls, types.ProviderToolCallCommit{
					OutputIndex: outputIndex, ToolType: toolType, ProviderItemID: item.providerItemID,
					CallID: item.callID, Name: item.name, RawInput: rawPayload,
				})
				seenToolItems[outputIndex] = struct{}{}
			}
			for outputIndex, item := range outputItems {
				if item == nil || (item.itemType != "function_call" && item.itemType != "custom_tool_call") {
					continue
				}
				if _, present := seenToolItems[outputIndex]; !present {
					sendProtocolError(types.ProviderFailureCompletedToolBatchConflict, item.itemType == "custom_tool_call", outputIndex, true)
					completedNormally = true
					return
				}
			}

			// Store response ID for next turn's chaining
			responseID := data.Response.ID
			continuation, continuationErr := buildResponsesContinuation(
				responseOutput, requestModel, data.Response.Model, data.Response.Status, semantics, responsesLite,
			)
			if continuationErr != nil {
				send(types.StreamEvent{
					Type: types.EventError,
					Error: &types.APIError{
						Type:    "invalid_continuation",
						Message: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyProviderResponsesContinuationInvalid),
					},
				})
				completedNormally = true
				return
			}

			// Determine stop reason from output items
			sr := types.StopReasonEndTurn
			for _, rawOutput := range responseOutput {
				var out struct {
					Type string `json:"type"`
				}
				if json.Unmarshal(rawOutput, &out) == nil && (out.Type == "function_call" || out.Type == "custom_tool_call") {
					sr = types.StopReasonToolUse
					break
				}
			}
			if !send(types.StreamEvent{
				Type:       types.EventMessageDelta,
				StopReason: &sr,
				Usage:      usage,
			}) {
				return
			}

			receipt := types.NewProviderToolCommitReceipt(baseDiagnostic.Provider, "responses", data.Response.Status, toolBatchCalls)
			completedNormally = true
			send(types.StreamEvent{
				Type: types.EventMessageStop, ResponseID: responseID,
				ProviderContinuation: continuation, ProviderCommitReceipt: receipt,
			})
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
				sendKnownEventParseError(0, false, err)
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
			code := data.Response.Error.Code
			errType, status := classifyResponsesFailedError(code, errMsg, status)
			retryAfter := responsesFailedRetryAfter(code, errMsg)
			if connectionScopedContinuation && errType == "previous_response_not_found" {
				errType, status = "stream_interrupted", 0
				code = errType
			}

			diagnostic := buildFailureDiagnostic(types.ProviderFailureResponseFailed, 0, false)
			send(types.StreamEvent{
				Type: types.EventError,
				Error: applyFailureDiagnosticToAPIError(&types.APIError{
					Type:         errType,
					Code:         code,
					Message:      errMsg,
					Status:       status,
					RetryAfter:   retryAfter,
					Stage:        types.ProviderErrorStageStream,
					ReplaySafety: types.ProviderReplaySafe,
				}, diagnostic),
			})
			completedNormally = true // explicit failure, not an interruption
			return

		case "error":
			if errors.Is(sse.Err, errResponsesFunctionCallDeltaLineTooLarge) {
				sendResponsesToolInputLimitError(send, buildFailureDiagnostic(types.ProviderFailureToolInputLimit, 0, false))
				completedNormally = true
				return
			}
			if idle, ok := streamIdleTimeoutFromError(sse.Err); ok {
				send(types.StreamEvent{
					Type: types.EventError,
					Error: &types.APIError{
						Type:    "stream_idle_timeout",
						Message: idle.Error(),
					},
				})
				completedNormally = true
				return
			}
			if sse.Err != nil {
				send(types.StreamEvent{
					Type: types.EventError,
					Error: &types.APIError{
						Type:    "stream_interrupted",
						Message: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeResponsesStreamIncomplete),
					},
				})
				completedNormally = true
				return
			}
			var data struct {
				Message string `json:"message"`
				Code    string `json:"code"`
				Status  int    `json:"status"`
				Error   struct {
					Message    string `json:"message"`
					Code       string `json:"code"`
					Status     int    `json:"status"`
					StatusCode int    `json:"status_code"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(sse.Data), &data); err != nil {
				sendKnownEventParseError(0, false)
				completedNormally = true
				return
			}
			message, code, status := data.Message, data.Code, data.Status
			if data.Error.Message != "" {
				message = data.Error.Message
			}
			if data.Error.Code != "" {
				code = data.Error.Code
			}
			if data.Error.Status != 0 {
				status = data.Error.Status
			} else if data.Error.StatusCode != 0 {
				status = data.Error.StatusCode
			}
			errType, status := classifyResponsesAPIError(code, message, status)
			if connectionScopedContinuation && errType == "previous_response_not_found" {
				errType, status = "stream_interrupted", 0
				code = errType
			}
			send(types.StreamEvent{
				Type: types.EventError,
				Error: &types.APIError{
					Type:         errType,
					Code:         code,
					Message:      message,
					Status:       status,
					Stage:        types.ProviderErrorStageStream,
					ReplaySafety: types.ProviderReplaySafe,
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
	}
}

// A runaway argument stream is an uncommitted generation failure, so replay is
// safe. Classifying it as a transport interruption reuses the generation-scoped
// retry budget while closing the response body immediately; the caller never
// accumulates the oversized JSON and no tool side effect can have started.
func sendResponsesToolInputLimitError(send func(types.StreamEvent) bool, diagnostic *types.ProviderFailureDiagnostic) {
	send(types.StreamEvent{
		Type: types.EventError,
		Error: applyFailureDiagnosticToAPIError(&types.APIError{
			Type:         "stream_interrupted",
			Code:         "tool_arguments_too_large",
			Message:      i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyRuntimeResponsesStreamIncomplete),
			Stage:        types.ProviderErrorStageStream,
			ReplaySafety: types.ProviderReplaySafe,
		}, diagnostic),
	})
}

func responsesToolInputLimit(name string) int {
	switch name {
	case "Inspect":
		return maxResponsesInspectToolInputBytes
	case "Run":
		return maxResponsesRunToolInputBytes
	case "ApplyPatch":
		return maxResponsesApplyPatchInputBytes
	default:
		return maxResponsesToolInputBytes
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

func classifyResponsesAPIError(code, _ string, status int) (string, int) {
	errCode := strings.ToLower(code)

	// A provider protocol code is stronger authority than the HTTP status or
	// diagnostic prose. Preserve the allowlisted context terminal exactly so
	// the machine projection never has to infer it from a localized message.
	if errCode == "context_length_exceeded" {
		return "context_length_exceeded", status
	}
	if strings.Contains(errCode, "previous_response_not_found") ||
		(strings.Contains(errCode, "previous_response") && strings.Contains(errCode, "not_found")) {
		return "previous_response_not_found", status
	}
	if errCode == "websocket_connection_limit_reached" {
		return "stream_interrupted", 0
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

// classifyResponsesFailedError preserves the protocol-level evidence carried by
// response.failed. When the provider omits a usable code and status, the event
// still proves that an uncommitted streaming generation failed upstream. That
// is materially different from an unclassified request/API error: the query
// loop can safely discard its partial blocks and replay the complete generation.
func classifyResponsesFailedError(code, message string, status int) (string, int) {
	errType, normalizedStatus := classifyResponsesAPIError(code, message, status)
	if errType == "api_error" && normalizedStatus == 0 {
		return "response_failed", 0
	}
	return errType, normalizedStatus
}

var responsesRateLimitRetryAfterPattern = regexp.MustCompile(`(?i)try again in\s*(\d+(?:\.\d+)?)\s*(s|ms|seconds?)`)

// responsesFailedRetryAfter mirrors Codex CLI's structured response.failed
// handling. Some Responses endpoints carry the retry delay only in the
// rate_limit_exceeded message instead of an HTTP Retry-After header.
func responsesFailedRetryAfter(code, message string) string {
	if !strings.EqualFold(strings.TrimSpace(code), "rate_limit_exceeded") {
		return ""
	}
	matches := responsesRateLimitRetryAfterPattern.FindStringSubmatch(message)
	if len(matches) != 3 {
		return ""
	}
	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil || value <= 0 {
		return ""
	}
	if strings.EqualFold(matches[2], "ms") {
		value /= 1000
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
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
			input = append(input, convertAssistantMessageToResponsesAPIForModel(msg, params.Model)...)
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
			input = append(input, convertAssistantMessageToResponsesAPIForModel(msg, params.Model)...)
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
	items, _ := convertUserMessageToResponsesAPIWithCallKinds(msg, nil)
	return items
}

func convertUserMessageToResponsesAPIWithCallKinds(msg types.Message, callKinds map[string]types.ToolDefinitionType) ([]any, error) {
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
			outputType := "function_call_output"
			if callKinds[b.ToolUseID] == types.ToolDefinitionTypeCustom {
				outputType = "custom_tool_call_output"
			} else if b.ToolType == types.ToolDefinitionTypeCustom {
				return nil, i18n.NewError(i18n.KeyProviderResponsesContinuationInvalid)
			}
			functionOutputs = append(functionOutputs, map[string]any{
				"type":    outputType,
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
	return append(functionOutputs, followUps...), nil
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

// convertAssistantMessageToResponsesAPI converts an assistant message to input
// items without replaying provider-bound continuation state. Production
// Responses requests use the model-aware variant below.
func convertAssistantMessageToResponsesAPI(msg types.Message) []any {
	return convertAssistantMessageToResponsesAPIForModel(msg, "")
}

// convertAssistantMessageToResponsesAPIForModel preserves the original output
// item order. In particular, encrypted reasoning must remain immediately before
// the function_call it authorized; moving text or calls around it changes the
// official stateless Responses history.
func convertAssistantMessageToResponsesAPIForModel(msg types.Message, model string) []any {
	return convertAssistantMessageToResponsesAPIForSemantics(msg, model, ResponsesSemanticsCompatible)
}

func convertAssistantMessageToResponsesAPIForSemantics(msg types.Message, model string, semantics ResponsesSemantics) []any {
	var items []any
	hasDeepSeekToolTurn := false
	if semantics == ResponsesSemanticsDeepSeek {
		for _, block := range msg.Content {
			if _, ok := block.(types.ToolUseBlock); ok {
				hasDeepSeekToolTurn = true
				break
			}
		}
	}
	for _, block := range msg.Content {
		switch value := block.(type) {
		case types.ThinkingBlock:
			if item, ok := openAIEncryptedReasoningInput(value, model); ok {
				items = append(items, item)
			} else if semantics == ResponsesSemanticsDeepSeek && hasDeepSeekToolTurn &&
				value.Thinking != "" && value.Kind != types.ThinkingKindSummary {
				items = append(items, map[string]any{
					"type": "reasoning",
					"content": []map[string]string{{
						"type": "reasoning_text",
						"text": value.Thinking,
					}},
				})
			}
		case types.TextBlock:
			if value.Text != "" {
				items = append(items, map[string]any{
					"role":    "assistant",
					"content": value.Text,
				})
			}
		case types.ToolUseBlock:
			if value.ToolType == types.ToolDefinitionTypeCustom {
				if value.ID == "" || value.Name == "" || value.RawInput == "" {
					continue
				}
				items = append(items, map[string]any{
					"type": "custom_tool_call", "call_id": value.ID,
					"name": responsesToolNameForSemantics(semantics, value.Name, true), "input": value.RawInput,
				})
				continue
			}
			args, err := json.Marshal(value.Input)
			if err != nil {
				continue
			}
			items = append(items, map[string]any{
				"type":      "function_call",
				"call_id":   value.ID,
				"name":      value.Name,
				"arguments": string(args),
			})
		}
	}

	return items
}

func openAIEncryptedReasoningInput(block types.ThinkingBlock, model string) (map[string]any, bool) {
	if block.Signature == "" || block.ProviderItemID == "" ||
		block.SignatureKind != types.ThinkingSignatureOpenAIEncryptedReasoning ||
		strings.TrimSpace(model) == "" || strings.TrimSpace(block.SignatureModel) != strings.TrimSpace(model) {
		return nil, false
	}
	summary := make([]map[string]string, 0, 1)
	if block.Thinking != "" {
		summary = append(summary, map[string]string{
			"type": "summary_text",
			"text": block.Thinking,
		})
	}
	item := map[string]any{
		"type":              "reasoning",
		"id":                block.ProviderItemID,
		"summary":           summary,
		"encrypted_content": block.Signature,
	}
	switch block.ProviderStatus {
	case "in_progress", "completed", "incomplete":
		item["status"] = block.ProviderStatus
	}
	return item, true
}

// ── Error handling ──────────────────────────────────────────────────────────

// parseResponsesHTTPError converts an HTTP error from the Responses API into an *types.APIError.
func parseResponsesHTTPError(status int, body []byte, retryAfter ...string) *types.APIError {
	errType := "api_error"
	errCode := ""
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
		if errResp.Error.Code != nil {
			errCode = strings.ToLower(strings.TrimSpace(fmt.Sprint(errResp.Error.Code)))
		}
		param := strings.ToLower(errResp.Error.Param)
		if strings.Contains(errCode, "previous_response_not_found") ||
			(strings.Contains(param, "previous_response") && strings.Contains(errCode, "not_found")) {
			errType = "previous_response_not_found"
		}
		if errCode == "context_length_exceeded" {
			errType = "context_length_exceeded"
		}
	}

	apiErr := &types.APIError{
		Status: status, Type: errType, Code: errCode, Message: message,
		Stage: types.ProviderErrorStageHeaders,
	}
	if len(retryAfter) > 0 {
		apiErr.RetryAfter = strings.TrimSpace(retryAfter[0])
	}
	return apiErr
}
