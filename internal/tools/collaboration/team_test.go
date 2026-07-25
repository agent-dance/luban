package collaboration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/agent-dance/luban/i18n"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"
	"github.com/agent-dance/luban/swarm"
	"github.com/agent-dance/luban/types"
)

func newFocusedTeamManager(t *testing.T, sessionID string) (*TeamManager, RuntimeIdentity, context.Context) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, "project")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	identity := RuntimeIdentity{
		SessionID: sessionID, ProjectRoot: root, AgentID: "team-lead@alpha", Model: "model-current",
	}
	manager := NewTeamManager(nil)
	manager.PublishRuntimeIdentity(identity)
	return manager, identity, context.Background()
}

func decodeFocusedResult(t *testing.T, result types.ToolResult) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(result.Content), &decoded); err != nil {
		t.Fatalf("decode result %q: %v", result.Content, err)
	}
	return decoded
}

func TestTeamCreatePublishesSingleDurableLead(t *testing.T) {
	manager, identity, ctx := newFocusedTeamManager(t, "session-create")
	var notifications atomic.Int32
	manager.SetTaskListChangeNotifier(func() { notifications.Add(1) })

	result, err := NewTeamCreateTool(manager).Execute(ctx, map[string]any{
		"team_name": "Alpha Team", "description": "release work", "agent_type": "reviewer",
	})
	if err != nil || result.IsError {
		t.Fatalf("TeamCreate error=%v result=%#v", err, result)
	}
	decoded := decodeFocusedResult(t, result)
	storageName := teamStorageName(decoded["team_name"].(string))
	config, err := swarm.LoadTeamConfig(storageName)
	if err != nil {
		t.Fatal(err)
	}
	if config.LeadSessionID != identity.SessionID || config.LeadAgentID != "team-lead@Alpha Team" {
		t.Fatalf("durable identity = %#v", config)
	}
	if len(config.Members) != 1 {
		t.Fatalf("members = %#v, want one durable lead", config.Members)
	}
	lead := config.Members[0]
	if lead.Name != teamLeadName || lead.AgentType != "reviewer" ||
		lead.Model != identity.Model || canonicalTeamOwnerRoot(lead.CWD) != canonicalTeamOwnerRoot(identity.ProjectRoot) {
		t.Fatalf("lead = %#v", lead)
	}
	if manager.CurrentTeamName() != "Alpha Team" || notifications.Load() != 1 {
		t.Fatalf("projection=%q notifications=%d", manager.CurrentTeamName(), notifications.Load())
	}
}

func TestTeamCreateRequiresPublishedRuntimeIdentity(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	result, err := NewTeamCreateTool(NewTeamManager(nil)).Execute(context.Background(), map[string]any{"team_name": "alpha"})
	want := i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolCollaborationRuntimeIdentityIncomplete)
	if err != nil || !result.IsError || result.Content != want {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestTeamLifecycleRestoreRequiresExactOwnerAndDurableConfig(t *testing.T) {
	manager, identity, ctx := newFocusedTeamManager(t, "session-owner")
	created, err := NewTeamCreateTool(manager).Execute(ctx, map[string]any{"team_name": "alpha"})
	if err != nil || created.IsError {
		t.Fatalf("create error=%v result=%#v", err, created)
	}

	restored := NewTeamManager(nil)
	restored.PublishRuntimeIdentity(identity)
	if got := restored.CurrentTeamName(); got != "alpha" {
		t.Fatalf("restored team = %q", got)
	}

	wrongSession := NewTeamManager(nil)
	wrongIdentity := identity
	wrongIdentity.SessionID = "different-session"
	wrongSession.PublishRuntimeIdentity(wrongIdentity)
	if got := wrongSession.CurrentTeamName(); got != "" {
		t.Fatalf("cross-session team = %q", got)
	}

	if err := swarm.DeleteTeamConfig("alpha"); err != nil {
		t.Fatal(err)
	}
	missingConfig := NewTeamManager(nil)
	missingConfig.PublishRuntimeIdentity(identity)
	if got := missingConfig.CurrentTeamName(); got != "" {
		t.Fatalf("team restored without durable config: %q", got)
	}
}

func TestTeamLifecycleRestoreRejectsIncompleteEventIdentity(t *testing.T) {
	manager, identity, _ := newFocusedTeamManager(t, "session-strict")
	config := buildPersistedTeamConfig("strict", "", "team-lead@strict", identity, "")
	if err := swarm.CreateTeamConfigAs("strict", config); err != nil {
		t.Fatal(err)
	}
	lifecycle := runtimestore.NewRuntimeLifecycle(identity.ProjectRoot)
	if err := lifecycle.Publish(context.Background(), runtimestore.RuntimeLifecycleEvent{
		Type:      runtimestore.LifecycleTeamCreate,
		EntityID:  "team-incomplete",
		ToolName:  "TeamCreate",
		Status:    "active",
		SessionID: identity.SessionID,
		Payload: map[string]any{
			"name": "strict",
			// storage_name and owner_project_root are intentionally absent.
		},
	}); err != nil {
		t.Fatal(err)
	}
	manager.PublishRuntimeIdentity(identity)
	if got := manager.CurrentTeamName(); got != "" {
		t.Fatalf("incomplete lifecycle event restored %q", got)
	}
}

func TestTeamDeleteRequiresInactiveDurableTeammates(t *testing.T) {
	manager, identity, ctx := newFocusedTeamManager(t, "session-delete")
	created, err := NewTeamCreateTool(manager).Execute(ctx, map[string]any{"team_name": "alpha"})
	if err != nil || created.IsError {
		t.Fatalf("create error=%v result=%#v", err, created)
	}
	if _, err := swarm.UpdateTeamConfig(context.Background(), "alpha", func(config *swarm.TeamConfig) error {
		config.Members = append(config.Members, swarm.TeamMember{
			AgentID: "worker@alpha", Name: "worker", BackendType: "in-process", CWD: identity.ProjectRoot, IsActive: true,
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	blocked, err := NewTeamDeleteTool(manager).Execute(ctx, map[string]any{})
	if err != nil || blocked.IsError {
		t.Fatalf("blocked delete error=%v result=%#v", err, blocked)
	}
	if success, _ := decodeFocusedResult(t, blocked)["success"].(bool); success {
		t.Fatalf("active teammate did not block delete: %s", blocked.Content)
	}
	if _, err := swarm.UpdateTeamConfig(context.Background(), "alpha", func(config *swarm.TeamConfig) error {
		config.Members[1].IsActive = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	deleted, err := NewTeamDeleteTool(manager).Execute(ctx, map[string]any{})
	if err != nil || deleted.IsError {
		t.Fatalf("delete error=%v result=%#v", err, deleted)
	}
	if manager.CurrentTeamName() != "" {
		t.Fatalf("manager retained deleted team %q", manager.CurrentTeamName())
	}
	if _, err := swarm.LoadTeamConfig("alpha"); err == nil {
		t.Fatal("durable config survived delete")
	}
}

func TestConcurrentTeamCreateCommitsOneTeamPerOwner(t *testing.T) {
	manager, _, ctx := newFocusedTeamManager(t, "session-concurrent")
	results := make(chan types.ToolResult, 2)
	for _, name := range []string{"alpha", "beta"} {
		name := name
		go func() {
			result, _ := NewTeamCreateTool(manager).Execute(ctx, map[string]any{"team_name": name})
			results <- result
		}()
	}
	successes := 0
	for range 2 {
		if result := <-results; !result.IsError {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful creates = %d, want 1", successes)
	}
}

func TestTeamToolMetadata(t *testing.T) {
	create := NewTeamCreateTool(nil).ToolMetadata(nil)
	if !create.Write || create.Destructive || create.ReadOnly {
		t.Fatalf("TeamCreate metadata = %#v", create)
	}
	remove := NewTeamDeleteTool(nil).ToolMetadata(nil)
	if !remove.Write || !remove.Destructive || remove.ReadOnly {
		t.Fatalf("TeamDelete metadata = %#v", remove)
	}
}
