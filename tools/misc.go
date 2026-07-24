package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// ─── SyntheticOutputTool ─────────────────────────────────────────────────────

// SyntheticOutputTool generates synthetic tool output for testing/debugging.
type SyntheticOutputTool struct{}

func (t *SyntheticOutputTool) Name() string { return "SyntheticOutput" }
func (t *SyntheticOutputTool) Description() string {
	return "Generate synthetic tool output for testing and debugging purposes"
}

func (t *SyntheticOutputTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": "The synthetic output content to return",
			},
			"is_error": map[string]any{
				"type":        "boolean",
				"description": "Whether to mark the output as an error",
			},
		},
		Required: []string{"content"},
	}
}

func (t *SyntheticOutputTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	in, toolErr := parseInputOrError[SyntheticOutputInput](input)
	if toolErr != nil {
		return *toolErr, nil
	}
	return types.ToolResult{Content: in.Content, IsError: in.IsError}, nil
}

// ─── RemoteTriggerTool ───────────────────────────────────────────────────────

// RemoteTriggerTool fires a webhook / remote trigger.
type RemoteTriggerTool struct {
	HTTPClient               *http.Client
	AccessTokenResolver      func(context.Context) (string, error)
	OrganizationUUIDResolver func(context.Context, string, string) (string, error)
	BaseURLResolver          func() (string, error)
	Availability             RemoteTriggerAvailability

	// FeatureFlagResolver, if set, gates the entire tool. When it returns
	// false the tool refuses to issue any HTTP traffic. When unset, the
	// tool falls back to the CLAUDE_CODE_DISABLE_REMOTE_TRIGGER env var
	// (the `tengu_surreal_dali` parity flag).
	FeatureFlagResolver func(context.Context) (bool, error)

	// PolicyResolver gates the tool against the allow_remote_sessions
	// policy. When it returns false the tool refuses. When unset, the
	// tool falls back to CLAUDE_CODE_ALLOW_REMOTE_SESSIONS.
	PolicyResolver func(context.Context) (bool, error)
}

func (t *RemoteTriggerTool) Name() string { return "RemoteTrigger" }
func (t *RemoteTriggerTool) Description() string {
	return "Manage scheduled remote Claude Code agents (triggers) via the claude.ai CCR API. Auth is handled in-process - the token never reaches the shell."
}

func (t *RemoteTriggerTool) Schema() types.JSONSchema {
	return types.StrictObjectSchema(
		map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "Trigger action to perform",
				"enum":        []string{"list", "get", "create", "update", "run"},
			},
			"trigger_id": map[string]any{
				"type":        "string",
				"description": "Required for get, update, and run",
				"pattern":     `^[\w-]+$`,
			},
			"body": map[string]any{
				"type":        "object",
				"description": "JSON body for create and update",
			},
		},
		"action",
	)
}

func (t *RemoteTriggerTool) ToolContract() types.ToolContract {
	outputSchema := types.StrictObjectSchema(map[string]any{
		"status": map[string]any{"type": "number"},
		"json":   map[string]any{"type": "string"},
	}, "status", "json")
	return types.ToolContract{
		OutputSchema:       &outputSchema,
		Strict:             true,
		ConcurrencySafe:    true,
		MaxResultSizeChars: remoteTriggerMaxResultSizeChars,
	}
}

func (t *RemoteTriggerTool) ToolMetadata(input map[string]any) types.ToolMetadata {
	action := strings.TrimSpace(remoteTriggerStringInputValue(input, "action"))
	readOnly := action == "list" || action == "get"
	return types.ToolMetadata{
		ReadOnly:           readOnly,
		Write:              !readOnly,
		ConcurrencySafe:    true,
		MaxResultSizeChars: remoteTriggerMaxResultSizeChars,
	}
}

func (t *RemoteTriggerTool) IsConcurrentSafe() bool { return true }

func (t *RemoteTriggerTool) IsReadOnlyInput(input map[string]any) bool {
	action := strings.TrimSpace(remoteTriggerStringInputValue(input, "action"))
	return action == "list" || action == "get"
}

func (t *RemoteTriggerTool) AutoClassifierInput(in RemoteTriggerInput) string {
	if strings.TrimSpace(in.TriggerID) == "" {
		return "RemoteTrigger " + strings.TrimSpace(in.Action)
	}
	return "RemoteTrigger " + strings.TrimSpace(in.Action) + " " + strings.TrimSpace(in.TriggerID)
}

func (t *RemoteTriggerTool) AutoClassifierInputMap(input map[string]any) string {
	in, err := types.DecodeStrictToolInput[RemoteTriggerInput](input)
	if err != nil {
		return "RemoteTrigger"
	}
	return t.AutoClassifierInput(in)
}

func (t *RemoteTriggerTool) ToAutoClassifierInput(input map[string]any) string {
	return t.AutoClassifierInputMap(input)
}

func (t *RemoteTriggerTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	out, ok := data.(remoteTriggerOutput)
	if !ok {
		return types.ToolResultBlock{ToolUseID: toolUseID, Content: ""}
	}
	return types.ToolResultBlock{
		ToolUseID: toolUseID,
		Content:   fmtRemoteTriggerOutput(out.Status, out.JSON),
	}
}

func (t *RemoteTriggerTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	in, err := types.DecodeStrictToolInput[RemoteTriggerInput](input)
	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolRuntimeInvalidInput, err), nil
	}

	// Feature-flag guard (tengu_surreal_dali parity).
	if disabled, err := t.featureFlagDisabled(ctx); err != nil {
		return toolRuntimeErrorf(i18n.KeyToolRemoteFeatureResolverFailed, err), nil
	} else if disabled {
		return toolRuntimeError(i18n.KeyToolRemoteFeatureDisabled), nil
	}

	// Policy guard (allow_remote_sessions).
	if forbidden, err := t.policyForbids(ctx); err != nil {
		return toolRuntimeErrorf(i18n.KeyToolRemotePolicyResolverFailed, err), nil
	} else if forbidden {
		return toolRuntimeError(i18n.KeyToolRemotePolicyBlocked), nil
	}

	return t.executeRemoteTriggerAPI(ctx, in)
}

// featureFlagDisabled returns true when the tool should refuse to run.
func (t *RemoteTriggerTool) featureFlagDisabled(ctx context.Context) (bool, error) {
	if t.FeatureFlagResolver != nil {
		enabled, err := t.FeatureFlagResolver(ctx)
		if err != nil {
			return false, err
		}
		return !enabled, nil
	}
	if t.Availability != nil {
		return !t.Availability.FeatureEnabled(remoteTriggerGrowthBookFeature, false), nil
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CLAUDE_CODE_DISABLE_REMOTE_TRIGGER"))) {
	case "1", "true", "yes", "on":
		return true, nil
	}
	return false, nil
}

// policyForbids returns true when the allow_remote_sessions policy denies use.
func (t *RemoteTriggerTool) policyForbids(ctx context.Context) (bool, error) {
	if t.PolicyResolver != nil {
		allowed, err := t.PolicyResolver(ctx)
		if err != nil {
			return false, err
		}
		return !allowed, nil
	}
	if t.Availability != nil {
		return !t.Availability.PolicyAllowed(remoteTriggerPolicyName), nil
	}
	v := strings.ToLower(strings.TrimSpace(os.Getenv("CLAUDE_CODE_ALLOW_REMOTE_SESSIONS")))
	switch v {
	case "0", "false", "no", "off":
		return true, nil
	}
	return false, nil
}

// ─── Typed inputs ────────────────────────────────────────────────────────────

// SyntheticOutputInput is the typed input for SyntheticOutputTool
type SyntheticOutputInput struct {
	Content string `json:"content"`
	IsError bool   `json:"is_error"`
}

// RemoteTriggerInput is the typed input for RemoteTriggerTool
type RemoteTriggerInput struct {
	Action    string         `json:"action,omitempty"`
	TriggerID string         `json:"trigger_id,omitempty"`
	Body      map[string]any `json:"body,omitempty"`
}

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return `{"error":"failed to encode json"}`
	}
	return string(data)
}

func remoteTriggerStringInputValue(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	value, _ := input[key].(string)
	return value
}
