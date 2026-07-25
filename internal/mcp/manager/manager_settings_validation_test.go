package manager

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/internal/mcp/catalog"
)

func TestManagerLoadFromSettingsRejectsFatalValidationAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	data := []byte(`{"mcpServers":{"valid":{"type":"stdio","command":"node"},"invalid":{"type":"stdio","command":""}}}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	manager := NewManager()
	err := manager.LoadFromSettings(path)
	var validationErr *catalog.FatalConfigValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("LoadFromSettings() error = %v, want catalog.FatalConfigValidationError", err)
	}
	if validationErr.Validation.MCPErrorMetadata.ServerName != "invalid" {
		t.Fatalf("fatal server = %q, want invalid", validationErr.Validation.MCPErrorMetadata.ServerName)
	}
	if names := manager.ServerNames(); len(names) != 0 {
		t.Fatalf("fatal settings were partially registered: %v", names)
	}
}

func TestManagerLoadFromSettingsAllowsWarnings(t *testing.T) {
	const variable = "LUBAN_TEST_MCP_LOAD_WARNING_MISSING"
	unsetEnvForTest(t, variable)
	path := filepath.Join(t.TempDir(), "settings.json")
	data := []byte(`{"mcpServers":{"warning-only":{"type":"stdio","command":"${LUBAN_TEST_MCP_LOAD_WARNING_MISSING}"}}}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	manager := NewManager()
	if err := manager.LoadFromSettings(path); err != nil {
		t.Fatalf("LoadFromSettings() warning = %v", err)
	}
	if _, ok := manager.State("warning-only"); !ok {
		t.Fatal("warning-only server was not registered")
	}
}

func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()
	value, existed := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, value)
			return
		}
		_ = os.Unsetenv(key)
	})
}
