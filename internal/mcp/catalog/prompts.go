package catalog

// PromptContent is the MCP prompt-message content union returned by
// prompts/get.
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
