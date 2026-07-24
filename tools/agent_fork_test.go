package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestAgentToolForkGateFallsBackToGeneralPurposeWhenDisabled(t *testing.T) {
	t.Setenv("FORK_SUBAGENT", "")
	t.Setenv("CLAUDE_CODE_FORK_SUBAGENT", "")
	provider := &captureAgentProvider{responses: []string{"regular done"}}
	tool := &AgentTool{
		Provider: provider,
		Registry: registry.New(),
		System:   "parent system",
		Model:    "parent-model",
	}

	result, err := tool.Execute(context.Background(), agentExecuteInput("regular task", nil))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected ordinary Agent success, got %s", result.Content)
	}
	if len(provider.params) != 1 {
		t.Fatalf("expected one provider call, got %d", len(provider.params))
	}
	params := provider.params[0]
	if params.Model != "parent-model" {
		t.Fatalf("expected ordinary Agent to inherit configured model, got %q", params.Model)
	}
	if !strings.Contains(params.System, "Complete the task fully") {
		t.Fatalf("expected general-purpose system prompt, got %q", params.System)
	}
	if strings.Contains(params.System, "<"+forkBoilerplateTag+">") {
		t.Fatalf("fork boilerplate leaked into ordinary Agent system prompt: %q", params.System)
	}
	if len(params.Messages) != 1 || !strings.Contains(params.Messages[0].GetText(), "regular task") {
		t.Fatalf("expected fresh ordinary Agent prompt, got %#v", params.Messages)
	}
}

func TestAgentToolForkGateExplicitGeneralPurposeIsFreshWhenEnabled(t *testing.T) {
	t.Setenv("FORK_SUBAGENT", "1")
	t.Setenv("CLAUDE_CODE_FORK_SUBAGENT", "")
	provider := &captureAgentProvider{responses: []string{"explicit general done"}}
	tool := &AgentTool{
		Provider: provider,
		Registry: registry.New(),
		System:   "tool system",
		Model:    "fallback-model",
	}
	ctx := loop.WithToolExecutionContext(context.Background(), loop.ToolExecutionContext{
		Messages: []types.Message{types.UserMessage("parent context that must not be inherited")},
		System:   "parent system",
		Model:    "parent-model",
	})

	result, err := tool.Execute(ctx, agentExecuteInput("fresh general task", map[string]any{
		"subagent_type": "general-purpose",
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected explicit general-purpose success, got %s", result.Content)
	}
	if len(provider.params) != 1 {
		t.Fatalf("expected one provider call, got %d", len(provider.params))
	}
	params := provider.params[0]
	if params.Model != "parent-model" {
		t.Fatalf("expected explicit general-purpose Agent to inherit parent model, got %q", params.Model)
	}
	if params.System == "parent system" || !strings.Contains(params.System, "tool system") {
		t.Fatalf("expected fresh Agent system prompt, got %q", params.System)
	}
	allMessages := ""
	for _, message := range params.Messages {
		allMessages += "\n" + message.GetText()
	}
	if strings.Contains(allMessages, "parent context that must not be inherited") {
		t.Fatalf("explicit general-purpose Agent inherited fork-only parent context: %#v", params.Messages)
	}
	if strings.Contains(allMessages, "<"+forkBoilerplateTag+">") || strings.Contains(allMessages, forkDirectivePrefix) {
		t.Fatalf("explicit general-purpose Agent received fork directive: %#v", params.Messages)
	}
	if len(params.Messages) != 1 || !strings.Contains(params.Messages[0].GetText(), "fresh general task") {
		t.Fatalf("expected fresh prompt only, got %#v", params.Messages)
	}
}

func TestAgentToolForkLaunchDoesNotAuthorizeOutputPolling(t *testing.T) {
	t.Setenv("FORK_SUBAGENT", "1")
	t.Setenv("CLAUDE_CODE_FORK_SUBAGENT", "")
	background := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(background.Shutdown)
	provider := &captureAgentProvider{responses: []string{"fork result must arrive by runtime notification"}}
	reg := registry.New()
	reg.Register(&AgentTool{})
	reg.Register(&FileReadTool{})
	tool := &AgentTool{
		Provider:   provider,
		Registry:   reg,
		Background: background,
	}
	ctx := loop.WithToolExecutionContext(context.Background(), loop.ToolExecutionContext{
		Messages: []types.Message{types.UserMessage("parent context")},
		ToolUse: types.ToolUseBlock{
			Type:  types.ContentTypeToolUse,
			ID:    "toolu_agent",
			Name:  "Agent",
			Input: map[string]any{"prompt": "fork safely"},
		},
		Model: "parent-model",
	})

	result, err := tool.Execute(ctx, agentExecuteInput("fork safely", nil))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected fork launch success, got %s", result.Content)
	}
	if strings.Contains(result.Content, "fork result must arrive by runtime notification") {
		t.Fatalf("fork launch response included model-produced result before runtime completion: %s", result.Content)
	}
	var payload agentAsyncToolResult
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("parse fork launch payload: %v\n%s", err, result.Content)
	}
	if payload.OutputFile == "" {
		t.Fatalf("expected runtime output file to be registered, got %#v", payload)
	}
	if payload.CanReadOutputFile {
		t.Fatalf("fork launch must not authorize model-side output polling, got %#v", payload)
	}
	if !strings.Contains(payload.Message, "will notify when it completes") {
		t.Fatalf("expected runtime notification guidance, got %#v", payload)
	}
	snap, status := background.Wait(payload.AgentID, 2*time.Second)
	if status != "success" || snap.Status != "completed" {
		t.Fatalf("expected fork runtime task to complete, status=%s snap=%#v", status, snap)
	}
}

func TestAgentToolForkSubagentUsesExactParentToolDefinitions(t *testing.T) {
	t.Setenv("FORK_SUBAGENT", "1")
	t.Setenv("CLAUDE_CODE_FORK_SUBAGENT", "")
	background := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(background.Shutdown)
	provider := &captureAgentProvider{responses: []string{"fork done"}}
	reg := registry.New()
	reg.Register(&AgentTool{})
	reg.Register(&FileReadTool{})
	reg.Register(fakeTool{name: "CustomParentTool"})
	tool := &AgentTool{
		Provider:   provider,
		Registry:   reg,
		Background: background,
	}
	ctx := loop.WithToolExecutionContext(context.Background(), loop.ToolExecutionContext{
		Messages: []types.Message{types.UserMessage("parent context")},
		ToolUse: types.ToolUseBlock{
			Type:  types.ContentTypeToolUse,
			ID:    "toolu_agent",
			Name:  "Agent",
			Input: map[string]any{"prompt": "inspect exact tools"},
		},
	})

	result, err := tool.Execute(ctx, agentExecuteInput("inspect exact tools", nil))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected fork launch success, got %s", result.Content)
	}
	var payload agentAsyncToolResult
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("parse fork launch payload: %v\n%s", err, result.Content)
	}
	if _, status := background.Wait(payload.AgentID, 2*time.Second); status != "success" {
		t.Fatalf("expected fork runtime task to complete, status=%s", status)
	}
	if len(provider.params) != 1 {
		t.Fatalf("expected one provider call, got %d", len(provider.params))
	}
	got := map[string]bool{}
	for _, def := range provider.params[0].Tools {
		got[def.Name] = true
	}
	want := []string{"Agent", "Read", "CustomParentTool"}
	if len(got) != len(want) {
		t.Fatalf("expected exact parent tool set %v, got %#v", want, got)
	}
	for _, name := range want {
		if !got[name] {
			t.Fatalf("expected fork tool pool to contain %s, got %#v", name, got)
		}
	}
}

func TestForkWorktreeNoticeOnlyForForkWorktreeMetadata(t *testing.T) {
	cases := []struct {
		name     string
		metadata agentSessionMetadata
		want     bool
	}{
		{
			name: "ordinary worktree agent",
			metadata: agentSessionMetadata{
				AgentType:    "general-purpose",
				WorktreePath: "/repo/.claude/worktrees/agent",
			},
			want: false,
		},
		{
			name: "fork without worktree",
			metadata: agentSessionMetadata{
				AgentType: forkSubagentType,
			},
			want: false,
		},
		{
			name: "fork worktree",
			metadata: agentSessionMetadata{
				AgentType:    forkSubagentType,
				WorktreePath: "/repo/.claude/worktrees/agent",
			},
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldAppendForkWorktreeNotice(tc.metadata); got != tc.want {
				t.Fatalf("shouldAppendForkWorktreeNotice()=%v want %v", got, tc.want)
			}
		})
	}
}
