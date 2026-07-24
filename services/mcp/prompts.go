package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// PromptContent is the MCP prompt-message content union. It intentionally
// mirrors the tool/resource content shapes so prompt output can preserve text,
// image, resource, and resource_link semantics without depending on the tool
// package's renderer.
type PromptContent struct {
	Type        string           `json:"type"`
	Text        string           `json:"text,omitempty"`
	Data        string           `json:"data,omitempty"`
	Blob        string           `json:"blob,omitempty"`
	MimeType    string           `json:"mimeType,omitempty"`
	URI         string           `json:"uri,omitempty"`
	Name        string           `json:"name,omitempty"`
	Description string           `json:"description,omitempty"`
	Resource    *ResourceContent `json:"resource,omitempty"`
	Annotations map[string]any   `json:"annotations,omitempty"`
	Meta        map[string]any   `json:"_meta,omitempty"`
}

// PromptMessage is one message returned from prompts/get.
type PromptMessage struct {
	Role    string         `json:"role,omitempty"`
	Content PromptContent  `json:"content"`
	Meta    map[string]any `json:"_meta,omitempty"`
}

// GetPromptResult is the raw prompts/get response envelope.
type GetPromptResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
	Meta        map[string]any  `json:"_meta,omitempty"`
}

// PromptCommandDescriptor is the services-layer command description consumed
// by the commands package. It keeps the MCP prompt's raw name for prompts/get
// while exposing the model/user-facing mcp__<server>__<prompt> command name.
type PromptCommandDescriptor struct {
	ServerName        string
	PromptName        string
	Name              string
	Description       string
	Arguments         []PromptArgument
	ArgumentNames     []string
	RequiredArguments []string
	ArgumentHint      string
}

// PromptCacheInvalidationHook lets task_13 notification wiring invalidate
// command/skill indexes without making the manager import commands or skills.
type PromptCacheInvalidationHook func(serverName string)

var promptInvalidationHooks struct {
	sync.Mutex
	nextID int
	hooks  map[int]PromptCacheInvalidationHook
}

// RegisterPromptCacheInvalidationHook installs a callback invoked when this
// package invalidates prompts/list derived state. The returned function removes
// the hook and is safe to call more than once.
func RegisterPromptCacheInvalidationHook(hook PromptCacheInvalidationHook) func() {
	if hook == nil {
		return func() {}
	}
	promptInvalidationHooks.Lock()
	defer promptInvalidationHooks.Unlock()
	if promptInvalidationHooks.hooks == nil {
		promptInvalidationHooks.hooks = make(map[int]PromptCacheInvalidationHook)
	}
	promptInvalidationHooks.nextID++
	id := promptInvalidationHooks.nextID
	promptInvalidationHooks.hooks[id] = hook
	var once sync.Once
	return func() {
		once.Do(func() {
			promptInvalidationHooks.Lock()
			delete(promptInvalidationHooks.hooks, id)
			promptInvalidationHooks.Unlock()
		})
	}
}

// GetPrompt invokes prompts/get and returns the structured response envelope.
func (c *Client) GetPrompt(ctx context.Context, name string, args map[string]string) (*GetPromptResult, error) {
	if c == nil {
		return nil, i18n.NewError(i18n.KeyServicesMCPNilClient)
	}
	params := map[string]any{"name": name}
	if args == nil {
		args = map[string]string{}
	}
	params["arguments"] = args
	var out GetPromptResult
	if err := c.CallRaw(ctx, "prompts/get", params, &out); err != nil {
		return nil, i18n.WrapError(i18n.KeyServicesMCPNamedMethodFailed, err, "prompts/get", name)
	}
	return &out, nil
}

// PromptCommandDescriptors connects enabled servers as needed, gates on the
// server's prompts capability, and returns prompt slash-command descriptors.
func (m *Manager) PromptCommandDescriptors(ctx context.Context) ([]PromptCommandDescriptor, error) {
	if m == nil {
		return nil, i18n.NewError(i18n.KeyServicesMCPNilManager)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	names := m.ServerNames()
	if len(names) == 0 {
		return PromptCommandDescriptorsFromConnections(m.Snapshot()), nil
	}
	var descriptors []PromptCommandDescriptor
	var errs []error
	for _, name := range names {
		state, err := m.GetOrConnect(ctx, name)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		descriptors = append(descriptors, PromptCommandDescriptorsFromConnections([]MCPServerConnection{state})...)
	}
	sortPromptCommandDescriptors(descriptors)
	return descriptors, errors.Join(errs...)
}

// ExecutePromptCommand reconnects/gates the server, calls prompts/get using the
// descriptor's ordered arguments, and converts MCP prompt messages to provider
// message blocks.
func (m *Manager) ExecutePromptCommand(ctx context.Context, descriptor PromptCommandDescriptor, args string) ([]types.Message, error) {
	if m == nil {
		return nil, i18n.NewError(i18n.KeyServicesMCPNilManager)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	state, err := m.GetOrConnect(ctx, descriptor.ServerName)
	if err != nil {
		return nil, err
	}
	if state.Type != MCPStateConnected || state.Client == nil {
		return nil, i18n.NewError(i18n.KeyServicesMCPServerNotConnected, descriptor.ServerName, state.Type)
	}
	if !capabilityExists(state.Capabilities, "prompts") {
		return nil, i18n.NewError(i18n.KeyServicesMCPServerPromptsUnsupported, descriptor.ServerName)
	}
	result, err := state.Client.GetPrompt(ctx, descriptor.PromptName, PromptArgumentsFromString(descriptor.ArgumentNames, args))
	if err != nil {
		return nil, err
	}
	return TransformPromptMessages(result.Messages, descriptor.ServerName), nil
}

// PromptCommandDescriptorsFromConnections converts already-known connection
// snapshots to descriptors. Only connected servers that advertise prompts are
// included; failed/needs-auth/disabled/pending states are intentionally gated
// out to match the TypeScript fetchCommandsForClient path.
func PromptCommandDescriptorsFromConnections(states []MCPServerConnection) []PromptCommandDescriptor {
	var out []PromptCommandDescriptor
	for _, state := range states {
		if state.Type != MCPStateConnected || !capabilityExists(state.Capabilities, "prompts") {
			continue
		}
		for _, prompt := range state.Prompts {
			if strings.TrimSpace(prompt.Name) == "" {
				continue
			}
			out = append(out, promptCommandDescriptor(state.Name, prompt))
		}
	}
	sortPromptCommandDescriptors(out)
	return out
}

func promptCommandDescriptor(serverName string, prompt PromptDefinition) PromptCommandDescriptor {
	argNames := promptArgumentNames(prompt.Arguments)
	return PromptCommandDescriptor{
		ServerName:        serverName,
		PromptName:        prompt.Name,
		Name:              BuildMCPToolName(serverName, prompt.Name),
		Description:       prompt.Description,
		Arguments:         append([]PromptArgument(nil), prompt.Arguments...),
		ArgumentNames:     argNames,
		RequiredArguments: requiredPromptArgumentNames(prompt.Arguments),
		ArgumentHint:      PromptArgumentHint(prompt.Arguments),
	}
}

func sortPromptCommandDescriptors(descriptors []PromptCommandDescriptor) {
	sort.SliceStable(descriptors, func(i, j int) bool {
		if descriptors[i].ServerName == descriptors[j].ServerName {
			return descriptors[i].PromptName < descriptors[j].PromptName
		}
		return descriptors[i].ServerName < descriptors[j].ServerName
	})
}

// PromptArgumentHint renders required arguments as <name> and optional
// arguments as [name], preserving the prompts/list order.
func PromptArgumentHint(args []PromptArgument) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		name := strings.TrimSpace(arg.Name)
		if name == "" {
			continue
		}
		if arg.Required {
			parts = append(parts, "<"+name+">")
		} else {
			parts = append(parts, "["+name+"]")
		}
	}
	return strings.Join(parts, " ")
}

// PromptArgumentsFromString zips slash-command args to MCP prompt arguments in
// the same declared order as TypeScript's fetchCommandsForClient argNames.
func PromptArgumentsFromString(argNames []string, args string) map[string]string {
	out := make(map[string]string)
	values := strings.Fields(args)
	for i, name := range argNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if i < len(values) {
			out[name] = values[i]
		} else {
			out[name] = ""
		}
	}
	return out
}

func promptArgumentNames(args []PromptArgument) []string {
	names := make([]string, 0, len(args))
	for _, arg := range args {
		name := strings.TrimSpace(arg.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func requiredPromptArgumentNames(args []PromptArgument) []string {
	names := make([]string, 0, len(args))
	for _, arg := range args {
		name := strings.TrimSpace(arg.Name)
		if name != "" && arg.Required {
			names = append(names, name)
		}
	}
	return names
}

// TransformPromptMessages maps MCP prompt messages into the provider message
// model while preserving rich content blocks where Go has a native block type.
func TransformPromptMessages(messages []PromptMessage, serverName string) []types.Message {
	out := make([]types.Message, 0, len(messages))
	for _, msg := range messages {
		blocks := TransformPromptContent(msg.Content, serverName)
		if len(blocks) == 0 {
			continue
		}
		out = append(out, types.Message{
			Role:    promptRole(msg.Role),
			Content: blocks,
		})
	}
	return out
}

// TransformPromptContent converts one MCP prompt content item to provider
// blocks. Unsupported binary/audio content is represented by a compact text
// marker instead of being silently dropped.
func TransformPromptContent(content PromptContent, serverName string) []types.ContentBlock {
	switch content.Type {
	case "text":
		return promptTextBlocks(content.Text)
	case "image":
		return promptImageBlocks(firstPromptValue(content.Data, content.Blob), content.MimeType)
	case "resource":
		if content.Resource == nil {
			return nil
		}
		return transformPromptResource(*content.Resource, serverName)
	case "resource_link":
		return promptTextBlocks(formatPromptResourceLink(content.Name, content.URI, content.Description))
	case "audio":
		return promptTextBlocks(fmt.Sprintf("[Audio from %s: %s]", serverName, fallbackPromptMime(content.MimeType)))
	default:
		if strings.TrimSpace(content.Text) != "" {
			return promptTextBlocks(content.Text)
		}
		if data := firstPromptValue(content.Data, content.Blob); data != "" && strings.HasPrefix(normalizePromptMime(content.MimeType), "image/") {
			return promptImageBlocks(data, content.MimeType)
		}
		raw, _ := json.Marshal(content)
		if len(raw) > 0 {
			return promptTextBlocks(string(raw))
		}
		return nil
	}
}

func transformPromptResource(resource ResourceContent, serverName string) []types.ContentBlock {
	prefix := "[Resource from " + serverName
	if strings.TrimSpace(resource.URI) != "" {
		prefix += " at " + strings.TrimSpace(resource.URI)
	}
	prefix += "] "

	if strings.TrimSpace(resource.Text) != "" {
		return promptTextBlocks(prefix + resource.Text)
	}
	if strings.TrimSpace(resource.Blob) != "" {
		if strings.HasPrefix(normalizePromptMime(resource.MimeType), "image/") {
			blocks := []types.ContentBlock{types.TextBlock{Type: types.ContentTypeText, Text: strings.TrimSpace(prefix)}}
			blocks = append(blocks, promptImageBlocks(resource.Blob, resource.MimeType)...)
			return blocks
		}
		return promptTextBlocks(fmt.Sprintf("%s[Binary resource: %s, base64 bytes=%d]", prefix, fallbackPromptMime(resource.MimeType), len(resource.Blob)))
	}
	return promptTextBlocks(strings.TrimSpace(prefix))
}

func promptTextBlocks(text string) []types.ContentBlock {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return []types.ContentBlock{types.TextBlock{Type: types.ContentTypeText, Text: text}}
}

func promptImageBlocks(data, mimeType string) []types.ContentBlock {
	data = stripPromptBase64Whitespace(data)
	if data == "" {
		return nil
	}
	return []types.ContentBlock{types.ImageBlock{
		Type: types.ContentTypeImage,
		Source: &types.ImageSource{
			Type:      "base64",
			MediaType: promptImageMime(mimeType),
			Data:      data,
		},
	}}
}

func promptRole(role string) types.Role {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case string(types.RoleAssistant):
		return types.RoleAssistant
	default:
		return types.RoleUser
	}
}

func formatPromptResourceLink(name, uri, description string) string {
	display := strings.TrimSpace(name)
	if display == "" {
		display = strings.TrimSpace(uri)
	}
	text := "[Resource link: " + display + "] " + strings.TrimSpace(uri)
	if strings.TrimSpace(description) != "" {
		text += " (" + strings.TrimSpace(description) + ")"
	}
	return text
}

// InvalidatePromptCache clears prompts/list derived state for one server and
// notifies registered command/skill cache hooks. task_13 can call this from
// notifications/prompts/list_changed without adding import cycles.
func (m *Manager) InvalidatePromptCache(serverName string) {
	if m == nil || strings.TrimSpace(serverName) == "" {
		return
	}
	m.cache.clearPrompts(serverName)
	m.mu.Lock()
	if state, ok := m.states[serverName]; ok {
		state.Prompts = nil
		m.setStateLocked(state)
	}
	m.mu.Unlock()
	runPromptInvalidationHooks(serverName)
}

func (c *Cache) clearPrompts(serverName string) {
	if c == nil || strings.TrimSpace(serverName) == "" {
		return
	}
	c.mu.Lock()
	delete(c.prompts, serverName)
	c.mu.Unlock()
}

func runPromptInvalidationHooks(serverName string) {
	promptInvalidationHooks.Lock()
	hooks := make([]PromptCacheInvalidationHook, 0, len(promptInvalidationHooks.hooks))
	for _, hook := range promptInvalidationHooks.hooks {
		hooks = append(hooks, hook)
	}
	promptInvalidationHooks.Unlock()
	for _, hook := range hooks {
		hook(serverName)
	}
}

func firstPromptValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func stripPromptBase64Whitespace(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t', ' ':
			return -1
		default:
			return r
		}
	}, s)
}

func normalizePromptMime(mimeType string) string {
	return strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
}

func promptImageMime(mimeType string) string {
	mime := normalizePromptMime(mimeType)
	if mime == "" {
		return "image/png"
	}
	return mime
}

func fallbackPromptMime(mimeType string) string {
	if strings.TrimSpace(mimeType) == "" {
		return "unknown type"
	}
	return strings.TrimSpace(mimeType)
}
