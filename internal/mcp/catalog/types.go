package catalog

import "encoding/json"

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

// ClientInfo is sent during initialization.
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
