package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCheckedInSchemaAndTemplatesAreJSONAndTemplateLoadsAfterLocking(t *testing.T) {
	root := benchmarkAgenticRoot(t)
	for _, relative := range []string{
		"schema/manifest.schema.json",
		"manifests/deepswe-v1.1.template.json",
		"manifests/deepswe-v1.1-pilot.template.json",
		"manifests/deepswe-v1.1-pilot-selection.json",
	} {
		raw, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			t.Fatal(err)
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatalf("%s is not valid JSON: %v", relative, err)
		}
	}

	fixture := fixtureManifest(t)
	replacements := map[string]string{
		"${DEEPSWE_TREE_SHA256}":                             strings.Repeat("a", 64),
		"${DEEPSWE_RESOLVED_TASK_INVENTORY_SHA256}":          strings.Repeat("b", 64),
		"${PIER_TREE_SHA256}":                                strings.Repeat("c", 64),
		"${PIER_RUNTIME_MANIFEST_SHA256}":                    strings.Repeat("d", 64),
		"${PIER_BINARY_SHA256}":                              strings.Repeat("f", 64),
		"${ABSOLUTE_CODEX_BINARY}":                           fixture.Agents[0].Binary,
		"${CODEX_BINARY_SHA256}":                             fixture.Agents[0].BinarySHA256,
		"${CODEX_V8_CANARY_RECEIPT_SHA256}":                  strings.Repeat("4", 64),
		"${CODEX_TOOL_CATALOG_SEMANTIC_SHA256}":              strings.Repeat("5", 64),
		"${CODEX_TOOL_EXEC_DEFINITION_SHA256}":               strings.Repeat("6", 64),
		"${CODEX_TOOL_WAIT_DEFINITION_SHA256}":               strings.Repeat("7", 64),
		"${CODEX_TOOL_REQUEST_USER_INPUT_DEFINITION_SHA256}": strings.Repeat("8", 64),
		"${ABSOLUTE_LUBAN_BINARY}":                           fixture.Agents[1].Binary,
		"${LUBAN_BINARY_SHA256}":                             fixture.Agents[1].BinarySHA256,
		"${LUBAN_V8_CANARY_RECEIPT_SHA256}":                  strings.Repeat("9", 64),
		"${LUBAN_TOOL_CATALOG_SEMANTIC_SHA256}":              strings.Repeat("a", 64),
		"${LUBAN_TOOL_INSPECT_DEFINITION_SHA256}":            strings.Repeat("b", 64),
		"${LUBAN_TOOL_APPLY_PATCH_DEFINITION_SHA256}":        strings.Repeat("c", 64),
		"${LUBAN_TOOL_RUN_DEFINITION_SHA256}":                strings.Repeat("d", 64),
		"${ABSOLUTE_LUBAN_WORKTREE}":                         t.TempDir(),
		"${LUBAN_SOURCE_BASE_COMMIT}":                        strings.Repeat("e", 40),
		"${LUBAN_SOURCE_TREE_OID}":                           strings.Repeat("d", 40),
		"${LUBAN_SOURCE_PATCH_SHA256}":                       strings.Repeat("1", 64),
		"${LUBAN_SOURCE_ARCHIVE_SHA256}":                     strings.Repeat("2", 64),
		"${ABSOLUTE_LUBAN_BUILD_RECEIPT}":                    filepath.Join(t.TempDir(), "build-receipt.json"),
		"${LUBAN_BUILD_RECEIPT_SHA256}":                      strings.Repeat("3", 64),
	}
	for _, name := range []string{"deepswe-v1.1.template.json", "deepswe-v1.1-pilot.template.json"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(root, "manifests", name))
			if err != nil {
				t.Fatal(err)
			}
			locked := string(raw)
			for placeholder, value := range replacements {
				locked = strings.ReplaceAll(locked, placeholder, value)
			}
			if strings.Contains(locked, "${") {
				t.Fatalf("test did not replace all manifest placeholders")
			}
			lockedPath := filepath.Join(t.TempDir(), "locked.json")
			if err := os.WriteFile(lockedPath, []byte(locked), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadManifest(lockedPath); err != nil {
				t.Fatalf("locked template does not satisfy Go manifest contract: %v", err)
			}
		})
	}
}

func benchmarkAgenticRoot(t testing.TB) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate benchmark package")
	}
	return filepath.Dir(filepath.Dir(filename))
}
