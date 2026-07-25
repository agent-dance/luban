package mcp

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/toolmeta"
	mcpauth "github.com/agent-dance/luban/internal/mcp/auth"
	"github.com/agent-dance/luban/internal/mcp/catalog"
	mcpmanager "github.com/agent-dance/luban/internal/mcp/manager"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

const maxMCPDynamicDescriptionLength = 2048

// DynamicMCPManager is the services-layer manager surface needed by
// model-facing MCP tools. Tests can provide a fake without starting transports.
type DynamicMCPManager interface {
	Snapshot() []mcpmanager.MCPServerConnection
	GetOrConnect(context.Context, string) (mcpmanager.MCPServerConnection, error)
}

type dynamicMCPReconnector interface {
	Reconnect(context.Context, string) (mcpmanager.MCPServerConnection, error)
}

// DynamicMCPTool exposes one MCP server tool as a first-class model-facing
// tool named mcp__<server>__<tool>.
type DynamicMCPTool struct {
	manager DynamicMCPManager

	serverName string
	toolName   string
	modelName  string

	description string
	inputSchema json.RawMessage
	annotations map[string]any
	meta        map[string]any
}

// NewDynamicMCPTool constructs a model-facing wrapper around one tools/list
// entry. serverName and definition.Name are intentionally kept unnormalized
// for the eventual tools/call RPC.
func NewDynamicMCPTool(manager DynamicMCPManager, serverName string, definition catalog.ToolDefinition) *DynamicMCPTool {
	return &DynamicMCPTool{
		manager:     manager,
		serverName:  serverName,
		toolName:    definition.Name,
		modelName:   catalog.BuildMCPToolName(serverName, definition.Name),
		description: truncateMCPDynamicDescription(definition.Description),
		inputSchema: cloneJSONRaw(definition.InputSchema),
		annotations: cloneAnyMap(definition.Annotations),
		meta:        cloneAnyMap(definition.Meta),
	}
}

func (t *DynamicMCPTool) Name() string {
	if t == nil {
		return ""
	}
	return t.modelName
}

func (t *DynamicMCPTool) Description() string {
	if t == nil {
		return ""
	}
	if strings.TrimSpace(t.description) != "" {
		return t.description
	}
	return toolPromptFormat(i18n.KeyMCPDynamicToolFallbackDescription, t.toolName, t.serverName)
}

func (t *DynamicMCPTool) Schema() types.JSONSchema {
	if t == nil {
		return types.JSONSchema{Type: "object", Properties: map[string]any{}}
	}
	return jsonSchemaFromRawMCPInput(t.inputSchema)
}

func (t *DynamicMCPTool) ToolMetadata(map[string]any) types.ToolMetadata {
	readOnly := t != nil && t.annotationBool("readOnlyHint")
	return types.ToolMetadata{
		ReadOnly:        readOnly,
		ConcurrencySafe: readOnly,
	}
}

func (t *DynamicMCPTool) MCPServerName() string {
	if t == nil {
		return ""
	}
	return t.serverName
}

func (t *DynamicMCPTool) MCPToolName() string {
	if t == nil {
		return ""
	}
	return t.toolName
}

func (t *DynamicMCPTool) MCPModelName() string {
	return t.Name()
}

func (t *DynamicMCPTool) RawInputSchema() json.RawMessage {
	if t == nil {
		return nil
	}
	return cloneJSONRaw(t.inputSchema)
}

func (t *DynamicMCPTool) MCPAnnotations() map[string]any {
	if t == nil {
		return nil
	}
	return cloneAnyMap(t.annotations)
}

func (t *DynamicMCPTool) MCPMeta() map[string]any {
	if t == nil {
		return nil
	}
	return cloneAnyMap(t.meta)
}

func (t *DynamicMCPTool) MCPDynamicRegistration() registry.MCPDynamicRegistration {
	if t == nil {
		return registry.MCPDynamicRegistration{}
	}
	return registry.MCPDynamicRegistration{
		ServerName: t.serverName,
		ToolName:   t.toolName,
		ModelName:  t.modelName,
		Kind:       "tool",
	}
}

func (t *DynamicMCPTool) ToolPermissionIdentity() string {
	return t.Name()
}

func (t *DynamicMCPTool) ToolDiscoveryMetadata() toolmeta.Metadata {
	if t == nil {
		return toolmeta.Metadata{}
	}
	return toolmeta.Metadata{
		ShouldDefer: true,
		AlwaysLoad:  t.metaBool("anthropic/alwaysLoad"),
		SearchHint:  collapseMCPDiscoveryWhitespace(t.metaString("anthropic/searchHint")),
	}
}

func (t *DynamicMCPTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if t == nil {
		return mcpRuntimeErrorResult(toolRuntimeText(i18n.KeyToolMCPDynamicUninitialized), "", ""), nil
	}
	return newMCPCallRuntime(t.manager, t.serverName, t.toolName).execute(ctx, input)
}

type dynamicMCPAuthTool struct {
	*McpAuthTool
	serverName string
}

func newDynamicMCPAuthTool(serverName string, config catalog.MCPServerConfig, oauth *mcpauth.OAuthManager, manager DynamicMCPManager, syncAfterAuth func(context.Context) error) *dynamicMCPAuthTool {
	auth := NewMcpAuthTool(serverName, config, oauth)
	auth.OnAuthenticated = func(ctx context.Context, name string, _ catalog.MCPServerConfig) error {
		if reconnector, ok := manager.(dynamicMCPReconnector); ok {
			if _, err := reconnector.Reconnect(ctx, name); err != nil {
				return err
			}
		}
		if syncAfterAuth != nil {
			return syncAfterAuth(ctx)
		}
		return nil
	}
	return &dynamicMCPAuthTool{McpAuthTool: auth, serverName: serverName}
}

func (t *dynamicMCPAuthTool) MCPDynamicRegistration() registry.MCPDynamicRegistration {
	if t == nil {
		return registry.MCPDynamicRegistration{}
	}
	return registry.MCPDynamicRegistration{
		ServerName: t.serverName,
		ToolName:   "authenticate",
		ModelName:  t.Name(),
		Kind:       "authenticate",
	}
}

func (t *dynamicMCPAuthTool) ToolPermissionIdentity() string {
	if t == nil {
		return ""
	}
	return t.Name()
}

func (t *dynamicMCPAuthTool) ToolDiscoveryMetadata() toolmeta.Metadata {
	return toolmeta.Metadata{
		ShouldDefer: true,
		SearchHint:  toolPromptText(i18n.KeyMCPAuthToolDiscoveryHint),
	}
}

// BuildDynamicMCPToolsFromSnapshot converts MCP manager state into
// model-facing dynamic tools without opening transports.
func BuildDynamicMCPToolsFromSnapshot(manager DynamicMCPManager, oauth *mcpauth.OAuthManager, syncAfterAuth func(context.Context) error) []types.Tool {
	if manager == nil {
		return nil
	}
	states := manager.Snapshot()
	out := make([]types.Tool, 0)
	for _, state := range states {
		switch state.Type {
		case mcpmanager.MCPStateConnected:
			for _, definition := range state.Tools {
				if strings.TrimSpace(definition.Name) == "" {
					continue
				}
				out = append(out, NewDynamicMCPTool(manager, state.Name, definition))
			}
		case mcpmanager.MCPStateNeedsAuth:
			out = append(out, newDynamicMCPAuthTool(state.Name, state.Config, oauth, manager, syncAfterAuth))
		}
	}
	return out
}

// RegisterDynamicMCPTools syncs the live registry with the manager snapshot.
func RegisterDynamicMCPTools(reg *registry.Registry, manager DynamicMCPManager, oauth *mcpauth.OAuthManager) {
	if reg == nil {
		return
	}
	reg.SyncMCPDynamicTools(BuildDynamicMCPToolsFromSnapshot(manager, oauth, nil))
}

// RefreshDynamicMCPTools connects configured servers, then syncs the registry
// with connected tools or needs-auth pseudo-tools.
func RefreshDynamicMCPTools(ctx context.Context, reg *registry.Registry, manager *mcpmanager.Manager, oauth *mcpauth.OAuthManager) error {
	if reg == nil || manager == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := manager.ConnectAll(ctx)
	syncAfterAuth := func(syncCtx context.Context) error {
		reg.SyncMCPDynamicTools(BuildDynamicMCPToolsFromSnapshot(manager, oauth, nil))
		return nil
	}
	reg.SyncMCPDynamicTools(BuildDynamicMCPToolsFromSnapshot(manager, oauth, syncAfterAuth))
	return err
}

func (t *DynamicMCPTool) annotationBool(key string) bool {
	if t == nil || t.annotations == nil {
		return false
	}
	return anyBool(t.annotations[key])
}

func (t *DynamicMCPTool) metaBool(key string) bool {
	if t == nil || t.meta == nil {
		return false
	}
	return anyBool(t.meta[key])
}

func (t *DynamicMCPTool) metaString(key string) string {
	if t == nil || t.meta == nil {
		return ""
	}
	value, _ := t.meta[key].(string)
	return value
}

func collapseMCPDiscoveryWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func anyBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

func jsonSchemaFromRawMCPInput(raw json.RawMessage) types.JSONSchema {
	if len(raw) == 0 {
		return types.JSONSchema{Type: "object", Properties: map[string]any{}}
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return types.JSONSchema{Type: "object", Properties: map[string]any{}}
	}
	schema := types.JSONSchema{Type: "object", Properties: map[string]any{}}
	if typ, ok := decoded["type"].(string); ok && strings.TrimSpace(typ) != "" {
		schema.Type = typ
	}
	if desc, ok := decoded["description"].(string); ok {
		schema.Description = desc
	}
	if properties, ok := decoded["properties"].(map[string]any); ok {
		schema.Properties = properties
	}
	if required, ok := decoded["required"].([]any); ok {
		schema.Required = make([]string, 0, len(required))
		for _, item := range required {
			if s, ok := item.(string); ok {
				schema.Required = append(schema.Required, s)
			}
		}
	}
	return schema
}

func cloneJSONRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func truncateMCPDynamicDescription(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= maxMCPDynamicDescriptionLength {
		return value
	}
	return toolPromptFormat(i18n.KeyMCPDynamicToolTruncatedDescription, string(runes[:maxMCPDynamicDescriptionLength]))
}
