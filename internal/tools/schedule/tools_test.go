package schedule

import (
	"context"
	"encoding/json"
	"testing"
)

func TestCreateToolRejectsStringBooleans(t *testing.T) {
	service := newTestService(t, ExecutorFunc(func(context.Context, Execution) error { return nil }))
	tool := NewCreateTool(service)
	for _, field := range []string{"recurring", "durable"} {
		t.Run(field, func(t *testing.T) {
			input := map[string]any{"cron": "* * * * *", "prompt": "strict input"}
			input[field] = "false"
			result, err := tool.Execute(context.Background(), input)
			if err != nil {
				t.Fatalf("execute CronCreate: %v", err)
			}
			if !result.IsError || result.Metadata["error_code"] != "invalid_input" {
				t.Fatalf("string boolean result = %#v, want invalid_input", result)
			}
		})
	}
}

func TestCreateToolRejectsDurableTeammateJob(t *testing.T) {
	service := newTestService(t, ExecutorFunc(func(context.Context, Execution) error { return nil }))
	base := NewCreateTool(service).(*createTool)
	tool := base.BindAgentScope("teammate-1", service.root)
	result, err := tool.Execute(context.Background(), map[string]any{
		"cron": "* * * * *", "prompt": "durable teammate work", "durable": true,
	})
	if err != nil {
		t.Fatalf("execute CronCreate: %v", err)
	}
	if !result.IsError || result.Metadata["error_code"] != "durable_agent_denied" {
		t.Fatalf("durable teammate result = %#v, want durable_agent_denied", result)
	}
	jobs, listErr := service.list("")
	if listErr != nil {
		t.Fatalf("list after rejected create: %v", listErr)
	}
	if len(jobs) != 0 {
		t.Fatalf("rejected durable teammate job was stored: %#v", jobs)
	}
}

func TestCreateToolUsesExplicitBooleanDataAndSnakeCaseMetadata(t *testing.T) {
	service := newTestService(t, ExecutorFunc(func(context.Context, Execution) error { return nil }))
	result, err := NewCreateTool(service).Execute(context.Background(), map[string]any{
		"cron": "* * * * *", "prompt": "one shot", "recurring": false, "durable": false,
	})
	if err != nil {
		t.Fatalf("execute CronCreate: %v", err)
	}
	if result.IsError {
		t.Fatalf("CronCreate failed: %#v", result)
	}
	output, ok := result.Data.(createOutput)
	if !ok {
		t.Fatalf("CronCreate data type = %T, want createOutput", result.Data)
	}
	if output.Recurring || output.Durable {
		t.Fatalf("explicit false booleans changed: %#v", output)
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("marshal CronCreate data: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(encoded, &body); err != nil {
		t.Fatalf("decode CronCreate data: %v", err)
	}
	for _, field := range []string{"recurring", "durable"} {
		value, exists := body[field]
		if !exists || value != false {
			t.Fatalf("data field %q = %#v (present=%t), want explicit false", field, value, exists)
		}
	}
	if _, exists := body["next_fire"]; !exists {
		t.Fatalf("typed data lacks snake_case next_fire: %s", encoded)
	}
	if _, exists := body["nextFire"]; exists {
		t.Fatalf("typed data exposes camelCase compatibility key: %s", encoded)
	}
	for _, field := range []string{"id", "cron", "next_fire", "recurring", "durable"} {
		if _, exists := result.Metadata[field]; !exists {
			t.Fatalf("metadata lacks %q: %#v", field, result.Metadata)
		}
	}
	if result.Metadata["recurring"] != "false" || result.Metadata["durable"] != "false" {
		t.Fatalf("metadata booleans = %#v, want explicit false strings", result.Metadata)
	}
	if _, exists := result.Metadata["nextFire"]; exists {
		t.Fatalf("metadata exposes camelCase compatibility key: %#v", result.Metadata)
	}
	mapped := NewCreateTool(service).(*createTool).MapToolResultToToolResultBlock(result.Data, "tool-use-1")
	if mapped.Content != result.Content {
		t.Fatalf("mapped create result lost schedule scope: mapped=%q execute=%q", mapped.Content, result.Content)
	}
}
