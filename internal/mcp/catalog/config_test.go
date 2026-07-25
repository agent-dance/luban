package catalog

import (
	"encoding/json"
	"errors"
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

func TestParseMCPConfigAcceptsRemoteHeadersOAuthAndIDEShapes(t *testing.T) {
	data := []byte(`{
		"mcpServers": {
			"sse": {
				"type": "sse",
				"url": "https://example.com/sse",
				"headers": {"Authorization": "Bearer ${TOKEN:-fallback}"},
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
				"headers": {"X-Trace": "1"}
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
				"ideName": "vscode"
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
	if sse.OAuth == nil || sse.OAuth.ClientID != "client-1" || sse.OAuth.CallbackPort == nil || *sse.OAuth.CallbackPort != 8765 {
		t.Fatalf("oauth fields not preserved: %#v", sse.OAuth)
	}
	if sse.OAuth.XAA == nil || !*sse.OAuth.XAA {
		t.Fatalf("oauth xaa not preserved: %#v", sse.OAuth)
	}
	if result.Servers["sseide"].IDERunningInWindows == nil || !*result.Servers["sseide"].IDERunningInWindows {
		t.Fatalf("IDE fields not preserved")
	}
}

func TestParseMCPConfigPreservesDisabledServers(t *testing.T) {
	data := []byte(`{
		"disabledMcpServers": ["github"],
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
			"missing-command": {"type": "stdio"},
			"bad-transport": {"type": "bogus"},
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
		catalogFormat(i18n.KeyMCPValidationServerNameInvalid, "bad name"),
		catalogText(i18n.KeyMCPValidationCommandEmpty),
		catalogFormat(i18n.KeyMCPValidationTransportInvalid, TransportType("bogus")),
		catalogFormat(i18n.KeyMCPValidationURLInvalid, "://bad"),
		catalogText(i18n.KeyMCPValidationMetadataHTTPS),
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

	result, err := ParseMCPConfigFile(path, ParseOptions{Scope: ScopeProject})
	if err != nil {
		t.Fatalf("ParseMCPConfigFile: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error")
	}
	if result.Errors[0].File != path {
		t.Fatalf("error file = %q, want %q", result.Errors[0].File, path)
	}
	if result.Servers["bad"].Scope != ScopeProject {
		t.Fatalf("scope = %q", result.Servers["bad"].Scope)
	}
}

func TestParseMCPConfigRejectsInvalidJSONWithActionableError(t *testing.T) {
	_, err := ParseMCPConfig([]byte(`{invalid}`), ParseOptions{Scope: ScopeUser})
	if err == nil {
		t.Fatal("expected invalid JSON error")
	}
	var syntaxError *json.SyntaxError
	if !errors.As(err, &syntaxError) {
		t.Fatalf("error %T does not preserve json.SyntaxError: %v", err, err)
	}
	if got, want := err.Error(), catalogFormat(i18n.KeyMCPValidationInvalidJSONCause, syntaxError); got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
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
