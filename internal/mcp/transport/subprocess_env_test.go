package transport

import (
	"strings"
	"testing"
)

func TestSubprocessEnvScrubsSensitiveActionsSecrets(t *testing.T) {
	t.Setenv("LUBAN_CODE_SUBPROCESS_ENV_SCRUB", "yes")
	t.Setenv("LUBAN_CODE_OAUTH_TOKEN", "secret")
	t.Setenv("INPUT_LUBAN_CODE_OAUTH_TOKEN", "duplicated-secret")
	t.Setenv("ACTIONS_RUNTIME_TOKEN", "runtime-secret")
	t.Setenv("GH_TOKEN", "allowed-gh-token")
	t.Setenv("MCP_PARENT_ENV", "parent")

	env := subprocessEnv()
	if _, ok := env["LUBAN_CODE_OAUTH_TOKEN"]; ok {
		t.Fatalf("LUBAN_CODE_OAUTH_TOKEN was not scrubbed")
	}
	if _, ok := env["INPUT_LUBAN_CODE_OAUTH_TOKEN"]; ok {
		t.Fatalf("INPUT_LUBAN_CODE_OAUTH_TOKEN was not scrubbed")
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
}

func TestBuildSubprocessEnvAppliesServerOverridesAfterScrub(t *testing.T) {
	t.Setenv("LUBAN_CODE_SUBPROCESS_ENV_SCRUB", "true")
	t.Setenv("LUBAN_CODE_OAUTH_TOKEN", "parent-secret")
	t.Setenv("MCP_PARENT_ENV", "parent")

	list := buildSubprocessEnv(map[string]string{
		"LUBAN_CODE_OAUTH_TOKEN": "server-secret",
		"MCP_PARENT_ENV":         "server-override",
		"MCP_SERVER_ONLY":        "1",
	})
	env := environToMap(list)

	if got := env["LUBAN_CODE_OAUTH_TOKEN"]; got != "server-secret" {
		t.Fatalf("LUBAN_CODE_OAUTH_TOKEN = %q, want server override after scrub", got)
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
