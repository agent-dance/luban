package agent

import (
	"context"
	agentcontract "github.com/agent-dance/luban/internal/contracts/agent"
	"sync"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/permission"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type mutableAgentRuntimeProvider struct {
	mu      sync.RWMutex
	runtime types.ToolRuntimeContext
}

func (p *mutableAgentRuntimeProvider) ToolRuntimeContext() types.ToolRuntimeContext {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return cloneToolRuntimeContext(p.runtime)
}

// ToolRuntimeContextUnbarriered marks this test provider as safe to sample
// while AgentTool holds the session publication barrier. Its own mutex is
// independent of that barrier, matching the production runtime-scope port.
func (p *mutableAgentRuntimeProvider) ToolRuntimeContextUnbarriered() types.ToolRuntimeContext {
	return p.ToolRuntimeContext()
}

func (p *mutableAgentRuntimeProvider) set(runtime types.ToolRuntimeContext) {
	p.mu.Lock()
	p.runtime = cloneToolRuntimeContext(runtime)
	p.mu.Unlock()
}

func TestAgentLaunchSnapshotsCurrentParentPermissionMode(t *testing.T) {
	root := t.TempDir()
	barrier := &sync.RWMutex{}
	scope := &mutableAgentRuntimeProvider{runtime: types.ToolRuntimeContext{
		ProjectRoot: root, AllowedDirs: []string{root}, PermissionMode: permissionModeDefault,
	}}
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

	scope.set(types.ToolRuntimeContext{
		ProjectRoot:    root,
		AllowedDirs:    []string{root},
		PermissionMode: "bypassPermissions",
		AllowedTools:   map[string]bool{"Bash": true},
		DeniedTools:    map[string]bool{"Write": true},
		AllowedRules:   []types.PermissionRuleValue{{ToolName: "Bash"}},
		DeniedRules:    []types.PermissionRuleValue{{ToolName: "Write"}},
		AskRules:       []types.PermissionRuleValue{{ToolName: "Bash", RuleContent: "git push *"}},
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
		ShellPolicyAnalyzer: func(string, types.PolicyContext) types.PolicyDecision {
			return types.PolicyDecision{Disposition: types.PolicyAllow}
		},
	})
	t.Cleanup(func() { permissions.SetSafetyConfig(permissions.SafetyConfig{}) })
	prompted := false
	checker := permissions.NewChecker(permissions.ModeAskAlways, nil)
	checker.SetStructuredPromptFunc(func(context.Context, permissions.PromptRequest) permissions.PromptResponse {
		prompted = true
		return permissions.PromptResponse{Decision: permissions.DecisionDeny, Outcome: permissions.PromptOutcomeRejected}
	})
	parent := permissions.NewCLIPermissionHandler(checker)
	handler := agentPermissionHandlerForSnapshot(launch.session.ToolRuntime, parent, agentcontract.ApprovalAttached, agentProfile{}, "parent-session")
	decision, err := handler.Check(context.Background(), permission.PermissionRequest{
		SessionID: "child-session",
		ToolName:  "Bash",
		Input: map[string]any{
			"command": `curl -s "https://www.google.com/search?q=test" 2>/dev/null | head -200`,
		},
	})
	if err != nil {
		t.Fatalf("check inherited Auto curl permission: %v", err)
	}
	if decision != permission.PermissionAllow || prompted {
		t.Fatalf("inherited Auto curl decision=%v prompted=%v, want allow without prompt", decision, prompted)
	}
}

func TestAgentPermissionHandlerEvaluatesSpawnSnapshotInRealChecker(t *testing.T) {
	checker := permissions.NewChecker(permissions.ModeRuleBased, []permissions.Rule{{
		Tool: "Write", Decision: permissions.DecisionAllow,
	}})
	checker.SetStructuredPromptFunc(func(context.Context, permissions.PromptRequest) permissions.PromptResponse {
		return permissions.PromptResponse{Decision: permissions.DecisionDeny, Outcome: permissions.PromptOutcomeRejected}
	})
	parent := permissions.NewCLIPermissionHandler(checker)
	handler := agentPermissionHandlerForSnapshot(types.ToolRuntimeContext{
		SessionID: "parent-session", PermissionMode: permissionModeDefault,
	}, parent, agentcontract.ApprovalAttached, agentProfile{}, "parent-session")

	decision, err := handler.Check(context.Background(), permission.PermissionRequest{
		SessionID: "child-session", ToolName: "Write", Input: map[string]any{"file_path": "note.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision != permission.PermissionDeny {
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
