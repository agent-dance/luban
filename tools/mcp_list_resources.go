package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agent-dance/luban/i18n"
	svcmcp "github.com/agent-dance/luban/services/mcp"
	"github.com/agent-dance/luban/types"
)

// ListMcpResourcesTool lists resources from MCP servers. When a services-layer
// manager is supplied, it follows the TypeScript runtime shape: connected
// servers contribute resource records and non-connected servers are skipped.
type ListMcpResourcesTool struct {
	manager        *MCPManager
	serviceManager *svcmcp.Manager
}

const listMcpResourcesMaxResultSizeChars = 100_000

func listMCPResourcesEmptyResult() string {
	return toolRuntimeText(i18n.KeyToolRuntimeMCPResourcesEmpty)
}

// NewListMcpResourcesTool constructs a ListMcpResourcesTool. The optional
// services manager lets task_08+ callers route through the new MCP connection
// manager without breaking the legacy constructor surface.
func NewListMcpResourcesTool(manager *MCPManager, serviceManagers ...*svcmcp.Manager) *ListMcpResourcesTool {
	var serviceManager *svcmcp.Manager
	if len(serviceManagers) > 0 {
		serviceManager = serviceManagers[0]
	}
	return &ListMcpResourcesTool{manager: manager, serviceManager: serviceManager}
}

// WithServiceManager attaches the services-layer MCP manager and returns the
// tool for call-site chaining.
func (t *ListMcpResourcesTool) WithServiceManager(manager *svcmcp.Manager) *ListMcpResourcesTool {
	if t != nil {
		t.serviceManager = manager
	}
	return t
}

func (t *ListMcpResourcesTool) Name() string           { return "ListMcpResourcesTool" }
func (t *ListMcpResourcesTool) Aliases() []string      { return []string{"ListMcpResources"} }
func (t *ListMcpResourcesTool) IsConcurrentSafe() bool { return true }
func (t *ListMcpResourcesTool) IsReadOnly() bool       { return true }
func (t *ListMcpResourcesTool) Description() string {
	return "List available resources from MCP servers"
}

// ToAutoClassifierInput mirrors the TS tool's server-only classifier input.
func (t *ListMcpResourcesTool) ToAutoClassifierInput(input map[string]any) string {
	server, _ := input["server"].(string)
	return server
}

// IsResultTruncated mirrors the TS JSON-output truncation predicate. The
// result budget itself is enforced by the shared tool-result contract.
func (t *ListMcpResourcesTool) IsResultTruncated(output any) bool {
	data, err := json.Marshal(output)
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

func (t *ListMcpResourcesTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{
		ReadOnly:           true,
		ConcurrencySafe:    true,
		MaxResultSizeChars: listMcpResourcesMaxResultSizeChars,
	}
}

func (t *ListMcpResourcesTool) ToolContract() types.ToolContract {
	outputSchema := types.JSONSchema{
		Type: "array",
		Items: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"uri": map[string]any{
					"type":        "string",
					"description": "Resource URI",
				},
				"name": map[string]any{
					"type":        "string",
					"description": "Resource name",
				},
				"mimeType": map[string]any{
					"type":        "string",
					"description": "MIME type of the resource",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Resource description",
				},
				"server": map[string]any{
					"type":        "string",
					"description": "Server that provides this resource",
				},
			},
			"required": []string{"uri", "name", "server"},
		},
	}
	return types.ToolContract{
		OutputSchema:       &outputSchema,
		Strict:             true,
		ReadOnly:           true,
		ConcurrencySafe:    true,
		MaxResultSizeChars: listMcpResourcesMaxResultSizeChars,
	}
}

func (t *ListMcpResourcesTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"server": map[string]any{
				"type":        "string",
				"description": "Optional server name to filter resources by",
			},
		},
	)
}

func (t *ListMcpResourcesTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	in, err := types.DecodeStrictToolInput[ListMcpResourcesInput](input)
	if err != nil {
		message := toolRuntimeFormat(i18n.KeyToolRuntimeMCPResourcesInvalidInput, err)
		return types.ToolResult{
			Content:       message,
			ContentBlocks: []types.ContentBlock{newTextBlock(message)},
			IsError:       true,
		}, nil
	}

	server := strings.TrimSpace(in.Server)
	if t != nil && t.serviceManager != nil {
		return t.listWithServiceManager(ctx, server)
	}
	if server == "" {
		return t.listAllLegacy(ctx)
	}
	return t.listOneLegacy(ctx, server)
}

// ListMcpResource is the typed public output item for ListMcpResourcesTool.
// It intentionally contains exactly the TS output-schema fields.
type ListMcpResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	MimeType    string `json:"mimeType,omitempty"`
	Description string `json:"description,omitempty"`
	Server      string `json:"server"`
}

// ListMcpResourcesOutput is the typed flat resource array retained in
// ToolResult.Data for SDK/internal consumers.
type ListMcpResourcesOutput []ListMcpResource

type mcpListedResource = ListMcpResource

type mcpSkippedServer struct {
	Server string `json:"server"`
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
}

func (t *ListMcpResourcesTool) listWithServiceManager(ctx context.Context, targetServer string) (types.ToolResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	names := t.serviceManager.ServerNames()
	if targetServer != "" {
		if !stringInSlice(names, targetServer) {
			return mcpResourceErrorResult(toolRuntimeFormat(
				i18n.KeyToolRuntimeMCPResourcesServerNotFound,
				targetServer,
				strings.Join(names, ", "),
			)), nil
		}
		resources, skipped := t.serviceResourcesForServer(ctx, targetServer)
		return mcpRenderServiceResourceList(resources, skipped), nil
	}
	if len(names) == 0 {
		return mcpRenderServiceResourceList(nil, nil), nil
	}

	resources := make([]mcpListedResource, 0)
	skipped := make([]mcpSkippedServer, 0)
	for _, name := range names {
		serverResources, serverSkipped := t.serviceResourcesForServer(ctx, name)
		resources = append(resources, serverResources...)
		skipped = append(skipped, serverSkipped...)
	}
	return mcpRenderServiceResourceList(resources, skipped), nil
}

func (t *ListMcpResourcesTool) serviceResourcesForServer(ctx context.Context, name string) ([]mcpListedResource, []mcpSkippedServer) {
	// The services manager owns its cache and refreshes it atomically from the
	// list_changed handler. Consume the legacy dirty marker without closing an
	// otherwise healthy service-layer connection.
	_ = ConsumeResourcesChanged(name)

	state, exists := t.serviceManager.State(name)
	if !exists {
		return nil, []mcpSkippedServer{{Server: name, State: "unknown", Reason: toolRuntimeText(i18n.KeyToolRuntimeMCPResourcesStateUnavailable)}}
	}
	if state.Type != svcmcp.MCPStateConnected {
		return nil, []mcpSkippedServer{mcpSkippedFromServiceState(state)}
	}
	if !mcpStateSupportsResources(state) {
		return nil, []mcpSkippedServer{{Server: name, State: "resources-unsupported", Reason: toolRuntimeFormat(i18n.KeyToolRuntimeMCPResourcesUnsupported, name)}}
	}

	result, err := t.listServiceResources(ctx, state)
	if err != nil {
		return nil, []mcpSkippedServer{{Server: name, State: "failed", Reason: err.Error()}}
	}
	return mcpListedResourcesFromService(name, result.Resources), nil
}

func (t *ListMcpResourcesTool) listServiceResources(ctx context.Context, state svcmcp.MCPServerConnection) (svcmcp.ListResourcesResult, error) {
	if state.Client == nil {
		return svcmcp.ListResourcesResult{}, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeMCPResourcesNoActiveClient, state.Name))
	}
	if state.Client.IsClosed() {
		return t.reconnectServiceResources(ctx, state.Name)
	}
	if cached, ok := t.serviceManager.Cache().Resources(state.Name); ok {
		return cached, nil
	}
	callCtx, cancel := withMCPCallTimeout(ctx)
	defer cancel()

	result, err := state.Client.ListResourcesResult(callCtx)
	if err == nil {
		t.serviceManager.Cache().StoreResources(state.Name, result)
		return *result, nil
	}
	if state.Client.IsClosed() || errors.Is(err, svcmcp.ErrTransportClosed) {
		return t.reconnectServiceResources(ctx, state.Name)
	}
	return svcmcp.ListResourcesResult{}, err
}

func (t *ListMcpResourcesTool) reconnectServiceResources(ctx context.Context, serverName string) (svcmcp.ListResourcesResult, error) {
	reconnected, reconnectErr := t.serviceManager.Reconnect(ctx, serverName)
	if reconnectErr != nil {
		return svcmcp.ListResourcesResult{}, reconnectErr
	}
	if reconnected.Type != svcmcp.MCPStateConnected || reconnected.Client == nil {
		return svcmcp.ListResourcesResult{}, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeMCPResourcesReconnectNotConnected, serverName, reconnected.Type))
	}
	if !mcpStateSupportsResources(reconnected) {
		return svcmcp.ListResourcesResult{}, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeMCPResourcesUnsupportedAfterReconnect, serverName))
	}
	if cached, ok := t.serviceManager.Cache().Resources(serverName); ok {
		return cached, nil
	}
	callCtx, cancel := withMCPCallTimeout(ctx)
	defer cancel()
	result, err := reconnected.Client.ListResourcesResult(callCtx)
	if err != nil {
		return svcmcp.ListResourcesResult{}, err
	}
	t.serviceManager.Cache().StoreResources(serverName, result)
	return *result, nil
}

func mcpRenderServiceResourceList(resources []mcpListedResource, skipped []mcpSkippedServer) types.ToolResult {
	metadata := mcpSkippedServersMetadata(skipped)
	output := make(ListMcpResourcesOutput, len(resources))
	copy(output, resources)
	if len(resources) == 0 {
		return types.ToolResult{Content: listMCPResourcesEmptyResult(), Data: output, Metadata: metadata}
	}
	data, err := json.Marshal(output)
	if err != nil {
		return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolRuntimeMCPResourcesMarshalFailed, err), IsError: true}
	}
	return types.ToolResult{Content: string(data), Data: output, Metadata: metadata}
}

func (t *ListMcpResourcesTool) listAllLegacy(ctx context.Context) (types.ToolResult, error) {
	if t == nil || t.manager == nil {
		return mcpRenderLegacyResourceList(nil, nil), nil
	}
	names := t.manager.ServerNames()
	if len(names) == 0 {
		return mcpRenderLegacyResourceList(nil, nil), nil
	}

	resources := make([]mcpListedResource, 0)
	skipped := make([]mcpSkippedServer, 0)
	for _, name := range names {
		serverResources, serverSkipped := t.legacyResourcesForServer(ctx, name)
		resources = append(resources, serverResources...)
		skipped = append(skipped, serverSkipped...)
	}
	return mcpRenderLegacyResourceList(resources, skipped), nil
}

func (t *ListMcpResourcesTool) listOneLegacy(ctx context.Context, name string) (types.ToolResult, error) {
	resources, skipped := t.legacyResourcesForServer(ctx, name)
	if len(skipped) > 0 && skipped[0].State == "unknown" {
		available := []string(nil)
		if t != nil && t.manager != nil {
			available = t.manager.ServerNames()
		}
		return mcpResourceErrorResult(toolRuntimeFormat(
			i18n.KeyToolRuntimeMCPServerUnavailable,
			name,
			skipped[0].Reason,
			strings.Join(available, ", "),
		)), nil
	}
	return mcpRenderLegacyResourceList(resources, skipped), nil
}

func (t *ListMcpResourcesTool) legacyResourcesForServer(ctx context.Context, name string) ([]mcpListedResource, []mcpSkippedServer) {
	if t == nil || t.manager == nil {
		return nil, []mcpSkippedServer{{Server: name, State: "unknown", Reason: toolRuntimeText(i18n.KeyToolRuntimeMCPResourcesManagerUnavailable)}}
	}

	// Legacy resource listings are not cached, so consuming the marker is enough
	// to guarantee the next request observes the server after list_changed.
	_ = ConsumeResourcesChanged(name)

	t.manager.mu.RLock()
	conn, exists := t.manager.servers[name]
	_, configured := t.manager.configs[name]
	t.manager.mu.RUnlock()

	if !exists || conn == nil || !conn.ready {
		if configured {
			return nil, []mcpSkippedServer{{Server: name, State: "pending", Reason: toolRuntimeText(i18n.KeyToolRuntimeMCPResourcesConfiguredNotConnected)}}
		}
		return nil, []mcpSkippedServer{{Server: name, State: "unknown", Reason: toolRuntimeText(i18n.KeyToolRuntimeMCPResourcesServerNotConfigured)}}
	}
	if conn.client == nil {
		if conn.httpBaseURL != "" {
			return nil, []mcpSkippedServer{{Server: name, State: "resources-unsupported", Reason: toolRuntimeText(i18n.KeyToolRuntimeMCPResourcesLegacyHTTPUnsupported)}}
		}
		return nil, []mcpSkippedServer{{Server: name, State: "disconnected", Reason: toolRuntimeText(i18n.KeyToolRuntimeMCPResourcesNoActiveClientReason)}}
	}

	callCtx, cancel := withMCPCallTimeout(ctx)
	defer cancel()
	var raw json.RawMessage
	if err := conn.client.CallRaw(callCtx, "resources/list", map[string]any{}, &raw); err != nil {
		return nil, []mcpSkippedServer{{Server: name, State: "failed", Reason: err.Error()}}
	}
	resources, err := mcpListedResourcesFromRaw(name, raw)
	if err != nil {
		return nil, []mcpSkippedServer{{Server: name, State: "failed", Reason: err.Error()}}
	}
	return resources, nil
}

func mcpRenderLegacyResourceList(resources []mcpListedResource, skipped []mcpSkippedServer) types.ToolResult {
	return mcpRenderServiceResourceList(resources, skipped)
}

func mcpListedResourcesFromRaw(serverName string, raw json.RawMessage) ([]mcpListedResource, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, nil
	}
	var result svcmcp.ListResourcesResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("%s: %w", toolRuntimeText(i18n.KeyToolRuntimeMCPResourcesDecodeResult), err)
	}
	return mcpListedResourcesFromService(serverName, result.Resources), nil
}

func mcpListedResourcesFromService(serverName string, resources []svcmcp.Resource) []mcpListedResource {
	out := make([]mcpListedResource, 0, len(resources))
	for _, resource := range resources {
		out = append(out, mcpListedResource{
			Server:      serverName,
			URI:         resource.URI,
			Name:        resource.Name,
			Description: resource.Description,
			MimeType:    resource.MimeType,
		})
	}
	return out
}

func mcpStateSupportsResources(state svcmcp.MCPServerConnection) bool {
	if state.Capabilities == nil {
		return false
	}
	_, ok := state.Capabilities["resources"]
	return ok
}

func mcpSkippedFromServiceState(state svcmcp.MCPServerConnection) mcpSkippedServer {
	skipped := mcpSkippedServer{Server: state.Name, State: string(state.Type)}
	switch state.Type {
	case svcmcp.MCPStateNeedsAuth:
		if state.NeedsAuth != nil && state.NeedsAuth.Reason != "" {
			skipped.Reason = state.NeedsAuth.Reason
		} else {
			skipped.Reason = toolRuntimeText(i18n.KeyToolRuntimeMCPResourcesRequiresAuthentication)
		}
	case svcmcp.MCPStateFailed:
		skipped.Reason = state.Error
	case svcmcp.MCPStateDisabled:
		skipped.Reason = toolRuntimeText(i18n.KeyToolRuntimeMCPResourcesDisabled)
	case svcmcp.MCPStatePending:
		skipped.Reason = toolRuntimeText(i18n.KeyToolRuntimeMCPResourcesPending)
	default:
		skipped.Reason = toolRuntimeText(i18n.KeyToolRuntimeMCPResourcesNotConnected)
	}
	return skipped
}

func mcpSkippedServersMetadata(skipped []mcpSkippedServer) map[string]string {
	if len(skipped) == 0 {
		return nil
	}
	data, err := json.Marshal(skipped)
	if err != nil {
		return nil
	}
	return map[string]string{"mcp.skippedServers": string(data)}
}

func mcpResourceErrorResult(message string) types.ToolResult {
	return types.ToolResult{
		Content:       message,
		ContentBlocks: []types.ContentBlock{newTextBlock(message)},
		IsError:       true,
	}
}

// MapToolResultToToolResultBlock keeps typed resource data separate from the
// provider-visible JSON/no-resources text, matching the TS result mapper.
func (t *ListMcpResourcesTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	output, ok := data.(ListMcpResourcesOutput)
	if !ok {
		if resources, resourcesOK := data.([]ListMcpResource); resourcesOK {
			output = ListMcpResourcesOutput(resources)
		} else {
			output = ListMcpResourcesOutput{}
		}
	}
	content := listMCPResourcesEmptyResult()
	if len(output) > 0 {
		if raw, err := json.Marshal(output); err == nil {
			content = string(raw)
		}
	}
	return types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: toolUseID,
		Content:   content,
	}
}

func stringInSlice(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
