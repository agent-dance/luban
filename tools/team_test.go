package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/coordinator"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/swarm"
)

func newTestManager(t *testing.T) *TeamManager {
	t.Helper()
	home := t.TempDir()
	return newTestManagerForHome(t, home)
}

func newTestManagerForHome(t *testing.T, home string) *TeamManager {
	t.Helper()
	t.Setenv("HOME", home)
	mgr := NewTeamManager(coordinator.NewCoordinator())
	mgr.CWD = home
	mgr.SessionID = func() string { return "sess-test" }
	return mgr
}

func decodeJSONResult(t *testing.T, content string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		t.Fatalf("failed to decode JSON result %q: %v", content, err)
	}
	return out
}

// ─── SendMessageTool ──────────────────────────────────────────────────────────

func TestSendMessage_DeliverSuccess(t *testing.T) {
	mgr := newTestManager(t)

	tool := NewSendMessageTool(mgr)
	res, err := tool.Execute(context.Background(), map[string]any{
		"to":      "alice",
		"summary": "send a direct greeting",
		"message": "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "alice") {
		t.Errorf("expected agent name in response, got: %s", res.Content)
	}

	mailbox, mailboxErr := swarm.NewMailbox("default")
	if mailboxErr != nil {
		t.Fatal(mailboxErr)
	}
	messages, mailboxErr := mailbox.Read(context.Background(), "alice")
	if mailboxErr != nil || len(messages) != 1 || messages[0].Text != "hello" {
		t.Fatalf("default mailbox messages=%#v err=%v", messages, mailboxErr)
	}
}

func TestSendMessage_AgentNotFound(t *testing.T) {
	mgr := newTestManager(t)
	tool := NewSendMessageTool(mgr)
	res, err := tool.Execute(context.Background(), map[string]any{
		"to":      "nobody",
		"summary": "send an unknown recipient",
		"message": "ping",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError || !strings.Contains(res.Content, `"success":true`) {
		t.Fatalf("TS mailbox accepts a direct inbox name, got: %s", res.Content)
	}
}

func TestSendMessage_MissingTo(t *testing.T) {
	mgr := newTestManager(t)
	tool := NewSendMessageTool(mgr)
	res, _ := tool.Execute(context.Background(), map[string]any{"to": "", "summary": "send a short greeting", "message": "hi"})
	if !res.IsError {
		t.Errorf("expected error for empty 'to', got: %s", res.Content)
	}
}

func TestSendMessage_MissingContent(t *testing.T) {
	mgr := newTestManager(t)
	tool := NewSendMessageTool(mgr)
	res, _ := tool.Execute(context.Background(), map[string]any{"to": "alice"})
	if !res.IsError {
		t.Errorf("expected error for missing 'message', got: %s", res.Content)
	}
}

// ─── TeamCreateTool ───────────────────────────────────────────────────────────

func TestTeamCreate_Success(t *testing.T) {
	mgr := newTestManager(t)
	tool := NewTeamCreateTool(mgr)
	res, err := tool.Execute(context.Background(), map[string]any{
		"team_name":  "alpha",
		"agent_type": "planner",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got: %s", res.Content)
	}
	if !strings.Contains(res.Content, `"team_name":"alpha"`) {
		t.Errorf("expected team_name in response, got: %s", res.Content)
	}
	if !strings.Contains(res.Content, `"lead_agent_id":"team-lead@alpha"`) {
		t.Errorf("expected lead_agent_id in response, got: %s", res.Content)
	}
	payload := decodeJSONResult(t, res.Content)
	wantPath := filepath.Join(os.Getenv("HOME"), brand.ConfigDirName, "teams", "alpha", "team.json")
	gotPath, _ := payload["team_file_path"].(string)
	if gotPath == "" {
		t.Fatalf("expected team_file_path in response, got: %s", res.Content)
	}
	if gotPath != wantPath {
		t.Fatalf("expected team_file_path %q, got %q", wantPath, gotPath)
	}

	mgr.mu.Lock()
	info, ok := mgr.teams["team-1"]
	mgr.mu.Unlock()
	if !ok {
		t.Fatal("team-1 not stored in manager")
	}
	if info.Name != "alpha" {
		t.Errorf("expected name 'alpha', got %q", info.Name)
	}
	if info.StorageName != "alpha" {
		t.Errorf("expected storage name 'alpha', got %q", info.StorageName)
	}
	if info.LeadAgentID != "team-lead@alpha" {
		t.Errorf("expected lead agent %q, got %q", "team-lead@alpha", info.LeadAgentID)
	}
	if info.FilePath != gotPath {
		t.Errorf("expected stored team file path %q, got %q", gotPath, info.FilePath)
	}
	if len(info.Agents) != 1 {
		t.Errorf("expected only lead agent, got %d", len(info.Agents))
	}
	if info.Agents[0] != "team-lead@alpha" {
		t.Errorf("expected lead agent only, got %#v", info.Agents)
	}
}

func TestTeamCreate_Task32StrictSchemaAndPersistedLeadShape(t *testing.T) {
	mgr := newTestManager(t)
	tool := NewTeamCreateTool(mgr)
	schema := tool.Schema()
	if !schema.RejectsUnknownFields() {
		t.Fatal("TeamCreate schema must reject unknown fields")
	}
	if _, ok := schema.Properties["name"]; ok {
		t.Fatal("TeamCreate schema must not expose legacy name")
	}
	if _, ok := schema.Properties["agents"]; ok {
		t.Fatal("TeamCreate schema must not expose legacy agents")
	}

	ctx := loop.WithToolExecutionContext(context.Background(), loop.ToolExecutionContext{Model: "openai/gpt-5.4"})
	res, err := tool.Execute(ctx, map[string]any{
		"team_name":   "My Team",
		"description": "A focused team",
		"agent_type":  "researcher",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got: %s", res.Content)
	}
	payload := decodeJSONResult(t, res.Content)
	wantPath := filepath.Join(os.Getenv("HOME"), brand.ConfigDirName, "teams", "my-team", "team.json")
	if payload["team_file_path"] != wantPath {
		t.Fatalf("expected slugged team_file_path %q, got %#v", wantPath, payload["team_file_path"])
	}
	if payload["lead_agent_id"] != "team-lead@My Team" {
		t.Fatalf("unexpected lead_agent_id: %#v", payload["lead_agent_id"])
	}

	cfg, err := swarm.LoadTeamConfig("my-team")
	if err != nil {
		t.Fatalf("LoadTeamConfig: %v", err)
	}
	if cfg.Name != "My Team" {
		t.Fatalf("persisted config name must preserve display name, got %q", cfg.Name)
	}
	if len(cfg.Members) != 1 {
		t.Fatalf("expected only lead member, got %#v", cfg.Members)
	}
	lead := cfg.Members[0]
	if lead.AgentID != "team-lead@My Team" || lead.Name != teamLeadName || lead.AgentType != "researcher" {
		t.Fatalf("unexpected lead member identity: %#v", lead)
	}
	if lead.Model != "openai/gpt-5.4" || lead.JoinedAt == 0 || lead.CWD != os.Getenv("HOME") {
		t.Fatalf("unexpected lead member runtime fields: %#v", lead)
	}
	if lead.Subscriptions == nil || len(lead.Subscriptions) != 0 {
		t.Fatalf("expected empty subscriptions array, got %#v", lead.Subscriptions)
	}
}

func TestTeamCreate_SaveFailureDoesNotActivateTeam(t *testing.T) {
	base := t.TempDir()
	homeFile := filepath.Join(base, "home-file")
	if err := os.WriteFile(homeFile, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("failed to create home file: %v", err)
	}
	mgr := newTestManagerForHome(t, homeFile)

	res, err := NewTeamCreateTool(mgr).Execute(context.Background(), map[string]any{
		"team_name": "alpha",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected save failure, got success: %s", res.Content)
	}
	if got := mgr.CurrentTeamName(); got != "" {
		t.Fatalf("expected no active team after save failure, got %q", got)
	}
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	if mgr.activeTeamID != "" {
		t.Fatalf("expected empty active team ID, got %q", mgr.activeTeamID)
	}
	if len(mgr.teams) != 0 {
		t.Fatalf("expected no stored teams after save failure, got %d", len(mgr.teams))
	}
}

func TestTeamCreateAgentInheritsCurrentSessionModel(t *testing.T) {
	provider := &captureAgentProvider{responses: []string{"team model ok"}}
	mgr := newTestManager(t)
	mgr.Provider = provider
	mgr.Registry = registry.New()
	mgr.System = "team system"
	ctx := loop.WithToolExecutionContext(context.Background(), loop.ToolExecutionContext{
		Model: "openai/gpt-5.4",
	})

	res, err := NewTeamCreateTool(mgr).Execute(ctx, map[string]any{
		"team_name":  "alpha",
		"agent_type": "executor",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected team create success, got: %s", res.Content)
	}

	mgr.coordinator.AddTask("inspect inherited model", 1)
	results := mgr.coordinator.Dispatch(context.Background())
	if len(results) != 1 {
		t.Fatalf("expected one dispatched task, got %d", len(results))
	}
	if results[0].Error != nil {
		t.Fatalf("expected team agent success, got %v", results[0].Error)
	}
	if len(provider.params) != 1 {
		t.Fatalf("expected one provider call, got %d", len(provider.params))
	}
	if provider.params[0].Model != "openai/gpt-5.4" {
		t.Fatalf("expected team agent to inherit current session model openai/gpt-5.4, got %q", provider.params[0].Model)
	}
}

func TestTeamCreateRealAgentDoesNotUseFixedTurnCapOrTimeout(t *testing.T) {
	const toolTurns = 51
	provider := &turnLimitAgentProvider{
		toolName:  "Echo",
		toolTurns: toolTurns,
		finalText: "team done after long task",
	}
	mgr := newTestManager(t)
	mgr.Provider = provider
	reg := registry.New()
	reg.Register(fakeTool{name: "Echo"})
	mgr.Registry = reg
	mgr.System = "team system"
	ctx := loop.WithToolExecutionContext(context.Background(), loop.ToolExecutionContext{
		Model: "openai/gpt-5.4",
	})

	res, err := NewTeamCreateTool(mgr).Execute(ctx, map[string]any{
		"team_name":  "alpha",
		"agent_type": "executor",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected team create success, got: %s", res.Content)
	}

	mgr.coordinator.AddTask("inspect long task", 1)
	results := mgr.coordinator.Dispatch(context.Background())
	if len(results) != 1 {
		t.Fatalf("expected one dispatched task, got %d", len(results))
	}
	if results[0].Error != nil {
		t.Fatalf("expected long team agent success, got %v", results[0].Error)
	}
	if !strings.Contains(results[0].Result, "team done after long task") {
		t.Fatalf("expected final team output, got %q", results[0].Result)
	}
	if provider.calls != toolTurns+1 {
		t.Fatalf("expected %d provider calls, got %d", toolTurns+1, provider.calls)
	}
	if provider.sawDeadline {
		t.Fatal("team Agent should inherit the dispatch context without installing a fixed deadline")
	}
}

func TestTeamCreate_SequentialIDs(t *testing.T) {
	mgr := newTestManager(t)
	tool := NewTeamCreateTool(mgr)
	for i := 1; i <= 3; i++ {
		name := "t" + string(rune('0'+i))
		res, _ := tool.Execute(context.Background(), map[string]any{"team_name": name})
		if !strings.Contains(res.Content, fmt.Sprintf(`"team_name":"%s"`, name)) {
			t.Errorf("iter %d: expected team_name in response, got: %s", i, res.Content)
		}
		delRes, _ := NewTeamDeleteTool(mgr).Execute(context.Background(), map[string]any{})
		if delRes.IsError {
			t.Fatalf("iter %d: TeamDelete failed: %s", i, delRes.Content)
		}
	}
	if mgr.nextTeamID != 3 {
		t.Errorf("expected nextTeamID=3, got %d", mgr.nextTeamID)
	}
}

func TestTeamCreate_AlreadyLeadingTeamFails(t *testing.T) {
	mgr := newTestManager(t)
	tool := NewTeamCreateTool(mgr)
	first, err := tool.Execute(context.Background(), map[string]any{"team_name": "alpha"})
	if err != nil {
		t.Fatalf("unexpected error creating first team: %v", err)
	}
	if first.IsError {
		t.Fatalf("expected first create to succeed, got: %s", first.Content)
	}

	second, err := tool.Execute(context.Background(), map[string]any{"team_name": "beta"})
	if err != nil {
		t.Fatalf("unexpected error creating second team: %v", err)
	}
	if !second.IsError {
		t.Fatalf("expected second create to fail, got: %s", second.Content)
	}
	if !strings.Contains(second.Content, `Already leading team "alpha"`) {
		t.Fatalf("unexpected error message: %s", second.Content)
	}
}

func TestTeamCreate_ExistingDiskNameGeneratesUniqueName(t *testing.T) {
	home := t.TempDir()
	firstMgr := newTestManagerForHome(t, home)
	firstTool := NewTeamCreateTool(firstMgr)
	first, err := firstTool.Execute(context.Background(), map[string]any{"team_name": "alpha"})
	if err != nil {
		t.Fatalf("unexpected error creating first team: %v", err)
	}
	if first.IsError {
		t.Fatalf("expected first create to succeed, got: %s", first.Content)
	}

	secondMgr := newTestManagerForHome(t, home)
	secondTool := NewTeamCreateTool(secondMgr)
	second, err := secondTool.Execute(context.Background(), map[string]any{"team_name": "alpha"})
	if err != nil {
		t.Fatalf("unexpected error creating second team: %v", err)
	}
	if second.IsError {
		t.Fatalf("expected second create to succeed with a new name, got: %s", second.Content)
	}
	payload := decodeJSONResult(t, second.Content)
	if payload["team_name"] == "alpha" {
		t.Fatalf("expected unique team name when alpha already exists, got: %s", second.Content)
	}
	if !strings.Contains(payload["team_file_path"].(string), filepath.Join(brand.ConfigDirName, "teams")) {
		t.Fatalf("expected unique team path in response, got: %s", second.Content)
	}
}

func TestTeamCreate_MissingName(t *testing.T) {
	mgr := newTestManager(t)
	tool := NewTeamCreateTool(mgr)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"team_name": "",
	})
	if !res.IsError {
		t.Errorf("expected error for empty name, got: %s", res.Content)
	}
}

func TestTeamCreate_RejectsLegacyNameAndAgents(t *testing.T) {
	mgr := newTestManager(t)
	tool := NewTeamCreateTool(mgr)
	for _, input := range []map[string]any{
		{"name": "solo"},
		{"team_name": "solo", "agents": []any{}},
	} {
		res, _ := tool.Execute(context.Background(), input)
		if !res.IsError {
			t.Errorf("expected strict input error for %#v, got: %s", input, res.Content)
		}
	}
	if !tool.Schema().RejectsUnknownFields() {
		t.Fatal("TeamCreate schema must reject unknown fields")
	}
}

func TestTeamCreate_EmptyAgentID(t *testing.T) {
	mgr := newTestManager(t)
	tool := NewTeamCreateTool(mgr)
	res, _ := tool.Execute(context.Background(), map[string]any{
		"name":   "bad",
		"agents": []any{map[string]any{"id": "", "role": "r"}},
	})
	if !res.IsError {
		t.Errorf("expected error for empty agent id, got: %s", res.Content)
	}
}

// ─── TeamDeleteTool ───────────────────────────────────────────────────────────

func TestTeamDelete_Success(t *testing.T) {
	mgr := newTestManager(t)
	createRes, _ := NewTeamCreateTool(mgr).Execute(context.Background(), map[string]any{ //nolint:errcheck
		"team_name": "beta",
	})
	createPayload := decodeJSONResult(t, createRes.Content)
	teamFilePath, _ := createPayload["team_file_path"].(string)
	if teamFilePath == "" {
		t.Fatalf("expected team file path in create response, got: %s", createRes.Content)
	}

	res, err := NewTeamDeleteTool(mgr).Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got: %s", res.Content)
	}
	if !strings.Contains(res.Content, `"team_name":"beta"`) {
		t.Errorf("expected team_name in response, got: %s", res.Content)
	}
	if _, err := os.Stat(teamFilePath); !os.IsNotExist(err) {
		t.Fatalf("expected team file %q to be removed, stat err=%v", teamFilePath, err)
	}

	mgr.mu.Lock()
	_, stillThere := mgr.teams["team-1"]
	mgr.mu.Unlock()
	if stillThere {
		t.Error("team-1 should have been removed from the manager")
	}
}

func TestTeamDelete_FailsWithActiveMembers(t *testing.T) {
	mgr := newTestManager(t)
	createMailboxTeam(t, mgr, "beta", []any{map[string]any{"id": "worker-1", "role": "worker"}})

	res, err := NewTeamDeleteTool(mgr).Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected JSON failure payload, got tool error: %s", res.Content)
	}
	if !strings.Contains(res.Content, `"success":false`) || !strings.Contains(res.Content, "active member(s): worker-1") {
		t.Fatalf("unexpected delete failure payload: %s", res.Content)
	}

	mgr.mu.Lock()
	_, stillThere := mgr.teams["team-1"]
	mgr.mu.Unlock()
	if !stillThere {
		t.Fatal("team-1 should remain registered after failed cleanup")
	}
}

func TestTeamDelete_NotFound(t *testing.T) {
	mgr := newTestManager(t)
	res, _ := NewTeamDeleteTool(mgr).Execute(context.Background(), map[string]any{})
	if res.IsError {
		t.Errorf("expected no-op success for missing team, got: %s", res.Content)
	}
	if !strings.Contains(res.Content, "nothing to clean up") {
		t.Errorf("expected cleanup no-op message, got: %s", res.Content)
	}
	if strings.Contains(res.Content, "team_name") {
		t.Errorf("no-team cleanup response must not include team_name, got: %s", res.Content)
	}
}

func TestTeamDelete_RejectsUnexpectedInput(t *testing.T) {
	mgr := newTestManager(t)
	tool := NewTeamDeleteTool(mgr)
	if !tool.Schema().RejectsUnknownFields() {
		t.Fatal("TeamDelete schema must reject unknown fields")
	}
	res, _ := tool.Execute(context.Background(), map[string]any{"team_id": ""})
	if !res.IsError || !strings.Contains(res.Content, "unknown field") {
		t.Errorf("expected team_id strict input error, got: %s", res.Content)
	}
}

// ─── Tool metadata ────────────────────────────────────────────────────────────

func TestTeamToolNames(t *testing.T) {
	mgr := newTestManager(t)
	cases := []struct {
		tool interface{ Name() string }
		want string
	}{
		{NewSendMessageTool(mgr), "SendMessage"},
		{NewTeamCreateTool(mgr), "TeamCreate"},
		{NewTeamDeleteTool(mgr), "TeamDelete"},
	}
	for _, c := range cases {
		if c.tool.Name() != c.want {
			t.Errorf("Name() = %q, want %q", c.tool.Name(), c.want)
		}
	}
}
