package collaboration

import (
	"context"
	"reflect"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestSendMessageMetadataTreatsDeliveryAsWriteAndApprovedShutdownAsDestructive(t *testing.T) {
	tool := NewSendMessageTool(nil, nil, nil)
	tests := []struct {
		name        string
		input       map[string]any
		destructive bool
	}{
		{
			name: "plain",
			input: map[string]any{
				"to": "worker", "summary": "share status", "message": "ready",
			},
		},
		{
			name: "shutdown request",
			input: map[string]any{
				"to": "worker", "message": map[string]any{"type": "shutdown_request"},
			},
		},
		{
			name: "shutdown rejected",
			input: map[string]any{
				"to": teamLeadName,
				"message": map[string]any{
					"type": "shutdown_response", "request_id": "request-1", "approve": false, "reason": "busy",
				},
			},
		},
		{
			name: "shutdown approved",
			input: map[string]any{
				"to": teamLeadName,
				"message": map[string]any{
					"type": "shutdown_response", "request_id": "request-1", "approve": true,
				},
			},
			destructive: true,
		},
		{
			name: "string boolean is not approval",
			input: map[string]any{
				"to": teamLeadName,
				"message": map[string]any{
					"type": "shutdown_response", "request_id": "request-1", "approve": "true",
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metadata := tool.ToolMetadata(test.input)
			if metadata.ReadOnly || !metadata.Write || metadata.Destructive != test.destructive {
				t.Fatalf("ToolMetadata() = %#v", metadata)
			}
			if metadata.MaxResultSizeChars != sendMessageMaxResultSizeChars {
				t.Fatalf("MaxResultSizeChars = %d", metadata.MaxResultSizeChars)
			}
		})
	}
}

func TestSendMessageDoesNotOverridePermissionEngine(t *testing.T) {
	if _, exists := reflect.TypeOf((*SendMessageTool)(nil)).MethodByName("CheckPermissions"); exists {
		t.Fatal("SendMessage must not unconditionally allow its own execution")
	}
}

func TestSendMessageStructuredInputIsMapOnlyAndBooleanStrict(t *testing.T) {
	type structuredFixture struct {
		Type string `json:"type"`
	}

	tests := []map[string]any{
		{"to": "worker", "message": structuredFixture{Type: "shutdown_request"}},
		{"to": "worker", "message": map[string]any{"type": "shutdown_request", "extra": true}},
		{"to": "worker", "message": map[string]any{"type": "shutdown_request", "reason": 7}},
		{"to": teamLeadName, "message": map[string]any{
			"type": "shutdown_response", "request_id": "request-1", "approve": "true",
		}},
		{"to": teamLeadName, "message": map[string]any{
			"type": "shutdown_response", "request_id": "request-1", "approve": "false",
		}},
	}
	tool := NewSendMessageTool(nil, nil, nil)
	for _, input := range tests {
		result, err := tool.Execute(context.Background(), input)
		if err != nil {
			t.Fatalf("Execute(%#v) error = %v", input, err)
		}
		if !result.IsError || result.Outcome != types.ToolOutcomeFailed {
			t.Fatalf("Execute(%#v) = %#v", input, result)
		}
	}
}
