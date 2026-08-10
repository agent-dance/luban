package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	openai "github.com/sashabaranov/go-openai"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/prompt"
	"github.com/agent-dance/luban/types"
)

// OpenAIDialect identifies the dialect of an OpenAI-compatible API.
// Different services have minor behavioural quirks; this enum lets us
// branch on them without breaking the core OpenAI flow.
type OpenAIDialect string

const (
	DialectStandard OpenAIDialect = ""         // standard OpenAI
	DialectGemini   OpenAIDialect = "gemini"   // Google Gemini OpenAI compat
	DialectMistral  OpenAIDialect = "mistral"  // Mistral
	DialectGroq     OpenAIDialect = "groq"     // Groq
	DialectDeepSeek OpenAIDialect = "deepseek" // DeepSeek
	DialectOllama   OpenAIDialect = "ollama"   // Ollama local
)

// OpenAIProvider implements Provider for OpenAI-compatible APIs
// Works with: OpenAI, DeepSeek, Ollama, vLLM, LiteLLM, Azure OpenAI, etc.
type OpenAIProvider struct {
	client                     *openai.Client
	name                       string
	baseURL                    string
	model                      string
	maxTokens                  int
	dialect                    OpenAIDialect
	disableStrictTools         bool
	officialOpenAIChatEndpoint bool
	cacheRouting               CacheRoutingMode
	cacheUserNamespace         string
	cacheRoutingShards         int
	cacheRoutingRejections     *cacheRoutingRejectionMemory
}

type openAIChatDeveloperProjection uint8

const (
	openAIChatDeveloperAsUserReminder openAIChatDeveloperProjection = iota
	openAIChatDeveloperNative
)

// headerTransport injects custom headers into every outgoing request.
type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}
	return t.base.RoundTrip(req)
}

// noAuthTransport strips the Authorization header. Used for local servers
// like Ollama that don't require authentication.
type noAuthTransport struct {
	base http.RoundTripper
}

func (t *noAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Del("Authorization")
	return t.base.RoundTrip(req)
}

type openAIChatRequestContextKey struct{}
type retryAfterCaptureContextKey struct{}

type retryAfterCapture struct {
	mu        sync.Mutex
	value     string
	requestID string
}

func (c *retryAfterCapture) set(value string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.value = strings.TrimSpace(value)
	c.mu.Unlock()
}

func (c *retryAfterCapture) get() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}

func (c *retryAfterCapture) setRequestID(value string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.requestID = strings.TrimSpace(value)
	c.mu.Unlock()
}

func (c *retryAfterCapture) getRequestID() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.requestID
}

// retryAfterCaptureTransport retains only the Retry-After response field. The
// go-openai SDK preserves status/body errors but discards response headers.
type retryAfterCaptureTransport struct{ base http.RoundTripper }

func (t *retryAfterCaptureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(req)
	capture, _ := req.Context().Value(retryAfterCaptureContextKey{}).(*retryAfterCapture)
	if response == nil {
		capture.set("")
		capture.setRequestID("")
	} else {
		capture.set(response.Header.Get("Retry-After"))
		capture.setRequestID(providerRequestID(response.Header))
	}
	return response, err
}

type openAIChatRequestExtensions struct {
	CacheRouting      CacheRoutingMode
	CacheRoutingKey   string
	CacheMemoryKey    string
	PromptCachePolicy openAIPromptCachePolicy
	SystemBlocks      []prompt.SystemPromptBlock
	ThinkingType      string
	UseMaxTokens      bool
}

type cacheRoutingRejectionMemory struct {
	rejected sync.Map
	ttl      time.Duration
	now      func() time.Time
}

func (m *cacheRoutingRejectionMemory) has(key string) bool {
	if m == nil {
		return false
	}
	expiresAtValue, found := m.rejected.Load(key)
	if !found {
		return false
	}
	expiresAt, ok := expiresAtValue.(time.Time)
	if !ok || !m.currentTime().Before(expiresAt) {
		m.rejected.Delete(key)
		return false
	}
	return true
}

func (m *cacheRoutingRejectionMemory) remember(key string) {
	if m == nil {
		return
	}
	ttl := m.ttl
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	m.rejected.Store(key, m.currentTime().Add(ttl))
}

func (m *cacheRoutingRejectionMemory) currentTime() time.Time {
	if m != nil && m.now != nil {
		return m.now()
	}
	return time.Now()
}

// openAIChatRequestTransport adds compatible request fields that are not
// represented by go-openai's ChatCompletionRequest. Values travel through the
// request context so concurrent streams can safely use different cache
// lineages on one provider client.
type openAIChatRequestTransport struct {
	base                   http.RoundTripper
	cacheRoutingRejections *cacheRoutingRejectionMemory
}

func (t *openAIChatRequestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	extensions, _ := req.Context().Value(openAIChatRequestContextKey{}).(openAIChatRequestExtensions)
	if (extensions.CacheRoutingKey == "" && !extensions.PromptCachePolicy.Options && extensions.PromptCachePolicy.Retention == "" && extensions.ThinkingType == "" && !extensions.UseMaxTokens) || req.Body == nil || !strings.HasSuffix(req.URL.Path, "/chat/completions") {
		return t.base.RoundTrip(req)
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyProviderRequestBuildFailed, err)
	}
	_ = req.Body.Close()
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, i18n.WrapError(i18n.KeyProviderRequestBuildFailed, err)
	}
	if extensions.CacheRoutingKey != "" {
		encodedCacheRoutingKey, encodeErr := json.Marshal(extensions.CacheRoutingKey)
		if encodeErr != nil {
			return nil, i18n.WrapError(i18n.KeyProviderRequestEncodeFailed, encodeErr)
		}
		switch extensions.CacheRouting {
		case CacheRoutingPromptCacheKey, CacheRoutingPromptCacheKeyBestEffort:
			payload["prompt_cache_key"] = encodedCacheRoutingKey
		case CacheRoutingDeepSeekUserID:
			payload["user_id"] = encodedCacheRoutingKey
		}
	}
	if err := applyOpenAIPromptCachePolicyRaw(payload, extensions.PromptCachePolicy); err != nil {
		return nil, i18n.WrapError(i18n.KeyProviderRequestEncodeFailed, err)
	}
	if extensions.PromptCachePolicy.Options {
		if err := applyOpenAIChatSystemCacheBreakpoint(payload, extensions.SystemBlocks); err != nil {
			return nil, i18n.WrapError(i18n.KeyProviderRequestEncodeFailed, err)
		}
	}
	if extensions.ThinkingType != "" {
		encodedThinking, encodeErr := json.Marshal(map[string]string{"type": extensions.ThinkingType})
		if encodeErr != nil {
			return nil, i18n.WrapError(i18n.KeyProviderRequestEncodeFailed, encodeErr)
		}
		payload["thinking"] = encodedThinking
	}
	if extensions.UseMaxTokens {
		if maxTokens, ok := payload["max_completion_tokens"]; ok {
			payload["max_tokens"] = maxTokens
			delete(payload, "max_completion_tokens")
		}
	}
	body, err = json.Marshal(payload)
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyProviderRequestEncodeFailed, err)
	}
	fallbackBody := body
	if extensions.CacheRouting == CacheRoutingPromptCacheKeyBestEffort && extensions.CacheRoutingKey != "" {
		delete(payload, "prompt_cache_key")
		fallbackBody, err = json.Marshal(payload)
		if err != nil {
			return nil, i18n.WrapError(i18n.KeyProviderRequestEncodeFailed, err)
		}
	}

	response, err := t.base.RoundTrip(cloneRequestWithBody(req, body))
	if err != nil || extensions.CacheRouting != CacheRoutingPromptCacheKeyBestEffort || extensions.CacheRoutingKey == "" || response == nil || (response.StatusCode != http.StatusBadRequest && response.StatusCode != http.StatusUnprocessableEntity) {
		return response, err
	}
	// Some compatible APIs reject unknown OpenAI fields instead of ignoring
	// them. Only best-effort providers take this guarded, pre-stream retry; APIs
	// that document prompt_cache_key surface the rejection unchanged.
	responseBody, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	response.Body = io.NopCloser(bytes.NewReader(responseBody))
	if readErr != nil || !explicitlyRejectsPromptCacheKey(responseBody) {
		return response, nil
	}
	t.cacheRoutingRejections.remember(extensions.CacheMemoryKey)
	_ = response.Body.Close()
	cause := parseResponsesHTTPError(response.StatusCode, responseBody)
	if attemptErr := beginNestedTransportAttempt(req.Context(), cause); attemptErr != nil {
		return nil, attemptErr
	}
	return t.base.RoundTrip(cloneRequestWithBody(req, fallbackBody))
}

func explicitlyRejectsPromptCacheKey(responseBody []byte) bool {
	lower := strings.ToLower(string(responseBody))
	if !strings.Contains(lower, "prompt_cache_key") {
		return false
	}
	var payload any
	if json.Unmarshal(responseBody, &payload) == nil {
		return promptCacheRejectionNode(payload, false)
	}
	return promptCacheUnsupportedMessage(lower, false)
}

func promptCacheRejectionNode(value any, inheritedFieldMatch bool) bool {
	switch typed := value.(type) {
	case map[string]any:
		fieldMatch := inheritedFieldMatch || promptCacheRejectionFieldMatch(typed)
		if fieldMatch && promptCacheRejectionCodeMatch(typed) {
			return true
		}
		for _, key := range []string{"message", "msg", "detail", "error_description"} {
			if message, ok := typed[key].(string); ok && promptCacheUnsupportedMessage(strings.ToLower(message), fieldMatch) {
				return true
			}
		}
		for _, nested := range typed {
			if promptCacheRejectionNode(nested, fieldMatch) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if promptCacheRejectionNode(nested, inheritedFieldMatch) {
				return true
			}
		}
	}
	return false
}

func promptCacheRejectionFieldMatch(value map[string]any) bool {
	for _, key := range []string{"param", "parameter", "field"} {
		if field, ok := value[key].(string); ok && strings.EqualFold(strings.TrimSpace(field), "prompt_cache_key") {
			return true
		}
	}
	if location, ok := value["loc"].([]any); ok {
		for _, part := range location {
			if field, ok := part.(string); ok && strings.EqualFold(strings.TrimSpace(field), "prompt_cache_key") {
				return true
			}
		}
	}
	return false
}

func promptCacheRejectionCodeMatch(value map[string]any) bool {
	for _, key := range []string{"code", "type"} {
		code, ok := value[key].(string)
		if !ok {
			continue
		}
		code = strings.ToLower(strings.TrimSpace(code))
		for _, marker := range []string{"unknown_parameter", "unsupported_parameter", "unrecognized_parameter", "unexpected_parameter", "unknown_field", "unsupported_field", "extra_forbidden"} {
			if code == marker {
				return true
			}
		}
	}
	return false
}

func promptCacheUnsupportedMessage(lower string, fieldMatch bool) bool {
	for _, phrase := range []string{
		"prompt_cache_key is not supported",
		"prompt_cache_key is unsupported",
		"does not support prompt_cache_key",
		"unsupported prompt_cache_key",
		"unsupported parameter: prompt_cache_key",
		"unknown parameter: prompt_cache_key",
		"unknown field \"prompt_cache_key\"",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	if !fieldMatch {
		return false
	}
	for _, marker := range []string{
		"unsupported parameter",
		"unsupported field",
		"parameter is not supported",
		"field is not supported",
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
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func cloneRequestWithBody(req *http.Request, body []byte) *http.Request {
	cloned := req.Clone(req.Context())
	cloned.Body = io.NopCloser(bytes.NewReader(body))
	cloned.ContentLength = int64(len(body))
	cloned.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	return cloned
}

func deepSeekCacheUserID(cacheLineageID string) string {
	value := strings.TrimSpace(cacheLineageID)
	if value == "" {
		return ""
	}
	valid := len(value) <= 512
	for index := 0; valid && index < len(value); index++ {
		char := value[index]
		valid = (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_' || char == '-'
	}
	if valid {
		return value
	}
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("cache_%x", digest)
}

// NewOpenAI creates a Provider for OpenAI-compatible APIs.
func NewOpenAI(cfg Config) *OpenAIProvider {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
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

	dialect := detectDialect(cfg)
	providerName := CanonicalProviderName(cfg.ProviderName)
	officialOpenAIChatEndpoint := providerName == "openai" && !isCustomOpenAIBaseURL(baseURL)
	// OpenAI, Mistral, and Kimi document prompt_cache_key. Other compatible
	// providers still receive it as a best-effort fallback per the shared cache
	// lineage policy, with a single guarded retry when the field is rejected.
	cacheRouting := resolveOpenAIChatCacheRouting(cfg, dialect, baseURL)
	cacheUserNamespace := promptCacheUserNamespace(cfg)
	cacheRoutingRejections := &cacheRoutingRejectionMemory{ttl: 30 * time.Minute}
	var transport http.RoundTripper = http.DefaultTransport
	if len(cfg.Headers) > 0 {
		transport = &headerTransport{base: transport, headers: cfg.Headers}
	}

	bearerToken := cfg.APIKey
	if authToken := strings.TrimSpace(cfg.AuthToken); authToken != "" {
		bearerToken = authToken
	}
	oaiCfg := openai.DefaultConfig(bearerToken)
	// H7: When API key is empty (e.g. Ollama), wrap transport to strip the
	// "Authorization: Bearer " header that go-openai always sends.
	if bearerToken == "" {
		transport = &noAuthTransport{base: transport}
	}
	transport = &openAIChatRequestTransport{base: transport, cacheRoutingRejections: cacheRoutingRejections}
	transport = &retryAfterCaptureTransport{base: transport}
	oaiCfg.BaseURL = baseURL
	oaiCfg.HTTPClient = &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}

	return &OpenAIProvider{
		client:                     openai.NewClientWithConfig(oaiCfg),
		name:                       providerName,
		baseURL:                    baseURL,
		model:                      model,
		maxTokens:                  maxTokens,
		dialect:                    dialect,
		disableStrictTools:         cfg.DisableStrictTools,
		officialOpenAIChatEndpoint: officialOpenAIChatEndpoint,
		cacheRouting:               cacheRouting,
		cacheUserNamespace:         cacheUserNamespace,
		cacheRoutingShards:         promptCacheRoutingShardCount(providerName),
		cacheRoutingRejections:     cacheRoutingRejections,
	}
}

func resolveOpenAIChatCacheRouting(cfg Config, dialect OpenAIDialect, resolvedBaseURL string) CacheRoutingMode {
	if cfg.CacheRoutingPreference == CacheRoutingOff {
		return CacheRoutingNone
	}
	providerName := CanonicalProviderName(cfg.ProviderName)
	hostname := cacheEndpointHostname(resolvedBaseURL)
	var mode CacheRoutingMode
	switch {
	case dialect == DialectDeepSeek || hostname == "api.deepseek.com":
		mode = CacheRoutingDeepSeekUserID
	case ((providerName == "" || providerName == "openai") && !isCustomOpenAIBaseURL(resolvedBaseURL)) ||
		dialect == DialectMistral || providerName == "kimi" ||
		hostname == "api.mistral.ai" || hostname == "api.moonshot.cn":
		mode = CacheRoutingPromptCacheKey
	default:
		mode = CacheRoutingPromptCacheKeyBestEffort
	}
	if cfg.CacheRoutingPreference == CacheRoutingOn && mode == CacheRoutingPromptCacheKeyBestEffort {
		return CacheRoutingPromptCacheKey
	}
	return mode
}

func cacheEndpointHostname(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}

// detectDialect infers the OpenAI dialect from the provider config.
func detectDialect(cfg Config) OpenAIDialect {
	providerName := CanonicalProviderName(cfg.ProviderName)
	switch providerName {
	case "gemini":
		return DialectGemini
	case "mistral":
		return DialectMistral
	case "groq":
		return DialectGroq
	case "deepseek":
		return DialectDeepSeek
	case "ollama":
		return DialectOllama
	}
	if providerName != "" {
		return DialectStandard
	}
	base := strings.ToLower(cfg.BaseURL)
	switch {
	case strings.Contains(base, "generativelanguage.googleapis.com") ||
		strings.Contains(base, "googleapis.com"):
		return DialectGemini
	case strings.Contains(base, "api.mistral.ai"):
		return DialectMistral
	case strings.Contains(base, "api.groq.com"):
		return DialectGroq
	case strings.Contains(base, "api.deepseek.com"):
		return DialectDeepSeek
	case strings.Contains(base, "localhost") || strings.Contains(base, "11434"):
		return DialectOllama
	default:
		return DialectStandard
	}
}

func (p *OpenAIProvider) Name() string {
	if p.name != "" {
		return p.name
	}
	switch p.dialect {
	case DialectDeepSeek:
		return "deepseek"
	case DialectGemini:
		return "gemini"
	case DialectMistral:
		return "mistral"
	case DialectGroq:
		return "groq"
	case DialectOllama:
		return "ollama"
	default:
		return "openai"
	}
}
func (p *OpenAIProvider) ModelID() string { return p.model }

// Capabilities implements CapabilityProvider for OpenAIProvider.
func (p *OpenAIProvider) Capabilities() ProviderCapabilities {
	serviceTier := CapabilityUnsupported
	if p.officialOpenAIChatEndpoint {
		serviceTier = CapabilitySupported
	}
	return ProviderCapabilities{
		Thinking:     p.dialect == DialectDeepSeek,
		ToolUse:      true,
		ServiceTier:  serviceTier,
		CacheControl: false,
		CacheRouting: p.cacheRouting,
		SystemParts:  true,
		Vision:       p.dialect != DialectDeepSeek,
		MaxContext:   LookupMaxContext(p.model),
	}
}

func (p *OpenAIProvider) CreateStream(ctx context.Context, params Params) (<-chan types.StreamEvent, error) {
	if err := ValidateParams(p, params); err != nil {
		return nil, err
	}
	retryAfter := &retryAfterCapture{}
	ctx = context.WithValue(ctx, retryAfterCaptureContextKey{}, retryAfter)
	model := params.Model
	if model == "" {
		model = p.model
	}
	developerProjection := openAIChatDeveloperProjectionFor(p.officialOpenAIChatEndpoint, p.dialect, model)
	msgs := convertMessagesToOpenAIForDialect(params, params.JoinedSystemPrompt(), developerProjection, p.dialect)

	maxTokens := params.MaxTokens
	if params.MaxOutputTokensOverride > 0 {
		maxTokens = params.MaxOutputTokensOverride
	}
	if maxTokens == 0 {
		maxTokens = p.maxTokens
	}

	req := openai.ChatCompletionRequest{
		Model:               model,
		Messages:            msgs,
		MaxCompletionTokens: maxTokens,
		ReasoningEffort:     reasoningEffortForRequest(params.ReasoningEffort),
		Stream:              true,
		StreamOptions: &openai.StreamOptions{
			IncludeUsage: true,
		},
	}
	if params.ServiceTier != "" {
		req.ServiceTier = openai.ServiceTier(params.ServiceTier)
	}
	if tools := convertToolsToOpenAIWithStrictMode(params.Tools, !p.disableStrictTools); len(tools) > 0 {
		req.Tools = tools
	}
	if params.ToolChoice != nil {
		switch params.ToolChoice.Type {
		case "any":
			req.ToolChoice = "required"
		case "tool":
			req.ToolChoice = openai.ToolChoice{
				Type: openai.ToolTypeFunction,
				Function: openai.ToolFunction{
					Name: params.ToolChoice.Name,
				},
			}
		default: // "auto"
			req.ToolChoice = "auto"
		}
	}
	extensions := openAIChatRequestExtensions{}
	// Most OpenAI-compatible Chat endpoints implement the long-standing
	// max_tokens field. Keep max_completion_tokens for the first-party API and
	// translate it at the transport boundary everywhere else.
	if !p.officialOpenAIChatEndpoint {
		extensions.UseMaxTokens = true
	}
	if params.UsePromptCache && params.PromptCacheKey != "" && p.cacheRouting != CacheRoutingNone {
		cacheMemoryKey := model
		if p.cacheRouting != CacheRoutingPromptCacheKeyBestEffort || !p.cacheRoutingRejections.has(cacheMemoryKey) {
			extensions.CacheRouting = p.cacheRouting
			extensions.CacheRoutingKey = scopedPromptCacheKey(
				p.cacheUserNamespace,
				params.PromptCacheKey,
				model,
				p.cacheRoutingShards,
			)
			extensions.CacheMemoryKey = cacheMemoryKey
			if p.cacheRouting == CacheRoutingDeepSeekUserID {
				// DeepSeek user_id is the KV-cache privacy boundary. Keep it
				// credential-stable across sessions and models when available.
				if p.cacheUserNamespace != "" {
					extensions.CacheRoutingKey = p.cacheUserNamespace
				}
				extensions.CacheRoutingKey = deepSeekCacheUserID(extensions.CacheRoutingKey)
			}
		}
	}
	if p.officialOpenAIChatEndpoint && extensions.CacheRoutingKey != "" {
		extensions.PromptCachePolicy = promptCachePolicyForOpenAIModel(model)
		if extensions.PromptCachePolicy.Options {
			extensions.SystemBlocks = params.SystemTextBlocks()
		}
	}
	if p.dialect == DialectDeepSeek {
		extensions.ThinkingType = "enabled"
		extensions.UseMaxTokens = true
		if params.Thinking != nil && !params.Thinking.Enabled {
			extensions.ThinkingType = "disabled"
		}
	}
	if extensions.CacheRoutingKey != "" || extensions.PromptCachePolicy.Options || extensions.PromptCachePolicy.Retention != "" || extensions.ThinkingType != "" || extensions.UseMaxTokens {
		ctx = context.WithValue(ctx, openAIChatRequestContextKey{}, extensions)
	}

	stream, err := p.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		apiErr := parseOpenAIError(err)
		apiErr.RetryAfter = retryAfter.get()
		annotateProviderRequestError(apiErr, p.name, "chat-completions", p.baseURL+"/chat/completions", nil)
		apiErr.RequestID = retryAfter.getRequestID()
		return nil, i18n.WrapError(i18n.KeyProviderStreamCreateFailed, apiErr)
	}

	ch := make(chan types.StreamEvent, 64)
	go func() {
		defer close(ch)
		defer stream.Close()
		processStreamWithDialect(ctx, stream, ch, p.dialect)
	}()

	return ch, nil
}

// processStreamWithDialect reads chunks from the go-openai stream and emits types.StreamEvent values,
// applying dialect-specific transformations (e.g. DeepSeek <think> tag parsing).
func processStreamWithDialect(ctx context.Context, stream *openai.ChatCompletionStream, ch chan<- types.StreamEvent, dialect OpenAIDialect) {
	// sendEvent sends an event to the channel, respecting context cancellation.
	send := func(evt types.StreamEvent) bool {
		select {
		case ch <- evt:
			return true
		case <-ctx.Done():
			return false
		}
	}

	// Emit the mandatory message_start event.
	if !send(types.StreamEvent{
		Type: types.EventMessageStart,
	}) {
		return
	}

	// Track per-index tool-call state so we can emit block-start once per tool call.
	type toolAcc struct {
		id, name string
		blockIdx int // actual block index assigned to this tool call
	}
	toolCalls := make(map[int]*toolAcc)
	var textStarted bool

	// DeepSeek <think> tag state machine
	inThinkTag := false
	nativeReasoning := false
	thinkBlockIdx := -1 // block index for the current thinking block
	nextBlockIdx := 0   // next available block index
	textBlockIdx := 0   // block index of the currently open text block
	carry := ""         // leftover from previous chunk that might contain a partial tag
	systemFingerprint := ""

	for {
		if ctx.Err() != nil {
			return
		}

		raw, err := stream.RecvRaw()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			send(types.StreamEvent{
				Type:  types.EventError,
				Error: parseOpenAIError(err, types.ProviderErrorStageStream),
			})
			return
		}
		resp, usage, err := decodeOpenAIStreamChunk(raw, dialect)
		if err != nil {
			send(types.StreamEvent{
				Type: types.EventError,
				Error: &types.APIError{
					Type: "parse_error",
					Message: i18n.Format(
						i18n.DetectOrLoadLanguage(),
						i18n.KeyProviderOpenAIStreamChunkParseFailed,
						err,
					),
				},
			})
			return
		}
		if resp.SystemFingerprint != "" {
			systemFingerprint = resp.SystemFingerprint
		}

		// Usage-only chunk (OpenAI sends this as the final data frame when IncludeUsage is set).
		if usage != nil {
			if !send(types.StreamEvent{
				Type:              types.EventMessageDelta,
				Usage:             usage,
				SystemFingerprint: systemFingerprint,
			}) {
				return
			}
		}

		if len(resp.Choices) == 0 {
			continue
		}

		choice := resp.Choices[0]
		delta := choice.Delta
		if dialect == DialectDeepSeek && delta.ReasoningContent != "" {
			if textStarted {
				if !send(types.StreamEvent{Type: types.EventContentBlockStop, Index: textBlockIdx}) {
					return
				}
				textStarted = false
			}
			if !nativeReasoning {
				thinkBlockIdx = nextBlockIdx
				nextBlockIdx++
				if !send(types.StreamEvent{
					Type:         types.EventContentBlockStart,
					Index:        thinkBlockIdx,
					ContentBlock: &types.ContentDelta{Type: types.ContentTypeThinking},
				}) {
					return
				}
				nativeReasoning = true
			}
			if !send(types.StreamEvent{
				Type:  types.EventContentBlockDelta,
				Index: thinkBlockIdx,
				Delta: &types.ContentDelta{Type: "thinking_delta", Thinking: delta.ReasoningContent},
			}) {
				return
			}
		}

		// ── Text content ──────────────────────────────────────────────────────
		if delta.Content != "" {
			if nativeReasoning {
				if !send(types.StreamEvent{Type: types.EventContentBlockStop, Index: thinkBlockIdx}) {
					return
				}
				nativeReasoning = false
			}
			content := delta.Content

			// DeepSeek <think> tag handling: parse thinking content into ThinkingBlocks.
			// Uses a carry buffer to handle tags split across SSE chunks
			// (e.g. "<thi" + "nk>" arriving in separate frames).
			if dialect == DialectDeepSeek {
				content = carry + content
				carry = ""

				for content != "" {
					if inThinkTag {
						if idx := strings.Index(content, "</think>"); idx >= 0 {
							if idx > 0 {
								if !send(types.StreamEvent{
									Type:  types.EventContentBlockDelta,
									Index: thinkBlockIdx,
									Delta: &types.ContentDelta{Type: "thinking_delta", Thinking: content[:idx]},
								}) {
									return
								}
							}
							if !send(types.StreamEvent{Type: types.EventContentBlockStop, Index: thinkBlockIdx}) {
								return
							}
							inThinkTag = false
							content = content[idx+len("</think>"):]
						} else {
							// Check for partial closing tag at end (e.g. "</thi")
							safe, remainder := splitAtPartialTag(content, "</think>")
							if safe != "" {
								if !send(types.StreamEvent{
									Type:  types.EventContentBlockDelta,
									Index: thinkBlockIdx,
									Delta: &types.ContentDelta{Type: "thinking_delta", Thinking: safe},
								}) {
									return
								}
							}
							carry = remainder
							content = ""
						}
					} else {
						if idx := strings.Index(content, "<think>"); idx >= 0 {
							if idx > 0 {
								if !textStarted {
									if !send(types.StreamEvent{
										Type:         types.EventContentBlockStart,
										Index:        nextBlockIdx,
										ContentBlock: &types.ContentDelta{Type: types.ContentTypeText},
									}) {
										return
									}
									textStarted = true
									textBlockIdx = nextBlockIdx
									nextBlockIdx++
								}
								if !send(types.StreamEvent{
									Type:  types.EventContentBlockDelta,
									Index: textBlockIdx,
									Delta: &types.ContentDelta{Type: "text_delta", Text: content[:idx]},
								}) {
									return
								}
							}
							if textStarted {
								if !send(types.StreamEvent{Type: types.EventContentBlockStop, Index: textBlockIdx}) {
									return
								}
								textStarted = false
							}
							thinkBlockIdx = nextBlockIdx
							nextBlockIdx++
							if !send(types.StreamEvent{
								Type:         types.EventContentBlockStart,
								Index:        thinkBlockIdx,
								ContentBlock: &types.ContentDelta{Type: types.ContentTypeThinking},
							}) {
								return
							}
							inThinkTag = true
							content = content[idx+len("<think>"):]
						} else {
							// Check for partial opening tag at end (e.g. "<thi")
							safe, remainder := splitAtPartialTag(content, "<think>")
							if safe != "" {
								if !textStarted {
									if !send(types.StreamEvent{
										Type:         types.EventContentBlockStart,
										Index:        nextBlockIdx,
										ContentBlock: &types.ContentDelta{Type: types.ContentTypeText},
									}) {
										return
									}
									textStarted = true
									textBlockIdx = nextBlockIdx
									nextBlockIdx++
								}
								if !send(types.StreamEvent{
									Type:  types.EventContentBlockDelta,
									Index: textBlockIdx,
									Delta: &types.ContentDelta{Type: "text_delta", Text: safe},
								}) {
									return
								}
							}
							carry = remainder
							content = ""
						}
					}
				}
			} else {
				// Standard path: no dialect-specific processing
				if !textStarted {
					textBlockIdx = nextBlockIdx
					nextBlockIdx++
					if !send(types.StreamEvent{
						Type:         types.EventContentBlockStart,
						Index:        textBlockIdx,
						ContentBlock: &types.ContentDelta{Type: types.ContentTypeText},
					}) {
						return
					}
					textStarted = true
				}
				if !send(types.StreamEvent{
					Type:  types.EventContentBlockDelta,
					Index: textBlockIdx,
					Delta: &types.ContentDelta{Type: "text_delta", Text: content},
				}) {
					return
				}
			}
		}

		// ── Tool-call deltas ──────────────────────────────────────────────────
		for _, tc := range delta.ToolCalls {
			if nativeReasoning {
				if !send(types.StreamEvent{Type: types.EventContentBlockStop, Index: thinkBlockIdx}) {
					return
				}
				nativeReasoning = false
			}
			if inThinkTag {
				if !send(types.StreamEvent{Type: types.EventContentBlockStop, Index: thinkBlockIdx}) {
					return
				}
				inThinkTag = false
			}
			idx := 0
			if tc.Index != nil {
				idx = *tc.Index
			}

			if _, exists := toolCalls[idx]; !exists {
				// First delta for this tool call: close any open text block, then
				// announce a new tool_use content block.
				if textStarted {
					if !send(types.StreamEvent{Type: types.EventContentBlockStop, Index: textBlockIdx}) {
						return
					}
					textStarted = false
				}
				blockIdx := nextBlockIdx
				nextBlockIdx = blockIdx + 1
				toolCalls[idx] = &toolAcc{id: tc.ID, name: tc.Function.Name, blockIdx: blockIdx}
				if !send(types.StreamEvent{
					Type:  types.EventContentBlockStart,
					Index: blockIdx,
					ContentBlock: &types.ContentDelta{
						Type: types.ContentTypeToolUse,
						ID:   tc.ID,
						Name: tc.Function.Name,
					},
				}) {
					return
				}
			}

			// Stream argument JSON fragments.
			if tc.Function.Arguments != "" {
				ta := toolCalls[idx]
				if !send(types.StreamEvent{
					Type:  types.EventContentBlockDelta,
					Index: ta.blockIdx,
					Delta: &types.ContentDelta{
						Type:        "input_json_delta",
						PartialJSON: tc.Function.Arguments,
					},
				}) {
					return
				}
			}
		}

		// ── Finish reason ─────────────────────────────────────────────────────
		// go-openai uses "" for non-final chunks and "null" for explicit JSON null.
		if fr := choice.FinishReason; fr != "" && fr != "null" {
			if nativeReasoning {
				if !send(types.StreamEvent{Type: types.EventContentBlockStop, Index: thinkBlockIdx}) {
					return
				}
				nativeReasoning = false
			}
			if inThinkTag {
				if !send(types.StreamEvent{Type: types.EventContentBlockStop, Index: thinkBlockIdx}) {
					return
				}
				inThinkTag = false
			}
			if textStarted {
				if !send(types.StreamEvent{Type: types.EventContentBlockStop, Index: textBlockIdx}) {
					return
				}
			}
			for _, ta := range toolCalls {
				if !send(types.StreamEvent{Type: types.EventContentBlockStop, Index: ta.blockIdx}) {
					return
				}
			}

			sr := types.StopReasonEndTurn
			switch fr {
			case openai.FinishReasonToolCalls:
				sr = types.StopReasonToolUse
			case openai.FinishReasonLength:
				sr = types.StopReasonMaxTokens
			}
			if !send(types.StreamEvent{Type: types.EventMessageDelta, StopReason: &sr}) {
				return
			}
		}
	}

	// Flush any remaining carry buffer content (partial tag that turned out to be just text)
	if dialect == DialectDeepSeek && carry != "" {
		if inThinkTag {
			send(types.StreamEvent{
				Type:  types.EventContentBlockDelta,
				Index: thinkBlockIdx,
				Delta: &types.ContentDelta{Type: "thinking_delta", Thinking: carry},
			})
		} else {
			if !textStarted {
				send(types.StreamEvent{
					Type:         types.EventContentBlockStart,
					Index:        nextBlockIdx,
					ContentBlock: &types.ContentDelta{Type: types.ContentTypeText},
				})
				textStarted = true
				textBlockIdx = nextBlockIdx
			}
			send(types.StreamEvent{
				Type:  types.EventContentBlockDelta,
				Index: textBlockIdx,
				Delta: &types.ContentDelta{Type: "text_delta", Text: carry},
			})
		}
	}
	if nativeReasoning {
		send(types.StreamEvent{Type: types.EventContentBlockStop, Index: thinkBlockIdx})
	}
	if inThinkTag {
		send(types.StreamEvent{Type: types.EventContentBlockStop, Index: thinkBlockIdx})
	}

	send(types.StreamEvent{Type: types.EventMessageStop, SystemFingerprint: systemFingerprint})
}

// splitAtPartialTag splits content into a safe prefix and a remainder that
// might be a partial tag. For example, if tag is "<think>" and content ends
// with "<thi", safe="everything before <thi", remainder="<thi".
// If no partial tag is possible, remainder is empty and safe == content.
func splitAtPartialTag(content, tag string) (safe, remainder string) {
	// Check if content ends with any prefix of the tag (length 1..len(tag)-1)
	for i := len(tag) - 1; i >= 1; i-- {
		prefix := tag[:i]
		if strings.HasSuffix(content, prefix) {
			return content[:len(content)-i], content[len(content)-i:]
		}
	}
	return content, ""
}

// ── Message conversion ────────────────────────────────────────────────────────

// convertMessagesToOpenAIWithSystemAndDeveloperProjection preserves the
// internal conversation order while projecting developer instructions to the
// strongest role the backend is known to support.
func convertMessagesToOpenAIWithSystemAndDeveloperProjection(
	params Params,
	systemPrompt string,
	developerProjection openAIChatDeveloperProjection,
) []openai.ChatCompletionMessage {
	return convertMessagesToOpenAIForDialect(params, systemPrompt, developerProjection, DialectStandard)
}

func convertMessagesToOpenAIForDialect(
	params Params,
	systemPrompt string,
	developerProjection openAIChatDeveloperProjection,
	dialect OpenAIDialect,
) []openai.ChatCompletionMessage {
	var msgs []openai.ChatCompletionMessage
	var pendingDeveloperReminders []string
	flushDeveloperReminders := func() {
		if len(pendingDeveloperReminders) == 0 {
			return
		}
		msgs = append(msgs, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: strings.Join(pendingDeveloperReminders, "\n\n"),
		})
		pendingDeveloperReminders = nil
	}

	if systemPrompt != "" {
		msgs = append(msgs, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemPrompt,
		})
	}

	for _, msg := range params.Messages {
		role := msg.Role
		if role == types.RoleDeveloper && !params.isTrustedDeveloperMessage(msg) {
			// Public RoleDeveloper values are ordinary data until a runtime
			// capability authenticates the complete message.
			role = types.RoleUser
		}
		switch role {
		case types.RoleUser:
			converted := convertUserMessage(msg)
			if len(pendingDeveloperReminders) > 0 {
				converted = prependOpenAIDeveloperReminders(converted, pendingDeveloperReminders)
				pendingDeveloperReminders = nil
			}
			msgs = append(msgs, converted...)
		case types.RoleAssistant:
			flushDeveloperReminders()
			msgs = append(msgs, convertAssistantMessageForDialect(msg, dialect))
		case types.RoleDeveloper:
			if text := msg.GetText(); text != "" {
				if developerProjection == openAIChatDeveloperNative {
					msgs = append(msgs, openai.ChatCompletionMessage{
						Role:    openai.ChatMessageRoleDeveloper,
						Content: text,
					})
				} else {
					pendingDeveloperReminders = append(pendingDeveloperReminders,
						"<system-reminder>\n"+text+"\n</system-reminder>",
					)
				}
			}
		}
	}
	flushDeveloperReminders()

	return normalizeEmptyOpenAIToolResults(msgs)
}

// openAIChatDeveloperProjectionFor returns the strongest instruction channel
// the current Chat Completions backend is known to accept. Unknown compatible
// backends deliberately use a user reminder instead of a late system message:
// that keeps the stable serialized history prefix intact when the catalog
// revision changes.
func openAIChatDeveloperProjectionFor(officialOpenAIEndpoint bool, dialect OpenAIDialect, model string) openAIChatDeveloperProjection {
	if officialOpenAIEndpoint && dialect == DialectStandard &&
		openAIChatModelSupportsDeveloperRole(model) {
		return openAIChatDeveloperNative
	}
	return openAIChatDeveloperAsUserReminder
}

func openAIChatModelSupportsDeveloperRole(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if strings.HasPrefix(model, "gpt-5") || strings.HasPrefix(model, "chat-latest") {
		return true
	}
	return len(model) >= 2 && model[0] == 'o' && model[1] >= '1' && model[1] <= '9'
}

func prependOpenAIDeveloperReminders(messages []openai.ChatCompletionMessage, reminders []string) []openai.ChatCompletionMessage {
	reminder := strings.Join(reminders, "\n\n")
	for i := range messages {
		if messages[i].Role != openai.ChatMessageRoleUser {
			continue
		}
		if len(messages[i].MultiContent) > 0 {
			messages[i].MultiContent = append([]openai.ChatMessagePart{{
				Type: openai.ChatMessagePartTypeText,
				Text: reminder,
			}}, messages[i].MultiContent...)
		} else if messages[i].Content == "" {
			messages[i].Content = reminder
		} else {
			messages[i].Content = reminder + "\n\n" + messages[i].Content
		}
		return messages
	}
	return append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: reminder,
	})
}

// normalizeEmptyOpenAIToolResults mirrors the original client's API-boundary
// normalization. Empty tool output is valid (for example, a successful silent
// shell command), but go-openai omits an empty content string from JSON and
// strict compatible APIs reject the resulting message.
func normalizeEmptyOpenAIToolResults(messages []openai.ChatCompletionMessage) []openai.ChatCompletionMessage {
	toolNames := make(map[string]string)
	for _, message := range messages {
		for _, call := range message.ToolCalls {
			if call.ID != "" && call.Function.Name != "" {
				toolNames[call.ID] = call.Function.Name
			}
		}
	}
	for i := range messages {
		message := &messages[i]
		if message.Role != openai.ChatMessageRoleTool || strings.TrimSpace(message.Content) != "" {
			continue
		}
		toolName := toolNames[message.ToolCallID]
		if toolName == "" {
			toolName = "Tool"
		}
		message.Content = fmt.Sprintf("(%s completed with no output)", toolName)
	}
	return messages
}

// convertUserMessage handles user messages, splitting tool results into separate
// "tool" role messages as required by the OpenAI API.
func convertUserMessage(msg types.Message) []openai.ChatCompletionMessage {
	var hasToolResults bool
	for _, block := range msg.Content {
		if _, ok := block.(types.ToolResultBlock); ok {
			hasToolResults = true
			break
		}
	}

	if hasToolResults {
		var toolMessages []openai.ChatCompletionMessage
		var followUps []openai.ChatCompletionMessage
		var textParts []string
		var imageParts []types.ImageBlock

		flushUserParts := func() {
			if len(textParts) == 0 && len(imageParts) == 0 {
				return
			}
			followUps = append(followUps, buildOpenAIUserMessage(textParts, imageParts))
			textParts = nil
			imageParts = nil
		}

		for _, block := range msg.Content {
			switch tr := block.(type) {
			case types.ToolResultBlock:
				flushUserParts()
				toolMessages = append(toolMessages, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					Content:    tr.TextContent(),
					ToolCallID: tr.ToolUseID,
				})
				if tr.HasStructuredContent() {
					for _, contentBlock := range tr.ContentBlocks {
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
				textParts = append(textParts, tr.Text)
			case types.ImageBlock:
				imageParts = append(imageParts, tr)
			case types.DocumentBlock:
				textParts = append(textParts, "[document]")
			}
		}
		flushUserParts()
		// OpenAI requires every sibling tool_call to receive a contiguous tool
		// message before any user follow-up. Structured attachments are therefore
		// emitted only after all tool results in this internal user message.
		return append(toolMessages, followUps...)
	}

	var textParts []string
	var imageParts []types.ImageBlock
	for _, block := range msg.Content {
		switch b := block.(type) {
		case types.TextBlock:
			textParts = append(textParts, b.Text)
		case types.ImageBlock:
			imageParts = append(imageParts, b)
		case types.DocumentBlock:
			textParts = append(textParts, "[document]")
		}
	}
	return []openai.ChatCompletionMessage{buildOpenAIUserMessage(textParts, imageParts)}
}

func buildOpenAIUserMessage(textParts []string, imageParts []types.ImageBlock) openai.ChatCompletionMessage {
	var parts []openai.ChatMessagePart
	for _, imagePart := range imageParts {
		if imagePart.Source == nil || imagePart.Source.Data == "" {
			continue
		}
		mediaType := imagePart.Source.MediaType
		if mediaType == "" {
			mediaType = "image/png"
		}
		parts = append(parts, openai.ChatMessagePart{
			Type: openai.ChatMessagePartTypeImageURL,
			ImageURL: &openai.ChatMessageImageURL{
				URL: fmt.Sprintf("data:%s;base64,%s", mediaType, imagePart.Source.Data),
			},
		})
	}
	if len(parts) > 0 {
		if len(textParts) > 0 {
			parts = append([]openai.ChatMessagePart{{
				Type: openai.ChatMessagePartTypeText,
				Text: strings.Join(textParts, "\n"),
			}}, parts...)
		}
		return openai.ChatCompletionMessage{
			Role:         openai.ChatMessageRoleUser,
			MultiContent: parts,
		}
	}
	return openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: strings.Join(textParts, "\n"),
	}
}

// convertAssistantMessage converts an assistant message, including any tool_use
// blocks into OpenAI's tool_calls format.
func convertAssistantMessage(msg types.Message) openai.ChatCompletionMessage {
	return convertAssistantMessageForDialect(msg, DialectStandard)
}

func convertAssistantMessageForDialect(msg types.Message, dialect OpenAIDialect) openai.ChatCompletionMessage {
	oaiMsg := openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: msg.GetText(),
	}
	if dialect == DialectDeepSeek {
		var reasoning []string
		for _, block := range msg.Content {
			switch thinking := block.(type) {
			case types.ThinkingBlock:
				if thinking.Thinking != "" {
					reasoning = append(reasoning, thinking.Thinking)
				}
			case *types.ThinkingBlock:
				if thinking != nil && thinking.Thinking != "" {
					reasoning = append(reasoning, thinking.Thinking)
				}
			}
		}
		oaiMsg.ReasoningContent = strings.Join(reasoning, "\n")
	}

	for _, tu := range msg.GetToolUses() {
		args, err := json.Marshal(tu.Input)
		if err != nil {
			// Skip tool calls that fail to serialize — sending empty args
			// would cause downstream errors that are harder to diagnose.
			continue
		}
		oaiMsg.ToolCalls = append(oaiMsg.ToolCalls, openai.ToolCall{
			ID:   tu.ID,
			Type: openai.ToolTypeFunction,
			Function: openai.FunctionCall{
				Name:      tu.Name,
				Arguments: string(args),
			},
		})
	}

	return oaiMsg
}

// ── Tool conversion ───────────────────────────────────────────────────────────

func convertToolsToOpenAIWithStrictMode(tools []types.ToolDefinition, allowStrict bool) []openai.Tool {
	result := make([]openai.Tool, 0, len(tools))
	for _, t := range canonicalToolDefinitions(tools) {
		schema := t.InputSchema
		// OpenAI requires "properties" on object schemas, even if empty.
		if schema.Properties == nil {
			schema.Properties = map[string]any{}
		}
		result = append(result, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Strict:      allowStrict && t.Strict,
				Parameters:  schema,
			},
		})
	}
	return result
}

// parseOpenAIError converts an error from the go-openai SDK into a *types.APIError
// with a proper HTTP status code so that RetryProvider can correctly classify it.
func parseOpenAIError(err error, stages ...types.ProviderErrorStage) *types.APIError {
	stage := types.ProviderErrorStageConnect
	if len(stages) > 0 {
		stage = stages[0]
	}
	var oaiErr *openai.APIError
	if errors.As(err, &oaiErr) {
		errType := "api_error"
		switch oaiErr.HTTPStatusCode {
		case 429:
			errType = "rate_limit_error"
		case 529, 503:
			errType = "overloaded_error"
		}
		return &types.APIError{
			Status:  oaiErr.HTTPStatusCode,
			Type:    errType,
			Message: oaiErr.Message,
			Stage:   types.ProviderErrorStageHeaders,
		}
	}
	if transportErr, ok := typedTransportAPIError(err, stage); ok {
		return transportErr
	}
	return &types.APIError{Type: "stream_error", Message: err.Error(), Stage: stage}
}
