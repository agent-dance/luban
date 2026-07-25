package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/sandbox"
)

func TestRegistrySetupPropagatesInitialMCPSettingsError(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, brand.ConfigDirName)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(configDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"mcpServers":`), 0o600); err != nil {
		t.Fatal(err)
	}

	deps := SetupRegistry(provider.NewProviderRef(nil), root, []string{root}, sandbox.NoopBackend{}, nil)
	defer stopScheduleForTest(t, deps)
	defer deps.StopWebFetchCache()
	defer stopMCPRuntimeBridgeForTest(t, deps)

	// Replacing the file proves the returned error came from SetupRegistry's
	// Manager.LoadFromSettings call instead of the later preparation read.
	if err := os.WriteFile(settingsPath, []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	err := prepareInitialRegistryRuntime(deps, root, []string{root})
	if err == nil {
		t.Fatal("prepareInitialRegistryRuntime() succeeded after initial MCP settings failure")
	}
	if !strings.Contains(err.Error(), settingsPath) {
		t.Fatalf("initialization error omitted settings path: %v", err)
	}
}

func TestLoadWorkspaceMCPConfigsAllowsWarnings(t *testing.T) {
	const variable = "LUBAN_TEST_APP_MCP_WARNING_MISSING"
	unsetRegistryEnvForTest(t, variable)
	root := t.TempDir()
	configDir := filepath.Join(root, brand.ConfigDirName)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	settings := []byte(`{"mcpServers":{"warning-only":{"type":"stdio","command":"${LUBAN_TEST_APP_MCP_WARNING_MISSING}"}}}`)
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), settings, 0o600); err != nil {
		t.Fatal(err)
	}

	configs, err := loadWorkspaceMCPConfigs(root)
	if err != nil {
		t.Fatalf("loadWorkspaceMCPConfigs() warning = %v", err)
	}
	if _, ok := configs["warning-only"]; !ok {
		t.Fatalf("warning-only server missing from configs: %#v", configs)
	}
}

func unsetRegistryEnvForTest(t *testing.T, key string) {
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
