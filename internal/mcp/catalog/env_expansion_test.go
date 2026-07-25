package catalog

import (
	"reflect"
	"testing"
)

func TestExpandEnvVarsInStringDefaultsAndDeduplicatesMissing(t *testing.T) {
	t.Setenv("CATALOG_TOKEN", "secret")
	result := expandEnvVarsInString("${CATALOG_TOKEN}:${CATALOG_MISSING}:${CATALOG_OPTIONAL:-fallback}:${CATALOG_MISSING}")
	if result.Expanded != "secret:${CATALOG_MISSING}:fallback:${CATALOG_MISSING}" {
		t.Fatalf("expanded = %q", result.Expanded)
	}
	if !reflect.DeepEqual(result.MissingVars, []string{"CATALOG_MISSING"}) {
		t.Fatalf("missing variables = %#v", result.MissingVars)
	}
}

func TestExpandEnvVarsInConfigCoversLocalAndRemoteFields(t *testing.T) {
	t.Setenv("CATALOG_VALUE", "resolved")
	local, missing := expandEnvVarsInConfig(MCPServerConfig{
		Command: "${CATALOG_VALUE}",
		Args:    []string{"${CATALOG_MISSING}"},
		Env:     map[string]string{"VALUE": "${CATALOG_VALUE}"},
	})
	if local.Type != TransportStdio || local.Command != "resolved" || local.Env["VALUE"] != "resolved" {
		t.Fatalf("local expansion = %#v", local)
	}
	if !reflect.DeepEqual(missing, []string{"CATALOG_MISSING"}) {
		t.Fatalf("local missing variables = %#v", missing)
	}

	for _, transport := range []TransportType{TransportSSE, TransportSSEIDE, TransportHTTP, TransportWebSocket, TransportWebSocketIDE} {
		remote, remoteMissing := expandEnvVarsInConfig(MCPServerConfig{
			Type: transport, URL: "https://${CATALOG_VALUE}.example/mcp",
			Headers: map[string]string{"Authorization": "Bearer ${CATALOG_VALUE}"},
		})
		if remote.URL != "https://resolved.example/mcp" || remote.Headers["Authorization"] != "Bearer resolved" || len(remoteMissing) != 0 {
			t.Fatalf("%s expansion = %#v, missing = %#v", transport, remote, remoteMissing)
		}
	}
}
