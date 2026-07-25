package transport

import (
	"context"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/mcp/catalog"
)

// GetPrompt invokes prompts/get and returns the structured response envelope.
func (c *Client) GetPrompt(ctx context.Context, name string, args map[string]string) (*catalog.GetPromptResult, error) {
	if c == nil {
		return nil, i18n.NewError(i18n.KeyServicesMCPNilClient)
	}
	params := map[string]any{"name": name}
	if args == nil {
		args = map[string]string{}
	}
	params["arguments"] = args
	var out catalog.GetPromptResult
	if err := c.CallRaw(ctx, "prompts/get", params, &out); err != nil {
		return nil, i18n.WrapError(i18n.KeyServicesMCPNamedMethodFailed, err, "prompts/get", name)
	}
	return &out, nil
}
