package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/loop"
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
	t.Cleanup(background.Shutdown)
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
	t.Cleanup(background.Shutdown)
	reg := registry.New()
	reg.Register(&FileReadTool{})
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

func TestSendMessageResumesCompletedLocalAgentSession(t *testing.T) {
	background := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(background.Shutdown)
	tool := &AgentTool{
		Provider:   &mockProvider{responses: []string{"first response", "second response"}},
		Registry:   registry.New(),
		Background: background,
	}

	agentResult, err := tool.Execute(context.Background(), agentExecuteInput("initial task", map[string]any{
		"name": "helper",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agentResult.IsError {
		t.Fatalf("unexpected agent error: %s", agentResult.Content)
	}
	if !strings.Contains(agentResult.Content, "first response") {
		t.Fatalf("expected initial response in agent output, got: %s", agentResult.Content)
	}
	var completed agentCompletedToolResult
	if err := json.Unmarshal([]byte(agentResult.Content), &completed); err != nil {
		t.Fatalf("expected JSON completed agent result, got %q: %v", agentResult.Content, err)
	}
	if completed.Status != "completed" || len(completed.Content) != 1 || completed.Content[0].Text != "first response" {
		t.Fatalf("unexpected completed agent payload: %+v", completed)
	}

	agentID := extractAgentIDFromToolResult(agentResult.Content)
	if agentID == "" {
		t.Fatalf("expected agent id in output, got: %s", agentResult.Content)
	}

	manager := newTestManager(t)
	manager.Background = background
	sendTool := NewSendMessageTool(manager)

	sendResult, err := sendTool.Execute(context.Background(), map[string]any{
		"to":      "helper",
		"summary": "continue the helper task",
		"message": "follow up",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sendResult.IsError {
		t.Fatalf("unexpected send message error: %s", sendResult.Content)
	}
	if !strings.Contains(sendResult.Content, `resumed it in the background`) {
		t.Fatalf("expected resume message, got: %s", sendResult.Content)
	}

	deadline := time.Now().Add(2 * time.Second)
	var snap BackgroundTaskSnapshot
	var ok bool
	for time.Now().Before(deadline) {
		snap, ok = background.Snapshot(agentID)
		if ok && snap.Status == "completed" && snap.Result == "second response" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ok {
		t.Fatalf("expected background snapshot for %s", agentID)
	}
	if snap.Status != "completed" {
		t.Fatalf("expected completed status after resume, got %s", snap.Status)
	}
	if snap.Result != "second response" {
		t.Fatalf("expected resumed result %q, got %q", "second response", snap.Result)
	}

	output, err := readBackgroundTaskOutput(snap.OutputPath, 64*1024)
	if err != nil {
		t.Fatalf("readBackgroundTaskOutput: %v", err)
	}
	if !strings.Contains(output.Content, "first response") || !strings.Contains(output.Content, "second response") {
		t.Fatalf("expected output file to contain both responses, got: %s", output.Content)
	}
}

func TestSendMessageResumeCapturesCurrentExecutionLineage(t *testing.T) {
	background := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(background.Shutdown)
	tool := &AgentTool{
		Provider:   &mockProvider{responses: []string{"first response", "second response"}},
		Registry:   registry.New(),
		Background: background,
	}
	initialCtx := loop.WithToolExecutionContext(context.Background(), loop.ToolExecutionContext{
		TurnID: "turn-initial", RunID: "parent-initial", BatchID: "batch-initial", ActorID: "lead", ActorType: "agent", AgentPath: "lead",
	})
	agentResult, err := tool.Execute(initialCtx, agentExecuteInput("initial task", map[string]any{"name": "lineage-helper"}))
	if err != nil || agentResult.IsError {
		t.Fatalf("initial Agent err=%v result=%+v", err, agentResult)
	}
	completed, ok := agentResult.Data.(AgentCompleted)
	if !ok || completed.AgentID == "" {
		t.Fatalf("initial result=%+v (%T)", agentResult, agentResult.Data)
	}

	manager := newTestManager(t)
	manager.Background = background
	resumeCtx := loop.WithToolExecutionContext(context.Background(), loop.ToolExecutionContext{
		TurnID: "turn-resume", RunID: "parent-resume", BatchID: "batch-resume", ActorID: "reviewer", ActorType: "agent", AgentPath: "reviewer",
	})
	sendResult, err := NewSendMessageTool(manager).Execute(resumeCtx, map[string]any{
		"to": "lineage-helper", "summary": "continue with reviewer lineage", "message": "follow up",
	})
	if err != nil || sendResult.IsError {
		t.Fatalf("SendMessage err=%v result=%+v", err, sendResult)
	}

	deadline := time.Now().Add(2 * time.Second)
	var snapshot BackgroundTaskSnapshot
	for time.Now().Before(deadline) {
		snapshot, ok = background.Snapshot(completed.AgentID)
		if ok && snapshot.Status == "completed" && snapshot.Attempt >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ok || len(snapshot.Runs) < 2 {
		t.Fatalf("resumed snapshot=%+v ok=%v", snapshot, ok)
	}
	resumed := snapshot.Runs[len(snapshot.Runs)-1]
	if resumed.BatchID != "batch-resume" || resumed.ParentRunID != "parent-resume" || resumed.AgentPath != "reviewer/"+completed.AgentID {
		t.Fatalf("SendMessage resume lost current lineage: %+v", resumed)
	}
	if resumed.BatchID == snapshot.Runs[0].BatchID || resumed.ParentRunID == snapshot.Runs[0].ParentRunID {
		t.Fatalf("resumed run reused stale lineage: first=%+v resumed=%+v", snapshot.Runs[0], resumed)
	}
}

func TestSendMessageRejectsCrossProcessAgentResumeWithoutTrustedSnapshot(t *testing.T) {
	root := t.TempDir()
	background := NewBackgroundTaskManager(root)
	tool := &AgentTool{
		Provider:   &mockProvider{responses: []string{"first response"}},
		Registry:   registry.New(),
		Background: background,
	}
	background.SetAgentSessionFactory(tool.RestoreAgentSessionFromRecord)

	agentResult, err := tool.Execute(context.Background(), agentExecuteInput("initial task", map[string]any{
		"name": "helper",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agentResult.IsError {
		t.Fatalf("unexpected agent error: %s", agentResult.Content)
	}
	agentID := extractAgentIDFromToolResult(agentResult.Content)
	if agentID == "" {
		t.Fatalf("expected agent id in output, got: %s", agentResult.Content)
	}
	background.Shutdown()

	restoredBackground := NewBackgroundTaskManager(root)
	t.Cleanup(restoredBackground.Shutdown)
	restoredTool := &AgentTool{
		Provider:   &mockProvider{responses: []string{"second response"}},
		Registry:   registry.New(),
		Background: restoredBackground,
	}
	restoredBackground.SetAgentSessionFactory(restoredTool.RestoreAgentSessionFromRecord)

	manager := newTestManager(t)
	manager.Background = restoredBackground
	sendTool := NewSendMessageTool(manager)

	sendResult, err := sendTool.Execute(context.Background(), map[string]any{
		"to":      "helper",
		"summary": "continue after session restart",
		"message": "follow up after restart",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(sendResult.Content, "could not be resumed") || !strings.Contains(sendResult.Content, "untrusted or modified") {
		t.Fatalf("expected fail-closed persisted resume error, got: %s", sendResult.Content)
	}
	restoredBackground.mu.Lock()
	_, live := restoredBackground.sessions[agentID]
	restoredBackground.mu.Unlock()
	if live {
		t.Fatal("untrusted persisted task was registered as a live session")
	}
}
