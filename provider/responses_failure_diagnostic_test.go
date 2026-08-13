package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestResponsesHTTPFailureDiagnosticPreservesSafeHandshakeEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("x-request-id", "upstream-http-503")
		http.Error(writer, `{"error":{"message":"private upstream prose"}}`, http.StatusServiceUnavailable)
	}))
	defer server.Close()

	responses := NewResponses(Config{
		ProviderName: "deepseek", BaseURL: server.URL, Model: "deepseek-v4-flash",
		ResponsesSemantics: ResponsesSemanticsDeepSeek,
	})
	ctx := WithLocalProviderRequestID(context.Background(), "local-http-503")
	_, err := responses.CreateStream(ctx, Params{
		Model: "deepseek-v4-flash", Messages: []types.Message{types.UserMessage("private prompt")}, MaxTokens: 128,
	})
	if err == nil {
		t.Fatal("expected HTTP error")
	}
	apiErr, ok := AsAPIError(err)
	if !ok || apiErr.FailureDiagnostic == nil {
		t.Fatalf("HTTP error omitted failure diagnostic: %#v", err)
	}
	diagnostic := apiErr.FailureDiagnostic
	if diagnostic.FailurePoint != types.ProviderFailureRequestHTTPStatus || diagnostic.HTTPStatus != http.StatusServiceUnavailable ||
		diagnostic.LocalRequestID != "local-http-503" || diagnostic.UpstreamRequestID != "upstream-http-503" {
		t.Fatalf("HTTP failure diagnostic = %+v", diagnostic)
	}
	if diagnostic.Stage != types.ProviderErrorStageHeaders || diagnostic.Class != types.ProviderErrorClassOverload ||
		diagnostic.ReplaySafety != types.ProviderReplaySafe {
		t.Fatalf("HTTP failure contract = %+v", diagnostic)
	}
	encoded, marshalErr := json.Marshal(diagnostic)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	for _, private := range []string{"private upstream prose", "private prompt"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("HTTP diagnostic leaked %q: %s", private, encoded)
		}
	}
}

func collectResponsesDiagnosticEvents(t *testing.T, sse string) []types.StreamEvent {
	t.Helper()
	ctx := WithLocalProviderRequestID(context.Background(), "local-request-17")
	ctx = withProviderFailureDiagnostic(ctx, baseResponsesFailureDiagnostic(
		ctx, "deepseek", "deepseek-v4-flash", "https", "https://gateway.example/private/responses?token=secret", "upstream-42",
	))
	ch := make(chan types.StreamEvent, 32)
	go func() {
		defer close(ch)
		processResponsesStreamForRequest(ctx, strings.NewReader(sse), ch, "deepseek-v4-flash", ResponsesSemanticsDeepSeek, false)
	}()
	var events []types.StreamEvent
	for event := range ch {
		events = append(events, event)
	}
	return events
}

func terminalDiagnostic(t *testing.T, events []types.StreamEvent) *types.ProviderFailureDiagnostic {
	t.Helper()
	for _, event := range events {
		if event.Type == types.EventError && event.Error != nil {
			if event.Error.FailureDiagnostic == nil {
				t.Fatal("terminal API error omitted failure diagnostic")
			}
			return event.Error.FailureDiagnostic
		}
	}
	t.Fatal("stream omitted terminal API error")
	return nil
}

func TestResponsesFailureDiagnosticDistinguishesProtocolBranches(t *testing.T) {
	tests := []struct {
		name  string
		sse   string
		point types.ProviderFailurePoint
	}{
		{
			name:  "known event parse",
			sse:   "event: response.created\ndata: {\"response\":\n\n",
			point: types.ProviderFailureSSEKnownEventParse,
		},
		{
			name: "function delta without item",
			sse: strings.Join([]string{
				"event: response.created\ndata: {\"response\":{\"id\":\"resp-safe\"}}\n",
				"event: response.function_call_arguments.delta\ndata: {\"output_index\":2,\"delta\":\"SECRET_TOOL_INPUT\"}\n",
			}, "\n") + "\n",
			point: types.ProviderFailureFunctionDeltaWithoutItem,
		},
		{
			name: "function done missing arguments",
			sse: strings.Join([]string{
				"event: response.created\ndata: {\"response\":{\"id\":\"resp-safe\"}}\n",
				"event: response.output_item.added\ndata: {\"output_index\":0,\"item\":{\"type\":\"function_call\",\"id\":\"item-1\",\"call_id\":\"call-1\",\"name\":\"Run\",\"status\":\"in_progress\"}}\n",
				"event: response.function_call_arguments.done\ndata: {\"output_index\":0,\"item_id\":\"item-1\"}\n",
			}, "\n") + "\n",
			point: types.ProviderFailureFunctionDoneMissingArguments,
		},
		{
			name: "function invalid status",
			sse: strings.Join([]string{
				"event: response.created\ndata: {\"response\":{\"id\":\"resp-safe\"}}\n",
				"event: response.output_item.added\ndata: {\"output_index\":0,\"item\":{\"type\":\"function_call\",\"id\":\"item-1\",\"call_id\":\"call-1\",\"name\":\"Run\",\"status\":\"in_progress\"}}\n",
				"event: response.output_item.done\ndata: {\"output_index\":0,\"item\":{\"type\":\"function_call\",\"id\":\"item-1\",\"call_id\":\"call-1\",\"name\":\"Run\",\"arguments\":\"{}\",\"status\":\"failed\"}}\n",
			}, "\n") + "\n",
			point: types.ProviderFailureFunctionInvalidStatus,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagnostic := terminalDiagnostic(t, collectResponsesDiagnosticEvents(t, test.sse))
			if diagnostic.FailurePoint != test.point {
				t.Fatalf("failure point = %q, want %q; diagnostic=%+v", diagnostic.FailurePoint, test.point, diagnostic)
			}
			if diagnostic.LocalRequestID != "local-request-17" || diagnostic.UpstreamRequestID != "upstream-42" ||
				diagnostic.Provider != "deepseek" || diagnostic.APIFormat != "responses" || diagnostic.Transport != "https" {
				t.Fatalf("correlation/request identity = %+v", diagnostic)
			}
			if diagnostic.Endpoint != "https://gateway.example/…/responses" || diagnostic.WireSequence == 0 || diagnostic.DataBytes == 0 {
				t.Fatalf("safe wire checkpoint = %+v", diagnostic)
			}
			encoded, err := json.Marshal(diagnostic)
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{"SECRET_TOOL_INPUT", "token=secret", "private/responses"} {
				if strings.Contains(string(encoded), secret) {
					t.Fatalf("safe diagnostic leaked %q: %s", secret, encoded)
				}
			}
		})
	}
}

func TestDeepSeekIncompleteFunctionItemDefersToResponseMaxTokens(t *testing.T) {
	sse := strings.Join([]string{
		"event: response.created\ndata: {\"response\":{\"id\":\"resp-cutoff\"}}\n",
		"event: response.output_item.added\ndata: {\"output_index\":0,\"item\":{\"type\":\"function_call\",\"id\":\"item-1\",\"call_id\":\"call-1\",\"name\":\"ApplyPatch\",\"status\":\"in_progress\"}}\n",
		"event: response.output_item.done\ndata: {\"output_index\":0,\"item\":{\"type\":\"function_call\",\"id\":\"item-1\",\"call_id\":\"call-1\",\"name\":\"ApplyPatch\",\"arguments\":\"{\\\"patch\\\":\\\"PARTIAL_SECRET\\\"\",\"status\":\"incomplete\"}}\n",
		"event: response.incomplete\ndata: {\"response\":{\"id\":\"resp-cutoff\",\"model\":\"deepseek-v4-flash\",\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"},\"usage\":{\"input_tokens\":10,\"output_tokens\":16384},\"output\":[]}}\n",
	}, "\n") + "\n"
	events := collectResponsesDiagnosticEvents(t, sse)
	var stopped, toolReceipt, failed, maxTokens bool
	for _, event := range events {
		failed = failed || event.Type == types.EventError
		if event.Type == types.EventMessageStop {
			stopped = true
			toolReceipt = event.ProviderCommitReceipt != nil
		}
		maxTokens = maxTokens || event.Type == types.EventMessageDelta && event.StopReason != nil && *event.StopReason == types.StopReasonMaxTokens
	}
	if failed || !stopped || !maxTokens || toolReceipt {
		t.Fatalf("incomplete tool item events = %#v", events)
	}
}
