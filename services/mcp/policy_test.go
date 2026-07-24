package mcp

import (
	"strings"
	"testing"
)

func TestMCPPolicyDenyPrecedenceAndAllowlistFailClosed(t *testing.T) {
	configs := map[string]MCPServerConfig{
		"ok":      {Type: TransportStdio, Command: "npx", Args: []string{"-y", "ok"}},
		"blocked": {Type: TransportStdio, Command: "npx", Args: []string{"-y", "ok"}},
		"other":   {Type: TransportStdio, Command: "npx", Args: []string{"-y", "other"}},
		"sdk":     {Type: TransportSDK, Name: "claude-vscode"},
	}
	settings := MCPPolicySettings{
		AllowedMCPServersSet: true,
		AllowedMCPServers: []MCPPolicyEntry{{
			ServerCommand: []string{"npx", "-y", "ok"},
		}},
		DeniedMCPServers: []MCPPolicyEntry{{ServerName: "blocked"}},
	}

	result := FilterMCPServersByPolicy(configs, settings)
	if _, ok := result.Allowed["ok"]; !ok {
		t.Fatalf("expected ok server allowed, got %#v", result)
	}
	if _, ok := result.Allowed["sdk"]; !ok {
		t.Fatalf("expected sdk placeholder exempt, got %#v", result)
	}
	if _, ok := result.Blocked["blocked"]; !ok {
		t.Fatalf("expected explicit deny to win, got %#v", result)
	}
	if decision, ok := result.Blocked["other"]; !ok || !strings.Contains(decision.Reason, "command") {
		t.Fatalf("expected other stdio server blocked by command allowlist, got %#v", result.Blocked["other"])
	}
}

func TestMCPPolicyRemoteURLWildcard(t *testing.T) {
	settings := MCPPolicySettings{
		AllowedMCPServersSet: true,
		AllowedMCPServers: []MCPPolicyEntry{{
			ServerURL: "https://*.example.com/mcp/*",
		}},
	}
	allowed := IsMCPServerAllowedByPolicy("remote", MCPServerConfig{
		Type: TransportHTTP,
		URL:  "https://api.example.com/mcp/v1",
	}, settings)
	if !allowed.Allowed {
		t.Fatalf("expected wildcard URL allow, got %#v", allowed)
	}
	blocked := IsMCPServerAllowedByPolicy("remote", MCPServerConfig{
		Type: TransportHTTP,
		URL:  "https://evil.example.net/mcp/v1",
	}, settings)
	if blocked.Allowed || !strings.Contains(blocked.Reason, "URL") {
		t.Fatalf("expected URL block, got %#v", blocked)
	}
}

func TestMCPPolicyNilAllowlistAllowsEmptyAllowlistBlocks(t *testing.T) {
	config := MCPServerConfig{Type: TransportHTTP, URL: "https://example.com/mcp"}
	if decision := IsMCPServerAllowedByPolicy("srv", config, MCPPolicySettings{}); !decision.Allowed {
		t.Fatalf("nil allowlist should allow, got %#v", decision)
	}
	decision := IsMCPServerAllowedByPolicy("srv", config, MCPPolicySettings{AllowedMCPServersSet: true})
	if decision.Allowed || !strings.Contains(decision.Reason, "empty") {
		t.Fatalf("empty allowlist should block all, got %#v", decision)
	}
}

func TestProjectMCPApprovalStatusAndFiltering(t *testing.T) {
	settings := ProjectApprovalSettings{
		EnabledMCPJSONServers:  []string{"approved server"},
		DisabledMCPJSONServers: []string{"rejected server"},
	}
	if got := ProjectMCPServerStatus("approved_server", settings); got != ProjectApprovalApproved {
		t.Fatalf("approved status = %q", got)
	}
	if got := ProjectMCPServerStatus("rejected_server", settings); got != ProjectApprovalRejected {
		t.Fatalf("rejected status = %q", got)
	}
	if got := ProjectMCPServerStatus("new_server", settings); got != ProjectApprovalPending {
		t.Fatalf("pending status = %q", got)
	}

	configs := map[string]MCPServerConfig{
		"approved_server": {Scope: ScopeProject, Type: TransportStdio, Command: "ok"},
		"new_server":      {Scope: ScopeProject, Type: TransportStdio, Command: "new"},
		"user_server":     {Scope: ScopeUser, Type: TransportStdio, Command: "user"},
	}
	approved, blocked := FilterApprovedProjectMCPServers(configs, settings)
	if _, ok := approved["approved_server"]; !ok {
		t.Fatalf("approved project server missing: %#v", approved)
	}
	if _, ok := approved["user_server"]; !ok {
		t.Fatalf("non-project server should pass approval filter: %#v", approved)
	}
	if blocked["new_server"] != ProjectApprovalPending {
		t.Fatalf("new server block status = %#v", blocked)
	}
}

func TestPendingProjectApprovalRequestsShape(t *testing.T) {
	requests := PendingProjectApprovalRequests(map[string]MCPServerConfig{
		"pending": {Scope: ScopeProject, Type: TransportStdio, Command: "node", Args: []string{"server.js"}},
	}, ProjectApprovalSettings{})
	if len(requests) != 1 {
		t.Fatalf("requests = %#v", requests)
	}
	if requests[0].ServerName != "pending" || requests[0].Scope != ScopeProject || requests[0].Status != ProjectApprovalPending || requests[0].ConfigHash == "" {
		t.Fatalf("bad approval request shape: %#v", requests[0])
	}
}

func TestDedupPluginMCPServersManualAndFirstPluginWin(t *testing.T) {
	plugins := map[string]MCPServerConfig{
		"plugin:slack:main": {Scope: ScopeDynamic, Type: TransportHTTP, URL: "https://slack.example/mcp", Headers: map[string]string{"Authorization": "Bearer plugin"}},
		"plugin:a:dup":      {Scope: ScopeDynamic, Type: TransportStdio, Command: "npx", Args: []string{"srv"}},
		"plugin:b:dup":      {Scope: ScopeDynamic, Type: TransportStdio, Command: "npx", Args: []string{"srv"}, Env: map[string]string{"CLAUDE_PLUGIN_ROOT": "/tmp/plugin"}},
		"plugin:c:sdk":      {Scope: ScopeDynamic, Type: TransportSDK, Name: "custom-sdk"},
	}
	manual := map[string]MCPServerConfig{
		"slack": {Scope: ScopeUser, Type: TransportHTTP, URL: "https://slack.example/mcp", Headers: map[string]string{"Authorization": "Bearer user"}},
	}
	deduped, suppressed := DedupPluginMCPServers(plugins, manual)
	if _, ok := deduped["plugin:slack:main"]; ok {
		t.Fatalf("manual duplicate plugin was not suppressed: %#v", deduped)
	}
	if _, ok := deduped["plugin:a:dup"]; !ok {
		t.Fatalf("first plugin duplicate should remain: %#v", deduped)
	}
	if _, ok := deduped["plugin:b:dup"]; ok {
		t.Fatalf("second plugin duplicate should be suppressed: %#v", deduped)
	}
	if _, ok := deduped["plugin:c:sdk"]; !ok {
		t.Fatalf("signature-less sdk plugin should remain: %#v", deduped)
	}
	if len(suppressed) != 2 {
		t.Fatalf("suppressed = %#v", suppressed)
	}
}

func TestDedupClaudeAIUsesUnwrappedCCRProxyURL(t *testing.T) {
	manual := map[string]MCPServerConfig{
		"slack": {Scope: ScopeUser, Type: TransportHTTP, URL: "https://mcp.slack.com/shttp"},
	}
	connectors := map[string]MCPServerConfig{
		"claude.ai Slack": {
			Scope: ScopeClaudeAI,
			Type:  TransportClaudeAIProxy,
			URL:   "https://claude.ai/v2/session_ingress/shttp/mcp/x?mcp_url=https%3A%2F%2Fmcp.slack.com%2Fshttp",
		},
		"claude.ai Other": {Scope: ScopeClaudeAI, Type: TransportHTTP, URL: "https://other.example/mcp"},
	}
	deduped, suppressed := DedupClaudeAIMCPServers(connectors, manual)
	if _, ok := deduped["claude.ai Slack"]; ok {
		t.Fatalf("duplicate connector was not suppressed: %#v", deduped)
	}
	if _, ok := deduped["claude.ai Other"]; !ok {
		t.Fatalf("non-duplicate connector missing: %#v", deduped)
	}
	if len(suppressed) != 1 || suppressed[0].DuplicateOf != "slack" {
		t.Fatalf("suppressed = %#v", suppressed)
	}
}

func TestGateChannelServerRequiresCapabilityAuthPolicySessionAndAllowlist(t *testing.T) {
	capabilities := ServerCapabilities{
		"experimental": map[string]any{"claude/channel": map[string]any{}},
	}
	channelsEnabled := true
	opts := ChannelGateOptions{
		ChannelsEnabled:        true,
		HasClaudeAIOAuth:       true,
		Subscription:           "enterprise",
		ManagedChannelsEnabled: &channelsEnabled,
		SessionChannels: []ChannelEntry{{
			Kind:        "plugin",
			Name:        "telegram",
			Marketplace: "anthropic",
		}},
		AllowedChannelPlugins: []ChannelAllowlistEntry{{
			Plugin:      "telegram",
			Marketplace: "anthropic",
		}},
	}
	if got := GateChannelServer("plain", nil, "", opts); got.Kind != "capability" {
		t.Fatalf("missing capability gate = %#v", got)
	}
	if got := GateChannelServer("plugin:telegram:tg", capabilities, "telegram@evil", opts); got.Kind != "marketplace" {
		t.Fatalf("marketplace gate = %#v", got)
	}
	if got := GateChannelServer("plugin:telegram:tg", capabilities, "telegram@anthropic", opts); got.Action != "register" {
		t.Fatalf("expected register, got %#v", got)
	}
	if name, marketplace := ParsePluginIdentifier("telegram@anthropic@ignored"); name != "telegram" || marketplace != "anthropic" {
		t.Fatalf("ParsePluginIdentifier mismatch: name=%q marketplace=%q", name, marketplace)
	}
}

func TestGateChannelServerDeniesNonDevServerEntries(t *testing.T) {
	capabilities := ServerCapabilities{
		"experimental": map[string]any{"claude/channel": true},
	}
	got := GateChannelServer("raw-server", capabilities, "", ChannelGateOptions{
		ChannelsEnabled:  true,
		HasClaudeAIOAuth: true,
		SessionChannels:  []ChannelEntry{{Kind: "server", Name: "raw-server"}},
	})
	if got.Kind != "allowlist" {
		t.Fatalf("server entry should fail allowlist by default, got %#v", got)
	}
	dev := GateChannelServer("raw-server", capabilities, "", ChannelGateOptions{
		ChannelsEnabled:  true,
		HasClaudeAIOAuth: true,
		SessionChannels:  []ChannelEntry{{Kind: "server", Name: "raw-server", Dev: true}},
	})
	if dev.Action != "register" {
		t.Fatalf("dev server entry should register, got %#v", dev)
	}
}

func TestFilterPermissionRelayConnectionsRequiresBothCapabilities(t *testing.T) {
	connections := []MCPServerConnection{
		{Name: "ok", Type: MCPStateConnected, Capabilities: ServerCapabilities{"experimental": map[string]any{"claude/channel": true, "claude/channel/permission": map[string]any{}}}},
		{Name: "text-only", Type: MCPStateConnected, Capabilities: ServerCapabilities{"experimental": map[string]any{"claude/channel": true}}},
		{Name: "pending", Type: MCPStatePending, Capabilities: ServerCapabilities{"experimental": map[string]any{"claude/channel": true, "claude/channel/permission": true}}},
		{Name: "not-allowed", Type: MCPStateConnected, Capabilities: ServerCapabilities{"experimental": map[string]any{"claude/channel": true, "claude/channel/permission": true}}},
	}
	got := FilterPermissionRelayConnections(connections, func(name string) bool { return name != "not-allowed" })
	if len(got) != 1 || got[0].Name != "ok" {
		t.Fatalf("relay connections = %#v", got)
	}
}

func TestChannelPermissionCallbacksResolveOnce(t *testing.T) {
	callbacks := NewChannelPermissionCallbacks()
	var got ChannelPermissionResponse
	callbacks.OnResponse("AbCdE", func(response ChannelPermissionResponse) {
		got = response
	})
	if !callbacks.Resolve("abcde", "allow", "plugin:telegram:tg") {
		t.Fatalf("expected resolve to find pending request")
	}
	if got.Behavior != "allow" || got.FromServer != "plugin:telegram:tg" {
		t.Fatalf("response = %#v", got)
	}
	if callbacks.Resolve("abcde", "deny", "plugin:telegram:tg") {
		t.Fatalf("duplicate response should not resolve")
	}
}

func TestChannelPermissionRequestShapeAndPreview(t *testing.T) {
	request := BuildChannelPermissionRequest("toolu_123", "mcp__srv__send", "Send message", map[string]any{
		"text": strings.Repeat("x", 250),
	})
	if !PermissionReplyPattern.MatchString("yes " + request.RequestID) {
		t.Fatalf("request id does not match reply pattern: %#v", request)
	}
	if request.ToolName != "mcp__srv__send" || request.Description != "Send message" {
		t.Fatalf("bad request shape: %#v", request)
	}
	if len(request.InputPreview) > 203 || !strings.HasSuffix(request.InputPreview, "...") {
		t.Fatalf("preview not truncated as expected: %q", request.InputPreview)
	}
}

func TestWrapChannelMessageEscapesAttributesAndFiltersKeys(t *testing.T) {
	wrapped := WrapChannelMessage("srv&1", "hello", map[string]string{
		"user":        `a"b`,
		`bad key`:     "drop",
		`x" injected`: "drop",
		"thread_id":   "42",
	})
	if !strings.Contains(wrapped, `source="srv&amp;1"`) || !strings.Contains(wrapped, `user="a&#34;b"`) || !strings.Contains(wrapped, `thread_id="42"`) {
		t.Fatalf("wrapped message missing escaped attrs: %s", wrapped)
	}
	if strings.Contains(wrapped, "bad key") || strings.Contains(wrapped, "injected") {
		t.Fatalf("unsafe meta key leaked: %s", wrapped)
	}
}
