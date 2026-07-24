// Package mcp services-layer client module.
//
// The services client owns MCP's JSON-RPC request lifecycle: initialize
// handshake, response correlation, notification/request handlers, connection
// close semantics, and structured result preservation. Existing lower-level
// callers can still inject a RawCaller through NewClient while new MCP code
// can use NewProtocolClient with a Transport.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agent-dance/luban/i18n"
)

const (
	// MCPProtocolVersion is the protocol version sent by the TypeScript client.
	MCPProtocolVersion = "2024-11-05"

	// MaxMCPDescriptionLength matches the TypeScript cap for tool descriptions
	// and server instructions.
	MaxMCPDescriptionLength = 2048

	mcpTruncationSuffix = "\u2026 [truncated]"
	defaultInitTimeout  = 30 * time.Second
)

// ResourceContent is one item in the contents[] array returned by
// resources/read. The JSON-RPC reply preserves uri / mimeType / text / blob.
type ResourceContent struct {
	URI         string         `json:"uri,omitempty"`
	MimeType    string         `json:"mimeType,omitempty"`
	Text        string         `json:"text,omitempty"`
	Blob        string         `json:"blob,omitempty"`
	Type        string         `json:"type,omitempty"`
	Annotations map[string]any `json:"annotations,omitempty"`
	Meta        map[string]any `json:"_meta,omitempty"`
}

// Resource is one item in the resources[] array returned by resources/list.
type Resource struct {
	URI         string         `json:"uri"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	MimeType    string         `json:"mimeType,omitempty"`
	Annotations map[string]any `json:"annotations,omitempty"`
	Meta        map[string]any `json:"_meta,omitempty"`
}

// ToolCall is the structured envelope returned by tools/call.
type ToolCall struct {
	Content           []ToolContent   `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
	IsError           bool            `json:"isError,omitempty"`
	Meta              map[string]any  `json:"_meta,omitempty"`
}

// ToolContent is one item in the tools/call content[] array.
type ToolContent struct {
	Type        string         `json:"type"`
	Text        string         `json:"text,omitempty"`
	URI         string         `json:"uri,omitempty"`
	MimeType    string         `json:"mimeType,omitempty"`
	Blob        string         `json:"blob,omitempty"`
	Annotations map[string]any `json:"annotations,omitempty"`
	Meta        map[string]any `json:"_meta,omitempty"`
}

// ToolDefinition is one item in tools/list.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
	Annotations map[string]any  `json:"annotations,omitempty"`
	Meta        map[string]any  `json:"_meta,omitempty"`
}

// ListToolsResult is the raw tools/list result envelope.
type ListToolsResult struct {
	Tools      []ToolDefinition `json:"tools"`
	NextCursor string           `json:"nextCursor,omitempty"`
	Meta       map[string]any   `json:"_meta,omitempty"`
}

// ListResourcesResult is the raw resources/list result envelope.
type ListResourcesResult struct {
	Resources  []Resource     `json:"resources"`
	NextCursor string         `json:"nextCursor,omitempty"`
	Meta       map[string]any `json:"_meta,omitempty"`
}

// ReadResourceResult is the raw resources/read result envelope.
type ReadResourceResult struct {
	Contents []ResourceContent `json:"contents"`
	Meta     map[string]any    `json:"_meta,omitempty"`
}

// PromptArgument is one argument definition returned by prompts/list.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// PromptDefinition is one prompt returned by prompts/list.
type PromptDefinition struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
	Meta        map[string]any   `json:"_meta,omitempty"`
}

// ListPromptsResult is the raw prompts/list result envelope.
type ListPromptsResult struct {
	Prompts    []PromptDefinition `json:"prompts"`
	NextCursor string             `json:"nextCursor,omitempty"`
	Meta       map[string]any     `json:"_meta,omitempty"`
}

// ServerCapabilities mirrors the MCP initialize result while preserving
// unknown capability objects.
type ServerCapabilities map[string]any

// ServerInfo is the server version object returned by initialize.
type ServerInfo struct {
	Name    string         `json:"name,omitempty"`
	Version string         `json:"version,omitempty"`
	Meta    map[string]any `json:"_meta,omitempty"`
}

// ClientInfo is the TypeScript-parity clientInfo sent during initialize.
type ClientInfo struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	WebsiteURL  string `json:"websiteUrl,omitempty"`
}

// Root is one roots/list entry.
type Root struct {
	URI  string `json:"uri"`
	Name string `json:"name,omitempty"`
}

// NotificationHandler handles a server notification. It is invoked from a
// goroutine so notifications cannot block response processing.
type NotificationHandler func(context.Context, JSONRPCMessage)

// RequestHandler handles a server request and returns the JSON-RPC result.
type RequestHandler func(context.Context, JSONRPCMessage) (any, error)

// ClientOptions configures a protocol client before initialize.
type ClientOptions struct {
	ClientInfo           ClientInfo
	Capabilities         map[string]any
	InitializeTimeout    time.Duration
	NotificationHandlers map[string]NotificationHandler
	RequestHandlers      map[string]RequestHandler
	Roots                []Root
}

// RawCaller is the legacy minimal contract the services-layer wrapper needs
// from a lower-level client. It returns a JSON-RPC result without further
// coercion so the services layer can surface uri / mimeType verbatim.
type RawCaller interface {
	CallRaw(ctx context.Context, method string, params any, out any) error
}

type pendingCall struct {
	ch chan rpcResponse
}

type rpcResponse struct {
	result json.RawMessage
	err    error
}

// Client is the services-layer MCP client. It can wrap a legacy RawCaller or
// own a full Transport-backed JSON-RPC connection.
type Client struct {
	raw  RawCaller
	auth TokenSource

	transport Transport
	options   ClientOptions
	cancel    context.CancelFunc

	nextID atomic.Int64
	sendMu sync.Mutex

	mu                   sync.RWMutex
	pending              map[string]*pendingCall
	notificationHandlers map[string]NotificationHandler
	requestHandlers      map[string]RequestHandler
	capabilities         ServerCapabilities
	serverInfo           *ServerInfo
	instructions         string
	initializeRaw        json.RawMessage
	initialized          bool
	closeErr             error

	closeOnce sync.Once
	done      chan struct{}
}

// DefaultClientInfo returns the TypeScript clientInfo sent to MCP servers.
func DefaultClientInfo() ClientInfo {
	return ClientInfo{
		Name:        "claude-code",
		Title:       "Claude Code",
		Version:     "unknown",
		Description: "Anthropic's agentic coding tool",
		WebsiteURL:  "https://claude.com/claude-code",
	}
}

// DefaultClientCapabilities returns the TypeScript-parity MCP client
// capabilities. Elicitation is intentionally an empty object for compatibility
// with Java MCP SDK servers, matching the TypeScript implementation.
func DefaultClientCapabilities() map[string]any {
	return map[string]any{
		"roots":       map[string]any{},
		"elicitation": map[string]any{},
	}
}

// NewClient constructs a services-layer wrapper around a legacy RawCaller.
func NewClient(raw RawCaller, auth TokenSource) *Client {
	if auth == nil {
		auth = DefaultTokenSource()
	}
	return &Client{raw: raw, auth: auth}
}

// NewProtocolClient constructs a Transport-backed client, starts the receive
// pump, performs initialize, stores server metadata, and sends the initialized
// notification.
func NewProtocolClient(ctx context.Context, transport Transport, options ClientOptions) (*Client, error) {
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
		requestHandlers:      make(map[string]RequestHandler),
		done:                 make(chan struct{}),
	}
	c.installDefaultRequestHandlers()
	for method, handler := range c.options.NotificationHandlers {
		c.notificationHandlers[method] = handler
	}
	for method, handler := range c.options.RequestHandlers {
		c.requestHandlers[method] = handler
	}

	go c.readLoop(clientCtx)

	if err := c.Initialize(ctx); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

// Initialize performs the MCP initialize handshake and sends
// notifications/initialized.
func (c *Client) Initialize(ctx context.Context) error {
	if c == nil || c.transport == nil {
		return i18n.NewError(i18n.KeyServicesMCPInitializeNeedsTransport)
	}
	initCtx, cancel := withDefaultTimeout(ctx, c.options.InitializeTimeout)
	defer cancel()

	params := map[string]any{
		"protocolVersion": MCPProtocolVersion,
		"capabilities":    cloneMap(c.options.Capabilities),
		"clientInfo":      c.options.ClientInfo,
	}

	var raw json.RawMessage
	if err := c.CallRaw(initCtx, "initialize", params, &raw); err != nil {
		return i18n.WrapError(i18n.KeyServicesMCPInitializeFailed, err)
	}

	var result struct {
		ProtocolVersion string             `json:"protocolVersion"`
		Capabilities    ServerCapabilities `json:"capabilities"`
		ServerInfo      *ServerInfo        `json:"serverInfo"`
		Instructions    string             `json:"instructions"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &result); err != nil {
			return i18n.WrapError(i18n.KeyServicesMCPDecodeInitialize, err)
		}
	}
	if result.Capabilities == nil {
		result.Capabilities = ServerCapabilities{}
	}

	c.mu.Lock()
	c.capabilities = result.Capabilities
	c.serverInfo = cloneServerInfo(result.ServerInfo)
	c.instructions = truncateMCPDescription(result.Instructions)
	c.initializeRaw = cloneRawMessage(raw)
	c.initialized = true
	c.mu.Unlock()

	if err := c.Notify(initCtx, "notifications/initialized", nil); err != nil {
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
	if c.raw != nil && c.transport == nil {
		if params == nil {
			params = map[string]any{}
		}
		return c.raw.CallRaw(ctx, method, params, out)
	}
	if c.transport == nil {
		return i18n.NewError(i18n.KeyServicesMCPClientNoTransport)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	id := c.nextID.Add(1)
	msg, err := NewRequestMessage(id, method, params)
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
			*rawOut = cloneRawMessage(resp.result)
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

// Notify sends a JSON-RPC notification.
func (c *Client) Notify(ctx context.Context, method string, params any) error {
	if c == nil || c.transport == nil {
		return i18n.NewError(i18n.KeyServicesMCPNotifyNeedsTransport)
	}
	msg, err := NewNotificationMessage(method, params)
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

// SetRequestHandler registers or replaces a method-specific server request
// handler. Passing nil removes the handler.
func (c *Client) SetRequestHandler(method string, handler RequestHandler) {
	if c == nil || method == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.requestHandlers == nil {
		c.requestHandlers = make(map[string]RequestHandler)
	}
	if handler == nil {
		delete(c.requestHandlers, method)
		return
	}
	c.requestHandlers[method] = handler
}

// GetServerCapabilities returns a copy of the initialize capabilities.
func (c *Client) GetServerCapabilities() ServerCapabilities {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneCapabilities(c.capabilities)
}

// GetServerInfo returns a copy of the initialize serverInfo.
func (c *Client) GetServerInfo() *ServerInfo {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneServerInfo(c.serverInfo)
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

// InitializeRaw returns the raw initialize result bytes.
func (c *Client) InitializeRaw() json.RawMessage {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneRawMessage(c.initializeRaw)
}

// IsInitialized reports whether initialize completed successfully.
func (c *Client) IsInitialized() bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.initialized
}

// IsClosed reports whether a transport-backed client has terminated. Raw
// caller adapters do not expose lifecycle state and are treated as open; their
// next RPC remains the authoritative health check.
func (c *Client) IsClosed() bool {
	if c == nil {
		return true
	}
	if c.transport == nil || c.done == nil {
		return false
	}
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

// ListTools invokes tools/list and returns the structured result envelope.
func (c *Client) ListTools(ctx context.Context) (*ListToolsResult, error) {
	if c == nil {
		return nil, i18n.NewError(i18n.KeyServicesMCPNilClient)
	}
	var out ListToolsResult
	if err := c.CallRaw(ctx, "tools/list", nil, &out); err != nil {
		return nil, i18n.WrapError(i18n.KeyServicesMCPMethodFailed, err, "tools/list")
	}
	return &out, nil
}

// CallTool invokes tools/call and returns the structured envelope.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (*ToolCall, error) {
	if c == nil {
		return nil, i18n.NewError(i18n.KeyServicesMCPNilClient)
	}
	if args == nil {
		args = map[string]any{}
	}
	params := map[string]any{"name": name, "arguments": args}
	var out ToolCall
	if err := c.CallRaw(ctx, "tools/call", params, &out); err != nil {
		return nil, i18n.WrapError(i18n.KeyServicesMCPNamedMethodFailed, err, "tools/call", name)
	}
	return &out, nil
}

// ReadResourceResult invokes resources/read and returns the structured result.
func (c *Client) ReadResourceResult(ctx context.Context, uri string) (*ReadResourceResult, error) {
	if c == nil {
		return nil, i18n.NewError(i18n.KeyServicesMCPNilClient)
	}
	params := map[string]any{"uri": uri}
	var out ReadResourceResult
	if err := c.CallRaw(ctx, "resources/read", params, &out); err != nil {
		return nil, i18n.WrapError(i18n.KeyServicesMCPNamedMethodFailed, err, "resources/read", uri)
	}
	return &out, nil
}

// ReadResource invokes resources/read and returns the contents array.
func (c *Client) ReadResource(ctx context.Context, uri string) ([]ResourceContent, error) {
	result, err := c.ReadResourceResult(ctx, uri)
	if err != nil {
		return nil, err
	}
	return result.Contents, nil
}

// ListResourcesResult invokes resources/list and returns the structured result.
func (c *Client) ListResourcesResult(ctx context.Context) (*ListResourcesResult, error) {
	if c == nil {
		return nil, i18n.NewError(i18n.KeyServicesMCPNilClient)
	}
	var out ListResourcesResult
	if err := c.CallRaw(ctx, "resources/list", nil, &out); err != nil {
		return nil, i18n.WrapError(i18n.KeyServicesMCPMethodFailed, err, "resources/list")
	}
	return &out, nil
}

// ListResources invokes resources/list and returns the structured array.
func (c *Client) ListResources(ctx context.Context) ([]Resource, error) {
	result, err := c.ListResourcesResult(ctx)
	if err != nil {
		return nil, err
	}
	return result.Resources, nil
}

// ListPrompts invokes prompts/list and returns the structured result envelope.
func (c *Client) ListPrompts(ctx context.Context) (*ListPromptsResult, error) {
	if c == nil {
		return nil, i18n.NewError(i18n.KeyServicesMCPNilClient)
	}
	var out ListPromptsResult
	if err := c.CallRaw(ctx, "prompts/list", nil, &out); err != nil {
		return nil, i18n.WrapError(i18n.KeyServicesMCPMethodFailed, err, "prompts/list")
	}
	return &out, nil
}

// Close shuts down the client. For transport-backed clients it rejects all
// pending calls with ErrTransportClosed.
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	if c.transport == nil {
		if closer, ok := c.raw.(interface{ Close() error }); ok {
			return closer.Close()
		}
		return nil
	}
	return c.closeWithError(i18n.KeyServicesMCPTransportClientClosedReason, nil)
}

// MarshalToolCall returns the structured envelope as JSON so the tool layer can
// hand it back to the model verbatim.
func MarshalToolCall(tc *ToolCall) (string, error) {
	if tc == nil {
		return "null", nil
	}
	data, err := json.Marshal(tc)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// MarshalContents serialises a contents[] slice with uri / mimeType preserved.
func MarshalContents(contents []ResourceContent) (string, error) {
	envelope := map[string]any{"contents": contents}
	data, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// MarshalResources serialises a resources[] slice into the structured envelope.
func MarshalResources(resources []Resource) (string, error) {
	envelope := map[string]any{"resources": resources}
	data, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	return string(data), nil
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

func (c *Client) dispatchMessage(msg JSONRPCMessage) {
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

func (c *Client) dispatchResponse(msg JSONRPCMessage) {
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
	resp := rpcResponse{result: cloneRawMessage(msg.Result)}
	if msg.Error != nil {
		resp.err = msg.Error
	}
	select {
	case pending.ch <- resp:
	default:
	}
}

func (c *Client) dispatchNotification(msg JSONRPCMessage) {
	c.mu.RLock()
	handler := c.notificationHandlers[msg.Method]
	c.mu.RUnlock()
	if handler == nil {
		return
	}
	go handler(context.Background(), msg)
}

func (c *Client) dispatchRequest(msg JSONRPCMessage) {
	c.mu.RLock()
	handler := c.requestHandlers[msg.Method]
	c.mu.RUnlock()
	go func() {
		var resp JSONRPCMessage
		var err error
		if handler == nil {
			resp, err = NewErrorMessage(msg.ID, -32601, "Method not found", nil)
		} else {
			result, handlerErr := handler(context.Background(), msg)
			if handlerErr != nil {
				resp, err = NewErrorMessage(msg.ID, -32603, handlerErr.Error(), nil)
			} else {
				resp, err = NewResultMessage(msg.ID, result)
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
		return c.closedError()
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

func (c *Client) send(ctx context.Context, msg JSONRPCMessage) error {
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
		err := newTransportClosedError(reasonKey, cause)
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
	err := c.closeErr
	c.mu.RUnlock()
	if err != nil {
		return err
	}
	return newTransportClosedError(i18n.KeyServicesMCPTransportClientClosedReason, ErrTransportClosed)
}

func (c *Client) installDefaultRequestHandlers() {
	c.requestHandlers["roots/list"] = func(context.Context, JSONRPCMessage) (any, error) {
		roots := c.options.Roots
		if len(roots) == 0 {
			roots = []Root{{URI: defaultRootURI()}}
		}
		return map[string]any{"roots": roots}, nil
	}
	c.requestHandlers["elicitation/create"] = func(context.Context, JSONRPCMessage) (any, error) {
		return map[string]any{"action": "cancel"}, nil
	}
}

func normalizeClientOptions(options ClientOptions) ClientOptions {
	if options.ClientInfo.Name == "" {
		defaultInfo := DefaultClientInfo()
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
		options.Capabilities = DefaultClientCapabilities()
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
	if len(value) <= MaxMCPDescriptionLength {
		return value
	}
	return value[:MaxMCPDescriptionLength] + mcpTruncationSuffix
}

func defaultRootURI() string {
	cwd, err := os.Getwd()
	if err != nil || cwd == "" {
		return "file://"
	}
	return "file://" + filepath.ToSlash(cwd)
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

func cloneCapabilities(in ServerCapabilities) ServerCapabilities {
	if in == nil {
		return nil
	}
	out := make(ServerCapabilities, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
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

func cloneServerInfo(in *ServerInfo) *ServerInfo {
	if in == nil {
		return nil
	}
	out := *in
	if in.Meta != nil {
		out.Meta = cloneMap(in.Meta)
	}
	return &out
}
