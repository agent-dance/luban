package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/brand"
)

// newTestConfigStore creates a ConfigStore backed by a temp file so tests
// never touch the real user config file.
func newTestConfigStore(t *testing.T) *ConfigStore {
	t.Helper()
	dir := t.TempDir()
	cs := &ConfigStore{
		path:     filepath.Join(dir, "config.json"),
		settings: make(map[string]string),
	}
	return cs
}

// ─── ConfigStore unit tests ───────────────────────────────────────────────────

func TestConfigStore_GetSetAll(t *testing.T) {
	cs := newTestConfigStore(t)

	// Get on missing key
	if _, ok := cs.Get("model"); ok {
		t.Fatal("expected missing key to return ok=false")
	}

	// Set and Get round-trip
	if err := cs.Set("model", "claude-3-5-sonnet"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	v, ok := cs.Get("model")
	if !ok {
		t.Fatal("expected key to be present after Set")
	}
	if v != "claude-3-5-sonnet" {
		t.Fatalf("got %q, want %q", v, "claude-3-5-sonnet")
	}

	// All returns all settings
	if err := cs.Set("theme", "dark"); err != nil {
		t.Fatalf("Set theme: %v", err)
	}
	all := cs.All()
	if all["model"] != "claude-3-5-sonnet" {
		t.Errorf("All model = %q", all["model"])
	}
	if all["theme"] != "dark" {
		t.Errorf("All theme = %q", all["theme"])
	}
}

func TestConfigStore_PersistsToDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cs1 := &ConfigStore{path: path, settings: make(map[string]string)}
	if err := cs1.Set("verbose", "true"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Load a second store from the same path — should see persisted value.
	cs2 := &ConfigStore{path: path, settings: make(map[string]string)}
	if err := cs2.load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	v, ok := cs2.Get("verbose")
	if !ok || v != "true" {
		t.Fatalf("expected verbose=true after reload, got ok=%v v=%q", ok, v)
	}
}

func TestConfigStore_MissingFileIsOK(t *testing.T) {
	dir := t.TempDir()
	cs := &ConfigStore{
		path:     filepath.Join(dir, "nonexistent", "config.json"),
		settings: make(map[string]string),
	}
	// load should not error on missing file
	if err := cs.load(); err != nil {
		t.Fatalf("unexpected error on missing file: %v", err)
	}
}

func TestNewConfigStore_CreatesDir(t *testing.T) {
	// Override home via env so NewConfigStore uses our temp dir.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")

	cs := NewConfigStore()
	if err := cs.Set("max_turns", "10"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	expected := filepath.Join(tmpHome, brand.ConfigDirName, "config.json")
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("expected config file at %s: %v", expected, err)
	}
}

// ─── ConfigTool Execute tests ─────────────────────────────────────────────────

func newConfigTool(t *testing.T) *ConfigTool {
	t.Helper()
	return NewConfigTool(newTestConfigStore(t))
}

func TestConfigTool_Name(t *testing.T) {
	tool := newConfigTool(t)
	if tool.Name() != "Config" {
		t.Errorf("Name() = %q, want Config", tool.Name())
	}
}

func TestConfigTool_SetAndGet(t *testing.T) {
	tool := newConfigTool(t)
	ctx := context.Background()

	// Set
	res, err := tool.Execute(ctx, map[string]any{"action": "set", "key": "model", "value": "claude-opus-4"})
	if err != nil {
		t.Fatalf("Execute set: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", res.Content)
	}
	if res.Content != "Config updated: model = claude-opus-4" {
		t.Errorf("set content = %q", res.Content)
	}

	// Get
	res, err = tool.Execute(ctx, map[string]any{"action": "get", "key": "model"})
	if err != nil {
		t.Fatalf("Execute get: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", res.Content)
	}
	if res.Content != "model = claude-opus-4" {
		t.Errorf("get content = %q", res.Content)
	}
}

func TestConfigTool_GetMissingKey(t *testing.T) {
	tool := newConfigTool(t)
	res, err := tool.Execute(context.Background(), map[string]any{"action": "get", "key": "theme"})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if res.Content != "(not set) theme" {
		t.Errorf("got %q", res.Content)
	}
}

func TestConfigTool_GetAllEmpty(t *testing.T) {
	tool := newConfigTool(t)
	res, err := tool.Execute(context.Background(), map[string]any{"action": "get", "key": ""})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if res.Content != "(no settings configured)" {
		t.Errorf("got %q", res.Content)
	}
}

func TestConfigTool_GetAll(t *testing.T) {
	tool := newConfigTool(t)
	ctx := context.Background()
	_ = tool.Store.Set("model", "claude-3-haiku")
	_ = tool.Store.Set("theme", "light")
	_ = tool.Store.Set("custom_key", "custom_val")

	res, err := tool.Execute(ctx, map[string]any{"action": "get", "key": ""})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	// All three keys must appear
	for _, want := range []string{"model = claude-3-haiku", "theme = light", "custom_key = custom_val"} {
		if !contains(res.Content, want) {
			t.Errorf("output missing %q:\n%s", want, res.Content)
		}
	}
}

func TestConfigTool_SetUnknownKey(t *testing.T) {
	tool := newConfigTool(t)
	res, err := tool.Execute(context.Background(), map[string]any{"action": "set", "key": "my_custom_flag", "value": "yes"})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unknown key should be allowed, got error: %s", res.Content)
	}
	v, ok := tool.Store.Get("my_custom_flag")
	if !ok || v != "yes" {
		t.Errorf("unknown key not stored: ok=%v v=%q", ok, v)
	}
}

func TestConfigTool_SetMissingKey(t *testing.T) {
	tool := newConfigTool(t)
	res, err := tool.Execute(context.Background(), map[string]any{"action": "set", "key": ""})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected error when key is empty for set")
	}
}

func TestConfigTool_UnknownAction(t *testing.T) {
	tool := newConfigTool(t)
	res, err := tool.Execute(context.Background(), map[string]any{"action": "delete", "key": "model"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected error for unknown action")
	}
}

func TestConfigTool_InvalidInput(t *testing.T) {
	tool := newConfigTool(t)
	// Passing something unparseable (nested object where string expected)
	res, err := tool.Execute(context.Background(), map[string]any{"action": map[string]any{"bad": true}, "key": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected error for bad input type")
	}
}

func TestConfigTool_IsConcurrentSafe(t *testing.T) {
	tool := newConfigTool(t)
	if !tool.IsConcurrentSafe() {
		t.Error("ConfigTool should be concurrent-safe")
	}
}

// contains is a simple substring helper for test assertions.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && stringContains(s, sub))
}

func stringContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
