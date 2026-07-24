package hooks

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadConfigRichEventMapHasStableCanonicalConfigIdentity(t *testing.T) {
	data := []byte(`{
		"hooks": {
			"UserPromptSubmit": [{"hooks": [{"type": "command", "command": "true"}]}],
			"PostQuery": [{"hooks": [{"type": "command", "command": "true"}]}],
			"PreQuery": [{"hooks": [
				{"type": "command", "command": "true"},
				{"type": "command", "command": "printf second"}
			]}]
		}
	}`)
	wantOrder := []HookType{HookPostQuery, HookPreQuery, HookPreQuery, HookUserPromptSubmit}
	wantConfigIDs := []string{"config-1", "config-2", "config-3", "config-4"}
	var wantExecutionIDs []string

	for reload := 0; reload < 64; reload++ {
		runner, err := LoadConfigData(data, "stable-settings.json")
		if err != nil {
			t.Fatalf("reload %d: %v", reload, err)
		}
		gotOrder := make([]HookType, len(runner.hooks))
		for i := range runner.hooks {
			gotOrder[i] = runner.hooks[i].Type
		}
		if !reflect.DeepEqual(gotOrder, wantOrder) {
			t.Fatalf("reload %d hook order = %v, want canonical %v", reload, gotOrder, wantOrder)
		}

		var gotConfigIDs, gotExecutionIDs []string
		for _, hookType := range []HookType{HookPostQuery, HookPreQuery, HookUserPromptSubmit} {
			for _, execution := range runner.RunDetailed(t.Context(), hookType, HookInput{
				SessionID: "session-stable", TurnID: "turn-stable", WorkUnitID: "work-stable", AgentID: "actor-stable",
			}) {
				gotConfigIDs = append(gotConfigIDs, execution.ConfigID)
				gotExecutionIDs = append(gotExecutionIDs, execution.ExecutionID)
				if execution.ExecutionID != execution.Input.HookExecutionID {
					t.Fatalf("reload %d execution/input ID diverged: %#v", reload, execution)
				}
			}
		}
		if !reflect.DeepEqual(gotConfigIDs, wantConfigIDs) {
			t.Fatalf("reload %d config IDs = %v, want %v", reload, gotConfigIDs, wantConfigIDs)
		}
		if reload == 0 {
			wantExecutionIDs = append([]string(nil), gotExecutionIDs...)
		} else if !reflect.DeepEqual(gotExecutionIDs, wantExecutionIDs) {
			t.Fatalf("reload %d execution IDs = %v, want stable %v", reload, gotExecutionIDs, wantExecutionIDs)
		}
	}
}

func TestLoadConfigLegacyFlatArray(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	os.WriteFile(p, []byte(`{
		"hooks": [
			{"type": "PreToolUse", "command": "echo hi", "timeout": 5},
			{"type": "PostToolUse", "kind": "http", "url": "https://example.com/hook"}
		]
	}`), 0644)

	runner, err := LoadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.hooks) != 2 {
		t.Fatalf("expected 2 hooks, got %d", len(runner.hooks))
	}
	if runner.hooks[0].Type != HookPreToolUse {
		t.Errorf("expected PreToolUse, got %s", runner.hooks[0].Type)
	}
	if runner.hooks[1].Kind != HookKindHTTP {
		t.Errorf("expected http kind, got %s", runner.hooks[1].Kind)
	}
}

func TestLoadConfigRichEventMap(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	os.WriteFile(p, []byte(`{
		"hooks": {
			"PreToolUse": [
				{
					"matcher": "Bash",
					"hooks": [
						{"type": "command", "command": "echo check", "timeout": 10}
					]
				}
			],
			"Notification": [
				{
					"hooks": [
						{"type": "notification"}
					]
				}
			],
			"PostToolUse": [
				{
					"hooks": [
						{
							"type": "http",
							"url": "https://example.com/post-hook",
							"headers": {"Authorization": "Bearer token"},
							"retry_count": 2
						}
					]
				}
			],
			"PreCompact": [
				{
					"hooks": [
						{"type": "command", "command": "echo precompact"}
					]
				}
			]
		}
	}`), 0644)

	runner, err := LoadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.hooks) != 4 {
		t.Fatalf("expected 4 hooks, got %d", len(runner.hooks))
	}

	var hasPreToolUse, hasNotification, hasPostToolUse, hasPreCompact bool
	for _, h := range runner.hooks {
		switch h.Type {
		case HookPreToolUse:
			hasPreToolUse = true
			if h.Kind != HookKindCommand {
				t.Errorf("PreToolUse: expected command kind, got %s", h.Kind)
			}
			if h.Matcher != "Bash" {
				t.Errorf("expected matcher=Bash, got %q", h.Matcher)
			}
			if h.Command != "echo check" {
				t.Errorf("expected command='echo check', got %q", h.Command)
			}
		case HookNotification:
			hasNotification = true
			if h.Kind != HookKindNotification {
				t.Errorf("Notification: expected notification kind, got %s", h.Kind)
			}
		case HookPostToolUse:
			hasPostToolUse = true
			if h.Kind != HookKindHTTP {
				t.Errorf("PostToolUse: expected http kind, got %s", h.Kind)
			}
			if h.URL != "https://example.com/post-hook" {
				t.Errorf("expected URL, got %q", h.URL)
			}
			if h.RetryCount != 2 {
				t.Errorf("expected retry_count=2, got %d", h.RetryCount)
			}
		case HookPreCompact:
			hasPreCompact = true
			if h.Command != "echo precompact" {
				t.Errorf("expected precompact command, got %q", h.Command)
			}
		}
	}
	if !hasPreToolUse {
		t.Error("missing PreToolUse hook")
	}
	if !hasNotification {
		t.Error("missing Notification hook")
	}
	if !hasPostToolUse {
		t.Error("missing PostToolUse hook")
	}
	if !hasPreCompact {
		t.Error("missing PreCompact hook")
	}
}

func TestLoadConfigTeammateAndTaskCompletedHooks(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	os.WriteFile(p, []byte(`{
		"hooks": {
			"TeammateIdle": [
				{"hooks": [{"type": "command", "command": "echo idle"}]}
			],
			"TaskCompleted": [
				{"hooks": [{"type": "command", "command": "echo completed"}]}
			]
		}
	}`), 0644)

	runner, err := LoadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if !runner.HasHooks(HookTeammateIdle) {
		t.Fatal("expected TeammateIdle hook to load")
	}
	if !runner.HasHooks(HookTaskCompleted) {
		t.Fatal("expected TaskCompleted hook to load")
	}
}

func TestHookTypeFromFilenameTeammateAndTaskCompleted(t *testing.T) {
	tests := map[string]HookType{
		"teammate-idle-notify.sh": HookTeammateIdle,
		"teammateidle-notify.sh":  HookTeammateIdle,
		"task-completed-check.sh": HookTaskCompleted,
		"taskcompleted-check.sh":  HookTaskCompleted,
	}
	for name, want := range tests {
		if got := hookTypeFromFilename(strings.TrimSuffix(name, ".sh")); got != want {
			t.Fatalf("hookTypeFromFilename(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestLoadConfigNonexistentFile(t *testing.T) {
	runner, err := LoadConfig("/nonexistent/path/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.hooks) != 0 {
		t.Error("expected empty runner for nonexistent file")
	}
}

func TestLoadConfigInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	os.WriteFile(p, []byte(`{not valid json`), 0644)

	_, err := LoadConfig(p)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadConfigEmptyHooks(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	os.WriteFile(p, []byte(`{"other_setting": "value"}`), 0644)

	runner, err := LoadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.hooks) != 0 {
		t.Error("expected empty runner when no hooks key")
	}
}

// TestHookConfigToHookUnknownKind verifies that hookConfigToHook returns an
// error for an unrecognised hook kind (C1).
func TestHookConfigToHookUnknownKind(t *testing.T) {
	hc := HookConfig{
		Type:    "websocket", // not a known kind
		Command: "echo hi",
	}
	_, err := hookConfigToHook(HookPreToolUse, "", hc)
	if err == nil {
		t.Fatal("expected error for unknown hook kind, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unknown hook kind") {
		t.Errorf("expected 'unknown hook kind' in error, got: %v", err)
	}
}

// TestHookConfigToHookKnownKinds verifies that all documented kinds are
// accepted without error (C1).
func TestHookConfigToHookKnownKinds(t *testing.T) {
	knownKinds := []string{"command", "http", "notification", ""}
	for _, k := range knownKinds {
		hc := HookConfig{Type: k, Command: "echo", URL: "https://example.com"}
		h, err := hookConfigToHook(HookPreToolUse, "", hc)
		if err != nil {
			t.Errorf("kind %q: unexpected error: %v", k, err)
		}
		// Empty kind must be normalised to command.
		if k == "" && h.Kind != HookKindCommand {
			t.Errorf("empty kind: expected HookKindCommand normalisation, got %q", h.Kind)
		}
	}
}

// TestLoadConfigUnknownEventTypeWarning verifies that an unknown event type in
// the rich format logs a warning but does NOT cause LoadConfig to return an
// error (C2 forward-compatibility).
func TestLoadConfigUnknownEventTypeWarning(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	// "FutureEvent" is not in validHookTypes.
	os.WriteFile(p, []byte(`{
		"hooks": {
			"FutureEvent": [
				{"hooks": [{"type": "command", "command": "echo future"}]}
			],
			"PreToolUse": [
				{"hooks": [{"type": "command", "command": "echo known"}]}
			]
		}
	}`), 0644)

	runner, err := LoadConfig(p)
	if err != nil {
		t.Fatalf("expected no error for unknown event type, got: %v", err)
	}
	// Only the known PreToolUse hook should be loaded; FutureEvent is skipped.
	if len(runner.hooks) != 1 {
		t.Errorf("expected 1 hook (unknown type skipped), got %d", len(runner.hooks))
	}
	if runner.hooks[0].Type != HookPreToolUse {
		t.Errorf("expected PreToolUse hook, got %s", runner.hooks[0].Type)
	}
}

// TestLoadConfigUnknownKindInRichFormat verifies that an unknown hook kind
// inside the rich event-map format causes LoadConfig to return an error (C1).
func TestLoadConfigUnknownKindInRichFormat(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	os.WriteFile(p, []byte(`{
		"hooks": {
			"PreToolUse": [
				{"hooks": [{"type": "websocket", "command": "echo hi"}]}
			]
		}
	}`), 0644)

	_, err := LoadConfig(p)
	if err == nil {
		t.Fatal("expected error for unknown hook kind in rich format, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unknown hook kind") {
		t.Errorf("expected 'unknown hook kind' in error, got: %v", err)
	}
}
