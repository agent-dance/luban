package remote

import (
	"context"
	"net/http"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// Trigger fires a remote trigger through the provider API.
type Trigger struct {
	HTTPClient               *http.Client
	AccessTokenResolver      func(context.Context) (string, error)
	OrganizationUUIDResolver func(context.Context, string, string) (string, error)
	BaseURLResolver          func() (string, error)
}

func (t *Trigger) Name() string { return "RemoteTrigger" }

func (t *Trigger) Description() string {
	return promptText(i18n.KeyToolRemoteTriggerDescription)
}

func (t *Trigger) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": promptText(i18n.KeyToolRemoteTriggerInputAction),
				"enum":        []string{"list", "get", "create", "update", "run"},
			},
			"trigger_id": map[string]any{
				"type":        "string",
				"description": promptText(i18n.KeyToolRemoteTriggerInputTriggerID),
				"pattern":     `^[\w-]+$`,
			},
			"body": map[string]any{
				"type":        "object",
				"description": promptText(i18n.KeyToolRemoteTriggerInputBody),
			},
		},
		"action",
	)
}

func (t *Trigger) ToolMetadata(input map[string]any) types.ToolMetadata {
	action := strings.TrimSpace(stringInputValue(input, "action"))
	readOnly := action == "list" || action == "get"
	return types.ToolMetadata{
		ReadOnly:           readOnly,
		Write:              !readOnly,
		ConcurrencySafe:    true,
		MaxResultSizeChars: maxResultSizeChars,
	}
}

func (t *Trigger) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	out, ok := data.(triggerOutput)
	if !ok {
		return types.ToolResultBlock{ToolUseID: toolUseID, Content: ""}
	}
	return types.ToolResultBlock{
		ToolUseID: toolUseID,
		Content:   formatOutput(out.Status, out.JSON),
	}
}

func (t *Trigger) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	in, err := types.DecodeStrictToolInput[TriggerInput](input)
	if err != nil {
		return runtimeErrorf(i18n.KeyToolRuntimeInvalidInput, err), nil
	}

	return t.executeAPI(ctx, in)
}

// TriggerInput is the typed input for Trigger.
type TriggerInput struct {
	Action    string         `json:"action,omitempty"`
	TriggerID string         `json:"trigger_id,omitempty"`
	Body      map[string]any `json:"body,omitempty"`
}

func stringInputValue(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	value, _ := input[key].(string)
	return value
}
