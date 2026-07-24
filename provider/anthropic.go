package provider

import (
	"bufio"
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

const (
	// streamEventBufferSize is the capacity of the buffered channel used to
	// deliver stream events from the background goroutine to the consumer.
	streamEventBufferSize = 64

	// minThinkingBudget is the Anthropic API minimum for extended thinking.
	minThinkingBudget = 1024

	toolSearchBetaHeader1P = "advanced-tool-use-2025-11-20"
)

// AnthropicProvider wraps the official Anthropic SDK as a Provider
type AnthropicProvider struct {
	client  anthropic.Client
	model   string
	baseURL string
}

// NewAnthropic creates a Provider backed by the Anthropic API
func NewAnthropic(cfg Config) *AnthropicProvider {
	var opts []option.RequestOption
	for _, key := range sortedHeaderKeys(cfg.Headers) {
		if strings.EqualFold(key, "anthropic-beta") {
			continue
		}
		opts = append(opts, option.WithHeader(key, cfg.Headers[key]))
	}

	authToken := strings.TrimSpace(cfg.AuthToken)
	apiKey := strings.TrimSpace(cfg.APIKey)
	if authToken != "" {
		opts = append(opts, option.WithHeaderDel("X-Api-Key"))
		opts = append(opts, option.WithAuthToken(authToken))
	} else if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	if betaHeader := mergedAnthropicBetaHeader(cfg.Headers["anthropic-beta"]); betaHeader != "" {
		opts = append(opts, option.WithHeader("anthropic-beta", betaHeader))
	}
	if cfg.Timeout > 0 {
		opts = append(opts, option.WithRequestTimeout(time.Duration(cfg.Timeout)*time.Second))
	}
	opts = append(opts, option.WithHTTPClient(newAnthropicHTTPClientFromEnv()))

	model := cfg.Model
	if model == "" {
		model = CatalogDefaultModel("anthropic", "claude-sonnet-5")
	}

	return &AnthropicProvider{
		client:  anthropic.NewClient(opts...),
		model:   model,
		baseURL: cfg.BaseURL,
	}
}

func sortedHeaderKeys(headers map[string]string) []string {
	if len(headers) == 0 {
		return nil
	}
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func newAnthropicHTTPClientFromEnv() *http.Client {
	var out io.Writer
	debugPath, ok := anthropicDebugLogPathFromEnv()
	if ok {
		f, err := openAnthropicDebugLog(debugPath)
		if err != nil {
			slog.Warn(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyProviderDebugOpenFailed, debugPath, err))
		} else {
			slog.Info(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyProviderDebugWriting, debugPath))
			out = f
		}
	}
	return &http.Client{
		Transport: &anthropicDebugTransport{
			base: newAnthropicBaseTransport(),
			out:  out,
		},
	}
}

func newAnthropicBaseTransport() http.RoundTripper {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return http.DefaultTransport
	}
	cloned := base.Clone()
	// Some proxies incorrectly gzip SSE bodies but forget to set
	// Content-Encoding. Avoid advertising gzip support up front.
	cloned.DisableCompression = true
	return cloned
}

func anthropicDebugLogPathFromEnv() (string, bool) {
	if !isTruthyEnv(os.Getenv("ANTHROPIC_DEBUG_SSE")) {
		return "", false
	}
	if path := strings.TrimSpace(os.Getenv("ANTHROPIC_DEBUG_SSE_FILE")); path != "" {
		return path, true
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "anthropic-sse.log", true
	}
	return filepath.Join(home, brand.ConfigDirName, "logs", "anthropic-sse.log"), true
}

func openAnthropicDebugLog(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
}

func isTruthyEnv(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func mergedAnthropicBetaHeader(existing string) string {
	if isTruthyEnv(os.Getenv("CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS")) {
		return strings.TrimSpace(existing)
	}

	value := strings.TrimSpace(os.Getenv("ENABLE_TOOL_SEARCH"))
	toolSearchEnabled := value == "" || isTruthyEnv(value) || strings.EqualFold(value, "auto") || strings.HasPrefix(strings.ToLower(value), "auto:")
	if !toolSearchEnabled || strings.EqualFold(value, "auto:100") || strings.EqualFold(value, "0") || strings.EqualFold(value, "false") || strings.EqualFold(value, "no") || strings.EqualFold(value, "off") {
		return strings.TrimSpace(existing)
	}

	parts := make([]string, 0, 2)
	seen := make(map[string]struct{})
	for _, part := range strings.Split(existing, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		parts = append(parts, part)
	}
	if _, ok := seen[toolSearchBetaHeader1P]; !ok {
		parts = append(parts, toolSearchBetaHeader1P)
	}
	return strings.Join(parts, ",")
}

type anthropicDebugTransport struct {
	base http.RoundTripper
	out  io.Writer
}

func (t *anthropicDebugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	if t.out != nil {
		fmt.Fprintf(t.out, "[anthropic-debug] %s %s\n", req.Method, req.URL.String())
		for _, key := range sortedHeaderKeys(headerMap(req.Header)) {
			fmt.Fprintf(t.out, "[anthropic-debug] > %s: %s\n", key, redactHeaderValue(key, req.Header.Get(key)))
		}
	}

	resp, err := base.RoundTrip(req)
	if err != nil {
		if t.out != nil {
			fmt.Fprintln(t.out, i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyLogAnthropicRequestError, err))
		}
		return nil, err
	}

	normalizedResp, err := normalizeAnthropicResponse(resp, t.out)
	if err != nil {
		if t.out != nil {
			fmt.Fprintln(t.out, i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyLogAnthropicNormalizeError, err))
		}
		return nil, err
	}
	resp = normalizedResp

	if t.out != nil {
		fmt.Fprintf(t.out, "[anthropic-debug] < status: %s\n", resp.Status)
		for _, key := range sortedHeaderKeys(headerMap(resp.Header)) {
			fmt.Fprintf(t.out, "[anthropic-debug] < %s: %s\n", key, resp.Header.Get(key))
		}
	}

	if resp.Body != nil && t.out != nil {
		if shouldDumpDebugBody(resp.Header) {
			resp.Body = &teeReadCloser{
				Reader: newDebugBodyReader(resp.Body, t.out),
				Closer: resp.Body,
			}
		} else {
			fmt.Fprintln(t.out, i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyLogAnthropicBodyOmitted,
				resp.Header.Get("Content-Type"), resp.Header.Get("Content-Encoding")))
		}
	}
	return resp, nil
}

func normalizeAnthropicResponse(resp *http.Response, out io.Writer) (*http.Response, error) {
	if resp == nil || resp.Body == nil {
		return resp, nil
	}
	if strings.TrimSpace(resp.Header.Get("Content-Encoding")) != "" {
		return resp, nil
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type"))), "text/event-stream") {
		return resp, nil
	}
	br := bufio.NewReader(resp.Body)
	reader, err := normalizeEncodedReader(br, resp.Body, out, resp.Header)
	if err != nil {
		return nil, err
	}
	switch r := reader.(type) {
	case io.ReadCloser:
		resp.Body = r
	default:
		resp.Body = &bufferedReadCloser{Reader: r, Closer: resp.Body}
	}
	return resp, nil
}

type bufferedReadCloser struct {
	io.Reader
	io.Closer
}

type multiCloser struct {
	io.Reader
	closers []io.Closer
}

func (m *multiCloser) Close() error {
	var firstErr error
	for _, c := range m.closers {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func normalizeEncodedReader(br *bufio.Reader, src io.Closer, out io.Writer, header http.Header) (io.Reader, error) {
	magic, err := br.Peek(4)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		return nil, err
	}
	if out != nil && len(magic) >= 2 {
		fmt.Fprintln(out, i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyLogAnthropicBodySniff, magic))
	}
	if len(magic) >= 2 {
		if magic[0] == 0x1f && magic[1] == 0x8b {
			gzr, err := gzip.NewReader(br)
			if err != nil {
				return nil, err
			}
			if out != nil {
				fmt.Fprintln(out, i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogAnthropicNormalizedGzip))
			}
			header.Set("Content-Encoding", "gzip")
			return &multiCloser{
				Reader: gzr,
				closers: []io.Closer{
					gzr,
					src,
				},
			}, nil
		}
		if isLikelyZlibHeader(magic[0], magic[1]) {
			zr, err := zlib.NewReader(br)
			if err != nil {
				return nil, err
			}
			if out != nil {
				fmt.Fprintln(out, i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogAnthropicNormalizedZlib))
			}
			header.Set("Content-Encoding", "deflate")
			return &multiCloser{
				Reader: zr,
				closers: []io.Closer{
					zr,
					src,
				},
			}, nil
		}
	}
	return br, nil
}

func isLikelyZlibHeader(b0, b1 byte) bool {
	if b0 != 0x78 {
		return false
	}
	switch b1 {
	case 0x01, 0x5e, 0x9c, 0xda:
		return true
	default:
		return false
	}
}

type teeReadCloser struct {
	io.Reader
	io.Closer
}

func headerMap(header http.Header) map[string]string {
	if len(header) == 0 {
		return nil
	}
	out := make(map[string]string, len(header))
	for key := range header {
		out[key] = header.Get(key)
	}
	return out
}

func redactHeaderValue(key, value string) string {
	switch strings.ToLower(key) {
	case "authorization":
		if value == "" {
			return ""
		}
		return "Bearer [REDACTED]"
	case "x-api-key":
		if value == "" {
			return ""
		}
		return "[REDACTED]"
	default:
		return value
	}
}

func shouldDumpDebugBody(header http.Header) bool {
	encoding := strings.ToLower(strings.TrimSpace(header.Get("Content-Encoding")))
	if encoding != "" && encoding != "identity" {
		return false
	}

	contentType := strings.ToLower(strings.TrimSpace(header.Get("Content-Type")))
	switch {
	case strings.HasPrefix(contentType, "text/event-stream"):
		return true
	case strings.HasPrefix(contentType, "application/json"):
		return true
	case strings.HasPrefix(contentType, "text/plain"):
		return true
	case strings.HasPrefix(contentType, "text/"):
		return true
	default:
		return false
	}
}

func newDebugBodyReader(src io.Reader, out io.Writer) io.Reader {
	return io.TeeReader(src, &debugLineWriter{out: out})
}

type debugLineWriter struct {
	out io.Writer
	buf strings.Builder
}

func (w *debugLineWriter) Write(p []byte) (int, error) {
	for _, b := range p {
		if b == '\n' {
			fmt.Fprintf(w.out, "[anthropic-debug] <body> %s\n", w.buf.String())
			w.buf.Reset()
			continue
		}
		if b != '\r' {
			w.buf.WriteByte(b)
		}
	}
	return len(p), nil
}

func (p *AnthropicProvider) Name() string    { return "anthropic" }
func (p *AnthropicProvider) ModelID() string { return p.model }

// CountTokens uses Anthropic's token-count endpoint for near-limit Read
// validation. The content is sent as one user text block, matching the TS
// countTokensWithAPI path for file content.
func (p *AnthropicProvider) CountTokens(ctx context.Context, content string) (int, error) {
	if p == nil {
		return 0, i18n.NewError(i18n.KeyProviderAnthropicUnavailable)
	}
	count, err := p.client.Messages.CountTokens(ctx, anthropic.MessageCountTokensParams{
		Model: anthropic.Model(p.model),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(content)),
		},
	})
	if err != nil {
		return 0, err
	}
	if count == nil || count.InputTokens < 0 {
		return 0, i18n.NewError(i18n.KeyProviderTokenCountInvalid)
	}
	return int(count.InputTokens), nil
}

// Capabilities implements CapabilityProvider for AnthropicProvider.
func (p *AnthropicProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		Thinking:     true,
		ToolUse:      true,
		CacheControl: true,
		SystemParts:  true,
		Vision:       true,
		MaxContext:   LookupMaxContext(p.model),
	}
}

func (p *AnthropicProvider) CreateStream(ctx context.Context, params Params) (<-chan types.StreamEvent, error) {
	if params.Model == "" {
		params.Model = p.model
	}
	if params.PromptCacheTTL == "" {
		params.PromptCacheTTL = anthropicPromptCacheTTL("anthropic", params.Model, p.baseURL)
	}
	return createAnthropicStream(ctx, &p.client, params)
}

// createAnthropicStream is the shared streaming implementation used by AnthropicProvider,
// BedrockProvider, and VertexProvider. The caller must set params.Model before calling.
func createAnthropicStream(ctx context.Context, client *anthropic.Client, params Params) (<-chan types.StreamEvent, error) {
	maxTokens := int64(params.MaxTokens)
	if params.MaxOutputTokensOverride > 0 {
		maxTokens = int64(params.MaxOutputTokensOverride)
	}
	if maxTokens == 0 {
		maxTokens = 16384
	}

	reqParams := anthropic.MessageNewParams{
		Model:     params.Model,
		MaxTokens: maxTokens,
		Messages:  convertToAnthropicMessagesForParams(params),
	}
	if systemBlocks := params.SystemTextBlocks(); len(systemBlocks) > 0 {
		cacheBudget := anthropicSystemCacheBreakpointBudget(reqParams.Messages, params.Tools)
		cacheEligible := 0
		for _, part := range systemBlocks {
			if part.Cache {
				cacheEligible++
			}
		}
		skipCacheMarkers := max(cacheEligible-cacheBudget, 0)
		var sysBlocks []anthropic.TextBlockParam
		for _, part := range systemBlocks {
			block := anthropic.TextBlockParam{Text: part.Text}
			if part.Cache {
				if skipCacheMarkers > 0 {
					skipCacheMarkers--
				} else {
					block.CacheControl = anthropicCacheControl(params.PromptCacheTTL)
				}
			}
			sysBlocks = append(sysBlocks, block)
		}
		reqParams.System = sysBlocks
	}
	if params.Thinking != nil && params.Thinking.Enabled {
		budgetTokens := int64(params.Thinking.BudgetTokens)
		if budgetTokens < minThinkingBudget {
			budgetTokens = minThinkingBudget
		}
		reqParams.Thinking = anthropic.ThinkingConfigParamOfEnabled(budgetTokens)
	}
	if taskBudget := taskBudgetBody(params.TaskBudget); len(taskBudget) > 0 {
		outputConfig := anthropic.OutputConfigParam{}
		outputConfig.SetExtraFields(map[string]any{
			"task_budget": taskBudget,
		})
		reqParams.OutputConfig = outputConfig
	}
	if len(params.Tools) > 0 || len(params.ExtraToolSchemas) > 0 {
		tools, err := convertToAnthropicTools(params.Tools, params.PromptCacheTTL)
		if err != nil {
			return nil, i18n.WrapError(i18n.KeyProviderToolsConvertFailed, err)
		}
		serverTools, err := convertToAnthropicServerTools(params.ExtraToolSchemas)
		if err != nil {
			return nil, i18n.WrapError(i18n.KeyProviderServerToolsConvertFailed, err)
		}
		tools = append(tools, serverTools...)
		reqParams.Tools = tools
	}
	if params.ToolChoice != nil {
		switch params.ToolChoice.Type {
		case "any":
			reqParams.ToolChoice = anthropic.ToolChoiceUnionParam{OfAny: &anthropic.ToolChoiceAnyParam{}}
		case "tool":
			reqParams.ToolChoice = anthropic.ToolChoiceUnionParam{OfTool: &anthropic.ToolChoiceToolParam{Name: params.ToolChoice.Name}}
		default: // "auto"
			reqParams.ToolChoice = anthropic.ToolChoiceUnionParam{OfAuto: &anthropic.ToolChoiceAutoParam{}}
		}
	}

	requestOptions := anthropicServerToolRequestOptions(params.ExtraToolSchemas)
	stream := client.Messages.NewStreaming(ctx, reqParams, requestOptions...)

	// Derive a child context so that Close() on the returned channel (via
	// cancel) can unblock the goroutine even if the caller never cancels the
	// parent context. The goroutine also calls defer cancel() when it exits
	// naturally, ensuring the child context is always cleaned up.
	streamCtx, cancel := context.WithCancel(ctx)

	ch := make(chan types.StreamEvent, streamEventBufferSize)
	go func() {
		defer cancel()
		defer close(ch)
		defer stream.Close()
		summary, streamErr := processAnthropicStream(streamCtx, stream, ch)
		if streamCtx.Err() != nil {
			return
		}
		if shouldFallbackAnthropicStream(summary, streamErr) {
			if fallbackErr := emitAnthropicNonStreamingFallback(streamCtx, client, reqParams, requestOptions, ch); fallbackErr == nil {
				return
			} else if streamErr == nil {
				streamErr = fallbackErr
			}
		}
		if streamErr != nil {
			sendEvent(streamCtx, ch, types.StreamEvent{
				Type:  types.EventError,
				Error: parseAnthropicStreamError(streamErr),
			})
		}
	}()

	return ch, nil
}

const anthropicMaxCacheControlBreakpoints = 4

func anthropicSystemCacheBreakpointBudget(messages []anthropic.MessageParam, tools []types.ToolDefinition) int {
	reserved := 0
	if len(messages) > 0 {
		reserved++
	}
	if len(tools) > 0 {
		reserved++
	}
	return max(anthropicMaxCacheControlBreakpoints-reserved, 0)
}

func anthropicCacheControl(ttls ...string) anthropic.CacheControlEphemeralParam {
	control := anthropic.NewCacheControlEphemeralParam()
	if len(ttls) > 0 && strings.EqualFold(strings.TrimSpace(ttls[0]), "1h") {
		control.TTL = anthropic.CacheControlEphemeralTTLTTL1h
	}
	return control
}

// sendEvent sends an event to the channel, respecting context cancellation.
// Returns false if the context is cancelled.
func sendEvent(ctx context.Context, ch chan<- types.StreamEvent, evt types.StreamEvent) bool {
	select {
	case ch <- evt:
		return true
	case <-ctx.Done():
		return false
	}
}

type anthropicStreamSummary struct {
	messageStarted     bool
	contentBlockClosed bool
	messageStopped     bool
}

func processAnthropicStream(ctx context.Context, stream *ssestream.Stream[anthropic.MessageStreamEventUnion], ch chan<- types.StreamEvent) (anthropicStreamSummary, error) {
	var summary anthropicStreamSummary
	for stream.Next() {
		if ctx.Err() != nil {
			return summary, nil
		}

		event := stream.Current()

		switch e := event.AsAny().(type) {
		case anthropic.MessageStartEvent:
			summary.messageStarted = true
			evt := types.StreamEvent{
				Type:    types.EventMessageStart,
				Message: &types.APIMessage{Role: types.RoleAssistant},
			}
			// Capture input token usage from the initial message
			usage := anthropicUsageToTypes(e.Message.Usage)
			if usageHasValues(usage) {
				evt.Usage = usage
			}
			if !sendEvent(ctx, ch, evt) {
				return summary, nil
			}

		case anthropic.ContentBlockStartEvent:
			idx := int(e.Index)
			cb := anthropicStreamContentBlock(e.ContentBlock)
			if !sendEvent(ctx, ch, types.StreamEvent{
				Type:         types.EventContentBlockStart,
				Index:        idx,
				ContentBlock: cb,
			}) {
				return summary, nil
			}

		case anthropic.ContentBlockDeltaEvent:
			idx := int(e.Index)
			delta := &types.ContentDelta{}
			switch d := e.Delta.AsAny().(type) {
			case anthropic.TextDelta:
				delta.Type = "text_delta"
				delta.Text = d.Text
			case anthropic.InputJSONDelta:
				delta.Type = "input_json_delta"
				delta.PartialJSON = d.PartialJSON
			case anthropic.ThinkingDelta:
				delta.Type = "thinking_delta"
				delta.Thinking = d.Thinking
			}
			if !sendEvent(ctx, ch, types.StreamEvent{
				Type:  types.EventContentBlockDelta,
				Index: idx,
				Delta: delta,
			}) {
				return summary, nil
			}

		case anthropic.ContentBlockStopEvent:
			summary.contentBlockClosed = true
			if !sendEvent(ctx, ch, types.StreamEvent{
				Type:  types.EventContentBlockStop,
				Index: int(e.Index),
			}) {
				return summary, nil
			}

		case anthropic.MessageDeltaEvent:
			sr := types.StopReason(e.Delta.StopReason)
			evt := types.StreamEvent{
				Type:       types.EventMessageDelta,
				StopReason: &sr,
			}
			usage := &types.Usage{
				InputTokens:              anthropicTotalInputTokens(e.Usage.InputTokens, e.Usage.CacheCreationInputTokens, e.Usage.CacheReadInputTokens),
				OutputTokens:             int(e.Usage.OutputTokens),
				CacheCreationInputTokens: int(e.Usage.CacheCreationInputTokens),
				CacheReadInputTokens:     int(e.Usage.CacheReadInputTokens),
				ServerToolUse: types.ServerToolUsage{
					WebSearchRequests: int(e.Usage.ServerToolUse.WebSearchRequests),
					WebFetchRequests:  int(e.Usage.ServerToolUse.WebFetchRequests),
				},
			}
			if usageHasValues(usage) {
				evt.Usage = usage
			}
			if !sendEvent(ctx, ch, evt) {
				return summary, nil
			}

		case anthropic.MessageStopEvent:
			summary.messageStopped = true
			if !sendEvent(ctx, ch, types.StreamEvent{Type: types.EventMessageStop}) {
				return summary, nil
			}
			_ = e
		}
	}

	if err := stream.Err(); err != nil {
		return summary, err
	}
	return summary, nil
}

func shouldFallbackAnthropicStream(summary anthropicStreamSummary, streamErr error) bool {
	if streamErr != nil {
		return !summary.contentBlockClosed
	}
	if !summary.messageStarted {
		return true
	}
	return !summary.contentBlockClosed && !summary.messageStopped
}

func emitAnthropicNonStreamingFallback(
	ctx context.Context,
	client *anthropic.Client,
	reqParams anthropic.MessageNewParams,
	requestOptions []option.RequestOption,
	ch chan<- types.StreamEvent,
) error {
	msg, err := client.Messages.New(ctx, reqParams, requestOptions...)
	if err != nil {
		return err
	}

	start := types.StreamEvent{
		Type:    types.EventMessageStart,
		Message: &types.APIMessage{Role: types.RoleAssistant},
		Usage:   anthropicUsageToTypes(msg.Usage),
	}
	if !sendEvent(ctx, ch, start) {
		return nil
	}

	for i, block := range msg.Content {
		contentBlock, delta, ok := anthropicBlockToEvents(block)
		if !ok {
			continue
		}
		if !sendEvent(ctx, ch, types.StreamEvent{
			Type:         types.EventContentBlockStart,
			Index:        i,
			ContentBlock: contentBlock,
		}) {
			return nil
		}
		if delta != nil {
			if !sendEvent(ctx, ch, types.StreamEvent{
				Type:  types.EventContentBlockDelta,
				Index: i,
				Delta: delta,
			}) {
				return nil
			}
		}
		if !sendEvent(ctx, ch, types.StreamEvent{
			Type:  types.EventContentBlockStop,
			Index: i,
		}) {
			return nil
		}
	}

	stopReason := types.StopReason(msg.StopReason)
	if !sendEvent(ctx, ch, types.StreamEvent{
		Type:       types.EventMessageDelta,
		StopReason: &stopReason,
		Usage: &types.Usage{
			OutputTokens: int(msg.Usage.OutputTokens),
		},
	}) {
		return nil
	}
	sendEvent(ctx, ch, types.StreamEvent{Type: types.EventMessageStop})
	return nil
}

func anthropicUsageToTypes(u anthropic.Usage) *types.Usage {
	return &types.Usage{
		InputTokens:              anthropicTotalInputTokens(u.InputTokens, u.CacheCreationInputTokens, u.CacheReadInputTokens),
		OutputTokens:             int(u.OutputTokens),
		CacheCreationInputTokens: int(u.CacheCreationInputTokens),
		CacheReadInputTokens:     int(u.CacheReadInputTokens),
		ServerToolUse: types.ServerToolUsage{
			WebSearchRequests: int(u.ServerToolUse.WebSearchRequests),
			WebFetchRequests:  int(u.ServerToolUse.WebFetchRequests),
		},
	}
}

func anthropicTotalInputTokens(input, cacheCreation, cacheRead int64) int {
	total := int(input) + int(cacheCreation) + int(cacheRead)
	if total < 0 {
		return 0
	}
	return total
}

func usageHasValues(usage *types.Usage) bool {
	return usage != nil && (usage.InputTokens != 0 || usage.OutputTokens != 0 ||
		usage.CacheCreationInputTokens != 0 || usage.CacheReadInputTokens != 0 ||
		usage.ServerToolUse.WebSearchRequests != 0 || usage.ServerToolUse.WebFetchRequests != 0)
}

func rawContentDelta(blockType types.ContentType, raw string) *types.ContentDelta {
	return &types.ContentDelta{Type: blockType, RawJSON: append(json.RawMessage(nil), raw...)}
}

func anthropicStreamContentBlock(block anthropic.ContentBlockStartEventContentBlockUnion) *types.ContentDelta {
	switch variant := block.AsAny().(type) {
	case anthropic.TextBlock:
		return &types.ContentDelta{Type: types.ContentTypeText}
	case anthropic.ToolUseBlock:
		return &types.ContentDelta{Type: types.ContentTypeToolUse, ID: variant.ID, Name: variant.Name}
	case anthropic.ThinkingBlock:
		return &types.ContentDelta{Type: types.ContentTypeThinking, Signature: variant.Signature}
	case anthropic.ServerToolUseBlock:
		delta := rawContentDelta(types.ContentTypeServerToolUse, variant.RawJSON())
		delta.ID = variant.ID
		delta.Name = string(variant.Name)
		return delta
	case anthropic.WebSearchToolResultBlock:
		delta := rawContentDelta(types.ContentTypeWebSearchToolResult, variant.RawJSON())
		delta.ToolUseID = variant.ToolUseID
		return delta
	default:
		if block.Type != "" {
			return rawContentDelta(types.ContentType(block.Type), block.RawJSON())
		}
		return &types.ContentDelta{}
	}
}

func anthropicBlockToEvents(block anthropic.ContentBlockUnion) (*types.ContentDelta, *types.ContentDelta, bool) {
	switch b := block.AsAny().(type) {
	case anthropic.TextBlock:
		return &types.ContentDelta{Type: types.ContentTypeText}, &types.ContentDelta{Type: "text_delta", Text: b.Text}, true
	case anthropic.ToolUseBlock:
		return &types.ContentDelta{Type: types.ContentTypeToolUse, ID: b.ID, Name: b.Name}, &types.ContentDelta{Type: "input_json_delta", PartialJSON: string(b.Input)}, true
	case anthropic.ThinkingBlock:
		return &types.ContentDelta{Type: types.ContentTypeThinking, Signature: b.Signature}, &types.ContentDelta{Type: "thinking_delta", Thinking: b.Thinking}, true
	case anthropic.ServerToolUseBlock:
		start := rawContentDelta(types.ContentTypeServerToolUse, b.RawJSON())
		start.ID, start.Name = b.ID, string(b.Name)
		input, _ := json.Marshal(b.Input)
		return start, &types.ContentDelta{Type: "input_json_delta", PartialJSON: string(input)}, true
	case anthropic.WebSearchToolResultBlock:
		start := rawContentDelta(types.ContentTypeWebSearchToolResult, b.RawJSON())
		start.ToolUseID = b.ToolUseID
		return start, nil, true
	default:
		if block.Type != "" {
			return rawContentDelta(types.ContentType(block.Type), block.RawJSON()), nil, true
		}
		return nil, nil, false
	}
}

// parseAnthropicStreamError converts an Anthropic SDK error into our typed APIError.
// This enables downstream retry logic to inspect status codes and error types.
func parseAnthropicStreamError(err error) *types.APIError {
	var sdkErr *anthropic.Error
	if errors.As(err, &sdkErr) {
		retryAfter := ""
		if sdkErr.Response != nil {
			retryAfter = sdkErr.Response.Header.Get("retry-after")
		}
		errType := "api_error"
		switch sdkErr.StatusCode {
		case 429:
			errType = "rate_limit_error"
		case 529:
			errType = "overloaded_error"
		}
		return &types.APIError{
			Status:     sdkErr.StatusCode,
			Type:       errType,
			Message:    sdkErr.Error(),
			RetryAfter: retryAfter,
		}
	}
	return &types.APIError{
		Type:    "stream_error",
		Message: err.Error(),
	}
}

// convertToAnthropicMessages converts internal messages to SDK format
func convertToAnthropicMessages(msgs []types.Message, cacheTTLs ...string) []anthropic.MessageParam {
	params := Params{Messages: msgs}
	if len(cacheTTLs) > 0 {
		params.PromptCacheTTL = cacheTTLs[0]
	}
	return convertToAnthropicMessagesForParams(params)
}

func convertToAnthropicMessagesForParams(params Params) []anthropic.MessageParam {
	var result []anthropic.MessageParam
	seenCacheEditRefs := make(map[string]struct{})
	var pendingDeveloperReminders []anthropic.ContentBlockParamUnion
	flushDeveloperReminders := func() {
		if len(pendingDeveloperReminders) == 0 {
			return
		}
		result = append(result, anthropic.NewUserMessage(pendingDeveloperReminders...))
		pendingDeveloperReminders = nil
	}
	for _, msg := range params.Messages {
		role := msg.Role
		if role == types.RoleDeveloper && !params.isTrustedDeveloperMessage(msg) {
			// Role is an SDK-controlled descriptor. Preserve untrusted content as
			// ordinary user data; never wrap it as a privileged reminder.
			role = types.RoleUser
		}
		switch role {
		case types.RoleDeveloper:
			if text := msg.GetText(); text != "" {
				pendingDeveloperReminders = append(pendingDeveloperReminders, anthropic.NewTextBlock(
					"<system-reminder>\n"+text+"\n</system-reminder>",
				))
			}
		case types.RoleUser:
			var parts []anthropic.ContentBlockParamUnion
			hasToolResult := false
			for _, block := range msg.Content {
				switch b := block.(type) {
				case types.TextBlock:
					parts = append(parts, anthropic.NewTextBlock(b.Text))
				case types.ToolResultBlock:
					hasToolResult = true
					if b.HasStructuredContent() {
						parts = append(parts, anthropicToolResultBlock(b.ToolUseID, b.IsError, convertToolResultContentToAnthropic(b.ContentBlocks)))
					} else {
						parts = append(parts, anthropicToolResultBlock(b.ToolUseID, b.IsError, []anthropic.ToolResultBlockParamContentUnion{
							{OfText: &anthropic.TextBlockParam{Text: b.Content}},
						}))
					}
				case types.UnknownBlock:
					if b.Type == compactCacheEditsContentType && len(b.Raw) > 0 {
						if raw, ok := deduplicateCacheEditsRaw(b.Raw, seenCacheEditRefs); ok {
							parts = append(parts, anthropicRawContentBlock(raw))
						}
					}
				case types.ImageBlock:
					if b.Source == nil || b.Source.Data == "" {
						continue
					}
					parts = append(parts, anthropic.NewImageBlock(anthropic.Base64ImageSourceParam{
						Data:      b.Source.Data,
						MediaType: anthropic.Base64ImageSourceMediaType(b.Source.MediaType),
					}))
				case types.DocumentBlock:
					if b.Source == nil || b.Source.Data == "" {
						continue
					}
					if b.Source.MediaType == "" || b.Source.MediaType == "application/pdf" {
						parts = append(parts, anthropic.NewDocumentBlock(anthropic.Base64PDFSourceParam{
							Data: b.Source.Data,
						}))
					}
				}
			}
			if len(parts) > 0 {
				if hasToolResult {
					// Anthropic requires tool_result blocks to immediately follow
					// assistant tool_use blocks and to precede all user text. Keep a
					// catalog reminder in the same user turn, after the results.
					parts = append(parts, pendingDeveloperReminders...)
					pendingDeveloperReminders = nil
				} else {
					parts = append(pendingDeveloperReminders, parts...)
					pendingDeveloperReminders = nil
				}
				result = append(result, anthropic.NewUserMessage(parts...))
			}
		case types.RoleAssistant:
			flushDeveloperReminders()
			var parts []anthropic.ContentBlockParamUnion
			for _, block := range msg.Content {
				switch b := block.(type) {
				case types.TextBlock:
					parts = append(parts, anthropic.NewTextBlock(b.Text))
				case types.ToolUseBlock:
					parts = append(parts, anthropic.NewToolUseBlock(b.ID, b.Input, b.Name))
				case types.ThinkingBlock:
					parts = append(parts, anthropic.NewThinkingBlock(b.Signature, b.Thinking))
				case types.UnknownBlock:
					if len(b.Raw) > 0 {
						parts = append(parts, anthropicRawContentBlock(b.Raw))
					}
				}
			}
			if len(parts) > 0 {
				result = append(result, anthropic.NewAssistantMessage(parts...))
			}
		}
	}
	flushDeveloperReminders()
	// Set cache breakpoint on the last cache-eligible content block of the last message.
	// Not all block types support cache_control (e.g. ThinkingBlock, RedactedThinkingBlock
	// return nil from GetCacheControl). Walk backward to find a block that supports it.
	if len(result) > 0 {
		lastMsg := &result[len(result)-1]
		for i := len(lastMsg.Content) - 1; i >= 0; i-- {
			block := &lastMsg.Content[i]
			if cc := block.GetCacheControl(); cc != nil {
				*cc = anthropicCacheControl(params.PromptCacheTTL)
				break
			}
		}
	}
	return addAnthropicCacheReferences(result)
}

const compactCacheEditsContentType types.ContentType = "cache_edits"

type anthropicCacheEditsBlock struct {
	Type  string `json:"type"`
	Edits []struct {
		Type           string `json:"type"`
		CacheReference string `json:"cache_reference"`
	} `json:"edits"`
}

func deduplicateCacheEditsRaw(raw []byte, seen map[string]struct{}) ([]byte, bool) {
	var block anthropicCacheEditsBlock
	if err := json.Unmarshal(raw, &block); err != nil || block.Type != string(compactCacheEditsContentType) {
		return raw, true
	}
	if seen == nil {
		seen = make(map[string]struct{})
	}
	edits := block.Edits[:0]
	for _, edit := range block.Edits {
		if edit.CacheReference == "" {
			continue
		}
		if _, ok := seen[edit.CacheReference]; ok {
			continue
		}
		seen[edit.CacheReference] = struct{}{}
		edits = append(edits, edit)
	}
	if len(edits) == 0 {
		return nil, false
	}
	block.Edits = edits
	out, err := json.Marshal(block)
	if err != nil {
		return raw, true
	}
	return out, true
}

func anthropicToolResultBlock(toolUseID string, isError bool, content []anthropic.ToolResultBlockParamContentUnion) anthropic.ContentBlockParamUnion {
	block := anthropic.ToolResultBlockParam{
		ToolUseID: toolUseID,
		IsError:   anthropic.Bool(isError),
		Content:   content,
	}
	return anthropic.ContentBlockParamUnion{OfToolResult: &block}
}

func anthropicRawContentBlock(raw []byte) anthropic.ContentBlockParamUnion {
	return param.Override[anthropic.ContentBlockParamUnion](json.RawMessage(raw))
}

func addAnthropicCacheReferences(messages []anthropic.MessageParam) []anthropic.MessageParam {
	lastCacheControlMessage := -1
	for i, msg := range messages {
		for _, block := range msg.Content {
			if cc := block.GetCacheControl(); cc != nil {
				lastCacheControlMessage = i
			}
		}
	}
	if lastCacheControlMessage <= 0 {
		return messages
	}
	out := append([]anthropic.MessageParam(nil), messages...)
	for i := 0; i < lastCacheControlMessage; i++ {
		msg := out[i]
		if msg.Role != "user" {
			continue
		}
		content := append([]anthropic.ContentBlockParamUnion(nil), msg.Content...)
		changed := false
		for j := range content {
			if content[j].OfToolResult == nil || content[j].OfToolResult.ToolUseID == "" {
				continue
			}
			toolResult := *content[j].OfToolResult
			toolResult.SetExtraFields(map[string]any{"cache_reference": toolResult.ToolUseID})
			content[j] = anthropic.ContentBlockParamUnion{OfToolResult: &toolResult}
			changed = true
		}
		if changed {
			msg.Content = content
			out[i] = msg
		}
	}
	return out
}

func convertToolResultContentToAnthropic(blocks []types.ContentBlock) []anthropic.ToolResultBlockParamContentUnion {
	content := make([]anthropic.ToolResultBlockParamContentUnion, 0, len(blocks))
	for _, block := range blocks {
		switch typed := block.(type) {
		case types.TextBlock:
			content = append(content, anthropic.ToolResultBlockParamContentUnion{
				OfText: &anthropic.TextBlockParam{
					Text: typed.Text,
				},
			})
		case types.ImageBlock:
			if typed.Source == nil || typed.Source.Data == "" {
				continue
			}
			content = append(content, anthropic.ToolResultBlockParamContentUnion{
				OfImage: &anthropic.ImageBlockParam{
					Source: anthropic.ImageBlockParamSourceUnion{
						OfBase64: &anthropic.Base64ImageSourceParam{
							Data:      typed.Source.Data,
							MediaType: anthropic.Base64ImageSourceMediaType(typed.Source.MediaType),
						},
					},
				},
			})
		case types.DocumentBlock:
			if typed.Source == nil || typed.Source.Data == "" {
				continue
			}
			content = append(content, anthropic.ToolResultBlockParamContentUnion{
				OfDocument: &anthropic.DocumentBlockParam{
					Source: anthropic.DocumentBlockParamSourceUnion{
						OfBase64: &anthropic.Base64PDFSourceParam{
							Data: typed.Source.Data,
						},
					},
				},
			})
		case types.ToolReferenceBlock:
			if typed.ToolName == "" {
				continue
			}
			content = append(content, anthropic.ToolResultBlockParamContentUnion{
				OfToolReference: &anthropic.ToolReferenceBlockParam{
					ToolName: typed.ToolName,
				},
			})
		}
	}
	return content
}

// convertToAnthropicTools converts tool definitions to SDK format
func convertToAnthropicTools(tools []types.ToolDefinition, cacheTTLs ...string) ([]anthropic.ToolUnionParam, error) {
	var result []anthropic.ToolUnionParam
	for _, t := range canonicalToolDefinitions(tools) {
		schemaBytes, err := json.Marshal(t.InputSchema)
		if err != nil {
			return nil, i18n.WrapError(i18n.KeyProviderToolSchemaEncodeFailed, err, t.Name)
		}
		var schemaMap map[string]interface{}
		if err := json.Unmarshal(schemaBytes, &schemaMap); err != nil {
			return nil, i18n.WrapError(i18n.KeyProviderToolSchemaDecodeFailed, err, t.Name)
		}

		// Build the input schema, promoting known top-level fields to their
		// typed slots and forwarding everything else via ExtraFields so that
		// fields like additionalProperties, items, enum, etc. are preserved.
		inputSchema := anthropic.ToolInputSchemaParam{
			Properties: schemaMap["properties"],
		}
		if req, ok := schemaMap["required"].([]interface{}); ok {
			required := make([]string, 0, len(req))
			for _, r := range req {
				if s, ok := r.(string); ok {
					required = append(required, s)
				}
			}
			inputSchema.Required = required
		}

		// Pass through any remaining fields (additionalProperties, items,
		// enum, $defs, etc.) that are not natively typed on ToolInputSchemaParam.
		knownFields := map[string]bool{"type": true, "properties": true, "required": true}
		extras := make(map[string]any, len(schemaMap))
		for k, v := range schemaMap {
			if !knownFields[k] {
				extras[k] = v
			}
		}
		if len(extras) > 0 {
			inputSchema.ExtraFields = extras
		}

		tool := anthropic.ToolUnionParamOfTool(inputSchema, t.Name)
		if tool.OfTool != nil {
			tool.OfTool.Description = anthropic.String(t.Description)
			if t.Strict {
				tool.OfTool.Strict = anthropic.Bool(true)
			}
		}
		result = append(result, tool)
	}
	// Set cache breakpoint on the last tool definition
	if len(result) > 0 {
		last := &result[len(result)-1]
		if last.OfTool != nil {
			last.OfTool.CacheControl = anthropicCacheControl(cacheTTLs...)
		}
	}
	return result, nil
}

const anthropicWebSearchBetaHeader = "web-search-2025-03-05"

// convertToAnthropicServerTools converts only explicitly typed server schemas.
// Ordinary ToolDefinition values never enter this path, even when named
// "web_search".
func convertToAnthropicServerTools(schemas []types.ServerToolDefinition) ([]anthropic.ToolUnionParam, error) {
	result := make([]anthropic.ToolUnionParam, 0, len(schemas))
	for _, schema := range canonicalServerToolDefinitions(schemas) {
		typeName := strings.TrimSpace(schema.Type)
		name := strings.TrimSpace(schema.Name)
		switch typeName {
		case "web_search_20250305":
			if name != "" && name != "web_search" {
				return nil, i18n.NewError(i18n.KeyProviderServerToolNameInvalid, typeName)
			}
			if len(schema.AllowedDomains) > 0 && len(schema.BlockedDomains) > 0 {
				return nil, i18n.NewError(i18n.KeyProviderServerToolDomainsConflict, typeName)
			}
			if schema.MaxUses < 0 {
				return nil, i18n.NewError(i18n.KeyProviderServerToolMaxUsesInvalid, typeName)
			}
			tool := &anthropic.WebSearchTool20250305Param{
				AllowedDomains: append([]string(nil), schema.AllowedDomains...),
				BlockedDomains: append([]string(nil), schema.BlockedDomains...),
			}
			if schema.MaxUses > 0 {
				tool.MaxUses = param.NewOpt(int64(schema.MaxUses))
			}
			result = append(result, anthropic.ToolUnionParam{OfWebSearchTool20250305: tool})
		default:
			return nil, i18n.NewError(i18n.KeyProviderServerToolTypeUnsupported, typeName)
		}
	}
	return result, nil
}

func anthropicServerToolRequestOptions(schemas []types.ServerToolDefinition) []option.RequestOption {
	for _, schema := range schemas {
		if strings.TrimSpace(schema.Type) == "web_search_20250305" {
			return []option.RequestOption{option.WithHeader("anthropic-beta", anthropicWebSearchBetaHeader)}
		}
	}
	return nil
}
