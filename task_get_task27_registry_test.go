package main

import (
	"testing"

	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/sandbox"
)

func TestTaskGetTask27SetupRegistryRegistrationGate(t *testing.T) {
	tests := []struct {
		name        string
		interactive bool
		enableTasks string
		want        bool
	}{
		{name: "interactive", interactive: true, want: true},
		{name: "non-interactive", interactive: false, want: false},
		{name: "non-interactive forced", interactive: false, enableTasks: "true", want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CLAUDE_CODE_ENABLE_TASKS", tc.enableTasks)
			deps := SetupRegistry(provider.NewProviderRef(nil), t.TempDir(), nil, sandbox.NoopBackend{}, nil, tc.interactive)
			if deps.CronStore != nil {
				t.Cleanup(deps.CronStore.Stop)
			}
			for _, name := range []string{"TaskCreate", "TaskGet", "TaskUpdate", "TaskList"} {
				if got := deps.Registry.Get(name) != nil; got != tc.want {
					t.Errorf("Registry.Get(%q) present=%v, want %v", name, got, tc.want)
				}
			}
			if deps.Registry.Get("TaskStop") == nil || deps.Registry.Get("TaskOutput") == nil {
				t.Fatal("TaskStop and TaskOutput must remain registration-gate independent")
			}
		})
	}
}
