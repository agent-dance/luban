package permissions

import "testing"

func TestMCPPermissionRuleMatchesServerToolAndWildcard(t *testing.T) {
	cases := []struct {
		name string
		rule string
		tool string
		want bool
	}{
		{name: "server level", rule: "mcp__github", tool: "mcp__github__create_issue", want: true},
		{name: "wildcard level", rule: "mcp__github__*", tool: "mcp__github__create_issue", want: true},
		{name: "tool exact", rule: "mcp__github__create_issue", tool: "mcp__github__create_issue", want: true},
		{name: "other tool", rule: "mcp__github__read_issue", tool: "mcp__github__create_issue", want: false},
		{name: "other server", rule: "mcp__linear", tool: "mcp__github__create_issue", want: false},
		{name: "builtin name isolated", rule: "Write", tool: "mcp__github__Write", want: false},
		{name: "server identity is not concrete tool", rule: "mcp__github", tool: "mcp__github", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MCPPermissionRuleMatches(tc.rule, tc.tool); got != tc.want {
				t.Fatalf("MCPPermissionRuleMatches(%q, %q) = %v, want %v", tc.rule, tc.tool, got, tc.want)
			}
		})
	}
}

func TestParseMCPPermissionIdentityPreservesToolDoubleUnderscore(t *testing.T) {
	info, ok := ParseMCPPermissionIdentity("mcp__srv__tool__with__parts")
	if !ok {
		t.Fatalf("expected MCP identity")
	}
	if info.ServerName != "srv" || !info.HasTool || info.ToolName != "tool__with__parts" {
		t.Fatalf("parsed info = %#v", info)
	}
}

func TestMCPPermissionRuleValidRejectsInputPatterns(t *testing.T) {
	if err := MCPPermissionRuleValid("mcp__github", false); err != nil {
		t.Fatalf("server-level rule should be valid: %v", err)
	}
	if err := MCPPermissionRuleValid("mcp__github__*", false); err != nil {
		t.Fatalf("wildcard rule should be valid: %v", err)
	}
	if err := MCPPermissionRuleValid("mcp__github(pattern)", true); err == nil {
		t.Fatalf("expected MCP pattern rule to be rejected")
	}
}

func TestMCPToolPermissionIdentityIsolatesReplacementNames(t *testing.T) {
	got := MCPToolPermissionIdentity("Write", "my server", "Write")
	if got != "mcp__my_server__Write" {
		t.Fatalf("identity = %q", got)
	}
	if fallback := MCPToolPermissionIdentity("Write", "", ""); fallback != "Write" {
		t.Fatalf("fallback = %q", fallback)
	}
}

func TestClassifyMCPAnnotations(t *testing.T) {
	readOnly := ClassifyMCPAnnotations(map[string]any{"readOnlyHint": true})
	if !readOnly.ReadOnly || !readOnly.ConcurrentSafe || readOnly.Risk != RiskLow {
		t.Fatalf("readOnly classification = %#v", readOnly)
	}

	destructive := ClassifyMCPAnnotations(map[string]any{
		"readOnlyHint":    true,
		"destructiveHint": "true",
	})
	if !destructive.Destructive || destructive.ConcurrentSafe || destructive.Risk != RiskHigh {
		t.Fatalf("destructive classification = %#v", destructive)
	}

	openWorld := ClassifyMCPAnnotations(map[string]any{
		"readOnlyHint":  true,
		"openWorldHint": true,
	})
	if !openWorld.OpenWorld || openWorld.Risk != RiskMedium {
		t.Fatalf("openWorld classification = %#v", openWorld)
	}
}
