package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestParseMCPConfigStdioDefaultsTypeAndPreservesScope(t *testing.T) {
	data := []byte(`{
		"mcpServers": {
			"filesystem": {
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-filesystem"]
			}
		}
	}`)

	result, err := ParseMCPConfig(data, ParseOptions{Scope: ScopeProject})
	if err != nil {
		t.Fatalf("ParseMCPConfig: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %#v", result.Errors)
	}
	cfg := result.Servers["filesystem"]
	if cfg.Type != TransportStdio {
		t.Fatalf("type = %q, want %q", cfg.Type, TransportStdio)
	}
	if cfg.Scope != ScopeProject {
		t.Fatalf("scope = %q, want project", cfg.Scope)
	}
	if len(cfg.Args) != 2 {
		t.Fatalf("args = %#v", cfg.Args)
	}
}

func TestParseMCPConfigAcceptsRemoteHeadersOAuthAndSDKShapes(t *testing.T) {
	data := []byte(`{
		"mcpServers": {
			"sse": {
				"type": "sse",
				"url": "https://example.com/sse",
				"headers": {"Authorization": "Bearer ${TOKEN:-fallback}"},
				"headersHelper": "op read token",
				"oauth": {
					"clientId": "client-1",
					"callbackPort": 8765,
					"authServerMetadataUrl": "https://auth.example.com/.well-known/oauth-authorization-server",
					"xaa": true
				}
			},
			"http": {
				"type": "http",
				"url": "https://example.com/mcp",
				"headers": {"X-Api-Key": "abc"}
			},
			"ws": {
				"type": "ws",
				"url": "wss://example.com/mcp",
				"headers": {"X-Trace": "1"},
				"headersHelper": "helper"
			},
			"sseide": {
				"type": "sse-ide",
				"url": "http://127.0.0.1:1234/sse",
				"ideName": "vscode",
				"ideRunningInWindows": true
			},
			"wside": {
				"type": "ws-ide",
				"url": "ws://127.0.0.1:1234/mcp",
				"ideName": "vscode",
				"authToken": "token"
			},
			"sdk": {
				"type": "sdk",
				"name": "claude-vscode"
			},
			"claudeai": {
				"type": "claudeai-proxy",
				"url": "https://claude.ai/api/mcp",
				"id": "connector-id"
			}
		}
	}`)

	result, err := ParseMCPConfig(data, ParseOptions{Scope: ScopeUser, ExpandVars: true})
	if err != nil {
		t.Fatalf("ParseMCPConfig: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %#v", result.Errors)
	}
	sse := result.Servers["sse"]
	if sse.Type != TransportSSE || sse.URL != "https://example.com/sse" {
		t.Fatalf("sse config not preserved: %#v", sse)
	}
	if sse.Headers["Authorization"] != "Bearer fallback" {
		t.Fatalf("expanded header = %q", sse.Headers["Authorization"])
	}
	if sse.HeadersHelper != "op read token" {
		t.Fatalf("headersHelper = %q", sse.HeadersHelper)
	}
	if sse.OAuth == nil || sse.OAuth.ClientID != "client-1" || sse.OAuth.CallbackPort == nil || *sse.OAuth.CallbackPort != 8765 {
		t.Fatalf("oauth fields not preserved: %#v", sse.OAuth)
	}
	if sse.OAuth.XAA == nil || !*sse.OAuth.XAA {
		t.Fatalf("oauth xaa not preserved: %#v", sse.OAuth)
	}
	if result.Servers["sdk"].Name != "claude-vscode" {
		t.Fatalf("sdk name = %q", result.Servers["sdk"].Name)
	}
	if result.Servers["claudeai"].ID != "connector-id" {
		t.Fatalf("claudeai id = %q", result.Servers["claudeai"].ID)
	}
	if result.Servers["wside"].AuthToken != "token" || result.Servers["sseide"].IDERunningInWindows == nil || !*result.Servers["sseide"].IDERunningInWindows {
		t.Fatalf("IDE fields not preserved")
	}
}

func TestParseMCPConfigPreservesEnabledDisabledSourceControls(t *testing.T) {
	data := []byte(`{
		"enabledMcpServers": ["computer-use"],
		"disabledMcpServers": ["github"],
		"enabledMcpjsonServers": ["project-a"],
		"disabledMcpjsonServers": ["project-b"],
		"mcpServers": {
			"github": {"command": "node"}
		}
	}`)

	result, err := ParseMCPConfig(data, ParseOptions{Scope: ScopeLocal})
	if err != nil {
		t.Fatalf("ParseMCPConfig: %v", err)
	}
	if !result.IsServerDisabled("github") {
		t.Fatalf("github should be disabled")
	}
	if !result.IsServerEnabled("computer-use") {
		t.Fatalf("computer-use should be enabled")
	}
	if !contains(result.EnabledMCPJSONServers, "project-a") || !contains(result.DisabledMCPJSONServers, "project-b") {
		t.Fatalf("mcpjson approval lists not preserved: %#v", result)
	}
	if result.Servers["github"].Scope != ScopeLocal {
		t.Fatalf("scope = %q", result.Servers["github"].Scope)
	}
}

func TestExpandEnvVarsInConfigCollectsMissingVariables(t *testing.T) {
	t.Setenv("MCP_CMD", "node")
	t.Setenv("TOKEN", "secret")

	result, err := ParseMCPConfig([]byte(`{
		"mcpServers": {
			"envtest": {
				"command": "${MCP_CMD}",
				"args": ["${MISSING}", "${OPTIONAL:-default-value}"],
				"env": {"TOKEN": "${TOKEN}", "AGAIN": "${MISSING}"},
				"type": "stdio"
			},
			"remote": {
				"type": "http",
				"url": "https://${HOST}/mcp",
				"headers": {"Authorization": "Bearer ${TOKEN}"}
			}
		}
	}`), ParseOptions{Scope: ScopeUser, ExpandVars: true, FilePath: "settings.json"})
	if err != nil {
		t.Fatalf("ParseMCPConfig: %v", err)
	}
	envtest := result.Servers["envtest"]
	if envtest.Command != "node" {
		t.Fatalf("command = %q", envtest.Command)
	}
	if envtest.Args[1] != "default-value" {
		t.Fatalf("default arg = %q", envtest.Args[1])
	}
	if envtest.Env["TOKEN"] != "secret" {
		t.Fatalf("env TOKEN = %q", envtest.Env["TOKEN"])
	}
	if result.Servers["remote"].Headers["Authorization"] != "Bearer secret" {
		t.Fatalf("remote header not expanded")
	}

	missing := errorMessages(result.Errors)
	if !strings.Contains(missing, "mcpServers.envtest") || !strings.Contains(missing, "MISSING") {
		t.Fatalf("missing envtest variable error not reported: %s", missing)
	}
	if !strings.Contains(missing, "mcpServers.remote") || !strings.Contains(missing, "HOST") {
		t.Fatalf("missing remote variable error not reported: %s", missing)
	}
}

func TestParseMCPConfigValidationErrorsMatchTaskBoundaries(t *testing.T) {
	data := []byte(`{
		"mcpServers": {
			"bad name": {"command": "node"},
			"claude-in-chrome": {"command": "node"},
			"missing-command": {"type": "stdio"},
			"bad-url": {"type": "http", "url": "://bad"},
			"bad-oauth": {
				"type": "sse",
				"url": "https://example.com/sse",
				"oauth": {"authServerMetadataUrl": "http://auth.example.com/.well-known/oauth-authorization-server"}
			}
		}
	}`)

	result, err := ParseMCPConfig(data, ParseOptions{Scope: ScopeProject})
	if err != nil {
		t.Fatalf("ParseMCPConfig: %v", err)
	}
	messages := errorMessages(result.Errors)
	for _, want := range []string{
		"Invalid name bad name",
		"Cannot add MCP server \"claude-in-chrome\": this name is reserved.",
		"Command cannot be empty",
		"Invalid URL",
		"authServerMetadataUrl must use https://",
	} {
		if !strings.Contains(messages, want) {
			t.Fatalf("errors missing %q:\n%s", want, messages)
		}
	}
}

func TestParseMCPConfigFileIncludesFileInErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"bad":{"type":"http","url":"://bad"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ParseMCPConfigFile(path, ParseOptions{Scope: ScopeManaged})
	if err != nil {
		t.Fatalf("ParseMCPConfigFile: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error")
	}
	if result.Errors[0].File != path {
		t.Fatalf("error file = %q, want %q", result.Errors[0].File, path)
	}
	if result.Servers["bad"].Scope != ScopeManaged {
		t.Fatalf("scope = %q", result.Servers["bad"].Scope)
	}
}

func TestParseMCPConfigRejectsInvalidJSONWithActionableError(t *testing.T) {
	_, err := ParseMCPConfig([]byte(`{invalid}`), ParseOptions{Scope: ScopeUser})
	if err == nil {
		t.Fatal("expected invalid JSON error")
	}
	if !strings.Contains(err.Error(), mcpText(i18n.KeyMCPValidationInvalidJSON)) {
		t.Fatalf("error = %v", err)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func errorMessages(errors []ValidationError) string {
	var b strings.Builder
	for _, err := range errors {
		if err.Path != "" {
			b.WriteString(err.Path)
			b.WriteString(": ")
		}
		b.WriteString(err.Message)
		b.WriteString("\n")
	}
	return b.String()
}
