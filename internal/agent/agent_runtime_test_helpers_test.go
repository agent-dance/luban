package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/agent-dance/luban/internal/approvalcommit"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

type mockProvider struct {
	mu        sync.Mutex
	responses []string
	callIndex int
}

func (*mockProvider) Name() string    { return "agent-test" }
func (*mockProvider) ModelID() string { return "agent-test-model" }

func (p *mockProvider) CreateStream(_ context.Context, _ provider.Params) (<-chan types.StreamEvent, error) {
	p.mu.Lock()
	index := p.callIndex
	p.callIndex++
	text := "(no response)"
	if index < len(p.responses) {
		text = p.responses[index]
	}
	p.mu.Unlock()
	return eventStream(agentTextEvents(text)), nil
}

func requireBashAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash is not on PATH: %v", err)
	}
	if err := exec.Command("bash", "-c", "true").Run(); err != nil {
		t.Skipf("bash is present but not runnable: %v", err)
	}
}

type task07SandboxBackend struct {
	mu         sync.Mutex
	lastConfig sandbox.Config
}

func (*task07SandboxBackend) Name() string    { return "agent-test-sandbox" }
func (*task07SandboxBackend) Available() bool { return true }
func (*task07SandboxBackend) SandboxCapability() (sandbox.Capability, bool) {
	return sandbox.Capability{Backend: "agent-test-sandbox", ExecutablePath: "/usr/bin/agent-test", ExecutableIdentity: "v1"}, true
}
func (b *task07SandboxBackend) Command(ctx context.Context, cfg sandbox.Config, name string, args ...string) (*exec.Cmd, error) {
	b.mu.Lock()
	b.lastConfig = cfg
	b.lastConfig.ReadWritePaths = append([]string(nil), cfg.ReadWritePaths...)
	b.mu.Unlock()
	return exec.CommandContext(ctx, name, args...), nil
}

func executeApprovedRegistryToolForTest(t *testing.T, reg *registry.Registry, name string, input map[string]any) (types.ToolResultBlock, error) {
	t.Helper()
	preflight, err := reg.CheckToolPermissions(context.Background(), name, input, types.ToolPermissionRequest{
		SessionID: "test-session", TurnID: "test-turn", ToolUseID: "test-tool-use", ApprovalEpoch: "test-epoch",
	})
	if err != nil {
		t.Fatalf("%s permission preflight: %v", name, err)
	}
	if preflight.PermissionGrant == "" {
		t.Fatalf("%s permission preflight returned no grant (behavior=%s)", name, preflight.Behavior)
	}
	token := reg.AuthorizePermissionGrant(
		preflight.PermissionGrant, name, input, preflight.PermissionBinding, preflight.ExecutionPolicyCode,
	)
	if token == "" {
		t.Fatalf("%s permission authorization returned no execution grant", name)
	}
	ctx := approvalcommit.WithPending(context.Background(), approvalcommit.Pending{
		Token: token, Binding: preflight.PermissionBinding, PolicyCode: preflight.ExecutionPolicyCode,
	})
	return reg.ExecuteToolWithError(ctx, name, input)
}

func makeTempSkillDir(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	skillsDir := filepath.Join(root, "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return root, skillsDir
}

func writeMDSkill(t *testing.T, skillsDir, name, content string) {
	t.Helper()
	dir := filepath.Join(skillsDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newTestSkillManager(dirs ...string) *skills.Manager {
	sources := make([]skills.DirSource, len(dirs))
	for index, dir := range dirs {
		sources[index] = skills.DirSource{Dir: dir, Source: skills.SourceProject}
	}
	manager := skills.NewManager(sources...)
	manager.SetOverrideStore(agentSkillTestOverrideStore{})
	return manager
}

type agentSkillTestOverrideStore struct{}

func (agentSkillTestOverrideStore) Snapshot(string) (skills.OverrideSnapshot, error) {
	return skills.OverrideSnapshot{
		User: map[skills.SkillID]skills.VisibilityOverride{}, Project: map[skills.SkillID]skills.VisibilityOverride{},
		Managed: map[skills.SkillID]skills.VisibilityOverride{}, Session: map[skills.SkillID]skills.VisibilityOverride{},
	}, nil
}

func (agentSkillTestOverrideStore) Set(string, skills.VisibilityOverride) error { return nil }
func (agentSkillTestOverrideStore) Reset(string, skills.SkillScope, skills.SkillID) error {
	return nil
}
func (agentSkillTestOverrideStore) CompareAndSetProject(skills.OverrideStoreRevision, skills.SkillID, *skills.VisibilityOverride) (skills.ProjectOverrideRestore, error) {
	return skills.ProjectOverrideRestore{}, skills.ErrOverrideRevisionConflict
}
func (agentSkillTestOverrideStore) RestoreProject(skills.ProjectOverrideRestore) error { return nil }
