package tools

import (
	"context"
	"sync"
	"testing"

	"github.com/agent-dance/luban/engine"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestAgentLaunchSnapshotsCurrentParentPermissionMode(t *testing.T) {
	root := t.TempDir()
	barrier := &sync.RWMutex{}
	scope := NewRuntimeScope(root, true)
	scope.SetSessionBarrier(barrier)
	scope.SetPermissionModeDispatcher(func() string { return permissionModeDefault }, func(string) error { return nil })
	reg := registry.New()
	reg.SetRuntimeContextProvider(scope)
	tool := &AgentTool{Registry: reg}
	tool.SetSessionBarrier(barrier)
	tool.SetSessionRuntime(AgentSessionRuntime{ToolRuntime: types.ToolRuntimeContext{
		SessionID:      "parent-session",
		ProjectRoot:    root,
		AllowedDirs:    []string{root},
		PermissionMode: permissionModeDefault,
	}})

	if err := scope.TransitionPermissionMode("bypassPermissions"); err != nil {
		t.Fatalf("switch parent to Auto mode: %v", err)
	}
	scope.ApplyPermissionUpdates([]types.PermissionUpdate{
		{Type: types.PermissionUpdateAddRules, Behavior: types.PermissionBehaviorAllow, Rules: []types.PermissionRuleValue{{ToolName: "Bash"}}},
		{Type: types.PermissionUpdateAddRules, Behavior: types.PermissionBehaviorDeny, Rules: []types.PermissionRuleValue{{ToolName: "Write"}}},
		{Type: types.PermissionUpdateAddRules, Behavior: types.PermissionBehaviorAsk, Rules: []types.PermissionRuleValue{{ToolName: "Bash", RuleContent: "git push *"}}},
	})

	launch := tool.captureLaunchRuntime()
	if got := launch.session.ToolRuntime.PermissionMode; got != "bypassPermissions" {
		t.Fatalf("child launch permission mode = %q, want current parent Auto mode; stale session cache was inherited", got)
	}
	if len(launch.session.ToolRuntime.AllowedRules) != 1 || launch.session.ToolRuntime.AllowedRules[0].ToolName != "Bash" ||
		len(launch.session.ToolRuntime.DeniedRules) != 1 || launch.session.ToolRuntime.DeniedRules[0].ToolName != "Write" ||
		len(launch.session.ToolRuntime.AskRules) != 1 || launch.session.ToolRuntime.AskRules[0].RuleContent != "git push *" {
		t.Fatalf("child launch lost live parent permission rules: %+v", launch.session.ToolRuntime)
	}

	permissions.SetSafetyConfig(permissions.SafetyConfig{
		DangerousCommandChecker:  func(string) string { return "" },
		BashProtectedPathChecker: func(string) (bool, string) { return false, "" },
	})
	t.Cleanup(func() { permissions.SetSafetyConfig(permissions.SafetyConfig{}) })
	prompted := false
	checker := permissions.NewChecker(permissions.ModeAskAlways, nil)
	checker.SetStructuredPromptFunc(func(context.Context, permissions.PromptRequest) permissions.PromptResponse {
		prompted = true
		return permissions.PromptResponse{Decision: permissions.DecisionDeny, Outcome: permissions.PromptOutcomeRejected}
	})
	parent := engine.AsLoopPermissionHandler(permissions.NewCLIPermissionHandler(checker))
	handler := agentPermissionHandlerForSnapshot(launch.session.ToolRuntime, parent, approvalRouteAttached, agentProfile{}, "parent-session")
	decision, err := handler.Check(context.Background(), loop.PermissionRequest{
		SessionID: "child-session",
		ToolName:  "Bash",
		Input: map[string]any{
			"command": `curl -s "https://www.google.com/search?q=test" 2>/dev/null | head -200`,
		},
	})
	if err != nil {
		t.Fatalf("check inherited Auto curl permission: %v", err)
	}
	if decision != loop.PermissionAllow || prompted {
		t.Fatalf("inherited Auto curl decision=%v prompted=%v, want allow without prompt", decision, prompted)
	}
}

func TestTeamLaunchSnapshotsCurrentParentPermissionMode(t *testing.T) {
	root := t.TempDir()
	barrier := &sync.RWMutex{}
	scope := NewRuntimeScope(root, true)
	scope.SetSessionBarrier(barrier)
	scope.SetPermissionModeDispatcher(func() string { return permissionModeDefault }, func(string) error { return nil })
	reg := registry.New()
	reg.SetRuntimeContextProvider(scope)
	mgr := &TeamManager{Registry: reg}
	mgr.SetSessionBarrier(barrier)
	mgr.SetSessionRuntime(TeamSessionRuntime{
		SessionID: "parent-session",
		CWD:       root,
		ToolRuntime: types.ToolRuntimeContext{
			SessionID:      "parent-session",
			ProjectRoot:    root,
			AllowedDirs:    []string{root},
			PermissionMode: permissionModeDefault,
		},
	})

	if err := scope.TransitionPermissionMode("bypassPermissions"); err != nil {
		t.Fatalf("switch parent to Auto mode: %v", err)
	}

	launch := mgr.captureLaunchRuntime()
	if got := launch.session.ToolRuntime.PermissionMode; got != "bypassPermissions" {
		t.Fatalf("team child launch permission mode = %q, want current parent Auto mode; stale session cache was inherited", got)
	}
}

func TestCanonicalAgentModeTreatsAutoAsBypassPermissions(t *testing.T) {
	if got := canonicalAgentMode("auto"); got != "bypassPermissions" {
		t.Fatalf("canonicalAgentMode(auto) = %q, want bypassPermissions", got)
	}
}

func TestAgentPermissionHandlerEvaluatesSpawnSnapshotInRealChecker(t *testing.T) {
	checker := permissions.NewChecker(permissions.ModeRuleBased, []permissions.Rule{{
		Tool: "Write", Decision: permissions.DecisionAllow,
	}})
	checker.SetStructuredPromptFunc(func(context.Context, permissions.PromptRequest) permissions.PromptResponse {
		return permissions.PromptResponse{Decision: permissions.DecisionDeny, Outcome: permissions.PromptOutcomeRejected}
	})
	parent := engine.AsLoopPermissionHandler(permissions.NewCLIPermissionHandler(checker))
	handler := agentPermissionHandlerForSnapshot(types.ToolRuntimeContext{
		SessionID: "parent-session", PermissionMode: permissionModeDefault,
	}, parent, approvalRouteAttached, agentProfile{}, "parent-session")

	decision, err := handler.Check(context.Background(), loop.PermissionRequest{
		SessionID: "child-session", ToolName: "Write", Input: map[string]any{"file_path": "note.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision != loop.PermissionDeny {
		t.Fatalf("mutable foreground rule overrode child spawn snapshot: %v", decision)
	}
}

func TestPinnedBashRulesDoNotShortCircuitMandatoryChecksWithAllow(t *testing.T) {
	rules := agentPermissionRulesForRuntime(types.ToolRuntimeContext{
		AllowedRules: []types.PermissionRuleValue{{ToolName: "Bash", RuleContent: "*"}},
		DeniedRules:  []types.PermissionRuleValue{{ToolName: "Bash", RuleContent: "rm *"}},
		AskRules:     []types.PermissionRuleValue{{ToolName: "Bash", RuleContent: "git push *"}},
	})
	if len(rules) != 2 {
		t.Fatalf("pinned Bash rules = %#v, want only deny+ask", rules)
	}
	for _, rule := range rules {
		if rule.Decision == permissions.DecisionAllow {
			t.Fatalf("Bash allow rule bypassed snapshot-aware mandatory handler: %#v", rules)
		}
	}
}
