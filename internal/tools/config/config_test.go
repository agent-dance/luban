package config

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

type memoryStore struct {
	values map[string]string
	err    error
}

func newMemoryStore() *memoryStore {
	return &memoryStore{values: make(map[string]string)}
}

func (s *memoryStore) Get(key string) (string, bool) {
	value, ok := s.values[key]
	return value, ok
}

func (s *memoryStore) Set(key, value string) error {
	if s.err != nil {
		return s.err
	}
	s.values[key] = value
	return nil
}

func TestConfigToolContract(t *testing.T) {
	tool := NewConfigTool(newMemoryStore())
	if got := tool.Name(); got != "Config" {
		t.Fatalf("Name() = %q", got)
	}
	if got := tool.Description(); got != i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolConfigDescription) {
		t.Fatalf("Description() = %q", got)
	}
	if metadata := tool.ToolMetadata(nil); metadata != (types.ToolMetadata{ConcurrencySafe: true}) {
		t.Fatalf("ToolMetadata() = %#v", metadata)
	}
	schema := tool.Schema()
	if len(schema.Required) != 1 || schema.Required[0] != "setting" {
		t.Fatalf("required fields = %#v", schema.Required)
	}
	if _, ok := schema.Properties["action"]; ok {
		t.Fatal("legacy action field remains in schema")
	}
	if _, ok := schema.Properties["key"]; ok {
		t.Fatal("legacy key field remains in schema")
	}
}

func TestConfigToolSetAndGet(t *testing.T) {
	store := newMemoryStore()
	tool := NewConfigTool(store)

	set, err := tool.Execute(context.Background(), map[string]any{
		"setting": "model",
		"value":   "deepseek-v4-pro",
	})
	if err != nil || set.IsError {
		t.Fatalf("set result = %#v, err = %v", set, err)
	}
	wantSet := i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolConfigUpdated, "model", "deepseek-v4-pro")
	if set.Content != wantSet {
		t.Fatalf("set content = %q, want %q", set.Content, wantSet)
	}

	get, err := tool.Execute(context.Background(), map[string]any{"setting": "model"})
	if err != nil || get.IsError {
		t.Fatalf("get result = %#v, err = %v", get, err)
	}
	wantGet := i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolConfigValue, "model", "deepseek-v4-pro")
	if get.Content != wantGet {
		t.Fatalf("get content = %q, want %q", get.Content, wantGet)
	}
}

func TestConfigToolGetMissingSetting(t *testing.T) {
	tool := NewConfigTool(newMemoryStore())
	result, err := tool.Execute(context.Background(), map[string]any{"setting": "theme"})
	if err != nil || result.IsError {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	want := i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolConfigNotSet, "theme")
	if result.Content != want {
		t.Fatalf("content = %q, want %q", result.Content, want)
	}
}

func TestConfigToolValuePresenceSelectsWrite(t *testing.T) {
	store := newMemoryStore()
	tool := NewConfigTool(store)

	result, err := tool.Execute(context.Background(), map[string]any{
		"setting": "theme",
		"value":   "",
	})
	if err != nil || result.IsError {
		t.Fatalf("empty write result = %#v, err = %v", result, err)
	}
	if got, ok := store.Get("theme"); !ok || got != "" {
		t.Fatalf("stored empty value = %q, %v", got, ok)
	}
}

func TestConfigToolRejectsExplicitNull(t *testing.T) {
	store := newMemoryStore()
	tool := NewConfigTool(store)
	result, err := tool.Execute(context.Background(), map[string]any{
		"setting": "theme",
		"value":   nil,
	})
	if err != nil || !result.IsError || result.Outcome != types.ToolOutcomeFailed {
		t.Fatalf("null result = %#v, err = %v", result, err)
	}
	if _, ok := store.Get("theme"); ok {
		t.Fatal("null value was persisted")
	}
}

func TestConfigToolRejectsLegacyFields(t *testing.T) {
	tool := NewConfigTool(newMemoryStore())
	for _, input := range []map[string]any{
		{"action": "get", "key": "theme"},
		{"setting": "theme", "action": "get"},
		{"setting": "theme", "key": "theme"},
	} {
		result, err := tool.Execute(context.Background(), input)
		if err != nil || !result.IsError {
			t.Fatalf("legacy input %#v result = %#v, err = %v", input, result, err)
		}
	}
}

func TestConfigToolRejectsEmptySetting(t *testing.T) {
	tool := NewConfigTool(newMemoryStore())
	result, err := tool.Execute(context.Background(), map[string]any{"setting": "  "})
	if err != nil || !result.IsError {
		t.Fatalf("empty setting result = %#v, err = %v", result, err)
	}
}

func TestConfigToolReportsSaveFailure(t *testing.T) {
	store := newMemoryStore()
	store.err = errors.New("disk unavailable")
	tool := NewConfigTool(store)
	result, err := tool.Execute(context.Background(), map[string]any{
		"setting": "theme",
		"value":   "dark",
	})
	if err != nil || !result.IsError {
		t.Fatalf("save failure result = %#v, err = %v", result, err)
	}
	if !strings.Contains(result.Content, store.err.Error()) {
		t.Fatalf("save failure content = %q", result.Content)
	}
}
