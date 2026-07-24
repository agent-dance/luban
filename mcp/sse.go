package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
	svcmcp "github.com/agent-dance/luban/services/mcp"
)

// SSEClient connects to an MCP server over HTTP + Server-Sent Events.
//
// Requests are sent as HTTP POST to <baseURL>/message.
// Responses and server-push events arrive on an SSE stream at <baseURL>/sse.
//
// It implements the same logical interface as the stdio Client
// (ListTools, CallTool, ListResources, ReadResource) so callers can use
// either transport interchangeably.
type SSEClient struct {
	baseURL string
	client  *http.Client
	name    string

	// ctx/cancel govern the entire client lifetime. Cancelled by Close().
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	pending map[int64]chan json.RawMessage // id → response channel

	idCounter atomic.Int64
	closeOnce sync.Once
	closeCh   chan struct{}
}

// NewSSEClient creates an SSEClient and connects the SSE stream.
// The SSE connection is maintained in the background; call Close to shut down.
func NewSSEClient(name, baseURL string, httpClient *http.Client) (*SSEClient, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 0} // SSE stream — no timeout on the GET
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &SSEClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  httpClient,
		name:    name,
		ctx:     ctx,
		cancel:  cancel,
		pending: make(map[int64]chan json.RawMessage),
		closeCh: make(chan struct{}),
	}

	// Start the SSE reader loop.
	go c.sseReadLoop()

	// Perform MCP initialize handshake.
	if err := c.initialize(); err != nil {
		c.Close() //nolint:errcheck
		return nil, i18n.WrapError(i18n.KeyLegacyMCPSSEInitialize, err)
	}

	return c, nil
}

func (c *SSEClient) initialize() error {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": brand.CommandName, "version": "1.0.0"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var result json.RawMessage
	if err := c.call(ctx, "initialize", params, &result); err != nil {
		return err
	}
	// Fire-and-forget initialized notification.
	notifyCtx, notifyCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer notifyCancel()
	return c.notify(notifyCtx, "notifications/initialized", nil)
}

// ListTools discovers tools from the server.
func (c *SSEClient) ListTools() ([]*MCPTool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var allTools []*MCPTool
	var cursor string

	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var resp struct {
			Tools []struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				InputSchema json.RawMessage `json:"inputSchema"`
			} `json:"tools"`
			NextCursor string `json:"nextCursor"`
		}
		if err := c.call(ctx, "tools/list", params, &resp); err != nil {
			return nil, err
		}
		for _, t := range resp.Tools {
			allTools = append(allTools, &MCPTool{
				ToolName:     svcmcp.BuildMCPToolName(c.name, t.Name),
				OriginalName: t.Name,
				ToolDesc:     t.Description,
				InputSchema:  t.InputSchema,
			})
		}
		if resp.NextCursor == "" {
			break
		}
		cursor = resp.NextCursor
	}
	return allTools, nil
}

// CallTool calls a tool on the remote server.
func (c *SSEClient) CallTool(ctx context.Context, name string, arguments map[string]any) (string, error) {
	params := map[string]any{"name": name, "arguments": arguments}
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := c.call(ctx, "tools/call", params, &resp); err != nil {
		return "", err
	}
	if resp.IsError {
		var parts []string
		for _, item := range resp.Content {
			if item.Type == "text" {
				parts = append(parts, item.Text)
			}
		}
		return "", i18n.NewError(i18n.KeyLegacyMCPToolReturnedError, strings.Join(parts, "\n"))
	}
	var parts []string
	for _, item := range resp.Content {
		if item.Type == "text" {
			parts = append(parts, item.Text)
		}
	}
	return strings.Join(parts, "\n"), nil
}

// ListResources lists resources from the server.
func (c *SSEClient) ListResources() ([]MCPResource, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var resp struct {
		Resources []MCPResource `json:"resources"`
	}
	if err := c.call(ctx, "resources/list", map[string]any{}, &resp); err != nil {
		return nil, err
	}
	return resp.Resources, nil
}

// ReadResource reads a resource by URI.
func (c *SSEClient) ReadResource(ctx context.Context, uri string) (string, error) {
	var resp struct {
		Contents []struct {
			URI  string `json:"uri"`
			Text string `json:"text"`
		} `json:"contents"`
	}
	if err := c.call(ctx, "resources/read", map[string]any{"uri": uri}, &resp); err != nil {
		return "", err
	}
	var parts []string
	for _, item := range resp.Contents {
		if item.Text != "" {
			parts = append(parts, item.Text)
		}
	}
	return strings.Join(parts, "\n"), nil
}

// Close shuts down the SSE client.
func (c *SSEClient) Close() error {
	c.closeOnce.Do(func() {
		c.cancel()
		close(c.closeCh)
	})
	return nil
}

// ── internal RPC machinery ────────────────────────────────────────────────────

// call sends a JSON-RPC request over HTTP POST and waits for the response on
// the SSE stream.
func (c *SSEClient) call(ctx context.Context, method string, params any, result any) error {
	id := c.idCounter.Add(1)

	reqBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	respCh := make(chan json.RawMessage, 1)
	c.mu.Lock()
	c.pending[id] = respCh
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/message", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return i18n.WrapError(i18n.KeyLegacyMCPHTTPPost, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return i18n.NewError(i18n.KeyLegacyMCPHTTPStatus, resp.StatusCode, string(body))
	}

	// Some implementations return the JSON-RPC response directly in the POST
	// response body instead of over SSE.
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return err
		}
		return decodeRPCResult(body, result)
	}

	// Otherwise wait for the result to arrive on the SSE channel.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.closeCh:
		return i18n.NewError(i18n.KeyLegacyMCPSSEClientClosed)
	case raw := <-respCh:
		return decodeRPCResult(raw, result)
	}
}

// notify sends a JSON-RPC notification (no id, no response expected).
func (c *SSEClient) notify(ctx context.Context, method string, params any) error {
	reqBody := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/message", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// sseReadLoop connects to the SSE endpoint and dispatches incoming events to
// waiting callers. It reconnects on disconnect with a simple fixed delay.
func (c *SSEClient) sseReadLoop() {
	const reconnectDelay = 2 * time.Second

	for {
		select {
		case <-c.closeCh:
			return
		default:
		}

		err := c.connectAndRead()
		if err != nil {
			slog.Warn(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPStreamReconnect),
				"name", c.name, "error", err, "delay", reconnectDelay)
		}

		select {
		case <-c.closeCh:
			return
		case <-time.After(reconnectDelay):
		}
	}
}

// connectAndRead establishes the SSE GET connection and reads events until EOF
// or an error.
func (c *SSEClient) connectAndRead() error {
	// Derive a child context from the client's lifecycle context so that
	// cancellation (via Close) propagates here automatically, without a
	// separate goroutine to bridge closeCh → cancel.
	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/sse", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return i18n.NewError(i18n.KeyLegacyMCPSSEGetStatus, resp.StatusCode)
	}

	return c.parseSSEStream(resp.Body)
}

// parseSSEStream reads SSE events and dispatches JSON-RPC responses.
func (c *SSEClient) parseSSEStream(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			// Blank line = end of event.
			if len(dataLines) > 0 {
				c.dispatchSSEEvent(strings.Join(dataLines, "\n"))
				dataLines = dataLines[:0]
			}
			continue
		}

		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
		// Ignore other SSE fields (event:, id:, retry:).
	}
	return scanner.Err()
}

// dispatchSSEEvent decodes an SSE data payload as a JSON-RPC response and
// routes it to the appropriate pending caller.
func (c *SSEClient) dispatchSSEEvent(data string) {
	var envelope struct {
		ID     *int64          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(data), &envelope); err != nil {
		slog.Debug(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogMCPUnparseableEvent), "data", data)
		return
	}
	if envelope.ID == nil {
		return // notification — not handled here
	}

	c.mu.Lock()
	ch, ok := c.pending[*envelope.ID]
	c.mu.Unlock()
	if !ok {
		return
	}

	// Re-encode the full response so the receiver can decode.
	raw, _ := json.Marshal(map[string]any{
		"id":     *envelope.ID,
		"result": envelope.Result,
		"error":  envelope.Error,
	})
	select {
	case ch <- raw:
	default:
	}
}

// decodeRPCResult extracts the "result" field from a JSON-RPC response body
// and unmarshals it into out.
func decodeRPCResult(data []byte, out any) error {
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return i18n.WrapError(i18n.KeyLegacyMCPDecodeRPCEnvelope, err)
	}
	if envelope.Error != nil {
		return i18n.NewError(i18n.KeyLegacyMCPRPCError, envelope.Error.Code, envelope.Error.Message)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(envelope.Result, out)
}
