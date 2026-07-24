package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestAgentProfileRuntimeUsesInheritedParentPermissionMode(t *testing.T) {
	root := t.TempDir()
	provider := &captureAgentProvider{responses: []string{"planned"}}
	reg := registry.New()
	reg.Register(fakeTool{name: "Read"})
	reg.Register(fakeTool{name: "ExitPlanMode"})
	reg.Register(fakeTool{name: "EnterPlanMode"})
	tool := &AgentTool{
		Provider: provider,
		Registry: reg,
		Model:    "parent-model",
		InlineProfiles: map[string]agentProfile{
			"profile-plan": {
				Name:         "profile-plan",
				WhenToUse:    "Plan without teammate approval",
				SystemPrefix: "Profile plan system.",
			},
		},
	}
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: types.ToolRuntimeContext{ProjectRoot: root, AllowedDirs: []string{root}, PermissionMode: "plan"}})

	result, err := tool.Execute(context.Background(), agentExecuteInput("plan locally", map[string]any{
		"subagent_type": "profile-plan",
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected profile plan agent to run, got %s", result.Content)
	}
	if len(provider.params) != 1 {
		t.Fatalf("expected one provider call, got %d", len(provider.params))
	}
	if strings.Contains(provider.params[0].System, "lead approval") || strings.Contains(provider.params[0].System, "Permission mode is plan") {
		t.Fatalf("ordinary inherited plan mode must not inject teammate approval prompt:\n%s", provider.params[0].System)
	}
	if !strings.Contains(provider.params[0].System, "Permission mode: plan.") {
		t.Fatalf("expected inherited parent plan permission mode in system prompt, got:\n%s", provider.params[0].System)
	}
	toolNames := agentProviderToolNames(provider)
	if toolNames["ExitPlanMode"] {
		t.Fatalf("subagents must not receive permission transition tools, got %#v", toolNames)
	}
	if toolNames["EnterPlanMode"] {
		t.Fatalf("expected EnterPlanMode to remain base-filtered, got %#v", toolNames)
	}
}

func TestAgentProfileRuntimeDynamicMCPToolsRespectAllowAndDeny(t *testing.T) {
	manager := NewMCPManager()
	manager.AddServer(&MCPServer{
		Name: "github",
		Tools: []MCPServerTool{
			{Name: "search_code", Description: "Search code"},
		},
	})
	source := registry.New()
	source.Register(fakeTool{name: "Read"})
	source.Register(NewMCPTool(manager))
	dynamicName := "mcp__github__search_code"

	readOnlyProfile := agentProfile{
		Name:                  "read-only-mcp",
		MCPServers:            []string{"github"},
		AllowedToolsSpecified: true,
		AllowedTools:          map[string]struct{}{"read": {}},
	}
	filtered := registryForAgentProfile(source, readOnlyProfile)
	registerAgentMCPDynamicTools(source, filtered, readOnlyProfile)
	if filtered.Get(dynamicName) != nil {
		t.Fatalf("dynamic MCP tool should respect explicit tools allowlist")
	}

	allowMCPProfile := readOnlyProfile
	allowMCPProfile.Name = "allow-mcp"
	allowMCPProfile.AllowedTools = map[string]struct{}{"read": {}, dynamicName: {}}
	filtered = registryForAgentProfile(source, allowMCPProfile)
	registerAgentMCPDynamicTools(source, filtered, allowMCPProfile)
	if filtered.Get(dynamicName) == nil {
		t.Fatalf("expected explicitly allowed dynamic MCP tool %q", dynamicName)
	}

	disallowMCPProfile := agentProfile{
		Name:            "disallow-mcp",
		MCPServers:      []string{"github"},
		DisallowedTools: map[string]struct{}{dynamicName: {}},
	}
	filtered = registryForAgentProfile(source, disallowMCPProfile)
	registerAgentMCPDynamicTools(source, filtered, disallowMCPProfile)
	if filtered.Get(dynamicName) != nil {
		t.Fatalf("dynamic MCP tool should respect disallowedTools")
	}
}

func TestAgentProfileRuntimePrecedenceIncludesPluginProjectAndInline(t *testing.T) {
	configDir := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	t.Setenv("CLAUDE_CODE_PLUGIN_CACHE_DIR", "")
	t.Setenv("CLAUDE_CODE_USE_COWORK_PLUGINS", "")
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), []byte(`{"enabledPlugins":{"sample@market":true}}`), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	pluginRoot := filepath.Join(configDir, "plugins", "cache", "market", "sample", "1.0.0")
	if err := os.MkdirAll(filepath.Join(pluginRoot, ".claude-plugin"), 0o755); err != nil {
		t.Fatalf("mkdir plugin manifest dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(pluginRoot, "agents"), 0o755); err != nil {
		t.Fatalf("mkdir plugin agents dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginRoot, ".claude-plugin", "plugin.json"), []byte(`{"name":"sample"}`), 0o644); err != nil {
		t.Fatalf("write plugin manifest: %v", err)
	}
	pluginAgent := `---
name: review
description: Plugin review
---
Loaded from plugin profile.
`
	if err := os.WriteFile(filepath.Join(pluginRoot, "agents", "review.md"), []byte(pluginAgent), 0o644); err != nil {
		t.Fatalf("write plugin agent: %v", err)
	}

	profile, err := resolveAgentProfile("sample:review", cwd)
	if err != nil {
		t.Fatalf("resolve plugin profile: %v", err)
	}
	if !strings.Contains(profile.SystemPrefix, "plugin profile") {
		t.Fatalf("expected plugin profile before project override, got %#v", profile)
	}

	projectAgents := filepath.Join(cwd, ".claude", "agents")
	if err := os.MkdirAll(projectAgents, 0o755); err != nil {
		t.Fatalf("mkdir project agents: %v", err)
	}
	projectAgent := `---
name: sample:review
description: Project review
---
Loaded from project profile.
`
	if err := os.WriteFile(filepath.Join(projectAgents, "sample-review.md"), []byte(projectAgent), 0o644); err != nil {
		t.Fatalf("write project agent: %v", err)
	}
	profile, err = resolveAgentProfile("sample:review", cwd)
	if err != nil {
		t.Fatalf("resolve project profile: %v", err)
	}
	if !strings.Contains(profile.SystemPrefix, "project profile") {
		t.Fatalf("expected project profile to override plugin profile, got %#v", profile)
	}

	tool := &AgentTool{
		InlineProfiles: map[string]agentProfile{
			"sample:review": {
				Name:         "sample:review",
				WhenToUse:    "Inline review",
				SystemPrefix: "Loaded from inline profile.",
			},
		},
	}
	profile, err = tool.resolveProfileForInput(AgentInput{SubagentType: "sample:review", CWD: cwd})
	if err != nil {
		t.Fatalf("resolve inline profile: %v", err)
	}
	if !strings.Contains(profile.SystemPrefix, "inline profile") {
		t.Fatalf("expected inline profile to override project and plugin profiles, got %#v", profile)
	}
}

func TestAgentProfileRuntimeBackgroundAndSkillsDecorateBackgroundInput(t *testing.T) {
	_, skillsDir := makeTempSkillDir(t)
	writeMDSkill(t, skillsDir, "review", "Use this runtime review skill.")
	base := t.TempDir()
	background := NewBackgroundTaskManager(base)
	t.Cleanup(background.Shutdown)
	tool := &AgentTool{
		Provider:     &captureAgentProvider{responses: []string{"background done"}},
		Registry:     registry.New(),
		Background:   background,
		SkillManager: newTestSkillManager(skillsDir),
		InlineProfiles: map[string]agentProfile{
			"bg-skill": {
				Name:          "bg-skill",
				WhenToUse:     "Background skill profile",
				SystemPrefix:  "Background skill system.",
				Background:    true,
				Skills:        []string{"review"},
				InitialPrompt: "Read profile instructions first.",
			},
		},
	}

	result, err := tool.Execute(context.Background(), agentExecuteInput("main task", map[string]any{
		"subagent_type": "bg-skill",
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected background profile launch, got %s", result.Content)
	}
	payload := decodeJSONResult(t, result.Content)
	agentID, _ := payload["agentId"].(string)
	if agentID == "" {
		t.Fatalf("expected async launch payload with agentId, got %s", result.Content)
	}
	record, ok := background.store.Get(agentID)
	if !ok || record.AgentInput == nil {
		t.Fatalf("expected persisted background agent input for %s", agentID)
	}
	if !record.AgentInput.RunInBackground {
		t.Fatalf("expected background frontmatter to force run_in_background")
	}
	for _, expected := range []string{"Read profile instructions first.", `<skill name="review">`, "Use this runtime review skill.", "main task"} {
		if !strings.Contains(record.AgentInput.Prompt, expected) {
			t.Fatalf("expected decorated background prompt to contain %q, got:\n%s", expected, record.AgentInput.Prompt)
		}
	}
}

func TestAgentProfileRuntimeIsolationFrontmatterCreatesWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	repo := t.TempDir()
	runGitCommand(t, repo, "init")
	runGitCommand(t, repo, "config", "user.email", "test@example.com")
	runGitCommand(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "note.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	runGitCommand(t, repo, "add", "note.txt")
	runGitCommand(t, repo, "commit", "-m", "initial")

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir repo: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	tool := &AgentTool{
		Provider: &captureAgentProvider{responses: []string{"isolated"}},
		Registry: registry.New(),
		InlineProfiles: map[string]agentProfile{
			"isolate-profile": {
				Name:         "isolate-profile",
				WhenToUse:    "Use a worktree",
				SystemPrefix: "Isolated system.",
				Isolation:    "worktree",
			},
		},
	}
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: types.ToolRuntimeContext{ProjectRoot: repo, AllowedDirs: []string{repo}, PermissionMode: permissionModeDefault}})
	bundle, err := tool.buildSubAgentLoop("agent-profile-isolation", AgentInput{
		Prompt:       "inspect",
		SubagentType: "isolate-profile",
	})
	if err != nil {
		t.Fatalf("buildSubAgentLoop: %v", err)
	}
	t.Cleanup(func() {
		_, _ = runGit(repo, "worktree", "remove", bundle.Metadata.WorktreePath, "--force")
		_, _ = runGit(repo, "branch", "-D", bundle.Metadata.WorktreeBranch)
	})
	if bundle.Metadata.Isolation != "worktree" || bundle.Metadata.WorktreePath == "" || bundle.Metadata.CWD != bundle.Metadata.WorktreePath {
		t.Fatalf("expected profile isolation frontmatter to create worktree metadata, got %#v", bundle.Metadata)
	}
	if _, err := os.Stat(bundle.Metadata.WorktreePath); err != nil {
		t.Fatalf("expected profile worktree path to exist: %v", err)
	}
}

func agentProviderToolNames(provider *captureAgentProvider) map[string]bool {
	out := map[string]bool{}
	if provider == nil || len(provider.params) == 0 {
		return out
	}
	for _, def := range provider.params[0].Tools {
		out[def.Name] = true
	}
	return out
}
