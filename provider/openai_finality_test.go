package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

type openAIChatFinalityResult struct {
	events []types.StreamEvent
	stop   *types.StreamEvent
	apiErr *types.APIError
	starts int
	stops  int
}

func runOpenAIChatFinalityStream(t *testing.T, frames []string, done bool) openAIChatFinalityResult {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		for _, frame := range frames {
			_, _ = io.WriteString(writer, "data: "+frame+"\n\n")
		}
		if done {
			_, _ = io.WriteString(writer, "data: [DONE]\n\n")
		}
	}))
	t.Cleanup(server.Close)

	stream, err := NewOpenAI(Config{
		ProviderName: "custom", APIKey: "test-key", BaseURL: server.URL, Model: "test-model",
	}).CreateStream(context.Background(), Params{Messages: []types.Message{types.UserMessage("hello")}})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	result := openAIChatFinalityResult{}
	for event := range stream {
		result.events = append(result.events, event)
		switch event.Type {
		case types.EventContentBlockStart:
			result.starts++
		case types.EventContentBlockStop:
			result.stops++
		case types.EventMessageStop:
			copy := event
			result.stop = &copy
		case types.EventError:
			result.apiErr = event.Error
		}
	}
	return result
}

func chatChunk(delta, finishReason string, choiceIndex int) string {
	return fmt.Sprintf(
		`{"id":"chatcmpl_finality","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[{"index":%d,"delta":%s,"finish_reason":%s}]}`,
		choiceIndex, delta, finishReason,
	)
}

func TestOpenAIChatFinalityRequiresTerminalAndDoneSentinel(t *testing.T) {
	tests := []struct {
		name      string
		frames    []string
		done      bool
		wantStop  bool
		wantPoint types.ProviderFailurePoint
	}{
		{
			name:   "stop plus done commits",
			frames: []string{chatChunk(`{"role":"assistant","content":"ok"}`, `"stop"`, 0)},
			done:   true, wantStop: true,
		},
		{
			name:      "clean EOF after finish reason does not commit",
			frames:    []string{chatChunk(`{"role":"assistant","content":"partial"}`, `"stop"`, 0)},
			wantPoint: types.ProviderFailureChatEOFBeforeTerminal,
		},
		{
			name:   "done without finish reason does not commit",
			frames: []string{chatChunk(`{"role":"assistant","content":"partial"}`, `null`, 0)},
			done:   true, wantPoint: types.ProviderFailureChatEOFBeforeTerminal,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runOpenAIChatFinalityStream(t, test.frames, test.done)
			if got := result.stop != nil; got != test.wantStop {
				t.Fatalf("MessageStop present = %v, want %v; events=%#v", got, test.wantStop, result.events)
			}
			if test.wantPoint == "" {
				if result.apiErr != nil {
					t.Fatalf("unexpected stream error: %#v", result.apiErr)
				}
				if result.stop.ProviderCommitReceipt == nil || result.stop.ProviderCommitReceipt.ResponseStatus != "completed" {
					t.Fatalf("commit receipt = %#v", result.stop.ProviderCommitReceipt)
				}
				return
			}
			if result.apiErr == nil || result.apiErr.FailureDiagnostic == nil || result.apiErr.FailureDiagnostic.FailurePoint != test.wantPoint {
				t.Fatalf("failure = %#v, want point %q", result.apiErr, test.wantPoint)
			}
		})
	}
}

func TestOpenAIChatFinalityAuthorizesOnlyToolCallsFinishReason(t *testing.T) {
	toolDelta := `{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"Inspect","arguments":"{}"}}]}`
	for _, finishReason := range []string{`"stop"`, `"length"`, `"content_filter"`} {
		t.Run(finishReason, func(t *testing.T) {
			result := runOpenAIChatFinalityStream(t, []string{chatChunk(toolDelta, finishReason, 0)}, true)
			if result.stop != nil {
				t.Fatalf("unsafe tool response committed: %#v", result.stop)
			}
			if result.apiErr == nil || result.apiErr.FailureDiagnostic == nil || result.apiErr.FailureDiagnostic.FailurePoint != types.ProviderFailureChatFinishReasonMismatch {
				t.Fatalf("failure = %#v", result.apiErr)
			}
		})
	}

	result := runOpenAIChatFinalityStream(t, []string{chatChunk(toolDelta, `"tool_calls"`, 0)}, true)
	if result.apiErr != nil || result.stop == nil {
		t.Fatalf("completed tool stream did not commit: stop=%#v err=%#v", result.stop, result.apiErr)
	}
	want := types.NewProviderToolCommitReceipt("custom", "chat-completions", "completed", []types.ProviderToolCallCommit{{
		OutputIndex: 0, ToolType: types.ToolDefinitionTypeFunction, CallID: "call_1", Name: "Inspect", RawInput: "{}",
	}})
	got := result.stop.ProviderCommitReceipt
	if got == nil || !got.ToolsAuthorized || got.ToolCalls != 1 || got.ToolBatchDigest != want.ToolBatchDigest || got.ToolBatchBytes != want.ToolBatchBytes {
		t.Fatalf("commit receipt = %#v, want %#v", got, want)
	}
	if result.starts != 1 || result.stops != 1 {
		t.Fatalf("tool block lifecycle starts/stops = %d/%d, want 1/1", result.starts, result.stops)
	}
}

func TestOpenAIChatFinalityCommitsTextLengthWithoutAuthorizingTools(t *testing.T) {
	result := runOpenAIChatFinalityStream(t, []string{chatChunk(`{"role":"assistant","content":"truncated"}`, `"length"`, 0)}, true)
	if result.apiErr != nil || result.stop == nil {
		t.Fatalf("length terminal did not produce a non-tool commit: stop=%#v err=%#v", result.stop, result.apiErr)
	}
	receipt := result.stop.ProviderCommitReceipt
	if receipt == nil || receipt.ResponseStatus != "incomplete" || receipt.ToolsAuthorized || receipt.ToolCalls != 0 {
		t.Fatalf("length receipt = %#v", receipt)
	}
}

func TestOpenAIChatFinalityRejectsIdentityDriftAndLateDelta(t *testing.T) {
	tests := []struct {
		name      string
		frames    []string
		wantPoint types.ProviderFailurePoint
	}{
		{
			name:      "first tool delta requires stable identity",
			frames:    []string{chatChunk(`{"role":"assistant","tool_calls":[{"index":0,"type":"function","function":{"arguments":"{}"}}]}`, `"tool_calls"`, 0)},
			wantPoint: types.ProviderFailureChatIdentityConflict,
		},
		{
			name: "later tool id cannot change",
			frames: []string{
				chatChunk(`{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"Inspect","arguments":"{"}}]}`, `null`, 0),
				chatChunk(`{"tool_calls":[{"index":0,"id":"call_2","function":{"arguments":"}"}}]}`, `"tool_calls"`, 0),
			},
			wantPoint: types.ProviderFailureChatIdentityConflict,
		},
		{
			name: "later tool name cannot change",
			frames: []string{
				chatChunk(`{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"Inspect","arguments":"{"}}]}`, `null`, 0),
				chatChunk(`{"tool_calls":[{"index":0,"function":{"name":"Run","arguments":"}"}}]}`, `"tool_calls"`, 0),
			},
			wantPoint: types.ProviderFailureChatIdentityConflict,
		},
		{
			name: "duplicate call id at another index is rejected",
			frames: []string{
				chatChunk(`{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"Inspect","arguments":"{}"}}]}`, `null`, 0),
				chatChunk(`{"tool_calls":[{"index":1,"id":"call_1","type":"function","function":{"name":"Run","arguments":"{}"}}]}`, `"tool_calls"`, 0),
			},
			wantPoint: types.ProviderFailureChatIdentityConflict,
		},
		{
			name: "choice index cannot change",
			frames: []string{
				chatChunk(`{"role":"assistant","content":"a"}`, `null`, 0),
				chatChunk(`{"content":"b"}`, `"stop"`, 1),
			},
			wantPoint: types.ProviderFailureChatIdentityConflict,
		},
		{
			name: "delta after terminal is rejected",
			frames: []string{
				chatChunk(`{"role":"assistant","content":"done"}`, `"stop"`, 0),
				chatChunk(`{"content":"late"}`, `null`, 0),
			},
			wantPoint: types.ProviderFailureChatLateDelta,
		},
		{
			name: "non usage empty chunk after terminal is rejected",
			frames: []string{
				chatChunk(`{"role":"assistant","content":"done"}`, `"stop"`, 0),
				`{"id":"chatcmpl_finality","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[]}`,
			},
			wantPoint: types.ProviderFailureChatLateDelta,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runOpenAIChatFinalityStream(t, test.frames, true)
			if result.stop != nil {
				t.Fatalf("protocol violation committed: %#v", result.stop)
			}
			if result.apiErr == nil || result.apiErr.FailureDiagnostic == nil || result.apiErr.FailureDiagnostic.FailurePoint != test.wantPoint {
				t.Fatalf("failure = %#v, want point %q", result.apiErr, test.wantPoint)
			}
		})
	}
}

func TestOpenAIChatFinalityRejectsOversizedToolInput(t *testing.T) {
	arguments := strings.Repeat("x", maxResponsesInspectToolInputBytes+1)
	delta := `{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"Inspect","arguments":"` + arguments + `"}}]}`
	result := runOpenAIChatFinalityStream(t, []string{chatChunk(delta, `"tool_calls"`, 0)}, true)
	if result.stop != nil {
		t.Fatalf("oversized input committed: %#v", result.stop)
	}
	if result.apiErr == nil || result.apiErr.Code != "tool_arguments_too_large" || result.apiErr.FailureDiagnostic == nil || result.apiErr.FailureDiagnostic.FailurePoint != types.ProviderFailureChatToolInputLimit {
		t.Fatalf("failure = %#v", result.apiErr)
	}
}

func TestOpenAIChatDoneCaptureRequiresDedicatedSSEDataLine(t *testing.T) {
	for _, test := range []struct {
		name   string
		chunks []string
		want   bool
	}{
		{name: "complete line", chunks: []string{"data: [DONE]\n"}, want: true},
		{name: "split line", chunks: []string{"data: [DO", "NE]\r\n"}, want: true},
		{name: "quoted content", chunks: []string{`data: {"content":"data: [DONE]"}` + "\n"}},
		{name: "comment", chunks: []string{": data: [DONE]\n"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			capture := &retryAfterCapture{}
			body := &openAIChatDoneCaptureBody{ReadCloser: io.NopCloser(strings.NewReader(strings.Join(test.chunks, ""))), capture: capture}
			buffer := make([]byte, 3)
			for {
				_, err := body.Read(buffer)
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			if got := capture.sawDone(); got != test.want {
				t.Fatalf("sawDone = %v, want %v", got, test.want)
			}
		})
	}
}
