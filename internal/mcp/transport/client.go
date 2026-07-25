// Package transport owns the MCP JSON-RPC client and concrete transports.
//
// The client owns MCP's JSON-RPC request lifecycle: initialize
// handshake, response correlation, notification/request handlers, connection
// close semantics, and structured result preservation.
package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/buildinfo"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/mcp/catalog"
	"github.com/agent-dance/luban/internal/mcp/protocol"
)

const (
	protocolVersion         = "2024-11-05"
	maxMCPDescriptionLength = 2048

	mcpTruncationSuffix = "\u2026 [truncated]"
	defaultInitTimeout  = 30 * time.Second

	maxCatalogPages = 1_000
	maxCatalogItems = 100_000
)

// NotificationHandler handles a server notification. It is invoked from a
// goroutine so notifications cannot block response processing.
type NotificationHandler func(context.Context, protocol.JSONRPCMessage)

type requestHandler func(context.Context, protocol.JSONRPCMessage) (any, error)

// clientOptions configures a protocol client before initialize. It remains
// private because production callers use one canonical initialization policy.
type clientOptions struct {
	ClientInfo           catalog.ClientInfo
	Capabilities         map[string]any
	InitializeTimeout    time.Duration
	NotificationHandlers map[string]NotificationHandler
	Roots                []catalog.Root
}

type pendingCall struct {
	ch chan rpcResponse
}

type rpcResponse struct {
	result json.RawMessage
	err    error
}

// Client owns a full Transport-backed JSON-RPC connection.
type Client struct {
	transport Transport
	options   clientOptions
	cancel    context.CancelFunc

	nextID atomic.Int64
	sendMu sync.Mutex

	mu                   sync.RWMutex
	pending              map[string]*pendingCall
	notificationHandlers map[string]NotificationHandler
	requestHandlers      map[string]requestHandler
	capabilities         catalog.ServerCapabilities
	serverInfo           *catalog.ServerInfo
	instructions         string
	closeErr             error

	closeOnce sync.Once
	done      chan struct{}
}

func defaultClientInfo() catalog.ClientInfo {
	return catalog.ClientInfo{
		Name:        brand.CommandName,
		Title:       brand.DisplayName,
		Version:     buildinfo.Current("").Fingerprint.Version,
		Description: brand.DisplayName,
	}
}

// defaultClientCapabilities includes an empty elicitation capability because
// Java MCP SDK servers require the object to be present during initialization.
func defaultClientCapabilities() map[string]any {
	return map[string]any{
		"roots":       map[string]any{},
		"elicitation": map[string]any{},
	}
}

// NewClient constructs a Transport-backed client, starts the receive
// pump, performs initialize, stores server metadata, and sends the initialized
// notification.
func NewClient(ctx context.Context, transport Transport) (*Client, error) {
	return newClient(ctx, transport, clientOptions{})
}

func newClient(ctx context.Context, transport Transport, options clientOptions) (*Client, error) {
	if transport == nil {
		return nil, i18n.NewError(i18n.KeyServicesMCPNilTransport)
	}
	clientCtx, cancel := context.WithCancel(context.Background())
	c := &Client{
		transport:            transport,
		options:              normalizeClientOptions(options),
		cancel:               cancel,
		pending:              make(map[string]*pendingCall),
		notificationHandlers: make(map[string]NotificationHandler),
		requestHandlers:      make(map[string]requestHandler),
		done:                 make(chan struct{}),
	}
	c.installDefaultRequestHandlers()
	for method, handler := range c.options.NotificationHandlers {
		c.notificationHandlers[method] = handler
	}

	go c.readLoop(clientCtx)

	if err := c.initialize(ctx); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

// initialize performs the MCP initialize handshake and sends
// notifications/initialized.
func (c *Client) initialize(ctx context.Context) error {
	if c == nil || c.transport == nil {
		return i18n.NewError(i18n.KeyServicesMCPInitializeNeedsTransport)
	}
	initCtx, cancel := withDefaultTimeout(ctx, c.options.InitializeTimeout)
	defer cancel()

	params := map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    cloneMap(c.options.Capabilities),
		"clientInfo":      c.options.ClientInfo,
	}

	var raw json.RawMessage
	if err := c.CallRaw(initCtx, "initialize", params, &raw); err != nil {
		return i18n.WrapError(i18n.KeyServicesMCPInitializeFailed, err)
	}

	var result struct {
		ProtocolVersion string                     `json:"protocolVersion"`
		Capabilities    catalog.ServerCapabilities `json:"capabilities"`
		ServerInfo      *catalog.ServerInfo        `json:"serverInfo"`
		Instructions    string                     `json:"instructions"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &result); err != nil {
			return i18n.WrapError(i18n.KeyServicesMCPDecodeInitialize, err)
		}
	}
	if result.Capabilities == nil {
		result.Capabilities = catalog.ServerCapabilities{}
	}

	c.mu.Lock()
	c.capabilities = result.Capabilities
	c.serverInfo = catalog.CloneServerInfo(result.ServerInfo)
	c.instructions = truncateMCPDescription(result.Instructions)
	c.mu.Unlock()

	if err := c.notify(initCtx, "notifications/initialized", nil); err != nil {
		return i18n.WrapError(i18n.KeyServicesMCPInitializedNotification, err)
	}
	return nil
}

// CallRaw issues a JSON-RPC call and decodes the raw result into out. If out is
// *json.RawMessage, the exact result bytes are preserved.
func (c *Client) CallRaw(ctx context.Context, method string, params any, out any) error {
	if c == nil {
		return i18n.NewError(i18n.KeyServicesMCPNilClient)
	}
	if c.transport == nil {
		return i18n.NewError(i18n.KeyServicesMCPClientNoTransport)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	id := c.nextID.Add(1)
	msg, err := protocol.NewRequestMessage(id, method, params)
	if err != nil {
		return err
	}
	key := strconv.FormatInt(id, 10)
	pending := &pendingCall{ch: make(chan rpcResponse, 1)}

	if err := c.registerPending(key, pending); err != nil {
		return err
	}
	defer c.unregisterPending(key)

	if err := c.send(ctx, msg); err != nil {
		return err
	}

	select {
	case resp := <-pending.ch:
		if resp.err != nil {
			return resp.err
		}
		if out == nil {
			return nil
		}
		if rawOut, ok := out.(*json.RawMessage); ok {
			*rawOut = protocol.CloneRawMessage(resp.result)
			return nil
		}
		if len(bytes.TrimSpace(resp.result)) == 0 {
			return nil
		}
		return json.Unmarshal(resp.result, out)
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return c.closedError()
	}
}

// notify sends a JSON-RPC notification.
func (c *Client) notify(ctx context.Context, method string, params any) error {
	if c == nil || c.transport == nil {
		return i18n.NewError(i18n.KeyServicesMCPNotifyNeedsTransport)
	}
	msg, err := protocol.NewNotificationMessage(method, params)
	if err != nil {
		return err
	}
	return c.send(ctx, msg)
}

// SetNotificationHandler registers or replaces a method-specific notification
// handler. Passing nil removes the handler.
func (c *Client) SetNotificationHandler(method string, handler NotificationHandler) {
	if c == nil || method == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.notificationHandlers == nil {
		c.notificationHandlers = make(map[string]NotificationHandler)
	}
	if handler == nil {
		delete(c.notificationHandlers, method)
		return
	}
	c.notificationHandlers[method] = handler
}

// GetServerCapabilities returns a copy of the initialize capabilities.
func (c *Client) GetServerCapabilities() catalog.ServerCapabilities {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return catalog.CloneCapabilities(c.capabilities)
}

// GetServerInfo returns a copy of the initialize serverInfo.
func (c *Client) GetServerInfo() *catalog.ServerInfo {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return catalog.CloneServerInfo(c.serverInfo)
}

// GetInstructions returns the initialize instructions after TypeScript-parity
// truncation.
func (c *Client) GetInstructions() string {
	if c == nil {
		return ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.instructions
}

// IsClosed reports whether the protocol client has terminated.
func (c *Client) IsClosed() bool {
	if c == nil {
		return true
	}
	if c.transport == nil || c.done == nil {
		return true
	}
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

// Wait blocks until the protocol client terminates and returns its terminal
// transport error.
func (c *Client) Wait() error {
	if c == nil || c.done == nil {
		return NewTransportClosedError(i18n.KeyServicesMCPTransportClientClosedReason, ErrTransportClosed)
	}
	<-c.done
	return c.closedError()
}

// ListTools invokes tools/list and returns the structured result envelope.
func (c *Client) ListTools(ctx context.Context) (*catalog.ListToolsResult, error) {
	return collectCatalogPages(
		ctx,
		c,
		"tools/list",
		func(page *catalog.ListToolsResult) int { return len(page.Tools) },
		func(out, page *catalog.ListToolsResult) {
			out.Tools = append(out.Tools, page.Tools...)
			if out.Meta == nil {
				out.Meta = page.Meta
			}
			out.NextCursor = page.NextCursor
		},
		func(page *catalog.ListToolsResult) string { return page.NextCursor },
	)
}

// catalog.ReadResourceResult invokes resources/read and returns the structured result.
func (c *Client) ReadResourceResult(ctx context.Context, uri string) (*catalog.ReadResourceResult, error) {
	if c == nil {
		return nil, i18n.NewError(i18n.KeyServicesMCPNilClient)
	}
	params := map[string]any{"uri": uri}
	var out catalog.ReadResourceResult
	if err := c.CallRaw(ctx, "resources/read", params, &out); err != nil {
		return nil, i18n.WrapError(i18n.KeyServicesMCPNamedMethodFailed, err, "resources/read", uri)
	}
	return &out, nil
}

// catalog.ListResourcesResult invokes resources/list and returns the structured result.
func (c *Client) ListResourcesResult(ctx context.Context) (*catalog.ListResourcesResult, error) {
	return collectCatalogPages(
		ctx,
		c,
		"resources/list",
		func(page *catalog.ListResourcesResult) int { return len(page.Resources) },
		func(out, page *catalog.ListResourcesResult) {
			out.Resources = append(out.Resources, page.Resources...)
			if out.Meta == nil {
				out.Meta = page.Meta
			}
			out.NextCursor = page.NextCursor
		},
		func(page *catalog.ListResourcesResult) string { return page.NextCursor },
	)
}

// ListPrompts invokes prompts/list and returns the structured result envelope.
func (c *Client) ListPrompts(ctx context.Context) (*catalog.ListPromptsResult, error) {
	return collectCatalogPages(
		ctx,
		c,
		"prompts/list",
		func(page *catalog.ListPromptsResult) int { return len(page.Prompts) },
		func(out, page *catalog.ListPromptsResult) {
			out.Prompts = append(out.Prompts, page.Prompts...)
			if out.Meta == nil {
				out.Meta = page.Meta
			}
			out.NextCursor = page.NextCursor
		},
		func(page *catalog.ListPromptsResult) string { return page.NextCursor },
	)
}

func collectCatalogPages[T any](
	ctx context.Context,
	c *Client,
	method string,
	itemCount func(*T) int,
	merge func(*T, *T),
	nextCursor func(*T) string,
) (*T, error) {
	if c == nil {
		return nil, i18n.NewError(i18n.KeyServicesMCPNilClient)
	}
	var out T
	cursor := ""
	seenCursors := make(map[string]struct{})
	totalItems := 0
	for pageNumber := 1; pageNumber <= maxCatalogPages; pageNumber++ {
		var params any
		if cursor != "" {
			params = map[string]any{"cursor": cursor}
		}
		var page T
		if err := c.CallRaw(ctx, method, params, &page); err != nil {
			return nil, i18n.WrapError(i18n.KeyServicesMCPMethodFailed, err, method)
		}
		pageItems := itemCount(&page)
		if pageItems > maxCatalogItems-totalItems {
			return nil, i18n.NewError(i18n.KeyServicesMCPCatalogItemLimit, method, maxCatalogItems)
		}
		totalItems += pageItems
		merge(&out, &page)

		next := nextCursor(&page)
		if next == "" {
			return &out, nil
		}
		if _, exists := seenCursors[next]; exists {
			return nil, i18n.NewError(i18n.KeyServicesMCPCatalogCursorLoop, method, next)
		}
		seenCursors[next] = struct{}{}
		cursor = next
	}
	return nil, i18n.NewError(i18n.KeyServicesMCPCatalogPageLimit, method, maxCatalogPages)
}

// Close shuts down the client and rejects pending calls with
// ErrTransportClosed.
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	return c.closeWithError(i18n.KeyServicesMCPTransportClientClosedReason, nil)
}

func (c *Client) readLoop(ctx context.Context) {
	for {
		msg, err := c.transport.Receive(ctx)
		if err != nil {
			select {
			case <-c.done:
				return
			default:
			}
			_ = c.closeWithError(i18n.KeyServicesMCPTransportReceiveFailedReason, err)
			return
		}
		c.dispatchMessage(msg)
	}
}

func (c *Client) dispatchMessage(msg protocol.JSONRPCMessage) {
	if len(bytes.TrimSpace(msg.ID)) > 0 && (len(msg.Result) > 0 || msg.Error != nil) {
		c.dispatchResponse(msg)
		return
	}
	if msg.Method == "" {
		return
	}
	if len(bytes.TrimSpace(msg.ID)) > 0 {
		c.dispatchRequest(msg)
		return
	}
	c.dispatchNotification(msg)
}

func (c *Client) dispatchResponse(msg protocol.JSONRPCMessage) {
	key := messageIDKey(msg.ID)
	if key == "" {
		return
	}
	c.mu.RLock()
	pending := c.pending[key]
	c.mu.RUnlock()
	if pending == nil {
		return
	}
	resp := rpcResponse{result: protocol.CloneRawMessage(msg.Result)}
	if msg.Error != nil {
		resp.err = msg.Error
	}
	select {
	case pending.ch <- resp:
	default:
	}
}

func (c *Client) dispatchNotification(msg protocol.JSONRPCMessage) {
	c.mu.RLock()
	handler := c.notificationHandlers[msg.Method]
	c.mu.RUnlock()
	if handler == nil {
		return
	}
	go handler(context.Background(), msg)
}

func (c *Client) dispatchRequest(msg protocol.JSONRPCMessage) {
	c.mu.RLock()
	handler := c.requestHandlers[msg.Method]
	c.mu.RUnlock()
	go func() {
		var resp protocol.JSONRPCMessage
		var err error
		if handler == nil {
			resp, err = protocol.NewErrorMessage(msg.ID, -32601, "Method not found", nil)
		} else {
			result, handlerErr := handler(context.Background(), msg)
			if handlerErr != nil {
				resp, err = protocol.NewErrorMessage(msg.ID, -32603, handlerErr.Error(), nil)
			} else {
				resp, err = protocol.NewResultMessage(msg.ID, result)
			}
		}
		if err != nil {
			return
		}
		_ = c.send(context.Background(), resp)
	}()
}

func (c *Client) registerPending(key string, pending *pendingCall) error {
	select {
	case <-c.done:
		return c.closedError()
	default:
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-c.done:
		return c.closedErrorLocked()
	default:
	}
	c.pending[key] = pending
	return nil
}

func (c *Client) unregisterPending(key string) {
	c.mu.Lock()
	delete(c.pending, key)
	c.mu.Unlock()
}

func (c *Client) send(ctx context.Context, msg protocol.JSONRPCMessage) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-c.done:
		return c.closedError()
	default:
	}
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	select {
	case <-c.done:
		return c.closedError()
	default:
	}
	if err := c.transport.Send(ctx, msg); err != nil {
		return err
	}
	return nil
}

func (c *Client) closeWithError(reasonKey i18n.Key, cause error) error {
	if c == nil {
		return nil
	}
	if c.transport == nil {
		return nil
	}
	var closeErr error
	c.closeOnce.Do(func() {
		if c.cancel != nil {
			c.cancel()
		}
		err := NewTransportClosedError(reasonKey, cause)
		c.mu.Lock()
		c.closeErr = err
		close(c.done)
		pending := c.pending
		c.pending = make(map[string]*pendingCall)
		c.mu.Unlock()

		for _, call := range pending {
			select {
			case call.ch <- rpcResponse{err: err}:
			default:
			}
		}
		closeErr = c.transport.Close()
	})
	return closeErr
}

func (c *Client) closedError() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closedErrorLocked()
}

// closedErrorLocked returns the terminal client error while c.mu is held for
// reading or writing.
func (c *Client) closedErrorLocked() error {
	if c.closeErr != nil {
		return c.closeErr
	}
	return NewTransportClosedError(i18n.KeyServicesMCPTransportClientClosedReason, ErrTransportClosed)
}

func (c *Client) installDefaultRequestHandlers() {
	c.requestHandlers["roots/list"] = func(context.Context, protocol.JSONRPCMessage) (any, error) {
		roots := c.options.Roots
		if len(roots) == 0 {
			roots = []catalog.Root{{URI: defaultRootURI()}}
		}
		return map[string]any{"roots": roots}, nil
	}
	c.requestHandlers["elicitation/create"] = func(context.Context, protocol.JSONRPCMessage) (any, error) {
		return map[string]any{"action": "cancel"}, nil
	}
}

func normalizeClientOptions(options clientOptions) clientOptions {
	if options.ClientInfo.Name == "" {
		defaultInfo := defaultClientInfo()
		if options.ClientInfo.Title != "" {
			defaultInfo.Title = options.ClientInfo.Title
		}
		if options.ClientInfo.Version != "" {
			defaultInfo.Version = options.ClientInfo.Version
		}
		if options.ClientInfo.Description != "" {
			defaultInfo.Description = options.ClientInfo.Description
		}
		if options.ClientInfo.WebsiteURL != "" {
			defaultInfo.WebsiteURL = options.ClientInfo.WebsiteURL
		}
		options.ClientInfo = defaultInfo
	} else if options.ClientInfo.Version == "" {
		options.ClientInfo.Version = "unknown"
	}
	if options.Capabilities == nil {
		options.Capabilities = defaultClientCapabilities()
	}
	if options.InitializeTimeout <= 0 {
		options.InitializeTimeout = defaultInitTimeout
	}
	return options
}

func withDefaultTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func truncateMCPDescription(value string) string {
	runes := []rune(value)
	if len(runes) <= maxMCPDescriptionLength {
		return value
	}
	return string(runes[:maxMCPDescriptionLength]) + mcpTruncationSuffix
}

func defaultRootURI() string {
	cwd, err := os.Getwd()
	if err != nil || cwd == "" {
		return "file://"
	}
	path := filepath.ToSlash(cwd)
	if filepath.VolumeName(cwd) != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func messageIDKey(id json.RawMessage) string {
	trimmed := bytes.TrimSpace(id)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	var decoded any
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	if err := dec.Decode(&decoded); err != nil {
		return string(trimmed)
	}
	switch v := decoded.(type) {
	case json.Number:
		return v.String()
	case string:
		return strconv.Quote(v)
	default:
		return string(trimmed)
	}
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
