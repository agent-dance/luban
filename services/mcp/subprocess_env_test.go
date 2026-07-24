package mcp

import (
	"strings"
	"testing"
)

func TestSubprocessEnvInheritsProxyAndScrubsSensitiveActionsSecrets(t *testing.T) {
	t.Setenv("CLAUDE_CODE_SUBPROCESS_ENV_SCRUB", "yes")
	t.Setenv("ANTHROPIC_API_KEY", "secret")
	t.Setenv("INPUT_ANTHROPIC_API_KEY", "duplicated-secret")
	t.Setenv("ACTIONS_RUNTIME_TOKEN", "runtime-secret")
	t.Setenv("GH_TOKEN", "allowed-gh-token")
	t.Setenv("MCP_PARENT_ENV", "parent")

	restore := RegisterSubprocessProxyEnvFunc(func() map[string]string {
		return map[string]string{
			"HTTPS_PROXY":   "http://127.0.0.1:9999",
			"SSL_CERT_FILE": "/tmp/ca.pem",
		}
	})
	defer restore()

	env := SubprocessEnv()
	if _, ok := env["ANTHROPIC_API_KEY"]; ok {
		t.Fatalf("ANTHROPIC_API_KEY was not scrubbed")
	}
	if _, ok := env["INPUT_ANTHROPIC_API_KEY"]; ok {
		t.Fatalf("INPUT_ANTHROPIC_API_KEY was not scrubbed")
	}
	if _, ok := env["ACTIONS_RUNTIME_TOKEN"]; ok {
		t.Fatalf("ACTIONS_RUNTIME_TOKEN was not scrubbed")
	}
	if got := env["GH_TOKEN"]; got != "allowed-gh-token" {
		t.Fatalf("GH_TOKEN = %q, want preserved token", got)
	}
	if got := env["MCP_PARENT_ENV"]; got != "parent" {
		t.Fatalf("MCP_PARENT_ENV = %q, want inherited parent", got)
	}
	if got := env["HTTPS_PROXY"]; got != "http://127.0.0.1:9999" {
		t.Fatalf("HTTPS_PROXY = %q, want proxy env merged", got)
	}
}

func TestBuildSubprocessEnvAppliesServerOverridesAfterScrub(t *testing.T) {
	t.Setenv("CLAUDE_CODE_SUBPROCESS_ENV_SCRUB", "true")
	t.Setenv("ANTHROPIC_API_KEY", "parent-secret")
	t.Setenv("MCP_PARENT_ENV", "parent")

	list := BuildSubprocessEnv(map[string]string{
		"ANTHROPIC_API_KEY": "server-secret",
		"MCP_PARENT_ENV":    "server-override",
		"MCP_SERVER_ONLY":   "1",
	})
	env := environToMap(list)

	if got := env["ANTHROPIC_API_KEY"]; got != "server-secret" {
		t.Fatalf("ANTHROPIC_API_KEY = %q, want server override after scrub", got)
	}
	if got := env["MCP_PARENT_ENV"]; got != "server-override" {
		t.Fatalf("MCP_PARENT_ENV = %q, want server override", got)
	}
	if got := env["MCP_SERVER_ONLY"]; got != "1" {
		t.Fatalf("MCP_SERVER_ONLY = %q, want server var", got)
	}
}

func TestSubprocessEnvTruthyMatchesTypeScriptTruthySet(t *testing.T) {
	for _, value := range []string{"1", "true", "TRUE", " yes ", "on"} {
		if !subprocessEnvTruthy(value) {
			t.Fatalf("%q should be truthy", value)
		}
	}
	for _, value := range []string{"", "0", "false", "no", "off", "anything"} {
		if subprocessEnvTruthy(value) {
			t.Fatalf("%q should not be truthy", value)
		}
	}
}

func TestEnvMapToListIsDeterministic(t *testing.T) {
	list := envMapToList(map[string]string{"B": "2", "A": "1"})
	if got := strings.Join(list, ","); got != "A=1,B=2" {
		t.Fatalf("env list = %q, want sorted output", got)
	}
}
