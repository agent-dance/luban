package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
func (p *task05BlockingProbe) Connect(string) (*MCPServerConn, error) {
	p.once.Do(func() { close(p.attempted) })
	<-p.release
	return &MCPServerConn{}, nil
}

type task05FailingProbe struct{}

func (task05FailingProbe) ServerNames() []string { return []string{"github"} }
func (task05FailingProbe) Connect(string) (*MCPServerConn, error) {
	return nil, errors.New("still connecting")
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
	tool.SetMCPReadinessTimeout(time.Second)

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

func TestTask05AgentMCPReadinessTimeoutPreventsProviderStart(t *testing.T) {
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
	tool.SetMCPReadinessProbe(task05FailingProbe{})
	tool.SetMCPReadinessTimeout(20 * time.Millisecond)
	result, err := tool.Execute(context.Background(), agentExecuteInput("search", map[string]any{"subagent_type": "mcp-reader"}))
	if err != nil || !result.IsError {
		t.Fatalf("Execute err=%v result=%+v", err, result)
	}
	if provider.calls.Load() != 0 {
		t.Fatalf("provider ran despite readiness timeout")
	}
	if _, ok := result.Data.(AgentError); !ok || !strings.Contains(strings.ToLower(result.Content), "timed out") {
		t.Fatalf("timeout result=%+v", result)
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

func TestTask05InlineOnlyMCPReadinessUsesChildManagerBeforeProviderStart(t *testing.T) {
	parentManager := NewMCPManager()
	reg := registry.New()
	reg.Register(NewMCPTool(parentManager))

	type connectAttempt struct {
		manager *MCPManager
		name    string
	}
	connectStarted := make(chan connectAttempt, 1)
	releaseConnect := make(chan struct{})
	parentManager.connectStartedForTest = func(manager *MCPManager, name string) {
		connectStarted <- connectAttempt{manager: manager, name: name}
		<-releaseConnect
	}

	modelProvider := &task05CountingProvider{}
	tool := &AgentTool{
		Provider: modelProvider,
		Registry: reg,
		InlineProfiles: map[string]agentProfile{
			"inline-private": {
				Name:               "inline-private",
				RequiredMCPServers: []string{"private"},
				MCPServerConfigs: map[string]MCPServerConfig{
					"private": {
						Command: os.Args[0],
						Args:    []string{"-test.run=^TestTask05InlineMCPHelperProcess$"},
						Env: map[string]string{
							"GO_WANT_AGENT_INLINE_MCP_HELPER": "1",
						},
					},
				},
			},
		},
	}

	type execution struct {
		result types.ToolResult
		err    error
	}
	done := make(chan execution, 1)
	go func() {
		result, err := tool.Execute(context.Background(), agentExecuteInput("use private MCP", map[string]any{
			"subagent_type": "inline-private",
		}))
		done <- execution{result: result, err: err}
	}()

	var attempt connectAttempt
	select {
	case attempt = <-connectStarted:
	case got := <-done:
		t.Fatalf("Agent returned before the inline MCP child manager attempted readiness: result=%+v err=%v", got.result, got.err)
	}
	wrongManager := attempt.manager == parentManager
	wrongServer := attempt.name != "private"
	if got := modelProvider.calls.Load(); got != 0 {
		close(releaseConnect)
		<-done
		t.Fatalf("provider started before inline MCP readiness, calls=%d", got)
	}
	close(releaseConnect)

	got := <-done
	if wrongManager {
		t.Fatal("inline MCP readiness used the parent manager instead of the child manager")
	}
	if wrongServer {
		t.Fatalf("inline MCP readiness connected %q, want private", attempt.name)
	}
	if got.err != nil || got.result.IsError {
		t.Fatalf("Agent failed after inline MCP became ready: result=%+v err=%v", got.result, got.err)
	}
	if got := modelProvider.calls.Load(); got != 1 {
		t.Fatalf("provider calls=%d, want 1", got)
	}
	if names := parentManager.ServerNames(); len(names) != 0 {
		t.Fatalf("inline MCP config leaked into parent manager: %v", names)
	}
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
	contract := types.ResolveToolContract(tool)
	if contract.OutputSchema == nil || len(contract.OutputSchema.AnyOf) != 7 || !contract.ReadOnly || !contract.ConcurrencySafe || !contract.Strict {
		t.Fatalf("unexpected Agent contract: %+v", contract)
	}
	if got := types.ToolAutoClassifierInput(tool, map[string]any{"prompt": "inspect", "subagent_type": "Explore"}); got != "(Explore): inspect" {
		t.Fatalf("shared classifier input=%q", got)
	}
}

func TestTask05AgentDefinitionRejectsUnknownAllowedTool(t *testing.T) {
	reg := registry.New()
	reg.Register(&FileReadTool{})
	reg.Register(fakeTool{name: "mcp__github__search"})
	valid := []AgentDefinition{{
		Name:         "valid",
		Source:       string(AgentSourceProject),
		AllowedTools: []string{"Read", "FileRead", "mcp__github__*"},
	}}
	if err := validateAgentDefinitionToolAllowLists(valid, reg); err != nil {
		t.Fatalf("valid allow-list rejected: %v", err)
	}
	invalid := []AgentDefinition{{
		Name:         "invalid",
		Source:       string(AgentSourceProject),
		AllowedTools: []string{"DefinitelyUnknown"},
	}}
	err := validateAgentDefinitionToolAllowLists(invalid, reg)
	if err == nil || !strings.Contains(err.Error(), `unknown tool "DefinitelyUnknown"`) {
		t.Fatalf("unknown allow-list result=%v", err)
	}
}

func TestTask05AgentTranscriptContainsUserAssistantAndTerminal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.jsonl")
	t.Setenv("CLAUDE_AGENT_TRANSCRIPT", path)
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

func TestTask05AgentProgressStreamsAndCloses(t *testing.T) {
	tool := &AgentTool{Provider: &captureAgentProvider{responses: []string{"progress answer"}}, Registry: registry.New()}
	emitter := tool.Progress()
	result, err := tool.Execute(context.Background(), agentExecuteInput("show progress", nil))
	if err != nil || result.IsError {
		t.Fatalf("Execute err=%v result=%+v", err, result)
	}
	var events []AgentProgressEvent
	deadline := time.After(time.Second)
	for {
		select {
		case event, ok := <-emitter.Channel():
			if !ok {
				goto collected
			}
			events = append(events, event)
		case <-deadline:
			t.Fatal("progress channel was not closed")
		}
	}
collected:
	seen := map[AgentProgressPhase]bool{}
	for _, event := range events {
		seen[event.Phase] = true
	}
	for _, phase := range []AgentProgressPhase{AgentPhaseStart, AgentPhaseAssistant, AgentPhaseCompleted} {
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
		if aborted.Outcome != AgentRunOutcomeCancelled || aborted.ResultKind() != AgentResultKindCancelled {
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

func TestTask05RemoteNetworkFailureReturnsAgentError(t *testing.T) {
	remote := &fakeRemoteRuntime{spawnErr: errors.New("network unavailable")}
	tool := &AgentTool{RemoteRuntime: remote}
	root := t.TempDir()
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: types.ToolRuntimeContext{ProjectRoot: root, AllowedDirs: []string{root}, PermissionMode: permissionModeDefault}})
	result, err := tool.Execute(context.Background(), agentExecuteInput("remote", map[string]any{"isolation": "remote"}))
	if err != nil || !result.IsError {
		t.Fatalf("Execute err=%v result=%+v", err, result)
	}
	agentErr, ok := result.Data.(AgentError)
	if !ok || !strings.Contains(agentErr.Message, "network unavailable") {
		t.Fatalf("remote failure Data=%T %+v", result.Data, result.Data)
	}
}

func TestTask05HTTPRemoteRuntimeRefreshesExpiringToken(t *testing.T) {
	var resolverCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer refreshed-token" {
			t.Errorf("Authorization=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"taskId":"remote-1","permissionSnapshotEnforced":true,"promptRoutingEnforced":true}`))
	}))
	defer server.Close()
	runtime := &HTTPRemoteRuntime{
		BaseURL:              server.URL,
		AccessToken:          "stale-token",
		AccessTokenExpiresAt: time.Now().Add(time.Minute),
		AccessTokenResolver: func(context.Context) (string, error) {
			resolverCalls.Add(1)
			return "refreshed-token", nil
		},
	}
	launch, err := runtime.Spawn(context.Background(), RemoteAgentSpawnRequest{Prompt: "run"})
	if err != nil || launch.TaskID != "remote-1" {
		t.Fatalf("Spawn launch=%+v err=%v", launch, err)
	}
	if resolverCalls.Load() != 1 {
		t.Fatalf("resolver calls=%d, want 1", resolverCalls.Load())
	}
}

type task05NotificationSink struct {
	ch chan RuntimeNotification
}

func (s task05NotificationSink) DeliverRuntimeNotification(_ context.Context, notification RuntimeNotification) error {
	s.ch <- notification
	return nil
}

func TestTask05BackgroundAgentDeliversTaskNotification(t *testing.T) {
	manager := NewBackgroundTaskManager(t.TempDir())
	t.Cleanup(manager.Shutdown)
	notifications := make(chan RuntimeNotification, 1)
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
