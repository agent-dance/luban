package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
	svcmcp "github.com/agent-dance/luban/services/mcp"
	"github.com/agent-dance/luban/types"
	"github.com/creachadair/jrpc2"
	"github.com/creachadair/jrpc2/channel"
)

// ServerConfig defines an MCP server connection.
type ServerConfig struct {
	Type                string            `json:"type,omitempty"`
	Command             string            `json:"command,omitempty"`
	Args                []string          `json:"args,omitempty"`
	Env                 map[string]string `json:"env,omitempty"`
	URL                 string            `json:"url,omitempty"`
	Headers             map[string]string `json:"headers,omitempty"`
	HeadersHelper       string            `json:"headersHelper,omitempty"`
	OAuth               *OAuthConfig      `json:"oauth,omitempty"`
	IDEName             string            `json:"ideName,omitempty"`
	AuthToken           string            `json:"authToken,omitempty"`
	IDERunningInWindows *bool             `json:"ideRunningInWindows,omitempty"`
	Name                string            `json:"name,omitempty"`
	ID                  string            `json:"id,omitempty"`
	Scope               string            `json:"scope,omitempty"`
	PluginSource        string            `json:"pluginSource,omitempty"`
	// Stderr is where the MCP server subprocess's stderr is routed.
	// Defaults to io.Discard when nil so SDK/daemon callers are not polluted.
	Stderr io.Writer `json:"-"`
}

// OAuthConfig preserves the MCP OAuth configuration for remote transports.
type OAuthConfig struct {
	ClientID              string `json:"clientId,omitempty"`
	CallbackPort          *int   `json:"callbackPort,omitempty"`
	AuthServerMetadataURL string `json:"authServerMetadataUrl,omitempty"`
	XAA                   *bool  `json:"xaa,omitempty"`
}

// MCPTool represents a tool discovered from an MCP server.
type MCPTool struct {
	ToolName     string          `json:"name"`
	OriginalName string          `json:"-"` // the server's actual tool name, used for tools/call
	ToolDesc     string          `json:"description"`
	InputSchema  json.RawMessage `json:"inputSchema"`
	client       *Client
}

func (t *MCPTool) Name() string        { return t.ToolName }
func (t *MCPTool) Description() string { return t.ToolDesc }

func (t *MCPTool) Schema() types.JSONSchema {
	var schema types.JSONSchema
	if err := json.Unmarshal(t.InputSchema, &schema); err != nil {
		return types.JSONSchema{Type: "object"}
	}
	return schema
}

func (t *MCPTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	// Use OriginalName for the tools/call RPC; fall back to ToolName for backward compat.
	name := t.OriginalName
	if name == "" {
		name = t.ToolName
	}
	result, err := t.client.CallTool(ctx, name, input)
	if err != nil {
		return types.ToolResult{Content: i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyAuxMCPToolError, err), IsError: true}, nil
	}
	return types.ToolResult{Content: result}, nil
}

// MCPResource represents a resource advertised by an MCP server.
type MCPResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// Client manages a connection to an MCP server.
type Client struct {
	jc           *jrpc2.Client
	cmd          *exec.Cmd // non-nil when started via NewClient
	name         string
	instructions string
	capabilities svcmcp.ServerCapabilities
}

// NewClient starts an MCP server as a child process and connects to it over
// its stdin/stdout using newline-framed JSON-RPC 2.0.
func NewClient(name string, config ServerConfig) (*Client, error) {
	cmd := exec.Command(config.Command, config.Args...)

	cmd.Env = os.Environ()
	for k, v := range config.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyLegacyMCPStdinPipe, err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyLegacyMCPStdoutPipe, err)
	}

	// H9: Route MCP server stderr through injected writer; default to io.Discard
	// so SDK/daemon callers are not polluted by subprocess error output.
	stderrW := config.Stderr
	if stderrW == nil {
		stderrW = io.Discard
	}
	cmd.Stderr = stderrW

	if err := cmd.Start(); err != nil {
		return nil, i18n.WrapError(i18n.KeyLegacyMCPStartServer, err)
	}

	ch := channel.Line(stdout, stdin)
	client, err := newClientWithChannel(name, ch)
	if err != nil {
		cmd.Process.Kill() //nolint:errcheck
		cmd.Wait()         //nolint:errcheck
		return nil, err
	}
	client.cmd = cmd
	return client, nil
}

// NewClientFromChannel creates an MCP client over an already-established
// jrpc2 channel. Useful for in-process testing with a local jrpc2 server.
func NewClientFromChannel(name string, ch channel.Channel) (*Client, error) {
	return newClientWithChannel(name, ch)
}

// newClientWithChannel creates an MCP client over an already-established
// jrpc2 channel. Used internally and by tests.
func newClientWithChannel(name string, ch channel.Channel) (*Client, error) {
	jc := jrpc2.NewClient(ch, nil)
	c := &Client{jc: jc, name: name}
	if err := c.initialize(); err != nil {
		jc.Close() //nolint:errcheck
		return nil, i18n.WrapError(i18n.KeyLegacyMCPInitialize, err)
	}
	return c, nil
}

func (c *Client) initialize() error {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    brand.CommandName,
			"version": "1.0.0",
		},
	}

	initCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var result struct {
		Instructions string                    `json:"instructions"`
		Capabilities svcmcp.ServerCapabilities `json:"capabilities"`
	}
	if err := c.jc.CallResult(initCtx, "initialize", params, &result); err != nil {
		return err
	}
	c.instructions = strings.TrimSpace(result.Instructions)
	c.capabilities = cloneServerCapabilities(result.Capabilities)

	// Fire-and-forget: tell the server initialization is complete.
	notifyCtx, notifyCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer notifyCancel()
	return c.jc.Notify(notifyCtx, "notifications/initialized", nil)
}

// ServerCapabilities returns a defensive copy of the capabilities advertised
// by the server's initialize result.
func (c *Client) ServerCapabilities() svcmcp.ServerCapabilities {
	if c == nil {
		return nil
	}
	return cloneServerCapabilities(c.capabilities)
}

// IsClosed reports whether the JSON-RPC client has stopped because Close was
// called or its underlying channel terminated.
func (c *Client) IsClosed() bool {
	return c == nil || c.jc == nil || c.jc.IsStopped()
}

func cloneServerCapabilities(in svcmcp.ServerCapabilities) svcmcp.ServerCapabilities {
	if in == nil {
		return nil
	}
	data, err := json.Marshal(in)
	if err != nil {
		return nil
	}
	var out svcmcp.ServerCapabilities
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

// Instructions returns server-authored guidance from the initialize result.
func (c *Client) Instructions() string {
	if c == nil {
		return ""
	}
	return c.instructions
}

// ListTools discovers available tools from the MCP server.
// It handles cursor-based pagination per the MCP spec.
func (c *Client) ListTools() ([]*MCPTool, error) {
	listCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
		if err := c.jc.CallResult(listCtx, "tools/list", params, &resp); err != nil {
			return nil, err
		}

		for _, t := range resp.Tools {
			allTools = append(allTools, &MCPTool{
				ToolName:     svcmcp.BuildMCPToolName(c.name, t.Name),
				OriginalName: t.Name,
				ToolDesc:     t.Description,
				InputSchema:  t.InputSchema,
				client:       c,
			})
		}

		if resp.NextCursor == "" {
			break
		}
		cursor = resp.NextCursor
	}

	return allTools, nil
}

// CallTool invokes a tool on the MCP server.
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]any) (string, error) {
	params := map[string]any{
		"name":      name,
		"arguments": arguments,
	}

	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := c.jc.CallResult(ctx, "tools/call", params, &resp); err != nil {
		return "", err
	}

	if resp.IsError {
		var texts []string
		for _, item := range resp.Content {
			if item.Type == "text" {
				texts = append(texts, item.Text)
			}
		}
		return "", i18n.NewError(i18n.KeyLegacyMCPToolReturnedError, strings.Join(texts, "\n"))
	}

	var texts []string
	for _, item := range resp.Content {
		if item.Type == "text" {
			texts = append(texts, item.Text)
		}
	}
	return strings.Join(texts, "\n"), nil
}

// ListResources lists available resources from the MCP server.
func (c *Client) ListResources() ([]MCPResource, error) {
	listCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var resp struct {
		Resources []MCPResource `json:"resources"`
	}
	if err := c.jc.CallResult(listCtx, "resources/list", map[string]any{}, &resp); err != nil {
		return nil, err
	}
	return resp.Resources, nil
}

// ReadResource reads a resource from the MCP server by URI.
func (c *Client) ReadResource(ctx context.Context, uri string) (string, error) {
	params := map[string]any{"uri": uri}

	var resp struct {
		Contents []struct {
			Type string `json:"type"`
			URI  string `json:"uri"`
			Text string `json:"text"`
		} `json:"contents"`
	}
	if err := c.jc.CallResult(ctx, "resources/read", params, &resp); err != nil {
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

// CallRaw issues a JSON-RPC call and decodes the raw result into out.
// It exposes the protocol layer to services/tool callers that need the
// structured envelope (uri / mimeType) that ReadResource and CallTool
// flatten. Audit P2-6 demands this for the services-layer client surface.
func (c *Client) CallRaw(ctx context.Context, method string, params, out any) error {
	if c == nil || c.jc == nil {
		return i18n.NewError(i18n.KeyLegacyMCPNilClient)
	}
	if params == nil {
		params = map[string]any{}
	}
	return c.jc.CallResult(ctx, method, params, out)
}

// Close shuts down the MCP client and, if the server was launched as a child
// process, waits for it to exit (with a kill fallback on timeout).
func (c *Client) Close() error {
	closeErr := c.jc.Close()
	if c.cmd != nil {
		done := make(chan error, 1)
		go func() { done <- c.cmd.Wait() }()
		select {
		case <-time.After(5 * time.Second):
			c.cmd.Process.Kill() //nolint:errcheck
		case <-done:
		}
	}
	return closeErr
}
