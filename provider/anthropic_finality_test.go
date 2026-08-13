package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestAnthropicToolCommitRequiresEveryBlockClosed(t *testing.T) {
	stream := strings.Join([]string{
		anthropicSSE("message_start", `{"type":"message_start","message":{"id":"msg_open","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}`),
		anthropicSSE("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_1","name":"Inspect","input":{}}}`),
		anthropicSSE("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`),
		anthropicSSE("content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`),
		anthropicSSE("content_block_stop", `{"type":"content_block_stop","index":1}`),
		anthropicSSE("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":2}}`),
		anthropicSSE("message_stop", `{"type":"message_stop"}`),
	}, "")

	events, _ := collectAnthropicProtocolEvents(t, stream, nil)
	assertAnthropicProtocolFailure(t, events, types.ProviderFailureAnthropicOpenBlock)
}

func TestAnthropicToolCommitReceiptBindsOrderedClosedPayloads(t *testing.T) {
	stream := strings.Join([]string{
		anthropicSSE("message_start", `{"type":"message_start","message":{"id":"msg_ok","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}`),
		anthropicSSE("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_1","name":"Inspect","input":{}}}`),
		anthropicSSE("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"requests\":[]}"}}`),
		anthropicSSE("content_block_stop", `{"type":"content_block_stop","index":0}`),
		anthropicSSE("content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tool_2","name":"Run","input":{}}}`),
		anthropicSSE("content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"steps\":[]}"}}`),
		anthropicSSE("content_block_stop", `{"type":"content_block_stop","index":1}`),
		anthropicSSE("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":2}}`),
		anthropicSSE("message_stop", `{"type":"message_stop"}`),
	}, "")

	events, _ := collectAnthropicProtocolEvents(t, stream, nil)
	var receipt *types.ProviderCommitReceipt
	for _, event := range events {
		if event.Type == types.EventMessageStop {
			receipt = event.ProviderCommitReceipt
		}
		if event.Type == types.EventError {
			t.Fatalf("valid tool stream failed: %+v", event.Error)
		}
	}
	want := types.NewProviderToolCommitReceipt("anthropic", "anthropic_messages", "completed", []types.ProviderToolCallCommit{
		{OutputIndex: 0, ToolType: types.ToolDefinitionTypeFunction, CallID: "tool_1", Name: "Inspect", RawInput: `{"requests":[]}`},
		{OutputIndex: 1, ToolType: types.ToolDefinitionTypeFunction, CallID: "tool_2", Name: "Run", RawInput: `{"steps":[]}`},
	})
	if receipt == nil || !receipt.ToolsAuthorized || receipt.ToolCalls != 2 ||
		receipt.ToolBatchBytes != want.ToolBatchBytes || receipt.ToolBatchDigest != want.ToolBatchDigest {
		t.Fatalf("commit receipt = %+v, want %+v", receipt, want)
	}
}

func TestAnthropicToolCommitRejectsNonToolTerminalReasons(t *testing.T) {
	for _, stopReason := range []string{"end_turn", "max_tokens"} {
		t.Run(stopReason, func(t *testing.T) {
			stream := strings.Join([]string{
				anthropicSSE("message_start", `{"type":"message_start","message":{"id":"msg_stop","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}`),
				anthropicSSE("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_1","name":"Run","input":{}}}`),
				anthropicSSE("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"steps\":[]}"}}`),
				anthropicSSE("content_block_stop", `{"type":"content_block_stop","index":0}`),
				anthropicSSE("message_delta", fmt.Sprintf(`{"type":"message_delta","delta":{"stop_reason":%q,"stop_sequence":null},"usage":{"output_tokens":2}}`, stopReason)),
				anthropicSSE("message_stop", `{"type":"message_stop"}`),
			}, "")

			events, _ := collectAnthropicProtocolEvents(t, stream, nil)
			assertAnthropicProtocolFailure(t, events, types.ProviderFailureAnthropicStopReasonMismatch)
		})
	}
}

func TestAnthropicDoesNotFallbackAfterMessageStart(t *testing.T) {
	stream := anthropicSSE("message_start", `{"type":"message_start","message":{"id":"msg_cut","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}`)
	var requests atomic.Int32
	events, gotRequests := collectAnthropicProtocolEvents(t, stream, &requests)
	if gotRequests != 1 {
		t.Fatalf("requests = %d, want one stream attempt and no non-streaming fallback", gotRequests)
	}
	assertAnthropicProtocolFailure(t, events, types.ProviderFailureAnthropicUnsafeFallback)
}

func TestAnthropicToolInputLimitFailsBeforeCommit(t *testing.T) {
	partial := strings.Repeat("x", maxResponsesInspectToolInputBytes+1)
	delta, err := json.Marshal(map[string]any{
		"type": "content_block_delta", "index": 0,
		"delta": map[string]any{"type": "input_json_delta", "partial_json": partial},
	})
	if err != nil {
		t.Fatal(err)
	}
	stream := strings.Join([]string{
		anthropicSSE("message_start", `{"type":"message_start","message":{"id":"msg_limit","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}`),
		anthropicSSE("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_1","name":"Inspect","input":{}}}`),
		anthropicSSE("content_block_delta", string(delta)),
	}, "")

	events, _ := collectAnthropicProtocolEvents(t, stream, nil)
	assertAnthropicProtocolFailure(t, events, types.ProviderFailureAnthropicToolInputLimit)
}

func collectAnthropicProtocolEvents(t *testing.T, streamBody string, requests *atomic.Int32) ([]types.StreamEvent, int32) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests != nil {
			requests.Add(1)
		}
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, streamBody)
	}))
	defer server.Close()

	p := NewAnthropic(Config{AuthToken: "test", BaseURL: server.URL, Model: "claude-sonnet-4-6"})
	stream, err := p.CreateStream(context.Background(), Params{Messages: []types.Message{types.UserMessage("test")}})
	if err != nil {
		t.Fatal(err)
	}
	events := make([]types.StreamEvent, 0)
	for event := range stream {
		events = append(events, event)
	}
	if requests == nil {
		return events, 1
	}
	return events, requests.Load()
}

func assertAnthropicProtocolFailure(t *testing.T, events []types.StreamEvent, want types.ProviderFailurePoint) {
	t.Helper()
	var got *types.APIError
	for _, event := range events {
		if event.Type == types.EventMessageStop {
			t.Fatalf("rejected stream emitted MessageStop with receipt %+v", event.ProviderCommitReceipt)
		}
		if event.Type == types.EventError {
			got = event.Error
		}
	}
	if got == nil || got.FailureDiagnostic == nil || got.FailureDiagnostic.FailurePoint != want {
		t.Fatalf("failure = %+v, want point %s", got, want)
	}
}

func anthropicSSE(eventType, data string) string {
	return "event: " + eventType + "\ndata: " + data + "\n\n"
}
