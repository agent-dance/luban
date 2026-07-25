package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/mcp/catalog"
	mcpmanager "github.com/agent-dance/luban/internal/mcp/manager"
	mcptransport "github.com/agent-dance/luban/internal/mcp/transport"
	"github.com/agent-dance/luban/types"
)

// ListMcpResourcesTool lists resources from connected MCP servers.
type ListMcpResourcesTool struct {
	serviceManager *mcpmanager.Manager
}

// ListMcpResourcesInput is the typed input for ListMcpResourcesTool.
type ListMcpResourcesInput struct {
	Server string `json:"server"`
}

const listMcpResourcesMaxResultSizeChars = 100_000

func listMCPResourcesEmptyResult() string {
	return toolRuntimeText(i18n.KeyToolRuntimeMCPResourcesEmpty)
}

func NewListMcpResourcesTool(manager *mcpmanager.Manager) *ListMcpResourcesTool {
	return &ListMcpResourcesTool{serviceManager: manager}
}

func (t *ListMcpResourcesTool) Name() string { return "ListMcpResourcesTool" }
func (t *ListMcpResourcesTool) Description() string {
	return toolPromptText(i18n.KeyMCPListResourcesToolDescription)
}

// IsResultTruncated reports whether the serialized result exceeds the
// line-oriented preview contract. The shared tool-result contract enforces the
// byte budget itself.
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

func (t *ListMcpResourcesTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"server": map[string]any{
				"type":        "string",
				"description": toolPromptText(i18n.KeyMCPListResourcesServerDescription),
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
			ContentBlocks: []types.ContentBlock{newMCPTextBlock(message)},
			IsError:       true,
		}, nil
	}

	if t == nil || t.serviceManager == nil {
		return mcpRenderServiceResourceList(nil, nil), nil
	}
	return t.listWithServiceManager(ctx, strings.TrimSpace(in.Server))
}

// ListMcpResource is the stable public output item for ListMcpResourcesTool.
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

	resources := make([]ListMcpResource, 0)
	skipped := make([]mcpSkippedServer, 0)
	for _, name := range names {
		serverResources, serverSkipped := t.serviceResourcesForServer(ctx, name)
		resources = append(resources, serverResources...)
		skipped = append(skipped, serverSkipped...)
	}
	return mcpRenderServiceResourceList(resources, skipped), nil
}

func (t *ListMcpResourcesTool) serviceResourcesForServer(ctx context.Context, name string) ([]ListMcpResource, []mcpSkippedServer) {
	state, exists := t.serviceManager.State(name)
	if !exists {
		return nil, []mcpSkippedServer{{Server: name, State: "unknown", Reason: toolRuntimeText(i18n.KeyToolRuntimeMCPResourcesStateUnavailable)}}
	}
	if state.Type != mcpmanager.MCPStateConnected {
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

func (t *ListMcpResourcesTool) listServiceResources(ctx context.Context, state mcpmanager.MCPServerConnection) (catalog.ListResourcesResult, error) {
	if state.Client == nil {
		return catalog.ListResourcesResult{}, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeMCPResourcesNoActiveClient, state.Name))
	}
	if state.Client.IsClosed() {
		return t.reconnectServiceResources(ctx, state.Name)
	}
	callCtx, cancel := withMCPCallTimeout(ctx)
	defer cancel()

	result, err := t.serviceManager.ListResources(callCtx, state.Name)
	if err == nil {
		return result, nil
	}
	if state.Client.IsClosed() || errors.Is(err, mcptransport.ErrTransportClosed) {
		return t.reconnectServiceResources(ctx, state.Name)
	}
	return catalog.ListResourcesResult{}, err
}

func (t *ListMcpResourcesTool) reconnectServiceResources(ctx context.Context, serverName string) (catalog.ListResourcesResult, error) {
	reconnected, reconnectErr := t.serviceManager.Reconnect(ctx, serverName)
	if reconnectErr != nil {
		return catalog.ListResourcesResult{}, reconnectErr
	}
	if reconnected.Type != mcpmanager.MCPStateConnected || reconnected.Client == nil {
		return catalog.ListResourcesResult{}, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeMCPResourcesReconnectNotConnected, serverName, reconnected.Type))
	}
	if !mcpStateSupportsResources(reconnected) {
		return catalog.ListResourcesResult{}, fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeMCPResourcesUnsupportedAfterReconnect, serverName))
	}
	callCtx, cancel := withMCPCallTimeout(ctx)
	defer cancel()
	result, err := t.serviceManager.ListResources(callCtx, serverName)
	if err != nil {
		return catalog.ListResourcesResult{}, err
	}
	return result, nil
}

func mcpRenderServiceResourceList(resources []ListMcpResource, skipped []mcpSkippedServer) types.ToolResult {
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

func mcpListedResourcesFromService(serverName string, resources []catalog.Resource) []ListMcpResource {
	out := make([]ListMcpResource, 0, len(resources))
	for _, resource := range resources {
		out = append(out, ListMcpResource{
			Server:      serverName,
			URI:         resource.URI,
			Name:        resource.Name,
			Description: resource.Description,
			MimeType:    resource.MimeType,
		})
	}
	return out
}

func mcpStateSupportsResources(state mcpmanager.MCPServerConnection) bool {
	if state.Capabilities == nil {
		return false
	}
	_, ok := state.Capabilities["resources"]
	return ok
}

func mcpSkippedFromServiceState(state mcpmanager.MCPServerConnection) mcpSkippedServer {
	skipped := mcpSkippedServer{Server: state.Name, State: string(state.Type)}
	switch state.Type {
	case mcpmanager.MCPStateNeedsAuth:
		if state.NeedsAuth != nil && state.NeedsAuth.Reason != "" {
			skipped.Reason = state.NeedsAuth.Reason
		} else {
			skipped.Reason = toolRuntimeText(i18n.KeyToolRuntimeMCPResourcesRequiresAuthentication)
		}
	case mcpmanager.MCPStateFailed:
		skipped.Reason = state.Error
	case mcpmanager.MCPStateDisabled:
		skipped.Reason = toolRuntimeText(i18n.KeyToolRuntimeMCPResourcesDisabled)
	case mcpmanager.MCPStatePending:
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
		ContentBlocks: []types.ContentBlock{newMCPTextBlock(message)},
		IsError:       true,
	}
}

// MapToolResultToToolResultBlock keeps typed resource data separate from the
// provider-visible JSON or empty-resource text.
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
