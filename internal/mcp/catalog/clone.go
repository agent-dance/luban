package catalog

import "github.com/agent-dance/luban/internal/mcp/protocol"

// CloneCapabilities returns a detached top-level capability map.
func CloneCapabilities(in ServerCapabilities) ServerCapabilities {
	if in == nil {
		return nil
	}
	out := make(ServerCapabilities, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

// CloneServerInfo returns a detached serverInfo value.
func CloneServerInfo(in *ServerInfo) *ServerInfo {
	if in == nil {
		return nil
	}
	out := *in
	out.Meta = cloneMap(in.Meta)
	return &out
}

// CloneListToolsResult returns a detached copy of a tools/list envelope.
func CloneListToolsResult(in ListToolsResult) ListToolsResult {
	out := ListToolsResult{
		Tools:      make([]ToolDefinition, len(in.Tools)),
		NextCursor: in.NextCursor,
		Meta:       cloneMap(in.Meta),
	}
	for i, tool := range in.Tools {
		out.Tools[i] = cloneToolDefinition(tool)
	}
	return out
}

func cloneToolDefinition(in ToolDefinition) ToolDefinition {
	return ToolDefinition{
		Name:        in.Name,
		Description: in.Description,
		InputSchema: protocol.CloneRawMessage(in.InputSchema),
		Annotations: cloneMap(in.Annotations),
		Meta:        cloneMap(in.Meta),
	}
}

// CloneListResourcesResult returns a detached copy of a resources/list envelope.
func CloneListResourcesResult(in ListResourcesResult) ListResourcesResult {
	out := ListResourcesResult{
		Resources:  make([]Resource, len(in.Resources)),
		NextCursor: in.NextCursor,
		Meta:       cloneMap(in.Meta),
	}
	for i, resource := range in.Resources {
		out.Resources[i] = cloneResource(resource)
	}
	return out
}

func cloneResource(in Resource) Resource {
	return Resource{
		URI:         in.URI,
		Name:        in.Name,
		Description: in.Description,
		MimeType:    in.MimeType,
		Annotations: cloneMap(in.Annotations),
		Meta:        cloneMap(in.Meta),
	}
}

// CloneListPromptsResult returns a detached copy of a prompts/list envelope.
func CloneListPromptsResult(in ListPromptsResult) ListPromptsResult {
	out := ListPromptsResult{
		Prompts:    make([]PromptDefinition, len(in.Prompts)),
		NextCursor: in.NextCursor,
		Meta:       cloneMap(in.Meta),
	}
	for i, prompt := range in.Prompts {
		out.Prompts[i] = clonePromptDefinition(prompt)
	}
	return out
}

func clonePromptDefinition(in PromptDefinition) PromptDefinition {
	return PromptDefinition{
		Name:        in.Name,
		Description: in.Description,
		Arguments:   append([]PromptArgument(nil), in.Arguments...),
		Meta:        cloneMap(in.Meta),
	}
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
