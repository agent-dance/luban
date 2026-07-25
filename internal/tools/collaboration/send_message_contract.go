package collaboration

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/toolmeta"
	"github.com/agent-dance/luban/types"
)

const sendMessageMaxResultSizeChars = 100_000

// RetainedAgentResume is the narrow delivery result SendMessage needs after a
// retained agent accepts a prompt. Agent runtime state remains private to the
// adapter that implements RetainedAgentMessenger.
type RetainedAgentResume struct {
	Status     string
	OutputPath string
}

// RetainedAgentMessenger resumes or queues work for a retained agent. The
// boolean reports whether target belongs to the retained-agent runtime.
type RetainedAgentMessenger interface {
	ResumeAgent(context.Context, string, string) (RetainedAgentResume, bool, error)
}

// RetainedAgentStopper cancels an in-process retained agent after its approved
// shutdown has been durably published to the team mailbox and config.
type RetainedAgentStopper interface {
	AbortAgent(string) bool
}

// messageRouting is the model-visible routing metadata emitted by SendMessage.
type messageRouting struct {
	Sender      string `json:"sender"`
	SenderColor string `json:"senderColor,omitempty"`
	Target      string `json:"target"`
	TargetColor string `json:"targetColor,omitempty"`
	Summary     string `json:"summary,omitempty"`
	Content     string `json:"content,omitempty"`
}

// sendMessageResult is the typed output shared by direct, broadcast, retained
// agent, shutdown-control, and local-socket delivery.
type sendMessageResult struct {
	Success    bool            `json:"success"`
	Message    string          `json:"message"`
	Recipients *[]string       `json:"recipients,omitempty"`
	Routing    *messageRouting `json:"routing,omitempty"`
	RequestID  string          `json:"request_id,omitempty"`
	Target     string          `json:"target,omitempty"`
}

type sendMessageInput struct {
	To      string `json:"to"`
	Summary string `json:"summary,omitempty"`
	Message any    `json:"message"`
}

type structuredSendMessage struct {
	Type      string
	RequestID string
	Approve   *bool
	Reason    string
}

// SendMessageTool delivers plain prompts and the two supported shutdown
// control messages without importing the agent runtime implementation.
type SendMessageTool struct {
	manager  *TeamManager
	retained RetainedAgentMessenger
	stopper  RetainedAgentStopper
}

func NewSendMessageTool(manager *TeamManager, retained RetainedAgentMessenger, stopper RetainedAgentStopper) *SendMessageTool {
	return &SendMessageTool{manager: manager, retained: retained, stopper: stopper}
}

func (t *SendMessageTool) Name() string { return "SendMessage" }

func (t *SendMessageTool) Description() string {
	return sendMessagePromptText(i18n.KeyToolSendMessageDescription)
}

func strictSendMessageObject(properties map[string]any, required ...string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func (t *SendMessageTool) Schema() types.JSONSchema {
	structured := []any{
		strictSendMessageObject(map[string]any{
			"type":   map[string]any{"type": "string", "const": "shutdown_request"},
			"reason": map[string]any{"type": "string"},
		}, "type"),
		strictSendMessageObject(map[string]any{
			"type":       map[string]any{"type": "string", "const": "shutdown_response"},
			"request_id": map[string]any{"type": "string"},
			"approve":    map[string]any{"type": "boolean"},
			"reason":     map[string]any{"type": "string"},
		}, "type", "request_id", "approve"),
	}
	return types.StrictObjectSchema(map[string]any{
		"to": map[string]any{
			"type": "string", "description": sendMessagePromptText(i18n.KeyToolSendMessageToDescription),
		},
		"summary": map[string]any{
			"type": "string", "description": sendMessagePromptText(i18n.KeyToolSendMessageSummaryDescription),
		},
		"message": map[string]any{
			"description": sendMessagePromptText(i18n.KeyToolSendMessageMessageDescription),
			"oneOf": append([]any{map[string]any{
				"type": "string", "description": sendMessagePromptText(i18n.KeyToolSendMessagePlainTextDescription),
			}}, structured...),
		},
	}, "to", "message")
}

func (t *SendMessageTool) ToolDiscoveryMetadata() toolmeta.Metadata {
	return toolmeta.Metadata{
		ShouldDefer: true,
		SearchHint:  sendMessagePromptText(i18n.KeyToolSendMessageDescription),
	}
}

func (t *SendMessageTool) IsEnabled(runtime types.ToolRuntimeContext) bool {
	if runtime.Features == nil {
		return true
	}
	enabled, configured := runtime.Features[types.ToolFeatureTeams]
	return !configured || enabled
}

// ToolMetadata classifies every delivery as a write. An approved shutdown is
// additionally destructive because it both deactivates the durable member and
// may cancel its in-process runtime.
func (t *SendMessageTool) ToolMetadata(input map[string]any) types.ToolMetadata {
	return types.ToolMetadata{
		Write:              true,
		Destructive:        isApprovedShutdownInput(input),
		MaxResultSizeChars: sendMessageMaxResultSizeChars,
	}
}

func isApprovedShutdownInput(input map[string]any) bool {
	message, ok := input["message"].(map[string]any)
	if !ok || message["type"] != "shutdown_response" {
		return false
	}
	approve, ok := message["approve"].(bool)
	return ok && approve
}

func (t *SendMessageTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	encoded, err := json.Marshal(data)
	if err != nil {
		return types.ToolResultBlock{
			Type:      types.ContentTypeToolResult,
			ToolUseID: toolUseID,
			Content:   i18n.WrapInternalError(i18n.KeyAuxSwarmFailed, err).Error(),
			IsError:   true,
			Outcome:   types.ToolOutcomeFailed,
		}
	}
	return types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: toolUseID,
		Content:   string(encoded),
		Data:      data,
		Outcome:   types.ToolOutcomeSucceeded,
	}
}

func decodeStructuredSendMessage(message any) (structuredSendMessage, bool, error) {
	if _, plain := message.(string); plain || message == nil {
		return structuredSendMessage{}, false, nil
	}
	raw, ok := message.(map[string]any)
	if !ok {
		return structuredSendMessage{}, false, i18n.NewError(i18n.KeyToolSendMessageStructuredObjectRequired)
	}
	decoded := structuredSendMessage{}
	decoded.Type, _ = raw["type"].(string)
	decoded.RequestID, _ = raw["request_id"].(string)
	decoded.Reason, _ = raw["reason"].(string)
	if approve, ok := raw["approve"].(bool); ok {
		decoded.Approve = &approve
	}
	if err := validateStructuredSendMessageInput(raw, decoded); err != nil {
		return structuredSendMessage{}, true, err
	}
	return decoded, true, nil
}

func validateStructuredSendMessageInput(raw map[string]any, decoded structuredSendMessage) error {
	allowed := map[string]bool{"type": true}
	required := []string{"type"}
	switch decoded.Type {
	case "shutdown_request":
		allowed["reason"] = true
	case "shutdown_response":
		allowed["request_id"] = true
		allowed["approve"] = true
		allowed["reason"] = true
		required = append(required, "request_id", "approve")
	default:
		return i18n.NewError(i18n.KeyToolSendMessageStructuredTypeUnsupported, decoded.Type)
	}
	for key := range raw {
		if !allowed[key] {
			return i18n.NewError(i18n.KeyToolSendMessageStructuredFieldUnsupported, key)
		}
	}
	for _, key := range []string{"type", "reason"} {
		if value, exists := raw[key]; exists {
			if _, valid := value.(string); !valid {
				return i18n.NewError(i18n.KeyToolSendMessageStructuredFieldStringRequired, key)
			}
		}
	}
	for _, key := range required {
		value, exists := raw[key]
		if !exists {
			return i18n.NewError(i18n.KeyToolSendMessageStructuredFieldRequired, decoded.Type, key)
		}
		switch key {
		case "type", "request_id":
			text, valid := value.(string)
			if !valid || strings.TrimSpace(text) == "" {
				return i18n.NewError(i18n.KeyToolSendMessageStructuredFieldRequired, decoded.Type, key)
			}
		case "approve":
			if _, valid := value.(bool); !valid {
				return i18n.NewError(i18n.KeyToolSendMessageStructuredFieldRequired, decoded.Type, key)
			}
		}
	}
	return nil
}

func sendMessageResponse(result sendMessageResult) (types.ToolResult, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return types.ToolResult{
			Content: i18n.WrapInternalError(i18n.KeyAuxSwarmFailed, err).Error(),
			IsError: true,
			Outcome: types.ToolOutcomeFailed,
		}, nil
	}
	outcome := types.ToolOutcomeSucceeded
	if !result.Success {
		outcome = types.ToolOutcomeFailed
	}
	return types.ToolResult{Content: string(data), Data: result, Outcome: outcome}, nil
}

func sendMessageBroadcastResult(message string, recipients []string, routing *messageRouting) sendMessageResult {
	copyRecipients := append([]string(nil), recipients...)
	return sendMessageResult{
		Success: true, Message: message, Recipients: &copyRecipients, Routing: routing,
	}
}

func sendMessagePromptText(key i18n.Key) string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), key)
}

func sendMessageRuntimeText(key i18n.Key) string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), key)
}

func sendMessageRuntimeFormat(key i18n.Key, args ...any) string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), key, args...)
}

func sendMessageError(err error) types.ToolResult {
	return types.ToolResult{Content: err.Error(), IsError: true, Outcome: types.ToolOutcomeFailed}
}
