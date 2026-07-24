package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/swarm"
	"github.com/agent-dance/luban/types"
)

// MessageRouting is the model-visible routing metadata emitted by SendMessage.
// It mirrors SendMessageTool.ts instead of the legacy coordinator Envelope.
type MessageRouting struct {
	Sender      string `json:"sender"`
	SenderColor string `json:"senderColor,omitempty"`
	Target      string `json:"target"`
	TargetColor string `json:"targetColor,omitempty"`
	Summary     string `json:"summary,omitempty"`
	Content     string `json:"content,omitempty"`
}

// SendMessageResult is the typed union carrier for direct, broadcast, request,
// and response outputs. Recipients is a pointer so an empty broadcast list is
// still serialized, while direct results omit the field exactly like TS.
type SendMessageResult struct {
	Success    bool            `json:"success"`
	Message    string          `json:"message"`
	Recipients *[]string       `json:"recipients,omitempty"`
	Routing    *MessageRouting `json:"routing,omitempty"`
	RequestID  string          `json:"request_id,omitempty"`
	Target     string          `json:"target,omitempty"`
}

func sendMessageResponse(result SendMessageResult) (types.ToolResult, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return types.ToolResult{Content: toolRuntimeFormat(i18n.KeyToolLegacyCMarshalResponseFailed, err), IsError: true}, nil
	}
	return types.ToolResult{Content: string(data), Data: result}, nil
}

func sendMessageBroadcastResult(message string, recipients []string, routing *MessageRouting) SendMessageResult {
	copyRecipients := append([]string(nil), recipients...)
	return SendMessageResult{
		Success: true, Message: message, Recipients: &copyRecipients, Routing: routing,
	}
}

func (t *SendMessageTool) Description() string { return "Send a message to another agent" }

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
		strictSendMessageObject(map[string]any{
			"type":       map[string]any{"type": "string", "const": "plan_approval_response"},
			"request_id": map[string]any{"type": "string"},
			"approve":    map[string]any{"type": "boolean"},
			"feedback":   map[string]any{"type": "string"},
		}, "type", "request_id", "approve"),
	}
	return types.StrictObjectSchema(map[string]any{
		"to": map[string]any{
			"type":        "string",
			"description": `Recipient: teammate name, "*" for broadcast, or "uds:<socket-path>" for a local peer`,
		},
		"summary": map[string]any{
			"type":        "string",
			"description": "A 5-10 word summary shown as a preview in the UI (required when message is a string)",
		},
		"message": map[string]any{
			"description": "Plain text message content or a structured swarm control message",
			"oneOf":       append([]any{map[string]any{"type": "string", "description": "Plain text message content"}}, structured...),
		},
	}, "to", "message")
}

func (t *SendMessageTool) ToolContract() types.ToolContract {
	output := types.JSONSchema{Type: "object", AnyOf: []any{
		strictSendMessageObject(map[string]any{
			"success": map[string]any{"type": "boolean"}, "message": map[string]any{"type": "string"},
			"routing": sendMessageRoutingSchema(),
		}, "success", "message"),
		strictSendMessageObject(map[string]any{
			"success": map[string]any{"type": "boolean"}, "message": map[string]any{"type": "string"},
			"recipients": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"routing":    sendMessageRoutingSchema(),
		}, "success", "message", "recipients"),
		strictSendMessageObject(map[string]any{
			"success": map[string]any{"type": "boolean"}, "message": map[string]any{"type": "string"},
			"request_id": map[string]any{"type": "string"}, "target": map[string]any{"type": "string"},
		}, "success", "message", "request_id", "target"),
		strictSendMessageObject(map[string]any{
			"success": map[string]any{"type": "boolean"}, "message": map[string]any{"type": "string"},
			"request_id": map[string]any{"type": "string"},
		}, "success", "message"),
	}}
	return types.ToolContract{OutputSchema: &output, Strict: true, MaxResultSizeChars: 100_000}
}

func sendMessageRoutingSchema() map[string]any {
	return strictSendMessageObject(map[string]any{
		"sender": map[string]any{"type": "string"}, "senderColor": map[string]any{"type": "string"},
		"target": map[string]any{"type": "string"}, "targetColor": map[string]any{"type": "string"},
		"summary": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"},
	}, "sender", "target")
}

func (t *SendMessageTool) ToolDiscoveryMetadata() registry.ToolDiscoveryMetadata {
	return registry.ToolDiscoveryMetadata{
		ShouldDefer: true,
		SearchHint:  "send messages to agent teammates (swarm protocol)",
	}
}

func (t *SendMessageTool) IsEnabled(runtime types.ToolRuntimeContext) bool {
	if runtime.Features == nil {
		return true
	}
	enabled, configured := runtime.Features[types.ToolFeatureTeams]
	return !configured || enabled
}

func isPlainSendMessageInput(input map[string]any) bool {
	_, ok := input["message"].(string)
	return ok
}

func (t *SendMessageTool) IsReadOnlyInput(input map[string]any) bool {
	return isPlainSendMessageInput(input)
}

func (t *SendMessageTool) ToolMetadata(input map[string]any) types.ToolMetadata {
	readOnly := isPlainSendMessageInput(input)
	return types.ToolMetadata{
		ReadOnly:           readOnly,
		Write:              !readOnly,
		MaxResultSizeChars: 100_000,
	}
}

func (t *SendMessageTool) CheckPermissions(_ context.Context, input map[string]any, _ types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	return types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: input}, nil
}

func (t *SendMessageTool) ToAutoClassifierInput(input map[string]any) string {
	target, _ := input["to"].(string)
	if message, ok := input["message"].(string); ok {
		return fmt.Sprintf("to %s: %s", target, message)
	}
	message, _ := input["message"].(map[string]any)
	kind, _ := message["type"].(string)
	approve, _ := coerceSemanticBool(message["approve"])
	switch kind {
	case "shutdown_request":
		return "shutdown_request to " + target
	case "shutdown_response":
		return fmt.Sprintf("shutdown_response %s %v", map[bool]string{true: "approve", false: "reject"}[approve], message["request_id"])
	case "plan_approval_response":
		return fmt.Sprintf("plan_approval %s to %s", map[bool]string{true: "approve", false: "reject"}[approve], target)
	default:
		return strings.TrimSpace(kind)
	}
}

func (t *SendMessageTool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	encoded, err := json.Marshal(data)
	if err != nil {
		return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: toolRuntimeFormat(i18n.KeyToolLegacyCEncodeResultFailed, err), IsError: true}
	}
	return types.ToolResultBlock{Type: types.ContentTypeToolResult, ToolUseID: toolUseID, Content: string(encoded), Data: data}
}

type sendMessageTeamContext struct {
	Name   string
	Config *swarm.TeamConfig
	Active bool
}

func resolveSendMessageTeamContext(manager *TeamManager) (sendMessageTeamContext, error) {
	if teamName := strings.TrimSpace(os.Getenv("CLAUDE_CODE_TEAM_NAME")); teamName != "" {
		storageName := teamStorageName(teamName)
		cfg, err := swarm.LoadTeamConfig(storageName)
		if err == nil {
			return sendMessageTeamContext{Name: cfg.Name, Config: cfg, Active: true}, nil
		}
		return sendMessageTeamContext{Name: storageName, Active: true}, nil
	}
	if manager != nil {
		if info := manager.currentTeamInfo(); info != nil {
			storageName := strings.TrimSpace(info.StorageName)
			if storageName == "" {
				storageName = teamStorageName(info.Name)
			}
			cfg, err := loadActiveTeamConfig(manager)
			if err != nil {
				return sendMessageTeamContext{}, err
			}
			return sendMessageTeamContext{Name: storageName, Config: cfg, Active: true}, nil
		}
	}
	return sendMessageTeamContext{Name: "default"}, nil
}

func validateStructuredSendMessageInput(tool *SendMessageTool, message any, decoded structuredSendMessage) error {
	raw, ok := message.(map[string]any)
	if !ok {
		return i18n.NewError(i18n.KeyToolSendMessageStructuredObjectRequired)
	}
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
	case "plan_approval_response":
		allowed["request_id"] = true
		allowed["approve"] = true
		allowed["feedback"] = true
		required = append(required, "request_id", "approve")
	default:
		return i18n.NewError(i18n.KeyToolSendMessageStructuredTypeUnsupported, decoded.Type)
	}
	for key := range raw {
		if !allowed[key] {
			return i18n.NewError(i18n.KeyToolSendMessageStructuredFieldUnsupported, key)
		}
	}
	for _, key := range []string{"type", "reason", "feedback"} {
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
		if key == "request_id" {
			text, valid := value.(string)
			if !valid || strings.TrimSpace(text) == "" {
				return i18n.NewError(i18n.KeyToolSendMessageStructuredFieldRequired, decoded.Type, key)
			}
		}
		if key == "approve" {
			if _, valid := coerceSemanticBool(value); !valid {
				return i18n.NewError(i18n.KeyToolSendMessageStructuredFieldRequired, decoded.Type, key)
			}
		}
	}
	return nil
}
