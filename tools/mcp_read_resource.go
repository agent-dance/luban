package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agent-dance/luban/i18n"
	svcmcp "github.com/agent-dance/luban/services/mcp"
	"github.com/agent-dance/luban/types"
)

const readMcpResourceMaxResultSizeChars = 100_000

// ReadMcpResourceContent is the exact public item shape declared by the
// TypeScript ReadMcpResourceTool output schema. Pointers preserve the
// distinction between an omitted optional string and a present empty string.
type ReadMcpResourceContent struct {
	URI         string  `json:"uri"`
	MimeType    *string `json:"mimeType,omitempty"`
	Text        *string `json:"text,omitempty"`
	BlobSavedTo string  `json:"blobSavedTo,omitempty"`
}

// ReadMcpResourceOutput is retained in ToolResult.Data for typed consumers and
// serialized by the tool-result mapper for model-visible content.
type ReadMcpResourceOutput struct {
	Contents []ReadMcpResourceContent `json:"contents"`
}

type readMcpResourceInput struct {
	Server string `json:"server"`
	URI    string `json:"uri"`
}

type rawReadMcpResourceContent struct {
	URI      *string `json:"uri"`
	MimeType *string `json:"mimeType,omitempty"`
	Text     *string `json:"text,omitempty"`
	Blob     *string `json:"blob,omitempty"`
}

type rawReadMcpResourceEnvelope struct {
	Contents []json.RawMessage `json:"contents"`
}

// ReadMcpResourceTool reads one resource from an already-managed MCP server.
type ReadMcpResourceTool struct {
	manager        *MCPManager
	serviceManager *svcmcp.Manager
}

// NewReadMcpResourceTool constructs a ReadMcpResourceTool. Production wiring
// supplies the services manager; the legacy manager remains for compatibility
// with in-process tests and older embedders.
func NewReadMcpResourceTool(manager *MCPManager, serviceManagers ...*svcmcp.Manager) *ReadMcpResourceTool {
	var serviceManager *svcmcp.Manager
	if len(serviceManagers) > 0 {
		serviceManager = serviceManagers[0]
	}
	return &ReadMcpResourceTool{manager: manager, serviceManager: serviceManager}
}

func (t *ReadMcpResourceTool) WithServiceManager(manager *svcmcp.Manager) *ReadMcpResourceTool {
	if t != nil {
		t.serviceManager = manager
	}
	return t
}

func (t *ReadMcpResourceTool) Name() string           { return "ReadMcpResourceTool" }
func (t *ReadMcpResourceTool) Aliases() []string      { return []string{"ReadMcpResource"} }
func (t *ReadMcpResourceTool) IsConcurrentSafe() bool { return true }
func (t *ReadMcpResourceTool) IsReadOnly() bool       { return true }
func (t *ReadMcpResourceTool) UserFacingName() string { return "readMcpResource" }
func (t *ReadMcpResourceTool) Description() string {
	return "Reads a specific resource from an MCP server."
}

func (t *ReadMcpResourceTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"server": map[string]any{
				"type":        "string",
				"description": "The MCP server name",
			},
			"uri": map[string]any{
				"type":        "string",
				"description": "The resource URI to read",
			},
		},
		"server",
		"uri",
	)
}

func (t *ReadMcpResourceTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{
		ReadOnly:           true,
		ConcurrencySafe:    true,
		MaxResultSizeChars: readMcpResourceMaxResultSizeChars,
	}
}

func (t *ReadMcpResourceTool) ToolContract() types.ToolContract {
	itemSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"uri": map[string]any{
				"type":        "string",
				"description": "Resource URI",
			},
			"mimeType": map[string]any{
				"type":        "string",
				"description": "MIME type of the content",
			},
			"text": map[string]any{
				"type":        "string",
				"description": "Text content of the resource",
			},
			"blobSavedTo": map[string]any{
				"type":        "string",
				"description": "Path where binary blob content was saved",
			},
		},
		"required": []string{"uri"},
	}
	outputSchema := types.StrictObjectSchema(
		map[string]any{
			"contents": map[string]any{
				"type":  "array",
				"items": itemSchema,
			},
		},
		"contents",
	)
	return types.ToolContract{
		OutputSchema:       &outputSchema,
		Strict:             true,
		ReadOnly:           true,
		ConcurrencySafe:    true,
		MaxResultSizeChars: readMcpResourceMaxResultSizeChars,
	}
}

func (t *ReadMcpResourceTool) ToAutoClassifierInput(input map[string]any) string {
	server, _ := input["server"].(string)
	uri, _ := input["uri"].(string)
	return server + " " + uri
}

func (t *ReadMcpResourceTool) SearchReadClassification(map[string]any) types.ToolSearchReadClassification {
	return types.ToolSearchReadClassification{IsRead: true}
}

// IsResultTruncated mirrors isOutputLineTruncated(jsonStringify(output)). JSON
// stringification normally yields one line; this remains a first-class hook so
// UI callers share the TypeScript contract.
func (t *ReadMcpResourceTool) IsResultTruncated(output any) bool {
	data, err := marshalReadMcpResourceOutput(output)
	if err != nil {
		return false
	}
	position := 0
	for i := 0; i <= 3; i++ {
		next := bytes.IndexByte(data[position:], '\n')
		if next < 0 {
			return false
		}
		position += next + 1
	}
	return position < len(data)
}

// IsEnabled implements TS resourceTools gating: the helper is visible only
// while at least one connected server advertises resources support.
func (t *ReadMcpResourceTool) IsEnabled(types.ToolRuntimeContext) bool {
	if t == nil {
		return false
	}
	if t.serviceManager != nil {
		for _, state := range t.serviceManager.Snapshot() {
			if state.Type == svcmcp.MCPStateConnected && state.Client != nil && !state.Client.IsClosed() && mcpStateSupportsResources(state) {
				return true
			}
		}
		return false
	}
	if t.manager == nil {
		return false
	}
	t.manager.mu.RLock()
	defer t.manager.mu.RUnlock()
	for _, conn := range t.manager.servers {
		if legacyMCPConnectionSupportsResources(conn) {
			return true
		}
	}
	return false
}

func (t *ReadMcpResourceTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	in, err := decodeReadMcpResourceInput(input)
	if err != nil {
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolMCPReadInvalidInput, err)), nil
	}
	if t != nil && t.serviceManager != nil {
		return t.readWithServiceManager(ctx, in.Server, in.URI)
	}
	return t.readWithLegacyManager(ctx, in.Server, in.URI)
}

func decodeReadMcpResourceInput(input map[string]any) (readMcpResourceInput, error) {
	if input == nil {
		return readMcpResourceInput{}, i18n.NewError(i18n.KeyToolMCPReadServerURIRequired)
	}
	if _, ok := input["server"]; !ok {
		return readMcpResourceInput{}, i18n.NewError(i18n.KeyToolMCPReadServerRequired)
	}
	if _, ok := input["uri"]; !ok {
		return readMcpResourceInput{}, i18n.NewError(i18n.KeyToolMCPReadURIRequired)
	}
	in, err := types.DecodeStrictToolInput[readMcpResourceInput](input)
	if err != nil {
		return readMcpResourceInput{}, err
	}
	return in, nil
}

func (t *ReadMcpResourceTool) readWithServiceManager(ctx context.Context, server, uri string) (types.ToolResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	names := t.serviceManager.ServerNames()
	if !stringInSlice(names, server) {
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPResourcesServerNotFound, server, strings.Join(names, ", "))), nil
	}

	state, exists := t.serviceManager.State(server)
	if !exists || state.Type != svcmcp.MCPStateConnected {
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolMCPReadNotConnected, server)), nil
	}
	if !mcpStateSupportsResources(state) {
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolMCPReadUnsupported, server)), nil
	}
	if state.Client == nil {
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolMCPReadNotConnected, server)), nil
	}

	if state.Client.IsClosed() {
		reconnected, reconnectErr := t.reconnectServiceResourceClient(ctx, server)
		if reconnectErr != nil {
			return mcpResourceErrorResult(reconnectErr.Error()), nil
		}
		state = reconnected
	}

	raw, readErr := readResourceRaw(ctx, state.Client, uri)
	if readErr == nil {
		return renderMCPReadResourceToolResult(raw, server), nil
	}
	if svcmcp.IsAuthRequiredError(readErr) {
		t.serviceManager.MarkNeedsAuth(server, readErr)
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolMCPReadFailed, uri, server, readErr)), nil
	}

	var recovered svcmcp.MCPServerConnection
	var recoverErr error
	switch {
	case svcmcp.IsSessionExpiredError(readErr):
		var handled bool
		recovered, handled, recoverErr = t.serviceManager.RecoverExpiredSession(ctx, server, readErr)
		if !handled {
			return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolMCPReadFailed, uri, server, readErr)), nil
		}
	case state.Client.IsClosed() || errors.Is(readErr, svcmcp.ErrTransportClosed):
		recovered, recoverErr = t.reconnectServiceResourceClient(ctx, server)
	default:
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolMCPReadFailed, uri, server, readErr)), nil
	}
	if recoverErr != nil {
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolMCPReadFailed, uri, server, readErr)), nil
	}
	if validationErr := validateReadMcpResourceState(recovered, server); validationErr != nil {
		return mcpResourceErrorResult(validationErr.Error()), nil
	}
	raw, readErr = readResourceRaw(ctx, recovered.Client, uri)
	if readErr != nil {
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolMCPReadFailed, uri, server, readErr)), nil
	}
	return renderMCPReadResourceToolResult(raw, server), nil
}

func (t *ReadMcpResourceTool) reconnectServiceResourceClient(ctx context.Context, server string) (svcmcp.MCPServerConnection, error) {
	state, err := t.serviceManager.Reconnect(ctx, server)
	if err != nil {
		return svcmcp.MCPServerConnection{}, i18n.WrapError(i18n.KeyToolMCPReadNotConnectedCause, err, server)
	}
	if err := validateReadMcpResourceState(state, server); err != nil {
		return svcmcp.MCPServerConnection{}, err
	}
	return state, nil
}

func validateReadMcpResourceState(state svcmcp.MCPServerConnection, server string) error {
	if state.Type != svcmcp.MCPStateConnected || state.Client == nil || state.Client.IsClosed() {
		return i18n.NewError(i18n.KeyToolMCPReadNotConnected, server)
	}
	if !mcpStateSupportsResources(state) {
		return i18n.NewError(i18n.KeyToolMCPReadUnsupported, server)
	}
	return nil
}

func (t *ReadMcpResourceTool) readWithLegacyManager(ctx context.Context, server, uri string) (types.ToolResult, error) {
	if t == nil || t.manager == nil {
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPResourcesServerNotFound, server, "")), nil
	}
	names := t.manager.ServerNames()
	if !stringInSlice(names, server) {
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolRuntimeMCPResourcesServerNotFound, server, strings.Join(names, ", "))), nil
	}

	t.manager.mu.RLock()
	conn := t.manager.servers[server]
	t.manager.mu.RUnlock()
	if conn == nil || !conn.ready {
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolMCPReadNotConnected, server)), nil
	}
	if !legacyMCPConnectionSupportsResources(conn) {
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolMCPReadUnsupported, server)), nil
	}
	if conn.client == nil && conn.httpBaseURL != "" {
		return mcpHTTPReadResource(ctx, conn.httpBaseURL, uri, server)
	}
	if conn.client == nil || conn.client.IsClosed() {
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolMCPReadNotConnected, server)), nil
	}

	callCtx, callCancel := withMCPCallTimeout(ctx)
	defer callCancel()
	var raw json.RawMessage
	if err := conn.client.CallRaw(callCtx, "resources/read", map[string]any{"uri": uri}, &raw); err != nil {
		if conn.client.IsClosed() {
			t.manager.invalidateLegacyMCPConnection(server, conn)
		}
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolMCPReadFailed, uri, server, err)), nil
	}
	return renderMCPReadResourceToolResult(raw, server), nil
}

func legacyMCPConnectionSupportsResources(conn *MCPServerConn) bool {
	if conn == nil || !conn.ready {
		return false
	}
	if conn.httpBaseURL != "" {
		return true
	}
	if conn.client == nil || conn.client.IsClosed() {
		return false
	}
	capabilities := conn.capabilities
	if capabilities == nil {
		capabilities = conn.client.ServerCapabilities()
	}
	_, ok := capabilities["resources"]
	return ok
}

func (m *MCPManager) invalidateLegacyMCPConnection(name string, target *MCPServerConn) {
	if m == nil {
		return
	}
	m.mu.Lock()
	if current := m.servers[name]; current == target {
		delete(m.servers, name)
	}
	m.mu.Unlock()
}

func readResourceRaw(ctx context.Context, client *svcmcp.Client, uri string) (json.RawMessage, error) {
	callCtx, cancel := withMCPCallTimeout(ctx)
	defer cancel()
	var raw json.RawMessage
	if err := client.CallRaw(callCtx, "resources/read", map[string]any{"uri": uri}, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func renderMCPReadResourceToolResult(raw json.RawMessage, serverName string) types.ToolResult {
	output, err := normalizeMCPReadResourceOutput(raw, serverName)
	if err != nil {
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolMCPReadInvalidResult, err))
	}
	data, err := marshalReadMcpResourceOutput(output)
	if err != nil {
		return mcpResourceErrorResult(toolRuntimeFormat(i18n.KeyToolMCPReadMarshalResult, err))
	}
	return types.ToolResult{
		Content: string(data),
		Data:    output,
		Metadata: map[string]string{
			"mcp.resultType": "resourceRead",
			"mcp.serverName": serverName,
		},
	}
}

func normalizeMCPReadResourceOutput(raw json.RawMessage, serverName string) (ReadMcpResourceOutput, error) {
	output := ReadMcpResourceOutput{Contents: make([]ReadMcpResourceContent, 0)}
	if len(bytes.TrimSpace(raw)) == 0 {
		return output, nil
	}
	var envelope rawReadMcpResourceEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return output, err
	}
	for i, rawItem := range envelope.Contents {
		var item rawReadMcpResourceContent
		if err := json.Unmarshal(rawItem, &item); err != nil {
			return output, fmt.Errorf("contents[%d]: %w", i, err)
		}
		if item.URI == nil {
			return output, i18n.NewError(i18n.KeyToolMCPReadContentURIRequired, i)
		}
		content := ReadMcpResourceContent{URI: *item.URI, MimeType: cloneStringPointer(item.MimeType)}
		if item.Text != nil {
			content.Text = cloneStringPointer(item.Text)
			output.Contents = append(output.Contents, content)
			continue
		}
		if item.Blob == nil {
			output.Contents = append(output.Contents, content)
			continue
		}
		decoded, decodeErr := base64.StdEncoding.DecodeString(stripBase64Whitespace(*item.Blob))
		if decodeErr != nil {
			message := toolRuntimeFormat(i18n.KeyToolMCPReadInvalidBase64, decodeErr)
			content.Text = &message
			output.Contents = append(output.Contents, content)
			continue
		}
		persisted := persistMCPBinaryContent(decoded, dereferenceString(item.MimeType), newMCPPersistID("", fmt.Sprintf("resource-%d", i)))
		if persisted.Error != "" {
			message := toolRuntimeFormat(i18n.KeyToolMCPReadBinarySaveFailed, persisted.Error)
			content.Text = &message
			output.Contents = append(output.Contents, content)
			continue
		}
		content.BlobSavedTo = persisted.Filepath
		message := getMCPBinaryBlobSavedMessage(
			persisted.Filepath,
			dereferenceString(item.MimeType),
			persisted.Size,
			toolRuntimeFormat(i18n.KeyToolRuntimeMCPSourceResourceAt, serverName, *item.URI),
		)
		content.Text = &message
		output.Contents = append(output.Contents, content)
	}
	return output, nil
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func dereferenceString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// MapToolResultToToolResultBlock serializes only the typed TS output. Raw MCP
// envelopes and metadata never reach model-visible content through this path.
func (t *ReadMcpResourceTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	output, ok := data.(ReadMcpResourceOutput)
	if !ok {
		if pointer, pointerOK := data.(*ReadMcpResourceOutput); pointerOK && pointer != nil {
			output = *pointer
		} else {
			output = ReadMcpResourceOutput{Contents: []ReadMcpResourceContent{}}
		}
	}
	if output.Contents == nil {
		output.Contents = []ReadMcpResourceContent{}
	}
	content := `{"contents":[]}`
	if raw, err := marshalReadMcpResourceOutput(output); err == nil {
		content = string(raw)
	}
	return types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: toolUseID,
		Content:   content,
	}
}

func marshalReadMcpResourceOutput(output any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte{'\n'}), nil
}
