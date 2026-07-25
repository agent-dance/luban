package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	mcpmanager "github.com/agent-dance/luban/internal/mcp/manager"
	toolfile "github.com/agent-dance/luban/internal/tools/file"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type task05CountingProvider struct {
	calls atomic.Int32
}

func (p *task05CountingProvider) Name() string    { return "task05-counting" }
func (p *task05CountingProvider) ModelID() string { return "task05-model" }
func (p *task05CountingProvider) CreateStream(context.Context, provider.Params) (<-chan types.StreamEvent, error) {
	p.calls.Add(1)
	return eventStream(agentTextEvents("task05 complete")), nil
}

type task05BlockingProbe struct {
	attempted chan struct{}
	release   chan struct{}
	once      sync.Once
}

func (p *task05BlockingProbe) ServerNames() []string { return []string{"github"} }
func (p *task05BlockingProbe) GetOrConnect(_ context.Context, name string) (mcpmanager.MCPServerConnection, error) {
	p.once.Do(func() { close(p.attempted) })
	<-p.release
	return mcpmanager.MCPServerConnection{Name: name, Type: mcpmanager.MCPStateConnected}, nil
}

type task05FailingProbe struct{}

func (task05FailingProbe) ServerNames() []string { return []string{"github"} }
func (task05FailingProbe) GetOrConnect(context.Context, string) (mcpmanager.MCPServerConnection, error) {
	return mcpmanager.MCPServerConnection{}, errors.New("still connecting")
}

type task05DeadlineProbe struct{}

func (task05DeadlineProbe) ServerNames() []string { return []string{"github"} }
func (task05DeadlineProbe) GetOrConnect(ctx context.Context, _ string) (mcpmanager.MCPServerConnection, error) {
	<-ctx.Done()
	return mcpmanager.MCPServerConnection{}, ctx.Err()
}

func TestTask05AgentMCPReadinessPrecedesProviderStart(t *testing.T) {
	provider := &task05CountingProvider{}
	probe := &task05BlockingProbe{attempted: make(chan struct{}), release: make(chan struct{})}
	tool := &AgentTool{
		Provider: provider,
		Registry: registry.New(),
		InlineProfiles: map[string]agentProfile{
			"mcp-reader": {
				Name:                  "mcp-reader",
				AllowedToolsSpecified: true,
				AllowedTools:          map[string]struct{}{"mcp__github__search": {}},
				AllowedToolSpecs:      []string{"mcp__github__search"},
			},
		},
	}
	tool.SetMCPReadinessProbe(probe)

	done := make(chan types.ToolResult, 1)
	go func() {
		result, _ := tool.Execute(context.Background(), agentExecuteInput("search", map[string]any{"subagent_type": "mcp-reader"}))
		done <- result
	}()

	select {
	case <-probe.attempted:
	case <-time.After(time.Second):
		t.Fatal("MCP readiness probe was not called")
	}
	if got := provider.calls.Load(); got != 0 {
		t.Fatalf("provider started before MCP readiness, calls=%d", got)
	}
	close(probe.release)
	select {
	case result := <-done:
		if result.IsError {
			t.Fatalf("Agent failed after MCP became ready: %s", result.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Agent did not start after MCP became ready")
	}
	if got := provider.calls.Load(); got != 1 {
		t.Fatalf("provider calls=%d, want 1", got)
	}
}

func TestTask05WaitForMCPReadinessReportsStageTimeout(t *testing.T) {
	report, err := WaitForMCPReadiness(context.Background(), task05FailingProbe{}, []string{"github"}, 20*time.Millisecond)
	want := toolRuntimeText(i18n.KeyToolAgentMCPReadinessTimedOut)
	if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(report.Failed["github"], want) {
		t.Fatalf("readiness timeout err=%v report=%+v, want stage-specific copy %q", err, report, want)
	}
}

func TestTask05AgentContextTimeoutDuringMCPReadinessPreventsProviderStart(t *testing.T) {
	provider := &task05CountingProvider{}
	tool := &AgentTool{
		Provider: provider,
		Registry: registry.New(),
		InlineProfiles: map[string]agentProfile{
			"mcp-reader": {
				Name:                  "mcp-reader",
				AllowedToolsSpecified: true,
				AllowedTools:          map[string]struct{}{"mcp__github__search": {}},
				AllowedToolSpecs:      []string{"mcp__github__search"},
			},
		},
	}
	tool.SetMCPReadinessProbe(task05DeadlineProbe{})
	// Leave enough time for profile resolution and registry construction even
	// under the race detector; the deadline must expire inside the blocking MCP
	// probe so this test exercises the readiness boundary, not setup overhead.
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	result, err := tool.Execute(ctx, agentExecuteInput("search", map[string]any{"subagent_type": "mcp-reader"}))
	if err != nil || !result.IsError {
		t.Fatalf("Execute err=%v result=%+v", err, result)
	}
	if provider.calls.Load() != 0 {
		t.Fatalf("provider ran despite readiness timeout")
	}
	if _, ok := result.Data.(AgentError); !ok || !strings.Contains(result.Content, toolRuntimeText(i18n.KeyToolAgentMCPReadinessTimedOut)) {
		t.Fatalf("context timeout result=%+v", result)
	}
}

func TestTask05InlineMCPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_AGENT_INLINE_MCP_HELPER") != "1" {
		return
	}

	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			t.Fatalf("decode helper request: %v", err)
		}
		if len(request.ID) == 0 {
			continue
		}
		result := map[string]any{}
		switch request.Method {
		case "initialize":
			result = map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{},
			}
		case "tools/list":
			result = map[string]any{"tools": []any{}}
		}
		if err := encoder.Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result":  result,
		}); err != nil {
			t.Fatalf("encode helper response: %v", err)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read helper request: %v", err)
	}
	os.Exit(0)
}

func TestTask05AgentTypedResultContractAndMapper(t *testing.T) {
	tool := &AgentTool{Provider: &captureAgentProvider{responses: []string{"typed result"}}, Registry: registry.New()}
	result, err := tool.Execute(context.Background(), agentExecuteInput("typed result", nil))
	if err != nil || result.IsError {
		t.Fatalf("Execute err=%v result=%+v", err, result)
	}
	completed, ok := result.Data.(AgentCompleted)
	if !ok {
		t.Fatalf("ToolResult.Data=%T, want AgentCompleted", result.Data)
	}
	if completed.Kind != AgentResultKindCompleted || completed.AgentID == "" || completed.TranscriptPath == "" {
		t.Fatalf("incomplete typed result: %+v", completed)
	}
	raw, err := MarshalAgentResult(AgentCompleted{})
	if err != nil || !strings.Contains(string(raw), `"kind":"completed"`) {
		t.Fatalf("variant MarshalJSON must force discriminator: raw=%s err=%v", raw, err)
	}
	block := types.MapToolResult(tool, result, "toolu_agent")
	text := block.TextContent()
	if !strings.Contains(text, "typed result") || !strings.Contains(text, "agentId:") || !strings.Contains(text, "<usage>") {
		t.Fatalf("mapped Agent text does not match parent-facing contract: %q", text)
	}
}

func TestTask05AgentDefinitionRejectsUnknownAllowedTool(t *testing.T) {
	reg := registry.New()
	reg.Register(&toolfile.FileReadTool{})
	reg.Register(fakeTool{name: "mcp__github__search"})
	valid := []AgentDefinition{{
		Name:         "valid",
		Source:       "project",
		AllowedTools: []string{"Read", "mcp__github__*"},
	}}
	if err := validateAgentDefinitionToolAllowLists(valid, reg); err != nil {
		t.Fatalf("valid allow-list rejected: %v", err)
	}
	invalid := []AgentDefinition{{
		Name:         "invalid",
		Source:       "project",
		AllowedTools: []string{"DefinitelyUnknown"},
	}}
	err := validateAgentDefinitionToolAllowLists(invalid, reg)
	want := toolRuntimeFormat(i18n.KeyToolAgentDefinitionUnknownTool, "invalid", "DefinitelyUnknown")
	if err == nil || err.Error() != want {
		t.Fatalf("unknown allow-list result=%v", err)
	}
}

func TestTask05AgentTranscriptContainsUserAssistantAndTerminal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.jsonl")
	t.Setenv("LUBAN_AGENT_TRANSCRIPT", path)
	tool := &AgentTool{Provider: &captureAgentProvider{responses: []string{"transcribed answer"}}, Registry: registry.New()}
	result, err := tool.Execute(context.Background(), agentExecuteInput("transcribe this", nil))
	if err != nil || result.IsError {
		t.Fatalf("Execute err=%v result=%+v", err, result)
	}
	completed := result.Data.(AgentCompleted)
	if completed.TranscriptPath == path || !strings.HasPrefix(completed.TranscriptPath, strings.TrimSuffix(path, filepath.Ext(path))+".") || filepath.Ext(completed.TranscriptPath) != filepath.Ext(path) {
		t.Fatalf("transcriptPath=%q, want an immutable per-run sibling of %q", completed.TranscriptPath, path)
	}
	raw, err := os.ReadFile(completed.TranscriptPath)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	seen := map[string]bool{}
	for index, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("line %d is invalid JSONL: %v: %q", index, err, line)
		}
		if typ, _ := record["type"].(string); typ != "" {
			seen[typ] = true
		}
	}
	for _, typ := range []string{"user", "assistant", "terminal"} {
		if !seen[typ] {
			t.Fatalf("transcript missing %s record: %s", typ, raw)
		}
	}
}

func TestTask05AgentProgressObserverReceivesTerminalEvent(t *testing.T) {
	tool := &AgentTool{Provider: &captureAgentProvider{responses: []string{"progress answer"}}, Registry: registry.New()}
	var events []agentcontract.ProgressEvent
	unsubscribe := tool.SubscribeProgress(func(event agentcontract.ProgressEvent) {
		events = append(events, event)
	})
	defer unsubscribe()
	result, err := tool.Execute(context.Background(), agentExecuteInput("show progress", nil))
	if err != nil || result.IsError {
		t.Fatalf("Execute err=%v result=%+v", err, result)
	}
	seen := map[agentcontract.ProgressPhase]bool{}
	for _, event := range events {
		seen[event.Phase] = true
	}
	for _, phase := range []agentcontract.ProgressPhase{agentcontract.ProgressStart, agentcontract.ProgressAssistant, agentcontract.ProgressCompleted} {
		if !seen[phase] {
			t.Fatalf("progress missing phase %q: %+v", phase, events)
		}
	}
}

type task05CancelProvider struct {
	started chan struct{}
	once    sync.Once
}

func (p *task05CancelProvider) Name() string    { return "task05-cancel" }
func (p *task05CancelProvider) ModelID() string { return "task05-cancel-model" }
func (p *task05CancelProvider) CreateStream(ctx context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
	p.once.Do(func() { close(p.started) })
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestTask05AgentCancellationReachesChildWithin200ms(t *testing.T) {
	provider := &task05CancelProvider{started: make(chan struct{})}
	tool := &AgentTool{Provider: provider, Registry: registry.New()}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan types.ToolResult, 1)
	go func() {
		result, _ := tool.Execute(ctx, agentExecuteInput("wait", nil))
		done <- result
	}()
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("child provider did not start")
	}
	started := time.Now()
	cancel()
	select {
	case result := <-done:
		if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
			t.Fatalf("cancellation took %s, want <=200ms", elapsed)
		}
		aborted, ok := result.Data.(AgentIncomplete)
		if !ok {
			t.Fatalf("cancel result Data=%T, want AgentIncomplete: %+v", result.Data, result)
		}
		if aborted.Outcome != agentcontract.RunOutcomeCancelled || aborted.ResultKind() != AgentResultKindCancelled {
			t.Fatalf("cancel result lost typed outcome: %+v", aborted)
		}
		if aborted.TranscriptPath == "" {
			t.Fatalf("aborted result must retain transcript path: %+v", aborted)
		}
		raw, readErr := os.ReadFile(aborted.TranscriptPath)
		if readErr != nil || !strings.Contains(string(raw), `"phase":"aborted"`) {
			t.Fatalf("aborted transcript was not flushed: err=%v raw=%s", readErr, raw)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("cancellation did not reach child within 200ms")
	}
}

type task05NotificationSink struct {
	ch chan agentcontract.RuntimeNotification
}

func (s task05NotificationSink) DeliverRuntimeNotification(_ context.Context, notification agentcontract.RuntimeNotification) error {
	s.ch <- notification
	return nil
}

func TestTask05BackgroundAgentDeliversTaskNotification(t *testing.T) {
	manager := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	notifications := make(chan agentcontract.RuntimeNotification, 1)
	manager.SetNotificationSink(task05NotificationSink{ch: notifications})
	wantUsage := types.Usage{InputTokens: 120, OutputTokens: 18, CacheReadInputTokens: 80}
	tool := &AgentTool{
		Provider: &sequencedAgentProvider{responses: [][]types.StreamEvent{
			agentEventsWithUsage(agentTextEvents("background done"), wantUsage),
		}},
		Registry:   registry.New(),
		Background: manager,
	}
	result, err := tool.Execute(context.Background(), agentExecuteInput("background", map[string]any{"run_in_background": true}))
	if err != nil || result.IsError {
		t.Fatalf("Execute err=%v result=%+v", err, result)
	}
	partial, ok := result.Data.(AgentPartial)
	if !ok || partial.TaskID == "" {
		t.Fatalf("background Data=%T %+v", result.Data, result.Data)
	}
	if _, status := manager.Wait(partial.TaskID, 2*time.Second); status != "success" {
		t.Fatalf("background wait status=%s", status)
	}
	select {
	case notification := <-notifications:
		if notification.Kind != "task-notification" || notification.TaskID != partial.TaskID || notification.Status != "completed" {
			t.Fatalf("unexpected notification: %+v", notification)
		}
		if notification.TranscriptPath == "" || notification.DurationMs == nil || notification.TotalTokens == nil {
			t.Fatalf("notification is missing Agent lifecycle metadata: %+v", notification)
		}
		if notification.Provider != "sequenced" || notification.Model != "sequenced-model" || notification.Usage == nil || *notification.Usage != wantUsage {
			t.Fatalf("notification is missing billable Agent usage identity: %+v", notification)
		}
	case <-time.After(time.Second):
		t.Fatal("background completion notification was not delivered")
	}
}
