package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestResponsesProvider_TaskBudgetRequestTotalOnly(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("unmarshal request: %v\n%s", err, body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.completed\ndata: {\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1},\"output\":[]}}\n\n"))
	}))
	defer srv.Close()

	p := NewResponses(Config{APIKey: "test-key", BaseURL: srv.URL})
	_, err := p.CreateStream(context.Background(), Params{
		Messages:   []types.Message{types.UserMessage("hello")},
		TaskBudget: &TaskBudget{Total: 5000},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}

	taskBudget := capturedTaskBudget(t, capturedBody)
	if got := taskBudget["total"]; got != float64(5000) {
		t.Fatalf("task_budget.total = %#v, want 5000", got)
	}
	if _, ok := taskBudget["remaining"]; ok {
		t.Fatalf("task_budget.remaining should be omitted before compaction: %#v", taskBudget)
	}
}

func TestResponsesProvider_TaskBudgetRequestWithRemaining(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("unmarshal request: %v\n%s", err, body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.completed\ndata: {\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1},\"output\":[]}}\n\n"))
	}))
	defer srv.Close()

	remaining := 3210
	p := NewResponses(Config{APIKey: "test-key", BaseURL: srv.URL})
	_, err := p.CreateStream(context.Background(), Params{
		Messages:   []types.Message{types.UserMessage("hello")},
		TaskBudget: &TaskBudget{Total: 5000, Remaining: &remaining},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}

	taskBudget := capturedTaskBudget(t, capturedBody)
	if got := taskBudget["total"]; got != float64(5000) {
		t.Fatalf("task_budget.total = %#v, want 5000", got)
	}
	if got := taskBudget["remaining"]; got != float64(3210) {
		t.Fatalf("task_budget.remaining = %#v, want 3210", got)
	}
}

func TestResponsesProvider_TaskBudgetRequestOmittedWhenUnset(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("unmarshal request: %v\n%s", err, body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.completed\ndata: {\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1},\"output\":[]}}\n\n"))
	}))
	defer srv.Close()

	p := NewResponses(Config{APIKey: "test-key", BaseURL: srv.URL})
	_, err := p.CreateStream(context.Background(), Params{
		Messages: []types.Message{types.UserMessage("hello")},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	if outputConfig, ok := capturedBody["output_config"].(map[string]any); ok {
		if _, hasTaskBudget := outputConfig["task_budget"]; hasTaskBudget {
			t.Fatalf("task_budget should be omitted when unset: %#v", outputConfig)
		}
	}
}

func TestAnthropicProvider_TaskBudgetRequestWithRemaining(t *testing.T) {
	var capturedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("unmarshal request: %v\n%s", err, body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_123\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4-6\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n"))
		_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer srv.Close()

	remaining := 2500
	p := NewAnthropic(Config{
		AuthToken: "bearer-token",
		BaseURL:   srv.URL,
		Model:     "claude-sonnet-4-6",
	})
	ch, err := p.CreateStream(context.Background(), Params{
		Messages:   []types.Message{types.UserMessage("hello")},
		TaskBudget: &TaskBudget{Total: 5000, Remaining: &remaining},
	})
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	for range ch {
	}

	taskBudget := capturedTaskBudget(t, capturedBody)
	if got := taskBudget["total"]; got != float64(5000) {
		t.Fatalf("task_budget.total = %#v, want 5000", got)
	}
	if got := taskBudget["remaining"]; got != float64(2500) {
		t.Fatalf("task_budget.remaining = %#v, want 2500", got)
	}
}

func capturedTaskBudget(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	outputConfig, ok := body["output_config"].(map[string]any)
	if !ok {
		t.Fatalf("output_config = %#v, want object", body["output_config"])
	}
	taskBudget, ok := outputConfig["task_budget"].(map[string]any)
	if !ok {
		t.Fatalf("output_config.task_budget = %#v, want object", outputConfig["task_budget"])
	}
	return taskBudget
}
