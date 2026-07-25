package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	toolfile "github.com/agent-dance/luban/internal/tools/file"
	"github.com/agent-dance/luban/registry"
)

func extractAgentIDFromToolResult(content string) string {
	var payload struct {
		AgentID      string `json:"agentId"`
		AgentIDSnake string `json:"agent_id"`
	}
	if err := json.Unmarshal([]byte(content), &payload); err == nil {
		if strings.TrimSpace(payload.AgentID) != "" {
			return strings.TrimSpace(payload.AgentID)
		}
		if strings.TrimSpace(payload.AgentIDSnake) != "" {
			return strings.TrimSpace(payload.AgentIDSnake)
		}
	}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "agentId: ") {
			continue
		}
		value := strings.TrimPrefix(line, "agentId: ")
		if idx := strings.Index(value, " "); idx >= 0 {
			value = value[:idx]
		}
		return strings.TrimSpace(value)
	}
	return ""
}

func TestAgentBackgroundLaunchUsesPersistentAgentID(t *testing.T) {
	background := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(func() { _ = background.Shutdown(context.Background()) })
	tool := &AgentTool{
		Provider:   &mockProvider{responses: []string{"async done"}},
		Registry:   registry.New(),
		Background: background,
	}

	result, err := tool.Execute(context.Background(), agentExecuteInput("do work", map[string]any{
		"run_in_background": true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	var launch agentAsyncToolResult
	if err := json.Unmarshal([]byte(result.Content), &launch); err != nil {
		t.Fatalf("expected JSON async launch result, got %q: %v", result.Content, err)
	}
	if !launch.IsAsync || launch.Status != "async_launched" || launch.OutputFile == "" {
		t.Fatalf("unexpected async launch payload: %+v", launch)
	}
	if launch.CanReadOutputFile {
		t.Fatalf("expected unreadable output flag without Read/Bash tools, got %+v", launch)
	}

	agentID := extractAgentIDFromToolResult(result.Content)
	if !strings.HasPrefix(agentID, "agent-") {
		t.Fatalf("expected persistent agent id, got %q from %q", agentID, result.Content)
	}
	if _, ok := background.Snapshot(agentID); !ok {
		t.Fatalf("expected background manager to track %q", agentID)
	}

	snap, status := background.Wait(agentID, 2*time.Second)
	if status != "success" {
		t.Fatalf("expected background task to finish successfully, got %s", status)
	}
	if snap.Status != "completed" {
		t.Fatalf("expected completed status, got %s", snap.Status)
	}
}

func TestAgentBackgroundLaunchMarksReadableOutputWhenParentCanRead(t *testing.T) {
	background := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(func() { _ = background.Shutdown(context.Background()) })
	reg := registry.New()
	reg.Register(&toolfile.FileReadTool{})
	tool := &AgentTool{
		Provider:   &mockProvider{responses: []string{"async done"}},
		Registry:   reg,
		Background: background,
	}

	result, err := tool.Execute(context.Background(), agentExecuteInput("do work", map[string]any{
		"run_in_background": true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	var launch agentAsyncToolResult
	if err := json.Unmarshal([]byte(result.Content), &launch); err != nil {
		t.Fatalf("expected JSON async launch result, got %q: %v", result.Content, err)
	}
	if !launch.CanReadOutputFile {
		t.Fatalf("expected readable output flag with Read tool, got %+v", launch)
	}
	if launch.AgentID == "" {
		t.Fatalf("expected agent id in launch payload: %+v", launch)
	}

	if _, status := background.Wait(launch.AgentID, 2*time.Second); status != "success" {
		t.Fatalf("expected background task to finish successfully, got %s", status)
	}
}
